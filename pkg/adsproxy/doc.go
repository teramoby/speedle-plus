// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package adsproxy serves an ADS instance's synchronized policy copies to
// downstream ADS instances over mutual TLS.
//
// An ADS that holds verified copies of one or more services can act as a relay
// for other ADS instances. It speaks the same /policy-source/v1 protocol as
// Speedle Cloud: service discovery, snapshots, change pages, and the instance
// notification channel over WebSocket or HTTPS long polling. Every valid
// client certificate grants access to every service the relay holds; there are
// no per-certificate service subsets.
//
// # Listener
//
// Listener is a separate TLS listener; policy-source routes are never exposed
// on the application listener. It requires and verifies a client certificate
// (TLS 1.2 or newer, clientAuth usage) and rejects missing, expired, untrusted
// and revoked certificates during the handshake. Session resumption is
// disabled and the server certificate, client CA and CRLs reload without
// restarting: local files are re-read every 30 seconds and on SIGHUP, HTTPS
// CRLs are fetched on the CRL refresh interval. Because tls.Config is never
// mutated, a handshake that raced a reload is rechecked before its connection
// is admitted; a reload that changes the client CA or CRL set closes every
// established connection, so keep-alive connections, WebSockets and held polls
// cannot outlive material that was revoked or has expired.
//
// # Availability
//
// Server stops distributing while its caller reports the instance paused, or
// no longer live and therefore unable to vouch for the copies it holds. Data
// requests (including the three authorization operations) then answer 503
// with distribution_paused, the policy-source routes refuse new channels and
// held polls, established channels close, and a held poll is released instead
// of waiting out its hold. Distribution resumes without a restart once the
// instance is live and unpaused again.
//
// # CRL revocation
//
// CRLs come from PEM or DER files and/or HTTPS URLs (conditional fetches, no
// redirects, a ten-second stall timeout). Each CRL's issuer, signature and
// validity window are checked. Delta CRLs, CRLs scoped by an issuing
// distribution point and CRLs with any other unsupported critical extension
// are rejected: their serials do not describe the issuer's full certificate
// set. A newer CRL is never replaced by an older one. Each issuing CA in the
// client CA bundle needs
// fresh coverage; the serials of every fresh CRL for that issuer are unioned.
// Startup fails when a required CRL is unusable. A refresh failure keeps the
// last good CRL; a CRL that expires without replacement rejects new
// connections for its issuer and closes existing connections at expiry.
//
// # Session identity
//
// A downstream session is identified by the verified client certificate
// fingerprint together with the UUIDv7 the downstream reports in its hello or
// heartbeat. The certificate is the credential; the UUID is a self-reported
// operational identity that grants no extra access. A reconnect of the same
// pair replaces its previous channel and is counted once. A UUID collision
// under another certificate is logged with both fingerprints and never blocks
// or evicts the legitimate session. DownstreamCount is the number of distinct
// direct downstream sessions with an active channel or held poll.
//
// # Mirror
//
// A Mirror holds the verified snapshot each service was synchronized from and
// a bounded log of the commits applied after it. The synchronization loop
// feeds it the snapshots and change pages the ADS pulled; the mirror verifies
// them again. Snapshot anchoring is what makes hash pass-through possible: the
// snapshot response served to a downstream is the exact snapshot the proxy
// received, and the downstream catches up through the proxy's log.
//
// The log is bounded per service (by commit count and by approximate bytes).
// When a service reaches the bound it stops being advertised, because the proxy
// cannot produce a fresh snapshot on its own; the process should refresh
// the snapshot (Mirror reports such services through NeedsSnapshot). A fresh
// snapshot resets the log and makes the service serviceable again.
// Notifications are only accelerators: an ack always carries every service's
// head, so a lost notification delays a sync but cannot lose a change.
//
// # Limits
//
// The engine imposes no fan-out, chain-depth or CRL-size limit and has no
// settings for them; those are Cloud product concerns. The one bound the
// proxy enforces is the replay-protection store shared by active service
// copies and removed-service tombstones: a tombstone is a deletion guard and
// is never evicted, so a new service is refused once the store is full rather
// than forgetting a guard. The same bound keeps one channel ack encodable.
//
// The package never logs policy content or credentials.
package adsproxy
