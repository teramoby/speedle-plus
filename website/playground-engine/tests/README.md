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
