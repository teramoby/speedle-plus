import assert from 'node:assert/strict';
import test from 'node:test';
import {
  editableJSON, formToPolicy, formToRolePolicy, matches, parseJSON, policyToForm, principalsToText, rolePolicyToForm,
  splitList, textToPrincipals,
} from '../static/policy-model.js';

const policy = {
  id: 'p1', name: 'readers', effect: 'grant',
  principals: [['user:alice'], ['group:ops', 'role:oncall'], ['idd=corp:user:bob']],
  permissions: [{ resource: '/books', actions: ['get', 'list'] }, { resourceExpression: '/books/.*', actions: ['get'] }],
  condition: 'level > 1',
  metadata: { createby: 'admin' },
};

test('a policy survives the form', () => {
  const form = policyToForm(policy);
  assert.equal(form.principalsText, 'user:alice\ngroup:ops, role:oncall\nidd=corp:user:bob');
  const { ok, policy: back, errors } = formToPolicy(form);
  assert.ok(ok, JSON.stringify(errors));
  const { metadata, ...rest } = policy;
  assert.deepEqual(back, rest);
});

test('an empty policy form has one permission row and no ID', () => {
  const form = policyToForm();
  assert.equal(form.permissions.length, 1);
  const { ok, policy: p, errors } = formToPolicy({ ...form, name: ' new ', permissions: [{ kind: 'resource', value: 'r', actions: 'get' }] });
  assert.ok(ok, JSON.stringify(errors));
  assert.deepEqual(p, { name: 'new', effect: 'grant', permissions: [{ resource: 'r', actions: ['get'] }] });
});

test('policy errors name their fields', () => {
  const paths = (form) => formToPolicy({ ...policyToForm(policy), ...form }).errors.map((e) => e.path);
  assert.deepEqual(paths({ name: ' ' }), ['name']);
  assert.deepEqual(paths({ effect: 'allow' }), ['effect']);
  assert.deepEqual(paths({ principalsText: 'alice\nuser:bob, ops' }), ['principals.0', 'principals.2']);
  assert.deepEqual(paths({ permissions: [] }), ['permissions']);
  assert.deepEqual(paths({ permissions: [{ kind: 'resource', value: '', actions: '' }] }), ['permissions']);
  assert.deepEqual(paths({ permissions: [{ kind: 'resource', value: 'r', actions: ' , ' }, { kind: 'expression', value: '', actions: 'get' }] }),
    ['permissions.0.actions', 'permissions.1.value']);
});

test('principal lists split by lines and commas', () => {
  assert.deepEqual(textToPrincipals(' user:a , group:b \n\n role:c '), [['user:a', 'group:b'], ['role:c']]);
  assert.equal(principalsToText([['a', 'b'], ['c']]), 'a, b\nc');
  assert.deepEqual(splitList('a,, b\nc'), ['a', 'b', 'c']);
  assert.deepEqual(splitList(undefined), []);
});

const rolePolicy = {
  id: 'rp1', name: 'admins', effect: 'deny', roles: ['admin', 'auditor'], principals: ['user:alice', 'role:manager'],
  resources: ['/books'], resourceExpressions: ['/docs/.*'], condition: 'x == 1',
};

test('a role policy survives the form', () => {
  const { ok, policy: back, errors } = formToRolePolicy(rolePolicyToForm(rolePolicy));
  assert.ok(ok, JSON.stringify(errors));
  assert.deepEqual(back, rolePolicy);
  const minimal = formToRolePolicy({ ...rolePolicyToForm(), name: 'r', roles: 'admin' });
  assert.deepEqual(minimal.policy, { name: 'r', effect: 'grant', roles: ['admin'] });
});

test('role policy errors name their fields', () => {
  const paths = (form) => formToRolePolicy({ ...rolePolicyToForm(rolePolicy), ...form }).errors.map((e) => e.path);
  assert.deepEqual(paths({ name: '', effect: 'x' }), ['name', 'effect']);
  assert.deepEqual(paths({ roles: '' }), ['roles']);
  assert.deepEqual(paths({ roles: 'admin\nrole:x' }), ['roles.1']);
  assert.deepEqual(paths({ principals: 'user:a\nbob' }), ['principals.1']);
});

test('the JSON view hides metadata and keeps the path ID', () => {
  assert.ok(!editableJSON(policy).includes('createby'));
  assert.equal(editableJSON(undefined), '{}');
  assert.deepEqual(parseJSON('{"id":"other","name":"n","metadata":{}}', 'p1'), { ok: true, value: { id: 'p1', name: 'n' } });
  assert.deepEqual(parseJSON('{"id":"other","name":"n"}', ''), { ok: true, value: { name: 'n' } });
  for (const bad of ['{', '[]', 'null', '"x"']) assert.equal(parseJSON(bad, 'p1').ok, false, bad);
});

test('matches searches the whole item, ignoring case', () => {
  assert.ok(matches(policy, ''));
  assert.ok(matches(policy, 'ONCALL'));
  assert.ok(!matches(policy, 'nobody'));
});
