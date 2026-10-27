/* Check the site's real legacy generator, independently of published copies. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const output = process.argv[2];
assert.ok(output, 'Pass the temporary Hugo output directory');
const page = fs.readFileSync(path.join(output, 'playground/index.html'), 'utf8');
assert.ok(page.includes('id="policies"') && page.includes('id="evaluate"'), 'Hugo renders the complete editor');
assert.ok(!/<iframe\b|class="playground-hero\b/i.test(page), 'No recursive iframe or removed marketing hero');
assert.ok(page.includes('https://speedle.io/playground/'), 'Production canonical');
function compareAssets(relative) {
  for (const entry of fs.readdirSync(path.join(root, 'docs', relative), {withFileTypes:true})) {
    const name = path.join(relative, entry.name);
    if (entry.isDirectory()) compareAssets(name);
    else if (name !== 'playground/index.html') {
      assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated asset ${name}`);
    }
  }
}
compareAssets('playground');
for (const name of ['site.css', 'navigation.css', 'custom.css']) {
  assert.deepEqual(fs.readFileSync(path.join(output, 'css', name)), fs.readFileSync(path.join(root, 'docs/css', name)));
}
const sitemap = fs.readFileSync(path.join(output, 'sitemap.xml'), 'utf8');
assert.ok(sitemap.includes('https://speedle.io/en/sitemap.xml'), 'Multilingual sitemap index');
assert.ok(fs.readFileSync(path.join(output, 'en/sitemap.xml'), 'utf8').includes('https://speedle.io/playground/'));
for (const name of ['quick-start/index.html', 'zh/quick-start/index.html']) {
  const target = name.startsWith('zh/') ? '/zh/playground/' : '/playground/';
  assert.ok(fs.readFileSync(path.join(output, name), 'utf8').includes(`href="${target}"`), `Generated ${name} navigation`);
}
for (const name of ['docs/index.html', 'zh/docs/index.html', 'docs/spdl/index.html', 'zh/docs/spdl/index.html']) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  const zh = name.startsWith('zh/');
  const target = '/' + (zh ? name.slice(3) : 'zh/' + name).replace(/index\.html$/, '');
  const navigation = html.match(/<nav id="top-nav-lang"[^>]*>[\s\S]*?<\/nav>/)?.[0];
  assert.ok(navigation, `Language navigation in ${name}`);
  assert.equal((navigation.match(/<a\b/g) || []).length, 1, `One language choice in ${name}`);
  assert.ok(navigation.includes(`href="${target}"`) && navigation.includes(`>${zh ? 'English' : '中文'}</a>`), `Direct counterpart in ${name}`);
  assert.ok(!navigation.includes('hide-sm'), `Same control available in mobile navigation in ${name}`);
  assert.ok(!html.includes('langselFloatDiv') && !html.includes('langNameArrow'), `No dropdown in ${name}`);
}
console.log('PASS Hugo 0.48 output: editor, navigation, sitemap and matching release assets');

for (const locale of ['', 'zh/']) {
  for (const topic of ['', 'rbac/', 'abac/', 'pbac/', 'identity-iam/', 'oes/', 'xacml/']) {
    const name = `${locale}docs/access-control/${topic}index.html`;
    assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated bilingual topic ${name}`);
  }
}
assert.equal(fs.readFileSync(path.join(output, 'llms.txt'), 'utf8'), fs.readFileSync(path.join(root, 'docs/llms.txt'), 'utf8'));
console.log('PASS generated bilingual topic HTML and llms.txt match published copies');
