# Playground validation — 2026-10-26

All Playground release checks below passed, including the compact header and
empty-array type validation fix. An additional root-repository regression found
one independently reproduced baseline failure. The entire repository and a newly
published website are not claimed as green.

## Environment and provenance

- macOS darwin/arm64; official Go 1.26.3, archive SHA-256 verified before execution.
- Node 26.11.1; real Google Chrome 155.0.8059.86 via Playwright.
- Reviewed bundle: `e7930d76ec293d66040727f0175a514062ce1ad2`.
- Unmodified upstream engine source: `5cccf4c4e50a2b0ab901a9fa65acc39074ed1068`.
  `api/`, `pkg/`, `3rdparty/`, root `go.mod` and root `go.sum` still match this pin.

The original bundle's WASM, Go runtime and distributed licenses were rebuilt with
the exact pinned toolchain and matched the committed artifacts byte for byte.
Original WASM SHA-256:
`f146f86961dfb867a707da68846d172226c059932d90fe1a3ffb2ce0f30633c6`.

After the adapter fix, two successive builds produced the same final artifacts:

| Artifact | SHA-256 |
| --- | --- |
| `docs/playground/speedle.wasm` | `307a0a54d0365838036f16a592f9ac6e2c5bf40f4f990d1ef8d35b062735fd90` |
| `docs/playground/wasm_exec.js` | `0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14` |

## Passing Playground checks

| Check | Result |
| --- | --- |
| Exact Go 1.26.3 build and complete native adapter suite | Passed; includes 12 new type-validation cases |
| Complete adapter suite inside Go's actual WASM runtime | Passed |
| Adapter `go vet ./...` | Passed |
| Typed request model and production WASM | 57 checks passed |
| Production form interaction and race handling in DOM shim | 17 checks passed |
| Static homepage, navigation, footer and asset invariants | Passed |
| Independent production-WASM boundaries and semantics | 91 checks passed; zero failures after fix |
| Supplied real-browser site and worker smoke scripts | Both passed against final WASM |
| Extended real-browser failure recovery and layouts | 13 grouped checks passed against final WASM |
| HTML/CSS/module references and fragment targets | 79 references resolved; no missing assets |
| Sitemap, robots and CNAME | 17 unique sitemap routes present; Playground included |
| Compiled dependencies and distributed licenses | Complete; license text matches source/module files |
| Bash/JavaScript syntax and workflow schema/expressions | Passed; actionlint 1.7.12 |
| Diff validation | `git diff --check` passed |

Boundary checks covered exactly 256 KiB including UTF-8, 500 statements, 200
principals, 4096-byte conditions and 1024-byte resource regexes, plus rejection
immediately beyond each limit. They covered RBAC, ABAC, explicit/default denial,
identity domains, anonymous requests, typed values, malformed input and repeated
evaluation isolation.

An unknown, omitted or null attribute type previously bypassed validation when
its value was an empty array. Type validation now precedes array conversion.
All three invalid cases return request errors without `allowed`; all four valid
empty-array types and ordinary scalar values remain accepted. Native, WASM and
real-browser API checks confirmed the fix.

Real Chrome checks covered HTTP 503 during WASM loading, worker runtime failure,
the actual five-second evaluation deadline and actual 45-second startup deadline.
Each failure recovered through a fresh worker. Edits and sample changes discarded
stale responses; duplicate evaluation was ignored; errors never appeared as an
ordinary `false` decision. Compact headers, mobile menus and real decisions passed
at 320, 390, 768, 1024 and 1440 pixels without horizontal overflow. The extended
suite observed 84 same-origin static GET requests, no uploads and no exceptions.
These are desktop Chrome viewport checks, not physical mobile-device or
screen-reader certification.

`actionlint` checked workflow schema and expressions. Its optional external
shellcheck/pyflakes integrations were unavailable; the build script separately
passed `bash -n`, and JavaScript files passed `node --check`.

## Reproduce

Use Go 1.26.3 and run from the repository root:

```sh
website/playground-engine/build.sh
node website/playground-engine/tests/form-model.test.cjs
node website/playground-engine/tests/form-dom.test.cjs
node website/playground-engine/tests/site.test.cjs

cd website/playground-engine
go vet ./...
GOOS=js GOARCH=wasm go test -count=1 -mod=readonly \
  -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./playground
```

From the repository root, the workflow's artifact gate must remain clean after
committing the corresponding source and generated binary together and rebuilding:

```sh
git diff --exit-code -- docs/playground/speedle.wasm \
  docs/playground/wasm_exec.js docs/playground/licenses
test -z "$(git ls-files --others --exclude-standard -- docs/playground/licenses)"
```

The two supplied browser commands are in [README.md](README.md). Extended boundary
and failure harnesses, screenshots and raw logs were retained with the review's
verification artifacts outside the repository.

## Additional root-repository regression

All three existing command binaries built. Root
`go vet -composites=false ./pkg/... ./cmd/... ./api/...` and `go mod tidy -diff`
passed with Go 1.26.3.

This additional related-engine regression command did not fully pass:

```sh
STORE_TYPE=file go test -json -count=1 -timeout=120s \
  ./pkg/eval/... ./pkg/pdl ./pkg/store/file ./api/...
```

Evaluator, built-in function and SPDL tests passed. API packages had no tests.
File-store `TestWatch` failed at `pkg/store/file/fileStore_test.go:479` with
`Not receive enough event`; it was not skipped or treated as passing.

The untouched pinned source was exported into an independent temporary directory.
`go test -run '^TestWatch$' -count=2 -timeout=30s -v ./pkg/store/file` failed in both
runs with the same native Remove/Create events and missing reload. Relevant source
is unchanged. Atomic-rename writes combined with watcher handling that does not
reload on the observed Remove/Create path are consistent with the failure on this
host. The Playground uses an ephemeral in-memory cache and does not invoke
file-store watching. External-store and full service integration suites were
outside this feature's verification scope; the entire root CI matrix is not
claimed as passed.

## Publishing state

Read-only checks confirmed legacy Pages publishing from `master:/docs`,
`speedle.io`, enforced HTTPS and a verified domain. Current deployment remained
`5cccf4c4e50a2b0ab901a9fa65acc39074ed1068`; production `/`, `/robots.txt` and
`/sitemap.xml` returned HTTP 200, while `/playground/` returned HTTP 404 because the
feature has not been published. No push, merge, Pages/DNS setting change or
deployment was performed. GitHub-hosted CI and post-publication checks of the new
page require a later push/publication and are not represented as completed here.
