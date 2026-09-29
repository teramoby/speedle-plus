# Ark ReviewBot

Speedle+ uses `.github/workflows/reviewbot.yml` to perform a static Ark Agent Plan review of each non-draft pull request from this repository. Fork pull requests are skipped so repository secrets are never exposed to untrusted workflow code.

## Repository configuration

- Secret `ARK_API_KEY`: an Ark Agent Plan API key.
- Variable `ARK_REVIEW_MODEL`: optional model selection; defaults to `kimi-k3`.

The workflow uses the Agent Plan Responses endpoint. If the secret is absent, it exits successfully without sending code or publishing a comment.

## Trust boundary

The workflow runs under `pull_request_target`, and loads its executable workflow, review prompt, and optional root `AGENTS.md` from the protected base revision. It checks out the pull-request merge ref only to generate a static diff. It never builds, tests, or executes pull-request code.

Each new commit updates one ReviewBot comment. Review input is capped at 750,000 bytes; truncation is reported as a validation gap.

## Dependabot auto-merge

A pull request is merged automatically with squash only when all of these conditions hold:

1. GitHub identifies the author as `dependabot[bot]`;
2. Ark ReviewBot returns the exact machine-readable `no-actionable-findings` verdict for the current head commit;
3. the review comment is published successfully;
4. the current merge commit passes all four Speedle Plus CI checks: `lint`, `test (file)`, `test (etcd)`, and `build`;
5. the pull request remains open, non-draft, and on the same head commit while the checks run.

Any finding, missing/failed check, changed commit, API failure, or timeout prevents the merge. Other pull requests are never auto-merged.

Remove the `ARK_API_KEY` secret or disable the workflow to stop future reviews and automatic dependency merges.
