// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package policysource provides the read-only policy store served by an ADS
// that synchronizes its policies from a Speedle policy source (Engine step E3
// of the policy distribution work, issue #277).
//
// The store is configured with storeType "policy-source". It speaks the
// /policy-source/v1 data protocol through package adsync, assembles the
// verified copies of several services into one pms.PolicyStore, and parses
// each policy body with pkg/pdl. Policy identifiers and names come from the
// snapshot; the local SPDL text is kept only as the parse input.
//
// # Configuration
//
// UpstreamURL is one HTTPS root or an ordered list. Direct mode uses APIKey
// and one upstream. Cascade mode omits APIKey and requires ClientCertFile,
// ClientKeyFile, CAFile and CacheDir. Cascade sends no bearer header and
// verifies the relay chain, serverAuth usage and URL host SAN against CAFile.
// TLS files reload every 30 seconds (TLSReloadInterval) and on SIGHUP; valid
// changes reconnect upstream channels and HTTP connections. Invalid changes
// retain the last valid generation. Optional CRLFiles and CRLURLs check relay
// revocation. See docs/POLICY_SOURCE.md for configuration and current scope.
//
// Every instance generates a UUIDv7 and keeps it in CacheDir. Cascade requires
// this directory, rejects corrupt retained state, and keeps its ID across
// certificate rotation. Direct mode without CacheDir creates a new ID on each
// start. ServeDownstream is not supported yet.
//
// Removed prototype settings, including DownstreamCredentialsFile and Services,
// fail startup. The error names ClientCertFile, ClientKeyFile, CAFile and the
// relay's downstreamClientCAPath. No service subset or connection/depth limit
// is accepted. Engines using other store types are unaffected.
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
// # Request gate
//
// The store implements adsync.RequestGate for the REST and gRPC entry points:
// it reports the instance and per-service state, and ForwardFallback sends one
// authorization request to UpstreamURL with the mode's credentials while a service is
// syncing or the instance is stale. FallbackWhileSyncing (default true) turns
// that path off. An authentication failure from the upstream stops the
// instance as revoked.
//
// The package never logs policy content or the API key.
package policysource
