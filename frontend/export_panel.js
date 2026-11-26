// Right-hand Export & FACEIT Report panel (team audio, isolated players, marks, reasons).
import { t } from './i18n.js';
import { M, colors, playerById, periodById, L, R, BOTH, REASONS, reasonText, talkIn, groupPlayers } from './state.js';
import { api, openFaceitTicket } from './api.js';
import {
  $, $$, el, clock, debounce, store, formatError,
  periodRow, teamHead, renderTeamSwitch, downloadResult, saveFileToDir, uniqueReportName,
} from './ui.js';
import { makePlayer } from './player.js';

let rightPlayer = null;
let lastPreviewKey = '';

// Export is always for one team (each team only hears itself). "Both teams" is
// only possible when every selected round is an all-talk moment (halftime...).
const rightTeams = () => R.team === BOTH ? M.teams.filter(tm => tm) : [R.team];
const rightIds = () => M.players.filter(p => rightTeams().includes(p.team)).map(p => p.id);
const onlyAllTalk = () => R.periods.size > 0 && [...R.periods].every(id => periodById[id] && periodById[id].allTalk);

function setRightTeam(tm) {
  R.team = tm;
  const ids = new Set(rightIds());
  R.audio = new Set([...R.audio].filter(id => ids.has(id)));
  R.reported = new Set([...R.reported].filter(id => ids.has(id)));
  if (tm === BOTH) R.periods = new Set([...R.periods].filter(id => periodById[id].allTalk));
  renderRight();
  refreshRight();
}

function getExportDelivery() {
  return store.get('exportMode') === 'files' ? 'files' : 'zip';
}

function renderDelivery() {
  const mode = getExportDelivery();
  $$('#exportdelivery button').forEach(b => {
    b.classList.toggle('on', b.dataset.delivery === mode);
    b.onclick = () => {
      store.set('exportMode', b.dataset.delivery === 'files' ? 'files' : 'zip');
      renderDelivery();
    };
  });
  const dirBtn = $('#exportdir');
  if (dirBtn) dirBtn.classList.toggle('hidden', typeof window.showDirectoryPicker !== 'function');
}

function exportRequest() {
  const ids = new Set(rightIds());
  return {
    periods: [...R.periods].sort((a, b) => a - b),
    audioPlayers: [...R.audio].filter(id => ids.has(id)),
    reported: [...R.reported].filter(id => ids.has(id)),
    mode: R.mode,
    perRound: R.perRound,
    includeFull: R.includeFull,
    includeIsolated: R.includeIsolated,
    format: R.format,
    reason: reasonText(),
    matchIdPrefix: R.matchId && !!M.faceitMatchId,
    marks: R.marks.map(m => ({ start: m.start, end: m.end })),
  };
}

export function renderReasons() {
  const box = $('#reasonchips');
  box.innerHTML = '';
  for (const r of REASONS) {
    const b = el('button', 'chip' + (R.reasons.has(r.key) ? ' on' : ''), t(r.key));
    b.title = r.en;
    b.onclick = () => {
      R.reasons.has(r.key) ? R.reasons.delete(r.key) : R.reasons.add(r.key);
      renderReasons();
      refreshRight();
    };
    box.append(b);
  }
}

export function renderMarkLinks() {
  const a = $('#clearmarks');
  a.textContent = t('marks.clear', { n: R.marks.length });
  a.classList.toggle('hidden', !R.marks.length);
}

export function renderFiles() {
  const box = $('#right .files');
  box.innerHTML = '';
  if (!R.files.length) {
    const msg = !R.periods.size ? t('right.selectRounds') : !R.audio.size ? t('right.selectPlayers') : t('right.nothing');
    box.append(el('div', 'none', msg));
    return;
  }
  R.files.forEach((f, i) => {
    const r = el('div', 'file' + (i === R.fileIdx ? ' on' : ''));
    const kind = f.kind === 'isolated'
      ? t('right.kindIsolated', { name: (playerById[f.player] || {}).name || '?' })
      : f.kind === 'alltalk'
      ? t('right.kindAllTalk')
      : t('right.kindTeam');
    r.append(
      el('span', 'fkind' + (f.kind === 'isolated' ? ' iso' : ''), kind),
      el('span', 'fname', f.name),
      el('span', 'fdur', clock(f.duration)),
    );
    r.onclick = () => { R.fileIdx = i; renderFiles(); previewFile(); };
    box.append(r);
  });
}

async function previewFile() {
  if (!rightPlayer) return;
  const f = R.files[R.fileIdx];
  if (!f) {
    lastPreviewKey = '';
    return rightPlayer.set(null, t('right.previewHere'));
  }
  const req = f.kind === 'isolated'
    ? { slot: 'right', periods: f.periods, players: [f.player], team: f.team, mode: R.mode }
    : { slot: 'right', periods: f.periods, players: [...R.audio], team: f.team, mode: R.mode };
  const key = JSON.stringify(req);
  if (key === lastPreviewKey) return;
  lastPreviewKey = key;
  rightPlayer.loading();
  try {
    rightPlayer.set(await api('Preview', req), '');
  } catch (e) {
    lastPreviewKey = '';
    rightPlayer.set(null, formatError(e));
  }
}

export const refreshRight = debounce(async () => {
  if (!M) return;
  $('#exportmsg').textContent = '';
  const req = exportRequest();
  let plan = null;
  if (req.periods.length && ((req.includeFull && req.audioPlayers.length) || (req.includeIsolated && req.reported.length))) {
    try { plan = await api('PlanExport', req); }
    catch (e) { $('#exportmsg').textContent = formatError(e); }
  }
  R.files = (plan && plan.files) || [];
  if (R.fileIdx >= R.files.length) R.fileIdx = 0;
  renderFiles();
  if (!R.reportEdited) $('#report').value = plan ? plan.report : '';
  $('#export').disabled = R.files.length === 0;
  if ($('#exportdir')) $('#exportdir').disabled = R.files.length === 0;
  previewFile();
}, 300);

export function renderRight() {
  if (!M) return;
  const root = $('#right');
  const both = R.team === BOTH;

  // team switch: team A | team B | both teams (all-talk only)
  const seg = $('#right-team');
  renderTeamSwitch(seg, M.teams.filter(Boolean), R.team, setRightTeam);
  const bb = el('button', both ? 'on' : '', t('right.bothTeams'));
  bb.title = t('right.bothTip');
  bb.disabled = !both && !onlyAllTalk();
  bb.onclick = () => setRightTeam(BOTH);
  seg.append(bb);
  $('#right-hint').textContent = both ? t('right.hintBoth') : t('right.hintTeam');

  const pl = $('.periods', root);
  pl.innerHTML = '';
  const tids = rightIds();
  for (const p of M.periods) {
    const dis = both && !p.allTalk ? t('right.bothDisabledRow') : '';
    pl.append(periodRow(p, R.periods.has(p.id), talkIn(p, tids), on => {
      on ? R.periods.add(p.id) : R.periods.delete(p.id);
      renderRight();
      refreshRight();
    }, dis));
  }

  const ul = $('.players', root);
  ul.innerHTML = '';
  const names = el('div', 'colnames');
  names.append(el('span', '', t('right.colAudio')), el('span', 'grow', ''), el('span', '', t('right.colReport')));
  ul.append(names);
  for (const g of groupPlayers().filter(grp => rightTeams().includes(grp.team))) {
    const ids = g.players.map(p => p.id);
    ul.append(teamHead(
      g.team,
      ids,
      R.audio,
      () => { renderRight(); refreshRight(); },
      t('right.inAudio', { n: ids.filter(id => R.audio.has(id)).length, total: ids.length }),
    ));
    for (const p of g.players) {
      const r = el('div', 'row' + (R.audio.has(p.id) ? ' on' : ''));
      const a = el('input');
      a.type = 'checkbox';
      a.checked = R.audio.has(p.id);
      a.title = t('right.audioTip');
      const dot = el('span', 'dot');
      dot.style.background = colors[p.id];
      const rep = R.reported.has(p.id);
      const flag = el('button', 'flag' + (rep ? ' on' : ''), rep ? t('right.reported') : t('right.report'));
      flag.title = t('right.reportTip');
      r.append(a, dot, el('span', 'pname', p.name), el('span', 'talk' + (p.talk > 0 ? ' has' : ''), p.talk > 0 ? clock(p.talk) : '—'), flag);
      const toggleAudio = () => {
        R.audio.has(p.id) ? R.audio.delete(p.id) : R.audio.add(p.id);
        renderRight();
        refreshRight();
      };
      flag.onclick = e => {
        e.stopPropagation();
        if (R.reported.has(p.id)) R.reported.delete(p.id);
        else { R.reported.add(p.id); R.audio.add(p.id); }
        renderRight();
        refreshRight();
      };
      a.onclick = e => { e.stopPropagation(); toggleAudio(); };
      r.onclick = toggleAudio;
      ul.append(r);
    }
  }

  $$('.col-head .links a', root).forEach(a => a.onclick = () => {
    const act = a.dataset.act;
    if (act === 'all') M.periods.forEach(p => { if (!both || p.allTalk) R.periods.add(p.id); });
    if (act === 'none') R.periods.clear();
    if (act === 'copy') R.periods = new Set([...L.periods].filter(id => !both || periodById[id].allTalk));
    if (act === 'clearrep') R.reported.clear();
    renderRight();
    refreshRight();
  });

  $$('.mode button', root).forEach(b => {
    b.classList.toggle('on', b.dataset.mode === R.mode);
    b.onclick = () => { R.mode = b.dataset.mode; renderRight(); refreshRight(); };
  });
  $$('.split button', root).forEach(b => {
    b.classList.toggle('on', (b.dataset.split === 'round') === R.perRound);
    b.onclick = () => { R.perRound = b.dataset.split === 'round'; renderRight(); refreshRight(); };
  });
  $$('.format button', root).forEach(b => {
    b.classList.toggle('on', b.dataset.format === R.format);
    b.onclick = () => { R.format = b.dataset.format; renderRight(); refreshRight(); };
  });

  const f = $('.incfull', root);
  f.checked = R.includeFull;
  f.onchange = () => { R.includeFull = f.checked; refreshRight(); };
  const i = $('.inciso', root);
  i.checked = R.includeIsolated;
  i.onchange = () => { R.includeIsolated = i.checked; refreshRight(); };

  // isolated files: say whose, or that someone has to be marked first
  const repNames = [...R.reported].map(id => (playerById[id] || {}).name).filter(Boolean);
  $('.isohint', root).textContent = R.includeIsolated
    ? (repNames.length ? `(${repNames.join(', ')})` : `(${t('right.isolatedHint')})`)
    : '';
  const mid = $('.matchid', root);
  mid.classList.toggle('hidden', !M.faceitMatchId);
  const c = $('.incid', root);
  c.checked = R.matchId;
  c.onchange = () => { R.matchId = c.checked; refreshRight(); };
  renderDelivery();
}

export function initExportPanel() {
  rightPlayer = makePlayer($('#right .player'), {
    marks: {
      list: () => R.marks,
      set: list => {
        R.marks = list.slice().sort((a, b) => a.start - b.start);
        rightPlayer.marksChanged();
        refreshRight();
        renderMarkLinks();
      },
    },
  });
  rightPlayer.set(null, '');
  renderDelivery();

  $('#reasonother').oninput = debounce(e => { R.reasonOther = e.target.value; refreshRight(); }, 400);
  $('#clearmarks').onclick = () => {
    R.marks = [];
    rightPlayer.marksChanged();
    refreshRight();
    renderMarkLinks();
  };

  $('#export').onclick = async () => {
    const btn = $('#export');
    const dirBtn = $('#exportdir');
    btn.disabled = true;
    if (dirBtn) dirBtn.disabled = true;
    $('#exportmsg').textContent = t('right.exporting');
    try {
      const delivery = getExportDelivery();
      const req = { ...exportRequest(), reportOverride: R.reportEdited ? $('#report').value : '' };
      const res = await api('Export', req, delivery);
      if (!res) { $('#exportmsg').textContent = ''; return; }
      const view = res.view || {};
      if (!R.reportEdited && view.report != null) $('#report').value = view.report;
      await downloadResult(res, 'cs2voice.zip');
      if (delivery === 'files') {
        const list = res.files || [];
        $('#exportmsg').textContent = t('export.downloadedFiles', { n: list.length, report: res.reportFile || 'report.txt' });
      } else {
        const count = (view.files && view.files.length) || 0;
        $('#exportmsg').textContent = t('export.downloadedZip', { name: res.zipName || 'cs2voice.zip', n: count, report: view.reportFile || 'report.txt' });
      }
    } catch (e) {
      $('#exportmsg').textContent = formatError(e);
    } finally {
      btn.disabled = R.files.length === 0;
      if (dirBtn) dirBtn.disabled = R.files.length === 0;
    }
  };

  if ($('#exportdir')) {
    $('#exportdir').onclick = async () => {
      if (typeof window.showDirectoryPicker !== 'function') return;
      let dirHandle;
      try {
        dirHandle = await window.showDirectoryPicker({ mode: 'readwrite' });
      } catch (e) {
        if (e && e.name === 'AbortError') return;
        $('#exportmsg').textContent = String(e.message || e);
        return;
      }
      const btn = $('#export');
      const dirBtn = $('#exportdir');
      btn.disabled = true;
      dirBtn.disabled = true;
      $('#exportmsg').textContent = t('right.exporting');
      try {
        const req = { ...exportRequest(), reportOverride: R.reportEdited ? $('#report').value : '' };
        const res = await api('Export', req, 'files');
        if (!res) { $('#exportmsg').textContent = ''; return; }
        const view = res.view || {};
        if (!R.reportEdited && view.report != null) $('#report').value = view.report;
        const list = res.files || [];
        for (const item of list) {
          await saveFileToDir(dirHandle, item.name, item.bytes);
        }
        const reportName = await uniqueReportName(dirHandle);
        await saveFileToDir(dirHandle, reportName, res.reportText || '');
        $('#exportmsg').textContent = t('right.saved', { n: list.length, report: reportName, dir: dirHandle.name });
      } catch (e) {
        $('#exportmsg').textContent = formatError(e);
      } finally {
        btn.disabled = R.files.length === 0;
        dirBtn.disabled = R.files.length === 0;
      }
    };
  }

  $('#report').oninput = () => { R.reportEdited = true; };
  $('#faceit').onclick = openFaceitTicket;
  $('#regen').onclick = () => { R.reportEdited = false; refreshRight(); };
  $('#copyreport').onclick = async () => {
    const b = $('#copyreport');
    try { await navigator.clipboard.writeText($('#report').value); }
    catch { $('#report').select(); document.execCommand('copy'); }
    b.textContent = t('report.copied');
    b.classList.add('done');
    setTimeout(() => { b.textContent = t('report.copy'); b.classList.remove('done'); }, 1500);
  };
}

export function setupExport() {
  lastPreviewKey = '';
  $('#reasonother').value = '';
  renderReasons();
  renderRight();
  renderMarkLinks();
  if (rightPlayer) rightPlayer.marksChanged();
  refreshRight();
}

export function renderExportOnLang() {
  renderRight();
  renderFiles();
  renderMarkLinks();
  renderReasons();
  if (rightPlayer) rightPlayer.redraw();
}

export function redrawExport() {
  if (rightPlayer) rightPlayer.redraw();
}

export function toggleReportMark() {
  if (rightPlayer) rightPlayer.toggleMark();
}
