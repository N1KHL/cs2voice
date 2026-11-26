// Shared DOM helpers, period/player list rows, error formatting, and file downloads.
import { t, tOpt } from './i18n.js';
import { missingFeatures } from './api.js';

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

export function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

export function clock(sec) {
  const s = Math.max(0, Math.round(sec));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

export function debounce(fn, ms) {
  let timer;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

export const store = {
  get(k) { try { return localStorage.getItem(k); } catch { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch {} },
};

export function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export function formatError(err) {
  if (!err) return '';
  if (typeof err === 'string') return err;
  const kind = err.kind || '';
  const name = err.name || '';
  const msg = err.message || String(err);
  if (kind === 'unsupported') {
    if (Array.isArray(err.features) && err.features.length) {
      return t('error.unsupportedBrowser', { features: err.features.join(', ') });
    }
    if (missingFeatures.length) {
      return t('error.unsupportedBrowser', { features: missingFeatures.join(', ') });
    }
    return msg;
  }
  if (
    kind === 'oom' ||
    name === 'RangeError' ||
    name === 'RuntimeError' ||
    (typeof WebAssembly !== 'undefined' && WebAssembly.RuntimeError && err instanceof WebAssembly.RuntimeError) ||
    /out of memory|cannot allocate|memory access out of bounds|allocation failed|runtimeerror|\boom\b/i.test(msg)
  ) {
    return t('error.outOfMemory');
  }
  if (
    kind === 'invalidDemo' ||
    /failed to read demo|invalid file-type|hl2demo|errinvalidfiletype|magic number|corrupt|unexpected eof|zstd/i.test(msg)
  ) {
    return t('error.invalidDemo');
  }
  return msg;
}

export function showError(err) {
  const w = $('#warnings');
  if (!w) return;
  w.textContent = formatError(err);
  w.classList.remove('hidden');
}

export function periodName(p) {
  return p.kind === 'round' ? t('period.round', { n: p.number }) : tOpt('period.', p.label);
}

export function periodRow(p, checked, talk, onToggle, disabled) {
  const r = el('div', 'row' + (checked ? ' on' : '') + (disabled ? ' disabled' : ''));
  const cb = el('input');
  cb.type = 'checkbox';
  cb.checked = checked;
  cb.disabled = !!disabled;
  if (disabled) r.title = disabled;
  r.append(cb, el('span', 'lbl', p.kind === 'round' ? `R${String(p.number).padStart(2, '0')}` : periodName(p)));
  const sc = el('span', 'score');
  const win = p.winner ? t('period.win', { side: p.winner }) : '';
  if (p.kind === 'round') {
    sc.textContent = `${p.scoreCT}-${p.scoreT}  ${win ? win + ' · ' + tOpt('reason.', p.reason || '') : ''}`;
  } else if (p.kind === 'knife') {
    sc.textContent = win ? `${win} · ${t('period.picksSide')}` : clock(p.length);
  } else if (p.kind === 'break') {
    sc.textContent = '';
  } else {
    sc.textContent = clock(p.length);
  }
  r.append(sc);
  if (p.allTalk) r.append(el('span', 'badge alltalk', t('period.allTalk')));
  r.append(el('span', 'talk' + (talk > 0 ? ' has' : ''), talk > 0 ? clock(talk) : '—'));
  r.onclick = e => {
    if (disabled) return;
    if (e.target !== cb) cb.checked = !cb.checked;
    onToggle(cb.checked);
    r.classList.toggle('on', cb.checked);
  };
  return r;
}

// Team header row with a checkbox that selects / clears the whole team.
export function teamHead(label, ids, set, onChange, extra) {
  const h = el('div', 'teamhead');
  const cb = el('input');
  cb.type = 'checkbox';
  const n = ids.filter(id => set.has(id)).length;
  cb.checked = ids.length > 0 && n === ids.length;
  cb.indeterminate = n > 0 && n < ids.length;
  h.append(cb, el('span', '', label));
  if (extra) h.append(el('span', 'cnt', extra));
  h.title = t('common.wholeTeam');
  h.onclick = () => {
    const on = n !== ids.length;
    ids.forEach(id => on ? set.add(id) : set.delete(id));
    onChange();
  };
  return h;
}

export function renderTeamSwitch(container, teams, activeTeam, onSelect) {
  container.innerHTML = '';
  for (const tm of teams) {
    const b = el('button', tm === activeTeam ? 'on' : '', tm || t('common.spectators'));
    b.onclick = () => onSelect(tm);
    container.append(b);
  }
}

export function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

export async function downloadResult(res, defaultZipName = 'cs2voice.zip') {
  if (!res) return;
  if (res.zipBytes) {
    downloadBlob(new Blob([res.zipBytes], { type: 'application/zip' }), res.zipName || defaultZipName);
    return;
  }
  const list = res.files || [];
  for (let idx = 0; idx < list.length; idx++) {
    const item = list[idx];
    const mime = /\.wav$/i.test(item.name) ? 'audio/wav' : 'audio/mpeg';
    downloadBlob(new Blob([item.bytes], { type: mime }), item.name);
    if (idx + 1 < list.length) await sleep(250);
  }
  if (res.reportText) {
    if (list.length) await sleep(250);
    downloadBlob(new Blob([res.reportText], { type: 'text/plain;charset=utf-8' }), res.reportFile || 'report.txt');
  }
}

export async function saveFileToDir(dirHandle, filename, data) {
  const fh = await dirHandle.getFileHandle(filename, { create: true });
  const writable = await fh.createWritable();
  await writable.write(data);
  await writable.close();
}

export async function uniqueReportName(dirHandle) {
  const exists = async name => {
    try { await dirHandle.getFileHandle(name); return true; }
    catch { return false; }
  };
  if (!(await exists('report.txt'))) return 'report.txt';
  for (let idx = 2; ; idx++) {
    const cand = `report (${idx}).txt`;
    if (!(await exists(cand))) return cand;
  }
}
