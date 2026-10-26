/* Static integration invariants; real browser coverage is site-smoke.cjs. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {execFileSync} = require('node:child_process');
const root = path.resolve(__dirname, '../../..');
const read = name => fs.readFileSync(path.join(root,name),'utf8');
const home = read('docs/index.html');
const page = read('docs/playground/index.html');
const original = execFileSync('git',['show','5cccf4c4e50a2b0ab901a9fa65acc39074ed1068:docs/index.html'],{cwd:root,encoding:'utf8'});
assert.equal(read('docs/css/site.css'),original.match(/<style>([\s\S]*?)<\/style>/)[1], 'The shared stylesheet is the actual original homepage CSS, byte for byte');
const restored = home.replace('  <link rel="stylesheet" href="/css/site.css">\n  <link rel="stylesheet" href="/css/navigation.css">', `  <style>${read('docs/css/site.css')}</style>`).replace('<a href="/playground/">Playground</a>','');
assert.equal(restored,original,'Homepage content is unchanged except CSS extraction, responsive navigation fix and Playground entry');
assert.ok(home.includes('<a href="/playground/">Playground</a>'));
assert.ok(read('docs/zh/index.html').includes('<a href="/playground/">Playground</a>'));
function compareSource(relative) {
 const source = path.join(root, 'docs/hugo/static', relative);
 const published = path.join(root, 'docs', relative);
 assert.deepEqual(fs.readdirSync(source).sort(), fs.readdirSync(published).sort(), `Source/published file set: ${relative}`);
 for (const entry of fs.readdirSync(source, {withFileTypes:true})) {
  const name = path.join(relative, entry.name);
  if (entry.isDirectory()) compareSource(name);
  else assert.deepEqual(fs.readFileSync(path.join(source,entry.name)), fs.readFileSync(path.join(published,entry.name)), `Source/published bytes: ${name}`);
 }
}
compareSource('playground');
for (const name of ['css/site.css','css/navigation.css','js/master.js']) {
 assert.equal(read('docs/hugo/static/'+name), read('docs/'+name), `Source/published ${name}`);
}
assert.ok(page.includes('<a href="/playground/" aria-current="page">Playground</a>'));
assert.ok(page.includes('<link rel="stylesheet" href="/css/site.css">'));
assert.equal(page.match(/  <footer>[\s\S]*?<\/footer>/)[0],home.match(/  <footer>[\s\S]*?<\/footer>/)[0], 'Official footer is reused unchanged');
assert.ok(page.includes('href="https://speedle.io/playground/"'));
assert.equal(read('docs/CNAME').trim(),'speedle.io');
assert.ok(read('docs/sitemap.xml').includes('https://speedle.io/playground/'));
assert.ok(!/[\u4e00-\u9fff]/.test(page),'English page has no untranslated labels');
for(const source of [page,home]) {
 for(const [,url] of source.matchAll(/(?:src|href)="([^"]+)"/g)) {
  if(/^(?:https?:|data:|#)/.test(url))continue;
  const local=decodeURIComponent(url.split(/[?#]/)[0]);
  const file=path.resolve(root,'docs',local.startsWith('/')?local.slice(1):'playground/'+local);
  assert.ok(file.startsWith(root+path.sep),url);
  assert.ok(fs.existsSync(file),`Missing static linked asset ${url}`);
 }
}
for(const name of ['app.js','request-model.js','engine.js','engine-worker.js']) {
 const script=read('docs/playground/'+name);
 assert.ok(!/localStorage|sessionStorage|indexedDB|sendBeacon|XMLHttpRequest|WebSocket/.test(script),`Unexpected persistence or transport API in ${name}`);
}
assert.ok(!fs.existsSync(path.join(root,'docs/playground/.openai')),'No private hosting configuration');
assert.ok(read('docs/playground/engine-worker.js').includes("fetch(new URL('./speedle.wasm', import.meta.url))"),'WASM URL is module-relative');
console.log('PASS static site integration: original CSS/homepage, official footer, navigation, URLs/assets, canonical, English labels, no private hosting or persistent request storage');
