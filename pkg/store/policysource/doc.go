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
// UpstreamURL is the policy-source root. APIKey is accepted only in the
// configuration file and retained as a redacted secret in Options. Every
// request carries the key and the instance ID, a UUIDv7 the ADS generates on
// its first request and keeps in CacheDir; without CacheDir every start is a
// new instance. The upstream registers the instance on its first channel
// request and assigns its services. ServeDownstream enables the downstream
// mTLS listener: the store mirrors each verified copy and exposes the
// policy-source protocol handler (DownstreamHandler) for the process to serve
// on its dedicated listener. A service is withheld from downstream serving
// while a copy is being fetched and verified, after a failed pass, and while
// the instance is paused or no longer live; it becomes servable again only at
// a confirmed head. The listener's endpoint, certificate, key, client CA and
// CRL sources are server configuration; the store has no fan-out, chain-depth
// or connection limits.
//
// PolicySourceURL, CredentialFile, SPEEDLE_ADS_CREDENTIAL,
// PolicySourcePublicKeys, Services, CloudFallbackURL, CloudCredentialFile,
// InstanceName, DownstreamCredentialsFile, MaxDownstream, MaxChainDepth and
// MaxConnectionsPerDownstream are rejected. Configure UpstreamURL and APIKey
// instead; select services in the Cloud console. Engines using other store
// types are unaffected.
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
// authorization request to UpstreamURL with the same key while a service is
// syncing or the instance is stale. FallbackWhileSyncing (default true) turns
// that path off. An authentication failure from the upstream stops the
// instance as revoked.
//
// The package never logs policy content or the API key.
package policysource
