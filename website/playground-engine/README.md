# Speedle.io Playground engine

The public page is `docs/playground/index.html`, served at
<https://speedle.io/playground/>. The site is static: policies, request principals,
identity domains and attributes remain in browser memory. There is no evaluation
backend, credential, API token or persistent request storage.

The production `docs/playground/speedle.wasm` compiles the **actual Speedle+ Go
parser and evaluation library**, not a JavaScript decision implementation.
`docs/playground/engine.js` and `engine-worker.js` provide transport and worker
lifecycle only. The adapter here calls `pkg/pdl.ParsePolicy/ParseRolePolicy`,
`pkg/eval.PolicyEvalImpl.Diagnose`, upstream role resolution, `govaluate` and the
upstream deny-overrides combiner. It uses a single ephemeral service for each run.

## Rebuild the checked-in artifacts

Requirements: Git, Go **1.26.3**, Bash, and Node 22 or later for tests. From the
repository root:

```sh
website/playground-engine/build.sh
# If Go is installed outside PATH:
GO_BIN=/path/to/go website/playground-engine/build.sh
```

`go.mod` replaces the Speedle+ dependency with this repository's root. The
underlying engine source (`api/`, `pkg/`, `3rdparty/`, `go.mod`, `go.sum`) is pinned
to commit `5cccf4c4e50a2b0ab901a9fa65acc39074ed1068`. The script checks that these
paths match the pinned revision before building; unrelated website edits are
allowed. Full Git history is required (`git fetch --unshallow` for a shallow
clone). To deliberately upgrade the engine, review the source change, update the
pin and adapter `Revision`, update `ENGINE-NOTICE.txt` and tests, rebuild, and
review the new artifact together.

The build runs native adapter tests, uses locked modules (`-mod=readonly`),
`-trimpath`, `-buildvcs=false` and `-ldflags='-s -w'`, and copies the matching Go
runtime and upstream/dependency licenses. Disabling VCS stamping makes the
artifact independent of unrelated website commits or checkout directory.
Artifacts are checked in so the existing static-site deployment does not need a
Go or Node build. Do not copy local caches, private hosting configuration, or
credentials into `docs/`.

## Verification

From the repository root:

```sh
# Typed form serialization and actual production WASM: 57 checks.
node website/playground-engine/tests/form-model.test.cjs
# Production app.js interaction/race handling in a DOM shim: 17 checks.
node website/playground-engine/tests/form-dom.test.cjs
# Original shared site CSS, homepage/navigation, footer and asset invariants:
node website/playground-engine/tests/site.test.cjs

# Adapter tests in the actual Go WASM runtime (not a JS engine stub):
cd website/playground-engine
GOOS=js GOARCH=wasm go test -mod=readonly \
  -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./playground
```

`.github/workflows/playground.yml` runs these checks independently of the existing
repository CI. It rebuilds the artifacts and fails if the result differs from the
checked-in WASM, matching runtime or license files. No test failures are ignored.
The DOM shim does **not** prove browser layout, assistive-technology support,
native form control behavior or module-worker integration.

For an optional real Chromium worker smoke test, install Playwright in your
normal development environment and run:

```sh
CHROMIUM_PATH=/path/to/chromium node website/playground-engine/browser-smoke.cjs
# Full form UI, site navigation, 320–1440 px layouts and browser-only transport:
CHROMIUM_PATH=/path/to/chromium node website/playground-engine/site-smoke.cjs
```

Both browser smoke scripts passed in real Chrome during the 2026-10-26 review,
including the compact page header. Additional browser checks exercised download
and worker failures, both deadlines, retry recovery and stale results. See
[VALIDATION.md](VALIDATION.md) for the tested artifact, results and the separate
pre-existing file-store regression failure. Re-run checks for later changes.
Set `SCREENSHOT_DIR=/tmp/playground-shots` to capture layout images during
`site-smoke.cjs`.

To inspect the complete site, serve the docs root, not only the playground
folder, so the shared site CSS and official header/footer images resolve:

```sh
python3 -m http.server 8080 --directory docs
# Open http://localhost:8080/playground/
```

## API and boundaries

```js
import { initEngine, evaluate } from './engine.js';
await initEngine();
const result = await evaluate({
  policies: 'grant user alice reader on book\ngrant role reader read book',
  request: {
    subject: { principals: [{ type: 'user', name: 'alice' }] },
    serviceName: 'playground', resource: 'book', action: 'read',
    attributes: [{ name: 'clearance', type: 'numeric', value: 4 }]
  }
});
```

A successful response has `{ok:true, allowed:boolean, reason, reasonCode, ...}`.
An input/evaluation error has `{ok:false, error:{stage,message,line?}, ...}` and
**no `allowed` key**. Real engine errors are not displayed as ordinary denials.
Initialization, fetch, worker-runtime and timeout failures reject the Promise;
a retry can initialize a fresh worker. Initialization has a 45-second timeout,
evaluation a 5-second deadline. Diagnostics are genuine upstream diagnostics,
not a fabricated evaluation trace; upstream may short-circuit or mark additional
applicable policies ignored.

The adapter adds editor validation for trailing SPDL text, malformed input,
condition errors and nonboolean conditions; the underlying Speedle+ source is
unchanged. One statement per line; blank lines and full-line `#` / `//` comments
are skipped. User/group/entity principals are caller-provided test identities,
not authenticated users. Roles are derived by role policies. Typed attributes
accept string, numeric, bool and datetime scalars or arrays. Runtime `request_*`
attributes cannot be spoofed. Time-based functions use the browser/engine clock.

Limits: 256 KiB input, 500 statements, 200 principals, upstream 4096-byte
conditions and 1024-byte resource regexes. No external custom functions, token
assertion, persistent stores, discovery, multiple service stores or external ADS
connection. This is a learning sandbox, not a production enforcement point.

Licenses and provenance are served under `docs/playground/licenses/` and
`docs/playground/ENGINE-NOTICE.txt`. The wrapper source is covered by the
repository's Universal Permissive License (`LICENSE`).

## Website integration and deployment

The homepage and Playground both load `docs/css/site.css`, extracted byte for
byte from the original homepage's inline stylesheet. `navigation.css` only
extends the existing menu breakpoint to accommodate the additional top-level
link. The Playground keeps the official header/footer and logo assets; its own
stylesheet adds editor/form/result layout using the shared brand variables.
No private Sites hosting configuration is used.

The existing `docs/CNAME` is `speedle.io`; committed static assets under `docs/`
are ready for the site's existing GitHub Pages publishing flow. This change does
not change Pages settings, DNS or deployment permissions. Read-only checks on
2026-10-26 confirmed legacy GitHub Pages publishing from `master:/docs`, the
`speedle.io` custom domain and enforced HTTPS. This feature has not been pushed or
published; its production route still returns HTTP 404. Verify the deployed route
after publication.
