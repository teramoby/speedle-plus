# Speedle self-hosted runners

`charming-speedle` and `charming-speedle-2` are Linux x64 runners registered to the
`teramoby` organization. Their dedicated `Speedle` runner group allows only
`teramoby/speedle-plus`, including
this public repository. It does not reuse or change `pavulabs` registrations or
the `teramoby` default group's access policy.

`fiona-speedle` (macOS ARM64) is a third runner in the same group. All three
carry `speedle-runner`; only the two charming instances carry `speedle-linux`.

ReviewBot and Go lint, test and post-merge regression select
`[self-hosted, speedle-runner]`, so any of the three may take them. Go build
(it needs the Linux Hugo release and `sha256sum`) and the nightly (it needs
Docker) select `[self-hosted, Linux, X64, speedle-linux]`. The weekly selects
`[self-hosted, Linux, X64, charming-speedle]`, exclusive to the original instance.
A runner administrator must keep `speedle-runner` and `speedle-linux` exclusive
to these named instances; adding another host would also admit it to
credential-bearing jobs. A runner name alone is not a scheduling label. The two
charming registrations share a physical host, so fixed-port tests there take
the host test lock (`/tmp/speedle-plus-test-services.lock`) and never overlap.
Each registration must use its own runner installation and work directory.

The `speedle-control` label belongs only to `charming-speedle-2`. ReviewBot's
dependency merge waiter uses it so waiting for CI never occupies the only
fixed-port test runner. Keep this label off the first runner.

The group also restricts access to these exact default-branch workflows:

- `teramoby/speedle-plus/.github/workflows/go-ci-worker.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot-command.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/mongodb-nightly-validation.yml@refs/heads/master`

`ci.yml` calls the reusable Go worker at `@master`. Jobs are defined in that
trusted callee, which derives runner eligibility and checkout SHA from the
original GitHub event, not caller inputs. The validated task selects only between
the two fixed label lists after that eligibility check. A fork editing its own workflow cannot
directly select this runner or change the trusted worker's routing policy.

| Workload | Runner |
| --- | --- |
| Go lint for repository human/bot pushes, manual dispatch and human/bot same-repository PRs | Any runner via `speedle-runner` |
| Go build (including the site checks in `verify.sh`) for those trusted events | Either charming runner via `speedle-linux` |
| Go tests for those trusted events, both file/etcd modes | Any runner via `speedle-runner`, serialized by the host test lock |
| Go CI fork PRs, unknown actors or other events | GitHub-hosted |
| Post-merge file-store regression (`post-merge.yml`, every push to master) | Any runner via `speedle-runner`, serialized by the host test lock; its report job is GitHub-hosted |
| ReviewBot human/bot same-repository PR analysis/publication | Any runner via `speedle-runner` |
| ReviewBot fork PRs or unknown authors | GitHub-hosted |
| Dependency auto-merge waiter | `charming-speedle-2` via `speedle-control` |
| ReviewBot regression dispatch and authorized command analysis/publication | Any runner via `speedle-runner` |
| Public comment authorization and unprivileged inline-comment pointer | GitHub-hosted |

Fork code must not be routed to this persistent host. The inline pointer is also
hosted because its event can use a PR merge revision; privileged command handling
remains in the trusted default-branch receiver.

Unlike Pavu's current command workflow, the initial authorization job stays
hosted so arbitrary public comments cannot queue work on the self-hosted pool.
Only commands passing the existing API permission check reach its self-hosted
analysis and publication jobs; supported commands and permissions are unchanged.

By maintainer policy, Go CI deliberately trusts code in human- and bot-authored
same-repository PRs, including Dependabot updates before merge, repository-owned
pushes and changes merged into the default branch. `go test` and `go build`
execute admitted code on the
persistent runner account, which can access its caches and other host resources.
On a shared runner account, admitted code can inspect files and concurrent
processes, including those of ReviewBot; changes it leaves can also affect later
jobs. Separate work directories and pooling do not provide a security boundary.
Use separate OS accounts with restrictive permissions or isolated hosts if the
code admitted by this policy cannot be trusted with the runner account's access.
The runner group restriction prevents forks from changing the trusted routing
workflow; it does not sandbox code admitted by that workflow. Repository write
access and dependency changes therefore remain part of the host's trust boundary.
ReviewBot analysis only reads the diff and does not execute PR code.

## Requirements

- Current GitHub runner software supporting Node 24-based actions (2.327.1 or newer).
- Git, Bash, Python 3, curl and jq available to the runner service account.
- Node 22 for shell regression tests, provisioned with pinned `actions/setup-node`.
- Go 1.26 for CI, provisioned with `actions/setup-go`.
- Writable Actions tool cache and Go build/module caches, plus outbound HTTPS to
  GitHub, action/tool downloads, Go modules and the existing Ark endpoint.

Self-hosted Go jobs use the host's existing module/build caches without restoring
a GitHub cache archive over them. Hosted fallback jobs retain Actions caching.

The ReviewBot manual regression job checks the actual runner identity and tools,
checks Ark HTTPS reachability without credentials, performs checkout and runs the
real workflow fixtures. It does not call a model or publish a Review.

One runner instance executes one job at a time. Other runner processes on the same
physical machine may still run concurrently; separate work directories do not
isolate ports, CPU, memory or credentials. Do not stop or reconfigure Pavu services
to make a Speedle check pass.

The runner performs its normal job process cleanup (visible as `Cleaning up
orphan processes` in the job log). This and checkout cleanup are housekeeping,
not containment for hostile code. Do not kill processes merely because they
listen on a test port: they may belong to another runner or host service.

Go tests can use etcd ports 2379/2380, MongoDB 27017, PMS 6733 and ADS 6734.
Self-hosted test jobs verify wildcard and loopback IPv4/IPv6 bindings and refuse
to proceed if another service is detected at the time of the check. This is a
preflight check, not a reservation: another process can bind a port afterwards.
Fixed-port test jobs on other runner registrations must not overlap these tests;
separate network namespaces or dedicated hosts are needed for that isolation.
Packages execute serially
(`go test -p 1`), inherited CLI/admin/TLS settings are cleared, and test endpoints
are fixed to loopback. The workflow never stops an existing service. Package-list
and test failures propagate to the job instead of being discarded with `|| true`.

## Verify scheduling

```sh
gh workflow run reviewbot.yml --ref master
gh workflow run ci.yml --ref master
```

Inspect each run's jobs and confirm `runner_name` is one of the three Speedle
runners for `speedle-runner` jobs, a charming runner for build and nightly,
and only `charming-speedle` for the weekly.
The regression job prints its workspace and temp paths; verify both registrations
have distinct paths. An online/idle runner listing alone does not prove checkout,
tool setup, tests or model connectivity work. The dedicated routing regression
and protected-worker suites are:

```sh
node --test scripts/tests/test_runner_routing.cjs scripts/tests/test_go_ci_worker.cjs
```

Testing a changed ReviewBot workflow on a branch requires an administrator to
temporarily allow that exact reviewed branch in the runner group and remove it
after validation. Do not allow PR merge refs or wildcard workflow revisions.

The workflows limit GitHub tokens to required permissions and do not persist Git
credentials during CI checkout. `.github/actionlint.yaml` declares the custom
label for workflow validation.

## Database regression allocation

Every test runs on these runners; only the `changes` file classification and
the token-holding `report` jobs are GitHub-hosted. The nightly worker runs the
same full regression, `scripts/ci/store_regression.py` (full Go and native
PMS/ADS runtime stages), once per backend on a charming runner: file directly,
etcd on a temporary etcd from `validate-etcd.sh`, MySQL on an owned Docker
container from `run-mysql-tests.py`, and MongoDB on a temporary native replica
set from `validate-mongodb.sh`. Its results reach the nightly report.

`mongodb-nightly-validation.yml` runs the MongoDB regression on demand through
`workflow_dispatch`. It selects the `speedle-runner` label, which charming,
charming-2 and fiona (macOS ARM64, also in the `Speedle` group) carry, accepts
macOS ARM64 or Linux x64, and shares the nightly's concurrency group, so the two
never run at once on the etcd ports.

Shared integration fixtures select `file`, `etcd`, `mongodb` or `mysql` through
`STORE_TYPE`. `SPEEDLE_TEST_STORE_PROPS` supplies backend connection properties;
see `docs/MYSQL_TESTS.md` for isolation, optional-capability checks and artifacts.

## Fiona full nightly trial

The manual MongoDB validation workflow accepts `nightly_trial: true` to run
the complete nightly script on `fiona-speedle` (macOS ARM64), including all
four stores, executable guides, supported Go releases and CI script checks.
It uses the nightly host lock, port preflight and sanitized test environment;
results are uploaded as `fiona-nightly-results`. Its concurrency group remains
`nightly-tests`. Use `install_flock: true` if the existing macOS host lacks
`flock`. No scheduled nightly routing changes until this trial passes.

## Fiona CI trial

`ci_trial` on the same workflow runs one CI task on `fiona-speedle` with the
commands CI uses: `lint` (go vet, go mod tidy check), `test` (the merge gate,
`run-go-tests.sh gate`), `regression` (the post-merge regression) or `reviewbot`
(ReviewBot's tool and Ark reachability checks and its fixture suites, with no
credentials). `test` and `regression` hold the host test lock. All four passed on
fiona before CI routed these tasks to `speedle-runner`; `build` stays on Linux
because it needs the Linux Hugo release and `sha256sum`.
