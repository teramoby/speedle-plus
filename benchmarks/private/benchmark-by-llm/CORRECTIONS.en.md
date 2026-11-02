# Corrections to conclusions and handoff descriptions

> English translation of [CORRECTIONS.md](CORRECTIONS.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

These corrections were made during review and discussion after measurement. They do not change the raw performance data or add tests that were not executed.

1. **Relationship authorization.** The original handoff generalized limitations of Speedle+'s path-encoded example into product limitations, and the draft consequently made overly broad comparisons of general graph capabilities. The user pointed out that attributes, role hierarchies, and custom functions can implement relationship authorization; the official [SPDL](https://speedle.io/docs/spdl/) and [custom-function](https://speedle.io/docs/custom-function/) documentation supports these mechanisms. The report now marks equivalent relationship models as untested. It no longer uses the path example to conclude that Speedle+ cannot meet these requirements or to recommend OPA over Speedle+ on that basis.
2. **Role inheritance was verified.** All three engines passed all 4/4 cases in scenario s2. Ordinary role inheritance must not be listed as a missing Speedle+ feature.
3. **Release status.** The 2026-10-29 checks found v1.6.0 on GitHub Releases and the Go module proxy. The earlier statement that there was no recognized official Go release was outdated. This run still pinned the b276 pseudo-version and made no inference about newer-version behavior.
4. **Documentation status.** At the time of inspection, the current README already used go install/go build. The pinned old README and pages still describing dep or old Istio Mixer steps were assessed separately.
5. **Historical numbers and ordering.** Earlier shrek measurements were historical reference only and were not mixed into the charming report. No ranking was assumed in advance. All five repetitions/three rounds were retained under the original rules; results were not selected to match earlier conclusions.
6. **Memory units.** The original loading table labeled values MB but calculated them using 2^20 bytes. The report interprets them as MiB: retained Go heap increments, not process RSS or peak memory.
7. **Interim HTTP results.** The progress response showed first-round data. The completed report and summary use medians of all three rounds; first-round observations were not presented as final results.

Evaluating the maintenance cost, end-to-end latency, and consistency of richer relationship authorization requires a separate comparison of semantically equivalent implementations. This archive contains no such new experiment.

8. **Documentation repair during archival.** [PR #133](https://github.com/teramoby/speedle-plus/pull/133) merged on 2026-10-29 at 10:59 (Asia/Shanghai), updating embedded deployment instructions to Go modules and current Go. This was later than the approximately 09:24 page capture and initial report draft. The report retains the historical observation with a time note rather than describing it as unresolved at publication; the pinned b276 behavior and performance data are unchanged.
