# Authorization engine failure handling

`IsAllowed` contains a Go `defer`/`recover` boundary. A recovered panic aborts the
whole decision, returns `false`, `ERROR_IN_EVALUATION` and an `EvalEngineError`.
This includes a panic in a grant or deny condition, a customer-function adapter,
or an identity asserter. It never turns a panicking deny into a nonmatch while
allowing another grant. Diagnose drops partial role/policy results; role and
permission queries return no partial grants on error. Identity assertion runs on a private subject and commits it only on success,
so a failed callback cannot leave a reused request partially trusted. Locks
unwind before the recovery boundary, and a later healthy request can still
succeed.

Errors are logged at error level. Panic logs contain the operation, panic type
and stack trace; panic payloads are kept out of responses and these logs. The
REST decision response remains HTTP 200 with `allowed:false` and an error;
gRPC keeps its existing RPC-error contract. gRPC stops on assertion failure
instead of ignoring that error and forwarding a possibly partially asserted
identity.

The same request boundary cannot recover a panic in another goroutine.
Engine-owned condition-cache updaters, cache cleanup and watch events have their
own boundaries. A panicking watch event or a failed incremental synchronization marks the
authorization cache unusable;
requests fail closed until a complete snapshot reload succeeds. Startup and
explicit refresh return errors on panic instead of terminating the process.

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
  role graphs, the original malformed closed-cycle probe and oversized/nested
  conditions, followed by healthy requests.

This is panic containment and stack-exhaustion prevention, not recovery from
arbitrary fatal Go runtime failures. Fatal faults in arbitrary application code,
cgo or an unsupervised goroutine require process isolation if the host process
must survive them.

## Compatibility and tests

Existing test inputs and expected results remain unchanged. In particular, this
change does not reintroduce PublicGrant, change policy-ID rules, or restore the
other reverted changes from #166. Normal condition evaluation errors retain the
existing nonmatching-condition behavior; an unexpected panic or a condition
complexity-limit failure aborts the decision. Previously unbounded expressions
above the new limits now return an error.

The new `TestFeature*Recovery`, panic and stack-exhaustion regressions run in the
existing four-store nightly alongside every legacy evaluator, CLI, REST/gRPC,
PMS/ADS runtime and cache suite. Each backend must execute them; database
availability is required in its owning backend run.
