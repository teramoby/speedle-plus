// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package policysource provides the read-only policy store served by an ADS
// that synchronizes its policies from a Speedle policy source (Engine step E3
// of the policy distribution work, issue #277).
//
// The store is configured with storeType "policy-source". It speaks the
// /policy-source/v1 data protocol through package adsync, assembles the
// verified copies of several services into one pms.PolicyStore, and parses
// each policy body with pkg/pdl. Policy identifiers and names come from the
// signed snapshot; the local SPDL text is kept only as the parse input.
//
// # Change notification
//
// The assembled store is published atomically: readers either see the whole
// previous assembly or the whole new one. Every successful application of a
// new service revision, of a newly available service or of a service removal
// produces one FULL_RELOAD event. The evaluator reloads its runtime cache on
// that event, so a synchronization pass is a single switch for the whole
// store.
//
// A store of this type requires watching: the evaluator must subscribe to
// FULL_RELOAD events, and the evaluator's NewWithStore forces enableWatch on.
// A non-zero storeFailureGracePeriod is rejected at startup, because it would
// keep serving a copy that the (necessarily event-driven) store cannot
// refresh.
//
// # Read-only
//
// The policy source is authoritative. Every write method returns a read-only
// error. The store also implements pms.StoreHealthChecker: CheckHealth fails
// only when the adsync state machine reports the instance stopped, which makes
// authorization fail closed. Stale or syncing services do not fail the health
// check; the request gate routes them to the fallback or rejects them.
//
// The package never logs policy content or credentials.
package policysource
