/** The console page: sign-in, navigation and the hash router. */
import { createApi } from './api.js';
import { h, mount } from './dom.js';
import { getLocale, initLocale, setLocale, t } from './i18n.js';
import { alertBox, loading } from './views/common.js';
import { editorView } from './views/editor.js';
import { loginView } from './views/login.js';
import { listView } from './views/service.js';
import { servicesView } from './views/services.js';

/** Splits "#/services/a%2Fb/policies" into decoded parts. */
export function parseRoute(hash) {
  return String(hash || '').replace(/^#\/?/, '').split('/').filter(Boolean).map((p) => {
    try { return decodeURIComponent(p); } catch { return p; }
  });
}

/** Picks the view for route parts. */
export function resolve(parts) {
  const [section, service, kind, id] = parts;
  if (section === 'services' && service) {
    if (kind === 'policies' || kind === 'role-policies') {
      return id ? { view: 'editor', service, kind, id } : { view: 'list', service, kind };
    }
    if (kind === 'roles') return { view: 'roles', service };
    return { view: 'list', service, kind: 'policies' };
  }
  if (section === 'diagnose') return { view: 'diagnose' };
  return { view: 'services' };
}

export function start(root = document.getElementById('app')) {
  initLocale();
  setLocale(getLocale());
  const ctx = { config: {}, me: null };
  const api = createApi({ onUnauthorized: () => { ctx.me = null; renderChrome(); route(); } });
  ctx.api = api;
  const header = h('header', { class: 'topbar' });
  const banner = h('div');
  const toast = h('div', { class: 'toast', role: 'status', 'aria-live': 'polite' });
  const main = h('main', { id: 'main', tabindex: '-1' });
  mount(root, h('a', { class: 'skip', href: '#main', on: { click: (e) => { e.preventDefault(); main.focus(); } } }, t('Skip to content')),
    header, banner, main, toast);

  let toastTimer;
  ctx.notify = (message) => {
    toast.textContent = message;
    toast.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => toast.classList.remove('show'), 4000);
  };
  ctx.navigate = (hash) => {
    if (location.hash === hash) route(); else location.hash = hash;
  };
  ctx.onLogin = (me) => {
    ctx.me = me;
    renderChrome();
    route();
  };

  function renderChrome() {
    const nav = ctx.me ? [
      h('a', { href: '#/services' }, t('Services')),
      h('a', { href: '#/diagnose' }, t('Diagnose')),
    ] : [];
    const user = ctx.me ? [
      h('span', { class: 'user', title: ctx.me.principals.join(', ') }, ctx.me.user,
        h('span', { class: 'badge', text: t(ctx.me.access.write ? 'Administrator' : 'Read only') })),
      ctx.config.devMode ? null : h('button', { type: 'button', class: 'btn link', on: { click: async () => {
        try { await api.logout(); } catch { /* signed out locally anyway */ }
        ctx.me = null;
        renderChrome();
        route();
      } } }, t('Sign out')),
    ] : [];
    mount(header,
      h('a', { class: 'brand', href: '#/services' }, 'Speedle+ ', h('span', { text: t('Policy console') })),
      h('nav', { class: 'mainnav', 'aria-label': t('Main') }, nav),
      h('div', { class: 'spacer' }),
      user,
      h('button', { type: 'button', class: 'btn link', lang: getLocale() === 'zh' ? 'en' : 'zh', on: { click: () => {
        setLocale(getLocale() === 'zh' ? 'en' : 'zh');
        renderChrome();
        route();
      } } }, getLocale() === 'zh' ? 'English' : '中文'));
    mount(banner,
      ctx.config.devMode ? h('div', { class: 'banner danger', role: 'alert', text: t('Development mode: there is no sign-in and everyone is an administrator. Never expose this console.') }) : null,
      ctx.me?.access?.bootstrap ? h('div', { class: 'banner warning', role: 'note', text: t('You are a bootstrap administrator. Write the console policies in service {service}, then remove --bootstrap-admins.', { service: ctx.config.authzService }) }) : null,
      ctx.me && !ctx.me.access.read ? h('div', { class: 'banner warning', role: 'note', text: t('You are signed in, but no policy lets you read the console.') }) : null);
  }

  async function route() {
    if (!ctx.me) {
      mount(main, loginView(ctx));
      return;
    }
    const target = resolve(parseRoute(location.hash));
    try {
      if (target.view === 'services') await servicesView(ctx, main);
      else if (target.view === 'list') await listView(ctx, main, target.service, target.kind);
      else if (target.view === 'editor') await editorView(ctx, main, target.service, target.kind, target.id);
      else await servicesView(ctx, main);
    } catch (err) {
      if (err.status === 401) { ctx.me = null; renderChrome(); route(); return; }
      mount(main, alertBox(err));
    }
  }

  window.addEventListener('hashchange', () => route());
  (async () => {
    mount(main, loading());
    try {
      ctx.config = await api.config();
    } catch (err) {
      mount(main, alertBox(err));
      return;
    }
    try {
      ctx.me = await api.me();
    } catch (err) {
      if (err.status !== 401) { renderChrome(); mount(main, alertBox(err)); return; }
    }
    renderChrome();
    route();
  })();
  return ctx;
}

if (globalThis.document?.getElementById('app')) start();
