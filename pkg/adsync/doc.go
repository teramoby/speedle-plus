// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package adsync implements the client side of the Speedle policy-source
// distribution protocol (Engine steps E1, E2 and E5): service discovery,
// snapshots and change pages, canonical hashing, resume by revision, an
// optional per-service on-disk cache, the instance liveness and sync state
// machine, and the notification channel.
//
// An instance authenticates with an API key in direct mode or with mTLS in
// cascade mode (which sends no bearer token). It identifies itself with a
// UUIDv7 instance ID that it generates on first use and keeps in the cache
// directory; every request carries the ID. Integrity rests on TLS
// and on the canonical state and commit hashes, which every ack confirms.
//
// It speaks the /policy-source/v1 endpoints defined by the shared contract in
// Speedle Cloud. The golden vectors under testdata are copied verbatim from
// that repository and prove both implementations agree byte for byte on the
// canonical encodings.
//
// Discovery, snapshots, changes and caches have no fixed service, policy or
// payload-size ceiling. Changes retain pagination and are fetched until head.
// RequestTimeout bounds stalls (time without response bytes), not the total
// download duration. JSON responses are decoded from their streams. Channel
// frames have no size limit either; read and write timeouts bound a stalled
// frame, and a frame the source refuses as too large is logged with its size.
// Error bodies remain bounded.
// Removed-service tombstones retain only the latest 1,024 entries; live
// services are never evicted to meet that history bound.
//
// Machine owns the liveness and per-service sync state described in step E2. It
// is driven by confirmed acks and sync results and does not talk to the network
// itself. Channel carries the instance-level notification channel described in
// step E5: one WebSocket connection, or HTTPS long polling when the upgrade
// does not work, carrying hello, heartbeat, ack, head notification and command
// messages for every service of the instance. Channel feeds confirming acks and
// notifications into a Machine.
//
// Client also carries the forwarding path of the request gate: while a service
// is syncing or stale, ForwardFallback evaluates one authorization request on
// the upstream with the same key and returns the upstream's response
// unchanged. It retries only a connection that was never established, so a
// request that may have reached the upstream is not repeated, and it never
// logs policy content or the key.
//
// pkg/store/policysource builds the read-only store on top of Syncer and
// Machine. The package never parses SPDL and never logs policy content or
// the API key.
package adsync
