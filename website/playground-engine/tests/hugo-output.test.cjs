/* Check the site's real legacy generator, independently of published copies. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const output = process.argv[2];
assert.ok(output, 'Pass the temporary Hugo output directory');
const LOCALES = ['', 'ja/', 'ko/', 'es/', 'zh/'];
const LANG_ORDER = [['en', 'English'], ['ja', '日本語'], ['ko', '한국어'], ['es', 'Español'], ['zh-CN', '中文']];
const MENU_LABEL = {'': 'Menu', ja: 'メニュー', ko: '메뉴', es: 'Menú', zh: '菜单'};
const SDK_LABEL = {'': 'SDKs', ja: 'SDK', ko: 'SDK', es: 'SDK', zh: 'SDK'};
const DEPLOY_LABEL = {'': 'Deployment', ja: 'デプロイ', ko: '배포', es: 'Despliegue', zh: '部署'};
require('./performance-guide.cjs')(output);
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
for (const name of LOCALES.flatMap(locale => [`${locale}index.html`, `${locale}playground/index.html`])) {
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
for (const locale of ['en', 'ja', 'ko', 'es', 'zh']) {
  assert.ok(sitemap.includes(`https://engine.speedle.io/${locale}/sitemap.xml`), `Sitemap index lists ${locale}`);
}
assert.ok(fs.readFileSync(path.join(output, 'en/sitemap.xml'), 'utf8').includes('https://engine.speedle.io/playground/'));
const hugoConfig = fs.readFileSync(path.join(root, 'docs/hugo/config.toml'), 'utf8');
assert.match(hugoConfig, /enableGitInfo\s*=\s*false/, 'Generated pages do not read Git metadata');
assert.doesNotMatch(hugoConfig, /:git|:filemodtime/i, 'Page dates do not fall back to a commit or file time');
for (const locale of ['ja', 'ko', 'es', 'zh']) {
  assert.ok(fs.readFileSync(path.join(output, `${locale}/sitemap.xml`), 'utf8').includes(`https://engine.speedle.io/${locale}/playground/`), `Playground in ${locale} sitemap`);
}
for (const name of LOCALES.map(locale => `${locale}quick-start/index.html`)) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated quick-start guide ${name}`);
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  const locale = name.match(/^(ja|ko|es|zh)\//)?.[1];
  const target = locale ? `/${locale}/playground/` : '/playground/';
  assert.ok(html.includes(`href="${target}"`), `Generated ${name} navigation`);
  assert.ok(html.includes('/css/command-copy.css'), `Named custom stylesheet in ${name}`);
}
// #262: customcss without a filename is a build error, never a silent fallback.
for (const name of ['_default/baseof.html', 'docs/baseof.html']) {
  const template = fs.readFileSync(path.join(root, 'docs/hugo/layouts', name), 'utf8');
  assert.ok(/\{\{\s*else\s*\}\}\s*\{\{\s*errorf "customcss is true but customcssfile is missing on %s" \.Path\s*\}\}/.test(template), `Missing customcssfile fails the build in ${name}`);
}
for (const page of ['about/index.html', 'contribute/index.html', 'support/index.html', 'docs/assertor/index.html']) {
  for (const locale of LOCALES) {
    const name = `${locale}${page}`;
    assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated article metadata ${name}`);
  }
}
for (const locale of LOCALES) {
  const html = fs.readFileSync(path.join(output, `${locale}docs/api/management_api/index.html`), 'utf8');
  assert.ok(html.includes('/css/swagger-ui.css') && html.includes('/css/swagger-custom.css'), `Swagger styles in ${locale || 'en'}`);
}
const cloudRoutes = ['', 'getting-started/', 'tokens/', 'authorization-api/', 'api-reference/', 'limits-and-errors/'];
const navPages = ['docs/index.html', 'docs/spdl/index.html', ...cloudRoutes.map(route => `docs/cloud/${route}index.html`)];
for (const locale of LOCALES) {
  const lang = locale.replace(/\/$/, '');
  for (const page of navPages) {
    const name = `${locale}${page}`;
    const html = fs.readFileSync(path.join(output, name), 'utf8');
    const navigation = html.match(/<nav class="nav-links"[^>]*>[\s\S]*?<\/nav>/)?.[0];
    assert.ok(navigation, `Language navigation in ${name}`);
    const languageLinks = [...navigation.matchAll(/<a class="language-row"[^>]*hreflang="([^"]+)"[^>]*>([^<]+)/g)];
    assert.deepEqual(languageLinks.map(match => [match[1], match[2]]), LANG_ORDER, `Language order in ${name}`);
    const neutral = page.replace(/index\.html$/, '');
    for (const [code] of LANG_ORDER) {
      const prefix = code === 'en' ? '' : `${code === 'zh-CN' ? 'zh' : code}/`;
      assert.ok(navigation.includes(`href="/${prefix}${neutral}"`), `Language href /${prefix}${neutral} in ${name}`);
    }
    assert.ok(!navigation.includes('hide-sm'), `Same control available in mobile navigation in ${name}`);
    assert.ok(!html.includes('langselFloatDiv') && !html.includes('langNameArrow'), `No dropdown in ${name}`);
    const homepage = fs.readFileSync(path.join(output, `${locale}index.html`), 'utf8');
    const labels = value => [...value.matchAll(/<a\b[^>]*>([^<]+)<\/a>/g)].map(match => match[1]);
    const homeNavigation = homepage.match(/<nav class="nav-links"[^>]*>[\s\S]*?<\/nav>/)[0];
    assert.deepEqual(labels(navigation), labels(homeNavigation), `Homepage navigation order/labels in ${name}`);
    assert.ok(navigation.includes(`href="/${locale}#features"`), `Features returns to localized homepage from ${name}`);
    assert.ok(html.includes('/css/navigation.css?v=header-20261106-v8') && html.includes('/js/navigation.js?v=header-20261106-v8'), `Shared versioned navigation assets in ${name}`);
    assert.ok(html.includes('/css/docs.css') && html.includes('/js/docs.js'), `Documentation assets in ${name}`);
    assert.ok(!/theme\.min|kube\.min|kube\.legenda|master\.css|kube\.demo|(?:^|\/)custom\.css|spdl-title|fonts\.googleapis|fontawesome|jquery|tocbot|\/js\/kube|\/js\/master/.test(html), `No legacy docs stack in ${name}`);
    assert.ok(html.includes('class="docs-nav-heading') && html.includes('docs-nav-close') && html.includes('docs-nav-backdrop'), `Modern sidebar markup in ${name}`);
    assert.ok(html.includes('aria-current="page"'), `Current page marked in the sidebar in ${name}`);
    assert.ok(new RegExp(`<button class="docs-nav-toggle"[^>]*>[\\s\\S]*?<span>${MENU_LABEL[lang]}</span>`).test(html), `Menu drawer button in ${name}`);
    assert.ok(new RegExp(`docs-nav-heading[^"]*"><a [^>]*>${SDK_LABEL[lang]}</a>`).test(html), `SDK group heading in ${name}`);
    assert.ok(new RegExp(`docs-nav-heading[^"]*"><a [^>]*>${DEPLOY_LABEL[lang]}</a>`).test(html), `Deployment group heading in ${name}`);
    const afterFooter = html.split('</footer>')[1];
    assert.ok(afterFooter && !afterFooter.includes('/js/'), `No layout script after the footer in ${name}`);
  }
}
// #243: the remaining legacy-theme pages use the docs styles and load no Kube stack.
for (const page of ['quick-start/index.html', 'about/index.html', 'contribute/index.html', 'support/index.html',
  'integrations/index.html', 'versions/index.html', 'tags/index.html', 'categories/index.html',
  'tags/api/index.html', 'categories/docs/page/2/index.html']) {
  for (const locale of LOCALES) {
    const name = `${locale}${page}`;
    const html = fs.readFileSync(path.join(output, name), 'utf8');
    assert.ok(html.includes('/css/docs.css'), `Docs styles in ${name}`);
    assert.ok(!/theme\.min|kube\.min|kube\.legenda|master\.css|kube\.demo|(?:^|\/)custom\.css|spdl-title|fonts\.googleapis|fontawesome|jquery|tocbot|\/js\/kube|\/js\/master/.test(html), `No legacy stack in ${name}`);
    assert.ok(html.includes('<body class="page-kube docs-page">'), `Docs body in ${name}`);
    assert.ok(html.includes('class="content-container docs-article"'), `Stand-alone docs container in ${name}`);
  }
}
// #203: every page with the shared footer loads navigation.css, the stylesheet that holds the footer rules.
const footerRules = fs.readFileSync(path.join(output, 'css/navigation.css'), 'utf8');
assert.ok(footerRules.includes('html body footer.site-footer {') && footerRules.includes('.site-footer .footer-main {'), 'navigation.css holds the shared footer rules');
assert.ok(/^\.site-footer \.footer-legal span \{ display: block; \}$/m.test(footerRules), 'License and copyright stack at every viewport width');
assert.ok(footerRules.includes('.site-footer .footer-legal .footer-legal-sep {\n  position: absolute;\n  width: 1px;\n  height: 1px;'), 'Footer legal separator is visually hidden but stays in the text and accessibility tree');
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
      const parts = relative.split(path.sep);
      const start = /^[0-9a-f]{32}$/.test(parts[0]) ? 1 : 0;
      const lang = ['zh', 'ja', 'ko', 'es'].includes(parts[start]) ? parts[start] : 'en';
      const legal = {
        en: '<p class="footer-legal"><span>Licensed under the Universal Permissive License v1.0.</span><span class="footer-legal-sep"> · </span><span>Copyright &copy; 2019–2026 The Speedle Authors. All rights reserved.</span></p>',
        zh: '<p class="footer-legal"><span>基于 Universal Permissive License v1.0 授权。</span><span class="footer-legal-sep"> · </span><span>版权所有 &copy; 2019–2026 The Speedle Authors。</span></p>',
        ja: '<p class="footer-legal"><span>Universal Permissive License v1.0 の下でライセンスされています。</span><span class="footer-legal-sep"> · </span><span>Copyright &copy; 2019–2026 The Speedle Authors. All rights reserved.</span></p>',
        ko: '<p class="footer-legal"><span>Universal Permissive License v1.0에 따라 라이선스가 부여됩니다.</span><span class="footer-legal-sep"> · </span><span>Copyright &copy; 2019–2026 The Speedle Authors. All rights reserved.</span></p>',
        es: '<p class="footer-legal"><span>Con licencia Universal Permissive License v1.0.</span><span class="footer-legal-sep"> · </span><span>Copyright &copy; 2019–2026 The Speedle Authors. Todos los derechos reservados.</span></p>',
      }[lang];
      assert.ok(html.includes(legal), `Separate license and approved copyright lines in ${relative}`);
      assert.ok(!html.includes('footer-sep'), `No legal-line joiner in ${relative}`);
    }
  }
})(output);
for (const name of LOCALES.flatMap(locale => [`${locale}docs/sdks/index.html`, `${locale}docs/sdks/java/index.html`, `${locale}docs/api/index.html`, `${locale}docs/spdl/index.html`])) {
  assert.ok(fs.readFileSync(path.join(output, name), 'utf8').includes('<footer class="site-footer">'), `Shared footer in ${name}`);
}
assert.ok(footerPages > 190, `Footer pages checked: ${footerPages}`);
// #208: the SDK pages moved from /docs/api/sdk-<lang>/ to /docs/sdks/<lang>/; the old URLs redirect.
for (const locale of LOCALES) {
  for (const lang of ['java', 'python', 'go', 'dotnet', 'rust', 'node']) {
    const target = `https://engine.speedle.io/${locale}docs/sdks/${lang}/`;
    const alias = fs.readFileSync(path.join(output, `${locale}docs/api/sdk-${lang}/index.html`), 'utf8');
    assert.ok(alias.includes(`<link rel="canonical" href="${target}"`) && alias.includes(`url=/${locale}docs/sdks/${lang}/"`), `Redirect ${locale}docs/api/sdk-${lang}/ to ${target}`);
    const sitemap = fs.readFileSync(path.join(output, `${locale || 'en/'}sitemap.xml`), 'utf8');
    assert.ok(sitemap.includes(`<loc>${target}</loc>`) && !sitemap.includes(`docs/api/sdk-${lang}/`), `Sitemap lists only ${target}`);
  }
}
// The embedded guide spelling is corrected, and the Docker image page left SDKs.
// Both previous URLs keep redirecting.
for (const locale of LOCALES) {
  const embedded = `https://engine.speedle.io/${locale}docs/deployment/embedded/`;
  const alias = fs.readFileSync(path.join(output, `${locale}docs/deployment-embeded/index.html`), 'utf8');
  assert.ok(alias.includes(`<link rel="canonical" href="${embedded}"`) && alias.includes(`url=/${locale}docs/deployment/embedded/"`), `Redirect ${locale}docs/deployment-embeded/ to ${embedded}`);
  const docker = `https://engine.speedle.io/${locale}docs/deployment/docker/`;
  const dockerAlias = fs.readFileSync(path.join(output, `${locale}docs/sdks/docker/index.html`), 'utf8');
  assert.ok(dockerAlias.includes(`<link rel="canonical" href="${docker}"`) && dockerAlias.includes(`url=/${locale}docs/deployment/docker/"`), `Redirect ${locale}docs/sdks/docker/ to ${docker}`);
  const sitemap = fs.readFileSync(path.join(output, `${locale || 'en/'}sitemap.xml`), 'utf8');
  const homebrew = `https://engine.speedle.io/${locale}docs/deployment/homebrew/`;
  assert.ok(sitemap.includes(`<loc>${embedded}</loc>`) && sitemap.includes(`<loc>${docker}</loc>`) && sitemap.includes(`<loc>${homebrew}</loc>`), `Sitemap lists the Deployment pages in ${locale || 'en'}`);
  assert.ok(!sitemap.includes(`docs/deployment-embeded/`) && !sitemap.includes(`docs/sdks/docker/`), `Sitemap omits the old embedded and Docker URLs in ${locale || 'en'}`);
  assert.ok(fs.readFileSync(path.join(output, `${locale || 'en/'}sitemap.xml`), 'utf8').includes(`<loc>https://engine.speedle.io/${locale}docs/deployment/</loc>`), `Sitemap lists the Deployment section in ${locale || 'en'}`);
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
    const english = canonical.replace(/\/(?:zh|ja|ko|es)\/docs\//, '/docs/');
    const alternates = [
      ['en', english],
      ['ja', english.replace('/docs/', '/ja/docs/')],
      ['ko', english.replace('/docs/', '/ko/docs/')],
      ['es', english.replace('/docs/', '/es/docs/')],
      ['zh-CN', english.replace('/docs/', '/zh/docs/')],
      ['x-default', english],
    ];
    for (const [language, target] of alternates) {
      assert.ok(html.includes(`<link rel="alternate" hreflang="${language}" href="${target}">`), `${language} alternate in ${file}`);
    }
    docsAlternates++;
  }
}
for (const locale of LOCALES) checkDocsAlternates(path.join(output, `${locale}docs`));
assert.equal(docsAlternates, 255, 'Every canonical docs page and redirect alias has hreflang alternates for en, ja, ko, es, zh-CN and x-default');
console.log('PASS Hugo 0.48 output: editor, navigation, sitemap, footer stylesheet and matching release assets');

// The Cloud section is documented in every site language.
const cloudCopy = {
  '': {
    soon: 'Speedle Cloud is coming soon', badge: 'Coming soon', preview: 'production sign-in and authenticated authorization checks open',
    signin: 'Sign in with GitHub or Google (opens soon)',
    download: 'quickstart.en.md', legal: '', samples: true,
  },
  'zh/': {
    soon: 'Speedle Cloud 即将上线', badge: 'Coming soon', preview: '生产环境开放登录和授权 API 后的使用流程',
    signin: '使用 GitHub 或 Google 登录（即将开放）',
    download: 'quickstart.zh-CN.md', legal: 'zh-CN/', samples: true,
  },
  'ja/': {
    soon: 'Speedle Cloud はまもなく公開されます', badge: 'Coming soon（まもなく公開）', preview: '本番環境のサインインと認証付き認可 API が利用可能になった際',
    signin: 'GitHub または Google',
    download: 'quickstart.ja.md', legal: 'ja/', samples: false,
  },
  'ko/': {
    soon: 'Speedle Cloud는 출시 예정입니다', badge: 'Coming soon(출시 예정)', preview: '프로덕션 로그인과 인증된 인가 API가 제공될 때',
    signin: 'GitHub 또는 Google',
    download: 'quickstart.ko.md', legal: 'ko/', samples: false,
  },
  'es/': {
    soon: 'Speedle Cloud estará disponible próximamente', badge: 'Próximamente', preview: 'inicio de sesión en producción y las comprobaciones de autorización autenticadas',
    signin: 'Iniciar sesión con GitHub o Google (próximamente)',
    download: 'quickstart.es.md', legal: 'es/', samples: true,
  },
};
for (const [locale, copy] of Object.entries(cloudCopy)) {
  const prefix = `https://engine.speedle.io/${locale}docs/cloud/`;
  const read = route => fs.readFileSync(path.join(output, `${locale}docs/cloud/${route}index.html`), 'utf8');
  const overview = read('');
  const guide = read('getting-started/');
  const tokens = read('tokens/');
  const api = read('authorization-api/');
  const limits = read('limits-and-errors/');
  const sidebar = guide.split('<div class="sidebar">')[1];
  assert.ok(/docs-nav-heading[^"]*"><a [^>]*>Speedle Cloud<\/a>/.test(sidebar), `Cloud is a linked bold sidebar heading in ${locale || 'en'}`);
  for (const route of cloudRoutes.slice(1)) {
    assert.ok(sidebar.includes(`href="/${locale}docs/cloud/${route}"`), `Cloud ${route} link in ${locale || 'en'}`);
  }
  assert.ok(overview.includes(copy.soon) && overview.includes(copy.badge), `Coming-soon overview in ${locale || 'en'}`);
  assert.ok(guide.includes(copy.preview) && guide.includes(copy.signin), `Preview sign-in notice in ${locale || 'en'}`);
  assert.ok(guide.includes('grant user alice from corp get /books'), `Domain-scoped getting-started policy in ${locale || 'en'}`);
  const quickStart = ['SPEEDLE_API_KEY', 'books-app', 'read-books', 'cloud.speedle.io/authz-check/v1/is-allowed', 'HTTP 200'];
  assert.ok(quickStart.every(marker => guide.includes(marker)), `API-key quick start in ${locale || 'en'}`);
  assert.ok(!tokens.includes('UUIDv7'), `Obsolete token format removed in ${locale || 'en'}`);
  assert.ok(api.includes('POST /authz-check/v1/is-allowed'), `ADS preview route in ${locale || 'en'}`);
  if (copy.samples) {
    assert.ok(tokens.includes('spdl_&lt;canonical uppercase ULID id&gt;_&lt;43 base62 digits&gt;'), `Current token format in ${locale || 'en'}`);
    assert.ok(tokens.includes('reauth_required') && tokens.includes('DELETE /api/tokens/') && tokens.includes('nextCursor'), `Token creation, revocation and pagination in ${locale || 'en'}`);
    assert.ok(api.includes('service_mismatch') && api.includes('subject.token'), `ADS preview contract in ${locale || 'en'}`);
    assert.ok(api.includes('AbortSignal.timeout') && api.includes('SPEEDLE_ADS_TOKEN'), `Node.js ADS example in ${locale || 'en'}`);
    assert.ok(limits.includes('X-Quota-Daily-Remaining') && limits.includes('daily_quota_exceeded') && limits.includes('snapshot_unavailable'), `Cloud limits and errors in ${locale || 'en'}`);
  }
  assert.ok(overview.includes(`href="/docs/cloud/${copy.download}"`), `Localized Markdown download in ${locale || 'en'}`);
  assert.ok(overview.includes('https://cloud.speedle.io/openapi.yaml') && overview.includes('https://cloud.speedle.io/openapi.json') && overview.includes('https://cloud.speedle.io/build.json'), `Cloud schema and build downloads in ${locale || 'en'}`);
  assert.ok(overview.includes(`href="/${locale}docs/cloud/api-reference/"`), `Cloud API reference link in ${locale || 'en'}`);
  const reference = read('api-reference/');
  assert.ok(reference.includes('supportedSubmitMethods: []'), `Try it out disabled on the Cloud reference in ${locale || 'en'}`);
  assert.ok(reference.includes('id="swagger-ui" class="swagger-readonly"'), `Authorize hidden on the Cloud reference in ${locale || 'en'}`);
  assert.ok(reference.includes('/openapi/cloud-public.yaml'), `Public spec on the Cloud reference in ${locale || 'en'}`);
  assert.ok(reference.includes('/css/swagger-ui.css') && reference.includes('/css/swagger-custom.css'), `Swagger styles on the Cloud reference in ${locale || 'en'}`);
  const referenceLead = {
    '': 'authorization checks and policy management with an API key',
    'zh/': '使用 API key 进行授权检查和策略管理',
    'ja/': 'API key による認可チェックとポリシー管理',
    'ko/': 'API key를 사용한 인가 확인과 정책 관리',
    'es/': 'comprobaciones de autorización y gestión de políticas con una API key',
  }[locale];
  const referenceArticle = reference.split('<div id="main"')[1] || '';
  assert.ok(referenceArticle.includes(referenceLead), `Cloud reference lead in ${locale || 'en'}`);
  assert.ok(overview.includes(referenceLead), `Cloud index describes the reference in ${locale || 'en'}`);
  assert.ok(!/placeholder|vendored|Swagger UI|Try it out|application token|应用令牌|アプリケーション トークン|token de aplicación|애플리케이션 토큰/.test(referenceArticle), `Reader-facing Cloud reference in ${locale || 'en'}`);
  assert.ok(!overview.includes('Swagger UI') && !overview.includes('Try it out'), `Cloud index does not describe the reference as Swagger UI in ${locale || 'en'}`);
  const decision = fs.readFileSync(path.join(output, `${locale}docs/api/decision_api/index.html`), 'utf8');
  assert.ok(decision.includes('supportedSubmitMethods: []'), `Try it out disabled on the engine decision reference in ${locale || 'en'}`);
  assert.ok(decision.includes('id="swagger-ui" class="swagger-readonly"'), `Authorize hidden on the engine decision reference in ${locale || 'en'}`);
  assert.ok(overview.includes(`href="https://cloud.speedle.io/${copy.legal}terms"`) && overview.includes(`href="https://cloud.speedle.io/${copy.legal}privacy"`), `Localized Cloud legal links in ${locale || 'en'}`);
  const markdown = fs.readFileSync(path.join(output, 'docs/cloud', copy.download), 'utf8');
  assert.deepEqual(markdown, fs.readFileSync(path.join(root, 'docs/hugo/static/docs/cloud', copy.download), 'utf8'), `Generated ${copy.download}`);
  for (const phrase of ['getting-started', 'tokens', 'authorization-api', 'limits-and-errors']) {
    assert.ok(markdown.includes(`https://engine.speedle.io/${locale}docs/cloud/${phrase}`), `Offline Markdown links ${phrase} in ${locale || 'en'}`);
  }
  for (const route of cloudRoutes) {
    assert.ok(fs.readFileSync(path.join(output, `${locale || 'en/'}sitemap.xml`), 'utf8').includes(`<loc>${prefix}${route}</loc>`), `Cloud sitemap entry ${prefix}${route}`);
  }
}
const llms = fs.readFileSync(path.join(output, 'llms.txt'), 'utf8');
const llmsFull = fs.readFileSync(path.join(output, 'llms-full.txt'), 'utf8');
assert.ok(llms.includes('[Speedle Cloud](https://engine.speedle.io/docs/cloud/)'), 'llms.txt links the Cloud section');
assert.ok(llms.includes('[API reference](https://engine.speedle.io/docs/cloud/api-reference/)'), 'llms.txt links the Cloud API reference');
const docsJs = fs.readFileSync(path.join(root, 'docs/hugo/static/js/docs.js'), 'utf8');
assert.ok(docsJs.includes('.opblock-tag-section > h3') && docsJs.includes('section.models > h4'), 'Swagger UI 5 operation groups are h3 and models stay h4');
for (const name of ['css/swagger-ui.css', 'css/swagger-custom.css', 'js/swagger-js/swagger-ui-bundle.js', 'js/docs.js']) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Published Swagger asset ${name}`);
}
assert.ok(fs.readFileSync(path.join(output, 'js/swagger-js/swagger-ui-bundle.js'), 'utf8').includes('5.34.1'), 'Vendored Swagger UI is 5.34.1');
const swaggerCustom = fs.readFileSync(path.join(output, 'css/swagger-custom.css'), 'utf8');
assert.ok(swaggerCustom.includes('#swagger-ui.swagger-readonly .auth-wrapper') && swaggerCustom.includes('#swagger-ui.swagger-readonly .authorization__btn'), 'Read-only Swagger UI hides Authorize and the operation lock');
function listFiles(directory, suffix, found = []) {
  for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) listFiles(file, suffix, found);
    else if (entry.name.endsWith(suffix)) found.push(file);
  }
  return found;
}
const swaggerSources = listFiles(path.join(root, 'docs/hugo/content'), '.md').filter(file => fs.readFileSync(file, 'utf8').includes('tryitout="false"'));
const swaggerPages = listFiles(output, '.html').filter(file => fs.readFileSync(file, 'utf8').includes('SwaggerUIBundle({'));
assert.equal(swaggerPages.length, swaggerSources.length, 'Every read-only Swagger source page is published');
for (const file of swaggerPages) {
  const html = fs.readFileSync(file, 'utf8');
  const rel = path.relative(output, file);
  assert.ok(html.includes('id="swagger-ui" class="swagger-readonly"'), `${rel} hides Authorize`);
  assert.ok(html.includes('supportedSubmitMethods: []'), `${rel} disables Try it out`);
}
assert.ok(swaggerCustom.includes('max-width: 100%') && swaggerCustom.includes('overflow-x: clip') && swaggerCustom.includes('overflow-wrap: anywhere'), 'Swagger UI stays inside the article width');
assert.ok(/body\.docs-page > \.container \{[^}]*min-width: 0/.test(docsStyles), 'The docs article can shrink to the viewport');
assert.ok(docsStyles.includes('width: min(1160px, calc(100vw - 28px))'), 'Narrow docs pages size from the viewport');
assert.ok(/body\.docs-page \{[^}]*overflow-x: clip/.test(docsStyles), 'Docs pages do not scroll horizontally');
assert.ok(llms.includes('https://ai.speedle.io/llms.txt') && llms.includes('https://cloud.speedle.io/llms.txt'), 'llms.txt links the other sites\' llms.txt');
assert.ok(llmsFull.includes('grant role manager employee') && llmsFull.includes('ROLE_POLICY ='),
  'llms-full.txt contains the SPDL grammar and role-inheritance example');
for (const name of ['llms.txt', 'llms-full.txt']) {
  assert.deepEqual(fs.readFileSync(path.join(output, name)), fs.readFileSync(path.join(root, 'docs', name)), `Generated ${name} matches the published copy`);
}

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
        'deployment/', 'deployment/embedded/', 'deployment/docker/', 'deployment/homebrew/',
        'cloud/', 'cloud/getting-started/', 'cloud/tokens/', 'cloud/authorization-api/', 'cloud/api-reference/', 'cloud/limits-and-errors/']) {
        assert.ok(sidebar.includes(`href="/${locale}docs/${target}"`), `Complete sidebar ${target} in ${name}`);
      }
      for (const [depth, target] of [[0, 'security/'], [1, 'pms/policy-mgmt/'], [1, 'store/mysql/'],
        [1, 'api/decision_api/'], [1, 'sdks/java/'], [1, 'deployment/embedded/'], [1, 'deployment/docker/'], [1, 'deployment/homebrew/'], [1, 'cloud/getting-started/'], [1, 'cloud/api-reference/']]) {
        assert.ok(new RegExp(`<li data-docs-depth="${depth}"[^>]*><a href="/${locale}docs/${target}"`).test(sidebar), `Menu depth ${depth} for ${target} in ${name}`);
      }
    }
  }
}
for (const name of LOCALES.flatMap(locale => [`${locale}docs/index.html`, `${locale}quick-start/index.html`, `${locale}about/index.html`])) {
  const html = fs.readFileSync(path.join(output, name), 'utf8');
  assert.ok(html.includes('<footer class="site-footer">') && html.includes('class="footer-family"') && !html.includes('<footer class="row">'), `Shared homepage footer in ${name}`);
}
for (const locale of LOCALES) checkStorageNavigation(path.join(output, `${locale}docs`), locale);
console.log('PASS documentation sidebars in en, ja, ko, es and zh with nested MySQL, etcd and MongoDB guides');

// #281 review: Hugo 0.48 dates a list page from the first child page that has a
// date, so a category feed could report January 2019 while listing November 2026
// pages. The project RSS template now derives both feed dates from the newest
// dated page, so no feed may claim to be older than what it lists.
const undated = 'Mon, 01 Jan 0001 00:00:00 +0000';
const feedDates = xml => [...xml.matchAll(/<pubDate>([^<]+)<\/pubDate>/g)]
  .map(match => match[1]).filter(date => date !== undated).map(date => Date.parse(date));
const newest = dates => dates.reduce((max, date) => Math.max(max, date), -Infinity);
let feedCount = 0;
function checkFeedDates(directory) {
  for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) { checkFeedDates(file); continue; }
    if (entry.name !== 'index.xml') continue;
    feedCount++;
    const xml = fs.readFileSync(file, 'utf8');
    const items = feedDates(xml);
    if (items.length === 0) continue;
    const build = xml.match(/<lastBuildDate>([^<]+)<\/lastBuildDate>/)?.[1];
    assert.ok(build, `lastBuildDate in ${path.relative(output, file)}`);
    assert.ok(Date.parse(build) >= newest(items), `Newest item is not after lastBuildDate in ${path.relative(output, file)}`);
  }
}
checkFeedDates(output);
assert.ok(feedCount > 60, `Feeds checked: ${feedCount}`);
// The taxonomy feeds list a term whose own date is stale for the same reason.
for (const plural of ['categories', 'tags', 'versions']) {
  const list = path.join(output, plural, 'index.xml');
  if (!fs.existsSync(list)) continue;
  const xml = fs.readFileSync(list, 'utf8');
  for (const [, link, pubDate] of xml.matchAll(/<link>([^<]+)<\/link>\s*<pubDate>([^<]+)<\/pubDate>/g)) {
    if (pubDate === undated) continue;
    const feed = path.join(output, new URL(link).pathname, 'index.xml');
    if (!fs.existsSync(feed)) continue;
    const items = feedDates(fs.readFileSync(feed, 'utf8'));
    if (items.length === 0) continue;
    assert.ok(Date.parse(pubDate) >= newest(items), `Term ${link} is older than its newest item`);
  }
}
const docsCategory = fs.readFileSync(path.join(output, 'categories/docs/index.xml'), 'utf8');
assert.ok(Date.parse(docsCategory.match(/<lastBuildDate>([^<]+)<\/lastBuildDate>/)[1]) > Date.parse('Mon, 01 Jan 2020 00:00:00 +0000'),
  'Docs category feed is not dated before 2020');
console.log('PASS every feed is dated from its newest page');
