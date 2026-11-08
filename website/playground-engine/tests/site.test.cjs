/* Static integration invariants; real browser coverage is site-smoke.cjs. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {execFileSync} = require('node:child_process');
const root = path.resolve(__dirname, '../../..');
const read = name => fs.readFileSync(path.join(root,name),'utf8');
const home = read('docs/index.html');
const page = read('docs/playground/index.html');
const chineseLink = '<a href="/zh/" lang="zh-CN" hreflang="zh-CN" aria-label="切换到中文">中文</a>';
const playgroundChineseLink = chineseLink.replace('href="/zh/"','href="/zh/playground/"');
// #198 renamed the product "Speedle+" to "Speedle" (owner decision on legal advice); apply that rename to the reference homepage.
const renameProduct = html => html
  .replace('"name":"Speedle+","alternateName":"Speedle",','"name":"Speedle",')
  .replace('The original Speedle project continues as <strong>Speedle+</strong>, maintained by its founding members.','The original Speedle project continues here, maintained by its founding members.')
  .replace('Speedle+ carries the project forward under the Universal Permissive License.','Speedle continues to move forward under the Universal Permissive License.')
  .replace(/Speedle\+/g,'Speedle')
  // #204 moved the site's own canonical URLs to engine.speedle.io.
  .replace(/https:\/\/speedle\.io\//g,'https://engine.speedle.io/');
const original = renameProduct(execFileSync('git',['show','5cccf4c4e50a2b0ab901a9fa65acc39074ed1068:docs/index.html'],{cwd:root,encoding:'utf8'}));
for (const name of ['docs/index.html','docs/zh/index.html','docs/playground/index.html','docs/zh/playground/index.html']) {
  assert.ok(!/Speedle\+|Speedle&#43;|Speedle Plus/i.test(read(name)), `${name} uses the product name Speedle (#198)`);
}
const originalPolicyExample = original.match(/<pre>[\s\S]*?<\/pre>/)[0];
const policyExample = home.match(/<pre>[\s\S]*?<\/pre>/)[0];
assert.equal(policyExample.replace(/<[^>]+>/g,''), '# Employees can read and borrow books\ngrant group employee read,borrow book\n\n# Deny overrides grant: Bob cannot borrow with an invalid library card\ndeny user bob borrow book if is_card_valid == false', 'Homepage shows the corrected SPDL example');
const originalCss = original.match(/<style>([\s\S]*?)<\/style>/)[1];
// #203: the footer rules moved to navigation.css, the one stylesheet every page (homepages, Playground, Hugo docs, archive) loads.
const footerRule = /^ *(?:footer|\.footer-inner|\.footer-brand (?:img|p)|\.social(?: a| img)?) \{[^\n]*\n/gm;
// #241 added the Get started block and #244 the accessibility/banner block;
// their marked rules are the only additions to the shared stylesheet.
const getStartedCss = /\n    \/\* Get started block \(issue #241\): start\.[\s\S]*?    \/\* Get started block \(issue #241\): end\. \*\/\n/;
const accessibilityCss = /\n    \/\* Accessibility contrast and the community banner \(issue #244\): start\.[\s\S]*?    \/\* Accessibility contrast and the community banner \(issue #244\): end\. \*\/\n/;
assert.equal(read('docs/css/site.css').replace(getStartedCss,'').replace(accessibilityCss,''),originalCss.replace(footerRule,''), 'The shared stylesheet is the actual original homepage CSS, byte for byte, minus the footer rules, the approved Get started block and the approved accessibility/banner block');
for (const name of ['docs/css/site.css','docs/zh/index.html','docs/172c4fea633285df5f8950cdef354c38/index.html']) assert.ok(!/(?:^|[\s,}])(?:footer|\.footer-[a-z-]+|\.social)(?:[ .:>,][^{\n]*)?\{/m.test(read(name).replace(/<footer[\s\S]*?<\/footer>/g,'')), `No duplicate footer rules in ${name}`);
const navigationCss = read('docs/css/navigation.css');
assert.ok(navigationCss.includes('html body footer.site-footer {'), 'navigation.css holds the shared footer rules');
assert.ok(/^\.site-footer \.footer-legal span \{ display: block; \}$/m.test(navigationCss), 'License and copyright stack at every viewport width');
const oldCommunityLink = original.match(/<a href="[^"]+" aria-label="Slack">[\s\S]*?<\/a>/)[0];
const discordLink = '<a href="https://discord.gg/uaY2xVsbbT" aria-label="Discord"><img src="/img/speedle/Discord.svg" alt=""></a>';
const originalMailingLink = '<a href="https://groups.google.com/forum/#!forum/speedle-users" aria-label="Mailing list"><img src="/img/speedle/28.%20Mail.svg" alt=""></a>';
const contactLinks = '<a href="mailto:contact@speedle.io" aria-label="Email contact@speedle.io" title="contact@speedle.io"><img src="/img/speedle/28.%20Mail.svg" alt=""></a>\n        <a href="https://groups.google.com/forum/#!forum/speedle-users" aria-label="Google Groups" title="Google Groups — speedle-users"><img src="/img/speedle/Google-Groups.svg" alt=""></a>';
const copyright = '<p>Copyright &copy; 2019–2026. All rights reserved.</p>';
const footerLegal = zh => zh
  ? '<p class="footer-legal"><span>基于 Universal Permissive License v1.0 授权。</span><span>版权所有 &copy; 2019–2026 The Speedle Authors。</span></p>'
  : '<p class="footer-legal"><span>Licensed under the Universal Permissive License v1.0.</span><span>Copyright &copy; 2019–2026 The Speedle Authors. All rights reserved.</span></p>';
const originalNavigationScript = original.match(/  <script>[\s\S]*?<\/script>/)[0];
const sharedNavigationScript = '  <script src="/js/navigation.js?v=header-20261106-v8" defer></script>';
// #203 three-site lock-up: Speedle mark + wordmark | Engine.
const brandLockup = '<a class="brand" href="#top" aria-label="Speedle Engine home"><img class="brand-logo" src="/img/speedle/Speedle_logo_home_bold.svg?v=4" width="304" height="104" alt=""><span class="brand-divider" aria-hidden="true"></span><span class="brand-product">Engine</span></a>';
const originalBrand = original.match(/<a class="brand"[^\n]*?<\/a>/)[0];
// Header standard v3/v4: Speedle family menu, footer column, sphere favicons (v12: no hero cross-links).
const productsMenu = /<div class="products">[\s\S]*?<\/div><\/div>\n\s*/;
const familyFooter = /      <nav class="footer-family"[^>]*>[\s\S]*?<\/ul>\s*<\/nav>\n/;
const footerLockup = /<a class="footer-lockup" href="\/(?:zh\/)?" aria-label="Speedle Engine (?:home|首页)"><img src="\/img\/speedle\/Speedle_logo_home_bold\.svg\?v=4" width="304" height="104" alt="Speedle"><span class="footer-divider" aria-hidden="true"><\/span><span class="footer-product">Engine<\/span><\/a>/;
const iconLinks = /<link rel="icon" href="(?:https:\/\/speedle\.io)?\/favicon\.svg" type="image\/svg\+xml">\n\s*<link rel="icon" href="(?:https:\/\/speedle\.io)?\/favicon\.ico" sizes="any">\n\s*<link rel="apple-touch-icon" href="(?:https:\/\/speedle\.io)?\/apple-touch-icon\.png">/;
// #217: links between the three sites keep the reader's language.
const crossSiteRoots = zh => zh
  ? ['https://engine.speedle.io/zh/','https://cloud.speedle.io/zh-CN/','https://ai.speedle.io/zh/']
  : ['https://engine.speedle.io/','https://cloud.speedle.io/','https://ai.speedle.io/'];
for (const name of ['docs/index.html','docs/zh/index.html','docs/playground/index.html','docs/zh/playground/index.html']) {
  const html = read(name);
  const roots = crossSiteRoots(name.includes('/zh/'));
  assert.ok(iconLinks.test(html) && !html.includes('speedle_purple.png'), `Sphere favicons in ${name}`);
  const menu = html.match(productsMenu)?.[0] || '';
  assert.ok(menu.includes('aria-haspopup="true"') && menu.includes('aria-current="page"') && roots.every(url => menu.includes(`href="${url}"`)), `Products menu in ${name}`);
  const family = html.match(familyFooter)?.[0] || '';
  assert.ok(roots.every(url => family.includes(`href="${url}"`)), `Footer family language in ${name}`);
  assert.ok(familyFooter.test(html) && footerLockup.test(html) && html.includes('<footer class="site-footer">'), `Shared footer in ${name}`);
}
let chinesePages = 0;
(function walk(dir) {
  for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (file !== path.join(root, 'docs', 'hugo')) walk(file);
      continue;
    }
    if (!entry.name.endsWith('.html')) continue;
    const route = '/' + path.relative(path.join(root, 'docs'), file).split(path.sep).join('/');
    const zh = /^(?:\/[0-9a-f]{32})?\/zh\//.test(route);
    const html = fs.readFileSync(file, 'utf8');
    if (html.includes('<footer class="site-footer">')) {
      assert.ok(html.includes(footerLegal(zh)), `Separate license and approved copyright lines in ${route}`);
      assert.ok(!html.includes('footer-sep'), `No legal-line joiner in ${route}`);
    }
    const blocks = [html.match(/<div class="products-panel"[\s\S]*?<\/div>/)?.[0], html.match(/<nav class="footer-family"[\s\S]*?<\/nav>/)?.[0]].filter(Boolean);
    if (!blocks.length) continue;
    if (zh) chinesePages++;
    for (const block of blocks) {
      for (const [, href] of block.matchAll(/<a\b[^>]*\bhref="(https:\/\/(?:engine|cloud|ai)\.speedle\.io\/[^"]*)"/g)) {
        const pathname = new URL(href).pathname;
        if (zh) assert.ok(/^\/(?:zh|zh-CN)\//.test(pathname), `Chinese page ${route} links the English root ${href}`);
        else assert.ok(!/^\/(?:zh|zh-CN)\//.test(pathname), `English page ${route} links the Chinese root ${href}`);
      }
    }
  }
})(path.join(root, 'docs'));
assert.ok(chinesePages > 100, `Chinese pages scanned for cross-site language: ${chinesePages}`);
for (const name of ['docs/index.html','docs/zh/index.html']) assert.ok(!read(name).includes('hero-family'), `No contextual cross-links on the Engine home page in ${name} (v12)`);
const originNote = /        <p class="license origin-note"><em>.*<\/em><\/p>\n/;
// #250: the home status bar shows the current release and links the release
// notes; the full project origin text lives on /about/, and the home About
// block keeps one sentence linking there.
const statusBar = /    <div class="status">\n      <div class="wrap status-inner">\n        <div class="status-copy"><span class="status-dot"><\/span><span>.*<\/span><\/div>\n        <a class="text-link" href="[^"]+">.*<\/a>\n      <\/div>\n    <\/div>\n/;
for (const [name, release, link] of [
  ['docs/index.html', '<span>v1.6.0 · Go 1.26/1.27 · six SDKs · Playground runs in your browser</span>', '<a class="text-link" href="https://github.com/teramoby/speedle-plus/releases/tag/v1.6.0">Release notes</a>'],
  ['docs/zh/index.html', '<span>v1.6.0 · Go 1.26/1.27 · 六种 SDK · Playground 在浏览器中运行</span>', '<a class="text-link" href="https://github.com/teramoby/speedle-plus/releases/tag/v1.6.0">发行说明</a>'],
]) {
  const bar = read(name).match(statusBar)?.[0];
  assert.ok(bar && bar.includes(release) && bar.includes(link), `Release status bar in ${name}`);
}
const fullOrigin = {
  'docs/index.html': 'Its principal maintainers are the original developers of Oracle Speedle',
  'docs/zh/index.html': '主要维护者正是原 Oracle Speedle 的开发者',
};
for (const [name, about] of [['docs/index.html', '/about/'], ['docs/zh/index.html', '/zh/about/']]) {
  const note = read(name).match(originNote)?.[0];
  assert.ok(note && note.includes(`href="${about}" style="text-decoration:underline"`), `Home origin sentence links to ${about} in ${name}`);
  assert.ok(!read(name).includes(fullOrigin[name]), `Full project origin text is not repeated in ${name}`);
}
for (const [name, sentence, absent] of [
  ['docs/about/index.html', 'Its principal maintainers are the original developers of Oracle Speedle', '30 years'],
  ['docs/zh/about/index.html', '主要维护者正是原 Oracle Speedle 的开发者', '30 年'],
]) {
  const html = read(name);
  assert.ok(html.includes(sentence) && !html.includes(absent) && html.includes('href="https://github.com/oracle/speedle"') && html.includes('href="mailto:contact@speedle.io"'), `Full project origin text on ${name}`);
}
const identityTags = [/<title>[^<]*<\/title>/, /<link rel="icon"[^>]*>/,
 /<meta property="og:site_name"[^>]*>/, /<meta property="og:title"[^>]*>/,
 /<meta name="twitter:card"[^>]*>/, /<meta name="twitter:title"[^>]*>/, /\{"@type":"WebSite"[^\n]+/,
 /\{"@type":"SoftwareSourceCode"[^\n]+/];
// The site identity (title, icon, social and structured-data names) changed deliberately.
// #246 adds the share image on every page; the 2017 reference homepage predates it.
const shareImageTags = /^  <meta (?:property="og:image(?::width|:height|:type|:alt)?"|name="twitter:image(?::alt)?")[^\n]*>\n/gm;
const analyticsSnippet = read('website/cloudflare-web-analytics.html');
// #241 added the Get started block (Playground, SDKs, run locally) to both home pages.
const getStartedSection = /    <section class="getstarted" id="get-started">[\s\S]*?<\/section>\n\n/;
const originalFooter = original.match(/  <footer>[\s\S]*?<\/footer>\n/)[0];
// #244 reworked the community banner: flat soft panel, copy left, action right.
const originalContribute = original.match(/    <section class="contribute">[\s\S]*?<\/section>\n/)[0];
const contributeSection = /    <section class="contribute">[\s\S]*?<\/section>\n/;
// #247 added explicit image dimensions and the pre-paint navigation-js marker.
const imageSizing = html => html
  .replace("  <script>document.documentElement.classList.add('navigation-js');addEventListener('DOMContentLoaded',function(){if(!document.querySelector('header.site-header.navigation-ready'))document.documentElement.classList.remove('navigation-js')});</script>\n", '')
  .replace(' width="330" height="339" fetchpriority="high"', '')
  .replace(/<img src="\/img\/speedle\/Tick\.svg" width="22" height="22"/g, '<img src="/img/speedle/Tick.svg"')
  .replace(/(<img class="feature-icon" src="[^"]+") width="\d+" height="\d+"/g, '$1');
const restoredIdentity = identityTags.reduce((html, pattern) => html.replace(pattern, original.match(pattern)[0]), imageSizing(home).replace(shareImageTags, '').replace(analyticsSnippet, '').replace(iconLinks, original.match(/<link rel="icon"[^>]*>/)[0]).replace(productsMenu, '').replace(/  <footer class="site-footer">[\s\S]*?<\/footer>\n/, originalFooter));
const restored = restoredIdentity.replace(policyExample,originalPolicyExample).replace(originNote,'').replace(statusBar,'').replace('<a href="/docs/pms/policy-mgmt/">Learn more</a>','<a href="/docs/pms/">Learn more</a>').replace('Bring Your Own Storage','Bring Your Own Store').replace('Works with any identity provider: your application verifies identity and passes users, groups and attributes to Speedle for a decision.','Connect GitHub, Okta or your own identity provider.').replace('Speedle Policy Definition Language.','Secure Policy Definition Language.').replace(contactLinks,originalMailingLink).replace(sharedNavigationScript,originalNavigationScript).replace(copyright,'').replace(discordLink,oldCommunityLink).replace('  <link rel="stylesheet" href="/css/site.css">\n  <link rel="stylesheet" href="/css/navigation.css?v=header-20261106-v8">', `  <style>${originalCss}</style>`).replace('<a href="/playground/">Playground</a>','').replace(`        ${chineseLink}\n`,'').replace(brandLockup,originalBrand).replace(getStartedSection,'').replace(contributeSection,originalContribute).replace('<h1>Fast. Simple. Access control</h1>','<h1>Fast. Simple. Access control.</h1>').replace('Authorization that stays out of your application code','Authorization that stays out of your application code.').replace('Fine-grained control without the friction','Fine-grained control without the friction.').replace('Policies humans can read','Policies humans can read.').replace('Built for the infrastructure you already run','Built for the infrastructure you already run.');
// #236: the owner removed the home-page Integrations section; integrations live in the docs only.
// #237: the Cloud Native card now points at the architecture page.
const originalWithoutIntegrations = original
  .replace(/    <section id="integrations">[\s\S]*?<\/section>\n\n/, '')
  .replace('<a href="#integrations">Integrations</a>', '')
  .replace('<a href="/categories/cloudnative/">Learn more</a>', '<a href="/docs/architecture/">Learn more</a>')
  .replace(statusBar,'');
assert.equal(restored,originalWithoutIntegrations,'Homepage content is unchanged except shared navigation assets/entries, community/contact links, storage heading, SPDL name, corrected SPDL example, home feature-card links, the identity-card claim, origin note, release status bar, site identity, ZSP metadata, copyright notice, analytics beacon, logo lock-up, Speedle family links, favicons, titles without trailing full stops, the removed Integrations section, the Cloud Native card target, the added Get started block and the reworked community banner');
// #241: each Get started card reaches Playground, the six language SDK pages and Quick Start,
// and every local link keeps the page language.
for (const [name, prefix] of [['docs/index.html','/'],['docs/zh/index.html','/zh/']]) {
  const section = read(name).match(getStartedSection)?.[0] || '';
  assert.ok(section.includes(`href="${prefix}playground/"`), `Get started Playground link in ${name}`);
  assert.ok(section.includes(`href="${prefix}quick-start/"`), `Get started Quick Start link in ${name}`);
  for (const lang of ['java','python','go','dotnet','rust','node']) assert.ok(section.includes(`href="${prefix}docs/sdks/${lang}/"`), `Get started ${lang} SDK link in ${name}`);
  assert.equal((section.match(/class="sdk-icon"/g)||[]).length,6,`Get started shows six SDK icons in ${name}`);
}
// #221: the home pages carry the scoped ZSP wording in JSON-LD and the terms
// in the keyword list, in each language.
for (const [name, description, keywords] of [
  ['docs/index.html', 'its SPDL conditions and custom functions can support a zero standing privileges (ZSP) pattern with short-lived, scoped credentials, together with token verification and atomic consumption in the application.', '"keywords":["authorization engine","RBAC","ABAC","policy decision service","SPDL","zero standing privileges","ZSP"]'],
  ['docs/zh/index.html', '其 SPDL 条件与自定义函数可以支持零常驻权限（ZSP）模式，使用短期有效、范围受限的凭证，并由应用完成令牌校验与原子消费。', '"keywords":["authorization engine","RBAC","ABAC","policy decision service","SPDL","零常驻权限","ZSP"]'],
]) {
  const html = read(name);
  assert.ok(html.includes(description), `ZSP description in ${name}`);
  assert.ok(html.includes(keywords), `ZSP keywords in ${name}`);
}
assert.ok(home.includes(brandLockup), 'Homepage shows the Speedle | Engine lock-up (#203)');
for (const name of ['docs/zh/index.html','docs/playground/index.html','docs/zh/playground/index.html']) assert.ok(read(name).includes('<span class="brand-divider" aria-hidden="true"></span><span class="brand-product">Engine</span></a>'), `Lock-up in ${name}`);
assert.ok(home.includes(chineseLink), 'English homepage exposes the Chinese homepage');
assert.ok(page.includes(playgroundChineseLink), 'Playground switches to its Chinese counterpart');
assert.ok(read('docs/zh/index.html').includes('href="/" lang="en" hreflang="en"'), 'Chinese homepage links back to English');
assert.ok(home.includes('<a href="/playground/">Playground</a>'));
assert.ok(read('docs/zh/index.html').includes('<a href="/zh/playground/">策略实验室</a>'));
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
compareSource('zh/playground');
for (const name of ['index.html','zh/index.html']) {
 assert.equal(read('docs/hugo/static/'+name), read('docs/'+name), `Authoritative homepage ${name}`);
}
for (const name of ['css/site.css','css/navigation.css','css/docs.css','js/navigation.js','js/docs.js','img/speedle/Discord.svg','img/speedle/Discord-color.svg','img/speedle/Google-Groups.svg','img/speedle/Google-Groups-NOTICE.txt','img/speedle/Material-Icons-LICENSE.txt']) {
 assert.equal(read('docs/hugo/static/'+name), read('docs/'+name), `Source/published ${name}`);
}
assert.ok(page.includes('<a href="/playground/" aria-current="page">Playground</a>'));
assert.ok(page.includes('<link rel="stylesheet" href="/css/site.css">'));
assert.equal(page.match(/  <footer class="site-footer">[\s\S]*?<\/footer>/)[0],home.match(/  <footer class="site-footer">[\s\S]*?<\/footer>/)[0], 'Official footer is reused unchanged');
assert.ok(page.includes('href="https://engine.speedle.io/playground/"'));
assert.equal(read('docs/CNAME').trim(),'engine.speedle.io');
assert.ok(read('docs/sitemap.xml').includes('https://engine.speedle.io/en/sitemap.xml'));
assert.ok(read('docs/sitemap.xml').includes('https://engine.speedle.io/zh/sitemap.xml'));
assert.ok(read('docs/en/sitemap.xml').includes('https://engine.speedle.io/playground/'));
assert.ok(read('docs/zh/sitemap.xml').includes('https://engine.speedle.io/zh/playground/'));
assert.ok(!/[\u4e00-\u9fff]/.test(page.replace(playgroundChineseLink,'')),'English page has no untranslated labels except the Chinese language switch');
for(const source of [page,home]) {
 for(const [,url] of source.matchAll(/(?:src|href)="([^"]+)"/g)) {
  if(/^(?:https?:|mailto:|data:|#)/.test(url))continue;
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
// #218: title-like headings never end with a full stop (DESIGN.md content rule).
function assertNoTrailingFullStop(file) {
 const html = fs.readFileSync(file, 'utf8');
 for (const [, level, inner] of html.matchAll(/<h([1-4])\b[^>]*>([\s\S]*?)<\/h\1>/g)) {
  const text = inner.replace(/<[^>]+>/g, '').trim();
  assert.ok(!/[.。]$/.test(text), `No trailing full stop in h${level} of ${path.relative(root, file)}: ${text}`);
 }
}
function checkDocsHeadings(directory) {
 for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
  const file = path.join(directory, entry.name);
  if (entry.isDirectory()) checkDocsHeadings(file);
  else if (entry.name === 'index.html') assertNoTrailingFullStop(file);
 }
}
for (const name of ['docs/index.html', 'docs/zh/index.html']) assertNoTrailingFullStop(path.join(root, name));
for (const name of ['docs/docs', 'docs/zh/docs']) checkDocsHeadings(path.join(root, name));
// #243: every local stylesheet, script and image in the published tree must
// resolve to a file, so a regeneration cannot leave pages pointing at deleted
// assets. The archived snapshot at /172c4fea…/ is self-contained; it is checked
// too, and a directory reference counts only when it serves an index.html.
const docsRoot = path.join(root, 'docs');
const subresources = [
 [/<link\b[^>]*?\bhref="([^"]*)"/gi, '<link href>'],
 [/<script\b[^>]*?\bsrc="([^"]*)"/gi, '<script src>'],
 [/<img\b[^>]*?\bsrc="([^"]*)"/gi, '<img src>'],
];
let assetPages = 0;
let assetRefs = 0;
(function checkLocalAssets(directory) {
 for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
  const file = path.join(directory, entry.name);
  if (entry.isDirectory()) {
   if (file !== path.join(docsRoot, 'hugo')) checkLocalAssets(file);
   continue;
  }
  if (!entry.name.endsWith('.html')) continue;
  assetPages++;
  const html = fs.readFileSync(file, 'utf8');
  for (const [pattern, label] of subresources) {
   for (const [, raw] of html.matchAll(pattern)) {
    const url = raw.trim();
    if (!url || /^(?:[a-z][a-z0-9+.-]*:|\/\/|#)/i.test(url)) continue;
    const pathname = decodeURIComponent(url.split(/[?#]/)[0]);
    const target = pathname.startsWith('/')
     ? path.join(docsRoot, pathname.slice(1))
     : path.resolve(path.dirname(file), pathname);
    assert.ok(target === docsRoot || target.startsWith(docsRoot + path.sep), `Local ${label} escapes docs/: ${url} in ${path.relative(docsRoot, file)}`);
    assert.ok(fs.existsSync(target) && (fs.statSync(target).isFile() || fs.existsSync(path.join(target, 'index.html'))), `Missing local ${label} ${url} in ${path.relative(docsRoot, file)}`);
    assetRefs++;
   }
  }
 }
})(docsRoot);
assert.ok(assetPages > 250, `Published pages checked for local assets: ${assetPages}`);
assert.ok(assetRefs > 2000, `Local subresource references resolved: ${assetRefs}`);
// #244/#262: keep the home-page palette AA-safe by reading the shipped CSS,
// including its tokens and shorthand hex, rather than testing copied literals.
const homeStyles = read('docs/css/site.css');
const normalizeHex = value => {
 assert.ok(value, 'Missing CSS colour');
 const hex = value.replace('#','');
 assert.ok(/^(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/.test(hex), `Unsupported CSS colour ${value}`);
 return '#' + (hex.length === 3 ? [...hex].map(channel => channel + channel).join('') : hex).toLowerCase();
};
const rootTokens = homeStyles.match(/:root\s*\{([^}]*)\}/)?.[1] || '';
const tokens = new Map([...rootTokens.matchAll(/--([\w-]+)\s*:\s*(#[0-9a-fA-F]{3,6})\s*;/g)].map(match => [match[1],normalizeHex(match[2])]));
const token = name => {
 assert.ok(tokens.has(name), `CSS token --${name} is defined in :root`);
 return tokens.get(name);
};
const escapeRegExp = value => value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
const declaration = (selector,property = 'color') => {
 const pattern = new RegExp(`(?:^|[}\\n])\\s*${escapeRegExp(selector)}\\s*\\{[^}]*?\\b${property}:\\s*(#[0-9a-fA-F]{3,6})\\s*;`,'g');
 const matches = [...homeStyles.matchAll(pattern)];
 assert.ok(matches.length, `${selector} declares ${property} as a hex colour`);
 return normalizeHex(matches.at(-1)[1]);
};
const relativeLuminance = hex => {
 const channels = normalizeHex(hex).replace('#','').match(/../g).map(value => parseInt(value,16)/255)
  .map(value => value <= 0.03928 ? value/12.92 : ((value+0.055)/1.055)**2.4);
 return 0.2126*channels[0] + 0.7152*channels[1] + 0.0722*channels[2];
};
const contrastRatio = (foreground,background) => {
 const [lighter,darker] = [relativeLuminance(foreground),relativeLuminance(background)].sort((a,b)=>b-a);
 return (lighter+0.05)/(darker+0.05);
};
for (const [foreground,background,use] of [
 [token('purple-dark'),token('white'),'eyebrow on white'],
 [token('purple-dark'),token('soft'),'eyebrow on soft'],
 [declaration('.cm'),declaration('.code','background'),'SPDL comment on the code background'],
 [token('muted'),token('soft'),'muted text on soft'],
 [declaration('.button-primary'),token('cyan'),'primary button label'],
]) {
 const ratio = contrastRatio(foreground,background);
 assert.ok(ratio >= 4.5, `WCAG AA contrast for ${use}: ${ratio.toFixed(2)}:1`);
}
const contrastBlock = homeStyles.match(accessibilityCss)?.[0] || '';
assert.ok(homeStyles.includes('.eyebrow { display: inline-flex; align-items: center; gap: 9px; color: var(--cyan);') && contrastBlock.includes('.section-head .eyebrow, .contribute-card .eyebrow { color: var(--purple-dark); }'), 'Light-section eyebrow overrides stay scoped and dark grounds keep the cyan default (#262)');
assert.ok(!/(?:^|\n)\s*\.eyebrow\s*\{\s*color:\s*var\(--purple-dark\)/.test(contrastBlock), 'No global light-surface eyebrow override (#262)');
for (const [name,prefix] of [['docs/index.html','/'],['docs/zh/index.html','/zh/']]) {
 const section = read(name).match(contributeSection)?.[0] || '';
 assert.ok(section.includes('class="eyebrow"') && section.includes(`href="${prefix}contribute/"`), `Community banner structure in ${name} (#244)`);
 assert.ok(!section.includes('<img '), `Community banner has no image column in ${name} (#244)`);
}
// #245: the Playground result stays in view after Evaluate, and its copy and
// labels follow the content rules in either language.
const playgroundPages = [['EN', page], ['ZH', read('docs/zh/playground/index.html')]];
for (const [language, html] of playgroundPages) {
 const idleTitle = html.match(/<div class="result-label" id="result-label">([^<]*)<\/div>/)?.[1];
 assert.ok(idleTitle && !/[.。]$/.test(idleTitle), `Idle result title has no trailing full stop in ${language}`);
 assert.ok(html.includes('<link rel="alternate" hreflang="x-default" href="https://engine.speedle.io/playground/">'), `x-default alternate in the ${language} Playground`);
}
const chinesePlayground = playgroundPages[1][1];
assert.ok(chinesePlayground.includes('<a href="/zh/playground/" aria-current="page">策略实验室</a>'), 'Chinese Playground navigation label matches the page name');
assert.ok(chinesePlayground.includes('<meta name="description" content="在浏览器中使用真实 Speedle 引擎编写 SPDL 策略并测试授权请求。">'), 'Chinese Playground meta description is translated');
assert.ok(chinesePlayground.includes('<meta property="og:title" content="策略实验室 · Speedle Engine">') && chinesePlayground.includes('<meta property="og:description" content="在浏览器中使用真实 Speedle 引擎编写 SPDL 策略并测试授权请求。">'), 'Chinese Playground social metadata is translated');
// #246: a 1200x630 share image per language, TechArticle/BreadcrumbList on the
// docs pages, one title pattern per page and one lang value per language.
const pngDimensions = file => {
 const png = fs.readFileSync(file);
 assert.equal(png.subarray(0,8).toString('hex'), '89504e470d0a1a0a', `${file} is a PNG`);
 return [png.readUInt32BE(16), png.readUInt32BE(20)];
};
for (const name of ['engine-en.png','engine-zh.png']) {
 assert.deepEqual(pngDimensions(path.join(docsRoot, 'img/social', name)), [1200,630], `Share image ${name} is 1200x630`);
}
// #269: shared v13 sphere favicon — 115-unit viewBox, #ff9a45 only, no dark
// outline, outer polygon x2.0 (7.2) and inner lines x2.2 (5.28), matching the
// header mark's paths, plus 16/32/48 PNG ICO frames and 180x180 apple-touch-icon.
for (const name of ['favicon.svg','favicon.ico','apple-touch-icon.png']) {
 assert.deepEqual(fs.readFileSync(path.join(root, 'docs/hugo/static', name)), fs.readFileSync(path.join(docsRoot, name)), `Source/published ${name}`);
}
const faviconSvg = read('docs/favicon.svg');
const headerLogoSvg = read('docs/img/speedle/Speedle_logo_home_bold.svg');
assert.ok(faviconSvg.includes('viewBox="21.8 22 115 115"'), 'Favicon uses the shared 115-unit sphere viewBox');
assert.deepEqual([...faviconSvg.matchAll(/\bstroke="(#[0-9a-fA-F]+)"/g)].map(m => m[1].toLowerCase()), ['#ff9a45'], 'Favicon strokes use only Engine orange (#ff9a45) with no dark outline');
assert.ok(!/<circle\b|#1f1142/i.test(faviconSvg), 'Favicon has no v9 outline ring or dark underlay');
assert.deepEqual([...faviconSvg.matchAll(/\bstroke-width="([^"]+)"/g)].map(m => m[1]), ['7.2', '5.28'], 'Favicon scales outer polygon x2.0 and inner lines x2.2');
const sphereGeometry = svg => [...svg.matchAll(/<(polygon|path)\b(?![^>]*\bclass="cls-2")[^>]*\b(?:points|d)="([^"]+)"/g)].map(m => `${m[1]}:${m[2]}`);
assert.deepEqual(sphereGeometry(faviconSvg), sphereGeometry(headerLogoSvg), 'Favicon sphere geometry matches the header mark without path edits');
const faviconIco = fs.readFileSync(path.join(docsRoot, 'favicon.ico'));
assert.equal(faviconIco.readUInt16LE(0), 0);
assert.equal(faviconIco.readUInt16LE(2), 1);
assert.equal(faviconIco.readUInt16LE(4), 3, 'favicon.ico packs 3 frames');
for (const [index, size] of [16, 32, 48].entries()) {
 const entry = 6 + index * 16;
 const frameLength = faviconIco.readUInt32LE(entry + 8);
 const frameOffset = faviconIco.readUInt32LE(entry + 12);
 const frame = faviconIco.subarray(frameOffset, frameOffset + frameLength);
 assert.equal(faviconIco[entry], size, `ICO entry width ${size}`);
 assert.equal(faviconIco[entry + 1], size, `ICO entry height ${size}`);
 assert.equal(faviconIco.readUInt16LE(entry + 6), 32, `ICO entry 32bpp at ${size}`);
 assert.equal(frame.subarray(0, 8).toString('hex'), '89504e470d0a1a0a', `PNG payload at ${size}`);
 assert.deepEqual([frame.readUInt32BE(16), frame.readUInt32BE(20), frame[24], frame[25]], [size, size, 8, 6], `RGBA PNG IHDR at ${size}`);
}
assert.deepEqual(pngDimensions(path.join(docsRoot, 'apple-touch-icon.png')), [180, 180], 'apple-touch-icon.png is 180x180');
assert.ok(!fs.existsSync(path.join(docsRoot, 'public/speedle/docs/img/favicon.ico')), 'Stale unreferenced mkdocs favicon removed');
const titleOf = html => html.match(/<title>([\s\S]*?)<\/title>/)?.[1];
const contentOf = (html, pattern) => html.match(pattern)?.[1];
let identityPages = 0;
const homes = {'/': 'Speedle Engine — Fast. Simple. Access control', '/zh/': 'Speedle Engine — 快速、简单的权限控制'};
(function checkPageIdentity(directory) {
 for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
  const file = path.join(directory, entry.name);
  if (entry.isDirectory()) {
   if (file !== path.join(docsRoot, 'hugo')) checkPageIdentity(file);
   continue;
  }
  if (!entry.name.endsWith('.html')) continue;
  const route = '/' + path.relative(docsRoot, file).split(path.sep).join('/').replace(/index\.html$/, '');
  if (/^\/[0-9a-f]{32}\//.test(route)) continue; // the 2017 archive keeps its own metadata
  const html = fs.readFileSync(file, 'utf8');
  const zh = route === '/zh/' || route.startsWith('/zh/');
  assert.ok(html.includes(`<html lang="${zh ? 'zh-CN' : 'en'}">`), `One lang value per language: ${route}`);
  if (/http-equiv="refresh"/i.test(html)) continue;
  identityPages++;
  const title = titleOf(html);
  assert.ok(title, `Page title in ${route}`);
  assert.ok(!/[.。]$/.test(title), `No trailing full stop in the title of ${route}`);
  if (zh) {
   const pageTitle = title.replace(/ · Speedle Engine$/, '');
   // Product names, protocol names and version numbers stay as they are; everything else is localized.
   const technicalTitle = new Set(['API', 'SDK', 'PMS', 'PDP', 'SPDL', 'TLS', 'MySQL', 'MongoDB', 'v0.1.0', 'Speedle Cloud',
    'C#/.NET SDK', 'Go SDK', 'Java SDK', 'Node.js/TypeScript SDK', 'Python SDK', 'Rust SDK']).has(pageTitle);
   assert.ok(/[一-鿿]/.test(pageTitle) || technicalTitle, `Chinese page has a localized title in ${route}: ${pageTitle}`);
   const description = contentOf(html, /<meta name="description" content="([^"]*)">/);
   if (description) assert.ok(/[一-鿿]/.test(description), `Chinese page has a localized meta description in ${route}`);
  }
  assert.equal(contentOf(html, /<meta property="og:title" content="([^"]*)">/), title, `og:title follows title in ${route}`);
  const canonical = contentOf(html, /<link rel="canonical" href="([^"]+)">/);
  assert.ok(canonical, `Canonical in ${route}`);
  assert.equal(contentOf(html, /<meta property="og:url" content="([^"]+)">/), canonical, `og:url follows canonical in ${route}`);
  const target = new URL(canonical);
  const enPath = zh ? (target.pathname.replace(/^\/zh/, '') || '/') : target.pathname;
  const zhPath = zh ? target.pathname : '/zh' + (target.pathname === '/' ? '/' : target.pathname);
  for (const [language, href] of [['en', target.origin + enPath], ['zh-CN', target.origin + zhPath], ['x-default', target.origin + enPath]]) {
   assert.ok(html.includes(`<link rel="alternate" hreflang="${language}" href="${href}">`), `hreflang ${language} in ${route}`);
  }
  const image = `https://engine.speedle.io/img/social/engine-${zh ? 'zh' : 'en'}.png`;
  for (const tag of [`<meta property="og:image" content="${image}">`, `<meta name="twitter:image" content="${image}">`,
   '<meta name="twitter:card" content="summary_large_image">']) assert.ok(html.includes(tag), `${tag} in ${route}`);
  assert.ok(contentOf(html, /<meta property="og:image:alt" content="([^"]*)">/) && contentOf(html, /<meta name="twitter:image:alt" content="([^"]*)">/), `Share image alt text in ${route}`);
  if (route in homes) assert.equal(title, homes[route], `Home title follows the naming pattern in ${route}`);
  else assert.ok(title.endsWith(' · Speedle Engine'), `Inner title follows "<Page> · Speedle Engine" in ${route}: ${title}`);
 }
})(docsRoot);
assert.ok(identityPages > 190, `Pages checked for title and social metadata: ${identityPages}`);
// #246: pages without an explicit description still unfurl with their summary.
for (const name of ['docs/about/index.html','docs/zh/about/index.html']) {
 const html = read(name);
 assert.ok(contentOf(html, /<meta name="description" content="([^"]+)">/) && contentOf(html, /<meta property="og:description" content="([^"]+)">/) && contentOf(html, /<meta name="twitter:description" content="([^"]+)">/), `Summary fallback description in ${name}`);
}
// #262: all regular pages, not only /docs/, retain Hugo's Open Graph article
// typing and whichever publication/modification timestamps their content has.
for (const pageName of ['about','contribute','quick-start','support']) {
 for (const locale of ['', 'zh/']) {
  const name = `docs/${locale}${pageName}/index.html`;
  const html = read(name);
  assert.equal(contentOf(html, /<meta property="og:type" content="([^"]+)">/), 'article', `Article type on non-docs page ${name}`);
  const modified = contentOf(html, /<meta property="article:modified_time" content="([^"]+)">/);
  assert.ok(modified && !Number.isNaN(Date.parse(modified.replace(/&#43;/g,'+'))), `Valid article modification timestamp in ${name}`);
 }
}
for (const name of ['docs/docs/assertor/index.html','docs/zh/docs/assertor/index.html']) {
 const html = read(name);
 for (const property of ['article:published_time','article:modified_time']) {
  const timestamp = contentOf(html, new RegExp(`<meta property="${property}" content="([^"]+)">`));
  assert.ok(timestamp && !Number.isNaN(Date.parse(timestamp.replace(/&#43;/g,'+'))), `Valid ${property} in ${name}`);
 }
}
for (const name of ['docs/integrations/index.html','docs/zh/integrations/index.html','docs/docs/index.html','docs/zh/docs/index.html']) {
 const html = read(name);
 assert.equal(contentOf(html, /<meta property="og:type" content="([^"]+)">/), 'website', `Section/list page remains a website in ${name}`);
 assert.ok(!html.includes('property="article:published_time"') && !html.includes('property="article:modified_time"'), `No article timestamps on list page ${name}`);
}
let structuredDocs = 0;
function checkDocsStructuredData(directory, zh) {
 for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
  const file = path.join(directory, entry.name);
  if (entry.isDirectory()) { checkDocsStructuredData(file, zh); continue; }
  if (entry.name !== 'index.html') continue;
  const html = fs.readFileSync(file, 'utf8');
  if (/http-equiv="refresh"/i.test(html)) continue;
  const canonical = contentOf(html, /<link rel="canonical" href="([^"]+)">/);
  const blocks = [...html.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)].map(match => JSON.parse(match[1]));
  assert.ok(canonical && blocks.length >= 2, `Docs canonical and JSON-LD in ${path.relative(docsRoot, file)}`);
  const nodes = blocks.flatMap(block => block['@graph'] || [block]);
  const article = nodes.find(node => node['@type'] === 'TechArticle' || node['@type'] === 'WebPage');
  assert.ok(article && article.inLanguage === (zh ? 'zh-CN' : 'en'), `Article node and language in ${canonical}`);
  assert.ok(article['@id'].endsWith(article['@type'] === 'TechArticle' ? '#article' : '#webpage'), `Article id in ${canonical}`);
  assert.ok(article.image === `https://engine.speedle.io/img/social/engine-${zh ? 'zh' : 'en'}.png` && article.author?.name === 'Speedle', `Article image and author in ${canonical}`);
  assert.ok(article.dateModified && article.publisher?.logo?.width === 180, `Article modification date and publisher logo in ${canonical}`);
  if (article.datePublished) assert.ok(Date.parse(article.datePublished) <= Date.parse(article.dateModified), `datePublished is not after dateModified in ${canonical}`);
  const breadcrumb = nodes.find(node => node['@type'] === 'BreadcrumbList');
  assert.ok(breadcrumb && breadcrumb.itemListElement.length >= 2, `BreadcrumbList in ${canonical}`);
  assert.equal(breadcrumb.itemListElement[0].name, zh ? '首页' : 'Home', `Breadcrumb starts at the home page in ${canonical}`);
  assert.deepEqual(breadcrumb.itemListElement.map(item => item.position), breadcrumb.itemListElement.map((_, index) => index + 1), `Breadcrumb positions in ${canonical}`);
  assert.equal(breadcrumb.itemListElement.at(-1).item, canonical, `Breadcrumb ends at the page in ${canonical}`);
  structuredDocs++;
 }
}
checkDocsStructuredData(path.join(docsRoot, 'docs'), false);
checkDocsStructuredData(path.join(docsRoot, 'zh/docs'), true);
assert.ok(structuredDocs > 60, `Docs pages with structured data: ${structuredDocs}`);
console.log(`PASS static site integration: original CSS/homepage, official footer, navigation, URLs/assets, canonical, English labels, no private hosting or persistent request storage, ${assetRefs} local assets in ${assetPages} published pages, WCAG AA home-page palette, ${identityPages} pages with one title pattern and lang, ${structuredDocs} docs pages with structured data`);
