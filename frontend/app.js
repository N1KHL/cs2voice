// Top-level UI orchestration: startup checks, demo loading, tabs, theme, and About dialog.
import { t, initLang } from './i18n.js';
import { M, activeTab, setActiveTab, setMatch } from './state.js';
import { api, pickDemoFile, browserSupported, missingFeatures, setProgressHandler, revokeAllPreviewUrls } from './api.js';
import { $, $$, el, store, showError } from './ui.js';
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

async function openDemo(file) {
  if (!file || !browserSupported) return;
  $('#warnings').classList.add('hidden');
  $('#empty').classList.add('hidden');
  $('#main').classList.add('hidden');
  $('#cutview').classList.add('hidden');
  $('#loading').classList.remove('hidden');
  $('#loadingname').textContent = file.name;
  $('#loadingbar').value = 0;
  $('#loadingpct').textContent = '';

  let match;
  try {
    match = await api('OpenDemo', file);
  } catch (e) {
    $('#loading').classList.add('hidden');
    if (M) applyTab(); else $('#empty').classList.remove('hidden');
    return showError(e);
  }
  setMatch(match);
  $('#loading').classList.add('hidden');
  $('#tabs').classList.remove('hidden');
  applyTab();
  setup();
}

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
  if (!M) return;
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
