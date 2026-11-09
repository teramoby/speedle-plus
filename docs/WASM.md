# Offline WebAssembly engines

`pkg/eval` and `pkg/cfg` build with `GOOS=js GOARCH=wasm` without the custom-function HTTP/TLS transport or the assertion HTTP client. Native builds retain those transports.

## Custom-function behavior

Custom functions are unsupported on js/wasm. Direct, delegated and expression calls return `custom_function_unsupported`; expression calls do so before argument serialization or cache lookup.

Evaluator construction and snapshot reloads validate every authorization-policy and role-policy condition before loading it. If any condition calls a custom function, the entire snapshot is rejected with `custom_function_unsupported`. This includes calls in short-circuited branches and applies equally to GRANT and DENY policies. A rejected initial load returns no evaluator. A rejected reload does not activate the snapshot and suspends authorization until a valid snapshot loads successfully.

Policies without custom-function calls load and evaluate normally. Unused custom-function definitions are allowed, and built-in functions remain available. Native evaluation behavior, including its handling of condition-evaluation errors, is unchanged.

The evaluator does not start periodic function-result cache cleanup on js/wasm. `FuncResultCache.AddToCache` remains available to embedders. An expired entry is removed when `ReadFromCache` reads that key or when `CleanExpiredResult` is called explicitly; entries without a TTL remain until explicit deletion or cache replacement.

## AsserterConfig compatibility

`assertion.AsserterConfig` is an alias for the transport-free `assertion/config.AsserterConfig` type, and `cfg.Config.AsserterWebhookConfig` uses that defined type. Source assignments and JSON field names, zero values and round trips are preserved.

The defined type's package path changes from `github.com/teramoby/speedle-plus/pkg/assertion` to `github.com/teramoby/speedle-plus/pkg/assertion/config`. Package-path-based reflection therefore reports the new path. Gob values encoded behind an interface also use the new package-path-based type name; such gob interface streams are not wire-compatible across this change and require migration or an application-specific registration strategy.

## Verification

Run the wasm tests and dependency guard from the repository root:

```sh
GOOS=js GOARCH=wasm go test -count=1 \
  -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
  ./pkg/eval/wasmtest ./pkg/assertion/config
bash scripts/ci/check-wasm-dependencies.sh
```
