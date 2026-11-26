// Parsed-match cache in the Origin Private File System (OPFS): a private folder
// on the user's disk that only this site can read, with no permission prompt.
// It keeps the parsed match (voice included), never the demo file, so a reload
// or reopening a recent demo skips the parse. Nothing here ever leaves the
// browser. The cache is best effort: the browser may clear it, so callers
// treat every failure as "not cached".
(() => {
  const DIR = 'sessions';
  const INDEX = 'index.json';
  const KEEP_MS = 7 * 24 * 60 * 60 * 1000; // since the demo was last opened
  const KEEP_MAX = 10;
  const HASH_SPAN = 1 << 20;

  function available() {
    return typeof navigator !== 'undefined' && !!navigator.storage &&
      typeof navigator.storage.getDirectory === 'function' &&
      typeof FileSystemFileHandle !== 'undefined' &&
      typeof FileSystemFileHandle.prototype.createSyncAccessHandle === 'function';
  }

  // demoKey identifies a demo file without reading all of it: the size plus a
  // hash of the first and last MiB tell two demos apart and survive a rename.
  async function demoKey(file) {
    const head = file.slice(0, HASH_SPAN);
    const tail = file.slice(Math.max(HASH_SPAN, file.size - HASH_SPAN));
    const bytes = await new Blob([head, tail, String(file.size)]).arrayBuffer();
    const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
    const hex = Array.from(hash.subarray(0, 12), b => b.toString(16).padStart(2, '0')).join('');
    return `${file.size.toString(36)}-${hex}`;
  }

  async function folder() {
    const root = await navigator.storage.getDirectory();
    return root.getDirectoryHandle(DIR, { create: true });
  }

  async function readBytes(dir, name) {
    let fh;
    try {
      fh = await dir.getFileHandle(name);
    } catch (err) {
      if (err && err.name === 'NotFoundError') return null;
      throw err;
    }
    return new Uint8Array(await (await fh.getFile()).arrayBuffer());
  }

  // writeBytes uses a sync access handle: the one write API every modern
  // browser offers in a worker.
  async function writeBytes(dir, name, bytes) {
    const fh = await dir.getFileHandle(name, { create: true });
    const h = await fh.createSyncAccessHandle();
    try {
      h.truncate(0);
      h.write(bytes, { at: 0 });
      h.flush();
    } finally {
      h.close();
    }
  }

  async function removeFile(dir, name) {
    try {
      await dir.removeEntry(name);
    } catch (err) {
      if (!err || err.name !== 'NotFoundError') throw err;
    }
  }

  async function readIndex(dir) {
    const bytes = await readBytes(dir, INDEX);
    if (!bytes) return {};
    try {
      const idx = JSON.parse(new TextDecoder().decode(bytes));
      return idx && typeof idx === 'object' && !Array.isArray(idx) ? idx : {};
    } catch {
      return {}; // a broken index only costs a re-parse
    }
  }

  function writeIndex(dir, idx) {
    return writeBytes(dir, INDEX, new TextEncoder().encode(JSON.stringify(idx)));
  }

  const binName = key => `${key}.bin`;

  // expiredKeys returns the entries to drop: not opened for KEEP_MS, or beyond
  // the KEEP_MAX most recently opened.
  function expiredKeys(idx, now) {
    const entries = Object.values(idx).sort((a, b) => (b.openedAt || 0) - (a.openedAt || 0));
    return entries
      .filter((e, i) => i >= KEEP_MAX || !(now - (e.openedAt || 0) < KEEP_MS))
      .map(e => e.key);
  }

  async function prune(dir, idx, now) {
    let changed = false;
    for (const key of expiredKeys(idx, now)) {
      await removeFile(dir, binName(key));
      delete idx[key];
      changed = true;
    }
    // Snapshots without an index entry (a write cut short) are garbage too.
    if (typeof dir.keys === 'function') {
      const orphans = [];
      for await (const name of dir.keys()) {
        if (name !== INDEX && !idx[name.replace(/\.bin$/, '')]) orphans.push(name);
      }
      for (const name of orphans) await removeFile(dir, name);
    }
    if (changed) await writeIndex(dir, idx);
    return idx;
  }

  async function put(key, bytes, meta, now = Date.now()) {
    const dir = await folder();
    await writeBytes(dir, binName(key), bytes);
    const idx = await readIndex(dir);
    idx[key] = { ...meta, key, bytes: bytes.byteLength, savedAt: now, openedAt: now };
    await writeIndex(dir, idx);
    await prune(dir, idx, now);
  }

  async function get(key, now = Date.now()) {
    const dir = await folder();
    const idx = await readIndex(dir);
    if (!idx[key]) return null;
    const bytes = await readBytes(dir, binName(key));
    if (!bytes) {
      delete idx[key];
      await writeIndex(dir, idx);
      return null;
    }
    idx[key].openedAt = now;
    await writeIndex(dir, idx);
    return bytes;
  }

  async function list(now = Date.now()) {
    const dir = await folder();
    const idx = await prune(dir, await readIndex(dir), now);
    return Object.values(idx).sort((a, b) => b.openedAt - a.openedAt);
  }

  async function remove(key) {
    const dir = await folder();
    const idx = await readIndex(dir);
    await removeFile(dir, binName(key));
    if (idx[key]) {
      delete idx[key];
      await writeIndex(dir, idx);
    }
  }

  async function clear() {
    const root = await navigator.storage.getDirectory();
    try {
      await root.removeEntry(DIR, { recursive: true });
    } catch (err) {
      if (!err || err.name !== 'NotFoundError') throw err;
    }
  }

  self.opfsStore = { available, demoKey, put, get, list, remove, clear, expiredKeys, KEEP_MS, KEEP_MAX };
})();
