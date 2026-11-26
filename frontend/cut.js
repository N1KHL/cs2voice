// Listen & Cut tab: team-isolated preview, waveform range selection, and clip export (no report tools).
import { t } from './i18n.js';
import { M, colors, C, activeTab, teamPlayers, talkIn, groupPlayers } from './state.js';
import { api } from './api.js';
import {
  $, $$, el, clock, debounce, store, formatError,
  periodRow, teamHead, renderTeamSwitch, downloadResult,
} from './ui.js';
import { makePlayer } from './player.js';

let cutPlayer = null;
let needsRefresh = false;

function renderRanges() {
  const root = $('#cutpanel');
  const listBox = $('#cut-rangelist', root);
  listBox.innerHTML = '';

  const clr = $('#cut-clear', root);
  clr.textContent = t('cut.clearRanges', { n: C.ranges.length });
  clr.classList.toggle('hidden', !C.ranges.length);

  const whole = $('#cut-whole', root);
  whole.classList.toggle('hidden', !(cutPlayer && cutPlayer.previewDuration() > 0));

  if (!C.ranges.length) {
    listBox.append(el('div', 'none', t('cut.noRanges')));
  } else {
    C.ranges.forEach((rg, idx) => {
      const row = el('div', 'cutrange-row');
      const pieces = cutPlayer ? cutPlayer.demoToFile(rg.start, rg.end) : [];
      const dur = Math.max(0, rg.end - rg.start);
      const spanTxt = `${clock(rg.start)} – ${clock(rg.end)}`;
      const durTxt = pieces.length
        ? `(${clock(dur)})`
        : `(${clock(dur)} · ${t('cut.outsidePreview')})`;
      const del = el('button', 'cutrange-del', '×');
      del.type = 'button';
      del.title = t('cut.remove');
      del.onclick = ev => {
        ev.stopPropagation();
        C.ranges = C.ranges.filter(o => o !== rg);
        cutPlayer.marksChanged();
        renderRanges();
      };
      row.append(
        el('span', 'cutrange-idx', `#${idx + 1}`),
        el('span', 'cutrange-span', spanTxt),
        el('span', 'cutrange-dur', durTxt),
        del,
      );
      if (pieces.length) {
        row.onclick = () => cutPlayer.seekTo(pieces[0].s);
      }
      listBox.append(row);
    });
  }

  const expBtn = $('#cut-export', root);
  expBtn.disabled = !M || !C.ranges.length || !C.periods.size || !C.players.size;
}

async function runPreview() {
  if (!M || !cutPlayer) return;
  if (activeTab !== 'cut') {
    needsRefresh = true;
    return;
  }
  needsRefresh = false;
  const periods = [...C.periods].sort((a, b) => a - b);
  const players = [...C.players];
  if (!periods.length || !players.length) {
    cutPlayer.set(null, t('left.selectSome'));
    renderRanges();
    return;
  }
  cutPlayer.loading();
  try {
    const pv = await api('Preview', {
      slot: 'cut',
      periods,
      players,
      team: C.team,
      mode: C.mode,
      normalize: C.normalize,
    });
    cutPlayer.set(pv, pv.duration > 0 ? '' : t('left.nobody'));
    renderRanges();
  } catch (err) {
    cutPlayer.set(null, formatError(err));
    renderRanges();
  }
}

const refresh = debounce(runPreview, 250);

export function renderCut() {
  if (!M) return;
  const root = $('#cutpanel');

  renderTeamSwitch($('#cut-team', root), M.teams, C.team, tm => {
    C.team = tm;
    renderCut();
    refresh();
  });

  const pl = $('.periods', root);
  pl.innerHTML = '';
  const tp = teamPlayers(C.team);
  for (const p of M.periods) {
    const talk = talkIn(p, p.allTalk ? M.players.map(x => x.id) : tp);
    pl.append(periodRow(p, C.periods.has(p.id), talk, on => {
      on ? C.periods.add(p.id) : C.periods.delete(p.id);
      refresh();
      renderRanges();
    }));
  }

  // Players list (isolate 1..N players, or whole team)
  const ul = $('.players', root);
  ul.innerHTML = '';
  for (const grp of groupPlayers()) {
    const ids = grp.players.map(p => p.id);
    ul.append(teamHead(
      grp.team || t('common.spectators'),
      ids,
      C.players,
      () => { renderCut(); refresh(); },
      grp.team === C.team ? t('left.listening') : t('left.onlyAllTalk'),
    ));
    for (const p of grp.players) {
      const other = grp.team !== C.team;
      const row = el('div', 'row' + (C.players.has(p.id) ? ' on' : '') + (other ? ' dimmed' : ''));
      const cb = el('input');
      cb.type = 'checkbox';
      cb.checked = C.players.has(p.id);
      const dot = el('span', 'dot');
      dot.style.background = colors[p.id];
      row.append(cb, dot, el('span', 'pname', p.name));
      row.append(el('span', 'talk' + (p.talk > 0 ? ' has' : ''), p.talk > 0 ? clock(p.talk) : '—'));
      row.onclick = ev => {
        if (ev.target !== cb) cb.checked = !cb.checked;
        cb.checked ? C.players.add(p.id) : C.players.delete(p.id);
        row.classList.toggle('on', cb.checked);
        refresh();
        renderRanges();
      };
      ul.append(row);
    }
  }

  $$('.col-head .links a', root).forEach(a => a.onclick = () => {
    const isPeriods = a.closest('.col').querySelector('.periods');
    const act = a.dataset.act;
    if (isPeriods) {
      if (act === 'all') M.periods.forEach(p => C.periods.add(p.id));
      if (act === 'none') C.periods.clear();
      if (act === 'talk') {
        C.periods.clear();
        M.periods.forEach(p => {
          if (talkIn(p, p.allTalk ? M.players.map(x => x.id) : tp) > 0) C.periods.add(p.id);
        });
      }
    } else {
      if (act === 'all') M.players.forEach(p => C.players.add(p.id));
      if (act === 'none') C.players.clear();
    }
    renderCut();
    refresh();
  });

  $$('.mode button', root).forEach(b => {
    b.classList.toggle('on', b.dataset.mode === C.mode);
    b.onclick = () => { C.mode = b.dataset.mode; renderCut(); refresh(); };
  });
  const norm = $('.normalize', root);
  norm.checked = C.normalize;
  norm.onchange = () => { C.normalize = norm.checked; refresh(); };

  $$('.format button', root).forEach(b => {
    b.classList.toggle('on', b.dataset.format === C.format);
    b.onclick = () => { C.format = b.dataset.format; renderCut(); };
  });
  C.bundle = store.get('exportMode') === 'files' ? 'files' : 'zip';
  $$('.bundle button', root).forEach(b => {
    b.classList.toggle('on', b.dataset.bundle === C.bundle);
    b.onclick = () => {
      C.bundle = b.dataset.bundle === 'files' ? 'files' : 'zip';
      store.set('exportMode', C.bundle);
      renderCut();
    };
  });
  const mid = $('.matchid', root);
  mid.classList.toggle('hidden', !M.faceitMatchId);
  const incId = $('.incid', root);
  incId.checked = C.matchId;
  incId.onchange = () => { C.matchId = incId.checked; };

  renderRanges();
}

export function initCut() {
  const root = $('#cutpanel');
  cutPlayer = makePlayer($('.player', root), {
    cuts: {
      list: () => C.ranges,
      set: list => {
        C.ranges = list.slice().sort((a, b) => a.start - b.start);
        cutPlayer.marksChanged();
        renderRanges();
      },
    },
  });

  $('#cut-whole', root).onclick = () => {
    const sp = cutPlayer.fileSpan();
    if (!sp || sp.end <= sp.start) return;
    C.ranges = [...C.ranges, { start: sp.start, end: sp.end }].sort((a, b) => a.start - b.start);
    cutPlayer.marksChanged();
    renderRanges();
  };

  $('#cut-clear', root).onclick = () => {
    C.ranges = [];
    cutPlayer.marksChanged();
    renderRanges();
  };

  $('#cut-export', root).onclick = async () => {
    if (!M || !C.ranges.length) return;
    const btn = $('#cut-export', root);
    const msg = $('#cut-exportmsg', root);
    btn.disabled = true;
    msg.textContent = t('cut.exporting');
    try {
      const req = {
        periods: [...C.periods].sort((a, b) => a - b),
        players: [...C.players],
        team: C.team,
        mode: C.mode,
        format: C.format,
        matchIdPrefix: C.matchId && !!M.faceitMatchId,
        clips: C.ranges.map(rg => ({ start: rg.start, end: rg.end })),
      };
      const res = await api('ExportCut', req, C.bundle);
      await downloadResult(res, 'cs2voice_cuts.zip');
      const count = (res && res.files && res.files.length) || 0;
      msg.textContent = res && res.zipBytes
        ? t('cut.exportedZip', { name: res.zipName || 'cs2voice_cuts.zip', n: count })
        : t('cut.exportedFiles', { n: count });
    } catch (err) {
      msg.textContent = formatError(err);
    } finally {
      renderRanges();
    }
  };

  cutPlayer.set(null, '');
}

export function setupCut() {
  if (!M || !cutPlayer) return;
  const root = $('#cutpanel');
  $('#cut-exportmsg', root).textContent = '';
  cutPlayer.set(null, '');
  renderCut();
  runPreview();
}

export function redrawCut() {
  if (cutPlayer) cutPlayer.redraw();
}

export function onShowCut() {
  if (!M) return;
  const savedBundle = store.get('exportMode') === 'files' ? 'files' : 'zip';
  if (savedBundle !== C.bundle) {
    C.bundle = savedBundle;
    renderCut();
  }
  if (needsRefresh) {
    runPreview();
  } else {
    redrawCut();
  }
}

export function toggleCutRange() {
  if (cutPlayer) cutPlayer.toggleMark();
}
