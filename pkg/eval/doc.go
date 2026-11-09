// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package eval evaluates Speedle authorization policies.
//
// Custom functions are unsupported on js/wasm. Direct calls, delegated calls
// and expression calls return custom_function_unsupported. As with other
// condition-evaluation errors, a condition that calls a custom function
// evaluates as not matching. A GRANT or DENY guarded by such a condition is
// therefore skipped, and another matching GRANT can allow the request.
// Policies and role policies used on js/wasm must not rely on custom functions.
//
// The evaluator does not start periodic function-result cache cleanup on
// js/wasm. AddToCache remains available, expired entries are removed when the
// same key is read or CleanExpiredResult is called explicitly, and entries
// without a TTL remain until explicit deletion or cache replacement.
package eval
