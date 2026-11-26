importScripts('wasm_exec.js');
importScripts('opus_bridge.js');
importScripts('vendor/WasmMediaEncoder.min.js', 'mp3_bridge.js');

let wasmCrashed = null;
const crashListeners = new Set();

function classifyError(err) {
  const name = (err && err.name) || '';
  const msg = (err && err.message) ? err.message : String(err || 'Unknown error');
  if (
    name === 'RangeError' ||
    name === 'RuntimeError' ||
    (typeof WebAssembly !== 'undefined' && WebAssembly.RuntimeError && err instanceof WebAssembly.RuntimeError) ||
    /out of memory|cannot allocate|memory access out of bounds|allocation failed|runtimeerror|\boom\b/i.test(msg)
  ) {
    return { error: msg, errorName: name, errorKind: 'oom' };
  }
  if (/failed to read demo|invalid file-type|hl2demo|errinvalidfiletype|magic number|corrupt|unexpected eof|zstd/i.test(msg)) {
    return { error: msg, errorName: name, errorKind: 'invalidDemo' };
  }
  return { error: msg, errorName: name, errorKind: '' };
}

function failWasm(err) {
  if (!wasmCrashed) {
    wasmCrashed = err instanceof Error ? err : new Error(String(err || 'WebAssembly runtime exited'));
  }
  for (const cb of crashListeners) cb(wasmCrashed);
  crashListeners.clear();
}

async function ensureCodecs(needMp3) {
  await ensureOpusReady();
  if (needMp3 && typeof mp3Ready !== 'undefined') {
    await mp3Ready;
  }
}

function collectTransfers(res) {
  const transfers = [];
  if (!res) return transfers;
  if (res.zipBytes && res.zipBytes.buffer) {
    transfers.push(res.zipBytes.buffer);
  }
  if (Array.isArray(res.files)) {
    for (const f of res.files) {
      if (f.bytes && f.bytes.buffer) {
        transfers.push(f.bytes.buffer);
      }
    }
  }
  return transfers;
}

const wasmReady = (async () => {
  const go = new Go();
  go.env = { GOGC: '200' };
  let res;
  if (WebAssembly.instantiateStreaming) {
    try {
      res = await WebAssembly.instantiateStreaming(fetch('main.wasm'), go.importObject);
    } catch (err) {
      if (err instanceof RangeError || (WebAssembly.RuntimeError && err instanceof WebAssembly.RuntimeError)) throw err;
      const buf = await (await fetch('main.wasm')).arrayBuffer();
      res = await WebAssembly.instantiate(buf, go.importObject);
    }
  } else {
    const buf = await (await fetch('main.wasm')).arrayBuffer();
    res = await WebAssembly.instantiate(buf, go.importObject);
  }
  go.run(res.instance).then(
    () => failWasm(new Error('WebAssembly runtime exited unexpectedly')),
    failWasm
  );
})();

self.onmessage = async ({ data }) => {
  const { id, name, args = [] } = data;
  try {
    if (wasmCrashed) throw wasmCrashed;
    await wasmReady;
    if (wasmCrashed) throw wasmCrashed;
    switch (name) {
      case 'OpenDemo': {
        const file = args[0];
        let lastProgress = 0;
        const result = await new Promise((resolve, reject) => {
          const onCrash = err => reject(err);
          crashListeners.add(onCrash);
          const settle = fn => val => {
            crashListeners.delete(onCrash);
            fn(val);
          };
          const ok = settle(resolve);
          const fail = settle(reject);
          cs2openDemo(
            file,
            val => {
              const now = performance.now();
              if (now - lastProgress >= 80 || val >= 1) {
                lastProgress = now;
                self.postMessage({ type: 'progress', id, value: val });
              }
            },
            json => {
              try {
                const parsed = JSON.parse(json);
                if (parsed.error) fail(new Error(parsed.error));
                else ok(parsed.data);
              } catch (err) {
                fail(err);
              }
            }
          );
        });
        self.postMessage({ id, result });
        break;
      }

      case 'About': {
        const result = JSON.parse(cs2about());
        self.postMessage({ id, result });
        break;
      }

      case 'Licenses': {
        const resp = await fetch('licenses.txt');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const result = await resp.text();
        self.postMessage({ id, result });
        break;
      }

      case 'Preview': {
        await ensureCodecs(false);
        const res = cs2preview(JSON.stringify(args[0] || {}));
        if (!res || res.error) {
          throw new Error((res && res.error) || 'Preview failed');
        }
        const wav = res.wav;
        const wavBuffer = (wav.byteOffset === 0 && wav.byteLength === wav.buffer.byteLength)
          ? wav.buffer
          : wav.buffer.slice(wav.byteOffset, wav.byteOffset + wav.byteLength);
        self.postMessage({ id, result: { view: res.view, wavBuffer } }, [wavBuffer]);
        break;
      }

      case 'PlanExport': {
        const reqJSON = JSON.stringify(args[0] || {});
        const parsed = JSON.parse(cs2planExport(reqJSON));
        if (parsed.error) throw new Error(parsed.error);
        self.postMessage({ id, result: parsed.data });
        break;
      }

      case 'Export':
      case 'ExportCut': {
        const req = args[0] || {};
        const mode = args[1] === 'files' ? 'files' : 'zip';
        await ensureCodecs((req.format || 'mp3') !== 'wav');
        const fn = name === 'ExportCut' ? cs2exportCut : cs2export;
        const res = fn(JSON.stringify(req), mode);
        if (!res || res.error) {
          throw new Error((res && res.error) || `${name} failed`);
        }
        self.postMessage({ id, result: res }, collectTransfers(res));
        break;
      }

      default:
        self.postMessage({ id, error: `Unknown API method: ${name}` });
    }
  } catch (err) {
    self.postMessage({ id, ...classifyError(err) });
  }
};
