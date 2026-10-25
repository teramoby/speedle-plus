/* Speedle+ 5cccf4c4e50a2b0ab901a9fa65acc39074ed1068, compiled unmodified to Go WASM. */
import './wasm_exec.js';
let ready;
function initialize() {
  if (ready) return ready;
  ready = (async () => {
    const response = await fetch(new URL('./speedle.wasm', import.meta.url));
    if (!response.ok) throw new Error(`Could not load Speedle engine (HTTP ${response.status}).`);
    const go = new globalThis.Go();
    // ArrayBuffer instantiation also works on static hosts without application/wasm.
    const { instance } = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject);
    go.run(instance).catch(error => self.postMessage({ type: 'fatal', message: error.message || String(error) }));
    if (typeof globalThis.__speedleEvaluateJSON !== 'function') throw new Error('Speedle engine did not initialize.');
    return { engine: 'Speedle+', revision: '5cccf4c4e50a2b0ab901a9fa65acc39074ed1068', runtime: 'Go 1.26.3 WebAssembly' };
  })();
  return ready;
}
self.addEventListener('message', async ({ data }) => {
  const { id, type, input } = data || {};
  try {
    const metadata = await initialize();
    if (type === 'init') { self.postMessage({ id, result: metadata }); return; }
    if (type !== 'evaluate') throw new Error('Unknown engine message.');
    const result = JSON.parse(globalThis.__speedleEvaluateJSON(JSON.stringify(input)));
    self.postMessage({ id, result });
  } catch (error) {
    self.postMessage({ id, error: error.message || String(error) });
  }
});
