# Offline WebAssembly engines

`pkg/eval` and `pkg/cfg` build with `GOOS=js GOARCH=wasm` without the custom-function HTTP/TLS transport or the assertion HTTP client. Native builds retain those transports.

## Custom-function behavior

Custom functions are unsupported on js/wasm. Direct, delegated and expression calls return `custom_function_unsupported`; expression calls do so before argument serialization or cache lookup.

Condition-evaluation errors are treated as non-matches. Consequently, a policy or role policy whose condition calls a custom function does not match, regardless of whether its effect is GRANT or DENY. Another matching GRANT can still allow the request. Policies used on js/wasm must not rely on custom functions.

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
