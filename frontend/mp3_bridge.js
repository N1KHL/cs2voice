// Synchronous JS <-> WASM bridge for LAME MP3 encoding (wasm-media-encoders).
// Loaded by worker.js after vendor/WasmMediaEncoder.min.js.

let mp3Encoder = null;
let mp3NextHandle = 1;
let mp3ActiveHandle = 0;

self.mp3Ready = (async () => {
  if (typeof WasmMediaEncoder === 'undefined') {
    throw new Error('WasmMediaEncoder not loaded');
  }
  mp3Encoder = await WasmMediaEncoder.createEncoder('audio/mpeg', 'vendor/mp3.wasm');
  return mp3Encoder;
})();

self.cs2mp3Create = function (sampleRate, kbps) {
  if (!mp3Encoder) return 0;
  try {
    mp3Encoder.configure({
      channels: 1,
      sampleRate: sampleRate,
      bitrate: kbps,
    });
    mp3ActiveHandle = mp3NextHandle++;
    return mp3ActiveHandle;
  } catch (err) {
    console.error('cs2mp3Create failed:', err);
    return 0;
  }
};

self.cs2mp3Encode = function (handle, pcmInt16Uint8Array) {
  if (!mp3Encoder || handle !== mp3ActiveHandle) {
    throw new Error('invalid mp3 encoder handle');
  }
  if (!pcmInt16Uint8Array || pcmInt16Uint8Array.byteLength < 2) {
    return new Uint8Array(0);
  }
  const count = pcmInt16Uint8Array.byteLength >> 1;
  const pcm = new Float32Array(count);
  const view = new DataView(
    pcmInt16Uint8Array.buffer,
    pcmInt16Uint8Array.byteOffset,
    pcmInt16Uint8Array.byteLength
  );
  for (let i = 0; i < count; i++) {
    const s = view.getInt16(i * 2, true);
    pcm[i] = s < 0 ? s / 32768 : s / 32767;
  }
  const out = mp3Encoder.encode([pcm]);
  // Copy out of WASM linear memory, which is reused on subsequent calls.
  return out.slice();
};

self.cs2mp3Flush = function (handle) {
  if (!mp3Encoder || handle !== mp3ActiveHandle) {
    return new Uint8Array(0);
  }
  mp3ActiveHandle = 0;
  const out = mp3Encoder.finalize();
  return out.slice();
};

self.cs2mp3Version = function () {
  if (typeof WasmMediaEncoder !== 'undefined' && WasmMediaEncoder.jsLibraryVersion) {
    return WasmMediaEncoder.jsLibraryVersion();
  }
  return 'lame-wasm';
};
