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
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    const g = canvas.getContext('2d');
    g.scale(dpr, dpr);
    g.clearRect(0, 0, w, h);
    if (!pv || !pv.duration) return;

    // cuts / period changes
    g.fillStyle = cssVar('--line');
    for (const s of segs()) {
      const x = s.out / pv.duration * w;
      g.fillRect(x, 0, 1, h);
    }

    // One smooth outline per player rising from a shared baseline, overlaid
    // (not stacked): with 3+ people talking each voice is still its own curve.
    const base = h - 4;
    const top = 4;
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

    const n = peaks.length ? peaks[0].length : 0;
    const env = peaks.map(pk => {
      const raw = new Float32Array(w);
      for (let x = 0; x < w; x++) {
        const b0 = Math.floor(x / w * n);
        const b1 = Math.max(b0 + 1, Math.floor((x + 1) / w * n));
        let m = 0;
        for (let b = b0; b < b1 && b < n; b++) if (pk[b] > m) m = pk[b];
        raw[x] = m > 12 ? m / 255 : 0;
      }
      // fill tiny gaps between syllables, then smooth into an arc
      const held = new Float32Array(w);
      const out = new Float32Array(w);
      const hold = Math.max(2, Math.round(w / n * 6));
      for (let x = 0; x < w; x++) {
        let m = 0;
        for (let k = Math.max(0, x - hold); k <= Math.min(w - 1, x + hold); k++) if (raw[k] > m) m = raw[k];
        held[x] = m;
      }
      const r = Math.max(2, Math.round(w / n * 4));
      for (let x = 0; x < w; x++) {
        let s = 0;
        let c = 0;
        for (let k = Math.max(0, x - r); k <= Math.min(w - 1, x + r); k++) { s += held[k]; c++; }
        out[x] = s / c;
      }
      for (let x = 0; x < w; x++) if (raw[x] === 0 && held[x] === 0) out[x] = 0;
      return out;
    });

    // loudest-on-average first, so quieter voices are drawn on top
    const order = env
      .map((e, p) => [e.reduce((a, v) => a + v, 0), p])
      .sort((a, b) => b[0] - a[0])
      .map(v => v[1]);

    for (const p of order) {
      const e = env[p];
      const col = colors[pv.wavePlayers[p]] || '#888';
      g.beginPath();
      g.moveTo(0, base);
      for (let x = 0; x < w; x++) g.lineTo(x + 0.5, base - e[x] * (base - top));
      g.lineTo(w, base);
      g.closePath();
      g.globalAlpha = 0.16;
      g.fillStyle = col;
      g.fill();
      // outline only where the player talks (no colored line along the baseline)
      g.globalAlpha = 1;
      g.strokeStyle = col;
      g.lineWidth = 1.6;
      g.lineJoin = 'round';
      g.beginPath();
      let on = false;
      for (let x = 0; x < w; x++) {
        const y = base - e[x] * (base - top);
        if (e[x] > 0.005 || (x + 1 < w && e[x + 1] > 0.005)) {
          on ? g.lineTo(x + 0.5, y) : g.moveTo(x + 0.5, y);
          on = true;
        } else if (on) {
          g.lineTo(x + 0.5, base);
          on = false;
        }
      }
      g.stroke();
    }

    g.fillStyle = cssVar('--line');
    g.fillRect(0, base, w, 1);
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
      if (!v || !v.duration) {
        peaks = [];
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
      for (const id of v.wavePlayers || []) {
        const s = el('span');
        const d = el('span', 'dot');
        d.style.background = colors[id];
        s.append(d, (playerById[id] || {}).name || id);
        legend.append(s);
      }
      audioEl.src = v.url;
      audioEl.currentTime = 0;
      play.disabled = false;
      renderMarks();
      tick();
    },
  };
}
