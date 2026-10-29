/** Calls the console API, which forwards to PMS and ADS. */
export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

const enc = encodeURIComponent;

/**
 * onUnauthorized runs, after the failing call has been handled, whenever the
 * session has ended, so the page can show the sign-in form.
 */
export function createApi({ fetch = globalThis.fetch.bind(globalThis), base = '/console/api', onUnauthorized } = {}) {
  let csrf = '';

  async function request(method, path, { body, ifMatch } = {}) {
    const headers = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (method !== 'GET' && method !== 'HEAD' && csrf) headers['X-CSRF-Token'] = csrf;
    if (ifMatch) headers['If-Match'] = ifMatch;
    let resp;
    try {
      resp = await fetch(base + path, {
        method, headers, credentials: 'same-origin',
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (e) {
      throw new ApiError(0, 'The console is unreachable.');
    }
    const text = await resp.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch { data = null; }
    }
    if (!resp.ok) {
      if (resp.status === 401 && path !== '/login' && path !== '/me' && onUnauthorized) setTimeout(onUnauthorized, 0);
      const message = (data && (data.error || data.errorMessage)) || resp.statusText || 'Request failed';
      throw new ApiError(resp.status, message);
    }
    return { data, etag: resp.headers.get('ETag') || '' };
  }
  const get = async (path) => (await request('GET', path)).data;
  const svc = (s) => `/pms/service/${enc(s)}`;

  return {
    setCsrf(token) { csrf = token || ''; },
    config: () => get('/config'),
    async login(token, tokenType) {
      const me = (await request('POST', '/login', { body: { token, tokenType } })).data;
      csrf = me.csrfToken;
      return me;
    },
    async logout() { await request('POST', '/logout'); csrf = ''; },
    async me() {
      const me = await get('/me');
      csrf = me.csrfToken;
      return me;
    },

    services: async () => (await get('/pms/service')) || [],
    service: (s) => get(svc(s)),
    policyCounts: async () => (await get('/pms/policy-counts')) || {},
    createService: (name, type) => request('POST', '/pms/service', { body: { name, type } }),
    deleteService: (s) => request('DELETE', svc(s)),

    policies: async (s, filter) => (await get(`${svc(s)}/policy${filter ? '?filter=' + enc(filter) : ''}`)) || [],
    policy: (s, id) => request('GET', `${svc(s)}/policy/${enc(id)}`),
    createPolicy: (s, p) => request('POST', `${svc(s)}/policy`, { body: p }),
    updatePolicy: (s, p, etag) => request('PUT', `${svc(s)}/policy/${enc(p.id)}`, { body: p, ifMatch: etag }),
    deletePolicy: (s, id) => request('DELETE', `${svc(s)}/policy/${enc(id)}`),

    rolePolicies: async (s, filter) => (await get(`${svc(s)}/role-policy${filter ? '?filter=' + enc(filter) : ''}`)) || [],
    rolePolicy: (s, id) => request('GET', `${svc(s)}/role-policy/${enc(id)}`),
    createRolePolicy: (s, p) => request('POST', `${svc(s)}/role-policy`, { body: p }),
    updateRolePolicy: (s, p, etag) => request('PUT', `${svc(s)}/role-policy/${enc(p.id)}`, { body: p, ifMatch: etag }),
    deleteRolePolicy: (s, id) => request('DELETE', `${svc(s)}/role-policy/${enc(id)}`),

    isAllowed: async (req) => (await request('POST', '/ads/is-allowed', { body: req })).data,
    grantedRoles: async (req) => (await request('POST', '/ads/all-granted-roles', { body: req })).data,
    grantedPermissions: async (req) => (await request('POST', '/ads/all-granted-permissions', { body: req })).data,
    diagnose: async (req) => (await request('POST', '/ads/diagnose', { body: req })).data,
  };
}
