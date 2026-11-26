(() => {
  // opus-decoder.min.js defines OpusDecoderWebWorker extending globalThis.Worker,
  // which is undefined inside a Web Worker. Provide a temporary stub while loading.
  if (!self['opus-decoder']) {
    const hadWorker = typeof self.Worker !== 'undefined';
    if (!hadWorker) self.Worker = function () {};
    try {
      importScripts('vendor/opus-decoder.min.js');
    } finally {
      if (!hadWorker) delete self.Worker;
    }
  }

  const { OpusDecoder } = self['opus-decoder'];
  const POOL_SIZE = 16;
  const OPTS = { sampleRate: 48000, channels: 1 };

  const pool = [];
  const active = new Map();
  const inFlightResets = new Set();
  let nextHandle = 1;

  function makeDecoder() {
    const dec = new OpusDecoder(OPTS);
    return dec.ready.then(() => {
      if (dec.ei && dec.ei.U && dec.Ii) {
        const m = dec.ei.U(dec.ci, Uint8Array);
        m.F.set(dec.Ii);
        dec._mapPtr = m.Y;
      }
      return dec;
    });
  }

  function resetSync(dec) {
    if (!dec || !dec.ei || !dec.ei.I || !dec._mapPtr) return false;
    const I = dec.ei.I;
    if (typeof I.q !== 'function' || typeof I.R !== 'function') return false;
    I.q(dec.ui);
    dec.ui = I.R(dec.yi, dec.ci, dec.li, dec.di, dec._mapPtr, dec.pi, dec.mi);
    dec.hi = 0;
    dec.si = 0;
    dec.Hi = 0;
    return dec.ui !== 0;
  }

  const opusReady = (async () => {
    const decoders = await Promise.all(
      Array.from({ length: POOL_SIZE }, () => makeDecoder())
    );
    pool.push(...decoders);
  })();

  self.opusReady = opusReady;

  self.ensureOpusReady = async function () {
    await opusReady;
    while (inFlightResets.size > 0) {
      await Promise.all([...inFlightResets]);
    }
  };

  self.cs2opusCreate = function () {
    const dec = pool.pop();
    if (!dec) return null;
    const handle = nextHandle++;
    active.set(handle, dec);
    return handle;
  };

  self.cs2opusDecode = function (handle, pkt) {
    const dec = active.get(handle);
    if (!dec || !(pkt instanceof Uint8Array) || pkt.length === 0) return null;
    const res = dec.decodeFrame(pkt);
    if (!res || (res.errors && res.errors.length > 0 && res.samplesDecoded === 0)) {
      return null;
    }
    if (res.samplesDecoded === 0 || !res.channelData || !res.channelData[0]) {
      return new Float32Array(0);
    }
    return res.channelData[0];
  };

  self.cs2opusDestroy = function (handle) {
    const dec = active.get(handle);
    if (!dec) return;
    active.delete(handle);
    if (resetSync(dec)) {
      pool.push(dec);
      return;
    }
    const resetPromise = dec
      .reset()
      .then(
        () => {
          pool.push(dec);
        },
        () => makeDecoder().then((fresh) => {
          pool.push(fresh);
        })
      )
      .finally(() => {
        inFlightResets.delete(resetPromise);
      });
    inFlightResets.add(resetPromise);
  };

  self.cs2opusVersion = function () {
    return 'opus-decoder 0.7.12 (libopus wasm)';
  };
})();
