import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const storeSrc = fs.readFileSync(path.join(rootDir, 'frontend', 'opfs_store.js'), 'utf8');
const DAY = 24 * 60 * 60 * 1000;

const notFound = () => new DOMException('not found', 'NotFoundError');

// In-memory stand-in for the OPFS directory and file handles the store uses.
class MemDir {
  constructor() {
    this.files = new Map();
    this.dirs = new Map();
  }
  async getDirectoryHandle(name, { create = false } = {}) {
    if (!this.dirs.has(name)) {
      if (!create) throw notFound();
      this.dirs.set(name, new MemDir());
    }
    return this.dirs.get(name);
  }
  async getFileHandle(name, { create = false } = {}) {
    if (!this.files.has(name)) {
      if (!create) throw notFound();
      this.files.set(name, new Uint8Array(0));
    }
    return new MemFileHandle(this, name);
  }
  async removeEntry(name) {
    if (!this.files.delete(name) && !this.dirs.delete(name)) throw notFound();
  }
  async *keys() {
    yield* [...this.files.keys(), ...this.dirs.keys()];
  }
}

class MemFileHandle {
  constructor(dir, name) {
    this.dir = dir;
    this.name = name;
  }
  async getFile() {
    return new Blob([this.dir.files.get(this.name)]);
  }
  async createSyncAccessHandle() {
    const { dir, name } = this;
    return {
      truncate(n) { dir.files.set(name, dir.files.get(name).slice(0, n)); },
      write(buf, { at = 0 } = {}) {
        const cur = dir.files.get(name);
        const out = new Uint8Array(Math.max(cur.length, at + buf.length));
        out.set(cur);
        out.set(buf, at);
        dir.files.set(name, out);
        return buf.length;
      },
      flush() {},
      close() {},
    };
  }
}

function loadStore({ withStorage = true, locks = null } = {}) {
  const root = new MemDir();
  const navigatorObj = withStorage ? { storage: { getDirectory: async () => root } } : {};
  if (locks) navigatorObj.locks = locks;
  const sandbox = {
    Uint8Array, Blob, TextEncoder, TextDecoder, DOMException, crypto,
    navigator: navigatorObj,
    FileSystemFileHandle: MemFileHandle,
  };
  sandbox.self = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(storeSrc, sandbox, { filename: 'opfs_store.js' });
  return { store: sandbox.opfsStore, root };
}

const sessions = root => root.dirs.get('sessions');
const bytes = (...v) => new Uint8Array(v);
// Arrays made inside the vm sandbox have another Array prototype; copy them out.
const keysOf = async (store, now) => Array.from(await store.list(now), e => e.key);

test('available needs OPFS sync access handles', () => {
  assert.equal(loadStore().store.available(), true);
  assert.equal(loadStore({ withStorage: false }).store.available(), false);
});

test('put and get round-trip a snapshot and track when it was opened', async () => {
  const { store } = loadStore();
  await store.put('k1', bytes(1, 2, 3), { map: 'de_mirage' }, 1000);
  assert.deepEqual([...await store.get('k1', 5000)], [1, 2, 3]);
  const [entry] = await store.list(6000);
  assert.equal(entry.key, 'k1');
  assert.equal(entry.map, 'de_mirage');
  assert.equal(entry.bytes, 3);
  assert.equal(entry.savedAt, 1000);
  assert.equal(entry.openedAt, 5000);
  assert.equal(await store.get('missing'), null);
});

test('list drops demos not opened for 7 days', async () => {
  const { store, root } = loadStore();
  await store.put('old', bytes(1), {}, 0);
  await store.put('new', bytes(2), {}, 6 * DAY);
  const keys = await keysOf(store, 7 * DAY + 1);
  assert.deepEqual(keys, ['new']);
  assert.equal(sessions(root).files.has('old.bin'), false);
});

test('only the most recently opened demos are kept', async () => {
  const { store } = loadStore();
  for (let i = 0; i < store.KEEP_MAX; i++) {
    await store.put(`k${i}`, bytes(i), {}, 1000 + i);
  }
  await store.get('k0', 2000); // reopened: now the most recent, not the oldest
  await store.put('x1', bytes(1), {}, 3000);
  await store.put('x2', bytes(2), {}, 3001);
  const keys = await keysOf(store, 4000);
  assert.equal(keys.length, store.KEEP_MAX);
  assert.deepEqual(keys.slice(0, 3), ['x2', 'x1', 'k0']);
  assert.ok(!keys.includes('k1') && !keys.includes('k2'));
});

test('snapshots without an index entry are removed', async () => {
  const { store, root } = loadStore();
  await store.put('k1', bytes(1), {}, 1000);
  sessions(root).files.set('stray.bin', bytes(9));
  await store.list(1000);
  assert.deepEqual([...sessions(root).files.keys()].sort(), ['index.json', 'k1.bin']);
});

test('an index entry whose snapshot is gone is dropped', async () => {
  const { store, root } = loadStore();
  await store.put('k1', bytes(1), {}, 1000);
  sessions(root).files.delete('k1.bin');
  assert.equal(await store.get('k1', 1000), null);
  assert.deepEqual(await keysOf(store, 1000), []);
});

test('a broken index costs the cache, not an error', async () => {
  const { store, root } = loadStore();
  await store.put('k1', bytes(1), {}, 1000);
  sessions(root).files.set('index.json', new TextEncoder().encode('{nope'));
  assert.equal(await store.get('k1', 1000), null);
  assert.deepEqual(await keysOf(store, 1000), []);
});

test('remove and clear delete saved demos', async () => {
  const { store, root } = loadStore();
  await store.put('k1', bytes(1), {}, 1000);
  await store.put('k2', bytes(2), {}, 1000);
  await store.remove('k1');
  assert.deepEqual(await keysOf(store, 1000), ['k2']);
  await store.clear();
  assert.equal(root.dirs.has('sessions'), false);
  assert.deepEqual(await keysOf(store, 1000), []);
});

test('demoKey depends on the content, not the file name', async () => {
  const { store } = loadStore();
  const big = new Uint8Array(3 << 20).map((_, i) => i % 251);
  const other = big.slice();
  other[other.length - 1] ^= 1; // differs only in the last MiB
  const a = await store.demoKey(new File([big], 'a.dem.zst'));
  const b = await store.demoKey(new File([big], 'renamed.dem.zst'));
  const c = await store.demoKey(new File([other], 'a.dem.zst'));
  assert.equal(a, b);
  assert.notEqual(a, c);
  assert.match(a, /^[0-9a-z]+-[0-9a-f]{24}$/);
});

test('concurrent put and list serialize and use navigator.locks when available', async () => {
  const lockCalls = [];
  let active = 0;
  let maxActive = 0;
  const locks = {
    async request(name, fn) {
      lockCalls.push(name);
      active++;
      maxActive = Math.max(maxActive, active);
      try { return await fn(); } finally { active--; }
    },
  };
  const { store } = loadStore({ locks });
  await Promise.all([
    store.put('k1', bytes(1, 2), { map: 'de_mirage' }, 1000),
    store.list(1000),
    store.put('k2', bytes(3, 4), { map: 'de_inferno' }, 1001),
    store.get('k1', 1002),
  ]);
  assert.equal(maxActive, 1);
  assert.equal(lockCalls.length, 4);
  assert.deepEqual(await keysOf(store, 1003), ['k1', 'k2']);
  assert.deepEqual([...await store.get('k1', 1004)], [1, 2]);
  assert.deepEqual([...await store.get('k2', 1004)], [3, 4]);
});
