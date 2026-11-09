# ReviewBot

ReviewBot performs static pull-request reviews for Speedle. It uses the same
workflow and response-validation design as
[Pavu at a7b458e](https://github.com/pavulabs/pavu/tree/a7b458ee3194865b653826a1b0c1119c28634c9d),
with Speedle's Go/authorization review rules and repository-owned runners for
human and bot PRs.

## Automatic review

Opening, updating, reopening or marking a PR ready for review starts analysis.
Draft PRs are skipped. Each new head commit receives a formal `COMMENT` Review
with the commit SHA and actual model ID. An existing bot review with the exact
commit and marker suppresses duplicate model calls. Older reviews remain visible.
Reports use Pavu's rendered Markdown headings and `ReviewBot` signature. The
shared response parser removes a model-added fence around a complete report,
while retaining code examples, verdict markers, commit metadata and model IDs.

The workflow reads its instructions from the trusted base revision, checks out
GitHub's persistent PR head ref (including fork PRs), and verifies that checkout
equals the event's captured immutable head before constructing a merge-base diff. It does not
execute, build or test PR code. By default, human- and bot-authored same-repository PRs run
on any self-hosted Speedle runner via `speedle-runner` (`charming-speedle`,
`charming-speedle-2` and `fiona-speedle`), while `REVIEWBOT_RUNS_ON` can override the
runner labels of only the two model-calling jobs (`Analyze pull request` and
`Analyze ReviewBot command`). Fork PRs and unknown author types always use GitHub-hosted
`ubuntu-24.04`. Review publication, regression dispatch and comment-command
publication continue to use the `speedle-runner` pool, and public comment authorization and the
unprivileged inline pointer workflow remain hosted.
See [runner configuration](RUNNERS.md).

The dependency auto-merge waiter uses `speedle-control`, exclusive to
`charming-speedle-2`, so a waiting merge occupies only that runner while the
other two take the CI jobs it waits for. All review and CI merge gates
remain required.

## Comment commands

Collaborators with GitHub `write`, `maintain` or `admin` permission can write:

```text
@reviewbot review
@reviewbot review focus on authorization and tenant boundaries
@reviewbot recheck
@reviewbot check
@reviewbot explain why could this race?
Is this policy update safe? @reviewbot
```

A bare mention means `review`; `check` means `recheck`; other text becomes an
`explain` question. Mentions are case-insensitive. Commands work in ordinary PR
comments and code-line discussions, including on Draft PRs. The commenter must
be a human GitHub user, the PR must still be open, and its head must belong to
this repository. Commands on fork PRs are skipped.

`review` and `recheck` publish formal `COMMENT` Reviews at the captured head;
`explain` replies at the original comment location or code-discussion thread.
Recheck includes up to five recent bot outputs, capped at 80,000 characters,
including legacy Speedle ReviewBot issue comments. Each triggering comment is
handled once. An eyes reaction acknowledges an authorized command; a thumbs-up
marks successful publication. Queueing can delay the first reaction. A missing
API key or failed analysis produces a generic status reply without internal
response text. Reaction failures do not discard completed output.

Comments and diff contents are review data, not instructions to execute code,
reveal credentials, change repository settings, approve or merge PRs.

## Configuration and data

- Optional provider switch variable: `REVIEWBOT_PROVIDER` (`ark` by default when
  unset, or `deepseek`). Switching between providers is a repository-variable
  change and requires no code changes; unknown values fail closed.
- `ark` provider (default):
  - Secret: `ARK_API_KEY`, an Ark Agent Plan key.
  - Optional variable: `ARK_REVIEW_MODEL`, default `kimi-k3`.
  - Endpoint: `https://ark.cn-beijing.volces.com/api/plan/v3/responses`
    (`max_output_tokens` `8192`).
- `deepseek` provider:
  - Secret: `DEEPSEEK_API_KEY`.
  - Optional variable: `DEEPSEEK_REVIEW_MODEL`, default `deepseek-flash`
    (supported API models: `deepseek-flash` and `deepseek-v4-pro`, each with a
    1M-token context length and 384K-token maximum output).
  - Endpoint: `https://api.deepseek.com/chat/completions` (OpenAI-compatible,
    streaming, `max_tokens` `65536`). Trusted prompts and base policy are
    sent in the `system` message, and pull-request metadata, diff and any
    untrusted command context are sent in the `user` message. Review text is
    read from `choices[0].message.content`, ignoring any `reasoning_content`.
- Optional runner variable: `REVIEWBOT_RUNS_ON`, a JSON label array parsed with
  `fromJSON(vars.REVIEWBOT_RUNS_ON || '["self-hosted","speedle-runner"]')`. It
  applies only to the two model-calling jobs (`Analyze pull request` in
  `reviewbot.yml` for human- and bot-authored same-repository PRs, and `Analyze
  ReviewBot command` in `reviewbot-command.yml`); fork PRs and unknown author
  types remain on `ubuntu-24.04`, and every other job keeps its existing
  `runs-on` configuration. Any runner selected through `REVIEWBOT_RUNS_ON` must
  provide a GitHub Actions runner supporting Node 24-based actions (`2.327.1` or
  newer), `bash`, `git`, `curl`, `jq`, `python3` (standard library only),
  standard POSIX/GNU coreutils, and outbound HTTPS to GitHub and the active
  provider endpoint.
- Authentication for GitHub reads/reviews: the job-scoped `GITHUB_TOKEN`; no PAT
  or GitHub App installation is needed.

This matches Pavu's single configured model per run. The old automatic secondary-model
fallback is replaced by an explicit, bounded retry policy: an incomplete response
caused by the output-token limit (`max_output_tokens` on Ark or
`finish_reason: "length"` on DeepSeek) retries once with unified diff context reduced
from 80 to 3. Both requests share a 600-second HTTP budget. Other incomplete,
failed, malformed or empty responses fail without publishing partial review text.

Both providers are called with `stream: true` and `Accept: text/event-stream`, so
a long request keeps data flowing instead of sitting idle until the whole review
is ready. The trusted response helper reassembles the event stream into the same
response shape a non-streaming call returns and then applies the same checks; a
plain JSON body is still accepted. On Ark the first `response.completed`,
`response.incomplete` or `response.failed` event is the response. On DeepSeek the
helper joins `delta.content`, drops reasoning, and treats the stream as complete
only after a chunk carries a non-null `finish_reason` (`[DONE]` alone is not
enough). A stream that ends without its terminal event fails with reason code
`incomplete_stream` and publishes nothing.

A transport error (non-zero `curl` exit, for example a connection reset during
the stream) resends the same request to the same model after 5 seconds, then 15
seconds, for at most three sends, and only while at least 30 seconds of the HTTP
budget would remain. When the sends are used up the run fails with
`retry_exhausted`; when too little budget remains it fails with
`request_timeout`. HTTP errors and unusable output are never resent as transport
errors, and transport resends do not use up the output-token retry.
Input is limited to 750,000 bytes plus the truncation notice; output is limited
to 8192 tokens on Ark, 65,536 tokens on DeepSeek (within its 1M context and 384K
output limits), and the GitHub message-length limit. Reduced or truncated context
is recorded as a validation gap, and the published footer records the active
model ID.

With the selected provider's secret configured, the trusted prompts, repository policy if present, commit
metadata and static diff are sent to that provider (`ark` or `deepseek`). Commands also include their text,
comment URL/code context, and bounded bot history for recheck. There is no
connection to Speedle runtime requests or production policy stores. Provider-side
retention follows the account's data controls and terms. Do not put credentials
in source, fixtures or comments. Without the active provider's key, automatic analysis emits a
notice, sends no code and publishes no Review. Remove the secret or disable the
two workflows to stop model calls.

Only the response parser from the immutable trusted workflow revision executes in
the analysis job. Inline comments are routed through a no-checkout workflow with
no secret bindings; a default-branch `workflow_run` handler verifies the run,
comment, PR association, actor and time before applying the usual permission
checks. It retrieves comment data through GitHub's API and does not execute code
or unpack artifacts from the routing run. This avoids treating a PR review-comment
event's merge revision as trusted executable code. See
[GitHub's event reference](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows).
PR `.gitattributes` files cannot hide text changes from the static
diff. Checkout does not retain credentials, response diagnostics omit raw model
or error text, temporary review files are removed, and writable GitHub tokens
are confined to the jobs that publish output or perform the existing merge task.

## Existing Dependabot integration

Speedle's separate dependency-only merge job is retained. Commands never trigger
it. It requires a same-repository Dependabot PR, a complete first-attempt review
with the no-actionable-findings marker, and successful publication. Truncated
input or a reduced-context retry cannot authorize a merge. The publisher writes
its own eligible/ineligible marker before model text, bound to both head and base.
Reruns can reuse only that exact trusted eligibility record; a quoted model marker
or an old-base review cannot authorize merging. A duplicate review does not need
to publish again before retrying the merge gate.

Before merging, it pins the PR head and base and checks the canonical `ci.yml`
`pull_request` run for that exact PR/head/base. Required latest-attempt jobs are
`lint / execute`, `test (file) / execute`, `test (etcd) / execute` and
`build / execute`, all successful. It reads CI jobs
on the head commit, correcting the older merge-ref lookup that timed out despite
green CI. Missing, pending, failed or stale checks do not authorize a merge.
GitHub receives the exact reviewed head SHA for the squash operation.

These gates use CI's reported results. Go CI propagates test failures instead of
ignoring their exit status. Store tests run one package at a time, with a guard
against using another service's embedded-etcd ports on the shared machine.

## Maintenance and verification

Files are `.github/workflows/reviewbot.yml`,
`.github/workflows/reviewbot-command.yml`, `.github/workflows/reviewbot-inline.yml`, `.github/codex/reviewbot_response.py`
and the two prompts under `.github/codex/prompts/`. Actions are pinned to complete
commit SHAs. Review these files as privileged CI configuration.

Run from the repository root with Python 3, Git, Bash, jq and Node 22 or newer:

```sh
python3 scripts/tests/test_reviewbot_diff.py -v
python3 scripts/tests/test_reviewbot_response.py -v
node --test scripts/tests/test_reviewbot_commands.cjs
node --test scripts/tests/test_reviewbot_automerge.cjs
node --test scripts/tests/test_runner_routing.cjs
actionlint .github/workflows/reviewbot*.yml
zizmor .github/workflows/reviewbot*.yml
git diff --check
```

The Python suites run the actual workflow shell against synthetic Git history and
stubbed HTTP responses, including a malicious PR-head parser that must never
execute. JavaScript suites execute the actual workflow handlers with GitHub API
mocks, testing permissions, replies, history, deduplication and merge gates. No
credentials, real model requests, GitHub comments or merges are used by fixtures.

The `ReviewBot` workflow also has `workflow_dispatch` to run the regression suites on a
reviewed branch without receiving model API keys or publishing a Review:

```sh
gh workflow run reviewbot.yml --ref <reviewed-branch>
```

New workflows become active after they reach the default branch. A real PR event
and authorized comment are separate integration checks after installation; local
fixtures or a successful dispatch are not claimed as proof of a live model Review.

## Reused source

The automatic and command workflows, command prompt, response helper, Python
diff/response suites and their JSON fixtures were adapted from the pinned Pavu
revision above, under Apache 2.0. Modifications include Speedle configuration,
the trusted inline bridge, review eligibility caching and additional guards/tests.
The upstream [license](../.github/codex/PAVU-LICENSE.txt) and
[NOTICE](../.github/codex/PAVU-NOTICE.txt) are retained. This attribution applies
to the reused ReviewBot source; the rest of Speedle retains its existing license.
