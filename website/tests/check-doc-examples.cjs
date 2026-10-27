/* Evaluate the actual Markdown examples against the published Go/WASM engine. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../..');
globalThis.crypto ||= require('node:crypto').webcrypto;
require(path.join(root, 'docs/playground/wasm_exec.js'));

(async () => {
  const go = new Go();
  const {instance} = await WebAssembly.instantiate(fs.readFileSync(path.join(root, 'docs/playground/speedle.wasm')), go.importObject);
  void go.run(instance);
  const base = {subject:{principals:[{type:'user',name:'alice'}]},resource:'book',action:'read'};
  let checked = 0;
  function evaluate(policies, request, expected) {
    const result = JSON.parse(globalThis.__speedleEvaluateJSON(JSON.stringify({policies,request})));
    assert.equal(result.ok, true, JSON.stringify(result));
    assert.equal(result.allowed, expected, JSON.stringify(result));
    checked++;
  }
  for (const locale of ['', '.zh']) {
    for (const name of ['_index', 'rbac', 'abac', 'pbac', 'identity-iam', 'xacml']) {
      const markdown = fs.readFileSync(path.join(root, 'docs/hugo/content/docs/access-control', name + locale + '.md'), 'utf8');
      const policies = [...markdown.matchAll(/```text\n([\s\S]*?)\n```/g)].map(m => m[1]);
      const json = markdown.match(/```json\n([\s\S]*?)\n```/);
      const request = json ? JSON.parse(json[1]) : structuredClone(base);
      if (name === 'xacml') request.attributes = [{name:'clearance',type:'numeric',value:4}];
      for (const policy of policies) evaluate(policy, request, name !== 'pbac');
      if (name === '_index') {
        evaluate(policies[0], {...base, action:'delete'}, false);
        evaluate(policies[0], {...base, action:'write'}, false);
      }
      if (name === 'rbac') {
        evaluate(policies[0], {...base, subject:{principals:[{type:'user',name:'bob'}]}}, false);
        evaluate(policies[0], {...base, action:'delete'}, false);
        evaluate(policies[0], {...base, resource:'document'}, false);
      }
      if (name === 'abac' || name === 'xacml') {
        evaluate(policies[0], {...request,attributes:[{name:'clearance',type:'numeric',value:2}]}, false);
        const missing = JSON.parse(globalThis.__speedleEvaluateJSON(JSON.stringify({policies:policies[0],request:{...base}})));
        assert.equal(missing.ok, false, 'Missing attribute is an error, not a grant');
        checked++;
      }
      if (name === 'pbac') evaluate(policies[0].split('\n')[0], request, true);
      if (name === 'identity-iam') evaluate(policies[0], {...request,subject:{principals:[{type:'user',name:'alice',idd:'partner'}]}}, false);
    }
  }
  console.log(`PASS ${checked} bilingual documentation examples and negative cases against the real Go/WASM engine`);
  process.exit(0);
})().catch(error => {console.error(error);process.exit(1);});
