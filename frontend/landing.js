// Interactive sample preview shown beside the drop zone on the landing screen.
import { t } from './i18n.js';
import { PALETTE } from './state.js';
import { $, el, clock, cssVar } from './ui.js';

const SAMPLE_PLAYERS = [
  { id: 'p1', name: 'Player 1', color: PALETTE[0], talk: 18 },
  { id: 'p2', name: 'Player 2', color: PALETTE[1], talk: 11 },
  { id: 'p3', name: 'Player 3', color: PALETTE[3], talk: 22 },
];

const SAMPLE_ROUNDS = [
  { id: 'r7', kind: 'round', number: 7, score: '5-2', talk: 38, allTalk: false, dur: 44, demoStart: 480 },
  { id: 'ht', kind: 'halftime', label: 'Halftime', score: '6-6', talk: 19, allTalk: true, dur: 30, demoStart: 920 },
  { id: 'kn', kind: 'knife', label: 'Knife', score: 'CT', talk: 14, allTalk: false, dur: 24, demoStart: 15 },
];

// Deterministic voice blocks per round (including overlapping speech) so the
// preview renders the same overlaid per-player envelopes as player.js.
const WAVE_BLOCKS = {
  r7: [
    { p: 0, s: 0.03, e: 0.18, amp: 0.56 },
    { p: 1, s: 0.14, e: 0.30, amp: 0.44 },
    { p: 2, s: 0.28, e: 0.62, amp: 0.90 },
    { p: 0, s: 0.46, e: 0.58, amp: 0.48 },
    { p: 0, s: 0.65, e: 0.79, amp: 0.62 },
    { p: 1, s: 0.74, e: 0.87, amp: 0.52 },
    { p: 2, s: 0.80, e: 0.96, amp: 0.84 },
  ],
  ht: [
    { p: 1, s: 0.04, e: 0.25, amp: 0.58 },
    { p: 2, s: 0.20, e: 0.55, amp: 0.88 },
    { p: 0, s: 0.44, e: 0.74, amp: 0.54 },
    { p: 1, s: 0.68, e: 0.84, amp: 0.46 },
    { p: 2, s: 0.76, e: 0.95, amp: 0.80 },
  ],
  kn: [
    { p: 0, s: 0.06, e: 0.34, amp: 0.62 },
    { p: 1, s: 0.28, e: 0.60, amp: 0.50 },
    { p: 2, s: 0.54, e: 0.89, amp: 0.78 },
  ],
};

const P = {
  tab: 'report', // 'report' | 'cut'
  roundId: 'r7',
  reportedId: 'p3',
  inAudio: new Set(['p1', 'p2', 'p3']),
  mark: { s: 0.31, e: 0.62 },
  cuts: [
    { s: 0.14, e: 0.41 },
    { s: 0.55, e: 0.77 },
  ],
};

let canvasEl = null;

function activeRound() {
  return SAMPLE_ROUNDS.find(r => r.id === P.roundId) || SAMPLE_ROUNDS[0];
}

function roundTitle(r) {
  if (r.kind === 'round') return t('period.round', { n: r.number });
  return t(`period.${r.label}`);
}

function roundCode(r) {
  if (r.kind === 'round') return `R${String(r.number).padStart(2, '0')}`;
  return t(`period.${r.label}`);
}

function roundEnglish(r) {
  if (r.kind === 'round') return `Round ${r.number}`;
  return r.label;
}

function drawPreviewWave() {
  if (!canvasEl) return;
  const ctx = canvasEl.getContext('2d');
  const dpr = Math.max(1, window.devicePixelRatio || 1);
  const rect = canvasEl.getBoundingClientRect();
  const width = Math.max(240, Math.round(rect.width || 440));
  const height = Math.max(64, Math.round(rect.height || 74));
  const targetW = Math.round(width * dpr);
  const targetH = Math.round(height * dpr);
  if (canvasEl.width !== targetW || canvasEl.height !== targetH) {
    canvasEl.width = targetW;
    canvasEl.height = targetH;
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);

  ctx.fillStyle = cssVar('--sunk');
  ctx.fillRect(0, 0, width, height);

  const base = height - 4;
  const top = 6;

  // Highlight selected mark or cut range with the same band + 2px top bar as player.js.
  const ranges = P.tab === 'report' ? [P.mark] : P.cuts;
  const tint = P.tab === 'report' ? cssVar('--mark') : cssVar('--cut');
  for (const rg of ranges) {
    const x1 = rg.s * width;
    const x2 = rg.e * width;
    const spanW = Math.max(1, x2 - x1);
    ctx.globalAlpha = 0.13;
    ctx.fillStyle = tint;
    ctx.fillRect(x1, 0, spanW, height);
    ctx.globalAlpha = 1;
    ctx.fillRect(x1, 0, spanW, 2);
  }

  // One smooth outline per active player rising from a shared baseline, overlaid
  // (not stacked), matching player.js so overlapping speakers remain distinct.
  const blocks = WAVE_BLOCKS[P.roundId] || WAVE_BLOCKS.r7;
  const curves = [];
  for (let pIdx = 0; pIdx < SAMPLE_PLAYERS.length; pIdx++) {
    const pl = SAMPLE_PLAYERS[pIdx];
    if (!P.inAudio.has(pl.id)) continue;
    const pBlocks = blocks.filter(b => b.p === pIdx);
    const env = new Float32Array(width);
    let total = 0;
    for (let x = 0; x < width; x++) {
      const frac = (x + 0.5) / width;
      let val = 0;
      for (const blk of pBlocks) {
        if (frac < blk.s || frac > blk.e) continue;
        const rel = (frac - blk.s) / Math.max(0.01, blk.e - blk.s);
        // Smooth attack/release at edges with organic syllable ripples inside.
        const edge = Math.sin(rel * Math.PI);
        const edgeEnv = Math.pow(edge, 0.55);
        const ripple = 0.62
          + 0.22 * Math.sin(rel * Math.PI * 5 + pIdx * 1.7 + blk.s * 19)
          + 0.16 * Math.sin(rel * Math.PI * 11 + pIdx * 3.1 + blk.e * 13);
        const sample = Math.min(1, blk.amp * edgeEnv * Math.max(0.25, ripple));
        if (sample > val) val = sample;
      }
      env[x] = val;
      total += val;
    }
    if (total > 0) curves.push({ color: pl.color, env, total });
  }

  // Loudest-on-average first, so quieter voices are drawn on top.
  curves.sort((a, b) => b.total - a.total);

  for (const curve of curves) {
    const e = curve.env;
    ctx.beginPath();
    ctx.moveTo(0, base);
    for (let x = 0; x < width; x++) ctx.lineTo(x + 0.5, base - e[x] * (base - top));
    ctx.lineTo(width, base);
    ctx.closePath();
    ctx.globalAlpha = 0.16;
    ctx.fillStyle = curve.color;
    ctx.fill();

    // Outline only where the player talks (no colored line along the baseline).
    ctx.globalAlpha = 1;
    ctx.strokeStyle = curve.color;
    ctx.lineWidth = 1.6;
    ctx.lineJoin = 'round';
    ctx.beginPath();
    let on = false;
    for (let x = 0; x < width; x++) {
      const y = base - e[x] * (base - top);
      if (e[x] > 0.005 || (x + 1 < width && e[x + 1] > 0.005)) {
        on ? ctx.lineTo(x + 0.5, y) : ctx.moveTo(x + 0.5, y);
        on = true;
      } else if (on) {
        ctx.lineTo(x + 0.5, base);
        on = false;
      }
    }
    ctx.stroke();
  }

  // Baseline + playhead indicator.
  ctx.globalAlpha = 1;
  ctx.fillStyle = cssVar('--line');
  ctx.fillRect(0, base, width, 1);
  const playX = Math.round((P.tab === 'report' ? P.mark.s : P.cuts[0].s) * width);
  ctx.fillStyle = cssVar('--text');
  ctx.fillRect(playX, 0, 1.5, height);
}

function makeRangePill(lane, range, index, pillCls, onChange) {
  const pill = el('div', pillCls);
  const place = () => {
    pill.style.left = `${(range.s * 100).toFixed(2)}%`;
    pill.style.width = `${Math.max(5, (range.e - range.s) * 100).toFixed(2)}%`;
  };
  place();

  const rd = activeRound();
  const labelText = () => `#${index + 1} ${clock(range.s * rd.dur)}–${clock(range.e * rd.dur)}`;
  const lbl = el('span', 'mlbl', labelText());
  const leftH = el('span', 'h l');
  const rightH = el('span', 'h r');
  pill.append(leftH, lbl, rightH);

  const startDrag = (mode, startEvent) => {
    startEvent.preventDefault();
    startEvent.stopPropagation();
    const rect = lane.getBoundingClientRect();
    const origS = range.s;
    const origE = range.e;
    const minSpan = 0.08;

    const onMove = ev => {
      const dx = (ev.clientX - startEvent.clientX) / Math.max(1, rect.width);
      if (mode === 'move') {
        const span = origE - origS;
        const nextS = Math.min(Math.max(0, origS + dx), 1 - span);
        range.s = nextS;
        range.e = nextS + span;
      } else if (mode === 'left') {
        range.s = Math.min(Math.max(0, origS + dx), origE - minSpan);
      } else {
        range.e = Math.max(Math.min(1, origE + dx), origS + minSpan);
      }
      place();
      lbl.textContent = labelText();
      onChange();
    };
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
      window.removeEventListener('pointercancel', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
  };

  pill.addEventListener('pointerdown', ev => {
    if (ev.target === leftH) return startDrag('left', ev);
    if (ev.target === rightH) return startDrag('right', ev);
    startDrag('move', ev);
  });
  return pill;
}

function buildReportText() {
  const rd = activeRound();
  const rep = SAMPLE_PLAYERS.find(p => p.id === P.reportedId) || SAMPLE_PLAYERS[2];
  const fileStart = P.mark.s * rd.dur;
  const fileEnd = P.mark.e * rd.dur;
  const demoS = rd.demoStart + fileStart;
  const demoE = rd.demoStart + fileEnd;
  const tick = Math.round(demoS * 64);
  const clipFile = rd.allTalk
    ? `alltalk_${rd.label}.mp3`
    : `team_${rd.kind === 'round' ? 'R' + String(rd.number).padStart(2, '0') : rd.label}.mp3`;
  const blocks = WAVE_BLOCKS[rd.id] || WAVE_BLOCKS.r7;
  const speakers = [...new Set(
    blocks
      .filter(b => b.s < P.mark.e && b.e > P.mark.s && P.inAudio.has(SAMPLE_PLAYERS[b.p].id))
      .map(b => SAMPLE_PLAYERS[b.p].name),
  )].sort();
  const speaking = speakers.length ? ` - speaking: ${speakers.join(', ')}` : '';
  return [
    `Reported player(s): ${rep.name} | Reason: Toxic voice communication`,
    `#1 ${clipFile} at ${clock(fileStart)}-${clock(fileEnd)}`,
    `   ${roundEnglish(rd)} (demo ${clock(demoS)}-${clock(demoE)}, tick ${tick})${speaking}`,
  ].join('\n');
}

/** Render the interactive sample preview card on the landing screen. */
export function renderLanding() {
  const host = $('#landing-preview');
  if (!host) return;
  host.innerHTML = '';

  const head = el('div', 'preview-head');
  const titleWrap = el('div', 'preview-titlewrap');
  titleWrap.append(
    el('span', 'preview-badge', t('landing.badge')),
    el('span', 'preview-meta', t('landing.matchMeta')),
  );
  const seg = el('div', 'seg');
  const repBtn = el('button', P.tab === 'report' ? 'on' : '', t('tab.report'));
  repBtn.type = 'button';
  repBtn.onclick = () => { P.tab = 'report'; renderLanding(); };
  const cutBtn = el('button', P.tab === 'cut' ? 'on' : '', t('tab.cut'));
  cutBtn.type = 'button';
  cutBtn.onclick = () => { P.tab = 'cut'; renderLanding(); };
  seg.append(repBtn, cutBtn);
  head.append(titleWrap, seg);
  host.append(head);

  const cols = el('div', 'preview-cols');

  const rdCol = el('div', 'col');
  rdCol.append(el('div', 'col-head', t('common.rounds')));
  const rdList = el('div', 'preview-list');
  for (const rd of SAMPLE_ROUNDS) {
    const row = el('div', 'row' + (rd.id === P.roundId ? ' on' : ''));
    row.append(
      el('span', 'lbl', roundCode(rd)),
      el('span', 'score', rd.score),
    );
    if (rd.allTalk) row.append(el('span', 'badge alltalk', t('period.allTalk')));
    row.append(el('span', 'talk has', clock(rd.talk)));
    row.onclick = () => { P.roundId = rd.id; renderLanding(); };
    rdList.append(row);
  }
  rdCol.append(rdList);

  const plCol = el('div', 'col');
  plCol.append(el('div', 'col-head', t('common.players')));
  const plList = el('div', 'preview-list');
  for (const pl of SAMPLE_PLAYERS) {
    const isAud = P.inAudio.has(pl.id);
    const isRep = pl.id === P.reportedId;
    const row = el('div', 'row' + (isAud ? ' on' : ''));
    const dot = el('span', 'dot');
    dot.style.background = pl.color;
    row.append(dot, el('span', 'pname', pl.name));
    if (P.tab === 'report') {
      const flag = el('button', 'flag' + (isRep ? ' on' : ''), isRep ? t('right.reported') : t('right.report'));
      flag.type = 'button';
      flag.onclick = ev => {
        ev.stopPropagation();
        P.reportedId = pl.id;
        P.inAudio.add(pl.id);
        renderLanding();
      };
      row.append(flag);
    } else {
      row.append(el('span', 'talk' + (isAud ? ' has' : ''), clock(pl.talk)));
    }
    row.onclick = () => {
      const locked = P.tab === 'report' && pl.id === P.reportedId;
      if (!locked && P.inAudio.has(pl.id) && P.inAudio.size > 1) P.inAudio.delete(pl.id);
      else P.inAudio.add(pl.id);
      renderLanding();
    };
    plList.append(row);
  }
  plCol.append(plList);
  cols.append(rdCol, plCol);
  host.append(cols);

  const playerBox = el('div', 'preview-player');
  const pTop = el('div', 'preview-ptop');
  const rd = activeRound();
  const playDummy = el('span', 'preview-play', '▶');
  const pInfo = el('span', 'pwhere', `${roundTitle(rd)}${rd.allTalk ? ' · ' + t('period.allTalk') : ''} · ${clock(rd.dur)}`);
  const pMode = el('span', 'hint', P.tab === 'report' ? t('landing.dragHint') : t('cut.hintTeam'));
  pTop.append(playDummy, pInfo, pMode);

  canvasEl = el('canvas', 'preview-wave');
  const lane = el('div', P.tab === 'report' ? 'marklane' : 'cutlane');

  const outBox = el('div', 'preview-output');
  const updateOutput = () => {
    outBox.innerHTML = '';
    if (P.tab === 'report') {
      const outHead = el('div', 'preview-outhead');
      outHead.append(el('span', '', t('landing.outputReport')), el('span', 'preview-pill', 'MP3 · .zip'));
      const pre = el('pre', 'preview-report', buildReportText());
      outBox.append(outHead, pre);
    } else {
      const outHead = el('div', 'preview-outhead');
      outHead.append(el('span', '', t('landing.outputClips')), el('span', 'preview-pill cut', t('landing.clipsZip', { n: P.cuts.length })));
      const clips = el('div', 'preview-clips');
      P.cuts.forEach((rg, idx) => {
        const sTxt = clock(rg.s * rd.dur).replace(':', 'm') + 's';
        const eTxt = clock(rg.e * rd.dur).replace(':', 'm') + 's';
        const item = el('div', 'preview-cliprow');
        item.append(
          el('span', 'cutrange-idx', `#${idx + 1}`),
          el('span', 'preview-clipname', `clip_${String(idx + 1).padStart(2, '0')}_${roundCode(rd)}_${sTxt}-${eTxt}.mp3`),
          el('span', 'preview-clipdur', `${Math.max(1, Math.round((rg.e - rg.s) * rd.dur))}s`),
        );
        clips.append(item);
      });
      outBox.append(outHead, clips);
    }
  };

  if (P.tab === 'report') {
    lane.append(makeRangePill(lane, P.mark, 0, 'mark', () => { drawPreviewWave(); updateOutput(); }));
    lane.addEventListener('pointerdown', ev => {
      if (ev.target !== lane) return;
      const rect = lane.getBoundingClientRect();
      const center = Math.min(Math.max(0.08, (ev.clientX - rect.left) / Math.max(1, rect.width)), 0.92);
      const half = (P.mark.e - P.mark.s) / 2;
      P.mark.s = Math.max(0, Math.min(1 - half * 2, center - half));
      P.mark.e = P.mark.s + half * 2;
      renderLanding();
    });
  } else {
    P.cuts.forEach((rg, idx) => {
      lane.append(makeRangePill(lane, rg, idx, 'mark cut', () => { drawPreviewWave(); updateOutput(); }));
    });
  }

  const laneCaption = el('div', 'hint preview-caption', P.tab === 'report' ? t('landing.markHint') : t('landing.cutHint'));
  playerBox.append(pTop, canvasEl, lane, laneCaption);
  host.append(playerBox);

  updateOutput();
  host.append(outBox);
  requestAnimationFrame(drawPreviewWave);
}

/** Redraw the landing preview waveform (e.g. on theme change or returning to the landing screen). */
export function redrawLanding() {
  drawPreviewWave();
}

/** Wire static landing-screen controls and resize handling. */
export function initLanding() {
  const dropOpen = $('#drop-open');
  if (dropOpen) dropOpen.onclick = () => $('#open').click();

  window.addEventListener('resize', () => {
    if (!$('#empty').classList.contains('hidden')) drawPreviewWave();
  });
}
