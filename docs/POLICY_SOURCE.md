# Policy-source downstream client

Set `storeType` to `policy-source` to synchronize an ADS from a policy source.
Application listener authentication is independent of this store. Other store
implementations keep their existing behavior.

Direct mode uses `APIKey` and one HTTPS `UpstreamURL`. Cascade mode omits
`APIKey` and requires `ClientCertFile`, `ClientKeyFile`, `CAFile` and `CacheDir`.
An empty or missing `APIKey` without complete mTLS credentials fails startup
with an error naming both authentication choices.
A cascade client sends its generated instance ID and authenticates with its
certificate; it sends no Authorization bearer header. `TLSReloadInterval` and
the `CRL*` properties are cascade-only; direct mode rejects these settings.

```json
{
  "storeConfig": {
    "storeType": "policy-source",
    "storeProps": {
      "UpstreamURL": ["https://relay-one.example:7443", "https://relay-two.example:7443"],
      "ClientCertFile": "/etc/speedle/client.pem",
      "ClientKeyFile": "/etc/speedle/client.key",
      "CAFile": "/etc/speedle/relay-ca.pem",
      "CacheDir": "/var/lib/speedle/downstream"
    }
  }
}
```

`UpstreamURL` also accepts a single string. Lists preserve preference order;
empty lists, duplicate addresses, non-HTTPS addresses, user information,
queries and fragments are rejected. The downstream client retains the
candidates and connects to the first entry; priority failover is a separate
implementation step. `ServeDownstream` enables relay serving: the process
serves the policy-source protocol on the dedicated downstream mTLS listener
configured by the `downstream-*` server settings.

Cascade verifies the relay's certificate chain using only `CAFile`, including
certificate validity, server authentication usage and the URL's hostname/IP
SAN. TLS versions before 1.2 are rejected. Each instance generates its own
UUIDv7 and persists it in `CacheDir`. No certificate common-name matching or
configured instance ID is used. Keep this directory across restarts and
certificate rotations. Cascade startup rejects corrupt retained state and names
the offending file. A copied or restored cache can also fail this check when
its permissions are no longer `0600`. Delete the named `service-*.json` file to
re-fetch and re-verify that service from the relay; deleting the whole
directory also discards the retained CRL floors.

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
`CAFile` must contain each CRL issuer's CA certificate and the trust roots
for HTTPS CRL hosts. A CRL is trusted only from a CA in `CAFile`, so a relay
chain that presents an intermediate needs the leaf's issuer in `CAFile`, not
only the root; that issuer then becomes the trust anchor, and fresh coverage is
required for every certificate below it. This is one shared trust domain: every CA added to
`CAFile` is trusted for both relay certificates and HTTPS CRL hosts. Adding a
public or enterprise CA for a CRL host also expands relay trust to that CA.
Use a CRL host under the relay's intended PKI, or download CRLs separately
and use `CRLFiles` when that expanded trust is unacceptable. No separate CRL
transport trust bundle is configured. CRL fetches honor `RequestTimeout` for
TLS handshakes, response headers and gaps between response bytes. One transfer
also has a fixed ten-minute hard limit, so a publisher that keeps sending
bytes cannot hold startup or a reload forever. Sources are fetched in sequence,
so each configured source can add up to that limit to startup or a reload.
Startup requires every configured source: an unreadable file or an unreachable
URL fails startup rather than running with a configured revocation check
disabled. There is no response-size limit and no setting for one. Configured CRLs must have
trusted signatures and current validity windows. Unsupported critical CRL
extensions are rejected; delta and scoped CRLs are not supported. Revoked certificates
and chains without fresh issuer coverage fail verification. `CRLMaxAge`
defaults to 24 hours for CRLs without `NextUpdate`. The TLS poll and SIGHUP also
refresh CRLs; `CRLRefreshInterval`, when shorter, requests more frequent polls.
A failed refresh retains the previous generation only while it remains valid.
CRL and CA changes reconnect connections; CRL expiry closes them. No size,
upstream-count or chain-depth setting is introduced.

PEM files may contain multiple CRLs; every block is checked. Invalid later
blocks reject the replacement as a whole. Per-issuer CRL number/time
watermarks, bound to the signed DER hash, persist in `CacheDir` as an atomic,
checksummed `0600` file as part of TLS generation publication.
Restarting with an older, still-valid CRL
is rejected. Removing a CRL source, including removing every source, is
rejected while a retained floor's issuer is still in `CAFile`: the reload
keeps the last valid generation, and startup fails closed until a CRL at or
above the floor is loaded. A floor whose issuer is no longer listed in
`CAFile` is retained for a possible CA rollback, and a relay chain that still
uses that issuer, including an intermediate supplied by the relay, still
requires fresh CRL coverage. Because a CRL signer must be in `CAFile`, restore
both that issuer and a covering CRL to recover. Corrupt retained watermarks fail startup; a failed write keeps
the previous generation. Preserve this directory to retain rollback protection,
and use a separate directory for each running instance. Two CRLs with equal
number/time but different contents retain separate floors; dropping either
requires a newer CRL that covers its floor. Cancellation and expiry are checked
after staging the watermark file, before publication and its atomic rename.
A rejected generation leaves both the retained floor and active generation
unchanged; a failed rename also restores the previous generation.

`DownstreamCredentialsFile`, `Services`, `MaxDownstream`, `MaxChainDepth` and
`MaxConnectionsPerDownstream` are removed store properties. They fail startup,
including when explicitly set to an empty value. Downstream credentials and
service subsets are not migrated automatically. Use `ClientCertFile`,
`ClientKeyFile`, `CAFile` and the relay's `downstreamClientCAPath`; a downstream
receives the relay's service set.
