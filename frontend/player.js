import { t } from './i18n.js';
import { colors, playerById, periodById } from './state.js';
import { el, clock, cssVar, periodName } from './ui.js';

// ---- file time <-> demo time (segments: {out, len, demo} in seconds) ----

/** Map preview-file time `sec` into demo time using `segments`. */
export function fileToDemo(segments, sec) {
  const segs = segments || [];
  for (const g of segs) {
    if (sec < g.out) return g.demo;
    if (sec <= g.out + g.len) return g.demo + (sec - g.out);
  }
  const last = segs[segs.length - 1];
  return last ? last.demo + last.len : 0;
}

/** Map a demo-time span `[startDemo, endDemo]` into preview-file pieces `[{s, e}]`. */
export function demoToFile(segments, startDemo, endDemo) {
  const out = [];
  for (const g of segments || []) {
    const lo = Math.max(startDemo, g.demo);
    const hi = Math.min(endDemo, g.demo + g.len);
    if (hi > lo) out.push({ s: g.out + (lo - g.demo), e: g.out + (hi - g.demo) });
  }
  return out;
}

/** Choose a comfortable canvas height (px) for `lanes` stacked speaker tracks. */
export function waveHeight(lanes) {
  if (lanes <= 1) return 76;
  if (lanes === 2) return 84;
  if (lanes === 3) return 96;
  if (lanes === 4) return 108;
  if (lanes === 5) return 120;
  return Math.min(160, Math.max(120, lanes * 18));
}

function decodeBase64Bytes(b64) {
  const s = atob(b64 || '');
  const a = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
  return a;
}

export function makePlayer(root, opts = {}) {
  root.innerHTML = '';
  const ctl = el('div', 'pctl');
  const play = el('button', 'play', '▶');
  play.disabled = true;
  const time = el('span', 'ptime', '0:00 / 0:00');
  const where = el('span', 'pwhere', '');
  const status = el('span', 'pstatus', '');
  ctl.append(play, time, where, status);
  const canvas = el('canvas', 'wave');
  const legend = el('div', 'legend');
  root.append(ctl, canvas);

  const laneCfg = opts.marks
    ? {
        api: opts.marks,
        laneCls: 'marklane',
        pillCls: 'mark',
        btnCls: 'btn markbtn',
        colorVar: '--mark',
        btnKey: 'marks.button',
        endKey: 'marks.end',
        hintKey: 'marks.hint',
        tipKey: 'marks.tip',
        removeKey: 'marks.remove',
        shiftCanvas: false,
      }
    : opts.cuts
    ? {
        api: opts.cuts,
        laneCls: 'cutlane',
        pillCls: 'mark cut',
        btnCls: 'btn markbtn cut',
        colorVar: '--cut',
        btnKey: 'cut.markBtn',
        endKey: 'cut.markEnd',
        hintKey: 'cut.laneHint',
        tipKey: 'cut.rangeTip',
        removeKey: 'cut.remove',
        shiftCanvas: true,
      }
    : null;

  const mk = laneCfg && laneCfg.api;
  let lane = null;
  let markBtn = null;
  let pending = null; // demo time where a mark started (Mark button)
  if (laneCfg) {
    markBtn = el('button', laneCfg.btnCls, '');
    markBtn.disabled = true;
    ctl.insertBefore(markBtn, status);
    lane = el('div', laneCfg.laneCls);
    root.append(lane);
  }
  root.append(legend);

  const audioEl = new Audio();
  let pv = null;
  let peaks = [];
  let hoveredLane = -1;

  play.onclick = () => { audioEl.paused ? audioEl.play() : audioEl.pause(); };
  audioEl.onplay = () => { play.textContent = '❚❚'; };
  audioEl.onpause = () => { play.textContent = '▶'; };
  audioEl.onended = () => { play.textContent = '▶'; };
  audioEl.ontimeupdate = () => tick();

  const segs = () => (pv && pv.segments) || [];
  const xToTime = clientX => {
    const r = canvas.getBoundingClientRect();
    return Math.min(Math.max(0, clientX - r.left), r.width) / r.width * pv.duration;
  };

  let dragging = false;
  let drag = null; // mark being drawn / resized, file seconds
  const seek = e => {
    if (!pv || !pv.duration) return;
    const r = canvas.getBoundingClientRect();
    const x = Math.min(Math.max(0, e.clientX - r.left), r.width);
    audioEl.currentTime = x / r.width * pv.duration;
    tick();
  };

  canvas.onmousedown = e => {
    if (laneCfg && laneCfg.shiftCanvas && e.shiftKey && pv && pv.duration) {
      e.preventDefault();
      const t0 = xToTime(e.clientX);
      drag = { s: t0, e: t0 };
      const onMove = mv => {
        const t1 = xToTime(mv.clientX);
        drag = { s: Math.min(t0, t1), e: Math.max(t0, t1) };
        draw();
      };
      const onUp = () => {
        window.removeEventListener('mousemove', onMove);
        window.removeEventListener('mouseup', onUp);
        const d = drag;
        drag = null;
        if (d && d.e - d.s > 0.15) {
          const a = fileToDemo(segs(), d.s);
          const b = fileToDemo(segs(), d.e);
          if (b > a) mk.set([...mk.list(), { start: a, end: b }]);
        } else {
          draw();
        }
      };
      window.addEventListener('mousemove', onMove);
      window.addEventListener('mouseup', onUp);
      return;
    }
    dragging = true;
    seek(e);
  };
  window.addEventListener('mousemove', e => { if (dragging) seek(e); });
  window.addEventListener('mouseup', () => { dragging = false; });
  new ResizeObserver(() => draw()).observe(canvas);

  // ---- marks lane: drag to mark, drag the edges to adjust, × to remove ----
  function renderMarks() {
    if (!lane) return;
    lane.innerHTML = '';
    const list = mk.list();
    const ready = pv && pv.duration > 0;
    markBtn.disabled = !ready;
    markBtn.textContent = pending != null ? t(laneCfg.endKey, { t: clock(pending) }) : t(laneCfg.btnKey);
    markBtn.classList.toggle('rec', pending != null);
    if (!ready) return;

    let shown = 0;
    list.forEach((m, i) => {
      const pieces = demoToFile(segs(), m.start, m.end);
      if (!pieces.length) return;
      shown++;
      const s = pieces[0].s;
      const e = pieces[pieces.length - 1].e;
      const pill = el('div', laneCfg.pillCls);
      pill.style.left = (s / pv.duration * 100) + '%';
      pill.style.width = Math.max(0.6, (e - s) / pv.duration * 100) + '%';
      pill.title = t(laneCfg.tipKey, { n: i + 1, a: clock(s), b: clock(e) });
      const hl = el('span', 'h l');
      const hr = el('span', 'h r');
      const lbl = el('span', 'mlbl', '#' + (i + 1));
      const x = el('span', 'mx', '×');
      x.title = t(laneCfg.removeKey);
      pill.append(hl, lbl, x, hr);
      x.onpointerdown = ev => ev.stopPropagation();
      x.onclick = ev => {
        ev.stopPropagation();
        mk.set(list.filter(o => o !== m));
      };
      pill.onpointerdown = ev => {
        ev.stopPropagation();
        const edge = ev.target === hl ? 'start' : ev.target === hr ? 'end' : null;
        if (!edge) { audioEl.currentTime = s; tick(); return; } // click the mark: jump to it
        pill.setPointerCapture(ev.pointerId);
        const fixedFile = edge === 'start' ? e : s;
        pill.onpointermove = mv => {
          const tt = xToTime(mv.clientX);
          drag = { s: Math.min(tt, fixedFile), e: Math.max(tt, fixedFile) };
          draw();
        };
        pill.onpointerup = () => {
          pill.onpointermove = pill.onpointerup = null;
          if (drag && drag.e - drag.s > 0.1) {
            const nm = { start: fileToDemo(segs(), drag.s), end: fileToDemo(segs(), drag.e) };
            mk.set(list.map(o => o === m ? nm : o));
          }
          drag = null;
          draw();
        };
      };
      lane.append(pill);
    });
    if (!shown) lane.append(el('span', 'lanehint', t(laneCfg.hintKey)));
  }

  if (lane) {
    lane.onpointerdown = ev => {
      if (!pv || !pv.duration) return;
      lane.setPointerCapture(ev.pointerId);
      const t0 = xToTime(ev.clientX);
      drag = { s: t0, e: t0 };
      lane.onpointermove = mv => {
        const t1 = xToTime(mv.clientX);
        drag = { s: Math.min(t0, t1), e: Math.max(t0, t1) };
        draw();
      };
      lane.onpointerup = () => {
        lane.onpointermove = lane.onpointerup = null;
        const d = drag;
        drag = null;
        if (d && d.e - d.s > 0.15) {
          const a = fileToDemo(segs(), d.s);
          const b = fileToDemo(segs(), d.e);
          if (b > a) mk.set([...mk.list(), { start: a, end: b }]);
        } else {
          draw();
        }
      };
    };
    markBtn.onclick = () => toggleMark();
  }

  function toggleMark() {
    if (!pv || !pv.duration) return;
    const now = fileToDemo(segs(), audioEl.currentTime || 0);
    if (pending == null) { pending = now; renderMarks(); return; }
    const a = Math.min(pending, now);
    const b = Math.max(pending, now);
    pending = null;
    if (b - a > 0.1) mk.set([...mk.list(), { start: a, end: b }]);
    else renderMarks();
  }

  function draw() {
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    if (!w || !h) return;
    const targetW = Math.round(w * dpr);
    const targetH = Math.round(h * dpr);
    if (canvas.width !== targetW || canvas.height !== targetH) {
      canvas.width = targetW;
      canvas.height = targetH;
    }
    const g = canvas.getContext('2d');
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, w, h);
    if (!pv || !pv.duration) return;

    const lineCol = cssVar('--line');

    // cuts / period changes
    g.fillStyle = lineCol;
    for (const s of segs()) {
      const x = s.out / pv.duration * w;
      g.fillRect(x, 0, 1, h);
    }

    if (laneCfg) {
      const mcol = cssVar(laneCfg.colorVar);
      const band = (s0, e0, alpha) => {
        const x0 = s0 / pv.duration * w;
        const x1 = e0 / pv.duration * w;
        g.globalAlpha = alpha;
        g.fillStyle = mcol;
        g.fillRect(x0, 0, Math.max(1, x1 - x0), h);
        g.globalAlpha = 1;
        g.fillRect(x0, 0, Math.max(1, x1 - x0), 2);
      };
      for (const m of mk.list()) {
        const pieces = demoToFile(segs(), m.start, m.end);
        if (pieces.length) band(pieces[0].s, pieces[pieces.length - 1].e, 0.13);
      }
      if (drag) band(drag.s, drag.e, 0.22);
      if (pending != null) {
        const ps = demoToFile(segs(), pending, fileToDemo(segs(), audioEl.currentTime || 0) + 0.001);
        if (ps.length) band(ps[0].s, ps[ps.length - 1].e, 0.18);
      }
      g.globalAlpha = 1;
    }

    // DAW-style multi-lane symmetric bars: one horizontal track per speaker so
    // 3-5+ simultaneous voices never overlap into a muddy pile.
    const laneCount = peaks.length;
    if (laneCount > 0) {
      const laneH = h / laneCount;
      const step = 3;
      const barW = 2;
      const bars = Math.floor(w / step);
      const maxHalf = Math.max(3, laneH * 0.44 - 1);
      const textCol = cssVar('--text');
      const sunkCol = cssVar('--sunk');
      const fontSize = Math.max(9, Math.min(12, Math.floor(laneH * 0.52)));
      const maxLabelW = Math.min(140, w * 0.35);

      g.font = `600 ${fontSize}px system-ui, -apple-system, Segoe UI, sans-serif`;
      g.textBaseline = 'middle';
      g.textAlign = 'left';
      g.lineJoin = 'round';

      for (let p = 0; p < laneCount; p++) {
        const pk = peaks[p];
        const n = pk.length;
        const y0 = p * laneH;
        const mid = y0 + laneH / 2;
        const pid = pv.wavePlayers[p];
        const label = (playerById[pid] || {}).name || String(pid);

        if (p > 0) {
          g.globalAlpha = 0.55;
          g.fillStyle = lineCol;
          g.fillRect(0, Math.round(y0), w, 1);
        }
        g.globalAlpha = 0.25;
        g.fillStyle = lineCol;
        g.fillRect(0, Math.round(mid), w, 1);

        g.globalAlpha = hoveredLane === -1 || hoveredLane === p ? 0.95 : 0.22;
        g.fillStyle = colors[pid] || '#888';

        for (let i = 0; i < bars; i++) {
          const b0 = Math.floor((i * step) / w * n);
          const b1 = Math.max(b0 + 1, Math.floor(((i + 1) * step) / w * n));
          let m = 0;
          for (let b = b0; b < b1 && b < n; b++) if (pk[b] > m) m = pk[b];
          if (m <= 12) continue;
          const amp = Math.max(1.5, Math.pow(m / 255, 0.85) * maxHalf);
          g.fillRect(i * step, mid - amp, barW, amp * 2);
        }

        // Subtle player-name label with a background-colored halo so it stays
        // readable over speech at the start of a lane without a heavy box.
        g.lineWidth = 3;
        g.strokeStyle = sunkCol;
        g.globalAlpha = hoveredLane === -1 ? 0.85 : (hoveredLane === p ? 0.95 : 0.35);
        g.strokeText(label, 6, mid, maxLabelW);
        g.fillStyle = textCol;
        g.globalAlpha = hoveredLane === -1 ? 0.45 : (hoveredLane === p ? 0.85 : 0.18);
        g.fillText(label, 6, mid, maxLabelW);
      }
    }

    g.globalAlpha = 1;
    const cx = audioEl.currentTime / pv.duration * w;
    g.fillStyle = cssVar('--text');
    g.fillRect(cx, 0, 1.5, h);
  }

  function tick() {
    if (!pv) return;
    const now = audioEl.currentTime || 0;
    time.textContent = `${clock(now)} / ${clock(pv.duration)}`;
    let seg = null;
    for (const s of segs()) {
      if (now >= s.out - 0.01) seg = s;
    }
    if (seg) {
      const p = periodById[seg.period];
      const demoT = seg.demo + Math.min(Math.max(0, now - seg.out), seg.len);
      const name = p ? periodName(p) : '';
      const sinceStart = p ? demoT - p.start : 0;
      where.textContent = t('player.where', {
        name,
        allTalk: p && p.allTalk ? ' · ' + t('period.allTalk') : '',
        since: clock(sinceStart),
        demo: clock(demoT),
      });
    }
    draw();
  }

  return {
    loading() { status.textContent = t('player.preparing'); },
    redraw() { if (pv) tick(); else draw(); renderMarks(); }, // tick also re-translates the "where" text
    marksChanged() { renderMarks(); draw(); },
    toggleMark,
    demoToFile: (a, b) => demoToFile(segs(), a, b),
    previewDuration() { return pv ? pv.duration || 0 : 0; },
    fileSpan() {
      return pv && pv.duration
        ? { start: fileToDemo(segs(), 0), end: fileToDemo(segs(), pv.duration) }
        : null;
    },
    seekTo(sec) {
      if (!pv || !pv.duration) return;
      audioEl.currentTime = Math.max(0, Math.min(sec, pv.duration));
      tick();
    },
    set(v, msg) {
      audioEl.pause();
      pv = v;
      status.textContent = msg || '';
      legend.innerHTML = '';
      pending = null;
      hoveredLane = -1;
      if (!v || !v.duration) {
        peaks = [];
        canvas.style.height = waveHeight(1) + 'px';
        play.disabled = true;
        time.textContent = '0:00 / 0:00';
        where.textContent = '';
        audioEl.removeAttribute('src');
        audioEl.load();
        draw();
        renderMarks();
        return;
      }
      peaks = (v.peaks || []).map(decodeBase64Bytes);
      canvas.style.height = waveHeight(peaks.length) + 'px';
      (v.wavePlayers || []).forEach((id, idx) => {
        const s = el('span');
        const d = el('span', 'dot');
        d.style.background = colors[id];
        s.append(d, (playerById[id] || {}).name || id);
        s.onmouseenter = () => { hoveredLane = idx; draw(); };
        s.onmouseleave = () => { hoveredLane = -1; draw(); };
        legend.append(s);
      });
      audioEl.src = v.url;
      audioEl.currentTime = 0;
      play.disabled = false;
      renderMarks();
      tick();
    },
  };
}
