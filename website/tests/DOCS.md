# Documentation example checks

Every PR and master push runs these checks in the global test gate (`scripts/ci/run-go-tests.sh gate`). The gate reads the current files under `docs/hugo/content/`. For focused checks from the repository root:

```sh
go test -count=1 ./website/tests/docscheck
python3 website/tests/check_docs.py
python3 website/tests/check_quick_start.py --toolchain go1.26.10
python3 website/tests/check_embedded.py --toolchain go1.26.10 --candidate
```

The current docs inventory is 82 Markdown pages (41 English/Chinese pairs). Historical snapshot directories are unchanged. No shared stylesheet, template, logo or homepage design changes are part of this repair.

`check_docs.py` executes 132 Bash blocks against owned native PMS/ADS, mock assertion and custom-function services, including Allow/Deny, diagnosis, discovery/import/enforcement, TLS/mTLS and rejected credentials. Two deployment clone/build blocks are identical to the Quick Start build sequence, which the release check executes separately. Linux uses private user/network/PID namespaces and the documented ports; macOS remaps only the owned fixture ports. All fixture processes and temporary policy/certificate files are cleaned up.

Go contract tests check the published OpenAPI methods against native routers/responses, bilingual schema parity, webhook JSON, SPDL conditions, overview RBAC/ABAC/PBAC/IDD examples, and the inline Kubernetes manifest against current `apps/v1` and core `v1` types. Public proto files are checked byte-for-byte against the current server source.

Container image pulls and Kubernetes admission/rollout require a target registry/cluster and are **not executed by this suite**. Environment-dependent Bash templates receive syntax checks, and manifests receive typed structural checks. These are not claims of cluster acceptance.

| Page pair | Review/result | Verification |
| --- | --- | --- |
| `_index` | Reviewed; retained | Source review; no obsolete build/API example found |
| `api/_index` | Updated in both languages | Native router/schema contracts; source proto parity |
| `api/asserter_api` | Updated in both languages | Native router/schema contracts; source proto parity |
| `api/decision_api` | Updated in both languages | Native router/schema contracts; source proto parity |
| `api/management_api` | Updated in both languages | Native router/schema contracts; source proto parity |
| `sdks/java`, `sdks/python`, `sdks/go`, `sdks/dotnet`, `sdks/rust`, `sdks/node` | Added in both languages (moved from `api/sdk-*` in #208) | Not run by this suite. Each example was run by hand from the published package (Node.js: the packed SDK) against a local self-hosted Speedle ADS; commands and output are in the PR |
| `sdks/_index` | Added in both languages | Example policy text and `spctl` commands moved unchanged from the `api/_index` SDKs section |
| `deployment/docker` | Moved from `sdks/docker` in both languages | Commands were run against the published `docker.io/speedle/speedle:1.7.0` image. `ghcr.io/teramoby/speedle:1.7.0` was pulled and inspected; its index digest matches |
| `architecture` | Reviewed; retained | Source review; no obsolete build/API example found |
| `assertor` | Updated in both languages | Paired native commands and API results |
| `custom-function` | Updated in both languages | Paired native commands and API results |
| `decisions` | Updated in both languages | Paired native commands and API results |
| `deployment/embedded` | Moved from `deployment-embeded` in both languages | Public-module and candidate-library walkthrough checks |
| `deployment/_index` | Section landing; content retained from `deployment` | Shared Quick Start build, native services, typed manifest; image pulls and cluster rollout excluded |
| `deployment/homebrew` | Added in both languages | Install and local demo commands. This suite does not execute them |
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
| `store/_index` | Updated in both languages | Paired native commands and API results |
| `store/mysql` | Added in both languages | Docker MySQL 8.4, native PMS/ADS, provisioning, live updates and restart checks |
| `store/etcd` | Added in both languages | External Docker etcd, shared prefix, Allow/Deny, watch updates, snapshot and container restart |
| `store/mongodb` | Added in both languages | Docker MongoDB replica set, shared database, Allow/Deny, change stream updates, stored documents, backup and container restart |
| `usecases` | Reviewed; retained | Source review; no obsolete build/API example found |
| `versions` | Updated in both languages | Published release/tag links |

`check_mysql.py` executes both MySQL setup guides against owned Docker containers and named volumes, with native race-enabled PMS/ADS. It checks account grants, automatic schema creation, read-only ADS, allow/deny, polling propagation, process restart, and prebuilt schemas after revoking DDL privileges. The charming MySQL nightly runs this checker; the global documentation gate checks its paired commands and Bash syntax.

`check_etcd.py` executes the paired external-etcd setup guides using disposable Docker containers/volumes and native race-enabled PMS/ADS. It checks startup, initial policy load, live watch updates, stored keys, snapshot creation/export/status, and authorization persistence across an etcd container restart. TLS configuration JSON is checked statically; external production certificates and multi-member operation require environment-specific verification. The nightly suite runs it.

`check_mongodb.py` executes the paired MongoDB setup guides against an owned single-node replica set in Docker, with native race-enabled PMS/ADS. It checks replica set initialization, initial policy load, live change stream updates, the stored service document, a `mongodump` archive, and authorization persistence across a MongoDB container restart. The production connection string is checked statically; authentication, TLS and multi-member operation require environment-specific verification. The nightly suite runs it.
