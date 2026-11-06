/* Documentation navigation and TOC: no framework or third-party requests. */
(() => {
  'use strict';
  const toggle = document.querySelector('.docs-nav-toggle');
  const sidebar = document.getElementById('docs-navigation');
  const compact = window.matchMedia('(max-width: 900px)');

  function syncSidebar() {
    if (!toggle || !sidebar) return;
    toggle.hidden = !compact.matches;
    sidebar.hidden = compact.matches;
    toggle.setAttribute('aria-expanded', 'false');
  }

  if (toggle && sidebar) {
    syncSidebar();
    toggle.addEventListener('click', () => {
      const expanded = toggle.getAttribute('aria-expanded') !== 'true';
      toggle.setAttribute('aria-expanded', String(expanded));
      sidebar.hidden = !expanded;
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
