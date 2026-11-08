// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package adsync implements the client side of the Speedle policy-source
// distribution protocol (Engine step E1): service discovery, signed snapshots
// and change pages, canonical hashing, Ed25519 signature verification, resume
// by revision and an optional per-service on-disk cache.
//
// It speaks the /policy-source/v1 data endpoints defined by the shared
// contract in Speedle Cloud. The golden vectors under testdata are copied
// verbatim from that repository and prove both implementations agree byte for
// byte on the canonical encodings and signatures.
//
// Machine owns the liveness and per-service sync state described in step E2.
// It is driven by confirmed acks and sync results and does not talk to the
// network itself; the notification channel is a later step, and
// pkg/store/policysource builds the read-only store on top of Syncer and
// Machine.
//
// Client also carries the fallback path of the request gate: while a service
// is syncing, ForwardFallback evaluates one authorization request on Speedle
// Cloud with the instance credential and returns Cloud's response unchanged.
// It retries only a connection that was never established, so a request that
// may have reached Cloud is not repeated, and it never logs policy content or
// credentials.
package adsync
