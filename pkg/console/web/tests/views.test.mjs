import assert from 'node:assert/strict';
import test from 'node:test';
import { installDOM, settle, type } from './dom-shim.mjs';

const doc = installDOM();
const { h, mount } = await import('../static/dom.js');
const { setLocale } = await import('../static/i18n.js');
const { parseRoute, resolve } = await import('../static/app.js');
const { loginView } = await import('../static/views/login.js');
const { servicesView } = await import('../static/views/services.js');
const { listView } = await import('../static/views/service.js');
const { editorView } = await import('../static/views/editor.js');

/** A console API double that records calls. */
function fakeApi(overrides = {}) {
  const calls = [];
  const record = (name, result) => async (...args) => { calls.push([name, ...args]); return typeof result === 'function' ? result(...args) : result; };
  const policy = { id: 'p1', name: 'readers', effect: 'grant', principals: [['user:alice']], permissions: [{ resource: 'r', actions: ['get'] }], metadata: { createby: 'admin' } };
  return {
    calls,
    login: record('login', { user: 'alice', csrfToken: 't', principals: ['user:alice'], access: { read: true, write: true } }),
    services: record('services', [{ name: 'b-app', type: 'application' }, { name: 'a-app', type: 'application' }]),
    policyCounts: record('policyCounts', { 'a-app': { policycount: 2, rolePolicycount: 1 } }),
    createService: record('createService', {}),
    deleteService: record('deleteService', {}),
    policies: record('policies', [policy]),
    rolePolicies: record('rolePolicies', [{ id: 'rp1', name: 'admins', effect: 'grant', roles: ['admin'] }]),
    policy: record('policy', { data: policy, etag: '"v1"' }),
    updatePolicy: record('updatePolicy', (s, p) => ({ data: { ...p, metadata: {} }, etag: '"v2"' })),
    createPolicy: record('createPolicy', (s, p) => ({ data: { ...p, id: 'new1' } })),
    deletePolicy: record('deletePolicy', {}),
    ...overrides,
  };
}

function context(api, write = true) {
  const notes = [];
  const ctx = {
    api, config: { authzService: 'speedle-console' }, me: { access: { read: true, write } },
    notify: (m) => notes.push(m), navigate: (hash) => { ctx.went = hash; }, onLogin: (me) => { ctx.loggedIn = me; },
  };
  return { ctx, notes };
}

const labelled = (root) => {
  for (const control of root.querySelectorAll('input, select, textarea')) {
    if (control.getAttribute('aria-label')) continue;
    assert.ok(control.id, `a ${control.tagName} has no id`);
    assert.ok(root.querySelector(`label[for="${control.id}"]`), `no label for ${control.id}`);
  }
};

test('text is never parsed as markup', () => {
  const el = h('p', { text: '<img src=x onerror=alert(1)>' });
  assert.equal(el.children.length, 0);
  assert.equal(el.textContent, '<img src=x onerror=alert(1)>');
  const box = mount(h('div'), 'a', null, ['b', 2], h('span', {}, '<b>'));
  assert.equal(box.textContent, 'ab2<b>');
});

test('routes', () => {
  assert.deepEqual(parseRoute('#/services/a%2Fb/policies/p%1'), ['services', 'a/b', 'policies', 'p%1']);
  assert.deepEqual(resolve(parseRoute('')), { view: 'services' });
  assert.deepEqual(resolve(['services', 'app']), { view: 'list', service: 'app', kind: 'policies' });
  assert.deepEqual(resolve(['services', 'app', 'role-policies']), { view: 'list', service: 'app', kind: 'role-policies' });
  assert.deepEqual(resolve(['services', 'app', 'policies', 'new']), { view: 'editor', service: 'app', kind: 'policies', id: 'new' });
  assert.deepEqual(resolve(['services', 'app', 'roles']), { view: 'roles', service: 'app', role: '' });
  assert.deepEqual(resolve(['services', 'app', 'roles', 'admin']), { view: 'roles', service: 'app', role: 'admin' });
  assert.deepEqual(resolve(['diagnose']), { view: 'diagnose' });
});

test('login form signs in and reports a rejected token', async () => {
  setLocale('en');
  const api = fakeApi();
  const { ctx } = context(api);
  const form = loginView(ctx);
  labelled(form);
  type(form.querySelector('#login-token'), 'secret');
  form.dispatchEvent({ type: 'submit' });
  await settle();
  assert.deepEqual(api.calls[0], ['login', 'secret', '']);
  assert.equal(ctx.loggedIn.user, 'alice');

  api.login = async () => { const e = new Error('x'); e.status = 401; throw e; };
  form.dispatchEvent({ type: 'submit' });
  await settle();
  assert.match(form.textContent, /The token was not accepted/);
});

test('services are listed, filtered and created', async () => {
  const api = fakeApi();
  const { ctx } = context(api);
  const main = h('main');
  await servicesView(ctx, main);
  labelled(main);
  const names = main.querySelectorAll('tbody a').map((a) => a.textContent);
  assert.deepEqual(names, ['a-app', 'b-app']);
  assert.match(main.querySelector('tbody').textContent, /a-app.*2.*1/);
  type(main.querySelector('input[type="search"]'), 'b-');
  assert.deepEqual(main.querySelectorAll('tbody a').map((a) => a.textContent), ['b-app']);

  type(main.querySelector('#new-service-name'), ' c-app ');
  main.querySelector('form').dispatchEvent({ type: 'submit' });
  await settle();
  assert.deepEqual(api.calls.at(-1), ['createService', 'c-app', 'application']);
  assert.equal(ctx.went, '#/services/c-app');
});

test('deleting a service needs its name typed', async () => {
  const api = fakeApi();
  const { ctx } = context(api);
  const main = h('main');
  await servicesView(ctx, main);
  type(main.querySelector('input[type="search"]'), '');
  main.querySelectorAll('tbody button')[0].dispatchEvent({ type: 'click' });
  const confirm = main.querySelector('form.confirm');
  const submit = confirm.querySelector('button[type="submit"]');
  assert.equal(submit.disabled, true);
  type(confirm.querySelector('input'), 'a-ap');
  assert.equal(submit.disabled, true);
  confirm.dispatchEvent({ type: 'submit' });
  type(confirm.querySelector('input'), 'a-app');
  assert.equal(submit.disabled, false);
  confirm.dispatchEvent({ type: 'submit' });
  await settle();
  assert.deepEqual(api.calls.filter((c) => c[0] === 'deleteService'), [['deleteService', 'a-app']]);
});

test('read-only users see no write controls', async () => {
  const { ctx } = context(fakeApi(), false);
  const main = h('main');
  await servicesView(ctx, main);
  assert.equal(main.querySelectorAll('tbody button').length, 0);
  assert.equal(main.querySelector('#new-service-name'), null);
  await listView(ctx, main, 'app', 'policies');
  assert.equal(main.querySelector('a.btn.primary'), null);
  await editorView(ctx, main, 'app', 'policies', 'p1');
  for (const el of main.querySelectorAll('form input, form select, form textarea')) assert.equal(el.disabled, true, el.id);
});

test('policy and role policy lists', async () => {
  const api = fakeApi();
  const { ctx } = context(api);
  const main = h('main');
  await listView(ctx, main, 'app', 'policies');
  assert.match(main.textContent, /readers.*grant.*user:alice.*r: get/);
  await listView(ctx, main, 'app', 'role-policies');
  assert.match(main.textContent, /admins.*admin.*everyone.*all/);
  api.policies = async () => { const e = new Error('gone'); e.status = 404; throw e; };
  await listView(ctx, main, 'app', 'policies');
  assert.match(main.querySelector('[role="alert"]').textContent, /gone/);
});

test('the editor updates with If-Match and validates first', async () => {
  const api = fakeApi();
  const { ctx, notes } = context(api);
  const main = h('main');
  await editorView(ctx, main, 'app', 'policies', 'p1');
  labelled(main);
  const form = main.querySelector('form');
  type(main.querySelector('#f-name'), '');
  form.dispatchEvent({ type: 'submit' });
  await settle();
  assert.equal(main.querySelector('#f-name').getAttribute('aria-invalid'), 'true');
  assert.equal(doc.activeElement, main.querySelector('#f-name'));
  assert.ok(!api.calls.some((c) => c[0] === 'updatePolicy'));

  type(main.querySelector('#f-name'), 'renamed');
  form.dispatchEvent({ type: 'submit' });
  await settle();
  const update = api.calls.find((c) => c[0] === 'updatePolicy');
  assert.equal(update[1], 'app');
  assert.equal(update[2].id, 'p1');
  assert.equal(update[2].name, 'renamed');
  assert.equal(update[3], '"v1"');
  assert.deepEqual(notes, ['Saved renamed.']);

  assert.equal(api.calls.filter((c) => c[0] === 'policy').length, 2, 'the editor reloads after saving');
  api.updatePolicy = async () => { const e = new Error('changed'); e.status = 412; throw e; };
  main.querySelector('form').dispatchEvent({ type: 'submit' });
  await settle();
  assert.match(main.querySelector('[role="alert"]').textContent, /Someone changed this item/);
});

test('the editor switches between form and JSON', async () => {
  const api = fakeApi();
  const { ctx } = context(api);
  const main = h('main');
  await editorView(ctx, main, 'speedle-console', 'policies', 'new');
  assert.match(main.textContent, /lock you out/);
  const toggle = main.querySelectorAll('button').find((b) => b.textContent === 'Edit as JSON');
  type(main.querySelector('#f-name'), 'p');
  type(main.querySelector('#perm-0-value'), 'r');
  type(main.querySelector('#perm-0-actions'), 'get');
  toggle.dispatchEvent({ type: 'click' });
  const json = main.querySelector('#f-json');
  assert.deepEqual(JSON.parse(json.value), { name: 'p', effect: 'grant', permissions: [{ resource: 'r', actions: ['get'] }] });
  type(json, '{');
  toggle.dispatchEvent({ type: 'click' });
  assert.equal(json.getAttribute('aria-invalid'), 'true');
  type(json, '{"name":"q","effect":"deny","permissions":[{"resource":"r","actions":["put"]}]}');
  main.querySelector('form').dispatchEvent({ type: 'submit' });
  await settle();
  assert.deepEqual(api.calls.find((c) => c[0] === 'createPolicy')[2].name, 'q');
  assert.equal(ctx.went, '#/services/speedle-console/policies/new1');
});
