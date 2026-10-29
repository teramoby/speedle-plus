#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"
BIN=${BIN:-./bin}; T="$BIN/authzbench.test"
OUT=${OUT:-results/$(hostname -s)}; mkdir -p "$OUT"
STEPS=${STEPS:-"machine functional capabilities watchcheck loadcost bench-small bench-manyusers bench-100k-speedle bench-100k-opa bench-100k-casbin-tuned bench-100k-casbin parallel stress"}
log() { printf '[%s] %s\n' "$(date -Iseconds)" "$*" | tee -a "$OUT/run-all.log"; }
workers() { pgrep -c '[R]unner.Worker' || true; }
sample() {
  local now load count top swap
  now=$(date +%s); load=$(awk '{print $1}' /proc/loadavg); count=$(workers)
  top=$(ps -eo pid,pcpu,comm --sort=-pcpu | awk 'NR>1 && NR<=4 {printf "%s:%s:%s;",$1,$2,$3}')
  swap=$(awk '/^pswpin / {a=$2} /^pswpout / {b=$2} END {printf "pswpin=%s pswpout=%s",a,b}' /proc/vmstat)
  printf '%s load=%s workers=%s %s top=%s\n' "$now" "$load" "$count" "$swap" "$top" >> "$OUT/activity.log"
}
cleanup() {
  # Only terminate descendants launched by this run; never match global command names.
  python3 - "$$" <<'PY'
import os,signal,sys,pathlib
parent=int(sys.argv[1]); me=os.getpid(); pairs=[]
for p in pathlib.Path('/proc').iterdir():
 if not p.name.isdigit(): continue
 try:
  s=(p/'stat').read_text(); f=s[s.rfind(')')+1:].split(); pairs.append((int(p.name),int(f[1])))
 except (OSError,ValueError,IndexError): pass
def stop(pid):
 for child,ppid in pairs:
  if ppid==pid and child!=me:
   stop(child)
   try: os.kill(child,signal.SIGTERM)
   except ProcessLookupError: pass
stop(parent)
PY
}
trap cleanup EXIT
trap 'log "interrupted"; exit 130' INT TERM
: > "$OUT/activity.log"; : > "$OUT/run-all.log"; : > "$OUT/steps.tsv"
(while true; do sample; sleep 5; done) & MONITOR_PID=$!
wait_idle() {
  while true; do
    while [[ $(workers) != 0 ]]; do log 'waiting for CI'; sleep 5; done
    local begin lines
    begin=$(wc -l < "$OUT/activity.log")
    sleep "${COOLDOWN:-10}"; sample
    lines=$(tail -n +$((begin+1)) "$OUT/activity.log" | grep -Ec 'workers=[1-9]' || true)
    [[ "$lines" == 0 && $(workers) == 0 ]] && return 0
  done
}
bench() { "$T" -test.run '^$' -test.benchtime "${BENCHTIME:-1s}" -test.count "${COUNT:-5}" -test.timeout 0 "$@"; }
execute_step() {
  case "$1" in
  machine)
    {
      date -Iseconds; cat /etc/os-release; uname -a; uname -r; lscpu
      echo '=== frequency governors ==='
      for p in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do [[ -r "$p" ]] && echo "$p: $(cat "$p")"; done
      echo '=== turbo ==='; cat /sys/devices/system/cpu/intel_pstate/no_turbo 2>/dev/null || true
      echo '=== memory, disk, swap, affinity ==='; free -h; df -h /tmp; lsblk -o NAME,MODEL,SIZE,TYPE; taskset -pc $$; vmstat 1 3; uptime
      echo '=== build provenance captured on shrek ==='; cat build-info.txt; sha256sum bin/*
    } > "$OUT/machine.txt" 2>&1 ;;
  functional) "$T" -test.run '^(TestScenarios|TestPolicySize|TestGeneratedManyUsers)$' -test.v > "$OUT/functional.txt" 2>&1 ;;
  capabilities) "$T" -test.run '^(TestListPermissions|TestExplain|TestRuntimeUpdate|TestErrorMessages|TestBrokenPolicyBehavior)$' -test.v > "$OUT/capabilities.txt" 2>&1 ;;
  watchcheck)
    : > "$OUT/watchcheck.txt"
    for mode in inplace rename; do
      "$BIN/watchcheck" "$mode" >> "$OUT/watchcheck.txt" 2>&1; rc=$?
      echo "mode=$mode exit_code=$rc" >> "$OUT/watchcheck.txt"
    done; return 0 ;;
  loadcost) LOADCOST=1 "$T" -test.run '^TestLoadCost$' -test.v -test.timeout 0 > "$OUT/loadcost.txt" 2>&1 ;;
  bench-small)
    bench -test.cpu 2 -test.bench '^BenchmarkRBAC(Mixed|Allow|Deny)$/^n=(100|1000|10000)$/' > "$OUT/bench-seq.txt" 2>&1 || return $?
    bench -test.cpu 2 -test.bench '^Benchmark(ABAC|Wildcard)$' >> "$OUT/bench-seq.txt" 2>&1 ;;
  bench-manyusers) bench -test.cpu 2 -test.bench '^BenchmarkManyUsers$' > "$OUT/bench-manyusers.txt" 2>&1 ;;
  bench-100k-*) e=${1#bench-100k-}; AUTHZ_ENGINES=$e bench -test.cpu 2 -test.bench '^BenchmarkRBAC(Mixed|Allow|Deny)$/^n=100000$/' > "$OUT/bench-100k-$e.txt" 2>&1 ;;
  parallel) bench -test.cpu 1,2,4 -test.bench '^BenchmarkParallel$' > "$OUT/bench-par.txt" 2>&1 ;;
  stress) BIN=$BIN STRESS_LOGDIR="$OUT/stress-logs" ./stress.sh > "$OUT/stress.md" 2> "$OUT/stress.err" ;;
  *) echo "unknown step: $1" >&2; return 2 ;;
  esac
}
for step in $STEPS; do
  accepted=0
  for attempt in 1 2 3; do
    wait_idle; sample; start=$(date +%s); first=$(( $(wc -l < "$OUT/activity.log") + 1 ))
    log "start step=$step attempt=$attempt"
    execute_step "$step" & STEP_PID=$!; wait "$STEP_PID"; rc=$?
    sample; end=$(date +%s)
    dirty=$(tail -n +"$first" "$OUT/activity.log" | grep -Ec 'workers=[1-9]' || true)
    if [[ "$dirty" != 0 ]]; then
      log "contaminated step=$step attempt=$attempt rc=$rc; retry after CI ends"
      printf '%s\t%s\t%s\t%s\t%s\tcontaminated\n' "$step" "$attempt" "$start" "$end" "$rc" >> "$OUT/steps.tsv"
      dest="$OUT/contaminated/$step-attempt$attempt"; mkdir -p "$dest"
      cp "$OUT"/*.txt "$OUT"/stress.md "$OUT"/stress.err "$dest/" 2>/dev/null || true
      continue
    fi
    printf '%s\t%s\t%s\t%s\t%s\tclean\n' "$step" "$attempt" "$start" "$end" "$rc" >> "$OUT/steps.tsv"
    if [[ "$rc" != 0 ]]; then log "FAILED step=$step rc=$rc"; exit "$rc"; fi
    accepted=1; log "complete step=$step attempt=$attempt"; break
  done
  if [[ "$accepted" != 1 ]]; then log "FAILED CI remained disruptive after 2 reruns: $step"; exit 1; fi
done
sample; uptime >> "$OUT/machine.txt"; log "done: $OUT"; touch "$OUT/COMPLETE"
