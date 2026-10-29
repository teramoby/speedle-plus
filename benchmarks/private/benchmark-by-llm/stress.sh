#!/usr/bin/env bash
# Full defaults: two policy shapes, three concurrency levels, three interleaved rounds.
set -euo pipefail
cd "$(dirname "$0")"
N=${1:-10000}; D=${2:-15s}; CS=${3:-"1 16 64"}
MN=${MANYUSERS_N:-100000}; ROUNDS=${ROUNDS:-3}; WARMUP=${WARMUP:-3s}
BIN=${BIN:-./bin}; DATASETS=${DATASETS:-"plain manyusers"}
WORK=$(mktemp -d); ACTIVE_PID=""
LOGDIR=${STRESS_LOGDIR:-$WORK/logs}; mkdir -p "$LOGDIR"
stop() { if [[ -n "$ACTIVE_PID" ]]; then kill "$ACTIVE_PID" 2>/dev/null || true; wait "$ACTIVE_PID" 2>/dev/null || true; ACTIVE_PID=""; fi; }
trap 'stop; rm -rf "$WORK"' EXIT
trap 'exit 130' INT TERM
# Respect cpuset restrictions and separate physical cores when possible.
SRV_CPUS=""; GEN_CPUS=""; SRV_PROCS=${SRV_PROCS:-2}; GEN_PROCS=${GEN_PROCS:-2}
if command -v taskset >/dev/null && [[ -r /sys/devices/system/cpu/cpu0/topology/thread_siblings_list ]]; then
  read -r SRV_CPUS GEN_CPUS SRV_PROCS GEN_PROCS < <(python3 - <<'PY'
import os,pathlib
allowed=sorted(os.sched_getaffinity(0)); first=allowed[0]
def expand(s):
 out=[]
 for part in s.strip().split(','):
  ab=list(map(int,part.split('-'))); out.extend(range(ab[0],ab[-1]+1))
 return out
siblings=expand(pathlib.Path(f'/sys/devices/system/cpu/cpu{first}/topology/thread_siblings_list').read_text())
srv=[c for c in allowed if c in siblings]; gen=[c for c in allowed if c not in srv]
if not gen: srv,gen=allowed[:max(1,len(allowed)//2)],allowed[max(1,len(allowed)//2):]
if not gen: gen=srv
print(','.join(map(str,srv)),','.join(map(str,gen)),len(srv),len(gen))
PY
  )
fi
srv() { if [[ -n "$SRV_CPUS" ]]; then GOMAXPROCS=$SRV_PROCS exec taskset -c "$SRV_CPUS" "$@"; else GOMAXPROCS=$SRV_PROCS exec "$@"; fi; }
gen() { if [[ -n "$GEN_CPUS" ]]; then GOMAXPROCS=$GEN_PROCS taskset -c "$GEN_CPUS" "$@"; else GOMAXPROCS=$GEN_PROCS "$@"; fi; }
echo "CPU allocation: server=$SRV_CPUS GOMAXPROCS=$SRV_PROCS generator=$GEN_CPUS GOMAXPROCS=$GEN_PROCS" >&2
wait_ready() {
  for ((i=0;i<1200;i++)); do
    if ! kill -0 "$ACTIVE_PID" 2>/dev/null; then echo "server exited; logs: $LOGDIR" >&2; return 1; fi
    if curl -s --max-time 1 -o /dev/null "$1"; then return 0; fi
    sleep 0.25
  done
  echo "server not ready: $1" >&2; return 1
}
for kind in $DATASETS; do
  n=$N; [[ "$kind" == manyusers ]] && n=$MN
  mkdir -p "$WORK/$kind"
  "$BIN/authzd" -kind "$kind" -gen "$WORK/$kind" -n "$n" >/dev/null
done
printf '| dataset | round | setup | conc | req/s | p50 µs | p90 µs | p99 µs | p99.9 µs | max µs | errors | wrong | srv cores | gen cores |\n'
printf '|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n'
for ((round=1;round<=ROUNDS;round++)); do
  for kind in $DATASETS; do
    n=$N; [[ "$kind" == manyusers ]] && n=$MN
    base="$WORK/$kind/${kind}_${n}"
    for setup in thin-casbin thin-opa thin-speedle opa-server speedle-ads; do
      logfile="$LOGDIR/$kind-round$round-$setup.log"
      case "$setup" in
      thin-*)
        srv "$BIN/authzd" -engine "${setup#thin-}" -kind "$kind" -n "$n" -addr 127.0.0.1:19000 >"$logfile" 2>&1 &
        target=thin; url=http://127.0.0.1:19000/check; health=http://127.0.0.1:19000/health ;;
      opa-server)
        srv "$BIN/opa" run --server --addr 127.0.0.1:18181 --log-level error "${base}_opa.rego" "${base}_opa.json" >"$logfile" 2>&1 &
        target=opa; url=http://127.0.0.1:18181/v1/data/authz/allow; health=http://127.0.0.1:18181/health ;;
      speedle-ads)
        srv "$BIN/speedle-ads" --store-type file --filestore-loc "${base}_speedle.spdl" --endpoint 127.0.0.1:16735 --insecure=true --log-level error --auditlog-level error --enable-watch false >"$logfile" 2>&1 &
        target=speedle; url=http://127.0.0.1:16735/authz-check/v1/is-allowed; health=$url ;;
      esac
      ACTIVE_PID=$!
      wait_ready "$health"
      for c in $CS; do
        echo "round=$round dataset=$kind n=$n setup=$setup c=$c pid=$ACTIVE_PID" >&2
        row=$(gen "$BIN/loadgen" -target "$target" -url "$url" -kind "$kind" -n "$n" -c "$c" -d "$D" -warmup "$WARMUP" -srv-pid "$ACTIVE_PID" -label "$setup")
        printf '| %s n=%s | %s %s\n' "$kind" "$n" "$round" "$row"
      done
      stop
    done
  done
done
