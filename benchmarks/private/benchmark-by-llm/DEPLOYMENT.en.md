# Deployment and execution

> English translation of [DEPLOYMENT.md](DEPLOYMENT.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

Temporary benchmark programs and loopback HTTP services were deployed on charming. Formal measurement ran on 2026-10-29 from 09:19:29 to 10:17:58 (Asia/Shanghai). No production service was changed or engine release published.

## Build and upload

The initial manual cross-build used shrek (macOS 27.0.1 / Go 1.27.2): GOOS=linux, GOARCH=amd64, CGO_ENABLED=0. The programs were authzbench.test, authzd, loadgen, watchcheck, opa, and speedle-ads. Pinned modules are recorded in [build-info.txt](results/retest-20261029/charming/build-info.txt).

The original working path was `/Users/LOCAL_USER/cc/authz-bench`. The archived [build-linux.sh](build-linux.sh) later extracted an equivalent build entry point: all six programs rebuilt in that same source directory had SHA-256 values identical to the original test package; see the [rebuild record](results/retest-20261029/reproduction-binary-check.json). The binaries contain Go build-path information. Rebuilding in another working directory or with a different toolchain/platform is not guaranteed to produce identical byte hashes, although the pinned dependencies and logic remain reproducible.

The initial package was `authz-bench-linux-20261029.tar.gz`, 97289997 bytes. Its full hash and source-snapshot provenance are in [build-artifacts.json](provenance/build-artifacts.json). This public copy excludes that complete package and executables, and includes source, dependency locks, and policies for rebuilding.

The following is a readable restatement of commands actually executed, with personal directory identifiers redacted. `charming.local` was the SSH target alias. Access used a key and BatchMode; the archive contains no private-key material.

```bash
scp -q -o BatchMode=yes dist/authz-bench-linux-20261029.tar.gz \
  charming.local:/tmp/authz-bench-linux-20261029-codex.tar.gz
ssh -o BatchMode=yes charming.local '
  test ! -e /tmp/authz-bench-codex-20261029 &&
  mkdir /tmp/authz-bench-codex-20261029 &&
  tar xzf /tmp/authz-bench-linux-20261029-codex.tar.gz \
    -C /tmp/authz-bench-codex-20261029 --strip-components=1'
ssh -o BatchMode=yes charming.local '
  cd /tmp/authz-bench-codex-20261029 &&
  test ! -e run.pid &&
  (nohup ./run-all.sh > run-all.out 2>&1 < /dev/null & echo $! > run.pid)'
```

The temporary directory was separate from the old `/tmp/authz-bench-linux`; earlier results were not deleted. The initial Linux extraction emitted LIBARCHIVE.xattr warnings, but the programs ran normally. Account and xattr metadata were removed from the public source tarballs. The new build script uses --no-xattrs.

## Service configuration

| Configuration | Loopback HTTP endpoint | Implementation |
|---|---|---|
| thin-casbin | 127.0.0.1:19000/check | Identical net/http wrapper; SyncedEnforcer with the default matcher |
| thin-opa | 127.0.0.1:19000/check | Identical wrapper; PreparedEvalQuery |
| thin-speedle | 127.0.0.1:19000/check | Identical wrapper; NewFromFile with watching disabled |
| opa-server | 127.0.0.1:18181/v1/data/authz/allow | opa run --server，log-level error |
| speedle-ads | 127.0.0.1:16735/authz-check/v1/is-allowed | file store; insecure=true; log-level / auditlog-level error; enable-watch false; gRPC disabled by default |

Configurations ran sequentially and did not share port 19000 concurrently. Server taskset=0,2; generator=1,3; both used GOMAXPROCS=2. Each pair comprises the two threads of one physical core. Each setting warmed up for 3 seconds and measured for 15 seconds at concurrency 1/16/64, with the complete suite interleaved across 3 rounds. Startup parameter structures print both default and actual flag values; the actual 127.0.0.1 endpoints and scripts are authoritative.

[run-all.sh](run-all.sh) terminates only its own PIDs/descendants; [stress.sh](stress.sh) terminates only ACTIVE_PID. No global pkill was used to stop other people's processes. CI Workers were sampled every 5 seconds, with an idle 10-second cooldown before each step. No CI activity was detected in this run, so the retry branch was not exercised.

## Completion and evidence

- [Raw nohup output](results/retest-20261029/deployment/run-all.out), [PID record](results/retest-20261029/deployment/run.pid)
- [Small Linux smoke test](results/retest-20261029/deployment/results-linux-smoke.md), [smoke stderr](results/retest-20261029/deployment/results-linux-smoke.err): excluded from formal performance results
- [Formal step log](results/retest-20261029/charming/run-all.log), [steps.tsv](results/retest-20261029/charming/steps.tsv), [COMPLETE](results/retest-20261029/charming/COMPLETE)
- [Post-run remote verification](results/retest-20261029/final-remote-verification.json): no processes or listening ports remained from this run; all six binaries still matched initial records, the original local package, and the local rebuild

This is a verification snapshot at completion, not continuous host monitoring. Both the current and older test directories were retained. Archival publication occurred after measurement, so archive-PR CI activity was not included in the measurement window.
