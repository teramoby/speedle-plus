import { h, mount } from '../dom.js';
import { t } from '../i18n.js';
import { matches } from '../policy-model.js';
import { alertBox, button, field, href, loading } from './common.js';

/** Asks the user to type the service name before it is deleted. */
function deleteConfirm(name, onConfirm, onCancel) {
  const input = h('input', { type: 'text', autocomplete: 'off' });
  const confirmBtn = button('Delete service', null, { kind: 'danger', type: 'submit', disabled: true });
  input.addEventListener('input', () => { confirmBtn.disabled = input.value !== name; });
  const form = h('form', { class: 'confirm', on: { submit: (e) => { e.preventDefault(); if (input.value === name) onConfirm(); } } },
    h('p', { text: t('This deletes service {name} with all its policies and role policies. Type the service name to confirm.', { name }) }),
    field(`confirm-${name}`, 'Service name', input),
    h('div', { class: 'actions' }, confirmBtn, button('Cancel', onCancel)));
  setTimeout(() => input.focus(), 0);
  return form;
}

export async function servicesView(ctx, main) {
  mount(main, loading());
  let services, counts;
  try {
    [services, counts] = await Promise.all([ctx.api.services(), ctx.api.policyCounts()]);
  } catch (err) {
    mount(main, alertBox(err));
    return;
  }
  services.sort((a, b) => a.name.localeCompare(b.name));
  const canWrite = ctx.me.access.write;
  const filter = h('input', { type: 'search', placeholder: t('Filter services'), 'aria-label': t('Filter services') });
  const status = h('div', { 'aria-live': 'polite' });
  const body = h('tbody');

  function renderRows() {
    const rows = services.filter((s) => matches({ name: s.name, type: s.type }, filter.value)).map((s) => {
      const c = counts[s.name] || {};
      const row = h('tr', {},
        h('td', {}, h('a', { href: href('services', s.name), text: s.name })),
        h('td', { text: s.type || '' }),
        h('td', { class: 'num', text: String(c.policycount || 0) }),
        h('td', { class: 'num', text: String(c.rolePolicycount || 0) }),
        h('td', { class: 'actions' }, canWrite ? button('Delete', () => {
          const cell = h('td', { colspan: 5 });
          const confirmRow = h('tr', { class: 'confirm-row' }, cell);
          mount(cell, deleteConfirm(s.name, async () => {
            try {
              await ctx.api.deleteService(s.name);
              ctx.notify(t('Deleted service {name}.', { name: s.name }));
              servicesView(ctx, main);
            } catch (err) { mount(status, alertBox(err)); }
          }, () => confirmRow.remove()));
          row.after(confirmRow);
        }, { kind: 'danger small' }) : null));
      return row;
    });
    mount(body, rows.length ? rows : h('tr', {}, h('td', { colspan: 5, class: 'muted', text: t('No services.') })));
  }
  filter.addEventListener('input', renderRows);

  let createForm = null;
  if (canWrite) {
    const name = h('input', { type: 'text', autocomplete: 'off', required: true });
    const type = h('select', {}, h('option', { value: 'application', text: 'application' }), h('option', { value: 'k8s-cluster', text: 'k8s-cluster' }));
    createForm = h('form', { class: 'inline-form', on: { submit: async (e) => {
      e.preventDefault();
      const value = name.value.trim();
      if (!value) { name.focus(); return; }
      try {
        await ctx.api.createService(value, type.value);
        ctx.notify(t('Created service {name}.', { name: value }));
        ctx.navigate(href('services', value));
      } catch (err) { mount(status, alertBox(err)); }
    } } },
    field('new-service-name', 'New service', name, { required: true }),
    field('new-service-type', 'Type', type),
    button('Create', null, { kind: 'primary', type: 'submit' }));
  }

  renderRows();
  mount(main,
    h('div', { class: 'page-head' }, h('h1', { text: t('Services') }), filter),
    status,
    h('table', { class: 'grid' },
      h('thead', {}, h('tr', {},
        h('th', { text: t('Name') }), h('th', { text: t('Type') }),
        h('th', { class: 'num', text: t('Policies') }), h('th', { class: 'num', text: t('Role policies') }),
        h('th', { class: 'actions' }, h('span', { class: 'sr-only', text: t('Actions') })))),
      body),
    createForm);
}
