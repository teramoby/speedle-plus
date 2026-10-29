/**
 * Converts policies and role policies between their PMS JSON form and the
 * editor form, and checks them before they are sent.
 *
 * Principals of a policy are an OR of AND groups. The editor shows one
 * group per line, its principals separated by commas:
 *   user:alice
 *   group:ops, role:oncall
 * means alice, or anyone who is both in ops and oncall.
 */
export const EFFECTS = ['grant', 'deny'];

const PRINCIPAL = /^(idd=[^:\s]+:)?(user|group|entity|role):\S.*$/;

/** Splits text into trimmed, non-empty items. */
export function splitList(text, separator = /[,\n]/) {
  return String(text ?? '').split(separator).map((s) => s.trim()).filter(Boolean);
}

export function principalsToText(groups = []) {
  return groups.map((group) => group.join(', ')).join('\n');
}

export function textToPrincipals(text) {
  return String(text ?? '').split('\n').map((line) => splitList(line, ',')).filter((g) => g.length);
}

export function emptyPolicy() {
  return { id: '', name: '', effect: 'grant', principalsText: '', permissions: [{ kind: 'resource', value: '', actions: '' }], condition: '' };
}

export function policyToForm(p = {}) {
  const permissions = (p.permissions || []).map((perm) => ({
    kind: perm.resourceExpression ? 'expression' : 'resource',
    value: perm.resourceExpression || perm.resource || '',
    actions: (perm.actions || []).join(', '),
  }));
  return {
    id: p.id || '', name: p.name || '', effect: p.effect || 'grant',
    principalsText: principalsToText(p.principals || []),
    permissions: permissions.length ? permissions : emptyPolicy().permissions,
    condition: p.condition || '',
  };
}

function checkPrincipals(list, pattern, path, error) {
  list.forEach((principal, i) => {
    if (!pattern.test(principal)) error(`${path}.${i}`, 'Write principals as type:name, such as user:alice, group:ops or role:admin.');
  });
}

/** Returns {ok, policy, errors}; errors name the form field at fault. */
export function formToPolicy(form) {
  const errors = [];
  const error = (path, message) => errors.push({ path, message });
  const name = String(form.name || '').trim();
  if (!name) error('name', 'Enter a name.');
  if (!EFFECTS.includes(form.effect)) error('effect', 'Choose grant or deny.');
  const principals = textToPrincipals(form.principalsText);
  checkPrincipals(principals.flat(), PRINCIPAL, 'principals', error);
  const permissions = [];
  (form.permissions || []).forEach((row, i) => {
    const value = String(row.value || '').trim();
    const actions = splitList(row.actions, ',');
    if (!value && !actions.length) return;
    if (!value) error(`permissions.${i}.value`, 'Enter a resource or a resource expression.');
    if (!actions.length) error(`permissions.${i}.actions`, 'Enter at least one action.');
    permissions.push(row.kind === 'expression' ? { resourceExpression: value, actions } : { resource: value, actions });
  });
  if (!permissions.length) error('permissions', 'Add at least one permission.');
  const policy = { name, effect: form.effect, permissions };
  if (form.id) policy.id = form.id;
  if (principals.length) policy.principals = principals;
  const condition = String(form.condition || '').trim();
  if (condition) policy.condition = condition;
  return { ok: errors.length === 0, policy, errors };
}

export function emptyRolePolicy() {
  return { id: '', name: '', effect: 'grant', roles: '', principals: '', resources: '', resourceExpressions: '', condition: '' };
}

export function rolePolicyToForm(p = {}) {
  return {
    id: p.id || '', name: p.name || '', effect: p.effect || 'grant',
    roles: (p.roles || []).join('\n'),
    principals: (p.principals || []).join('\n'),
    resources: (p.resources || []).join('\n'),
    resourceExpressions: (p.resourceExpressions || []).join('\n'),
    condition: p.condition || '',
  };
}

export function formToRolePolicy(form) {
  const errors = [];
  const error = (path, message) => errors.push({ path, message });
  const name = String(form.name || '').trim();
  if (!name) error('name', 'Enter a name.');
  if (!EFFECTS.includes(form.effect)) error('effect', 'Choose grant or deny.');
  const roles = splitList(form.roles);
  if (!roles.length) error('roles', 'Enter at least one role.');
  roles.forEach((role, i) => { if (role.includes(':')) error(`roles.${i}`, 'Enter role names without a type, such as admin.'); });
  const principals = splitList(form.principals, '\n');
  checkPrincipals(principals, PRINCIPAL, 'principals', error);
  const policy = { name, effect: form.effect, roles };
  if (form.id) policy.id = form.id;
  if (principals.length) policy.principals = principals;
  const resources = splitList(form.resources, '\n');
  if (resources.length) policy.resources = resources;
  const expressions = splitList(form.resourceExpressions, '\n');
  if (expressions.length) policy.resourceExpressions = expressions;
  const condition = String(form.condition || '').trim();
  if (condition) policy.condition = condition;
  return { ok: errors.length === 0, policy, errors };
}

/** The editable fields of an item, for the JSON view; metadata is managed by PMS. */
export function editableJSON(item) {
  const { metadata, ...rest } = item || {};
  return JSON.stringify(rest, null, 2);
}

/** Parses the JSON view; the path ID wins over any ID in the text. */
export function parseJSON(text, id) {
  let value;
  try { value = JSON.parse(text); } catch (e) { return { ok: false, errors: [{ path: 'json', message: 'The JSON is not valid.' }] }; }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return { ok: false, errors: [{ path: 'json', message: 'Enter a JSON object.' }] };
  delete value.metadata;
  if (id) value.id = id; else delete value.id;
  return { ok: true, value };
}

/** Case-insensitive search over everything shown for an item. */
export function matches(item, query) {
  const q = String(query || '').trim().toLowerCase();
  if (!q) return true;
  return JSON.stringify(item).toLowerCase().includes(q);
}
