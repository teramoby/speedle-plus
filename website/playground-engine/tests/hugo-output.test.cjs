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
assert.ok(page.includes('https://engine.speedle.io/playground/'), 'Production canonical');
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
  assert.ok(html.includes('/css/navigation.css?v=header-20261106-v8') && html.includes('/js/navigation.js?v=header-20261106-v8'), `Shared navigation assets in ${name}`);
}
for (const name of ['site.css', 'navigation.css', 'custom.css', 'docs.css', 'command-copy.css']) {
  assert.deepEqual(fs.readFileSync(path.join(output, 'css', name)), fs.readFileSync(path.join(root, 'docs/css', name)));
}
for (const name of ['command-copy.js', 'docs.js']) {
  assert.deepEqual(fs.readFileSync(path.join(output, 'js', name)), fs.readFileSync(path.join(root, 'docs/js', name)), `Generated ${name}`);
}
const sitemap = fs.readFileSync(path.join(output, 'sitemap.xml'), 'utf8');
assert.ok(sitemap.includes('https://engine.speedle.io/en/sitemap.xml'), 'Multilingual sitemap index');
assert.ok(fs.readFileSync(path.join(output, 'en/sitemap.xml'), 'utf8').includes('https://engine.speedle.io/playground/'));
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
  assert.ok(html.includes('/css/navigation.css?v=header-20261106-v8') && html.includes('/js/navigation.js?v=header-20261106-v8'), `Shared versioned navigation assets in ${name}`);
  assert.ok(html.includes('/css/docs.css') && html.includes('/js/docs.js'), `Documentation assets in ${name}`);
  assert.ok(!/theme\.min|kube\.min|kube\.legenda|master\.css|kube\.demo|custom\.css|spdl-title|fonts\.googleapis|fontawesome|jquery|tocbot|\/js\/kube|\/js\/master/.test(html), `No legacy docs stack in ${name}`);
  assert.ok(html.includes('class="docs-nav-heading') && html.includes('docs-nav-close') && html.includes('docs-nav-backdrop'), `Modern sidebar markup in ${name}`);
  assert.ok(html.includes('aria-current="page"'), `Current page marked in the sidebar in ${name}`);
  assert.ok(new RegExp(`<button class="docs-nav-toggle"[^>]*>[\\s\\S]*?<span>${zh ? '菜单' : 'Menu'}</span>`).test(html), `Menu drawer button in ${name}`);
  if (!zh) assert.ok(/docs-nav-heading[^"]*"><a [^>]*>SDKs<\/a>/.test(html), `SDKs is a bold group heading like API in ${name}`);
  const afterFooter = html.split('</footer>')[1];
  assert.ok(afterFooter && !afterFooter.includes('/js/'), `No layout script after the footer in ${name}`);
}
// #203: every page with the shared footer loads navigation.css, the stylesheet that holds the footer rules.
const footerRules = fs.readFileSync(path.join(output, 'css/navigation.css'), 'utf8');
assert.ok(footerRules.includes('html body footer.site-footer {') && footerRules.includes('.site-footer .footer-inner {'), 'navigation.css holds the shared footer rules');
let footerPages = 0;
(function walk(dir) {
  for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(file);
    else if (entry.name.endsWith('.html')) {
      const html = fs.readFileSync(file, 'utf8');
      if (!html.includes('<footer class="site-footer">')) continue;
      footerPages++;
      assert.ok(html.includes('/css/navigation.css?v=header-20261106-v8'), `Footer stylesheet linked in ${path.relative(output, file)}`);
    }
  }
})(output);
for (const name of ['docs/sdks/index.html', 'zh/docs/sdks/index.html', 'docs/sdks/java/index.html', 'zh/docs/sdks/java/index.html', 'docs/api/index.html', 'docs/spdl/index.html']) {
  assert.ok(fs.readFileSync(path.join(output, name), 'utf8').includes('<footer class="site-footer">'), `Shared footer in ${name}`);
}
assert.ok(footerPages > 200, `Footer pages checked: ${footerPages}`);
// #208: the SDK pages moved from /docs/api/sdk-<lang>/ to /docs/sdks/<lang>/; the old URLs redirect.
for (const locale of ['', 'zh/']) {
  for (const lang of ['java', 'python', 'go', 'dotnet', 'rust', 'node']) {
    const target = `https://engine.speedle.io/${locale}docs/sdks/${lang}/`;
    const alias = fs.readFileSync(path.join(output, `${locale}docs/api/sdk-${lang}/index.html`), 'utf8');
    assert.ok(alias.includes(`<link rel="canonical" href="${target}"`) && alias.includes(`url=/${locale}docs/sdks/${lang}/"`), `Redirect ${locale}docs/api/sdk-${lang}/ to ${target}`);
    const sitemap = fs.readFileSync(path.join(output, `${locale || 'en/'}sitemap.xml`), 'utf8');
    assert.ok(sitemap.includes(`<loc>${target}</loc>`) && !sitemap.includes(`docs/api/sdk-${lang}/`), `Sitemap lists only ${target}`);
  }
}
let docsAlternates = 0;
function checkDocsAlternates(directory) {
  for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) { checkDocsAlternates(file); continue; }
    if (entry.name !== 'index.html') continue;
    const html = fs.readFileSync(file, 'utf8');
    const canonical = html.match(/<link rel="canonical" href="([^"]+)"\/?>/)?.[1];
    assert.ok(canonical, `Docs canonical in ${file}`);
    const english = canonical.replace('/zh/docs/', '/docs/');
    const chinese = english.replace('/docs/', '/zh/docs/');
    for (const [language, target] of [['en', english], ['zh-CN', chinese], ['x-default', english]]) {
      assert.ok(html.includes(`<link rel="alternate" hreflang="${language}" href="${target}">`), `${language} alternate in ${file}`);
    }
    docsAlternates++;
  }
}
checkDocsAlternates(path.join(output, 'docs'));
checkDocsAlternates(path.join(output, 'zh/docs'));
assert.equal(docsAlternates, 80, 'Every canonical docs page and redirect alias has three hreflang alternates');
console.log('PASS Hugo 0.48 output: editor, navigation, sitemap, footer stylesheet and matching release assets');

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
      for (const target of ['store/', 'store/mysql/', 'store/etcd/', 'store/mongodb/', 'pms/policy-mgmt/', 'api/decision_api/',
        'sdks/', 'sdks/java/', 'sdks/python/', 'sdks/go/', 'sdks/dotnet/', 'sdks/rust/', 'sdks/node/']) {
        assert.ok(sidebar.includes(`href="/${locale}docs/${target}"`), `Complete sidebar ${target} in ${name}`);
      }
    }
  }
}
for (const name of ['docs/index.html', 'zh/docs/index.html', 'quick-start/index.html', 'zh/blog/index.html']) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  assert.ok(html.includes('<footer class="site-footer">') && html.includes('class="footer-family"') && !html.includes('<footer class="row">'), `Shared homepage footer in ${name}`);
}
checkStorageNavigation(path.join(output, 'docs'));
checkStorageNavigation(path.join(output, 'zh/docs'), 'zh/');
console.log('PASS complete bilingual documentation sidebars with nested MySQL, etcd and MongoDB guides');
