import { h } from '../dom.js';
import { t } from '../i18n.js';

/** A labelled form control. */
export function field(id, label, control, { hint, required } = {}) {
  control.id = id;
  const hintId = hint ? `${id}-hint` : undefined;
  if (hintId) control.setAttribute('aria-describedby', hintId);
  return h('div', { class: 'field' },
    h('label', { for: id }, t(label), required ? h('span', { class: 'required', text: ' *' }) : null),
    control,
    hint ? h('p', { class: 'hint', id: hintId, text: t(hint) }) : null,
    h('p', { class: 'field-error', id: `${id}-error`, hidden: true }));
}

/** Shows model errors next to their fields and focuses the first one. */
export function showErrors(form, errors, fieldIds) {
  for (const el of form.querySelectorAll('.field-error')) { el.hidden = true; el.textContent = ''; }
  for (const el of form.querySelectorAll('[aria-invalid]')) el.removeAttribute('aria-invalid');
  let first = null;
  for (const e of errors) {
    const key = fieldIds(e.path);
    const control = key && form.querySelector(`#${key}`);
    const box = key && form.querySelector(`#${key}-error`);
    if (box) { box.hidden = false; box.textContent = t(e.message); }
    if (control) { control.setAttribute('aria-invalid', 'true'); first = first || control; }
  }
  if (first) first.focus();
}

export function errorText(err) {
  if (!err) return '';
  if (err.status === 412) return t('Someone changed this item after you opened it. Reload it and make your change again.');
  if (err.status === 403) return t('You do not have permission to do this.');
  if (err.status === 503) return t('The authorization service is unavailable.');
  if (err.status === 502 || err.status === 0) return t('The policy service is unreachable.');
  return err.message || String(err);
}

export function alertBox(err) {
  return h('div', { class: 'alert error', role: 'alert', text: errorText(err) });
}

export function loading() {
  return h('p', { class: 'muted', role: 'status', text: t('Loading…') });
}

export function button(label, onClick, { kind = 'secondary', type = 'button', disabled } = {}) {
  return h('button', { type, class: `btn ${kind}`, disabled, on: onClick ? { click: onClick } : undefined }, t(label));
}

export function link(href, label, cls) {
  return h('a', { href, class: cls }, label);
}

export function href(...parts) {
  return '#/' + parts.map((p) => encodeURIComponent(p)).join('/');
}

export function formatPrincipals(groups = []) {
  if (!groups.length) return t('everyone');
  return groups.map((g) => g.join(' & ')).join(' | ');
}
