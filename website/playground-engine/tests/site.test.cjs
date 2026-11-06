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
assert.equal(read('docs/css/site.css'),originalCss.replace(footerRule,''), 'The shared stylesheet is the actual original homepage CSS, byte for byte, minus the footer rules');
for (const name of ['docs/css/site.css','docs/zh/index.html','docs/172c4fea633285df5f8950cdef354c38/index.html']) assert.ok(!/(?:^|[\s,}])(?:footer|\.footer-[a-z-]+|\.social)(?:[ .:>,][^{\n]*)?\{/m.test(read(name).replace(/<footer[\s\S]*?<\/footer>/g,'')), `No duplicate footer rules in ${name}`);
assert.ok(read('docs/css/navigation.css').includes('html body footer.site-footer {'), 'navigation.css holds the shared footer rules');
const oldCommunityLink = original.match(/<a href="[^"]+" aria-label="Slack">[\s\S]*?<\/a>/)[0];
const discordLink = '<a href="https://discord.gg/uaY2xVsbbT" aria-label="Discord"><img src="/img/speedle/Discord.svg" alt=""></a>';
const originalMailingLink = '<a href="https://groups.google.com/forum/#!forum/speedle-users" aria-label="Mailing list"><img src="/img/speedle/28.%20Mail.svg" alt=""></a>';
const contactLinks = '<a href="mailto:contact@speedle.io" aria-label="Email contact@speedle.io" title="contact@speedle.io"><img src="/img/speedle/28.%20Mail.svg" alt=""></a>\n        <a href="https://groups.google.com/forum/#!forum/speedle-users" aria-label="Google Groups" title="Google Groups — speedle-users"><img src="/img/speedle/Google-Groups.svg" alt=""></a>';
const copyright = '<p>Copyright &copy; 2019–2026. All rights reserved.</p>';
const originalNavigationScript = original.match(/  <script>[\s\S]*?<\/script>/)[0];
const sharedNavigationScript = '  <script src="/js/navigation.js?v=header-20261106-v8" defer></script>';
// #203 three-site lock-up: Speedle mark + wordmark | Engine.
const brandLockup = '<a class="brand" href="#top" aria-label="Speedle Engine home"><img class="brand-logo" src="/img/speedle/Speedle_logo_home_bold.svg?v=4" alt=""><span class="brand-divider" aria-hidden="true"></span><span class="brand-product">Engine</span></a>';
const originalBrand = original.match(/<a class="brand"[^\n]*?<\/a>/)[0];
// Header standard v3/v4: Speedle family menu, footer column, sphere favicons (v12: no hero cross-links).
const productsMenu = /<div class="products">[\s\S]*?<\/div><\/div>\n\s*/;
const familyFooter = /      <div class="footer-family"[\s\S]*?<\/ul><\/div>\n/;
const footerLockup = '<div class="footer-lockup" role="img" aria-label="Speedle Engine"><img src="/img/speedle/Speedle_logo_home_bold.svg?v=4" alt=""><span class="brand-divider"></span><span class="brand-product">Engine</span></div>';
const iconLinks = /<link rel="icon" href="(?:https:\/\/speedle\.io)?\/favicon\.svg" type="image\/svg\+xml">\n\s*<link rel="icon" href="(?:https:\/\/speedle\.io)?\/favicon\.ico" sizes="any">\n\s*<link rel="apple-touch-icon" href="(?:https:\/\/speedle\.io)?\/apple-touch-icon\.png">/;
for (const name of ['docs/index.html','docs/zh/index.html','docs/playground/index.html','docs/zh/playground/index.html']) {
  const html = read(name);
  assert.ok(iconLinks.test(html) && !html.includes('speedle_purple.png'), `Sphere favicons in ${name}`);
  const menu = html.match(productsMenu)?.[0] || '';
  assert.ok(menu.includes('aria-haspopup="true"') && menu.includes('aria-current="page"') && ['https://engine.speedle.io/','https://cloud.speedle.io/','https://ai.speedle.io/'].every(url => menu.includes(`href="${url}`)), `Products menu in ${name}`);
  assert.ok(familyFooter.test(html) && html.includes(footerLockup) && html.includes('<footer class="site-footer">'), `Shared footer in ${name}`);
}
for (const name of ['docs/index.html','docs/zh/index.html']) assert.ok(!read(name).includes('hero-family'), `No contextual cross-links on the Engine home page in ${name} (v12)`);
const originNote = /        <p class="license origin-note"><em>.*<\/em><\/p>\n/;
for (const name of ['docs/index.html','docs/zh/index.html']) {
  const note = read(name).match(originNote)?.[0];
  assert.ok(note && note.includes('href="https://github.com/oracle/speedle" style="text-decoration:underline"><svg') && note.includes('href="mailto:contact@speedle.io" style="text-decoration:underline"'), `Origin note links in ${name}`);
}
const identityTags = [/<title>[^<]*<\/title>/, /<link rel="icon"[^>]*>/,
 /<meta property="og:site_name"[^>]*>/, /<meta property="og:title"[^>]*>/,
 /<meta name="twitter:title"[^>]*>/, /\{"@type":"WebSite"[^\n]+/];
// The site identity (title, icon, social and structured-data names) changed deliberately.
const analyticsSnippet = read('website/cloudflare-web-analytics.html');
const restoredIdentity = identityTags.reduce((html, pattern) => html.replace(pattern, original.match(pattern)[0]), home.replace(analyticsSnippet, '').replace(iconLinks, original.match(/<link rel="icon"[^>]*>/)[0]).replace(productsMenu, '').replace(familyFooter, '').replace('  <footer class="site-footer">\n', '  <footer>\n').replace(footerLockup, '<img src="/img/speedle/Speedle_logo_w.svg" alt="Speedle">'));
const restored = restoredIdentity.replace(policyExample,originalPolicyExample).replace(originNote,'').replace('Bring Your Own Storage','Bring Your Own Store').replace('Speedle Policy Definition Language.','Secure Policy Definition Language.').replace(contactLinks,originalMailingLink).replace(sharedNavigationScript,originalNavigationScript).replace(copyright,'').replace(discordLink,oldCommunityLink).replace('  <link rel="stylesheet" href="/css/site.css">\n  <link rel="stylesheet" href="/css/navigation.css?v=header-20261106-v8">', `  <style>${originalCss}</style>`).replace('<a href="/playground/">Playground</a>','').replace(`        ${chineseLink}\n`,'').replace(brandLockup,originalBrand);
assert.equal(restored,original,'Homepage content is unchanged except shared navigation assets/entries, community/contact links, storage heading, SPDL name, corrected SPDL example, origin note, site identity, copyright notice, analytics beacon, logo lock-up, Speedle family links and favicons');
assert.ok(home.includes(brandLockup), 'Homepage shows the Speedle | Engine lock-up (#203)');
for (const name of ['docs/zh/index.html','docs/playground/index.html','docs/zh/playground/index.html']) assert.ok(read(name).includes('<span class="brand-divider" aria-hidden="true"></span><span class="brand-product">Engine</span></a>'), `Lock-up in ${name}`);
assert.ok(home.includes(chineseLink), 'English homepage exposes the Chinese homepage');
assert.ok(page.includes(playgroundChineseLink), 'Playground switches to its Chinese counterpart');
assert.ok(read('docs/zh/index.html').includes('href="/" lang="en" hreflang="en"'), 'Chinese homepage links back to English');
assert.ok(home.includes('<a href="/playground/">Playground</a>'));
assert.ok(read('docs/zh/index.html').includes('<a href="/zh/playground/">Playground</a>'));
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
for (const name of ['css/site.css','css/navigation.css','css/custom.css','css/docs.css','js/master.js','js/navigation.js','js/docs.js','img/speedle/Discord.svg','img/speedle/Discord-color.svg','img/speedle/Google-Groups.svg','img/speedle/Google-Groups-NOTICE.txt','img/speedle/Material-Icons-LICENSE.txt']) {
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
console.log('PASS static site integration: original CSS/homepage, official footer, navigation, URLs/assets, canonical, English labels, no private hosting or persistent request storage');
