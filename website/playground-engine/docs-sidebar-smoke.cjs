/* #225: every documentation sidebar link must be reachable by page scroll.
   Requires Playwright + Chromium. Run from the repository root:
   node website/playground-engine/docs-sidebar-smoke.cjs [generated-site-dir]
   Optional CHROMIUM_PATH selects an installed Chromium/Chrome binary. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const {chromium} = require('playwright');

const repository = path.resolve(__dirname, '../..');
const output = path.resolve(process.argv[2] || path.join(repository, 'docs'));
const content = path.join(repository, 'docs/hugo/content/docs');

function menuConfig(zh) {
  const entries = {};
  (function walk(directory) {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
      const file = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        walk(file);
        continue;
      }
      if (!entry.name.endsWith('.md')) continue;
      const locale = entry.name.match(/\.(zh|ja|ko|es)\.md$/)?.[1] || 'en';
      if (locale !== (zh ? 'zh' : 'en')) continue;
      const relative = path.relative(content, file).split(path.sep).join('/');
      const section = entry.name === (zh ? '_index.zh.md' : '_index.md');
      const suffix = zh ? /\.zh\.md$/ : /\.md$/;
      const route = (section ? path.posix.dirname(relative) : relative.replace(suffix, ''))
        .replace(/^\.$/, '');
      const source = fs.readFileSync(file, 'utf8');
      const linked = !section || /^linksection\s*=\s*true\s*$/m.test(source);
      entries[`/${zh ? 'zh/' : ''}docs/${route ? route + '/' : ''}`] = {
        depth: /^misc\s*=\s*true\s*$/m.test(source) ? 1 : route ? route.split('/').length - 1 : -1,
        linked,
      };
    }
  })(content);
  return entries;
}

function generatedPages(directory) {
  const routes = [];
  (function walk(dir) {
    for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(file);
      } else if (entry.name === 'index.html' && fs.readFileSync(file, 'utf8').includes('class="docs-sidebar"')) {
        const route = path.relative(output, file).split(path.sep).join('/').replace(/index\.html$/, '');
        routes.push('/' + route);
      }
    }
  })(directory);
  return routes.sort();
}

const configs = {'/docs/': menuConfig(false), '/zh/docs/': menuConfig(true)};
for (const [prefix, config] of Object.entries(configs)) {
  const pages = generatedPages(path.join(output, prefix.slice(1)));
  assert.deepEqual(pages, Object.keys(config).sort(), `Every ${prefix} source page is browser-tested`);
}

const server = http.createServer((request, response) => {
  let pathname;
  try {
    pathname = decodeURIComponent(new URL(request.url, 'http://localhost').pathname);
  } catch {
    response.writeHead(400).end();
    return;
  }
  let file = path.resolve(output, '.' + pathname);
  if (file !== output && !file.startsWith(output + path.sep)) {
    response.writeHead(403).end();
    return;
  }
  if (fs.existsSync(file) && fs.statSync(file).isDirectory()) file = path.join(file, 'index.html');
  const types = {'.html': 'text/html; charset=utf-8', '.css': 'text/css', '.js': 'text/javascript', '.svg': 'image/svg+xml', '.yaml': 'text/yaml; charset=utf-8', '.yml': 'text/yaml; charset=utf-8'};
  fs.readFile(file, (error, bytes) => {
    if (error) {
      response.writeHead(404).end();
      return;
    }
    response.setHeader('content-type', types[path.extname(file)] || 'application/octet-stream');
    response.end(bytes);
  });
});

async function checkDesktop(page, width, expected) {
  const result = await page.evaluate(({width, expected}) => {
    document.documentElement.style.scrollBehavior = 'auto';
    window.scrollTo(0, 0);
    const failures = [];
    const sidebar = document.getElementById('docs-navigation');
    const footer = document.querySelector('body.docs-page > footer');
    if (!sidebar || !footer) return {failures: ['Missing sidebar or shared footer'], links: 0};
    const sidebarStyle = getComputedStyle(sidebar);
    if (sidebarStyle.position !== 'static') failures.push(`Sidebar is ${sidebarStyle.position}, not in normal flow`);
    if (sidebarStyle.maxHeight !== 'none') failures.push(`Sidebar max-height is ${sidebarStyle.maxHeight}`);
    if (!['visible', 'clip'].includes(sidebarStyle.overflowY)) failures.push(`Sidebar overflow-y is ${sidebarStyle.overflowY}`);
    if (sidebar.scrollHeight > sidebar.clientHeight + 1) failures.push(`Sidebar clips ${sidebar.scrollHeight - sidebar.clientHeight}px of its menu`);

    const items = [...sidebar.querySelectorAll('.sidebar li')];
    const seen = new Set();
    for (const item of items) {
      const listDepth = (() => {
        let depth = -2;
        for (let parent = item.parentElement; parent && parent !== sidebar; parent = parent.parentElement) {
          if (parent.tagName === 'UL') depth++;
        }
        return depth;
      })();
      const control = item.querySelector(':scope > a') || item.querySelector(':scope > .docs-nav-heading');
      if (!control) continue;
      const indentation = parseFloat(getComputedStyle(control).paddingLeft);
      const wanted = 12 + Math.max(0, listDepth) * 16;
      if (Math.abs(indentation - wanted) > 0.5) failures.push(`Depth ${listDepth} item has ${indentation}px left padding, expected ${wanted}px`);
      const link = control.matches('a') ? control : control.querySelector('a');
      if (link) {
        const route = new URL(link.href).pathname;
        seen.add(route);
        if (!expected[route]) failures.push(`Unexpected menu link ${route}`);
        else if (expected[route].depth !== listDepth) failures.push(`${route} has DOM depth ${listDepth}, menu config depth ${expected[route].depth}`);
      }
    }
    for (const [route, entry] of Object.entries(expected)) {
      if (entry.linked && !seen.has(route)) failures.push(`Missing expanded menu link ${route}`);
    }

    const misc = sidebar.querySelector('.sidebar nav > ul > li > ul > li:last-child');
    const guide = misc?.querySelector(':scope > ul > li > a');
    if (misc?.querySelector(':scope > .docs-nav-heading')?.textContent !== (document.documentElement.lang.startsWith('zh') ? '其他' : 'Misc')) failures.push('Misc is not the final section');
    if (!guide?.pathname.endsWith('/docs/performance-testing/')) failures.push('Guide is not nested in Misc');
    const links = [...sidebar.querySelectorAll('a[href]')];
    for (const link of links) {
      if (!link.getClientRects().length) {
        failures.push(`Collapsed or hidden link ${link.pathname}`);
        continue;
      }
      for (let parent = link.parentElement; parent && parent !== document.body; parent = parent.parentElement) {
        const style = getComputedStyle(parent);
        if (['auto', 'hidden', 'scroll'].includes(style.overflowY) && parent.scrollHeight > parent.clientHeight + 1) {
          failures.push(`${link.pathname} is inside a limited ${style.overflowY} container <${parent.tagName.toLowerCase()} class="${parent.className}">`);
        }
      }
      const top = link.getBoundingClientRect().top + window.scrollY;
      const maximum = document.documentElement.scrollHeight - innerHeight;
      window.scrollTo(0, Math.max(0, Math.min(maximum, top - 100)));
      const rectangle = link.getBoundingClientRect();
      if (rectangle.top < -0.5 || rectangle.bottom > innerHeight + 0.5) failures.push(`Page scroll cannot reveal ${link.pathname} at ${width}px`);
    }
    window.scrollTo(0, 0);
    const footerTop = footer.getBoundingClientRect().top + window.scrollY;
    const sidebarBottom = sidebar.getBoundingClientRect().bottom + window.scrollY;
    const mainBottom = sidebar.parentElement.querySelector(':scope > main').getBoundingClientRect().bottom + window.scrollY;
    if (footerTop + 1 < Math.max(sidebarBottom, mainBottom)) failures.push('Footer starts before the taller of sidebar and content');
    return {failures, links: links.length};
  }, {width, expected});
  assert.equal(result.failures.length, 0, `${width}px ${page.url()}\n${result.failures.slice(0, 15).join('\n')}`);
  assert.ok(result.links > 30, `Complete sidebar at ${width}px ${page.url()}`);
  return result.links;
}

async function checkPhoneDrawer(page, expected) {
  await page.locator('.docs-nav-toggle').click();
  const result = await page.evaluate(expected => {
    const sidebar = document.getElementById('docs-navigation');
    const failures = [];
    const misc = sidebar.querySelector('.sidebar nav > ul > li > ul > li:last-child');
    const guide = misc?.querySelector(':scope > ul > li > a');
    if (misc?.querySelector(':scope > .docs-nav-heading')?.textContent !== (document.documentElement.lang.startsWith('zh') ? '其他' : 'Misc')) failures.push('Misc is not the final section');
    if (!guide?.pathname.endsWith('/docs/performance-testing/')) failures.push('Guide is not nested in Misc');
    const links = [...sidebar.querySelectorAll('a[href]')];
    const seen = new Set(links.map(link => link.pathname));
    for (const [route, entry] of Object.entries(expected)) {
      if (entry.linked && !seen.has(route)) failures.push(`Drawer is missing ${route}`);
    }
    for (const link of links) {
      link.scrollIntoView({block: 'center', behavior: 'instant'});
      const rectangle = link.getBoundingClientRect();
      if (rectangle.top < -0.5 || rectangle.bottom > innerHeight + 0.5) failures.push(`Drawer scroll cannot reveal ${link.pathname}`);
    }
    return {failures, links: links.length};
  }, expected);
  assert.equal(result.failures.length, 0, `375px ${page.url()}\n${result.failures.slice(0, 15).join('\n')}`);
  await page.keyboard.press('Escape');
  assert.equal(await page.locator('.docs-nav-toggle').getAttribute('aria-expanded'), 'false');
}

async function checkReadOnlySwagger(page, origin) {
  for (const route of ['/docs/cloud/api-reference/', '/docs/api/decision_api/']) {
    await page.setViewportSize({width: 1440, height: 900});
    await page.goto(origin + route, {waitUntil: 'domcontentloaded'});
    await page.waitForSelector('#swagger-ui .opblock', {timeout: 20000});
    await page.locator('#swagger-ui .opblock-summary').first().click();
    // Playwright isVisible() follows ancestor display:none. A child's own
    // computed display stays "inline-block" when .auth-wrapper is hidden.
    const controls = page.locator('#swagger-ui .auth-wrapper, #swagger-ui .authorization__btn, #swagger-ui .btn.authorize');
    const count = await controls.count();
    if (route === '/docs/cloud/api-reference/') assert.ok(count > 0, `${route} did not render an authorize control to hide`);
    for (let index = 0; index < count; index++) {
      assert.equal(await controls.nth(index).isVisible(), false, `${route} shows an authorize control`);
    }
    const dialogs = page.locator('.dialog-ux');
    const dialogCount = await dialogs.count();
    for (let index = 0; index < dialogCount; index++) {
      assert.equal(await dialogs.nth(index).isVisible(), false, `${route} opened an authorize dialog`);
    }
  }
  console.log('PASS read-only Swagger UI hides Authorize and the operation lock');
}

(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({headless: true, ...(process.env.CHROMIUM_PATH ? {executablePath: process.env.CHROMIUM_PATH} : {})});
    const page = await browser.newPage({viewport: {width: 1440, height: 900}, reducedMotion: 'reduce'});
    await page.route('https://**/*', route => route.abort());
    let checks = 0;
    let links = 0;
    for (const [prefix, expected] of Object.entries(configs)) {
      for (const route of Object.keys(expected).sort()) {
        await page.setViewportSize({width: 1440, height: 900});
        await page.goto(origin + route);
        for (const width of [1440, 1024]) {
          await page.setViewportSize({width, height: 900});
          links = await checkDesktop(page, width, expected);
          checks++;
        }
        if (['docs/', 'docs/sdks/java/', 'docs/cloud/getting-started/', 'docs/performance-testing/'].includes(route.replace(/^\/(?:zh\/)?/, ''))) {
          await page.setViewportSize({width: 375, height: 844});
          await checkPhoneDrawer(page, expected);
        }
      }
    }
    const pageCount = Object.values(configs).reduce((total, config) => total + Object.keys(config).length, 0);
    console.log(`PASS docs sidebar: ${checks} desktop checks across ${pageCount} EN/ZH pages at 1440/1024px; ${links} links per menu; eight expanded phone drawers at 375px`);
    await checkReadOnlySwagger(page, origin);
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
