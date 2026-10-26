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
console.log('PASS Hugo 0.48 output: editor, navigation, sitemap and matching release assets');
