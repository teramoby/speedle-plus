# Speedle self-hosted runner

`charming-speedle` is a Linux x64 runner registered to the `teramoby` organization.
Its dedicated `Speedle` runner group allows only `teramoby/speedle-plus`, including
this public repository. It does not reuse or change `pavulabs` registrations or
the `teramoby` default group's access policy.

Jobs select the labels `[self-hosted, Linux, X64, charming-speedle]`. The custom
label is required: a runner name alone is not a scheduling label. Organization
administrators should keep this label exclusive to the intended instance.

The group also restricts access to these exact default-branch workflows:

- `teramoby/speedle-plus/.github/workflows/go-ci-worker.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot.yml@refs/heads/master`
- `teramoby/speedle-plus/.github/workflows/reviewbot-command.yml@refs/heads/master`

`ci.yml` calls the reusable Go worker at `@master`. Jobs are defined in that
trusted callee, which derives runner eligibility and checkout SHA from the
original GitHub event, not caller inputs. A fork editing its own workflow cannot
directly select this runner or change the trusted worker's routing policy.

| Workload | Runner |
| --- | --- |
| Go CI push, manual dispatch and same-repository PRs, including Dependabot | `charming-speedle` |
| Go CI fork PRs or events outside the configured routing cases | GitHub-hosted |
| ReviewBot human-authored same-repository PR analysis/publication | `charming-speedle`, matching Pavu's policy |
| ReviewBot fork/bot PR analysis/publication and dependency merge job | GitHub-hosted |
| ReviewBot regression dispatch and trusted default-branch command jobs | `charming-speedle` |
| Unprivileged inline-comment pointer workflow | GitHub-hosted |

Fork code must not be routed to this persistent host. The inline pointer is also
hosted because its event can use a PR merge revision; privileged command handling
remains in the trusted default-branch receiver.

## Requirements

- Current GitHub runner software supporting Node 24-based actions (2.327.1 or newer).
- Git, Bash, Python 3, curl and jq available to the runner service account.
- Node 22 for shell regression tests, provisioned with pinned `actions/setup-node`.
- Go 1.26 for CI, provisioned with `actions/setup-go`.
- Writable Actions tool cache and Go build/module caches, plus outbound HTTPS to
  GitHub, action/tool downloads, Go modules and the existing Ark endpoint.

The ReviewBot manual regression job checks the actual runner identity and tools,
checks Ark HTTPS reachability without credentials, performs checkout and runs the
real workflow fixtures. It does not call a model or publish a Review.

One runner instance executes one job at a time. Other runner processes on the same
physical machine may still run concurrently; separate work directories do not
isolate ports, CPU, memory or credentials. Do not stop or reconfigure Pavu services
to make a Speedle check pass.

Go tests can use etcd ports 2379/2380, MongoDB 27017, PMS 6733 and ADS 6734.
Self-hosted test jobs verify wildcard and loopback IPv4/IPv6 bindings and refuse
to proceed if another service is listening. Packages execute serially
(`go test -p 1`), inherited CLI/admin/TLS settings are cleared, and test endpoints
are fixed to loopback. No existing service is reused or stopped. Package-list
and test failures propagate to the job instead of being discarded with `|| true`.

## Verify scheduling

```sh
gh workflow run reviewbot.yml --ref master
gh workflow run ci.yml --ref master
```

Inspect each run's jobs and confirm `runner_name` is `charming-speedle` for the
self-hosted paths. An online/idle runner listing alone does not prove checkout,
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
