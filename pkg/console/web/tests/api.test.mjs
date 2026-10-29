import assert from 'node:assert/strict';
import test from 'node:test';
import { ApiError, createApi } from '../static/api.js';

/** A fetch that records calls and answers from replies. */
function fakeFetch(replies) {
  const calls = [];
  const fetch = async (url, init) => {
    calls.push({ url, ...init });
    const reply = typeof replies === 'function' ? replies(url, init) : replies.shift();
    if (reply instanceof Error) throw reply;
    const { status = 200, body = '', etag = null, statusText = '' } = reply;
    return {
      ok: status >= 200 && status < 300, status, statusText,
      headers: { get: (h) => (h === 'ETag' ? etag : null) },
      text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
    };
  };
  return { fetch, calls };
}

test('unsafe calls carry the CSRF token, safe ones do not', async () => {
  const { fetch, calls } = fakeFetch([
    { body: { csrfToken: 'tok', user: 'alice' } },
    { body: [] },
    { status: 201, body: { name: 'app' } },
  ]);
  const api = createApi({ fetch });
  assert.equal((await api.login('secret', 'bearer')).user, 'alice');
  assert.deepEqual(JSON.parse(calls[0].body), { token: 'secret', tokenType: 'bearer' });
  await api.services();
  assert.equal(calls[1].headers['X-CSRF-Token'], undefined);
  assert.equal(calls[1].credentials, 'same-origin');
  await api.createService('app', 'application');
  assert.equal(calls[2].headers['X-CSRF-Token'], 'tok');
  assert.equal(calls[2].headers['Content-Type'], 'application/json');
});

test('paths are escaped and updates send If-Match', async () => {
  const { fetch, calls } = fakeFetch(() => ({ body: { id: 'p/1' }, etag: '"v2"' }));
  const api = createApi({ fetch, base: '/api' });
  api.setCsrf('c');
  const saved = await api.updatePolicy('a/b c', { id: 'p/1', name: 'n' }, '"v1"');
  assert.equal(saved.etag, '"v2"');
  assert.equal(calls[0].url, '/api/pms/service/a%2Fb%20c/policy/p%2F1');
  assert.equal(calls[0].method, 'PUT');
  assert.equal(calls[0].headers['If-Match'], '"v1"');
  await api.policies('app', 'name eq x');
  assert.equal(calls[1].url, '/api/pms/service/app/policy?filter=name%20eq%20x');
  for (const call of [
    () => api.service('s'), () => api.policyCounts(), () => api.deleteService('s'),
    () => api.policy('s', 'i'), () => api.createPolicy('s', {}), () => api.deletePolicy('s', 'i'),
    () => api.rolePolicies('s'), () => api.rolePolicies('s', 'f'), () => api.rolePolicy('s', 'i'), () => api.createRolePolicy('s', {}),
    () => api.updateRolePolicy('s', { id: 'i' }, ''), () => api.deleteRolePolicy('s', 'i'),
    () => api.isAllowed({}), () => api.grantedRoles({}), () => api.grantedPermissions({}), () => api.diagnose({}),
    () => api.config(), () => api.me(), () => api.policies('s'),
  ]) await call();
  assert.equal(calls.length, 21);
});

test('empty lists come back as arrays and objects', async () => {
  const { fetch } = fakeFetch(() => ({ body: '' }));
  const api = createApi({ fetch });
  assert.deepEqual(await api.services(), []);
  assert.deepEqual(await api.policyCounts(), {});
  assert.deepEqual(await api.policies('s'), []);
});

test('errors carry the status and the server message', async () => {
  let unauthorized = 0;
  const { fetch } = fakeFetch([
    { status: 404, body: { error: 'SPDL-1002 not found' } },
    { status: 500, body: 'not json', statusText: 'Internal Server Error' },
    { status: 401, body: { error: 'Sign in' } },
    new TypeError('network down'),
    { status: 401, body: { error: 'bad token' } },
    { status: 204 },
  ]);
  const api = createApi({ fetch, onUnauthorized: () => { unauthorized++; } });
  await assert.rejects(api.service('x'), (e) => e instanceof ApiError && e.status === 404 && e.message === 'SPDL-1002 not found');
  await assert.rejects(api.service('x'), (e) => e.status === 500 && e.message === 'Internal Server Error');
  await assert.rejects(api.services(), (e) => e.status === 401);
  await assert.rejects(api.services(), (e) => e.status === 0);
  await assert.rejects(api.login('bad'), (e) => e.status === 401);
  await new Promise((r) => setTimeout(r, 5));
  assert.equal(unauthorized, 1, 'only a lapsed session signs the page out');
  await api.logout();
});
