import assert from 'node:assert/strict';
import test from 'node:test';
import { installDOM, settle, type } from './dom-shim.mjs';

installDOM();
const { h } = await import('../static/dom.js');
const { setLocale } = await import('../static/i18n.js');
const { rolesView, MAX_DRAWN_NODES } = await import('../static/views/roles.js');
const { buildRequest, diagnoseView, parseAttributes } = await import('../static/views/diagnose.js');

const graph = {
  service: 'app',
  nodes: [
    { id: '*', kind: 'everyone', name: '*' },
    { id: 'role:manager', kind: 'role', name: 'manager', inCycle: true },
    { id: 'role:reader', kind: 'role', name: 'reader', inCycle: true },
    { id: 'role:ghost', kind: 'role', name: 'ghost', undefined: true },
    { id: 'role:spare', kind: 'role', name: 'spare', unused: true },
    { id: 'user:alice', kind: 'principal', name: 'user:alice' },
  ],
  edges: [
    { from: 'user:alice', to: 'role:manager', effect: 'grant', service: 'app', policyId: 'rp1', policyName: 'm' },
    { from: 'role:manager', to: 'role:reader', effect: 'grant', service: 'app', policyId: 'rp2', policyName: 'r', condition: 'x > 1' },
    { from: 'role:reader', to: 'role:manager', effect: 'grant', service: 'app', policyId: 'rp3' },
    { from: '*', to: 'role:spare', effect: 'deny', service: 'global', policyId: 'rp4', policyName: 'no-spare' },
  ],
  cycles: [['role:manager', 'role:reader']],
  references: {},
};

const holders = {
  role: 'reader',
  holders: [{ principal: 'user:alice', conditional: true, path: ['user:alice', 'role:manager', 'role:reader'] }, { principal: '*', conditional: false, path: ['*', 'role:reader'] }],
  denials: [{ from: '*', to: 'role:reader', effect: 'deny', service: 'global', policyId: 'rp9', policyName: 'no' }],
  references: [{ id: 'p1', name: 'read', effect: 'grant' }],
};

function fakeApi(over = {}) {
  const calls = [];
  const rec = (name, result) => async (...args) => { calls.push([name, ...args]); if (result instanceof Error) throw result; return result; };
  return {
    calls,
    roleGraph: rec('roleGraph', graph),
    roleHolders: rec('roleHolders', holders),
    grantedRoles: rec('grantedRoles', ['manager']),
    services: rec('services', [{ name: 'app' }]),
    diagnose: rec('diagnose', {
      allowed: true, reason: 'GRANT_POLICY_FOUND', grantedRoles: ['reader'],
      rolePolicies: [{ id: 'rp1', name: 'm', effect: 'grant', roles: ['manager'], status: 'takeEffect', condition: { conditionExpression: 'x > 1', evaluationResult: 'true' } }],
      policies: [{ id: 'p1', name: 'read', effect: 'grant', status: 'conditionFailed' }],
    }),
    grantedPermissions: rec('grantedPermissions', [{ resource: '/books', actions: ['get'] }, { resourceExpression: '/docs/.*', actions: ['get'] }]),
    ...over,
  };
}

const context = (api) => ({ api, config: {}, me: { access: { read: true, write: false } } });

test('the role view draws the graph, lists issues and shows holders', async () => {
  setLocale('en');
  const api = fakeApi();
  const main = h('main');
  await rolesView(context(api), main, 'app', 'reader');
  assert.equal(main.querySelectorAll('g.node').length, 6);
  assert.equal(main.querySelectorAll('path.edge').length, 4);
  assert.equal(main.querySelectorAll('path.edge.deny').length, 1);
  assert.equal(main.querySelectorAll('path.edge.conditional').length, 1);
  assert.equal(main.querySelectorAll('g.node.cycle').length, 2);
  const text = main.textContent;
  assert.match(text, /Cycle: manager → reader → manager/);
  assert.match(text, /Role ghost is used but no role policy grants it/);
  assert.match(text, /Role spare is granted but no policy or role uses it/);
  assert.match(text, /Who holds role reader/);
  assert.match(text, /manager → reader.*Only when/);
  assert.match(text, /everyone/);
  assert.ok(main.querySelector('a[href="#/services/global/role-policies/rp9"]'));
  assert.ok(main.querySelector('a[href="#/services/app/policies/p1"]'));
  assert.equal(main.querySelector('g.node.selected').getAttribute('aria-label'), 'Role reader');

  // Clicking and pressing Enter select a role.
  const manager = main.querySelectorAll('g.node.role').find((g) => g.getAttribute('aria-label') === 'Role manager');
  manager.dispatchEvent({ type: 'click' });
  manager.dispatchEvent({ type: 'keydown', key: 'Enter' });
  manager.dispatchEvent({ type: 'keydown', key: 'x' });
  await settle();
  assert.equal(api.calls.filter((c) => c[0] === 'roleHolders').length, 3);

  // The subject panel asks ADS.
  type(main.querySelector('#roles-subject'), 'bad');
  main.querySelectorAll('form').at(-1).dispatchEvent({ type: 'submit' });
  await settle();
  assert.match(main.textContent, /Write subject principals/);
  type(main.querySelector('#roles-subject'), 'user:alice');
  type(main.querySelector('#roles-resource'), ' /books ');
  main.querySelectorAll('form').at(-1).dispatchEvent({ type: 'submit' });
  await settle();
  assert.deepEqual(api.calls.find((c) => c[0] === 'grantedRoles')[1],
    { subject: { principals: [{ type: 'user', name: 'alice' }] }, serviceName: 'app', resource: '/books' });

  // Leaving out the global service reloads the graph.
  const box = main.querySelector('#include-global');
  box.checked = false;
  box.dispatchEvent({ type: 'change' });
  await settle();
  assert.deepEqual(api.calls.filter((c) => c[0] === 'roleGraph').at(-1), ['roleGraph', 'app', false]);
});

test('large graphs are listed instead of drawn', async () => {
  const big = { ...graph, nodes: Array.from({ length: MAX_DRAWN_NODES + 1 }, (_, i) => ({ id: `role:r${i}`, kind: 'role', name: `r${i}` })),
    edges: [{ from: 'role:r0', to: 'role:r1', effect: 'deny', service: 'app', policyId: 'x' }], cycles: [] };
  const api = fakeApi({ roleGraph: async () => big });
  const main = h('main');
  await rolesView(context(api), main, 'app');
  assert.equal(main.querySelector('svg'), null);
  assert.equal(main.querySelectorAll('ul.role-list button').length, MAX_DRAWN_NODES + 1);
  main.querySelector('ul.role-list button').dispatchEvent({ type: 'click' });
  await settle();
  assert.match(main.textContent, /Who holds role reader/);
});

test('role view errors and empty services', async () => {
  const main = h('main');
  const err = Object.assign(new Error('Service "x" is not found.'), { status: 404 });
  await rolesView(context(fakeApi({ roleGraph: async () => { throw err; } })), main, 'x');
  assert.match(main.textContent, /not found/);
  await rolesView(context(fakeApi({ roleGraph: async () => ({ ...graph, nodes: [], edges: [], cycles: [] }), roleHolders: async () => { throw err; } })), main, 'x', 'r');
  assert.match(main.textContent, /no role policies/);
  assert.match(main.textContent, /No cycles/);
  assert.match(main.textContent, /not found/);
});

test('diagnosis requests are built from the form', () => {
  const form = { serviceName: ' app ', resource: ' /books ', action: 'get', principals: 'user:alice', attributes: '' };
  assert.deepEqual(buildRequest(form), { ok: true, request: { subject: { principals: [{ type: 'user', name: 'alice' }] }, serviceName: 'app', resource: '/books', action: 'get' } });
  assert.deepEqual(buildRequest({ ...form, attributes: '[{"name":"n","type":"numeric","value":0}]' }).request.attributes, [{ name: 'n', type: 'numeric', value: 0 }]);
  assert.equal(buildRequest({ ...form, serviceName: ' ' }).field, 'diag-service');
  assert.equal(buildRequest({ ...form, principals: 'role:x' }).field, 'diag-principals');
  assert.equal(buildRequest({ ...form, attributes: '{' }).field, 'diag-attributes');
  assert.equal(parseAttributes('[{"name":1}]').ok, false);
  assert.equal(parseAttributes('{}').ok, false);
});

test('the diagnose view shows the verdict and the policies that applied', async () => {
  const api = fakeApi();
  const ctx = context(api);
  const main = h('main');
  await diagnoseView(ctx, main);
  const form = main.querySelector('form');
  form.dispatchEvent({ type: 'submit' });
  await settle();
  assert.match(main.textContent, /Enter a service/);
  type(main.querySelector('#diag-service'), 'app');
  type(main.querySelector('#diag-resource'), '/books');
  type(main.querySelector('#diag-action'), 'get');
  type(main.querySelector('#diag-principals'), 'user:alice');
  form.dispatchEvent({ type: 'submit' });
  await settle();
  const text = main.textContent;
  assert.match(text, /Allowed/);
  assert.match(text, /GRANT_POLICY_FOUND/);
  assert.match(text, /Took effect.*x > 1 → true/);
  assert.match(text, /Condition false/);
  assert.ok(main.querySelector('a[href="#/services/app/roles/reader"]'));
  assert.ok(main.querySelector('a[href="#/services/app/role-policies/rp1"]'));

  main.querySelectorAll('button').find((b) => b.textContent === 'All granted permissions').dispatchEvent({ type: 'click' });
  await settle();
  assert.match(main.textContent, /\/books.*get.*~\/docs/);

  // The form is kept for the next visit, and failures are shown.
  api.diagnose = async () => { throw Object.assign(new Error('ADS down'), { status: 503 }); };
  await diagnoseView(ctx, main);
  assert.equal(main.querySelector('#diag-service').value, 'app');
  main.querySelector('form').dispatchEvent({ type: 'submit' });
  await settle();
  assert.match(main.textContent, /authorization service is unavailable/);

  api.diagnose = async () => ({ allowed: false, reason: 'NO_APPLICABLE_POLICIES' });
  api.grantedPermissions = async () => [];
  api.services = async () => { throw new Error('x'); };
  await diagnoseView(ctx, main);
  main.querySelector('form').dispatchEvent({ type: 'submit' });
  await settle();
  assert.match(main.textContent, /Denied.*No roles.*No role policy applied.*No authorization policy applied/);
  main.querySelectorAll('button').find((b) => b.textContent === 'All granted permissions').dispatchEvent({ type: 'click' });
  await settle();
  assert.match(main.textContent, /No permissions/);
});
