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
for (const name of ['site.css', 'navigation.css', 'docs.css', 'command-copy.css']) {
  assert.deepEqual(fs.readFileSync(path.join(output, 'css', name)), fs.readFileSync(path.join(root, 'docs/css', name)));
}
for (const name of ['favicon.svg', 'favicon.ico', 'apple-touch-icon.png']) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated ${name}`);
}
const docsStyles = fs.readFileSync(path.join(output, 'css/docs.css'), 'utf8');
const sidebarRules = docsStyles.match(/\.docs-sidebar\s*\{[^}]*\}/)?.[0] || '';
assert.ok(sidebarRules.includes('position: static') && sidebarRules.includes('max-height: none') && sidebarRules.includes('overflow-y: visible'), 'Desktop sidebar stays fully expanded in normal page flow');
assert.ok(docsStyles.includes('.sidebar li[data-docs-depth="1"] > a'), 'Child links receive depth-based indentation');
for (const name of ['command-copy.js', 'docs.js']) {
  assert.deepEqual(fs.readFileSync(path.join(output, 'js', name)), fs.readFileSync(path.join(root, 'docs/js', name)), `Generated ${name}`);
}
const sitemap = fs.readFileSync(path.join(output, 'sitemap.xml'), 'utf8');
assert.ok(sitemap.includes('https://engine.speedle.io/en/sitemap.xml'), 'Multilingual sitemap index');
assert.ok(fs.readFileSync(path.join(output, 'en/sitemap.xml'), 'utf8').includes('https://engine.speedle.io/playground/'));
for (const name of ['quick-start/index.html', 'zh/quick-start/index.html']) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated quick-start guide ${name}`);
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  const target = name.startsWith('zh/') ? '/zh/playground/' : '/playground/';
  assert.ok(html.includes(`href="${target}"`), `Generated ${name} navigation`);
  assert.ok(html.includes('/css/command-copy.css'), `Named custom stylesheet in ${name}`);
}
// #262: customcss without a filename is a build error, never a silent fallback.
for (const name of ['_default/baseof.html', 'docs/baseof.html']) {
  const template = fs.readFileSync(path.join(root, 'docs/hugo/layouts', name), 'utf8');
  assert.ok(/\{\{\s*else\s*\}\}\s*\{\{\s*errorf "customcss is true but customcssfile is missing on %s" \.Path\s*\}\}/.test(template), `Missing customcssfile fails the build in ${name}`);
}
for (const name of ['about/index.html', 'zh/about/index.html', 'contribute/index.html', 'zh/contribute/index.html',
  'support/index.html', 'zh/support/index.html', 'docs/assertor/index.html', 'zh/docs/assertor/index.html']) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated article metadata ${name}`);
}
for (const locale of ['', 'zh/']) {
  const html = fs.readFileSync(path.join(output, `${locale}docs/api/management_api/index.html`), 'utf8');
  assert.ok(html.includes('/css/swagger-ui.css') && html.includes('/css/swagger-custom.css'), `Swagger styles in ${locale || 'en'}`);
}
for (const name of ['docs/index.html', 'zh/docs/index.html', 'docs/spdl/index.html', 'zh/docs/spdl/index.html',
  'docs/cloud/index.html', 'zh/docs/cloud/index.html', 'docs/cloud/getting-started/index.html', 'zh/docs/cloud/getting-started/index.html']) {
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
// #243: the remaining legacy-theme pages use the docs styles and load no Kube stack.
for (const name of ['quick-start/index.html', 'zh/quick-start/index.html', 'about/index.html', 'zh/about/index.html',
  'contribute/index.html', 'zh/contribute/index.html', 'support/index.html', 'zh/support/index.html',
  'integrations/index.html', 'zh/integrations/index.html', 'versions/index.html', 'zh/versions/index.html',
  'tags/index.html', 'zh/tags/index.html', 'categories/index.html', 'zh/categories/index.html',
  'tags/api/index.html', 'zh/tags/api/index.html', 'categories/docs/page/2/index.html']) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  assert.ok(html.includes('/css/docs.css'), `Docs styles in ${name}`);
  assert.ok(!/theme\.min|kube\.min|kube\.legenda|master\.css|kube\.demo|custom\.css|spdl-title|fonts\.googleapis|fontawesome|jquery|tocbot|\/js\/kube|\/js\/master/.test(html), `No legacy stack in ${name}`);
  assert.ok(html.includes('<body class="page-kube docs-page">'), `Docs body in ${name}`);
  assert.ok(html.includes('class="content-container docs-article"'), `Stand-alone docs container in ${name}`);
}
// #203: every page with the shared footer loads navigation.css, the stylesheet that holds the footer rules.
const footerRules = fs.readFileSync(path.join(output, 'css/navigation.css'), 'utf8');
assert.ok(footerRules.includes('html body footer.site-footer {') && footerRules.includes('.site-footer .footer-main {'), 'navigation.css holds the shared footer rules');
assert.ok(/^\.site-footer \.footer-legal span \{ display: block; \}$/m.test(footerRules), 'License and copyright stack at every viewport width');
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
      const relative = path.relative(output, file);
      const zh = relative.split(path.sep).includes('zh');
      const legal = zh
        ? '<p class="footer-legal"><span>基于 Universal Permissive License v1.0 授权。</span><span>版权所有 &copy; 2019–2026 The Speedle Authors。</span></p>'
        : '<p class="footer-legal"><span>Licensed under the Universal Permissive License v1.0.</span><span>Copyright &copy; 2019–2026 The Speedle Authors. All rights reserved.</span></p>';
      assert.ok(html.includes(legal), `Separate license and approved copyright lines in ${relative}`);
      assert.ok(!html.includes('footer-sep'), `No legal-line joiner in ${relative}`);
    }
  }
})(output);
for (const name of ['docs/sdks/index.html', 'zh/docs/sdks/index.html', 'docs/sdks/java/index.html', 'zh/docs/sdks/java/index.html', 'docs/api/index.html', 'docs/spdl/index.html']) {
  assert.ok(fs.readFileSync(path.join(output, name), 'utf8').includes('<footer class="site-footer">'), `Shared footer in ${name}`);
}
assert.ok(footerPages > 190, `Footer pages checked: ${footerPages}`);
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
assert.equal(docsAlternates, 84, 'Every canonical docs page and redirect alias has three hreflang alternates');
console.log('PASS Hugo 0.48 output: editor, navigation, sitemap, footer stylesheet and matching release assets');

// #219: the Cloud overview is a linked, bold sidebar group and both languages
// carry the same six-step launch preview, limits, legal links and metadata.
for (const locale of ['', 'zh/']) {
  const zh = locale === 'zh/';
  const prefix = `https://engine.speedle.io/${locale}docs/cloud/`;
  const overview = fs.readFileSync(path.join(output, `${locale}docs/cloud/index.html`), 'utf8');
  const guide = fs.readFileSync(path.join(output, `${locale}docs/cloud/getting-started/index.html`), 'utf8');
  const sidebar = guide.split('<div class="sidebar">')[1];
  assert.ok(/docs-nav-heading[^"]*"><a [^>]*>Speedle Cloud<\/a>/.test(sidebar), `Cloud is a linked bold sidebar heading in ${locale || 'en'}`);
  assert.ok(sidebar.includes(`href="/${locale}docs/cloud/getting-started/"`), `Cloud getting-started link in ${locale || 'en'}`);
  assert.ok(overview.includes(zh ? 'Speedle Cloud 即将上线' : 'Speedle Cloud is coming soon'), `Coming-soon overview in ${locale || 'en'}`);
  assert.equal((guide.match(zh ? /<strong>截图待补充<\/strong>/g : /<strong>Screenshot coming soon<\/strong>/g) || []).length, 6, `Six screenshot slots in ${locale || 'en'}`);
  assert.ok(guide.includes('POST /authz-check/v1/is-allowed') && guide.includes('spdl_&lt;canonical lowercase UUIDv7 id&gt;_&lt;43 base62 digits&gt;'), `Cloud API and token format in ${locale || 'en'}`);
  assert.ok(guide.includes(`href="https://cloud.speedle.io/${zh ? 'zh-CN/' : ''}terms"`) && guide.includes(`href="https://cloud.speedle.io/${zh ? 'zh-CN/' : ''}privacy"`), `Localized Cloud legal links in ${locale || 'en'}`);
  for (const route of ['', 'getting-started/']) {
    assert.ok(fs.readFileSync(path.join(output, `${locale || 'en/'}sitemap.xml`), 'utf8').includes(`<loc>${prefix}${route}</loc>`), `Cloud sitemap entry ${prefix}${route}`);
  }
}
assert.ok(fs.readFileSync(path.join(output, 'llms.txt'), 'utf8').includes('[Speedle Cloud](https://engine.speedle.io/docs/cloud/)'), 'llms.txt links the Cloud section');

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
        'sdks/', 'sdks/java/', 'sdks/python/', 'sdks/go/', 'sdks/dotnet/', 'sdks/rust/', 'sdks/node/',
        'cloud/', 'cloud/getting-started/']) {
        assert.ok(sidebar.includes(`href="/${locale}docs/${target}"`), `Complete sidebar ${target} in ${name}`);
      }
      for (const [depth, target] of [[0, 'security/'], [1, 'pms/policy-mgmt/'], [1, 'store/mysql/'],
        [1, 'api/decision_api/'], [1, 'sdks/java/'], [1, 'cloud/getting-started/']]) {
        assert.ok(new RegExp(`<li data-docs-depth="${depth}"[^>]*><a href="/${locale}docs/${target}"`).test(sidebar), `Menu depth ${depth} for ${target} in ${name}`);
      }
    }
  }
}
for (const name of ['docs/index.html', 'zh/docs/index.html', 'quick-start/index.html', 'zh/about/index.html']) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  assert.ok(html.includes('<footer class="site-footer">') && html.includes('class="footer-family"') && !html.includes('<footer class="row">'), `Shared homepage footer in ${name}`);
}
checkStorageNavigation(path.join(output, 'docs'));
checkStorageNavigation(path.join(output, 'zh/docs'), 'zh/');
console.log('PASS complete bilingual documentation sidebars with nested MySQL, etcd and MongoDB guides');
