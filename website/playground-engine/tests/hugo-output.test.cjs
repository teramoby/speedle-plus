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
for (const name of ['index.html','zh/index.html','playground/index.html','zh/playground/index.html']) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  assert.ok(html.includes('/css/navigation.css?v=header-20261027') && html.includes('/js/navigation.js?v=header-20261027'), `Shared navigation assets in ${name}`);
}
for (const name of ['site.css', 'navigation.css', 'custom.css', 'command-copy.css']) {
  assert.deepEqual(fs.readFileSync(path.join(output, 'css', name)), fs.readFileSync(path.join(root, 'docs/css', name)));
}
assert.deepEqual(fs.readFileSync(path.join(output, 'js/command-copy.js')), fs.readFileSync(path.join(root, 'docs/js/command-copy.js')), 'Generated command copy script');
const sitemap = fs.readFileSync(path.join(output, 'sitemap.xml'), 'utf8');
assert.ok(sitemap.includes('https://speedle.io/en/sitemap.xml'), 'Multilingual sitemap index');
assert.ok(fs.readFileSync(path.join(output, 'en/sitemap.xml'), 'utf8').includes('https://speedle.io/playground/'));
for (const name of ['quick-start/index.html', 'zh/quick-start/index.html']) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated quick-start guide ${name}`);
  const target = name.startsWith('zh/') ? '/zh/playground/' : '/playground/';
  assert.ok(fs.readFileSync(path.join(output, name), 'utf8').includes(`href="${target}"`), `Generated ${name} navigation`);
}
for (const name of ['docs/index.html', 'zh/docs/index.html', 'docs/spdl/index.html', 'zh/docs/spdl/index.html']) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  const zh = name.startsWith('zh/');
  const target = '/' + (zh ? name.slice(3) : 'zh/' + name).replace(/index\.html$/, '');
  const navigation = html.match(/<nav class="nav-links"[^>]*>[\s\S]*?<\/nav>/)?.[0];
  assert.ok(navigation, `Language navigation in ${name}`);
  assert.equal((navigation.match(/hreflang=/g) || []).length, 1, `One language choice in ${name}`);
  assert.ok(navigation.includes(`href="${target}"`) && navigation.includes(`>${zh ? 'English' : '中文'}</a>`), `Direct counterpart in ${name}`);
  assert.ok(!navigation.includes('hide-sm'), `Same control available in mobile navigation in ${name}`);
  assert.ok(!html.includes('langselFloatDiv') && !html.includes('langNameArrow'), `No dropdown in ${name}`);
  const homepage = fs.readFileSync(path.join(output, zh ? 'zh/index.html' : 'index.html'), 'utf8');
  const labels = value => [...value.matchAll(/<a\b[^>]*>([^<]+)<\/a>/g)].map(m => m[1]);
  const homeNavigation = homepage.match(/<nav class="nav-links"[^>]*>[\s\S]*?<\/nav>/)[0];
  assert.deepEqual(labels(navigation), labels(homeNavigation), `Homepage navigation order/labels in ${name}`);
  assert.ok(navigation.includes(`href="/${zh ? 'zh/' : ''}#features"`), `Features returns to localized homepage from ${name}`);
  assert.ok(html.includes('/css/navigation.css?v=header-20261027') && html.includes('/js/navigation.js?v=header-20261027'), `Shared versioned navigation assets in ${name}`);
}
console.log('PASS Hugo 0.48 output: editor, navigation, sitemap and matching release assets');

// Every documentation sidebar must retain its complete section tree. A shared
// Page.Scratch value can expose an incomplete tree during parallel Hugo renders.
function checkStorageNavigation(directory, locale = '') {
  for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
    const name = path.join(directory, entry.name);
    if (entry.isDirectory()) checkStorageNavigation(name, locale);
    else if (entry.name === 'index.html') {
      const html = fs.readFileSync(name, 'utf8');
      if (!html.includes('<div class="sidebar">')) continue;
      const sidebar = html.split('<div class="sidebar">')[1];
      for (const target of ['store/', 'store/mysql/', 'store/etcd/', 'pms/policy-mgmt/', 'api/decision_api/']) {
        assert.ok(sidebar.includes(`href="/${locale}docs/${target}"`), `Complete sidebar ${target} in ${name}`);
      }
    }
  }
}
checkStorageNavigation(path.join(output, 'docs'));
checkStorageNavigation(path.join(output, 'zh/docs'), 'zh/');
console.log('PASS complete bilingual documentation sidebars with nested MySQL and etcd guides');
