import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const mp3BridgeSrc = fs.readFileSync(path.join(rootDir, 'frontend', 'mp3_bridge.js'), 'utf8');
const opusBridgeSrc = fs.readFileSync(path.join(rootDir, 'frontend', 'opus_bridge.js'), 'utf8');

function loadMp3Bridge({ WasmMediaEncoder } = {}) {
  const sandbox = {
    Uint8Array,
    Float32Array,
    DataView,
    ArrayBuffer,
    Error,
    console: { error: () => {} },
  };
  if (WasmMediaEncoder !== undefined) {
    sandbox.WasmMediaEncoder = WasmMediaEncoder;
  }
  sandbox.self = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(mp3BridgeSrc, sandbox, { filename: 'mp3_bridge.js' });
  return sandbox;
}

function loadOpusBridge({
  preloaded = true,
  withSyncReset = true,
  onReset = null,
  onDecode = null,
  onImportScripts = null,
} = {}) {
  let createdCount = 0;
  let syncFreeCalls = 0;
  let syncInitCalls = 0;
  let asyncResetCalls = 0;
  const createdDecoders = [];

  class MockOpusDecoder {
    constructor(opts) {
      this.opts = opts;
      this.id = ++createdCount;
      this.hi = 5;
      this.si = 6;
      this.Hi = 7;
      this.ready = Promise.resolve();
      if (withSyncReset) {
        this.ci = 8;
        this.Ii = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);
        this.yi = 100;
        this.li = 48000;
        this.di = 1;
        this.pi = 2;
        this.mi = 3;
        this.ui = 1000 + this.id;
        this.ei = {
          U: (len, TypedArr) => {
            const arr = new TypedArr(len);
            return { F: arr, Y: 4096 + this.id };
          },
          I: {
            q: ptr => {
              syncFreeCalls++;
              assert.ok(ptr > 0);
            },
            R: (yi, ci, li, di, mapPtr, pi, mi) => {
              syncInitCalls++;
              assert.equal(li, 48000);
              assert.equal(di, 1);
              assert.ok(mapPtr > 0);
              return 2000 + syncInitCalls;
            },
          },
        };
      }
      createdDecoders.push(this);
    }

    decodeFrame(pkt) {
      if (onDecode) return onDecode(this, pkt);
      return {
        samplesDecoded: 4,
        errors: [],
        channelData: [new Float32Array([0.1, -0.2, 0.3, -0.4])],
      };
    }

    reset() {
      asyncResetCalls++;
      if (onReset) return onReset(this, asyncResetCalls);
      return Promise.resolve();
    }
  }

  const sandbox = {
    Uint8Array,
    Float32Array,
    Map,
    Set,
    Array,
    Promise,
    Error,
    console: { error: () => {} },
    importScripts: (...scripts) => {
      if (onImportScripts) onImportScripts(scripts, sandbox);
      sandbox['opus-decoder'] = { OpusDecoder: MockOpusDecoder };
    },
  };
  if (preloaded) {
    sandbox['opus-decoder'] = { OpusDecoder: MockOpusDecoder };
  }
  sandbox.self = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(opusBridgeSrc, sandbox, { filename: 'opus_bridge.js' });

  return {
    sandbox,
    stats: () => ({
      createdCount,
      syncFreeCalls,
      syncInitCalls,
      asyncResetCalls,
      createdDecoders,
    }),
  };
}

test('mp3_bridge: mp3Ready rejects when WasmMediaEncoder is missing', async () => {
  const sb = loadMp3Bridge();
  await assert.rejects(sb.mp3Ready, /WasmMediaEncoder not loaded/);
  assert.equal(sb.cs2mp3Create(48000, 128), 0);
});

test('mp3_bridge: create, int16 LE -> Float32 [-1, 1], copied output, and flush', async () => {
  let configured = null;
  let encodedInputs = [];
  const wasmOutBuf = new Uint8Array([0xff, 0xfb, 0x90, 0x64]);
  const wasmFlushBuf = new Uint8Array([0x00, 0x01, 0x02]);

  const mockEncoder = {
    configure(opts) {
      configured = opts;
    },
    encode(channels) {
      encodedInputs.push(channels);
      return wasmOutBuf;
    },
    finalize() {
      return wasmFlushBuf;
    },
  };

  const sb = loadMp3Bridge({
    WasmMediaEncoder: {
      createEncoder: async (mime, wasmPath) => {
        assert.equal(mime, 'audio/mpeg');
        assert.equal(wasmPath, 'vendor/mp3.wasm');
        return mockEncoder;
      },
      jsLibraryVersion: () => 'wasm-media-encoders 0.7.0',
    },
  });

  await sb.mp3Ready;
  assert.equal(sb.cs2mp3Version(), 'wasm-media-encoders 0.7.0');

  const handle = sb.cs2mp3Create(48000, 192);
  assert.equal(handle, 1);
  assert.deepEqual({ ...configured }, { channels: 1, sampleRate: 48000, bitrate: 192 });

  // Prepare int16 LE samples at a non-zero byteOffset to verify DataView slicing:
  // padding (2 bytes) + [-32768, -16384, 0, 16384, 32767]
  const backing = new ArrayBuffer(12);
  const dv = new DataView(backing);
  dv.setInt16(0, 12345, true); // padding
  dv.setInt16(2, -32768, true);
  dv.setInt16(4, -16384, true);
  dv.setInt16(6, 0, true);
  dv.setInt16(8, 16384, true);
  dv.setInt16(10, 32767, true);
  const slice = new Uint8Array(backing, 2, 10);

  const mp3Chunk = sb.cs2mp3Encode(handle, slice);
  assert.equal(encodedInputs.length, 1);
  assert.equal(encodedInputs[0].length, 1);
  const f32 = encodedInputs[0][0];
  assert.ok(f32 instanceof Float32Array);
  assert.equal(f32.length, 5);
  assert.equal(f32[0], -1);
  assert.equal(f32[1], -0.5);
  assert.equal(f32[2], 0);
  assert.ok(Math.abs(f32[3] - 16384 / 32767) < 1e-6);
  assert.equal(f32[4], 1);

  // Returned Uint8Array must be copied out of WASM linear memory.
  assert.deepEqual(mp3Chunk, new Uint8Array([0xff, 0xfb, 0x90, 0x64]));
  wasmOutBuf.fill(0xaa);
  assert.deepEqual(mp3Chunk, new Uint8Array([0xff, 0xfb, 0x90, 0x64]));

  // Empty or 1-byte PCM input returns empty Uint8Array without calling encode.
  assert.deepEqual(sb.cs2mp3Encode(handle, new Uint8Array(1)), new Uint8Array(0));
  assert.deepEqual(sb.cs2mp3Encode(handle, null), new Uint8Array(0));
  assert.equal(encodedInputs.length, 1);

  // Flush returns a copied Uint8Array and invalidates handle.
  const flushed = sb.cs2mp3Flush(handle);
  assert.deepEqual(flushed, new Uint8Array([0x00, 0x01, 0x02]));
  wasmFlushBuf.fill(0x55);
  assert.deepEqual(flushed, new Uint8Array([0x00, 0x01, 0x02]));

  assert.deepEqual(sb.cs2mp3Flush(handle), new Uint8Array(0));
  assert.throws(() => sb.cs2mp3Encode(handle, slice), /invalid mp3 encoder handle/);
});

test('mp3_bridge: cs2mp3Version fallback and configure failure', async () => {
  const sb = loadMp3Bridge({
    WasmMediaEncoder: {
      createEncoder: async () => ({
        configure() {
          throw new Error('bad bitrate');
        },
      }),
    },
  });
  await sb.mp3Ready;
  assert.equal(sb.cs2mp3Version(), 'lame-wasm');
  assert.equal(sb.cs2mp3Create(48000, 999), 0);
});

test('opus_bridge: synchronous reset and pool reuse over >16 cycles', async () => {
  const { sandbox: sb, stats } = loadOpusBridge({ preloaded: true, withSyncReset: true });
  await sb.ensureOpusReady();

  assert.match(sb.cs2opusVersion(), /opus-decoder/);
  assert.equal(stats().createdCount, 16);

  // Run 40 synchronous create -> decode -> destroy cycles (exceeding POOL_SIZE = 16).
  for (let i = 0; i < 40; i++) {
    const h = sb.cs2opusCreate();
    assert.ok(h > 0, `expected valid handle at cycle ${i}`);
    const pcm = sb.cs2opusDecode(h, new Uint8Array([0xf8, 0xff, 0xfe]));
    assert.ok(pcm instanceof Float32Array);
    assert.equal(pcm.length, 4);
    sb.cs2opusDestroy(h);
  }

  const s = stats();
  assert.equal(s.createdCount, 16, 'pool should not allocate beyond 16 decoders');
  assert.equal(s.syncFreeCalls, 40);
  assert.equal(s.syncInitCalls, 40);
  assert.equal(s.asyncResetCalls, 0);

  // Exhausting all 16 active handles returns null on the 17th create until destroyed.
  const handles = [];
  for (let i = 0; i < 16; i++) handles.push(sb.cs2opusCreate());
  assert.equal(sb.cs2opusCreate(), null);
  for (const h of handles) sb.cs2opusDestroy(h);
  for (const dec of s.createdDecoders) {
    assert.equal(dec.hi, 0);
    assert.equal(dec.si, 0);
    assert.equal(dec.Hi, 0);
    assert.equal(dec._mapPtr, 4096 + dec.id);
  }
  assert.ok(sb.cs2opusCreate() > 0);
});

test('opus_bridge: decode edge cases and invalid handles', async () => {
  let mode = 'normal';
  const { sandbox: sb } = loadOpusBridge({
    onDecode: () => {
      if (mode === 'error') return { samplesDecoded: 0, errors: ['corrupt'], channelData: [] };
      if (mode === 'silence') return { samplesDecoded: 0, errors: [], channelData: [] };
      if (mode === 'null') return null;
      return { samplesDecoded: 2, errors: [], channelData: [new Float32Array([0.25, -0.25])] };
    },
  });
  await sb.ensureOpusReady();

  const h = sb.cs2opusCreate();
  assert.equal(sb.cs2opusDecode(9999, new Uint8Array([1])), null);
  assert.equal(sb.cs2opusDecode(h, new Uint8Array(0)), null);
  assert.equal(sb.cs2opusDecode(h, [1, 2, 3]), null);

  mode = 'error';
  assert.equal(sb.cs2opusDecode(h, new Uint8Array([1])), null);

  mode = 'silence';
  const empty = sb.cs2opusDecode(h, new Uint8Array([1]));
  assert.ok(empty instanceof Float32Array);
  assert.equal(empty.length, 0);

  mode = 'null';
  assert.equal(sb.cs2opusDecode(h, new Uint8Array([1])), null);

  mode = 'normal';
  const ok = sb.cs2opusDecode(h, new Uint8Array([1]));
  assert.ok(ok instanceof Float32Array);
  assert.equal(ok[0], 0.25);
  assert.equal(ok[1], -0.25);
});

test('opus_bridge: Worker stub during importScripts and async reset fallback', async () => {
  let workerWasFunction = false;
  let resetCalls = 0;
  const { sandbox: sb, stats } = loadOpusBridge({
    preloaded: false,
    withSyncReset: false,
    onImportScripts: (scripts, ctx) => {
      assert.deepEqual(scripts, ['vendor/opus-decoder.min.js']);
      workerWasFunction = typeof ctx.Worker === 'function';
    },
    onReset: () => {
      resetCalls++;
      if (resetCalls === 1) return Promise.reject(new Error('reset failed'));
      return Promise.resolve();
    },
  });

  assert.equal(workerWasFunction, true);
  assert.equal('Worker' in sb, false, 'temporary Worker stub must be cleaned up');

  await sb.ensureOpusReady();
  assert.equal(stats().createdCount, 16);

  // Exhaust all 16, destroy 2 via async reset (1 fails and falls back to makeDecoder, 1 succeeds).
  const handles = [];
  for (let i = 0; i < 16; i++) handles.push(sb.cs2opusCreate());
  assert.equal(sb.cs2opusCreate(), null);

  sb.cs2opusDestroy(handles[0]);
  sb.cs2opusDestroy(handles[1]);
  // Before async resets settle, pool is still empty.
  assert.equal(sb.cs2opusCreate(), null);

  await sb.ensureOpusReady();
  assert.equal(stats().asyncResetCalls, 2);
  assert.equal(stats().createdCount, 17, 'failed reset should recreate decoder');
  assert.ok(sb.cs2opusCreate() > 0);
  assert.ok(sb.cs2opusCreate() > 0);
  assert.equal(sb.cs2opusCreate(), null);
});
