// Web Worker RPC bridge, browser capability checks, and preview Blob URL cache.
export const FACEIT_TICKET_URL = 'https://support.faceit.com/hc/en-us/requests/new';

export const missingFeatures = [];
if (typeof WebAssembly !== 'object') missingFeatures.push('WebAssembly');
if (typeof Worker !== 'function') missingFeatures.push('Worker');
if (typeof Blob !== 'function') missingFeatures.push('Blob');
export const browserSupported = missingFeatures.length === 0;

const worker = browserSupported ? new Worker('worker.js') : null;
const pendingCalls = new Map();
let nextCallId = 1;
let progressHandler = null;

export function setProgressHandler(fn) {
  progressHandler = typeof fn === 'function' ? fn : null;
}

if (worker) {
  worker.onmessage = ev => {
    const msg = ev.data;
    if (!msg) return;
    if (msg.type === 'progress') {
      const pct = Math.max(0, Math.min(1, Number(msg.value) || 0));
      if (progressHandler) progressHandler(pct);
      return;
    }
    const pending = pendingCalls.get(msg.id);
    if (!pending) return;
    pendingCalls.delete(msg.id);
    if (msg.error) {
      const err = new Error(msg.error);
      if (msg.errorKind) err.kind = msg.errorKind;
      if (msg.errorName) err.name = msg.errorName;
      pending.reject(err);
    } else {
      pending.resolve(msg.result);
    }
  };

  worker.onerror = ev => {
    const err = new Error(ev.message || 'Web worker error');
    for (const pending of pendingCalls.values()) pending.reject(err);
    pendingCalls.clear();
  };
}

export function pickDemoFile() {
  return new Promise(resolve => {
    const inp = document.getElementById('fileinput');
    inp.value = '';
    const cleanup = () => {
      inp.removeEventListener('change', onChange);
      inp.removeEventListener('cancel', onCancel);
    };
    const onChange = () => { cleanup(); resolve(inp.files && inp.files[0] ? inp.files[0] : null); };
    const onCancel = () => { cleanup(); resolve(null); };
    inp.addEventListener('change', onChange, { once: true });
    inp.addEventListener('cancel', onCancel, { once: true });
    inp.click();
  });
}

const previewUrls = new Map();

export function revokePreviewUrl(slot) {
  const key = slot || 'left';
  const old = previewUrls.get(key);
  if (old) {
    URL.revokeObjectURL(old);
    previewUrls.delete(key);
  }
}

export function revokeAllPreviewUrls() {
  for (const url of previewUrls.values()) URL.revokeObjectURL(url);
  previewUrls.clear();
}

function finalizePreview(slot, res) {
  if (!res || !res.view) return res;
  const key = slot || 'left';
  revokePreviewUrl(key);
  const view = res.view;
  if (res.wavBuffer && view.duration > 0) {
    const url = URL.createObjectURL(new Blob([res.wavBuffer], { type: 'audio/wav' }));
    previewUrls.set(key, url);
    view.url = url;
  } else {
    view.url = '';
  }
  return view;
}

export function openFaceitTicket() {
  window.open(FACEIT_TICKET_URL, '_blank', 'noopener');
}

/** Call a worker method; the worker runs the Go WebAssembly backend. */
export function api(name, ...args) {
  if (!worker) {
    const err = new Error(`Missing browser features: ${missingFeatures.join(', ')}`);
    err.kind = 'unsupported';
    err.features = missingFeatures.slice();
    return Promise.reject(err);
  }
  return new Promise((resolve, reject) => {
    const id = nextCallId++;
    pendingCalls.set(id, {
      resolve: res => resolve(name === 'Preview' ? finalizePreview(args[0] && args[0].slot, res) : res),
      reject,
    });
    worker.postMessage({ id, name, args });
  });
}
