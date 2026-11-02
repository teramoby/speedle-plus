# Execution status for this run

> English translation of [EXECUTION.md](EXECUTION.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

- On 2026-10-29 the user explicitly authorized the full charming run.
- Local working directory: /Users/LOCAL_USER/cc/authz-bench (not a Git repository).
- Earlier source/report backup: /Users/LOCAL_USER/cc/authz-bench-backups/20261029-0914.
- Versions unchanged: Casbin v3.11.0, OPA v1.21.4, Speedle+ v0.0.0-20261028230603-b27604c077ce.
- Tools now include manyusers, CPU accounting, taskset, three interleaved rounds, CI sampling/retries, and summarize.py; generator functional tests and decision checks in timed loops were added.
- Local go vet, five generator variants, 100x ManyUsers smoke, and two shapes × five services smoke all passed. All 10 Linux smoke configurations had zero errors/wrong decisions and valid CPU accounting.
- Linux package: dist/authz-bench-linux-20261029.tar.gz; build-info.txt contains all module versions, source.tar.gz the Go source snapshot.
- Separate charming directory: /tmp/authz-bench-codex-20261029; old directories were not deleted and other people's processes were not stopped.
- Formal run-all.sh PID: 394288 (started near 09:19:29; machine step at 09:19:39 +08:00); run.pid is in the remote root directory.
- Result destination: results/retest-20261029/charming. COMPLETE marks the full matrix; do not mix older results/charming or shrek figures.
- At 09:37:47, machine / functional / capabilities / watchcheck / loadcost / bench-small were complete. Next: bench-manyusers, four separate 100k engines, parallel 1/2/4, and 90 stress windows.
- Linux watchcheck confirmed inplace 11ms/exit0 and rename fatal/exit1; malformed condition loaded with nil error, then returned false/ERROR_IN_EVALUATION. Runtime updates: 38.228µs/80.994µs/5ms.
- Full-result waiter: local exec session 36950 running /tmp/authz-bench-wait-20261029.py. It read-only checks every 2 minutes, then scp retrieves results and summarize/build_report generate the report after COMPLETE. write_stdin can inspect new output.
- Report build: python3 build_report.py requires COMPLETE, 107 groups × 5, and 30 HTTP groups × 3; it rejects incomplete results. At this checkpoint REPORT.md was still the old draft, noted in README.
- Optional figures: /tmp/authz-bench-report-venv-20261029/bin/python plot_results.py results/retest-20261029/charming; then python3 build_report.py to embed them. matplotlib is in an isolated temporary venv. Inspect both PNGs with view_image.
- Ecosystem queries saved under research/: GitHub repos/releases/search, contributors anon=1 (173/625/8), SO tags (33/270/176/0). Speedle+ v1.6.0 was already in the Go proxy at inspection, commit17da10eb. README was updated; do not repeat the outdated handoff's no-release/old-current-README claims.
- SSH always uses BatchMode=yes; no private keys read, no pushing/publication. On stop, terminate only this run's PID 394288 and its own descendants; TERM/EXIT traps clean owned children.
- Before completion: integrity checks, figure QA, figure/link proofreading, README/TEST_PLAN status updates, and verification that owned processes exited. Do not claim completion before formal results finish.

## 09:55 update

All 107 in-process groups were verified to have 5 repetitions and end in PASS. HTTP started at 09:50:32; by 09:54:56, 14/90 windows (round 1, plain) had completed with zero errors/wrong decisions. The user requested progress; commentary showed functional 30/30, mixed latency for three shapes, 100k loading cost, and first-round HTTP, explicitly labeled nonfinal. Work continued.

summarize.py now labels directories lacking COMPLETE as in progress rather than presenting one HTTP round as the final three-round median. plot_results.py adds --mixed-only to show completed in-process results; the rest still requires the full matrix. Generate both final figures, then rebuild REPORT.

## Final completion

At 2026-10-29 10:17:58 +08:00, COMPLETE appeared. All 107 groups × 5 and 30 HTTP groups × 3 were complete: 90 windows with zero errors/wrong decisions, 13 first-attempt steps, 734 samples without CI Workers, no retry triggered. Swap-in increased 1115 pages and swap-out 35, as disclosed. The formal REPORT replaced the old draft; PNG/SVG figures were generated and viewed. All 6 actual remote program hashes matched the initial package, machine records, and local rebuild. All owned test processes/listening ports exited; other people's processes were not stopped, and nothing was pushed/published during the run. README/TEST_PLAN were marked complete.
