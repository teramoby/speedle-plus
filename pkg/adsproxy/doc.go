// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package adsproxy serves an ADS instance's synchronized policy copies to
// downstream ADS instances (Engine step E6 of the policy distribution work,
// issue #277).
//
// An ADS that holds verified copies of one or more services can act as a policy
// source for other ADS instances. It speaks the same /policy-source/v1 protocol
// as Speedle Cloud: service discovery, signed snapshots, signed change pages,
// and the instance notification channel over WebSocket or HTTPS long polling.
// Downstream instances keep pointing at the same endpoints and verify the same
// Cloud signatures; a proxy forwards sourceId, epoch, hashes and signatures
// unchanged and never re-signs or rewrites them.
//
// # Synced copies and the change log
//
// A Mirror holds the signed snapshot each service was synchronized from and a
// bounded log of the signed commits applied after it. The synchronization loop
// feeds it the signed snapshots and change pages the ADS pulled; the mirror
// verifies them again. Snapshot anchoring is what makes signature pass-through
// possible: the snapshot response served to a downstream is the exact signed
// snapshot the proxy received, and the downstream catches up through the
// proxy's log.
//
// The log is bounded per service (by commit count and by approximate bytes).
// When a service reaches the bound it stops being advertised, because the proxy
// cannot produce a fresh signed snapshot on its own; the process should refresh
// the snapshot (Mirror reports such services through NeedsSnapshot). A fresh
// signed snapshot resets the log and makes the service serviceable again.
// Notifications are only accelerators: an ack always carries every service's
// head, so a lost notification delays a sync but cannot lose a change.
//
// # Downstream credentials
//
// Downstream credentials are configured locally in a 0600 file, one line per
// downstream:
//
//	downstreamId:tokenHash:serviceIds
//	edge-1:9f2c...e1:*
//	edge-2:71ab...07:01J...S1,01J...S2
//
// tokenHash is the lowercase hexadecimal SHA-256 digest of the bearer token.
// serviceIds is * or a comma-separated list of service ULIDs; a credential can
// only see that subset, and a service outside it is rejected exactly like an
// unknown service. CredentialFile.Reload replaces the whole set atomically; the
// process should call it on SIGHUP (Server.WatchReload wires a signal channel).
//
// # Limits
//
// A proxy accepts at most MaxDownstream distinct downstreams and rejects
// deeper chains than MaxChainDepth proxy hops. A chain that repeats an instance,
// places this proxy in an impossible position, or contains one of this proxy's
// own upstream instances is rejected as a loop. Rejections of a channel are
// reported with the contract's error codes: 503 fanout_limit and 403
// chain_too_deep or chain_loop. When distribution is paused, every endpoint
// answers 503 distribution_paused and existing channels close at the next
// message.
//
// The package never logs policy content or credentials.
package adsproxy
