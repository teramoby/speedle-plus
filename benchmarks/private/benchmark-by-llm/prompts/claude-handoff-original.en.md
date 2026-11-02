# Test plan: Casbin, OPA, and Speedle+ authorization-engine comparison (handoff document)

> English translation of [claude-handoff-original.md](claude-handoff-original.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.
> Historical input, not instructions to execute now. User quotations are translated; original Chinese wording is retained in the linked source. Superseded claims are documented in [Corrections](../CORRECTIONS.en.md).

This document was written for an AI assistant taking over execution. It contains the paths, commands, and versions needed to work independently without earlier chat history. The user means the person commissioning this benchmark.

## 0. Read this first

- **Task:** Run a complete, fair, reproducible comparison of three open-source authorization engines, Casbin, OPA, and Speedle+, on the Linux machine `charming`: functionality, performance (in-process latency, multicore throughput, loading cost), HTTP load, usability, and learning curve. Produce a Chinese report.
- **Retest all three engines from scratch.** Previous runs on the macOS laptop shrek and on charming were incomplete or affected by paging/CI tasks. Retain them only as reference; do not include their numbers in the report.
- **Document the complete test environment**, as specified in section 7.
- **Do not start charming tests before the user's instruction.** The user will stop other work on charming, including GitHub Actions CI, then explicitly authorize starting.
- **Conflict of interest:** The user maintains Speedle+. Conclusions must be neutral, and Speedle+ issues must be reported honestly and specifically.
- **Boundaries:** Do not use password login; do not touch processes you did not start (such as another session's `speedle-ads` on charming); do not push code or publish anything; stop immediately if asked.

## 1. Machines and access

| | shrek (preparation and aggregation) | charming (the only test machine) |
|---|---|---|
| Purpose | Store test source, cross-compile, aggregate results, write report | Run all tests |
| Hardware | Apple M1 Pro, 8 cores, 16 GB | Intel Core i5-7360U @ 2.30GHz; 2 cores/4 threads; 15 GiB RAM, 2 GiB swap, NVMe |
| System | macOS 27.0.1, Go 1.27.2 (`/opt/homebrew/bin/go`) | Linux Mint 22.3 (Ubuntu 24.04 base), x86_64; Go not installed |
| Access | Local | From shrek, run `ssh charming.local` (user `REMOTE_USER`, authorized key `~/.ssh/KEY_FILE`; use `-o BatchMode=yes` to prevent password prompts) |

GitHub Actions runner directories on charming, all under `/home/REMOTE_USER/`: `actions-runner`, `actions-runner-2`, `actions-runner-3`, `actions-runner-speedle`, `actions-runner-speedle-2`. A `Runner.Listener` without a Worker means idle; `Runner.Worker` indicates an active CI job (`pgrep -c Runner.Worker` counts them).

## 2. Pinned versions under test (do not upgrade)

| Engine | Go module | Version |
|---|---|---|
| Casbin | `github.com/casbin/casbin/v3` | v3.11.0 (in the Apache Incubator; repository `apache/casbin`) |
| OPA | `github.com/open-policy-agent/opa` | v1.21.4 (in-process `github.com/open-policy-agent/opa/v1/rego`) |
| Speedle+ | `github.com/teramoby/speedle-plus` | `v0.0.0-20261028230603-b27604c077ce` (pseudo-version for commit `b27604c077ce`; the historical handoff claimed no official Go-recognized release) |

All programs are built on shrek with Go 1.27.2. charming runs cross-compiled static Linux programs (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`).

## 3. Existing test source

Source is at `/Users/LOCAL_USER/cc/authz-bench/` on shrek (Go module `authzbench`).

| File | Purpose |
|---|---|
| `scenarios/s1_rbac` … `scenarios/s9_relationship` | 9 functional scenarios, each with three policy representations: `casbin.conf`+`casbin.csv`, `opa.rego`+`opa.json`, `speedle.spdl` |
| `engines.go` | Shared `Engine` interface: `Allowed(Req)`, `Prepare(Req)` (convert to native requests first, timing only decisions) |
| `scenarios_test.go` | `TestScenarios` (9×3 pass matrix), `TestPolicySize` (policy line/character counts for each scenario) |
| `capabilities_test.go` | `TestListPermissions`、`TestExplain`、`TestRuntimeUpdate`、`TestErrorMessages` |
| `broken_test.go` | `TestBrokenPolicyBehavior`: when malformed policies produce errors and how later requests are decided |
| `gen.go` | Generate policy sets in three formats: n roles, `roleI` can read `/books/bookI`, 10 users `userI-1..10` per role. Kinds: `plain` (exact match), `abac` (per-rule `att1=="val1" && att2=="val2"` conditions), `wildcard` (`/books/typeI/` prefix). `AUTHZ_ENGINES=a,b` loads only selected engines |
| `bench_test.go` | `BenchmarkRBACAllow`, `BenchmarkRBACDeny`, `BenchmarkRBACMixed` (4096 random requests repeated, 80% allow/20% deny), `BenchmarkABAC`, `BenchmarkWildcard`, `BenchmarkParallel`, `TestLoadCost` (requires `LOADCOST=1`) |
| `cmd/authzd` | Identical thin HTTP wrapper for each engine: `POST /check {"user","action","resource"}` → `{"allowed":bool}`; `-gen DIR` generates policy files only |
| `cmd/loadgen` | Closed-loop generator: `-target thin\|opa\|speedle`, `-c` concurrency, `-d` duration, `-warmup` warmup; checks each response against its expected decision; emits a Markdown row with throughput, p50/p90/p99/p99.9/max (microseconds), errors, and wrong decisions |
| `cmd/watchcheck` | Speedle+ embedded file-watch checks: `inplace` (write in place), `rename` (write a temporary file and rename over the original) |
| `stress.sh` | HTTP load tests for 3 thin wrappers plus official `opa run --server` and `speedle-ads` services (the Casbin core library has no HTTP server) |
| `run-all.sh` | Run steps in order, writing results under `results/<hostname>/` |
| `README.md`, `REPORT.md` | Instructions and draft report (draft shrek numbers must be replaced entirely by charming results) |

Benchmark engine variant names:

- `casbin`: default matcher `g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act`
- `casbin-tuned`: role lookup last, `r.obj == p.obj && r.act == p.act && g(r.sub, p.sub)`; included as a fairness optimization
- `casbin-cached`: `CachedEnforcer`; represents cache hits when the same request repeats and must be labeled accordingly
- `casbin-matcher` (ABAC only): conditions are in the matcher instead of per-rule `eval()`
- `opa`: data-driven, key-indexed Rego evaluated after `PrepareForEval`
- `speedle`: embedded evaluator constructed with `eval.NewFromFile`

## 4. Preparation required before execution

The following five items were planned but not yet implemented at handoff. Complete them on shrek, validate with small parameters, then cross-compile/package.

1. **Add `manyusers`: few roles, many users, a more favorable shape for Casbin.**
   - 10 roles with 10 permissions each: `roleR` can read `/books/bookR-K` (R,K=1..10; 100 rules). `userU` (U=1..n) has role `role((U-1)%10+1)`. Speedle+ has one role-grant line per user: `GRANT USER userU roleR`.
   - Random requests: select U, derive R as above; 80% request `/books/bookR-K` with random K (allow), 20% request `/books/bookR2-K` where `R2 = R%10+1` (deny).
   - Add the kind in `gen.go` (including `casbin-tuned` and `casbin-cached`); add `BenchmarkManyUsers` for n=1000 and 100000 in `bench_test.go`; add `-kind plain|manyusers` to `cmd/authzd` and `cmd/loadgen`.
2. **Record CPU usage in load tests.** Add `-srv-pid` to `loadgen`; read utime+stime from `/proc/<pid>/stat` at the start/end, divide by clock frequency (usually 100), and compute average server cores used during measurement. Read `/proc/self/stat` similarly for the generator. Add `srv cores` and `gen cores` columns; use `-` on macOS without /proc.
3. **Improve `stress.sh`.**
   - Use `taskset`: server on the first physical core's two threads (read `/sys/devices/system/cpu/cpu0/topology/thread_siblings_list`), generator on remaining threads; set each `GOMAXPROCS` to its allocated thread count. Without taskset, fall back to GOMAXPROCS only.
   - Two datasets: `plain` n=10000 (many roles), `manyusers` n=100000 (many users).
   - Concurrency 1,16,64; 3-second warmup, 15-second measurement; repeat the suite for 3 interleaved rounds (all configurations in round 1 before round 2), reducing time drift that affects only one engine.
   - Bind all services only to `127.0.0.1`. Add `--insecure=true --log-level error --auditlog-level error --enable-watch false` to speedle-ads (default info-level auditing adds overhead, whereas OPA does not log decisions by default); add `--log-level error` to `opa run --server`.
4. **Protect `run-all.sh` against CI interference.**
   - Every 5 seconds, append time, 1-minute load, `Runner.Worker` count, and the top three CPU-consuming processes to `activity.log`.
   - Before every step, require zero `Runner.Worker` processes plus a 10-second cooldown.
   - After each step, inspect activity samples within its time window. If any CI activity appeared, mark the step contaminated, wait for CI to finish, and retry up to 2 times. Record every retry in `run-all.log`.
   - Add `capabilities` (run `TestListPermissions|TestExplain|TestRuntimeUpdate|TestErrorMessages|TestBrokenPolicyBehavior`) and `watchcheck` (run inplace and rename modes, recording output/exit codes). Cross-compile watchcheck into the package too.
   - The `machine` step also records `uname -r`, `/sys/devices/system/cpu/cpu*/cpufreq/scaling_governor`, `/sys/devices/system/cpu/intel_pstate/no_turbo` if present, full `lscpu`, `free -h`, `df -h /tmp`, and `go version -m` for each program (module-version evidence).
   - If using `pkill -f`, patterns must look like `"[r]un-all.sh"`; otherwise they match the invoking shell and kill their own session.
5. **Write `summarize.py`.** Input `results/charming/`, output `results/charming/summary.md`:
   - Functional matrix and policy sizes (extract Markdown tables from `functional.txt`)
   - Capability-test output excerpts
   - Loading-cost table
   - Median ns/op per benchmark/size/variant, five-run min/max, B/op, and allocs/op
   - Multicore throughput: convert ns/op into decisions/second
   - HTTP: median of 3 rounds per dataset/configuration/concurrency
   - Automatic time units: ns, µs, ms

## 5. Test matrix

### 5.1 Functionality (9 scenarios, 30 cases)

Each case requires identical decisions from all three engines. Scenarios/cases already exist in `scenarios_test.go`:

| Scenario | Model | Cases (expected decisions) |
|---|---|---|
| s1 Basic RBAC | alice=admin (read/write /data1), bob=viewer (read only) | alice write ✔; bob write ✘; bob read ✔; carol read ✘ |
| s2 Role inheritance | editor inherits viewer; dave=editor, vic=viewer | dave read ✔, write ✔; vic read ✔, write ✘ |
| s3 Groups | eng group has dev role (deploy /svc) | erin(eng) ✔; frank without group ✘; frank(ops) ✘ |
| s4 Multi-tenant | gina is admin in tenant1, viewer in tenant2 | t1 write ✔; t2 write ✘; t2 read ✔; t3 read ✘ |
| s5 ABAC approval limit | manager may approve ≤1000 | hank 500 ✔; hank 5000 ✘; ivy 10 ✘ |
| s6 Owner-only edit | owner == requesting user required for edit | ivan edits own resource ✔; judy edits ivan's ✘; ivan delete ✘ |
| s7 Deny overrides | staff may read /hr/*, but kim explicitly denied /hr/salaries | kim read salaries ✘; kim read handbook ✔; lee read salaries ✔ |
| s8 Wildcard resource | analyst may read everything under /reports/ | leo read /reports/2026/q3 ✔; read /secret/x ✘; write ✘ |
| s9 Relationships | Documents in nested folders; mia may read F1 | D1 ✔; D2 ✘; D3 (inside F1's child folder) ✔ |

Document these implementation differences: Speedle+ tenant modeling encodes tenants in paths (`ON expr:/tenant1/.*`); the historical handoff claimed relationships could only encode folder hierarchy in paths (`/F1/F1a/D3`) and could not represent document moves/multiple parents. The ownership case used the undocumented `authenticated_role`. Casbin stores group membership in policy; OPA/Speedle+ receive groups in requests.

### 5.2 Capabilities beyond decisions

- List all permissions for a user (s1, alice)
- Explain a denial (s7, kim reading /hr/salaries): Casbin `EnforceEx`, OPA trace, Speedle+ `IsAllowed` reason and `Diagnose`
- Time until runtime updates take effect (s1, bob writing after viewer gains write): Casbin `AddPolicy`, OPA store write, Speedle+ file change/watch
- Malformed-policy feedback: unmatched matcher parentheses/misspelled fields in Casbin; missing brace/misspelled variable in OPA; missing resource, misspelled `GRANT`, incomplete condition in Speedle+
- Decisions after malformed-policy loading (incomplete Speedle+ condition; misspelled Casbin matcher)
- Embedded Speedle+ file replacement: `watchcheck inplace` and `watchcheck rename`

### 5.3 In-process performance (`-test.benchtime 1s -test.count 5`)

| Benchmark | Size | CPU | Engine variants |
|---|---|---|---|
| RBACAllow, RBACDeny, RBACMixed | n=100,1000,10000 | `-test.cpu 2` | All |
| Same as above | n=100000 | `-test.cpu 2` | Separate process per engine: `AUTHZ_ENGINES=speedle`, `opa`, `casbin-tuned`, `casbin` |
| ABAC, Wildcard | n=1000,10000 | `-test.cpu 2` | All |
| ManyUsers (new) | n=1000,100000 users | `-test.cpu 2` | All |
| Parallel | n=1000,10000 | `-test.cpu 1,2,4` | Casbin uses `SyncedEnforcer` |
| TestLoadCost | n=1000,10000,100000 | | casbin, opa, speedle: loading time and retained heap |

At 100K, Casbin uses roughly 1 GB, and the default matcher with random requests takes over 100 ms per decision in the preliminary context; run each engine in a separate process.

### 5.4 HTTP load testing

Run `stress.sh` with the section 4.3 improvements: 5 configurations (thin wrappers for casbin/opa/speedle, `opa run --server`, `speedle-ads`) × 2 datasets × 3 concurrency levels × 3 rounds.

### 5.5 Usability and learning curve (ordinary developer perspective)

Compare the following and include conclusions in the report:

- Installation, available prebuilt programs, browser trials (Casbin online editor, OPA Playground, Speedle+ Playground)
- Code required for Go embedding, using `engines.go` as the reference
- Concepts to learn: Casbin PERM model DSL; OPA Rego (declarative rules, `some`/`every`, undefined versus false, `data`/`input`); Speedle+ SPDL
- Documentation quality/freshness: historical examples include Speedle+ README instructions for Go 1.13/GOPATH/`go get` while Quick Start had been updated, embedded instructions still using `dep`, and Istio integration based on Mixer removed in Istio 1.8
- Tools: `opa eval/test/fmt/check/bench`, REPL; Casbin `EnforceEx`; Speedle+ `spctl`, `Diagnose`
- Quality of error feedback (from section 5.2)
- Help available for troubleshooting: query GitHub and Stack Overflow APIs at execution time, recording the date

## 6. Execution steps

1. **Wait for the user to confirm charming is available.**
2. Implement section 4 on shrek; run `go vet ./...`; validate with small settings (for example `./stress.sh 1000 3s 8` and benchmarks with `-test.benchtime 100x`).
3. Cross-compile and package, from `/Users/LOCAL_USER/cc/authz-bench`:

   ```bash
   D=dist/authz-bench-linux; rm -rf dist; mkdir -p $D/bin
   export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
   go test -c -o $D/bin/authzbench.test .
   go build -o $D/bin/ ./cmd/authzd ./cmd/loadgen ./cmd/watchcheck
   go build -o $D/bin/opa github.com/open-policy-agent/opa
   go build -o $D/bin/speedle-ads github.com/teramoby/speedle-plus/cmd/speedle-ads
   unset GOOS GOARCH CGO_ENABLED
   cp -R scenarios stress.sh run-all.sh $D/
   COPYFILE_DISABLE=1 tar czf dist/authz-bench-linux.tar.gz -C dist authz-bench-linux
   ```

   If `go build` reports `missing go.sum entry`, first run `go get github.com/open-policy-agent/opa@v1.21.4` or `go get github.com/teramoby/speedle-plus/cmd/speedle-ads@b27604c077ce`, then rebuild. `COPYFILE_DISABLE=1` prevents numerous `LIBARCHIVE.xattr` warnings on Linux extraction (the warnings themselves are harmless).
4. Read-only preflight: `ssh -o BatchMode=yes charming.local 'uptime; pgrep -c Runner.Worker; ps -eo pcpu,pmem,comm --sort=-pcpu | head -8; free -h'`. Confirm no CI, low load, and no other heavy process. Report someone else's CPU-intensive process to the user rather than stopping it yourself.
5. Upload and launch in the background, surviving SSH disconnection:

   ```bash
   scp -q dist/authz-bench-linux.tar.gz charming.local:/tmp/
   ssh -o BatchMode=yes charming.local 'cd /tmp && rm -rf authz-bench-linux && tar xzf authz-bench-linux.tar.gz && cd authz-bench-linux && (nohup ./run-all.sh > run-all.out 2>&1 &)'
   ```

   The previous directory `/tmp/authz-bench-linux` remained on charming; the historical command above deletes it first.
6. Monitor `results/charming/run-all.log` and `activity.log` every few minutes, without continuous polling. Estimated duration was 1 to 1.5 hours because charming has a slower CPU.
7. Retrieve results: `scp -r charming.local:/tmp/authz-bench-linux/results/charming /Users/LOCAL_USER/cc/authz-bench/results/`, then run `summarize.py`.
8. Write the report (section 7).

## 7. Report requirements

Update `/Users/LOCAL_USER/cc/authz-bench/REPORT.md` in Chinese with a neutral tone. The draft's structure may be reused, but every number must be replaced by charming results.

1. **Conclusions:** Lead with two or three evidence-backed sentences for each dimension: functionality, performance, resources, usability, ecosystem.
2. **Environment (required):**
   - Hardware: CPU model, core/thread counts, memory, disk, frequency governor, turbo status
   - System: distribution, kernel
   - Build: Go version, cross-compilation settings
   - Tested versions: module versions for all three engines, from `go version -m`
   - Run conditions: CPU allocation (`-test.cpu`, load-test affinity), load/CI activity (cite `activity.log` and disclose CI retries), test date
3. **Method:** Policy shapes, request composition, repetition count/median aggregation, per-decision correctness checks, meaning of each variant
4. **Functional results:** Pass matrix, implementation differences, policy size
5. **Capabilities:** Permission listing, explanations, runtime updates, error feedback, malformed-policy behavior, file watching
6. **Performance:** In-process latency by size, ABAC, wildcards, ManyUsers, multicore throughput, loading cost, HTTP load including srv cores/gen cores to help locate bottlenecks
7. **Usability and learning curve**
8. **Ecosystem/maturity:** Stars, contributors, issue/PR counts, latest formal release, Stack Overflow counts, governance (Casbin in the Apache Incubator; OPA CNCF-graduated; Speedle+ a continuation of Oracle Speedle); state query dates
9. **Observed issues:** Especially Speedle+ issues in section 8
10. **Limitations:** One machine, limited shapes, OPA dependence on Rego style, no distributed/high-availability testing

## 8. Previously observed issues to recheck on charming

These had been verified only on macOS. Recheck on Linux and explicitly identify the platform:

1. **Replacing an embedded Speedle+ policy file can terminate the host.** `pkg/store/file/fileStore.go` lines 341, 350, 354 call `log.Fatalf` when the file is deleted/renamed or re-watching fails. macOS observation: in-place writes reload in about 10 ms, but renaming a temporary file over the original exits with status 1. Linux uses inotify and may differ; verify with `watchcheck rename`.
2. **Malformed Speedle+ conditions do not fail loading.** `if amount <=` loads successfully with a log, then requests return `ERROR_IN_EVALUATION` and deny. `fmt.Println` at `pkg/eval/runtimeservice.go` lines 94, 118 prints the full policy to stdout.
3. **A misspelled Casbin matcher does not fail loading.** First `Enforce` reports `No parameter 'p_ob' found.` without a location. OPA reports these errors at compilation with filename/line.
4. **Other Speedle+ issues asserted by the historical handoff:** No official Go-recognized release (tag `v2.0`, module path without `/v2`); outdated README; Istio integration using removed Mixer; undocumented `authenticated_role`, `everyone_role`, `anonymous_role` (only in `api/ads/types.go`).

## 9. Earlier pitfalls

- **Speedle+ file watching:** Removing a watched file calls `log.Fatalf`. Do not put watched test policies in `t.TempDir()`, because cleanup would exit the entire process (`capabilities_test.go` had been changed to `os.MkdirTemp`).
- **Go benchmark `-bench` regex:** Matching is split into levels at `/`; `|` cannot cross levels. Use a form such as `'^BenchmarkRBAC(Mixed|Allow|Deny)$/^n=(100|1000|10000)$/'`.
- **zsh globs:** A nonmatching glob aborts the whole command line; avoid potentially unmatched patterns.
- **`pkill -f` matches itself:** Use a pattern like `"[r]un-all.sh"`; see section 4.4.
- **`speedle-ads` defaults to `0.0.0.0:6734`:** Explicitly pass `--endpoint 127.0.0.1:6734`.
- **Benchmark requests:** Repeating one request biases results optimistically, especially for Casbin. Use randomized `RBACMixed` as the main metric.
- **shrek load:** Browsers and other long-running applications kept load above 8 with nearly full swap, motivating the move to charming.

## 10. Reference: preliminary shrek figures (excluded from the report)

Median `RBACMixed` in-process decision time with `-cpu 2`, while shrek load was 8–11; only a rough order-of-magnitude reference:

| Engine | n=100 | n=1K | n=10K | n=100K |
|---|---|---|---|---|
| speedle | 3.3 µs | 4.6 µs | 5.0 µs | 5.0 µs |
| opa | 11.5 µs | 14.2 µs | 18.3 µs | 11.6 µs |
| casbin-tuned | 15.7 µs | 160 µs | 1.75 ms | 21 ms |
| casbin | 27 µs | 318 µs | 3.35 ms | 159 ms |

Loading at n=100K roles/1 million users: casbin 6.3 s / 1024 MB; opa 1.3 s / 197 MB; speedle 1.3 s / 500 MB. The original handoff expected charming's slower CPU to increase absolute latency but preserve ordering, and required investigating any changed ordering before reporting it.
