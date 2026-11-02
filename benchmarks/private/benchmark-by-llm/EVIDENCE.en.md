# Evidence index and limits of proof

> English translation of [EVIDENCE.md](EVIDENCE.md), added 2026-11-02. This preserves the archived 2026-10-29 versions, observations, and time-specific statements; it adds no measurements.

| Evidence | What it establishes | Limitation |
|---|---|---|
| [functional.txt](results/retest-20261029/charming/functional.txt) | 30 cases across 9 scenarios, generator consistency, and policy size | Only the selected inputs and models were verified |
| [capabilities.txt](results/retest-20261029/charming/capabilities.txt) | Permission listing, deny diagnostics, updates, and error samples | Some probes are observational; PASS does not mean every malformed policy correctly failed to load |
| [watchcheck.txt](results/retest-20261029/charming/watchcheck.txt) | Linux inplace / rename output and exit codes | Pinned b276 version; one execution of each mode in this run |
| bench-*.txt ([summary](results/retest-20261029/charming/summary.en.md)) | 107 groups with 5 repetitions each; ns/op, B/op, allocs/op | Finite request pool/fixed requests; cached results listed separately |
| [stress.md](results/retest-20261029/charming/stress.md) | 90 windows; latency, throughput, errors/wrong decisions, CPU time | Same-host loopback, fixed-concurrency closed loop |
| [stress.err](results/retest-20261029/charming/stress.err) / [service logs](results/retest-20261029/charming/stress-logs/) | CPU affinity, configuration, rounds, PIDs, actual service parameters | Not production-environment records |
| [machine.txt](results/retest-20261029/charming/machine.txt) / [build-info.txt](results/retest-20261029/charming/build-info.txt) | Hardware, OS, compiler/modules, initial binary hashes | go version -m was read on shrek from the same Linux programs |
| [activity.log](results/retest-20261029/charming/activity.log) / [steps.tsv](results/retest-20261029/charming/steps.tsv) | 734 samples, 13 clean steps, load, cumulative paging | 5-second samples can miss short activity; no CI retry was actually triggered |
| [Deployment evidence](results/retest-20261029/deployment/) | nohup output, PID, Linux smoke results | Smoke results are excluded from formal performance statistics |
| [Rebuild consistency](results/retest-20261029/reproduction-binary-check.json) / [completion verification](results/retest-20261029/final-remote-verification.json) | All six local rebuilds match remote hashes; test processes/ports exited | Binaries are not uploaded; their hashes are retained |
| [research/](results/retest-20261029/research/) | GitHub/StackExchange query snapshots | Dates/query scope are in JSON; authentication permission fields were removed |
| [preparation/](evidence/preparation/) | Small HTTP smoke output on shrek | Not the formal test machine; not a basis for final performance figures |
| [executed-source.tar.gz](provenance/executed-source.tar.gz) | Public copy of the source snapshot pinned before execution | Redacted with normalized tar metadata; member hashes are recorded below |
| [Original/public mapping](provenance/publication-source-records.json) / [archive-member hashes](provenance/archive-entry-hashes.json) | Original/public hashes and applied transformations | The public copy is not an unchanged byte-for-byte original |
| [preparation-changes.patch](provenance/preparation-changes.patch) | Main changes between the Claude handoff and executed tools | Added generator tests are in the current source |

The original shrek report and pre-preparation scripts are retained in the [HISTORICAL archive](provenance/before-preparation-HISTORICAL.tar.gz) solely to audit the handoff history. Their figures do not enter the formal report. The user-pasted input also contains old figures explicitly marked as reference only.

During this run, `go vet`, local generator validation, and 100x benchmarks succeeded through chat tools, but their complete raw terminal logs were not independently saved; missing logs are not fabricated. Formal Linux functional output includes generator checks. Later archive validation is stored separately under validation/ and must not be presented as preparation-stage logs from around 09:00.

No complete earlier Claude conversation, automated extended graph-authorization tests, distributed/high-availability tests, open-loop load test, peak RSS measurements, or human learning-time study was provided. Absence of this evidence was not used to infer that a product lacks capabilities implemented through attributes, role hierarchies, or custom functions.
