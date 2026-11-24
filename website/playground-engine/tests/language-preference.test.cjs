const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const source = fs.readFileSync(path.join(__dirname, '../../../docs/js/navigation.js'), 'utf8');

function run({cookie = '', pathname = '/', hostname = 'engine.speedle.io', protocol = 'https:', blocked = false} = {}) {
  const writes = [], redirects = [], listeners = {};
  const document = {
    querySelectorAll: () => [],
    querySelector: () => ({querySelectorAll: () => []}),
    addEventListener: (name, fn) => { listeners[name] = fn; },
    get cookie() { if (blocked) throw new Error('blocked'); return cookie; },
    set cookie(value) { if (blocked) throw new Error('blocked'); writes.push(value); },
  };
  vm.runInNewContext(source, {
    document, window: {},
    location: {hostname, protocol, pathname, search: '?from=products', hash: '#overview', replace: value => redirects.push(value)},
  });
  function select(locale, type = 'click', button = 0) {
    listeners[type]?.({type, button, target: {closest: () => ({getAttribute: () => locale})}});
  }
  return {writes, redirects, select};
}

for (const [locale, prefix] of [['ja', 'ja'], ['ko', 'ko'], ['es', 'es'], ['zh-CN', 'zh']]) {
  test(`shared ${locale} choice applies on neutral entry and keeps query/fragment`, () => {
    const page = run({cookie: `session=unrelated; speedle_locale=${locale}`});
    assert.deepEqual(page.redirects, [`/${prefix}/?from=products#overview`]);
    assert.deepEqual(page.writes, []);
  });
}

test('every explicit language selection, including English, writes the shared contract', () => {
  for (const locale of ['en', 'ja', 'ko', 'es', 'zh-CN']) {
    const page = run({pathname: '/ja/'});
    page.select(locale);
    assert.deepEqual(page.writes, [`speedle_locale=${locale}; Domain=speedle.io; Path=/; Max-Age=31536000; SameSite=Lax; Secure`]);
    assert.deepEqual(page.redirects, []);
  }
});

test('explicit localized and documentation URLs remain stable without changing the choice', () => {
  for (const pathname of ['/ja/', '/zh/docs/', '/docs/', '/patterns/']) {
    const page = run({pathname, cookie: 'speedle_locale=es'});
    assert.deepEqual(page.redirects, []);
    assert.deepEqual(page.writes, []);
  }
});

test('English, invalid and malformed choices never redirect', () => {
  for (const cookie of ['', 'speedle_locale=en', 'speedle_locale=fr', 'speedle_locale=%2Fja%2F', 'speedle_locale=ja=es']) {
    const page = run({cookie});
    assert.deepEqual(page.redirects, []);
    page.select('fr');
    assert.deepEqual(page.writes, []);
  }
});

test('blocked cookies leave selection and page rendering functional', () => {
  const page = run({blocked: true});
  assert.doesNotThrow(() => page.select('ja'));
  assert.deepEqual(page.redirects, []);
  assert.deepEqual(page.writes, []);
});

test('preview, staging and insecure hosts never consume or write production preferences', () => {
  for (const options of [{hostname: 'localhost'}, {hostname: 'b.speedle.io'}, {hostname: 'g.speedle.io'}, {hostname: 'preview.example'}, {protocol: 'http:'}]) {
    const page = run({...options, cookie: 'speedle_locale=ja'});
    page.select('es');
    assert.deepEqual(page.redirects, []);
    assert.deepEqual(page.writes, []);
  }
});

test('middle-click saves an explicit choice without handling right-click', () => {
  const page = run({pathname: '/ja/'});
  page.select('en', 'auxclick', 1);
  assert.equal(page.writes.length, 1);
  assert.ok(page.writes[0].startsWith('speedle_locale=en;'));
  page.select('es', 'auxclick', 2);
  page.select('es', 'click', 2);
  assert.equal(page.writes.length, 1);
});
