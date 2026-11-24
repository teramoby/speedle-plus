const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const source = fs.readFileSync(path.join(__dirname, '../../../docs/js/navigation.js'), 'utf8');

function run({cookie = '', pathname = '/', hostname = 'engine.speedle.io', protocol = 'https:', blocked = false, lang = 'en'} = {}) {
  const writes = [], redirects = [], listeners = {}, windowListeners = {};
  const jar = new Map(cookie.split(';').filter(Boolean).map(part => [part.trim().slice(0, part.trim().indexOf('=')), part.trim().slice(part.trim().indexOf('=') + 1)]));
  const local = new Map([['speedle-cloud.locale', 'ja']]);
  const session = new Map([['speedle-cloud.locale', 'ko']]);
  const element = () => ({children: [], handlers: {}, append(...children) { this.children.push(...children); }, setAttribute() {}, addEventListener(name, handler) { this.handlers[name] = handler; }});
  const panel = element();
  const document = {
    documentElement: {lang},
    querySelectorAll: selector => selector === '.languages-panel' ? [panel] : [],
    createElement: element,
    addEventListener: (name, fn) => { listeners[name] = fn; },
    get cookie() { if (blocked) throw new Error('blocked'); return [...jar].map(([name, value]) => `${name}=${value}`).join('; '); },
    set cookie(value) {
      if (blocked) throw new Error('blocked');
      writes.push(value);
      const [nameValue, ...attributes] = value.split(';');
      const [name, data] = nameValue.split('=');
      if (attributes.some(a => a.trim() === 'Max-Age=0')) jar.delete(name);
      else jar.set(name, data);
    },
  };
  vm.runInNewContext(source, {
    document,
    window: {localStorage: {removeItem: name => local.delete(name)}, sessionStorage: {removeItem: name => session.delete(name)}, addEventListener: (name, fn) => { windowListeners[name] = fn; }},
    location: {hostname, protocol, pathname, search: '?from=products', hash: '#overview', replace: value => redirects.push(value)},
  });
  const [label, clearButton, status] = panel.children[0].children;
  const checkbox = label.children[0];
  function select(locale, type = 'click', button = 0) {
    const link = {getAttribute: () => locale};
    listeners[type]?.({type, button, target: {closest: selector => selector.includes('a[hreflang]') ? link : null}});
  }
  function toggle(value) { checkbox.checked = value; checkbox.handlers.change(); }
  return {writes, redirects, select, toggle, checkbox, status, jar, local, session, focus: () => windowListeners.focus(), clear: () => clearButton.handlers.click()};
}

for (const [locale, prefix] of [['ja', 'ja'], ['ko', 'ko'], ['es', 'es'], ['zh-CN', 'zh']]) {
  test(`shared session ${locale} choice applies on neutral entry and keeps query/fragment`, () => {
    const page = run({cookie: `session=unrelated; speedle_locale=${locale}`});
    assert.deepEqual(page.redirects, [`/${prefix}/?from=products#overview`]);
    assert.ok(page.writes.every(w => !w.includes('Max-Age=31536000')));
  });
}

test('all five language selections are session-only without opt-in', () => {
  for (const locale of ['en', 'ja', 'ko', 'es', 'zh-CN']) {
    const page = run({pathname: '/ja/'});
    assert.equal(page.checkbox.checked, false);
    page.select(locale);
    const saved = page.writes.find(w => w.startsWith('speedle_locale='));
    assert.equal(saved, `speedle_locale=${locale}; Domain=speedle.io; Path=/; SameSite=Lax; Secure`);
    assert.equal(page.jar.has('speedle_locale_remember'), false);
    assert.deepEqual(page.redirects, []);
  }
});

test('explicit checking alone permits persistent language and consent cookies', () => {
  const page = run({pathname: '/ja/', lang: 'ja'});
  page.toggle(true);
  assert.equal(page.checkbox.checked, true);
  for (const name of ['speedle_locale', 'speedle_locale_remember']) {
    const saved = page.writes.find(w => w.startsWith(name + '='));
    assert.ok(saved.includes('Max-Age=31536000'));
    assert.ok(saved.includes('Domain=speedle.io; Path=/; SameSite=Lax; Secure'));
  }
  page.select('es');
  assert.equal(page.jar.get('speedle_locale'), 'es');
  assert.ok(page.writes.at(-1).includes('Max-Age=31536000'));
});

test('ordinary rendering with existing opt-in does not renew long-term cookies', () => {
  const page = run({pathname: '/ja/', cookie: 'speedle_locale=ja; speedle_locale_remember=1'});
  assert.equal(page.checkbox.checked, true);
  assert.deepEqual(page.writes, []);
});

test('unchecking removes consent and demotes current language to session', () => {
  const page = run({pathname: '/ja/', lang: 'ja', cookie: 'speedle_locale=ja; speedle_locale_remember=1'});
  page.toggle(false);
  assert.equal(page.jar.has('speedle_locale_remember'), false);
  assert.equal(page.jar.get('speedle_locale'), 'ja');
  assert.equal(page.checkbox.checked, false);
  assert.ok(!page.writes.at(-1).includes('Max-Age'));
  assert.equal(page.local.size, 0);
  assert.equal(page.session.size, 0);
});

test('clear deletes both cookies and legacy storage without navigating or recreating values', () => {
  const page = run({pathname: '/ja/', cookie: 'speedle_locale=ja; speedle_locale_remember=1'});
  page.clear();
  assert.equal(page.jar.size, 0);
  assert.equal(page.local.size, 0);
  assert.equal(page.session.size, 0);
  assert.equal(page.checkbox.checked, false);
  assert.ok(page.status.textContent);
  assert.deepEqual(page.redirects, []);
});

test('stale checked tabs honor withdrawal elsewhere instead of granting consent again', () => {
  const page = run({pathname: '/ja/', cookie: 'speedle_locale=ja; speedle_locale_remember=1'});
  page.jar.delete('speedle_locale_remember');
  page.select('en');
  assert.equal(page.jar.get('speedle_locale'), 'en');
  assert.equal(page.jar.has('speedle_locale_remember'), false);
  assert.ok(!page.writes.at(-1).includes('Max-Age'));
  page.focus();
  assert.equal(page.checkbox.checked, false);
});

test('unconsented legacy language storage is purged and cannot grant persistence', () => {
  const page = run({pathname: '/ja/', cookie: 'speedle_locale=ja; speedle_locale_remember=yes'});
  assert.equal(page.local.size, 0);
  assert.equal(page.session.size, 0);
  assert.equal(page.checkbox.checked, false);
  assert.ok(page.writes.every(w => !w.includes('Max-Age=31536000')));
});

test('explicit localized and documentation URLs remain stable', () => {
  for (const pathname of ['/ja/', '/zh/docs/', '/docs/', '/patterns/']) {
    assert.deepEqual(run({pathname, cookie: 'speedle_locale=es'}).redirects, []);
  }
});

test('English, invalid and malformed choices never redirect or save invalid locale', () => {
  for (const cookie of ['', 'speedle_locale=en', 'speedle_locale=fr', 'speedle_locale=%2Fja%2F', 'speedle_locale=ja=es']) {
    const page = run({cookie});
    assert.deepEqual(page.redirects, []);
    const before = page.writes.length;
    page.select('fr');
    assert.equal(page.writes.length, before);
  }
});

test('blocked cookies leave controls and ordinary language navigation functional', () => {
  const page = run({blocked: true});
  assert.doesNotThrow(() => { page.select('ja'); page.toggle(true); page.clear(); });
  assert.equal(page.checkbox.checked, false);
  assert.deepEqual(page.redirects, []);
  assert.deepEqual(page.writes, []);
});

test('preview, staging and insecure hosts disable production persistence', () => {
  for (const options of [{hostname: 'localhost'}, {hostname: 'b.speedle.io'}, {hostname: 'g.speedle.io'}, {hostname: 'preview.example'}, {protocol: 'http:'}]) {
    const page = run({...options, cookie: 'speedle_locale=ja; speedle_locale_remember=1'});
    assert.equal(page.checkbox.disabled, true);
    page.select('es');
    assert.deepEqual(page.redirects, []);
    assert.deepEqual(page.writes, []);
  }
});

test('middle-click saves a session choice, while right-click stays inert', () => {
  const page = run({pathname: '/ja/'});
  page.select('en', 'auxclick', 1);
  assert.equal(page.jar.get('speedle_locale'), 'en');
  assert.ok(!page.writes.at(-1).includes('Max-Age'));
  const before = page.writes.length;
  page.select('es', 'auxclick', 2);
  page.select('es', 'click', 2);
  assert.equal(page.writes.length, before);
});
