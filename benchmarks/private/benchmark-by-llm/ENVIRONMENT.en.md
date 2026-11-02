# Test environment and versions

> English translation of [ENVIRONMENT.md](ENVIRONMENT.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

This page is extracted from the full report. Original machine output and compiler metadata remain linked. The hostnames shrek / charming identify the two machines for this run; personal account directories were replaced with LOCAL_USER / REMOTE_USER.


| Item | Conditions for this run |
|---|---|
| CPU | Intel Core i5-7360U @ 2.30 GHz; 2 physical cores, 4 logical threads |
| Memory and swap | 15 GiB RAM, 2 GiB swap; approximately 10 GiB available initially; approximately 2 GiB swap already in use |
| Disk | APPLE SSD AP0128J NVMe, approximately 113 GiB; /tmp on the root partition, approximately 23 GiB free initially |
| System | Linux Mint 22.3 Zena, based on Ubuntu noble; kernel 7.0.0-38-generic, x86_64 |
| CPU governor / turbo | powersave on all four threads; intel_pstate/no_turbo=0, so turbo was not disabled; system configuration was not changed |
| Build host | shrek.local; Apple M1 Pro, 16 GiB / macOS 27.0.1 (26A434); Go 1.27.2 |
| Build settings | GOOS=linux GOARCH=amd64 CGO_ENABLED=0; no additional performance patches |
| Casbin | github.com/casbin/casbin/v3 v3.11.0 |
| OPA | github.com/open-policy-agent/opa v1.21.4; in-process v1/rego |
| Speedle+ | github.com/teramoby/speedle-plus v0.0.0-20261028230603-b27604c077ce; fixed commit b27604c077ce |
| In-process CPU | Sequential benchmarks: -test.cpu 2; Parallel: 1, 2, 4; no additional taskset restriction, online CPUs 0–3 allowed |
| HTTP CPU | Server taskset 0,2 (two threads of one physical core); load generator taskset 1,3; both GOMAXPROCS=2 |
| Run window | 2026-10-29T09:19:29+08:00 to 2026-10-29T10:17:58+08:00 |
| CI and load | 734 activity samples; maximum Worker count 0; 1-minute load 0.31–3.96; CI-contaminated attempts 0 |
| Paging | Cumulative swap-in increased by 1115 pages and swap-out by 35 pages; pre-existing swap usage and paging during this run are recorded separately |

Full `lscpu`, kernel, governor, disk, memory, binary SHA-256, and `go version -m` output are in [machine.txt](results/retest-20261029/charming/machine.txt). charming did not have Go installed. Module metadata was read on shrek from the same Linux binaries, whose SHA-256 values were then recorded on charming; see [build-info.txt](results/retest-20261029/charming/build-info.txt) and the [compiler-host record](results/retest-20261029/charming/compiler-host.txt).

The same Go source was rebuilt on shrek using the added build script. All six Linux programs had SHA-256 values identical to the binaries actually measured; see [rebuild verification](results/retest-20261029/reproduction-binary-check.json).

Original records: [activity.log](results/retest-20261029/charming/activity.log), [step/retry status](results/retest-20261029/charming/steps.tsv), [run-all.log](results/retest-20261029/charming/run-all.log). Activity was sampled every 5 seconds; each step required no Worker and a 10-second cooldown beforehand. Steps detecting a Worker could be retried up to twice. Sampling cannot exclude activity shorter than 5 seconds. Desktop sessions and other resident processes were retained; other people's processes were not stopped.
