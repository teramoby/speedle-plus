/* Share language for this session; remember it only after an explicit opt-in. */
(function () {
  if (typeof location === 'undefined') return;
  var hosts = ['speedle.io', 'engine.speedle.io', 'ai.speedle.io', 'cloud.speedle.io'];
  var shared = location.protocol === 'https:' && hosts.indexOf(location.hostname) >= 0;
  var locales = ['en', 'ja', 'ko', 'es', 'zh-CN'];
  var attributes = '; Domain=speedle.io; Path=/; SameSite=Lax; Secure';
  var controls = [];
  var messages = {
    en: ['Remember my language across Speedle Engine, AI and Cloud (uses cookies, up to one year)', 'Clear saved language', 'Saved language cleared.'],
    'zh-CN': ['记住我的语言，并在 Speedle Engine、AI 和 Cloud 间同步（使用 Cookie，最长保存一年）。', '清除已保存的语言', '已清除保存的语言。'],
    ja: ['言語設定を Speedle Engine、AI、Cloud 間で記憶する（Cookie を使用、最長1年間）', '保存した言語設定を削除', '保存した言語設定を削除しました。'],
    ko: ['Speedle Engine, AI, Cloud에서 내 언어 기억하기(Cookie 사용, 최대 1년)', '저장된 언어 지우기', '저장된 언어를 지웠습니다.'],
    es: ['Recordar mi idioma en Speedle Engine, AI y Cloud (usa cookies, hasta un año)', 'Borrar el idioma guardado', 'Se ha borrado el idioma guardado.']
  };
  function cookie(name) {
    if (!shared) return null;
    try {
      var parts = document.cookie.split(';');
      for (var i = 0; i < parts.length; i++) {
        var pair = parts[i].trim().split('=');
        if (pair.length === 2 && pair[0] === name) return pair[1];
      }
    } catch (error) { /* Storage restrictions must not block language links. */ }
    return null;
  }
  function remembers() { return cookie('speedle_locale_remember') === '1'; }
  function put(name, value, persistent) {
    if (!shared) return;
    try { document.cookie = name + '=' + value + attributes + (persistent ? '; Max-Age=31536000' : ''); }
    catch (error) { /* Ordinary localized navigation remains available. */ }
  }
  function remove(name) {
    if (!shared) return;
    try { document.cookie = name + '=' + attributes + '; Max-Age=0'; }
    catch (error) { /* Controls still work when storage is unavailable. */ }
  }
  function cleanLegacyStorage() {
    ['localStorage', 'sessionStorage'].forEach(function (name) {
      try { window[name].removeItem('speedle-cloud.locale'); }
      catch (error) { /* This origin may not expose browser storage. */ }
    });
  }
  function currentLocale() {
    var locale = document.documentElement.lang;
    return locales.indexOf(locale) >= 0 ? locale : 'en';
  }
  function refresh() {
    controls.forEach(function (control) { control.checked = remembers(); });
  }
  var preference = cookie('speedle_locale');
  if (locales.indexOf(preference) < 0) preference = null;
  if (!remembers()) {
    cleanLegacyStorage();
    // A legacy one-year value is not consent: shorten it to this session.
    if (preference) put('speedle_locale', preference, false);
  }

  document.querySelectorAll('.languages-panel').forEach(function (panel) {
    var text = messages[currentLocale()];
    var group = document.createElement('div');
    group.className = 'language-preferences';
    var label = document.createElement('label');
    label.className = 'language-remember';
    var checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.className = 'language-remember-input';
    checkbox.disabled = !shared;
    checkbox.checked = remembers();
    var description = document.createElement('span');
    description.textContent = text[0];
    label.append(checkbox, description);
    var clear = document.createElement('button');
    clear.type = 'button';
    clear.className = 'language-clear';
    clear.textContent = text[1];
    var status = document.createElement('span');
    status.className = 'language-preference-status';
    status.setAttribute('role', 'status');
    group.append(label, clear, status);
    panel.append(group);
    controls.push(checkbox);
    checkbox.addEventListener('change', function () {
      if (checkbox.checked) {
        put('speedle_locale_remember', '1', true);
        put('speedle_locale', currentLocale(), remembers());
      } else {
        remove('speedle_locale_remember');
        cleanLegacyStorage();
        put('speedle_locale', currentLocale(), false);
      }
      status.textContent = '';
      refresh();
    });
    clear.addEventListener('click', function () {
      remove('speedle_locale_remember');
      remove('speedle_locale');
      cleanLegacyStorage();
      refresh();
      status.textContent = text[2];
    });
  });
  function selectLanguage(event) {
    if (event.type === 'auxclick' && event.button !== 1) return;
    if (event.type === 'click' && event.button > 0) return;
    var target = event.target;
    if (!target || typeof target.closest !== 'function') return;
    if (target.closest('.languages-button, .menu')) refresh();
    var link = target.closest('.languages a[hreflang], #top-nav-lang a[hreflang]');
    if (!link || event.defaultPrevented) return;
    var locale = link.getAttribute('hreflang');
    if (locales.indexOf(locale) < 0) return;
    var persistent = remembers();
    if (!persistent) {
      remove('speedle_locale_remember');
      cleanLegacyStorage();
    }
    put('speedle_locale', locale, persistent);
  }
  document.addEventListener('click', selectLanguage, true);
  document.addEventListener('auxclick', selectLanguage, true);
  if (typeof window.addEventListener === 'function') {
    window.addEventListener('focus', refresh);
    window.addEventListener('pageshow', refresh);
  }
  // Explicit localized URLs remain stable and never create a saved choice.
  if (preference && preference !== 'en' && (location.pathname === '/' || location.pathname === '/index.html')) {
    var prefix = preference === 'zh-CN' ? 'zh' : preference;
    location.replace('/' + prefix + '/' + location.search + location.hash);
  }
})();

/* One accessible menu controller for the homepage, Playground and Hugo pages. */
document.querySelectorAll('header.site-header').forEach(function(header) {
  var button = header.querySelector('.menu');
  var navigation = header.querySelector('.nav-links');
  if (!button || !navigation) return;
  function setOpen(open) {
    navigation.classList.toggle('open', open);
    button.setAttribute('aria-expanded', String(open));
    button.textContent = open ? '×' : '☰';
  }
  button.addEventListener('click', function() {
    setOpen(button.getAttribute('aria-expanded') !== 'true');
  });
  navigation.addEventListener('click', function(event) {
    if (event.target.closest('a')) setOpen(false);
  });
  header.addEventListener('keydown', function(event) {
    if (event.key === 'Escape' && button.getAttribute('aria-expanded') === 'true') {
      setOpen(false);
      button.focus();
    }
  });
  // Speedle family "Products" menu: click/Enter/Space toggle, Esc and outside click close, arrows move.
  var products = header.querySelector('.products');
  var productsButton = products && products.querySelector('.products-button');
  var productRows = products ? Array.prototype.slice.call(products.querySelectorAll('.product-row')) : [];
  function setProducts(open, focusButton) {
    if (!productsButton) return;
    products.classList.toggle('open', open);
    productsButton.setAttribute('aria-expanded', String(open));
    if (!open && focusButton) productsButton.focus();
  }
  if (productsButton) {
    productsButton.addEventListener('click', function() {
      setProducts(productsButton.getAttribute('aria-expanded') !== 'true');
    });
    productsButton.addEventListener('keydown', function(event) {
      if (event.key === 'ArrowDown' && productRows.length) {
        event.preventDefault();
        event.stopPropagation();
        setProducts(true);
        productRows[0].focus();
      }
    });
    products.addEventListener('keydown', function(event) {
      var index = productRows.indexOf(document.activeElement);
      if (event.key === 'Escape' && productsButton.getAttribute('aria-expanded') === 'true') {
        event.preventDefault();
        event.stopPropagation();
        setProducts(false, true);
      } else if (index >= 0 && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
        event.preventDefault();
        var next = (index + (event.key === 'ArrowDown' ? 1 : productRows.length - 1)) % productRows.length;
        productRows[next].focus();
      } else if (index >= 0 && (event.key === 'Home' || event.key === 'End')) {
        event.preventDefault();
        productRows[event.key === 'Home' ? 0 : productRows.length - 1].focus();
      }
    });
    document.addEventListener('click', function(event) {
      if (!products.contains(event.target)) setProducts(false);
    });
    products.addEventListener('focusout', function(event) {
      if (event.relatedTarget && !products.contains(event.relatedTarget)) setProducts(false);
    });
  }
  // Languages menu: click/Enter/Space toggle, Esc and outside click close, arrows move.
  var languages = header.querySelector('.languages');
  var languagesButton = languages && languages.querySelector('.languages-button');
  var languageRows = languages ? Array.prototype.slice.call(languages.querySelectorAll('.language-row')) : [];
  function setLanguages(open, focusButton) {
    if (!languagesButton) return;
    languages.classList.toggle('open', open);
    languagesButton.setAttribute('aria-expanded', String(open));
    if (!open && focusButton) languagesButton.focus();
  }
  if (languagesButton) {
    languagesButton.addEventListener('click', function() {
      setLanguages(languagesButton.getAttribute('aria-expanded') !== 'true');
    });
    languagesButton.addEventListener('keydown', function(event) {
      if (event.key === 'ArrowDown' && languageRows.length) {
        event.preventDefault();
        event.stopPropagation();
        setLanguages(true);
        languageRows[0].focus();
      }
    });
    languages.addEventListener('keydown', function(event) {
      var index = languageRows.indexOf(document.activeElement);
      if (event.key === 'Escape' && languagesButton.getAttribute('aria-expanded') === 'true') {
        event.preventDefault();
        event.stopPropagation();
        setLanguages(false, true);
      } else if (index >= 0 && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
        event.preventDefault();
        var next = (index + (event.key === 'ArrowDown' ? 1 : languageRows.length - 1)) % languageRows.length;
        languageRows[next].focus();
      } else if (index >= 0 && (event.key === 'Home' || event.key === 'End')) {
        event.preventDefault();
        languageRows[event.key === 'Home' ? 0 : languageRows.length - 1].focus();
      }
    });
    document.addEventListener('click', function(event) {
      if (!languages.contains(event.target)) setLanguages(false);
    });
    languages.addEventListener('focusout', function(event) {
      if (event.relatedTarget && !languages.contains(event.relatedTarget)) setLanguages(false);
    });
  }
  window.matchMedia('(max-width: 1160px)').addEventListener('change', function() { setOpen(false); setProducts(false); setLanguages(false); });
  // Collapse links only after their controller has initialized successfully.
  header.classList.add('navigation-ready');
});
