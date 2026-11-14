# Contributing to the Speedle Project

We welcome your contributions! There are multiple ways to contribute.

## Promotion

If you like the Speedle project, please let your colleagues/friends know the project. You may consider to _star_ [the project at GitHub](https://github.com/teramoby/speedle-plus). So that more people can find the project.

And sharing us your success story with Speedle would be greatly appreciated if you don't mind.


## Discussion

Join the [speedle-users](https://groups.google.com/forum/#!forum/speedle-users) and [speedle-developers](https://groups.google.com/forum/#!forum/speedle-developers) Google Groups.

Join the [Speedle Discord community](https://discord.gg/uaY2xVsbbT). You can also reach the maintainers at [contact@speedle.io](mailto:contact@speedle.io).

## Issues

If you hit a bug or have an enhancement request then file a [GitHub issue](https://github.com/teramoby/speedle-plus/issues).

If you report a bug, please include:

1. Version of Speedle
2. Issue Description
3. Reproducible Environment and Steps
4. Expected behavior

## Security

Do not report security vulnerabilities in a public issue. Report them privately as described in [SECURITY.md](SECURITY.md).

## Code

We welcome code contributions.

Contributions to Speedle are made under the Universal Permissive License 1.0 (UPL-1.0). Please sign off your commits to certify that you have the right to submit your contribution under the Developer Certificate of Origin (DCO) 1.1. Retain existing copyright and license notices and identify any third-party material in your pull request.

Sign off each commit with `git commit -s` (see [DCO](DCO) and [UPL-1.0 LICENSE](LICENSE)), which adds a `Signed-off-by: Your Name <you@example.com>` trailer using your Git `user.name` and `user.email`. To add a sign-off to existing commits, run `git commit --amend -s` for the most recent commit or `git rebase --signoff origin/master` for a branch of commits.

Pull-request CI checks that every non-merge commit carries a `Signed-off-by` line whose email matches the commit author's email. Merge commits, including the ones GitHub creates for "Update branch" and when merging a pull request, are exempt from the check.

The process:

1. Sign off each commit with `git commit -s` (see [DCO](DCO) and [UPL-1.0 LICENSE](LICENSE))
2. Fork the repo
3. Fix an issue or create an issue and fix it
4. Create a Pull Request that fixes the issue
5. Watch your PR for pipeline results. If there are failures then fix them.
6. We will review your PR and merge as appropriate.

Pull requests also receive an automated static ReviewBot review. Collaborators
with write permission can request `@reviewbot review`, `@reviewbot recheck` or
`@reviewbot explain` in PR comments. See [ReviewBot](docs/REVIEWBOT.md) for its
commands, configuration and data boundaries.

## CI and the merge queue

Unlabeled pull requests run quick checks: formatting, vet, module tidiness,
build (including the container dispatcher without Docker), and tests for changed
Go packages under `pkg`, `cmd`, `api` and `deployment`. Quick checks use the file store,
compile service-backed store packages without starting services, and do not
apply the coverage gate. Changes under `docs/` or `website/` also run the
existing site build, browser/a11y checks and documentation walkthroughs in the
Linux build worker. Container, command and release paths retain image smoke
without a label: `deployment/container/**`, `cmd/**`,
`.github/workflows/release.yml`, `.github/workflows/image-smoke.yml` and
`scripts/release-smoke.sh`. Either full-CI label always runs image smoke.
DCO and automated review retain their own workflows.

Maintainers add `full-ci` or `merge-head` to run the full Go gate, integration
services, the 90.0% coverage gate, build and image smoke. Full test coverage is
independent of changed-file classification. Pushes to `master` always run full
CI; nightly and weekly schedules are unchanged. The stable `CI` check reports
the selected route, so a quick pass alone is not approval to merge. There is
currently no branch protection. The manual `full-ci`/`merge-head` procedure below
enforces full validation before merge; a green `CI` check alone does not enforce
that procedure, even if it is later configured as required.

Runner variables contain a JSON runner string or label array. `QUICK_RUNS_ON`
defaults to `"ubuntu-latest"` and supports Linux x64 or macOS ARM64 without
Docker. `HEAD_RUNS_ON`, when set, routes full jobs on a PR carrying `merge-head`
to that pool, including build; otherwise the existing task pools apply.
`HEAD_RUNS_ON` must therefore select approved Linux runners, as build's identity
check rejects macOS. Weekly schedules never use this PR-only override and keep
their approved runner. Labels are scheduling
hints, not a security boundary. All Go jobs use the allow-listed reusable worker
at `@master`, retaining trusted-event and runner-identity checks. A PR changing
that worker cannot exercise the new worker implementation until it reaches
`master`; runner routing tests validate those expressions locally meanwhile.
Image smoke runs on GitHub-hosted Ubuntu.

Adding either queue label cancels the preceding run in the same PR concurrency
group and starts full CI. Other label additions start no jobs and use an ignored
group. Removing a label starts no run and does not cancel or replace results.
The next push selects its route from the labels then present.

Maintainers merge in this order:

1. Update the queue-head branch with current `master` and resolve conflicts.
2. Add `full-ci` and `merge-head`; keep `merge-head` on only one PR at a time.
3. Wait for full CI, image smoke, DCO and review on the exact final head SHA.
4. Squash-merge with that head SHA matched (for example, `gh pr merge <PR>
   --squash --match-head-commit <SHA>`).
5. Delete the branch and remove the queue label. When handing over the head,
   explicitly cancel any unfinished old-head runs and wait for their runner
   slots to release; removing the label alone does not stop them.

Before changing routing, run `actionlint` on the modified workflows and
`node --test scripts/tests/test_ci_scope.cjs scripts/tests/test_go_ci_worker.cjs`.
Workflow and script changes run all CI script suites in the hosted `changes`
job on both the quick and full routes: `node --test scripts/tests/*.cjs` and
`python3 -m unittest discover -s scripts/tests -p 'test_*.py'`. Use an empty
`DOCKER_CONFIG` directory when running the Node suites locally so container
fixtures cannot read the host's Docker credentials.

The nightly suite runs the same full regression, every Go package plus the
tagged ADS authorization/cache tests against native PMS/ADS, once each with the
file, etcd, MySQL and MongoDB stores; pull request CI does not run MySQL or
MongoDB.
See [MySQL regression tests](docs/MYSQL_TESTS.md)
for runner selection, manual trials, fixture ownership and result artifacts.

## Releases

Maintainers publish a release by dispatching the Release workflow from `master`.
Image names, tags and the Docker Hub credentials are described in
[Releasing](docs/RELEASE.md).

## Code of Conduct

Follow the [Golden Rule](https://en.wikipedia.org/wiki/Golden_Rule). If
you'd like more specific guidelines see the
[Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/1/4/code-of-conduct.html)
