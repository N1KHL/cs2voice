// Top-level UI orchestration: startup checks, demo loading, saved sessions,
// tabs, theme, and About dialog.
import { t, initLang } from './i18n.js';
import { M, R, C, activeTab, setActiveTab, setMatch, snapshotState, restoreState } from './state.js';
import { api, pickDemoFile, browserSupported, missingFeatures, setProgressHandler, revokeAllPreviewUrls } from './api.js';
import { $, $$, el, debounce, store, tabStore, showError } from './ui.js';
import { initListenPanel, setupListen, renderLeft, redrawListen } from './listen_panel.js';
import { initExportPanel, setupExport, renderExportOnLang, redrawExport, toggleReportMark } from './export_panel.js';
import { initCut, setupCut, renderCut, redrawCut, onShowCut, toggleCutRange } from './cut.js';

initListenPanel();
initExportPanel();
initCut();

setProgressHandler(pct => {
  const bar = $('#loadingbar');
  const lbl = $('#loadingpct');
  if (bar) bar.value = pct;
  if (lbl) lbl.textContent = `${Math.round(pct * 100)}%`;
});

function renderInfo() {
  if (!M) return;
  const info = $('#matchinfo');
  info.innerHTML = '';
  const add = (k, v) => {
    const s = el('span');
    s.append(t(k) + ' ');
    s.append(el('b', '', v));
    info.append(s);
  };
  add('info.map', M.map || '?');
  if (M.finalScore) add('info.score', M.finalScore);
  if (M.faceitMatchId) add('info.faceit', M.faceitMatchId);
  add('info.gameVersion', String(M.gameVersion));
  add('info.voice', M.voiceFormat || t('info.voiceNone'));
  add('info.allTalk', M.allTalkSource === 'chat' ? t('info.allTalkChat') : t('info.allTalkOff'));
  if (openedFromCache) info.append(el('span', 'dim', t('info.fromCache')));
}

function applyTab() {
  $$('#tabs button').forEach(b => b.classList.toggle('on', b.dataset.tab === activeTab));
  if (!M) return;
  $('#main').classList.toggle('hidden', activeTab !== 'report');
  $('#cutview').classList.toggle('hidden', activeTab !== 'cut');
  if (activeTab === 'cut') onShowCut();
}

function setup() {
  revokeAllPreviewUrls();
  renderInfo();

  const w = $('#warnings');
  if (M.warnings && M.warnings.length) {
    w.textContent = M.warnings.join('  •  ');
    w.classList.remove('hidden');
  } else {
    w.classList.add('hidden');
  }

  setupListen();
  setupExport();
  setupCut();
}

// ---------- opening demos & saved sessions ----------

// A demo's parsed match is cached by the worker (OPFS); its selections, marks
// and report edits are saved here under SESSION_PREFIX + key. Both are dropped
// after KEEP_MS without opening. OPEN_KEY in tabStore says which demo this tab
// had open, so a reload brings it back while a fresh visit shows the list.
const SESSION_PREFIX = 'session:';
const OPEN_KEY = 'openDemo';
const KEEP_MS = 7 * 24 * 60 * 60 * 1000;

let demoKey = '';          // cache key of the open demo; '' = this demo is not saved
let openedFromCache = false;
let sessionSaved = false;  // the open demo's selections are being saved
let sessionRev = 0;        // revision of the session last loaded or written by this tab
let openSeq = 0;           // monotonic counter to ignore superseded open completions

function readSession(key) {
  try { return JSON.parse(store.get(SESSION_PREFIX + key)); } catch { return null; }
}

function saveSession() {
  if (!M || !demoKey) return;
  const existing = readSession(demoKey);
  if (existing && (existing.rev || 0) > sessionRev) return; // another tab saved a newer revision
  const nextRev = sessionRev + 1;
  const ok = store.set(SESSION_PREFIX + demoKey, JSON.stringify({ ...snapshotState(), rev: nextRev, savedAt: Date.now() }));
  if (ok) sessionRev = nextRev;
  sessionSaved = ok;
}
const autosave = debounce(saveSession, 500);

function pruneSessions(now = Date.now()) {
  for (const k of store.keys(SESSION_PREFIX)) {
    const saved = readSession(k.slice(SESSION_PREFIX.length));
    if (!saved || !(now - (saved.savedAt || 0) < KEEP_MS)) store.remove(k);
  }
}

function showLoading(name) {
  $('#warnings').classList.add('hidden');
  $('#empty').classList.add('hidden');
  $('#main').classList.add('hidden');
  $('#cutview').classList.add('hidden');
  $('#loading').classList.remove('hidden');
  $('#loadingname').textContent = name;
  $('#loadingbar').value = 0;
  $('#loadingpct').textContent = '';
}

function loadFailed(e) {
  $('#loading').classList.add('hidden');
  if (M) applyTab(); else showEmpty();
  showError(e);
}

function showMatch({ key, match, cached }, seq) {
  saveSession(); // the demo open until now
  setMatch(match);
  demoKey = key || '';
  openedFromCache = !!cached;
  sessionSaved = false;
  sessionRev = 0;
  if (demoKey) {
    const saved = readSession(demoKey);
    if (saved) {
      sessionRev = saved.rev || 0;
      restoreState(saved);
    }
    if (cached) tabStore.set(OPEN_KEY, demoKey);
    else tabStore.remove(OPEN_KEY);
    saveSession();
  } else {
    tabStore.remove(OPEN_KEY);
  }
  $('#loading').classList.add('hidden');
  $('#tabs').classList.remove('hidden');
  applyTab();
  setup();
  if (demoKey && !cached) {
    const savedKey = demoKey;
    const meta = {
      file: match.file, map: match.map, score: match.finalScore,
      faceitMatchId: match.faceitMatchId, duration: match.duration,
    };
    api('SaveDemo', savedKey, meta).then(
      () => { if (seq === openSeq && demoKey === savedKey) tabStore.set(OPEN_KEY, savedKey); },
      () => { if (seq === openSeq && tabStore.get(OPEN_KEY) === savedKey) tabStore.remove(OPEN_KEY); },
    ); // best effort: next time it parses again
  }
}

async function openDemo(file) {
  if (!file || !browserSupported) return;
  const seq = ++openSeq;
  showLoading(file.name);
  try {
    const res = await api('OpenDemo', file);
    if (seq !== openSeq) return;
    showMatch(res, seq);
  } catch (e) {
    if (seq !== openSeq) return;
    loadFailed(e);
  }
}

async function openRecent(key, name) {
  const seq = ++openSeq;
  showLoading(name || t('app.restoring'));
  try {
    const res = await api('OpenRecent', key);
    if (seq !== openSeq) return false;
    showMatch(res, seq);
    return true;
  } catch (e) {
    if (seq !== openSeq) return false;
    tabStore.remove(OPEN_KEY);
    loadFailed(e.kind === 'sessionGone' ? t('error.sessionGone') : e);
    return false;
  }
}

function showEmpty() {
  $('#empty').classList.remove('hidden');
  renderRecent();
}

async function renderRecent() {
  const box = $('#recent');
  const list = $('#recentlist');
  let res;
  try { res = await api('ListRecent'); } catch { res = { entries: [] }; }
  pruneSessions();
  list.innerHTML = '';
  const hasOrphans = store.keys(SESSION_PREFIX).length > 0;
  box.classList.toggle('hidden', !res.entries.length && !hasOrphans);
  const fmt = new Intl.DateTimeFormat(document.documentElement.lang || 'en', { dateStyle: 'short', timeStyle: 'short' });
  for (const e of res.entries) {
    const li = el('li');
    const open = el('button', 'recent-open');
    open.append(el('b', '', e.map || e.file || '?'));
    if (e.score) open.append(el('span', '', e.score));
    if (e.faceitMatchId) open.append(el('span', 'dim', e.faceitMatchId));
    const saved = readSession(e.key);
    const marks = saved && saved.R && Array.isArray(saved.R.marks) ? saved.R.marks.length : 0;
    if (marks) open.append(el('span', 'recent-marks', t('recent.marks', { n: marks })));
    open.append(el('span', 'dim', t('recent.opened', { when: fmt.format(new Date(e.openedAt)) })));
    open.onclick = () => openRecent(e.key, e.file || e.map);
    const del = el('button', 'btn icon', '×');
    del.title = t('recent.remove');
    del.onclick = async () => {
      await api('ForgetDemo', e.key).catch(showError);
      store.remove(SESSION_PREFIX + e.key);
      renderRecent();
    };
    li.append(open, del);
    list.append(li);
  }
}

$('#clearsaved').onclick = async () => {
  if (!confirm(t('recent.clearConfirm'))) return;
  try {
    await api('ClearSaved');
  } catch (e) {
    showError(e);
  } finally {
    for (const k of store.keys(SESSION_PREFIX)) store.remove(k);
    renderRecent();
  }
};

// Any interaction may change a selection, a mark or the report; saving a
// snapshot is cheap, so this beats teaching every panel to save.
for (const type of ['input', 'change', 'click', 'pointerup', 'keyup']) {
  document.addEventListener(type, autosave, true);
}
window.addEventListener('pagehide', saveSession);
document.addEventListener('visibilitychange', () => { if (document.hidden) saveSession(); });
window.addEventListener('storage', e => {
  if (!M || !demoKey || e.key !== SESSION_PREFIX + demoKey || !e.newValue) return;
  let saved;
  try { saved = JSON.parse(e.newValue); } catch { return; }
  if (!saved || (saved.rev || 0) <= sessionRev) return;
  const before = JSON.stringify(snapshotState());
  sessionRev = saved.rev || 0;
  if (!restoreState(saved)) return;
  sessionSaved = true;
  if (JSON.stringify(snapshotState()) !== before) {
    applyTab();
    setup();
  }
});

// Only when the work cannot be saved (private window, storage blocked) does
// leaving the page lose it, so only then ask before leaving.
window.addEventListener('beforeunload', e => {
  if (!M || sessionSaved) return;
  if (R.marks.length || R.reasons.size || R.reasonOther || R.reportEdited || C.ranges.length) {
    e.preventDefault();
    e.returnValue = '';
  }
});

// ---------- header & shortcuts ----------

$$('#tabs button').forEach(b => {
  b.onclick = () => {
    setActiveTab(b.dataset.tab);
    applyTab();
  };
});

$('#theme').onclick = () => {
  const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  store.set('theme', next);
  redrawListen();
  redrawExport();
  redrawCut();
};

$('#about').onclick = async () => {
  try {
    const a = await api('About');
    let s = t('about.version', { v: a.version });
    if (a.commit) s += ' · ' + t('about.build', { c: a.commit }) + (a.modified ? ' ' + t('about.modified') : '');
    $('#aboutver').textContent = s;
  } catch (e) {
    showError(e);
  }
  $('#aboutdlg').showModal();
};
$('#closeabout').onclick = () => $('#aboutdlg').close();
$('#showlicenses').onclick = async () => {
  const pre = $('#licenses');
  if (!pre.textContent) {
    try { pre.textContent = await api('Licenses'); }
    catch (e) { showError(e); return; }
  }
  pre.classList.toggle('hidden');
};

$('#open').onclick = async () => {
  if (!browserSupported) return;
  const file = await pickDemoFile();
  if (file) openDemo(file);
};

let dragDepth = 0;
document.body.addEventListener('dragenter', e => {
  if (!e.dataTransfer || !e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes('Files')) return;
  e.preventDefault();
  dragDepth++;
  document.body.classList.add('file-drop-target-active');
});
document.body.addEventListener('dragover', e => {
  if (!e.dataTransfer || !e.dataTransfer.types || !Array.from(e.dataTransfer.types).includes('Files')) return;
  e.preventDefault();
});
document.body.addEventListener('dragleave', () => {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) document.body.classList.remove('file-drop-target-active');
});
document.body.addEventListener('drop', e => {
  e.preventDefault();
  dragDepth = 0;
  document.body.classList.remove('file-drop-target-active');
  if (!browserSupported) return;
  const file = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
  if (!file) return;
  if (/\.(dem|zst)$/i.test(file.name)) openDemo(file);
  else showError(t('app.notDemo'));
});

window.addEventListener('keydown', e => {
  if (e.target.closest && e.target.closest('textarea, input[type=text], select')) return;
  if ((e.key === 'm' || e.key === 'M') && activeTab === 'report') { toggleReportMark(); e.preventDefault(); }
  if ((e.key === 'c' || e.key === 'C') && activeTab === 'cut') { toggleCutRange(); e.preventDefault(); }
});

// ---------- startup ----------

initLang(() => {
  if (!M) { renderRecent(); return; }
  renderInfo();
  renderLeft();
  redrawListen();
  renderExportOnLang();
  renderCut();
  redrawCut();
}).finally(async () => {
  if (!browserSupported) {
    $('#open').disabled = true;
    showError(t('error.unsupportedBrowser', { features: missingFeatures.join(', ') }));
    return;
  }
  const openKey = tabStore.get(OPEN_KEY);
  if (openKey && await openRecent(openKey)) return;
  if (!M) showEmpty();
  if (location.hostname === '127.0.0.1' || location.hostname === 'localhost') {
    const urlParam = new URLSearchParams(location.search).get('url');
    if (urlParam) {
      const resolved = new URL(urlParam, location.href);
      if (resolved.origin === location.origin) {
        try {
          const resp = await fetch(resolved.href);
          if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
          const blob = await resp.blob();
          const name = resolved.pathname.split('/').pop() || 'demo.dem.zst';
          openDemo(new File([blob], name));
        } catch (err) {
          showError(err);
        }
      }
    }
  }
});
