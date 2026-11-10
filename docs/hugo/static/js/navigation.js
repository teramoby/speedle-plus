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
