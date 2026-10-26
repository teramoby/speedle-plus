/** Actual Speedle+ engine, isolated in a terminable worker. No JS eval or mock fallback.
 * await initEngine() -> metadata
 * await evaluate({policies: string, request: object}) ->
 *   {ok:true,allowed:boolean,reason,reasonCode,policies,rolePolicies,roles,statements,durationMs,...}
 *   {ok:false,error:{stage,message,line?},durationMs,...}
 * Initialization, worker failure and timeout reject with Error; they are never false.
 */
let worker = null;
let startup = null;
let sequence = 0;
const pending = new Map();
function shutdown(error) {
  if (worker) worker.terminate();
  worker = null;
  startup = null;
  for (const { reject, timer } of pending.values()) { clearTimeout(timer); reject(error); }
  pending.clear();
}
function send(type, input, timeout) {
  return new Promise((resolve, reject) => {
    const id = ++sequence;
    const timer = setTimeout(() => shutdown(new Error(type === 'init'
      ? 'Speedle engine loading timed out. Check your connection and retry.'
      : 'Evaluation exceeded the 5-second safety limit. Simplify the policies and retry.')), timeout);
    pending.set(id, { resolve, reject, timer });
    try { worker.postMessage({ id, type, input }); }
    catch (error) { clearTimeout(timer); pending.delete(id); reject(error); }
  });
}
export function initEngine() {
  if (startup) return startup;
  startup = (async () => {
    if (typeof Worker === 'undefined' || typeof WebAssembly === 'undefined') throw new Error('This browser needs Web Workers and WebAssembly to run Speedle.');
    worker = new Worker(new URL('./engine-worker.js', import.meta.url), { type: 'module', name: 'speedle-evaluator' });
    worker.addEventListener('message', ({ data }) => {
      if (data?.type === 'fatal') { shutdown(new Error(data.message)); return; }
      const request = pending.get(data?.id);
      if (!request) return;
      clearTimeout(request.timer); pending.delete(data.id);
      if (data.error) request.reject(new Error(data.error)); else request.resolve(data.result);
    });
    worker.addEventListener('error', event => { event.preventDefault(); shutdown(new Error(event.message || 'Speedle worker failed to load. Please retry.')); });
    worker.addEventListener('messageerror', () => shutdown(new Error('Could not read the Speedle engine response.')));
    return send('init', null, 45000);
  })().catch(error => { shutdown(error); throw error; });
  return startup;
}
export async function evaluate(input) {
  if (!input || typeof input.policies !== 'string' || !input.request || typeof input.request !== 'object' || Array.isArray(input.request)) {
    return { ok: false, error: { stage: 'input', message: 'Expected {policies: string, request: object}.' } };
  }
  if (JSON.stringify(input).length > 256 * 1024) return { ok: false, error: { stage: 'input', message: 'Input exceeds the playground limit of 256 KiB.' } };
  await initEngine();
  return send('evaluate', input, 5000);
}
export const evaluateSpeedle = evaluate;
globalThis.evaluateSpeedle = evaluate;
