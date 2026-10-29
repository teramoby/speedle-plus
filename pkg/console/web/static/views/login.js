import { h } from '../dom.js';
import { t } from '../i18n.js';
import { alertBox, button, field } from './common.js';

export function loginView(ctx) {
  const token = h('input', { type: 'password', autocomplete: 'off', required: true });
  const tokenType = h('input', { type: 'text', autocomplete: 'off', placeholder: 'bearer' });
  const status = h('div', { 'aria-live': 'polite' });
  const form = h('form', { class: 'card narrow', on: { submit: async (e) => {
    e.preventDefault();
    status.replaceChildren();
    try {
      const me = await ctx.api.login(token.value, tokenType.value.trim());
      token.value = '';
      ctx.onLogin(me);
    } catch (err) {
      status.replaceChildren(alertBox(err.status === 401 ? { message: t('The token was not accepted.') } : err));
      token.focus();
    }
  } } },
  h('h1', { text: t('Sign in') }),
  h('p', { class: 'muted', text: t('Sign in with a token your identity provider issued. The console checks it with the token asserter.') }),
  field('login-token', 'Token', token, { required: true }),
  field('login-token-type', 'Token type', tokenType, { hint: 'Optional; passed to the token asserter.' }),
  status,
  button('Sign in', null, { kind: 'primary', type: 'submit' }));
  return form;
}
