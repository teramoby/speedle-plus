# Documentation example checks

Every PR and master push runs these checks in the global test gate (`scripts/ci/run-go-tests.sh gate`). The gate reads the current files under `docs/hugo/content/`. For focused checks from the repository root:

```sh
go test -count=1 ./website/tests/docscheck
python3 website/tests/check_docs.py
python3 website/tests/check_quick_start.py --toolchain go1.26.10
python3 website/tests/check_embedded.py --toolchain go1.26.10 --candidate
```

The current docs inventory is 48 Markdown pages (24 English/Chinese pairs). Historical snapshot directories are unchanged. No shared stylesheet, template, logo or homepage design changes are part of this repair.

`check_docs.py` executes 134 Bash blocks against owned native PMS/ADS, mock assertion and custom-function services, including Linux/amd64 cross-compilation, Allow/Deny, diagnosis, discovery/import/enforcement, TLS/mTLS and rejected credentials. Two deployment clone/build blocks are identical to the Quick Start build sequence, which the release check executes separately. Linux uses private user/network/PID namespaces and the documented ports; macOS remaps only the owned fixture ports. All fixture processes and temporary policy/certificate files are cleaned up.

Go contract tests check the published OpenAPI methods against native routers/responses, bilingual schema parity, webhook JSON, SPDL conditions, overview RBAC/ABAC/PBAC/IDD examples, and the inline Kubernetes manifest against current `apps/v1` and core `v1` types. Public proto files are checked byte-for-byte against the current server source.

Container build/push and Kubernetes admission/rollout require a target registry/cluster and are **not executed by this suite**. Environment-dependent Bash templates receive syntax checks, and manifests receive typed structural checks. These are not claims of cluster acceptance.

| Page pair | Review/result | Verification |
| --- | --- | --- |
| `_index` | Reviewed; retained | Source review; no obsolete build/API example found |
| `api/_index` | Updated in both languages | Native router/schema contracts; source proto parity |
| `api/asserter_api` | Updated in both languages | Native router/schema contracts; source proto parity |
| `api/decision_api` | Updated in both languages | Native router/schema contracts; source proto parity |
| `api/management_api` | Updated in both languages | Native router/schema contracts; source proto parity |
| `architecture` | Reviewed; retained | Source review; no obsolete build/API example found |
| `assertor` | Updated in both languages | Paired native commands and API results |
| `custom-function` | Updated in both languages | Paired native commands and API results |
| `decisions` | Updated in both languages | Paired native commands and API results |
| `deployment-embeded` | Reviewed; retained | Public-module and candidate-library walkthrough checks |
| `deployment` | Updated in both languages | Shared Quick Start build, cross-build, native services, typed manifest; cluster rollout excluded |
| `idd` | Updated in both languages | Paired native commands and API results |
| `logging` | Updated in both languages | JSON syntax and current logging/config source review |
| `overview` | Reviewed; retained | Extracted policy evaluation tests |
| `pms/_index` | Reviewed; retained | Source review; no obsolete build/API example found |
| `pms/diagnosis` | Updated in both languages | Paired native commands and API results |
| `pms/discover` | Updated in both languages | Paired native commands and API results |
| `pms/global-policy` | Updated in both languages | Paired native commands and API results |
| `pms/policy-mgmt` | Updated in both languages | Paired native commands and API results |
| `security` | Updated in both languages | Paired native commands and API results |
| `spdl` | Updated in both languages | Extracted condition/parser tests |
| `store` | Updated in both languages | Paired native commands and API results |
| `usecases` | Reviewed; retained | Source review; no obsolete build/API example found |
| `versions` | Updated in both languages | Published release/tag links |
