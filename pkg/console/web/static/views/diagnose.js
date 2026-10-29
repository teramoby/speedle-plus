import { h, mount } from '../dom.js';
import { N_, t } from '../i18n.js';
import { parsePrincipals } from '../principals.js';
import { alertBox, button, field, href, loading } from './common.js';

const STATUS = { takeEffect: N_('Took effect'), conditionFailed: N_('Condition false'), ignored: N_('Ignored') };

/** Parses the optional attributes, a JSON array of {name, type, value}. */
export function parseAttributes(text) {
  const trimmed = String(text ?? '').trim();
  if (!trimmed) return { ok: true, attributes: [] };
  let value;
  try { value = JSON.parse(trimmed); } catch { return { ok: false, message: 'Attributes must be a JSON array.' }; }
  if (!Array.isArray(value) || value.some((a) => !a || typeof a.name !== 'string' || typeof a.type !== 'string')) {
    return { ok: false, message: 'Each attribute needs a name, a type and a value.' };
  }
  return { ok: true, attributes: value };
}

/** Builds the ADS request from the form, or reports the first problem. */
export function buildRequest(form) {
  const { principals, errors } = parsePrincipals(form.principals);
  if (errors.length) return { ok: false, field: 'diag-principals', message: errors[0].message };
  if (!form.serviceName.trim()) return { ok: false, field: 'diag-service', message: 'Enter a service.' };
  const attrs = parseAttributes(form.attributes);
  if (!attrs.ok) return { ok: false, field: 'diag-attributes', message: attrs.message };
  const request = { subject: { principals }, serviceName: form.serviceName.trim(), resource: form.resource.trim(), action: form.action.trim() };
  if (attrs.attributes.length) request.attributes = attrs.attributes;
  return { ok: true, request };
}

function conditionText(c) {
  if (!c || !c.conditionExpression) return '';
  return `${c.conditionExpression} → ${c.evaluationResult || ''}`;
}

function rolePoliciesTable(service, rows) {
  if (!rows?.length) return h('p', { class: 'muted', text: t('No role policy applied.') });
  return h('table', { class: 'grid' },
    h('thead', {}, h('tr', {}, [N_('Name'), N_('Effect'), N_('Roles'), N_('Status'), N_('Condition')].map((c) => h('th', { text: t(c) })))),
    h('tbody', {}, rows.map((p) => h('tr', {},
      h('td', {}, h('a', { href: href('services', service, 'role-policies', p.id), text: p.name || p.id })),
      h('td', { text: p.effect }), h('td', { text: (p.roles || []).join(', ') }),
      h('td', { text: t(STATUS[p.status] || p.status || '') }), h('td', { text: conditionText(p.condition) })))));
}

function policiesTable(service, rows) {
  if (!rows?.length) return h('p', { class: 'muted', text: t('No authorization policy applied.') });
  return h('table', { class: 'grid' },
    h('thead', {}, h('tr', {}, [N_('Name'), N_('Effect'), N_('Status'), N_('Condition')].map((c) => h('th', { text: t(c) })))),
    h('tbody', {}, rows.map((p) => h('tr', {},
      h('td', {}, h('a', { href: href('services', service, 'policies', p.id), text: p.name || p.id })),
      h('td', { text: p.effect }), h('td', { text: t(STATUS[p.status] || p.status || '') }),
      h('td', { text: conditionText(p.condition) })))));
}

export function resultView(request, r) {
  return h('section', { class: 'card panel' },
    h('div', { class: `verdict ${r.allowed ? 'allowed' : 'denied'}`, role: 'status' }, r.allowed ? t('Allowed') : t('Denied')),
    h('p', {}, t('Reason'), ': ', h('code', { text: r.reason || '' })),
    h('h3', { text: t('Granted roles') }),
    r.grantedRoles?.length ? h('ul', { class: 'chips' }, r.grantedRoles.map((role) => h('li', {},
      h('a', { href: href('services', request.serviceName, 'roles') + '/' + encodeURIComponent(role), text: role }))))
      : h('p', { class: 'muted', text: t('No roles.') }),
    h('h3', { text: t('Role policies') }), rolePoliciesTable(request.serviceName, r.rolePolicies),
    h('h3', { text: t('Policies') }), policiesTable(request.serviceName, r.policies));
}

function permissionsView(perms) {
  return h('section', { class: 'card panel' }, h('h3', { text: t('All granted permissions') }),
    perms?.length ? h('table', { class: 'grid' },
      h('thead', {}, h('tr', {}, h('th', { text: t('Resource') }), h('th', { text: t('Actions') }))),
      h('tbody', {}, perms.map((p) => h('tr', {},
        h('td', { text: p.resource || (p.resourceExpression ? '~' + p.resourceExpression : '') }), h('td', { text: (p.actions || []).join(', ') })))))
      : h('p', { class: 'muted', text: t('No permissions.') }));
}

export async function diagnoseView(ctx, main) {
  mount(main, loading());
  let services = [];
  try { services = await ctx.api.services(); } catch { services = []; }
  const controls = {
    serviceName: h('input', { type: 'text', list: 'diag-services', autocomplete: 'off', value: ctx.lastDiagnose?.serviceName || '' }),
    resource: h('input', { type: 'text', autocomplete: 'off', value: ctx.lastDiagnose?.resource || '' }),
    action: h('input', { type: 'text', autocomplete: 'off', value: ctx.lastDiagnose?.action || '' }),
    principals: h('textarea', { rows: 3, spellcheck: 'false', placeholder: 'user:alice\ngroup:staff', value: ctx.lastDiagnose?.principals || '' }),
    attributes: h('textarea', { rows: 3, spellcheck: 'false', placeholder: '[{"name":"level","type":"numeric","value":3}]', value: ctx.lastDiagnose?.attributes || '' }),
  };
  const out = h('div', { 'aria-live': 'polite' });
  const read = () => {
    const form = Object.fromEntries(Object.entries(controls).map(([k, c]) => [k, c.value]));
    ctx.lastDiagnose = form;
    const built = buildRequest(form);
    if (!built.ok) {
      mount(out, h('p', { class: 'alert error', role: 'alert', text: t(built.message) }));
      main.querySelector(`#${built.field}`)?.focus();
      return null;
    }
    return built.request;
  };
  const run = async (fn) => {
    const request = read();
    if (!request) return;
    mount(out, loading());
    try { mount(out, await fn(request)); } catch (err) { mount(out, alertBox(err)); }
  };
  const form = h('form', { class: 'card', on: { submit: (e) => { e.preventDefault(); run(async (req) => resultView(req, await ctx.api.diagnose(req))); } } },
    h('datalist', { id: 'diag-services' }, services.map((s) => h('option', { value: s.name }))),
    h('div', { class: 'grid-form' },
      field('diag-service', 'Service', controls.serviceName, { required: true }),
      field('diag-resource', 'Resource', controls.resource),
      field('diag-action', 'Action', controls.action)),
    field('diag-principals', 'Subject principals', controls.principals, { hint: 'One per line, such as user:alice or idd=corp:group:staff. Leave empty for an anonymous subject.' }),
    field('diag-attributes', 'Attributes', controls.attributes, { hint: 'Optional JSON array for conditions.' }),
    h('div', { class: 'actions' },
      button('Diagnose', null, { kind: 'primary', type: 'submit' }),
      button('All granted permissions', () => run(async (req) => permissionsView(await ctx.api.grantedPermissions(req))))));
  mount(main,
    h('h1', { text: t('Diagnose a request') }),
    h('p', { class: 'muted', text: t('ADS evaluates the request with the policies it has loaded and shows which policies applied.') }),
    form, out);
}
