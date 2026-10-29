import { h, mount } from '../dom.js';
import { t } from '../i18n.js';
import {
  editableJSON, formToPolicy, formToRolePolicy, parseJSON, policyToForm, rolePolicyToForm,
} from '../policy-model.js';
import { alertBox, button, field, href, loading, showErrors } from './common.js';
import { tabs } from './service.js';

function effectSelect(value) {
  return h('select', {}, ['grant', 'deny'].map((e) => h('option', { value: e, selected: e === value }, e)));
}

function textarea(value, rows = 3) {
  return h('textarea', { rows, spellcheck: 'false', value });
}

/** Rows of permissions: a resource or an expression, and its actions. */
function permissionRows(initial) {
  const list = h('div', { class: 'permissions' });
  let rows = [];
  function addRow(perm = { kind: 'resource', value: '', actions: '' }) {
    const i = rows.length;
    const kind = h('select', { 'aria-label': t('Resource kind') },
      h('option', { value: 'resource', selected: perm.kind === 'resource' }, t('Resource')),
      h('option', { value: 'expression', selected: perm.kind === 'expression' }, t('Expression')));
    const value = h('input', { type: 'text', value: perm.value, id: `perm-${i}-value`, 'aria-label': t('Resource') });
    const actions = h('input', { type: 'text', value: perm.actions, id: `perm-${i}-actions`, 'aria-label': t('Actions'), placeholder: 'get, put' });
    const row = h('div', { class: 'perm-row' }, kind, value, actions,
      button('Remove', () => { row.remove(); rows = rows.filter((r) => r.row !== row); }, { kind: 'small' }),
      h('p', { class: 'field-error', id: `perm-${i}-value-error`, hidden: true }),
      h('p', { class: 'field-error', id: `perm-${i}-actions-error`, hidden: true }));
    rows.push({ row, kind, value, actions, index: i });
    list.append(row);
    return value;
  }
  initial.forEach(addRow);
  return {
    el: h('fieldset', { class: 'field' }, h('legend', { text: t('Permissions') }), list,
      button('Add permission', () => addRow().focus(), { kind: 'small' }),
      h('p', { class: 'field-error', id: 'permissions-error', hidden: true })),
    read: () => rows.map((r) => ({ kind: r.kind.value, value: r.value.value, actions: r.actions.value })),
    // Errors name rows by their position among the rows left.
    idOf: (index, part) => (rows[index] ? `perm-${rows[index].index}-${part}` : 'permissions'),
  };
}

/** Builds the form for a policy and returns how to read it back. */
function policyForm(form) {
  const name = h('input', { type: 'text', value: form.name, required: true });
  const effect = effectSelect(form.effect);
  const principals = textarea(form.principalsText, 4);
  const permissions = permissionRows(form.permissions);
  const condition = textarea(form.condition, 2);
  return {
    fields: [
      field('f-name', 'Name', name, { required: true }),
      field('f-effect', 'Effect', effect),
      field('f-principals', 'Principals', principals, { hint: 'One alternative per line; a line matches when all its principals, separated by commas, apply. Leave empty for everyone.' }),
      permissions.el,
      field('f-condition', 'Condition', condition, { hint: 'Optional SPDL condition, such as request_time > 9.' }),
    ],
    read: (id) => formToPolicy({ id, name: name.value, effect: effect.value, principalsText: principals.value, permissions: permissions.read(), condition: condition.value }),
    idOf: (path) => {
      const [head, index, part] = path.split('.');
      if (head === 'permissions') return index === undefined ? 'permissions' : permissions.idOf(Number(index), part);
      return { name: 'f-name', effect: 'f-effect', principals: 'f-principals', condition: 'f-condition' }[head];
    },
  };
}

function rolePolicyForm(form) {
  const controls = {
    name: h('input', { type: 'text', value: form.name, required: true }),
    effect: effectSelect(form.effect),
    roles: textarea(form.roles, 2),
    principals: textarea(form.principals, 4),
    resources: textarea(form.resources, 2),
    resourceExpressions: textarea(form.resourceExpressions, 2),
    condition: textarea(form.condition, 2),
  };
  return {
    fields: [
      field('f-name', 'Name', controls.name, { required: true }),
      field('f-effect', 'Effect', controls.effect),
      field('f-roles', 'Roles', controls.roles, { required: true, hint: 'One role per line, such as admin.' }),
      field('f-principals', 'Principals', controls.principals, { hint: 'One principal per line, such as user:alice, group:ops or role:manager. Leave empty for everyone.' }),
      field('f-resources', 'Resources', controls.resources, { hint: 'Optional; the role applies only to these resources.' }),
      field('f-resourceExpressions', 'Resource expressions', controls.resourceExpressions, { hint: 'Optional regular expressions, one per line.' }),
      field('f-condition', 'Condition', controls.condition, { hint: 'Optional SPDL condition.' }),
    ],
    read: (id) => formToRolePolicy({ id, ...Object.fromEntries(Object.entries(controls).map(([k, c]) => [k, c.value])) }),
    idOf: (path) => `f-${path.split('.')[0]}`,
  };
}

/** Edits a policy or role policy; id "new" creates one. */
export async function editorView(ctx, main, service, kind, id) {
  const role = kind === 'role-policies';
  const isNew = id === 'new';
  mount(main, loading());
  let item = null;
  let etag = '';
  if (!isNew) {
    try {
      ({ data: item, etag } = role ? await ctx.api.rolePolicy(service, id) : await ctx.api.policy(service, id));
    } catch (err) {
      mount(main, alertBox(err));
      return;
    }
  }
  const canWrite = ctx.me.access.write;
  const status = h('div', { 'aria-live': 'polite' });
  const body = h('div');
  let mode = 'form';
  let current = item || {};
  let editor = null;
  let json = null;

  function renderForm() {
    editor = role ? rolePolicyForm(rolePolicyToForm(current)) : policyForm(policyToForm(current));
    mount(body, editor.fields);
    lock();
  }
  /** Read-only users see every field but cannot change it. */
  function lock() {
    if (!canWrite) for (const el of body.querySelectorAll('input, select, textarea, button')) el.disabled = true;
  }
  function renderJSON() {
    json = textarea(editableJSON(current), 18);
    json.classList.add('code');
    mount(body, field('f-json', 'JSON', json));
    lock();
  }
  /** Reads the current view into an item, or shows its errors. */
  function read() {
    if (mode === 'json') {
      const parsed = parseJSON(json.value, isNew ? '' : id);
      if (!parsed.ok) { showErrors(formEl, parsed.errors, () => 'f-json'); return null; }
      return parsed.value;
    }
    const result = editor.read(isNew ? '' : id);
    if (!result.ok) { showErrors(formEl, result.errors, editor.idOf); return null; }
    return result.policy;
  }

  const toggle = button('Edit as JSON', () => {
    const value = read();
    if (!value) return;
    current = value;
    mode = mode === 'form' ? 'json' : 'form';
    toggle.textContent = t(mode === 'form' ? 'Edit as JSON' : 'Edit as form');
    if (mode === 'form') renderForm(); else renderJSON();
  });

  const formEl = h('form', { class: 'card editor', on: { submit: async (e) => {
    e.preventDefault();
    const value = read();
    if (!value) return;
    status.replaceChildren();
    try {
      let saved;
      if (isNew) saved = role ? await ctx.api.createRolePolicy(service, value) : await ctx.api.createPolicy(service, value);
      else saved = role ? await ctx.api.updateRolePolicy(service, value, etag) : await ctx.api.updatePolicy(service, value, etag);
      ctx.notify(t('Saved {name}.', { name: value.name }));
      if (isNew) ctx.navigate(href('services', service, kind, saved.data.id));
      else await editorView(ctx, main, service, kind, id);
    } catch (err) {
      mount(status, alertBox(err));
    }
  } } }, body, status,
  canWrite ? h('div', { class: 'actions' }, button('Save', null, { kind: 'primary', type: 'submit' }), toggle,
    h('a', { class: 'btn secondary', href: href('services', service, kind) }, t('Cancel'))) : h('div', { class: 'actions' }, toggle));

  renderForm();

  const title = isNew ? t(role ? 'New role policy' : 'New policy') : (item.name || item.id);
  const meta = item?.metadata ? h('dl', { class: 'meta' }, Object.entries(item.metadata).sort().map(([k, v]) => [h('dt', { text: k }), h('dd', { text: v })])) : null;
  mount(main,
    h('div', { class: 'page-head' }, h('h1', {},
      h('a', { href: '#/services', text: t('Services') }), ' / ',
      h('a', { href: href('services', service, kind), text: service }), ' / ', title)),
    tabs(service, kind),
    service === ctx.config.authzService && canWrite
      ? h('div', { class: 'alert warning', role: 'note', text: t('These policies decide who may use the console. A mistake here can lock you out.') }) : null,
    !isNew ? h('p', { class: 'muted' }, t('ID'), ': ', h('code', { text: id })) : null,
    formEl, meta);
}
