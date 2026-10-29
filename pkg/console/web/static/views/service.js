import { h, mount } from '../dom.js';
import { t } from '../i18n.js';
import { matches } from '../policy-model.js';
import { alertBox, button, formatPrincipals, href, loading } from './common.js';

export const TABS = [
  { key: 'policies', label: 'Policies' },
  { key: 'role-policies', label: 'Role policies' },
  { key: 'roles', label: 'Roles' },
];

export function tabs(service, active) {
  return h('nav', { class: 'tabs', 'aria-label': t('Service sections') },
    TABS.map((tab) => h('a', {
      href: href('services', service, tab.key),
      class: tab.key === active ? 'tab active' : 'tab',
      'aria-current': tab.key === active ? 'page' : undefined,
    }, t(tab.label))));
}

function permissionsText(p) {
  return (p.permissions || []).map((perm) =>
    `${perm.resourceExpression ? '~' + perm.resourceExpression : perm.resource || ''}: ${(perm.actions || []).join(', ')}`).join('; ');
}

const COLUMNS = {
  policies: [
    ['Name', (p) => p.name],
    ['Effect', (p) => p.effect],
    ['Principals', (p) => formatPrincipals(p.principals)],
    ['Permissions', permissionsText],
    ['Condition', (p) => p.condition || ''],
  ],
  'role-policies': [
    ['Name', (p) => p.name],
    ['Effect', (p) => p.effect],
    ['Roles', (p) => (p.roles || []).join(', ')],
    ['Principals', (p) => (p.principals || []).join(' | ') || t('everyone')],
    ['Resources', (p) => [...(p.resources || []), ...(p.resourceExpressions || []).map((e) => '~' + e)].join(', ') || t('all')],
    ['Condition', (p) => p.condition || ''],
  ],
};

/** Lists the policies or role policies of a service. */
export async function listView(ctx, main, service, kind) {
  mount(main, loading());
  const role = kind === 'role-policies';
  let items;
  try {
    items = role ? await ctx.api.rolePolicies(service) : await ctx.api.policies(service);
  } catch (err) {
    mount(main, h('h1', { text: service }), alertBox(err));
    return;
  }
  items.sort((a, b) => (a.name || '').localeCompare(b.name || ''));
  const canWrite = ctx.me.access.write;
  const columns = COLUMNS[kind];
  const filter = h('input', { type: 'search', placeholder: t('Filter'), 'aria-label': t('Filter') });
  const status = h('div', { 'aria-live': 'polite' });
  const body = h('tbody');
  const remove = role ? ctx.api.deleteRolePolicy : ctx.api.deletePolicy;

  function renderRows() {
    const rows = items.filter((p) => matches(p, filter.value)).map((p) => h('tr', {},
      columns.map(([, value], i) => h('td', {}, i === 0
        ? h('a', { href: href('services', service, kind, p.id), text: value(p) || p.id })
        : value(p))),
      h('td', { class: 'actions' }, canWrite ? button('Delete', async () => {
        if (!globalThis.confirm(t('Delete {name}?', { name: p.name || p.id }))) return;
        try {
          await remove(service, p.id);
          ctx.notify(t('Deleted {name}.', { name: p.name || p.id }));
          listView(ctx, main, service, kind);
        } catch (err) { mount(status, alertBox(err)); }
      }, { kind: 'danger small' }) : null)));
    mount(body, rows.length ? rows : h('tr', {}, h('td', { colspan: columns.length + 1, class: 'muted', text: t('Nothing here yet.') })));
  }
  filter.addEventListener('input', renderRows);
  renderRows();

  mount(main,
    h('div', { class: 'page-head' },
      h('h1', {}, h('a', { href: '#/services', text: t('Services') }), ' / ', service),
      canWrite ? h('a', { class: 'btn primary', href: href('services', service, kind, 'new') }, t(role ? 'New role policy' : 'New policy')) : null),
    tabs(service, kind),
    h('div', { class: 'toolbar' }, filter, h('span', { class: 'muted', text: t('{count} items', { count: items.length }) })),
    status,
    h('table', { class: 'grid' },
      h('thead', {}, h('tr', {}, columns.map(([label]) => h('th', { text: t(label) })),
        h('th', { class: 'actions' }, h('span', { class: 'sr-only', text: t('Actions') })))),
      body));
}
