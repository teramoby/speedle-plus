import { h, mount, svg } from '../dom.js';
import { layout } from '../graph-layout.js';
import { t } from '../i18n.js';
import { parsePrincipals } from '../principals.js';
import { alertBox, button, field, href, loading } from './common.js';
import { tabs } from './service.js';

/** Beyond this many nodes the graph is shown as a list. */
export const MAX_DRAWN_NODES = 200;

function edgeClass(e) {
  return ['edge', e.effect === 'deny' ? 'deny' : 'grant', e.condition || e.resources || e.resourceExpressions ? 'conditional' : ''].join(' ').trim();
}

function nodeClass(n, selected) {
  return ['node', n.kind, n.inCycle ? 'cycle' : '', n.undefined ? 'undefined' : '', n.unused ? 'unused' : '', n.id === selected ? 'selected' : '']
    .filter(Boolean).join(' ');
}

function shorten(text, max = 24) {
  return text.length > max ? text.slice(0, max - 1) + '…' : text;
}

/** Draws the graph; clicking a role selects it. */
export function drawGraph(graph, selected, onSelect) {
  // Roles stay right of the principals, even roles nothing grants.
  const hasPrincipals = graph.nodes.some((n) => n.kind !== 'role');
  const result = layout(graph.nodes, graph.edges, { minLayer: (n) => (hasPrincipals && n.kind === 'role' ? 1 : 0) });
  const { positions, nodeWidth: w, nodeHeight: nh } = result;
  const pad = 12;
  const edgePaths = graph.edges.filter((e) => positions.has(e.from) && positions.has(e.to)).map((e) => {
    const a = positions.get(e.from), b = positions.get(e.to);
    let d;
    if (b.layer > a.layer) {
      // Through the bend points of a long edge, curving between layers.
      const points = [[a.x + w, a.y + nh / 2]];
      for (const p of result.bends.get(`${e.from}\n${e.to}`) || []) points.push([p.x, p.y + nh / 2], [p.x + w, p.y + nh / 2]);
      points.push([b.x, b.y + nh / 2]);
      d = `M${points[0][0]},${points[0][1]}`;
      for (let i = 1; i < points.length; i++) {
        const [x0, y0] = points[i - 1], [x1, y1] = points[i];
        d += i % 2 ? ` C${x0 + 35},${y0} ${x1 - 35},${y1} ${x1},${y1}` : ` L${x1},${y1}`;
      }
    } else {
      // Back and same-layer edges loop over the top.
      d = `M${a.x + w / 2},${a.y} C${a.x + w / 2},${a.y - 40} ${b.x + w / 2},${b.y - 40} ${b.x + w / 2},${b.y}`;
    }
    return svg('path', { d, class: edgeClass(e), 'marker-end': 'url(#arrow)' },
      svg('title', { text: `${e.policyName || e.policyId} (${e.effect}${e.condition ? ', ' + e.condition : ''})` }));
  });
  const nodeEls = graph.nodes.map((n) => {
    const p = positions.get(n.id);
    const label = n.kind === 'role' ? n.name : n.id === '*' ? t('everyone') : n.name;
    const attrs = { class: nodeClass(n, selected), transform: `translate(${p.x},${p.y})` };
    if (n.kind === 'role') Object.assign(attrs, { tabindex: 0, role: 'button', 'aria-label': t('Role {name}', { name: n.name }),
      on: { click: () => onSelect(n.name), keydown: (ev) => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); onSelect(n.name); } } } });
    return svg('g', attrs,
      svg('rect', { width: w, height: nh, rx: n.kind === 'role' ? 17 : 4 }),
      svg('text', { x: w / 2, y: nh / 2 + 5, 'text-anchor': 'middle', text: shorten(label) }),
      svg('title', { text: n.id }));
  });
  return svg('svg', {
    class: 'graph', viewBox: `${-pad} ${-pad - 40} ${result.width + 2 * pad} ${result.height + 2 * pad + 40}`,
    width: result.width + 2 * pad, height: result.height + 2 * pad + 40, role: 'img', 'aria-label': t('Role graph'),
  },
  svg('defs', {}, svg('marker', { id: 'arrow', viewBox: '0 0 10 10', refX: 10, refY: 5, markerWidth: 7, markerHeight: 7, orient: 'auto-start-reverse' },
    svg('path', { d: 'M0,0 L10,5 L0,10 z', class: 'arrowhead' }))),
  edgePaths, nodeEls);
}

/** For large graphs: each role with what grants it. */
function graphList(graph, onSelect) {
  const into = new Map();
  for (const e of graph.edges) (into.get(e.to) || into.set(e.to, []).get(e.to)).push(e);
  return h('ul', { class: 'role-list' }, graph.nodes.filter((n) => n.kind === 'role').map((n) => h('li', {},
    h('button', { type: 'button', class: 'btn link', on: { click: () => onSelect(n.name) } }, n.name),
    h('ul', {}, (into.get(n.id) || []).map((e) => h('li', { class: e.effect === 'deny' ? 'deny' : '' },
      `${e.effect} ← ${e.from === '*' ? t('everyone') : e.from} (${e.policyName || e.policyId})`))))));
}

function cycleText(cycle) {
  return [...cycle, cycle[0]].map((id) => id.replace(/^role:/, '')).join(' → ');
}

function issues(graph) {
  const undefinedRoles = graph.nodes.filter((n) => n.undefined);
  const unused = graph.nodes.filter((n) => n.unused);
  const items = [
    ...graph.cycles.map((c) => h('li', { class: 'issue error' }, t('Cycle: {roles}', { roles: cycleText(c) }))),
    ...undefinedRoles.map((n) => h('li', { class: 'issue warning' }, t('Role {name} is used but no role policy grants it.', { name: n.name }))),
    ...unused.map((n) => h('li', { class: 'issue' }, t('Role {name} is granted but no policy or role uses it.', { name: n.name }))),
  ];
  return items.length ? h('ul', { class: 'issues' }, items) : h('p', { class: 'alert ok', text: t('No cycles, undefined or unused roles.') });
}

function holdersPanel(ctx, service, data) {
  const principalText = (p) => (p === '*' ? t('everyone') : p);
  return h('section', { class: 'card panel', 'aria-live': 'polite' },
    h('h2', { text: t('Who holds role {name}', { name: data.role }) }),
    data.holders.length ? h('table', { class: 'grid' },
      h('thead', {}, h('tr', {}, h('th', { text: t('Principal') }), h('th', { text: t('Through') }), h('th', { text: t('When') }))),
      h('tbody', {}, data.holders.map((holder) => h('tr', {},
        h('td', { text: principalText(holder.principal) }),
        h('td', { text: holder.path.slice(1).map((id) => id.replace(/^role:/, '')).join(' → ') }),
        h('td', { text: holder.conditional ? t('Only when a condition or resource scope applies') : t('Always') })))))
      : h('p', { class: 'muted', text: t('No principal is granted this role.') }),
    data.denials.length ? [h('h3', { text: t('Deny role policies that can remove it') }), h('ul', {}, data.denials.map((e) => h('li', {},
      h('a', { href: href('services', e.service, 'role-policies', e.policyId), text: e.policyName || e.policyId }),
      ` — ${e.from === '*' ? t('everyone') : e.from} ✕ ${e.to.replace(/^role:/, '')}${e.service !== service ? ` (${e.service})` : ''}`)))] : null,
    h('h3', { text: t('Policies that use it') }),
    data.references.length ? h('ul', {}, data.references.map((p) => h('li', {},
      h('a', { href: href('services', service, 'policies', p.id), text: p.name || p.id }), ` (${p.effect})`)))
      : h('p', { class: 'muted', text: t('No authorization policy names this role.') }));
}

function principalRolesPanel(ctx, service) {
  const input = h('textarea', { rows: 3, spellcheck: 'false', placeholder: 'user:alice\ngroup:staff' });
  const resource = h('input', { type: 'text', autocomplete: 'off' });
  const out = h('div', { 'aria-live': 'polite' });
  const form = h('form', { class: 'card panel', on: { submit: async (e) => {
    e.preventDefault();
    const { principals, errors } = parsePrincipals(input.value);
    if (errors.length) { mount(out, h('p', { class: 'field-error', text: t(errors[0].message) })); return; }
    try {
      const roles = await ctx.api.grantedRoles({ subject: { principals }, serviceName: service, resource: resource.value.trim() });
      mount(out, roles && roles.length
        ? h('ul', { class: 'chips' }, roles.map((r) => h('li', { text: r })))
        : h('p', { class: 'muted', text: t('No roles.') }));
    } catch (err) { mount(out, alertBox(err)); }
  } } },
  h('h2', { text: t('Roles of a subject') }),
  field('roles-subject', 'Subject principals', input, { hint: 'One per line. ADS works out the roles, conditions included.' }),
  field('roles-resource', 'Resource', resource, { hint: 'Optional; roles scoped to resources need one.' }),
  button('Show roles', null, { kind: 'primary', type: 'submit' }), out);
  return form;
}

export async function rolesView(ctx, main, service, selected = '', includeGlobal = true) {
  mount(main, loading());
  let graph;
  try {
    graph = await ctx.api.roleGraph(service, includeGlobal);
  } catch (err) {
    mount(main, alertBox(err));
    return;
  }
  const holders = h('div');
  const select = async (role) => {
    try {
      mount(holders, holdersPanel(ctx, service, await ctx.api.roleHolders(service, role, includeGlobal)));
    } catch (err) { mount(holders, alertBox(err)); }
    for (const el of main.querySelectorAll('g.node.role')) {
      const isSelected = el.getAttribute('aria-label') === t('Role {name}', { name: role });
      el.classList[isSelected ? 'add' : 'remove']('selected');
    }
  };
  const global = h('input', { type: 'checkbox', id: 'include-global', checked: includeGlobal,
    on: { change: () => rolesView(ctx, main, service, selected, global.checked) } });
  const roles = graph.nodes.filter((n) => n.kind === 'role');
  mount(main,
    h('div', { class: 'page-head' }, h('h1', {}, h('a', { href: '#/services', text: t('Services') }), ' / ', service)),
    tabs(service, 'roles'),
    h('div', { class: 'toolbar' },
      h('label', { for: 'include-global', class: 'check' }, global, ' ', t('Include role policies of the global service')),
      h('span', { class: 'muted', text: t('{roles} roles, {edges} grants', { roles: roles.length, edges: graph.edges.length }) })),
    issues(graph),
    roles.length ? h('div', { class: 'graph-box' },
      graph.nodes.length > MAX_DRAWN_NODES ? graphList(graph, select) : drawGraph(graph, selected, select))
      : h('p', { class: 'muted', text: t('This service has no role policies.') }),
    h('p', { class: 'legend muted', text: t('Solid arrows grant a role, red ones deny it, dotted ones have a condition or resource scope. Select a role to see who holds it.') }),
    h('div', { class: 'panels' }, holders, principalRolesPanel(ctx, service)));
  if (selected) await select(selected);
}
