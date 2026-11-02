# LLM-assisted authorization-engine benchmark archive: 2026-10-29

> English translation of [README.md](README.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

This directory archives the fixed-version Casbin, OPA, and Speedle+ comparison run on charming after Codex took over a Claude Code handoff. The client was the Speedle+ maintainer. The directory name `private` does not provide access control: this is a public repository. Personal directory identifiers and authentication metadata were redacted from the public copy; see [PRIVACY.en.md](PRIVACY.en.md).

Formal measurement window: 2026-10-29 09:19:29–10:17:58, Asia/Shanghai.

| Entry point | Contents |
|---|---|
| [REPORT.en.md](REPORT.en.md) | Full report: environment, method, performance, capabilities, ecosystem, defects, and limitations |
| [PROMPTS.en.md](PROMPTS.en.md) | User task instructions, authorizations, follow-up questions, and original Claude handoff input |
| [TEST_PLAN.en.md](TEST_PLAN.en.md) | Executed matrix and parameters; the original handoff version is also retained |
| [ENVIRONMENT.en.md](ENVIRONMENT.en.md) | Roles of both machines, compiler, pinned dependencies, CPU affinity, load, and paging |
| [DEPLOYMENT.en.md](DEPLOYMENT.en.md) | Static Linux package, SSH upload, startup, service parameters, and completion status |
| [EVIDENCE.en.md](EVIDENCE.en.md) | Raw evidence index, limits of proof, unsaved information, and exclusions |
| [CORRECTIONS.en.md](CORRECTIONS.en.md) | Outdated handoff statements and corrections to relationship-authorization conclusions |
| [RECOMMENDATION.en.md](RECOMMENDATION.en.md) | Workload-specific performance/capability analysis; untested approaches are not labeled unsupported |
| [HARNESS_README.en.md](HARNESS_README.en.md) | Original working-directory instructions and build entry points |
| [Complete result summary](results/retest-20261029/charming/summary.en.md) | All five-run ranges, allocations, three HTTP rounds, and capability output |
| [MANIFEST.json](MANIFEST.json) | Per-file SHA-256 values for this public copy; Git commits cover the manifest itself |

Each engine passed 30/30 functional cases. There were 107 in-process benchmark groups with 5 repetitions each, and 30 HTTP configurations with 3 rounds each: 90 windows with zero HTTP errors and zero wrong decisions. No Runner.Worker appeared in 734 activity samples; all 13 steps completed on the first attempt, with no CI retry triggered. Swap-in increased by 1115 pages and swap-out by 35 pages. These are disclosed, so the run is not described as entirely free of background interference.

Pinned baselines: Casbin v3.11.0, OPA v1.21.4, Speedle+ v0.0.0-20261028230603-b27604c077ce. All formal figures come from this charming run, without mixing the original shrek draft or earlier charming data. Relationship examples cover only the selected implementations. Equivalent models using attributes, role hierarchies, and custom functions were not compared, so these examples cannot rank general product capabilities.

Source, policies, and scripts retain their original standalone Go-module layout. This nested module pins an external Speedle+ baseline and does not change the repository's parent module or production engine. The parent module's `go list ./...` does not automatically include this nested module.

```bash
cd benchmarks/private/benchmark-by-llm
python3 verify_archive.py                 # Verify the archive and measurement matrix; starts no services
python3 summarize.py results/retest-20261029/charming
python3 build_report.py                   # Rebuild the report from the same data
# Optional: install plot-requirements.txt in an isolated Python environment for plots
python3 plot_results.py results/retest-20261029/charming
# To rerun, first arrange an exclusive window as documented; this command only builds
./build-linux.sh
```

Executables and the 97 MB complete Linux package are not committed. Original package hashes, hashes for six binaries, proof of byte-identical rebuilding from the same source, and small source snapshots are under [provenance/](provenance/). No keys, credentials, local configuration, or full user directories are included. Older macOS reports inside historical handoff archives serve only to audit the process; they are not data sources for the final report.

Archive publication PR: [\#134](https://github.com/teramoby/speedle-plus/pull/134). Documentation/ecosystem data were captured at approximately 09:20–09:25 that morning. [\#133](https://github.com/teramoby/speedle-plus/pull/133), merged during archival, updated the embedded-deployment documentation. Relevant older descriptions are historical observations; see [CORRECTIONS.en.md](CORRECTIONS.en.md) for updates.

## Complete English document index

The 15 translated documents are adjacent `.en.md` files. Four records already have English bodies and are linked directly. All 19 original Markdown documents remain unchanged. Benchmark tables, versions, request outcomes, logs, source archives, and figures are preserved. Chinese comments and explanatory annotations are translated; executable commands and raw result lines are retained. The historical generators still write their original-language reports; these English files translate the archived snapshot.

| Original | English version |
| --- | --- |
| [CORRECTIONS.md](CORRECTIONS.md) | [CORRECTIONS.en.md](CORRECTIONS.en.md) |
| [DEPLOYMENT.md](DEPLOYMENT.md) | [DEPLOYMENT.en.md](DEPLOYMENT.en.md) |
| [ENVIRONMENT.md](ENVIRONMENT.md) | [ENVIRONMENT.en.md](ENVIRONMENT.en.md) |
| [EVIDENCE.md](EVIDENCE.md) | [EVIDENCE.en.md](EVIDENCE.en.md) |
| [HARNESS_README.md](HARNESS_README.md) | [HARNESS_README.en.md](HARNESS_README.en.md) |
| [PRIVACY.md](PRIVACY.md) | [PRIVACY.en.md](PRIVACY.en.md) |
| [PROMPTS.md](PROMPTS.md) | [PROMPTS.en.md](PROMPTS.en.md) |
| [README.md](README.md) | [README.en.md](README.en.md) |
| [RECOMMENDATION.md](RECOMMENDATION.md) | [RECOMMENDATION.en.md](RECOMMENDATION.en.md) |
| [REPORT.md](REPORT.md) | [REPORT.en.md](REPORT.en.md) |
| [TEST_PLAN.md](TEST_PLAN.md) | [TEST_PLAN.en.md](TEST_PLAN.en.md) |
| [evidence/preparation/preparation-smoke-20261029.md](evidence/preparation/preparation-smoke-20261029.md) | [Already English](evidence/preparation/preparation-smoke-20261029.md) |
| [prompts/claude-handoff-original.md](prompts/claude-handoff-original.md) | [prompts/claude-handoff-original.en.md](prompts/claude-handoff-original.en.md) |
| [results/retest-20261029/EXECUTION.md](results/retest-20261029/EXECUTION.md) | [results/retest-20261029/EXECUTION.en.md](results/retest-20261029/EXECUTION.en.md) |
| [results/retest-20261029/charming/stress.md](results/retest-20261029/charming/stress.md) | [Already English](results/retest-20261029/charming/stress.md) |
| [results/retest-20261029/charming/summary.md](results/retest-20261029/charming/summary.md) | [results/retest-20261029/charming/summary.en.md](results/retest-20261029/charming/summary.en.md) |
| [results/retest-20261029/deployment/results-linux-smoke.md](results/retest-20261029/deployment/results-linux-smoke.md) | [Already English](results/retest-20261029/deployment/results-linux-smoke.md) |
| [results/retest-20261029/research/speedle-current-readme.md](results/retest-20261029/research/speedle-current-readme.md) | [Already English](results/retest-20261029/research/speedle-current-readme.md) |
| [validation/README.md](validation/README.md) | [validation/README.en.md](validation/README.en.md) |
