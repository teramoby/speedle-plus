# authz-bench

> English translation of [HARNESS_README.md](HARNESS_README.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

Casbin, OPA, and Speedle+ are compared with the same policy scenarios for functionality, performance, stress behavior, and usability. The full charming retest completed on 2026-10-29; conclusions are in [REPORT.en.md](REPORT.en.md), and the complete handoff plan is in [TEST_PLAN.en.md](TEST_PLAN.en.md).

## Versions

| Engine | Version | Usage |
|---|---|---|
| Casbin | `github.com/casbin/casbin/v3` v3.11.0 | In-process library |
| OPA | `github.com/open-policy-agent/opa` v1.21.4 | In-process library (`v1/rego`) and `opa run --server` |
| Speedle+ | `github.com/teramoby/speedle-plus` v0.0.0-20261028230603-b27604c077ce | In-process library (`pkg/eval`) and `speedle-ads` |

## Directory layout

- `scenarios/sN_*/`: 9 scenarios, each with policies in three formats: `casbin.conf` + `casbin.csv`, `opa.rego` + `opa.json`, and `speedle.spdl`
- `engines.go`: shared interface `Engine.Allowed(Req)` for the three engines
- `scenarios_test.go`: scenario cases/pass matrix (`TestScenarios`) and policy size (`TestPolicySize`)
- `capabilities_test.go`, `broken_test.go`: permission listing, decision explanations, runtime updates, error messages, and malformed-policy behavior
- `gen.go`: generates large policy sets in all three formats (plain: n roles, 10 users per role; manyusers: 10 roles, 100 permissions, n users)
- `bench_test.go`: in-process latency/concurrency benchmarks, load time, and memory (`TestLoadCost`)
- `cmd/authzd`: identical thin HTTP service wrapping any engine
- `cmd/loadgen`: closed-loop load generator reporting throughput and p50/p90/p99/p99.9
- `cmd/watchcheck`: tests Speedle+ file watching when a temporary file is written and renamed over the original
- `stress.sh`: HTTP load test (three thin services, `opa run --server`, and `speedle-ads`)
- `results/`: raw results

## Running the suite

Formal measurements ran only on charming. shrek was used for building, smoke validation, and aggregation.

```bash
# shrek: build the static Linux package with pinned Go 1.27.2; no remote tests start
./build-linux.sh
# charming: extract the package, wait for an idle machine, then run the full matrix
./run-all.sh
# shrek: after retrieving this run's results
python3 summarize.py results/retest-20261029/charming
python3 build_report.py
```

A complete run waits for CI to be idle and retries CI-contaminated steps at most twice. `build_report.py` requires COMPLETE, 107 in-process groups with 5 repetitions each, and 30 HTTP configurations with 3 rounds each; it will not replace the previous report with incomplete results. Parameters and detailed steps are in [TEST_PLAN.en.md](TEST_PLAN.en.md).

Optional plots: install `plot-requirements.txt` in an isolated Python environment, run `python plot_results.py results/retest-20261029/charming`, then rebuild the report. Add `--mixed-only` to view completed in-process results before the full matrix finishes.

## Tools for the 2026-10-29 retest

- `manyusers` benchmarks: 1000 / 100000 users; the HTTP datasets are plain with 10000 roles and manyusers with 100000 users.
- `stress.sh`: three interleaved rounds by default, concurrency 1/16/64, 3-second warmup, 15-second measurement. Linux server CPUs 0,2; load-generator CPUs 1,3 (actual allocation is recorded in stress.err). CPU columns use each process's own CPU-time difference.
- `run-all.sh`: samples CI, load, the top three processes, and cumulative paging every 5 seconds; requires an idle 10-second cooldown before each step; retries CI-contaminated steps at most twice and retains original records under contaminated/.
- `summarize.py results/<run>/charming`: aggregates only the specified directory, without mixing older results.
- Binary module metadata is read on shrek using `go version -m` and packaged as build-info.txt (charming has no Go installation).
- Only processes started by this run are cleaned up, using their recorded PIDs. Default service endpoints are 127.0.0.1:19000, 18181, and 16735.

## Results of this run

Formal window: 2026-10-29 09:19:29–10:17:58 (Asia/Shanghai). Each engine passed 30/30 functional cases; 107 in-process groups ran 5 repetitions each; 30 HTTP configurations ran 3 rounds each, with zero errors and zero wrong decisions. No CI Worker appeared in 734 activity samples, and no retry was triggered. Paging activity is disclosed in the report.

- [Complete summary](results/retest-20261029/charming/summary.en.md)
- [Execution log](results/retest-20261029/charming/run-all.log)
- [Rebuilt binary consistency](results/retest-20261029/reproduction-binary-check.json)
- [Post-run remote verification](results/retest-20261029/final-remote-verification.json)

Two issues in the pinned Speedle+ version were confirmed on Linux: atomically replacing a watched file terminated the host process, and an incomplete condition was not reported until request evaluation. The project's v1.6.0 and README status were checked separately at the time; outdated handoff descriptions were not treated as current conclusions. The version under test was not upgraded.
