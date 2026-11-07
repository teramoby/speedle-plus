# Request form QA

Run from the repository root with Node 22 or later (no packages/browser needed):

```sh
node website/playground-engine/tests/form-model.test.cjs
node website/playground-engine/tests/form-dom.test.cjs
node website/playground-engine/tests/site.test.cjs
```

- `form-model.test.cjs`: 57 checks for typed scalar/array serialization,
  validation, identity domains, anonymous requests, date/time normalization,
  and six real decisions using `docs/playground/speedle.wasm`.
- `form-dom.test.cjs`: 17 checks against production `docs/playground/app.js`
  in a lightweight DOM shim: samples, control labels, validation/focus,
  add/remove, stale results, duplicate runs and engine retry. Engine transport
  is stubbed only in this interaction test.

- `site.test.cjs`: exact reuse of original CSS/homepage, top navigation, official
  footer, asset paths and browser-only storage/transport invariants.

These tests do **not** replace real-browser rendering, mobile native-control,
assistive-technology or module-worker integration QA. See the parent README for
native/WASM adapter tests and the optional Chromium module-worker smoke test.

## Home-page axe contrast check

With Playwright and axe-core installed, run this from the repository root:

```sh
node website/playground-engine/a11y-smoke.cjs
```

It serves the published `docs/` tree, runs axe's `color-contrast` rule on the
English and Chinese home pages at 1440, 390 and 320 px, and fails on any
violation. Gradient and pseudo-element nodes that axe cannot resolve are
reported as unchecked; `site.test.cjs` separately guards the measured design
colours against WCAG AA.

## Documentation sidebar browser regression

With Playwright and Chromium installed, run this from the repository root:

```sh
node website/playground-engine/docs-sidebar-smoke.cjs
# Or check a temporary Hugo output directory:
node website/playground-engine/docs-sidebar-smoke.cjs /tmp/speedle-site
```

`CHROMIUM_PATH` can select an installed Chrome/Chromium binary. The test derives
menu depth from the bilingual content tree, visits every generated EN/ZH docs
page at 1440 and 1024 px, page-scrolls to every sidebar link, rejects limited
inner overflow, checks indentation at every depth, and verifies that the footer
follows the taller column. It also opens representative short-index, SDK and
Cloud drawers in both languages at 390 px and scrolls to every expanded link.
