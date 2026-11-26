// Translation lookup, placeholder interpolation, and locale loading (frontend/lang/*.json).
let EN = {};
let TR = {};

function safeGet(key) {
  try { return localStorage.getItem(key); } catch { return null; }
}

function safeSet(key, val) {
  try { localStorage.setItem(key, val); } catch {}
}

/** Translate a key using translation and fallback dictionaries, interpolating `{var}` placeholders. */
export function translate(tr, en, key, vars) {
  let s = (tr && tr[key] != null) ? tr[key] : (en && en[key] != null) ? en[key] : key;
  if (vars) {
    for (const [a, b] of Object.entries(vars)) {
      s = s.split('{' + a + '}').join(b);
    }
  }
  return s;
}

/** Translate an optional prefixed key if present in either dictionary; otherwise return the raw value. */
export function translateOpt(tr, en, prefix, value) {
  const k = prefix + value;
  const hit = (tr && tr[k] != null) ? tr[k] : (en && en[k] != null) ? en[k] : undefined;
  return hit !== undefined ? translate(tr, en, k) : value;
}

/** Create standalone `t` and `tOpt` functions bound to the given dictionaries. */
export function createTranslator(tr = {}, en = {}) {
  return {
    t: (key, vars) => translate(tr, en, key, vars),
    tOpt: (prefix, value) => translateOpt(tr, en, prefix, value),
  };
}

export function setDicts(en, tr = {}) {
  EN = en || {};
  TR = tr || {};
}

export function t(key, vars) {
  return translate(TR, EN, key, vars);
}

export function tOpt(prefix, value) {
  return translateOpt(TR, EN, prefix, value);
}

export function applyStatic(root = document) {
  root.querySelectorAll('[data-i18n]').forEach(e => { e.textContent = t(e.dataset.i18n); });
  root.querySelectorAll('[data-i18n-title]').forEach(e => { e.title = t(e.dataset.i18nTitle); });
  root.querySelectorAll('[data-i18n-ph]').forEach(e => { e.placeholder = t(e.dataset.i18nPh); });
}

export async function loadLang(code, onLoaded) {
  const get = c => fetch(`lang/${c}.json`).then(r => r.ok ? r.json() : {}).catch(() => ({}));
  EN = await get('en');
  TR = code && code !== 'en' ? await get(code) : {};
  document.documentElement.lang = code || 'en';
  applyStatic();
  if (onLoaded) onLoaded(code);
}

export async function initLang(onLoaded) {
  let codes = ['en'];
  try { codes = await fetch('lang/index.json').then(r => r.json()); } catch {}
  const sel = document.querySelector('#lang');
  let want = safeGet('lang') || navigator.language || 'en';
  if (!codes.includes(want)) want = codes.find(c => c.split('-')[0] === want.split('-')[0]) || 'en';
  if (sel && codes.length > 1) {
    for (const c of codes) {
      const j = await fetch(`lang/${c}.json`).then(r => r.json()).catch(() => ({}));
      const o = document.createElement('option');
      o.textContent = j._name || c;
      o.value = c;
      sel.append(o);
    }
    sel.value = want;
    sel.classList.remove('hidden');
    sel.onchange = () => {
      safeSet('lang', sel.value);
      loadLang(sel.value, onLoaded);
    };
  }
  await loadLang(want, onLoaded);
}
