# Playground release validation — 2026-10-26

Tracking: https://github.com/teramoby/speedle-plus/issues/77

This release uses unmodified Speedle+ core commit
`7f61995d3344e94ed614494e013dacdcfa491b26` and the reviewed compact-header
adapter, built with exact Go 1.26.3. Earlier review results in VALIDATION.md
describe a different artifact and are retained as historical evidence.

## Release artifact

Two independent builds from the pinned source archive produced identical WASM:
`62fe0a45dd8f9438c007af7e9fbbb349058d59dcce7864594c9f5c194d9dc14f`.
The adapter imports the actual Go parser/evaluator and runs in a browser Worker.
The original engine source is never patched for the browser build.

## Verified locally

- Complete native adapter suite and the same suite in Go's actual WASM runtime.
- Production-WASM typed request tests: 57 passed; DOM interaction/race tests: 17 passed.
- Independent production-WASM semantic and input-boundary checks: 91 passed.
- Original homepage/CSS invariants, footer, canonical, sitemap, asset references,
  and exact source/published asset parity passed.
- Real Chrome full-site and module-worker smoke checks passed, including all
  five samples, malformed policy errors, typed attributes, reset, principals,
  and 320–1440 px responsive layouts with no horizontal overflow.
- Extended real-browser failure/retry suite: 13 grouped checks passed, including
  actual 5-second evaluation and 45-second initialization timeouts, worker crashes,
  download failures, stale-result suppression and no policy uploads.
- Hugo 0.48 generation passed: complete editor, both language navigation menus,
  multilingual sitemap, and exact engine/runtime/CSS/license assets. Locally the
  original source was compiled for arm64, omitting its unavailable development-only
  Mage requirement; Linux CI uses the checksum-pinned official release binary.
- Runner scheduling and service-isolation regressions: 17 passed. Local socket
  checks required execution outside the macOS filesystem sandbox.
- Workflow schema and expressions: actionlint 1.7.12 passed; Bash syntax and
  `git diff --check` passed. Optional shellcheck/pyflakes were not installed.

The full Linux CI matrix and post-publication HTTPS checks
are required release gates. Their actual results will be recorded on issue #77;
they are not represented as completed by this local report.

Run `website/playground-engine/verify.sh` on the Linux runner for the complete
release gate. It builds from the pinned source, runs native/WASM/form suites,
renders the existing Hugo site into a temporary directory with a checksum-pinned
Hugo 0.48, and rejects changed or untracked generated assets. Pull requests and
pushes use the protected default-branch Go CI worker; manual checks use the existing Speedle Plus CI workflow on the same pool. No new
workflow or organization runner allowlist entry is needed.
