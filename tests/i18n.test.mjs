import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const langDir = path.join(rootDir, 'frontend', 'lang');
const i18nModulePath = path.join(rootDir, 'frontend', 'i18n.js');
const hasI18nModule = fs.existsSync(i18nModulePath);
const i18nMod = hasI18nModule ? await import(pathToFileURL(i18nModulePath).href) : null;

function readJSON(relPath) {
  return JSON.parse(fs.readFileSync(path.join(langDir, relPath), 'utf8'));
}

function placeholders(str) {
  const matches = str.match(/\{[a-zA-Z0-9_]+\}/g) || [];
  return [...new Set(matches)].sort();
}

test('lang/index.json lists en and pt-BR and every listed locale parses cleanly', () => {
  const index = readJSON('index.json');
  assert.ok(Array.isArray(index), 'lang/index.json must be an array');
  assert.ok(index.includes('en'), 'lang/index.json must include en');
  assert.ok(index.includes('pt-BR'), 'lang/index.json must include pt-BR');

  for (const code of index) {
    const data = readJSON(`${code}.json`);
    assert.equal(typeof data, 'object');
    assert.notEqual(data, null);
    assert.equal(typeof data._name, 'string', `${code}.json must have string _name`);
    assert.ok(data._name.trim().length > 0, `${code}.json _name must not be empty`);
  }
});

// en.json is the source of truth. pt-BR.json may lag behind: missing keys fall
// back to English at runtime (translate in i18n.js), but it must never carry
// keys that en.json no longer has.
test('pt-BR.json has no keys that en.json lacks', () => {
  const en = readJSON('en.json');
  const pt = readJSON('pt-BR.json');

  const extraInPt = Object.keys(pt).filter(k => !(k in en));
  assert.deepEqual(extraInPt, [], `Keys in pt-BR.json but not in en.json: ${extraInPt.join(', ')}`);
});

test('en.json and pt-BR.json have non-empty strings and matching {var} placeholders', () => {
  const en = readJSON('en.json');
  const pt = readJSON('pt-BR.json');

  for (const key of Object.keys(en)) {
    assert.equal(typeof en[key], 'string', `en.json[${key}] must be a string`);
    assert.ok(en[key].length > 0, `en.json[${key}] must not be empty`);
  }

  for (const key of Object.keys(pt)) {
    assert.equal(typeof pt[key], 'string', `pt-BR.json[${key}] must be a string`);
    assert.ok(pt[key].length > 0, `pt-BR.json[${key}] must not be empty`);
    if (!(key in en)) continue; // reported by the extra-keys test

    assert.deepEqual(
      placeholders(pt[key]),
      placeholders(en[key]),
      `Placeholder mismatch for key "${key}": en=${en[key]} | pt-BR=${pt[key]}`
    );
  }
});

test(
  'i18n.js translate, translateOpt, and createTranslator handle interpolation and fallbacks',
  { skip: hasI18nModule ? false : 'awaiting refactor-frontend merge (frontend/i18n.js)' },
  () => {
    const { translate, translateOpt, createTranslator } = i18nMod;
    const en = readJSON('en.json');
    const pt = readJSON('pt-BR.json');

    // Direct lookup, EN fallback, and raw key fallback
    assert.equal(translate({ k: 'PT' }, { k: 'EN' }, 'k'), 'PT');
    assert.equal(translate({}, { k: 'EN' }, 'k'), 'EN');
    assert.equal(translate(undefined, undefined, 'missing.key'), 'missing.key');

    // Placeholder interpolation (including repeated placeholders and numeric 0)
    assert.equal(
      translate({}, { msg: '{a} -> {b} ({a}, {n})' }, 'msg', { a: 'X', b: 'Y', n: 0 }),
      'X -> Y (X, 0)'
    );

    // translateOpt: prefix lookup with fallback to raw value
    assert.equal(translateOpt(pt, en, 'period.', 'Halftime'), pt['period.Halftime']);
    assert.equal(translateOpt({}, en, 'reason.', 'bomb defused'), en['reason.bomb defused']);
    assert.equal(translateOpt(pt, en, 'reason.', 'unknown reason'), 'unknown reason');

    // createTranslator binds tr and en cleanly
    const { t, tOpt } = createTranslator(pt, en);
    assert.equal(t('period.round', { n: 12 }), pt['period.round'].replace('{n}', '12'));
    assert.equal(tOpt('period.', 'Knife'), pt['period.Knife']);
    assert.equal(tOpt('period.', 'Custom Period'), 'Custom Period');
  }
);
