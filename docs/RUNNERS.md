# Speedle self-hosted runners

The repository uses two self-hosted Linux x64 runners and one self-hosted
macOS ARM64 runner, registered to the `teramoby` organization. Their dedicated
`Speedle` runner group allows only `teramoby/speedle-plus`, including this
public repository. It does not reuse or change `pavulabs` registrations or
the `teramoby` default group's access policy.

Every Speedle runner carries `speedle-runner`; the Linux runners also carry
`speedle-linux`. The macOS runner carries a dedicated scheduling label that the
weekly workflow pins; the exact label strings appear in the workflow `runs-on`
values and in `.github/actionlint.yaml`.

Weekly runs stay on the macOS runner through its dedicated scheduling label.

ReviewBot selects `[self-hosted, speedle-runner]`, which all three runners
carry, and its regression fixture job accepts all three. Its two model-calling
jobs wait for a GitHub-hosted preflight that
validates `REVIEWBOT_RUNS_ON` against one literal allowlist of label sets and
publishes the accepted set as the job output they route on, so an unapproved
override fails the run before a provider key reaches a runner. The allowlist
comparison is case-sensitive: a value that differs in case from an approved
set is rejected.
Go lint and test use the same label, and the Go CI worker accepts both Linux
runners and the macOS runner for those tasks. Go build, post-merge regression
and the nightly select `[self-hosted, Linux, X64, speedle-linux]`; their identity
check also requires Linux x64. Nightly needs Docker for MySQL and the storage
guides; build needs the Linux Hugo release and `sha256sum`. The weekly selects the macOS
runner's dedicated scheduling label to reduce timing noise in its benchmark
check, and its identity step fails any run on an OS or architecture other than
macOS ARM64, so a host that later carries that label elsewhere cannot silently
take the run.
The benchmarks (`scripts/ci/benchmark.py`) measure master as of May 2026 and
the current code alternately in the same run; each report names the runner that
measured it.
A runner administrator must keep `speedle-runner` and `speedle-linux` exclusive
to the registered instances; adding another host would also admit it to
credential-bearing jobs. A runner name alone is not a scheduling label.
Remove an identity from the workflow identity checks only after its
registration no longer carries any scheduling label; until then a job routed to
that label still queues there and fails at the identity check before it starts.
The routing regression derives the scheduling labels from every workflow
`runs-on` and requires its runner inventory to cover them. Setting
`SPEEDLE_RUNNERS_LIVE=1` also compares that inventory with the registration API
through `gh`; otherwise the live comparison is skipped, even when `GH_TOKEN` or
`GITHUB_TOKEN` is present. A registration change requires refreshing the
snapshot from the two `gh api` commands in that test.
Fixed-port tests take the host test lock
(`/tmp/speedle-plus-test-services.lock`), so registrations on the same physical
host never overlap. Each registration must use its own runner installation and
work directory.

The `speedle-control` label is carried by the two Linux runners; no workflow
currently routes on it.
ReviewBot's dependency auto-merge job runs on GitHub-hosted `ubuntu-24.04` and
groups its waiters per pull request, so an auto-merge wait never occupies a
self-hosted runner. See [ReviewBot](REVIEWBOT.md) for its merge gates.

The group also restricts access to these exact default-branch workflows:

- `teramoby/speedle-plus/.github/workflows/go-ci-worker.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot-command.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/mongodb-nightly-validation.yml@refs/heads/master`

`ci.yml` calls the reusable Go worker at `@master`. Jobs are defined in that
trusted callee, which derives runner eligibility and checkout SHA from the
original GitHub event, not caller inputs. The validated task selects only between
the three fixed label lists after that eligibility check. A fork editing its own workflow cannot
directly select this runner or change the trusted worker's routing policy.

| Workload | Runner |
| --- | --- |
| Go lint for repository human/bot pushes, manual dispatch and human/bot same-repository PRs | Any runner via `speedle-runner` |
| Go build (including the site checks in `verify.sh`) for those trusted events | Either Linux runner via `speedle-linux` |
| Go tests for those trusted events, both file/etcd modes | Any runner via `speedle-runner`, serialized by the host test lock |
| Nightly four-store regression and Docker storage guides (`nightly.yml`) | Either Linux runner via `speedle-linux`, serialized by the host test lock; its report job is GitHub-hosted |
| Go CI fork PRs, unknown actors or other events | GitHub-hosted |
| Post-merge file-store regression (`post-merge.yml`, every push to master) | Either Linux runner via `speedle-linux`, serialized by the host test lock; its report job is GitHub-hosted |
| Weekly tests and benchmark comparison (`weekly.yml`) | The macOS runner via its dedicated scheduling label, serialized by the host test lock; its report job is GitHub-hosted |
| ReviewBot human/bot same-repository PR analysis/publication | Any runner via `speedle-runner` |
| ReviewBot fork PRs or unknown authors | GitHub-hosted |
| Dependency auto-merge waiter | GitHub-hosted Ubuntu |
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

Inspect each run's jobs and confirm `runner_name` is one of the registered
Speedle runners for `speedle-runner` jobs: either Linux runner or the macOS runner.
The weekly runs on the macOS runner; build, nightly and post-merge regression
run on a Linux runner.
The regression job prints its workspace and temp paths; verify every
registration has a distinct path. An online/idle runner listing alone does not prove checkout,
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
scheduling labels for workflow validation.

## Database regression allocation

Every test runs on these runners; only the `changes` file classification and
the token-holding `report` jobs are GitHub-hosted. The nightly worker runs the
same full regression, `scripts/ci/store_regression.py` (full Go and native
PMS/ADS runtime stages), once per backend on the Linux pool with Docker: file directly,
etcd on a temporary etcd from `validate-etcd.sh`, MySQL on an owned Docker
container from `run-mysql-tests.py`, and MongoDB on a temporary native replica
set from `validate-mongodb.sh`. Its results reach the nightly report.

`mongodb-nightly-validation.yml` runs the MongoDB regression on demand through
`workflow_dispatch`. It selects the `speedle-runner` label, which all three
registered runners carry, accepts macOS ARM64 or Linux x64, and shares the
nightly's concurrency group, so the two never run at once on the etcd ports.

Shared integration fixtures select `file`, `etcd`, `mongodb` or `mysql` through
`STORE_TYPE`. `SPEEDLE_TEST_STORE_PROPS` supplies backend connection properties;
see `docs/MYSQL_TESTS.md` for isolation, optional-capability checks and artifacts.

## Report email

The nightly, weekly and post-merge regression workflows publish their report by
email from a GitHub-hosted `report` job. `scripts/ci/test_report.py` builds the
message and attaches the complete report as a text attachment; the transport is
the Cloudflare Email Service REST API. The report jobs read five repository
settings, and all five must be provisioned before these workflows run:

| Setting | Kind | Contents |
| --- | --- | --- |
| `CF_EMAIL_TOKEN` | secret | API token used to send the report |
| `CF_ACCOUNT_ID` | variable | the account that sends it |
| `OPS_EMAIL_FROM` | variable | the sender, exactly one address |
| `OPS_EMAIL_TO` | variable | the recipients, one or more addresses |
| `OPS_EMAIL_REPLY_TO` | variable | the reply address, exactly one |

Every setting is required. The sender is an outbound-only address that cannot
receive mail, so a report sent without a Reply-To loses any reply. A second
sender address, or an `OPS_EMAIL_TO` entry that is not an address, is a
configuration error rather than a recipient that is dropped silently.

The report jobs set `REPORT_EMAIL_REQUIRED: "true"`, so a missing or malformed
setting fails the job with an `::error::` annotation that names the setting,
after the step summary and the failure issues have been written. Of the
non-empty values, exactly `0`, `false` and `no` turn that requirement off,
compared case-insensitively after trimming whitespace; an empty or unset marker
is off as well. Every other non-empty value turns it on, so a misspelling such
as `ture` fails closed instead of silently dropping the guarantee. To make a
broken email channel a notice instead of a failed job, set the marker to one
of those three values (or remove it) in the report job's `env:` in the
workflow. The post-merge regression sends nothing when it passes, but it
still checks the settings and fails on a missing or malformed one. A local run
or a test without the settings only prints a notice and stays green. The step summary keeps the report for the run, up to 900000 bytes
and with the cut marked when the report is longer; the email carries the full
report as an attachment, capped at 17 MiB with a note when it had to be cut.

## Reports without results

The report job reads only the worker job's result and the contents of its
`*-results` artifact. A worker that never starts, or that fails before
`scripts/ci/run-go-tests.sh` creates its results directory, uploads nothing; an
incomplete upload can also leave an existing results directory with no readable
test results.
GitHub cancels a self-hosted job that stays queued for 24 hours: no eligible
runner accepted it in time, because runners were offline or busy, none
carried the job's labels, or the runner group's workflow or repository
restrictions kept them out. A failed runner identity or toolchain check ends
the job before any test runs.

Such a run is still reported as `FAILED` and tracked under
`Test failure: check results test-results`. Its one failed check names the
worker's result, for example `The run job ended with cancelled before any
result reached this report`, and the summary says that no test result was
read. This means no test ran or its results were not uploaded; it is not
evidence of a failing test. Open the worker job on the run page to see whether
it ran at all; if it did not, find out why no eligible runner took it and
rerun the job. The `Download results` step is allowed to fail so that the
report still runs, so its green check does not mean an artifact was found.

## macOS full nightly trial

The manual MongoDB validation workflow accepts `nightly_trial: true` to run
the complete nightly script on the macOS runner (pinning its scheduling label),
including all four stores, executable guides, supported Go releases and CI
script checks. It uses the nightly host lock, port preflight and sanitized test
environment; results are uploaded as a trial-results artifact. Its concurrency
group remains `nightly-tests`. Use `install_flock: true` if the existing macOS
host lacks `flock`. This explicit trial requires Docker Desktop running;
the normal nightly uses the Linux pool with Docker. On macOS,
`run-go-tests.sh` gives Docker a copy of the user's config without its keychain
credential store, which a runner service cannot open; the test images are
public.

## macOS CI trial

`ci_trial` on the same workflow runs one CI task on the macOS runner (pinning
its scheduling label) with the commands CI uses: `lint` (go vet, go mod tidy
check), `test` (the merge gate, `run-go-tests.sh gate`), `regression` (the
post-merge regression), `weekly` (`run-go-tests.sh weekly`) or `reviewbot`
(ReviewBot's tool and Ark reachability checks and its fixture suites, with no
credentials). `test`, `regression` and `weekly` hold the host test lock. Lint,
test and reviewbot run on any Speedle runner; normal post-merge regression runs
on Linux. Weekly trials and the
scheduled weekly both stay on the macOS runner for lower-noise benchmark
comparisons.
`build` stays on Linux because it needs the Linux Hugo release and `sha256sum`.
