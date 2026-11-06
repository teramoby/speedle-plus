/* Documentation navigation and TOC: no framework or third-party requests. */
(() => {
  'use strict';
  document.documentElement.classList.add('docs-js');

  const toggle = document.querySelector('.docs-nav-toggle');
  const sidebar = document.getElementById('docs-navigation');
  const backdrop = document.querySelector('.docs-nav-backdrop');
  const close = sidebar ? sidebar.querySelector('.docs-nav-close') : null;
  const compact = window.matchMedia('(max-width: 900px)');
  let lastFocus = null;

  const drawerOpen = () => !!sidebar && sidebar.classList.contains('is-open');

  function setDrawer(open, options = {}) {
    const {moveFocus = true} = options;
    if (!toggle || !sidebar) return;
    toggle.hidden = !compact.matches;
    if (!compact.matches) {
      sidebar.classList.remove('is-open');
      sidebar.removeAttribute('aria-hidden');
      toggle.setAttribute('aria-expanded', 'false');
      if (backdrop) backdrop.hidden = true;
      document.body.classList.remove('docs-nav-open');
      return;
    }
    sidebar.classList.toggle('is-open', open);
    sidebar.setAttribute('aria-hidden', String(!open));
    toggle.setAttribute('aria-expanded', String(open));
    if (backdrop) backdrop.hidden = !open;
    document.body.classList.toggle('docs-nav-open', open);
    if (!moveFocus) return;
    if (open) {
      lastFocus = document.activeElement;
      const first = close || sidebar.querySelector('a[href], button');
      if (first) first.focus();
    } else if (lastFocus && document.contains(lastFocus)) {
      lastFocus.focus();
    } else {
      toggle.focus();
    }
  }

  function syncSidebar() {
    if (!toggle || !sidebar) return;
    if (compact.matches) {
      toggle.hidden = false;
      if (!drawerOpen()) {
        sidebar.setAttribute('aria-hidden', 'true');
        toggle.setAttribute('aria-expanded', 'false');
        if (backdrop) backdrop.hidden = true;
        document.body.classList.remove('docs-nav-open');
      }
    } else {
      setDrawer(false, {moveFocus: false});
    }
  }

  if (toggle && sidebar) {
    syncSidebar();
    toggle.addEventListener('click', () => setDrawer(!drawerOpen()));
    if (close) close.addEventListener('click', () => setDrawer(false));
    if (backdrop) backdrop.addEventListener('click', () => setDrawer(false));
    document.addEventListener('keydown', event => {
      if (!drawerOpen()) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        setDrawer(false);
        return;
      }
      if (event.key !== 'Tab' || !sidebar) return;
      const items = [...sidebar.querySelectorAll('a[href], button:not([disabled])')]
        .filter(element => element.offsetParent !== null);
      if (!items.length) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    });
    if (typeof compact.addEventListener === 'function') compact.addEventListener('change', syncSidebar);
    else compact.addListener(syncSidebar);
  }

  function enableKeyboardScroll(root = document) {
    root.querySelectorAll('pre').forEach(pre => {
      if (pre.scrollWidth > pre.clientWidth) {
        pre.tabIndex = 0;
        pre.dataset.docsScroll = 'true';
      } else if (pre.dataset.docsScroll) {
        pre.removeAttribute('tabindex');
        delete pre.dataset.docsScroll;
      }
    });
  }
  enableKeyboardScroll();
  window.addEventListener('resize', () => enableKeyboardScroll());

  const swagger = document.getElementById('swagger-ui');
  if (swagger) {
    const normalizeHeadings = () => {
      enableKeyboardScroll(swagger);
      swagger.querySelectorAll('.opblock-tag-section > h4, section.models > h4').forEach(heading => {
        heading.setAttribute('role', 'heading');
        heading.setAttribute('aria-level', '3');
      });
    };
    normalizeHeadings();
    new MutationObserver(normalizeHeadings).observe(swagger, {childList: true, subtree: true});
  }

  document.querySelectorAll('nav[data-docs-toc]').forEach(navigation => {
    const content = document.getElementById('kube-component');
    if (!content) return;
    const selector = navigation.dataset.docsToc || 'h2';
    const list = navigation.querySelector('ol');
    const headings = content.querySelectorAll(selector);
    headings.forEach(heading => {
      if (!heading.id) return;
      const item = document.createElement('li');
      const link = document.createElement('a');
      link.href = `#${heading.id}`;
      link.textContent = heading.textContent;
      item.append(link);
      list.append(item);
    });
    navigation.hidden = list.children.length === 0;
  });
})();
