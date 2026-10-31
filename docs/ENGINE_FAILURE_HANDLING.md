# Authorization engine failure handling

`IsAllowed` contains a Go `defer`/`recover` boundary. A recovered panic aborts the
whole decision, returns `false`, `ERROR_IN_EVALUATION` and an `EvalEngineError`.
This includes a panic in a grant or deny condition, a customer-function adapter,
or an identity asserter. It never turns a panicking deny into a nonmatch while
allowing another grant. Diagnose drops partial role/policy results; role and
permission queries return no partial grants on error. Identity assertion runs on a private subject and private JSON-shaped attribute
containers and commits them only on success,
so a failed callback cannot leave a reused request partially trusted. Locks
unwind before the recovery boundary, and a later healthy request can still
succeed.

Errors are logged at error level. Panic logs contain the operation, panic type
and stack trace; panic payloads are kept out of responses and these logs. If the
logger itself panics, recovery falls back to standard error and still denies. The
REST decision response remains HTTP 200 with `allowed:false` and an error;
gRPC keeps its existing RPC-error contract. gRPC stops on assertion failure
instead of ignoring that error and forwarding a possibly partially asserted
identity.

The same request boundary cannot recover a panic in another goroutine.
Engine-owned condition-cache updaters, cache cleanup and watch events have their
own boundaries. A panicking watch event or a failed incremental synchronization marks the
authorization cache unusable;
requests fail closed until a complete snapshot reload succeeds. Cache batches
and decisions share a read/write gate. A decision holds its read lock from the
failed-state check through evaluation; a batch sets failure before releasing
its write lock. Concurrent requests wait for the whole batch rather than
observing partially applied updates. Startup and
explicit refresh return errors on panic instead of terminating the process.
A read-only refresh failure retains the complete snapshot's data but, by default,
suspends its use for authorization. File, etcd, MySQL and MongoDB implement the
same lightweight health-check interface. Both watched and static evaluators
check health about once per second (static evaluators probe on requests), with
a one-second probe timeout. Watch/read failures can detect outages sooner.
Only a successful full snapshot read/install restores authorization; connectivity
alone and old incremental events cannot do so. In watch mode a detected fault
also discards the old subscription and requires a new subscription before the
recovery read. Manual successful reads cannot clear that state or reset its
grace deadline while subscription recovery is still pending. Built-in watch events invalidate
the snapshot and cause a new complete read, preventing queued pre-outage data
from restoring revoked grants. Etcd recovery reads all pages at one revision.

`storeFailureGracePeriod` / `--store-failure-grace-period` /
`SPDL_STORE_FAILURE_GRACE_PERIOD` defaults to `0s`. A finite explicit value allows
the last complete snapshot to remain usable for that duration after the first
detected failure. Repeated failures do not reset the deadline. Partial-cache
failures and authorization panics still deny immediately. An initial load failure
has no snapshot to use. A lost file retries indefinitely instead of calling
Fatal/Exit; cancellation unblocks watches even without an event consumer.

File PMS writers serialize before touching the live file, sync a complete
same-directory temporary file and atomically rename it into place. Separate
readers never observe normal writes as truncated/partial JSON. Existing mode
bits and configured symlinks are preserved (including recreation of a missing
symlink target; looping chains fail with a bounded error); atomic replacement requires parent
directory write access and creates the replacement as the writer account.
A concurrent-manager regression reproduces unexpected EOF on the old writer.

This intentionally changes the old TestFullReload availability expectation:
a failed read now denies rather than using stale grants without a time bound.
With watching disabled, ordinary policy updates still require Refresh, but
recovery from an outage always reloads the complete store. Failure detection
and policy propagation remain asynchronous and bounded by probes and backend
operation timeouts, not instantaneous revocation guarantees.

## Stack exhaustion

A real Go runtime stack overflow is fatal and cannot be caught with `recover`.
The [Go runtime documentation](https://pkg.go.dev/runtime/debug#SetMaxStack)
states that exceeding the stack limit crashes the program. This implementation
prevents the known engine paths from reaching that state:

- Role ancestor and descendant traversal use explicit heap stacks and visited
  state. Deep acyclic graphs do not consume one Go frame per role. Rooted cycles
  remain supported; a closed cyclic dependency with no independent subject
  source returns an evaluation error rather than recursing forever.
- Condition constructors reject more than 32,768 tokens or 128 nested clauses
  before entering the recursive planner. Tokens inside quoted strings and
  escaped variable names do not count as clauses. Conditions over these limits
  fail the whole evaluation; an alternate grant cannot hide the failure.
- Subprocess regressions reduce Go's maximum stack and exercise 20,000-level
  role graphs, a valid explicit-subject cycle whose only entry is pending denial, the original
  malformed closed-cycle probe and oversized/nested
  conditions, followed by healthy requests.

This is panic containment and stack-exhaustion prevention, not recovery from
arbitrary fatal Go runtime failures. Fatal faults in arbitrary application code,
cgo or an unsupervised goroutine require process isolation if the host process
must survive them.

## Compatibility and tests

Role/condition evaluation inputs and expected results remain unchanged. The
full-refresh availability test intentionally changes to the new fail-closed
contract; watcher tests now accept explicit unavailability events. This
change does not reintroduce PublicGrant, change policy-ID rules, or restore the
other reverted changes from #166. Normal condition evaluation errors retain the
existing nonmatching-condition behavior; an unexpected panic or a condition
complexity-limit failure aborts the decision. Previously unbounded expressions
above the new limits now return an error.

The new `TestFeature*Recovery`, panic and stack-exhaustion regressions run in the
existing four-store nightly alongside every legacy evaluator, CLI, REST/gRPC,
PMS/ADS runtime and cache suite. Each backend must execute them; database
availability is required in its owning backend run.
