# Speedle self-hosted runners

`charming-speedle` and `charming-speedle-2` are Linux x64 runners registered to the
`teramoby` organization. Their dedicated `Speedle` runner group allows only
`teramoby/speedle-plus`, including
this public repository. It does not reuse or change `pavulabs` registrations or
the `teramoby` default group's access policy.

ReviewBot and Go lint/build select `[self-hosted, Linux, X64, speedle-linux]`,
which both instances carry. Fixed-port Go tests select
`[self-hosted, Linux, X64, charming-speedle]`, exclusive to the original instance.
A runner administrator must keep `speedle-linux` exclusive to these two named
instances; adding another host would also admit it to credential-bearing jobs.
A runner name alone is not a scheduling label. Keep the test label exclusive so
two registrations on the same physical host cannot run these tests concurrently.
Each registration must use its own runner installation and work directory.

The `speedle-control` label belongs only to `charming-speedle-2`. ReviewBot's
dependency merge waiter uses it so waiting for CI never occupies the only
fixed-port test runner. Keep this label off the first runner.

The group also restricts access to these exact default-branch workflows:

- `teramoby/speedle-plus/.github/workflows/go-ci-worker.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot-command.yml@refs/heads/master`

`ci.yml` calls the reusable Go worker at `@master`. Jobs are defined in that
trusted callee, which derives runner eligibility and checkout SHA from the
original GitHub event, not caller inputs. The validated task selects only between
the two fixed label lists after that eligibility check. A fork editing its own workflow cannot
directly select this runner or change the trusted worker's routing policy.

| Workload | Runner |
| --- | --- |
| Go lint/build for repository human/bot pushes, manual dispatch and human/bot same-repository PRs | Either runner via `speedle-linux` |
| Go tests for those trusted events, both file/etcd modes | `charming-speedle` only, serializing fixed ports |
| Go CI fork PRs, unknown actors or other events | GitHub-hosted |
| ReviewBot human/bot same-repository PR analysis/publication | Either runner via `speedle-linux` |
| ReviewBot fork PRs or unknown authors | GitHub-hosted |
| Dependency auto-merge waiter | `charming-speedle-2` via `speedle-control` |
| ReviewBot regression dispatch and authorized command analysis/publication | Either runner via `speedle-linux` |
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

Inspect each run's jobs and confirm `runner_name` is `charming-speedle` or
`charming-speedle-2` for pooled jobs, and only `charming-speedle` for Go tests.
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

MySQL validation stays on charming. MongoDB validation selects
`[self-hosted, macOS, ARM64, speedle-macos]` (fiona is macOS ARM64).
The MongoDB launcher owns a temporary native replica set, and both launchers use
`scripts/ci/store_regression.py` for the same full Go and native PMS/ADS runtime
stages. It runs every day at 17:00 UTC (01:00 GMT+8), the same time as the MySQL
nightly, and on demand through `workflow_dispatch`.

Shared integration fixtures select `file`, `etcd`, `mongodb` or `mysql` through
`STORE_TYPE`. `SPEEDLE_TEST_STORE_PROPS` supplies backend connection properties;
see `docs/MYSQL_TESTS.md` for isolation, optional-capability checks and artifacts.
