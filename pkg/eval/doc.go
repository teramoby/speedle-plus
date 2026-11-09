// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package eval evaluates Speedle authorization policies.
//
// Custom functions are unsupported on js/wasm. Direct calls, delegated calls
// and expression calls return custom_function_unsupported. Evaluator
// construction and snapshot reloads validate every authorization and role
// policy condition before loading it. A condition that calls a custom function
// rejects the entire snapshot with custom_function_unsupported, including a
// call in a short-circuited branch. Incremental service, policy, role-policy
// and custom-function additions are validated as well. A rejected reload or
// mutation suspends authorization until a valid snapshot loads. Policies
// without custom-function calls load
// and evaluate normally; unused custom-function definitions are permitted.
// Native evaluation behavior is unchanged.
//
// The evaluator does not start periodic function-result cache cleanup on
// js/wasm. AddToCache remains available, expired entries are removed when the
// same key is read or CleanExpiredResult is called explicitly, and entries
// without a TTL remain until explicit deletion or cache replacement.
package eval
