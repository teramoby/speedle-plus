# Speedle self-hosted runner

`charming-speedle` is a Linux x64 runner registered to the `teramoby` organization.
Its dedicated `Speedle` runner group allows only `teramoby/speedle-plus`, including
this public repository. It does not reuse or change `pavulabs` registrations or
the `teramoby` default group's access policy.

Jobs select the labels `[self-hosted, Linux, X64, charming-speedle]`. The custom
label is required: a runner name alone is not a scheduling label. Organization
administrators should keep this label exclusive to the intended instance.

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

Go store tests use fixed ports 2379/2380. Self-hosted test jobs first verify both
IPv4 and IPv6 ports are unused and refuse to proceed if another service is
listening. Packages execute serially (`go test -p 1`) so their embedded servers
do not overlap. No existing etcd service is reused or stopped. Test failures
propagate to the job rather than being discarded with `|| true`.

## Verify scheduling

```sh
gh workflow run reviewbot.yml --ref <reviewed-branch>
gh workflow run ci.yml --ref <reviewed-branch>
```

Inspect each run's jobs and confirm `runner_name` is `charming-speedle` for the
self-hosted paths. An online/idle runner listing alone does not prove checkout,
tool setup, tests or model connectivity work. The dedicated routing regression
suite is `node --test scripts/tests/test_runner_routing.cjs`.

The workflows limit GitHub tokens to required permissions and do not persist Git
credentials during CI checkout. `.github/actionlint.yaml` declares the custom
label for workflow validation.
