# Policy-source downstream client

Set `storeType` to `policy-source` to synchronize an ADS from a policy source.
Application listener authentication is independent of this store. Other store
implementations keep their existing behavior.

Direct mode uses `APIKey` and one HTTPS `UpstreamURL`. Cascade mode omits
`APIKey` and requires `ClientCertFile`, `ClientKeyFile`, `CAFile` and `CacheDir`.
A cascade client sends its generated instance ID and authenticates with its
certificate; it sends no Authorization bearer header. `TLSReloadInterval` and
the `CRL*` properties are cascade-only; direct mode rejects these settings.

```json
{
  "storeType": "policy-source",
  "storeProps": {
    "UpstreamURL": ["https://relay-one.example:7443", "https://relay-two.example:7443"],
    "ClientCertFile": "/etc/speedle/client.pem",
    "ClientKeyFile": "/etc/speedle/client.key",
    "CAFile": "/etc/speedle/relay-ca.pem",
    "CacheDir": "/var/lib/speedle/downstream"
  }
}
```

`UpstreamURL` also accepts a single string. Lists preserve preference order;
empty lists, duplicate addresses, non-HTTPS addresses, user information,
queries and fragments are rejected. The downstream client configuration step
retains the candidates and connects to the first entry; priority failover and
relay serving are separate implementation steps. `ServeDownstream` continues
to fail startup until the relay listener is available.

Cascade verifies the relay's certificate chain using only `CAFile`, including
certificate validity, server authentication usage and the URL's hostname/IP
SAN. TLS versions before 1.2 are rejected. Each instance generates its own
UUIDv7 and persists it in `CacheDir`. No certificate common-name matching or
configured instance ID is used. Keep this directory across restarts and
certificate rotations. Cascade startup rejects corrupt retained state.

The store polls TLS files every 30 seconds; `TLSReloadInterval` accepts a Go
duration to override that period. The `speedle-ads` command routes SIGHUP to
the store's owned watcher. Embedded hosts can call `RequestTLSReload` from
their own signal handler; the store does not register process signals.
`adsync.NewClientContext` supports cancellation during initial CRL downloads. A valid
replacement is installed as one generation, then upstream connections,
including WebSockets, held polls and idle HTTP connections, are closed to
reconnect with that generation. A malformed replacement logs a warning and
retains the last valid generation. Certificate expiry closes connections even
when they remain active.

Optional `CRLFiles` and `CRLURLs` contain local full X.509 CRLs and HTTPS CRL
addresses for relay certificate revocation checks. When either property is
present, at least one source is required; omit both properties to disable
optional revocation checking. For CRL verification,
`CAFile` must contain each CRL issuer's CA certificate. Configured CRLs must have
trusted signatures and current validity windows. Revoked relay certificates
and chains without fresh issuer coverage fail verification. `CRLMaxAge`
defaults to 24 hours for CRLs without `NextUpdate`. The TLS poll and SIGHUP also
refresh CRLs; `CRLRefreshInterval`, when shorter, requests more frequent polls.
A failed refresh retains the previous generation only while it remains valid.
CRL and CA changes reconnect connections; CRL expiry closes them. No size,
upstream-count or chain-depth setting is introduced.

`DownstreamCredentialsFile`, `Services`, `MaxDownstream`, `MaxChainDepth` and
`MaxConnectionsPerDownstream` are removed store properties. They fail startup,
including when explicitly set to an empty value. Downstream credentials and
service subsets are not migrated automatically. Use `ClientCertFile`,
`ClientKeyFile`, `CAFile` and the relay's `downstreamClientCAPath`; a downstream
receives the relay's service set.
