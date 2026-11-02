# User task instructions and handoff input

> English translation of [PROMPTS.md](PROMPTS.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.
> Historical input, not instructions to execute now. User quotations are translated; original Chinese wording is retained in the linked source. Superseded claims are documented in [Corrections](CORRECTIONS.en.md).

This page preserves excerpts of the user instructions visible in the chat and links the public original handoff files. Personal account identifiers and key filenames were redacted under PRIVACY.md, with original byte hashes retained. The earlier Claude Code conversation was not provided, so its complete prompt cannot be reconstructed. The handoff assistant was identified as Claude Code and the executing assistant as Codex. Exact underlying model snapshots, sampling settings, and complete CLI invocation history were not recorded and are not inferred. Reproducibility covers the archived test programs, pinned versions, execution method, and data processing.

## Initial handoff

The user introduced the handoff as coming from their friend Claude Code and provided a summary with the full test plan. Complete inputs are preserved in:

- [User-pasted original handoff](prompts/user-pasted-handoff.txt)
- [Pre-execution TEST_PLAN.md](prompts/claude-handoff-original.en.md)
- [Plan updated after execution](TEST_PLAN.en.md)

Main instructions: retest all three engines from scratch on charming; wait for explicit permission to start; pin versions; complete five tooling items; compare neutrally; do not use password login or stop other people's processes; stop immediately on request; do not push or publish during the test phase.

Descriptions such as Speedle+ can only encode relationships in paths, the project has no official Go release, and the ranking should remain the same are retained as historical input and were subsequently corrected; see [CORRECTIONS.en.md](CORRECTIONS.en.md). They are not final capability conclusions or criteria for selecting results.

## User authorization to start (translated excerpt)

> The other tasks on charming have all finished. You can use charming to run the tests now.

## User request for interim results (translated excerpt)

> Show me the results so far.

At that point all 107 in-process groups were complete, but the three HTTP rounds were not. HTTP figures in the progress response were explicitly marked as first-round observations. The final report uses complete three-round medians; the archived summary.md is complete.

## User authorization to archive on GitHub (translated excerpt; original link normalized as plain text)

> Put the corresponding prompts, test environment, test plan, deployment details, test report, and related material/evidence at https://github.com/teramoby/speedle-plus/benchmarks/ private/benchmark-by-llm

This instruction authorized committing the directory at `benchmarks/private/benchmark-by-llm` after measurement. The earlier prohibition on pushing during testing was not treated as a permanent ban on publication. The repository is public, and the private directory name was explicitly explained not to provide access control.

## Subsequent capability discussion (translated excerpts, in order)

> Considering only product performance and capabilities, which would you recommend?

> What do you mean by complex graph traversal? Is that a problem access management should solve?

> Why can't this be implemented with attributes and role hierarchies?

> It could also be implemented with a custom function.

> What is the problem?

These follow-up questions prompted corrections to the relationship-authorization assessment; no new test results were added or fabricated. The final interpretation is in [RECOMMENDATION.en.md](RECOMMENDATION.en.md) and [CORRECTIONS.en.md](CORRECTIONS.en.md).

## User question about archival progress (translated excerpt)

> What are you doing now? Has it been merged?

At the time of this question, local archive checks were complete but nothing had been pushed. Archive PR #134 was subsequently created and awaited required CI. Its final merge status is determined by that PR; this historical document does not claim a merge in advance.
