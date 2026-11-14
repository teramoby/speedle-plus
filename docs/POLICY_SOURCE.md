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
Configure the intended private CA in `CAFile`. The bundle is trusted for both
relay verification and HTTPS CRL hosts; adding a CA expands both trust domains.

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
queries and fragments are rejected. The downstream client starts at the first entry, then tries the remaining
candidates in order after connection, TLS, protocol, authentication, stale or
syncing errors. After a minute on a healthy alternate it retries the first
entry; exhausting the list uses bounded reconnect backoff. Data synchronization
and authorization forwarding use an endpoint only after its complete ack has
passed cadence, ancestry, freshness and retained-head checks. Requests fail
closed while no endpoint is admitted. A stale candidate,
a lower head within the same epoch or an equal-revision conflicting hash cannot
replace retained verified data, including a disk-cache floor after restart.
Service-level forwarding errors do not trigger node failover; old in-flight
failures cannot cancel a replacement selection. Epoch changes require full
resynchronization. `ServeDownstream` enables relay serving: the process
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
their own signal handler; the store does not register process signals. This owned watcher is cancelled
and joined during store shutdown. A direct `Client.ReloadTLS` call blocks until
its reload finishes; `Client.Close` prevents it from publishing another
generation but does not cancel that explicit call. An embedded caller must
join its own explicit reload calls during shutdown.
`adsync.NewClientContext` supports cancellation during initial CRL downloads. A valid
replacement is installed as one generation, then upstream connections,
including WebSockets, held polls and idle HTTP connections, are closed to
reconnect with that generation. A malformed replacement logs a warning and
retains the last valid generation. Certificate expiry closes connections even
when they remain active.

Optional `CRLFiles` and `CRLURLs` contain local full X.509 CRLs and HTTPS CRL
addresses for relay certificate revocation checks. When either property is
present, at least one source is required; omit both properties to disable
optional revocation checking. Startup warns when no source is configured;
retained floors still require fresh coverage for their issuers. For CRL verification,
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
after staging the watermark file, immediately before its atomic rename.
The floor is committed first, followed by in-memory generation publication in
the same locked step. A rejected generation or failed rename leaves both the
retained floor and active generation unchanged.

`DownstreamCredentialsFile`, `Services`, `MaxDownstream`, `MaxChainDepth` and
`MaxConnectionsPerDownstream` are removed store properties. They fail startup,
including when explicitly set to an empty value. Downstream credentials and
service subsets are not migrated automatically. Use `ClientCertFile`,
`ClientKeyFile`, `CAFile` and the relay's `downstreamClientCAPath`; a downstream
receives the relay's service set.


Relay acks carry `chain`, `upstreamState` and `upstreamFreshForMs`. The chain
contains the ancestor UUIDs followed by the relay UUID, and populates the next
relay's runtime loop guard. A hello or poll from an ancestor or the relay itself
is rejected with `chain_loop`; topology updates also close an existing ancestor
session. There is no chain-depth or channel-frame-size limit.

Freshness is an end-to-end budget. A relay sends the minimum of its own
confirming-ack window and its inherited remaining budget. Cascade frames carry
`seq` and acks echo it as `ackSeq`; children subtract the full monotonic RTT
(including a held poll and queues) before installing a local deadline, so delays
cannot extend ancestor freshness. The absolute deadline survives validation
and topology-callback waits; installation never restarts the budget. This
conservative accounting can shorten the
usable window on slow links or long-held polls. Unsolicited acks keep aging their
last echoed frame; a later frame supplies a newer reference. No wall-clock
synchronization is required. A syncing ancestor answers data requests with 503
`upstream_syncing`; an exhausted budget answers with 503 `upstream_stale`.
Channels propagate those states without treating temporarily withheld services
as authoritative removals. Direct-child counts remain part of heartbeats.

An embedded `adsproxy.Server` without `Config.Upstream` fails closed: its acks
report `syncing` with no freshness budget and data requests are refused. The
policy-source store always supplies the state machine callback. Embedded
authoritative sources must supply their own truthful freshness provider.

The configured store builder always enables its owned notification runtime.
Embedded callers using `policysource.Open` must set `StartRuntime` for an ordered
upstream list; multi-upstream manual mode is rejected instead of silently
using only its first address. Low-level `adsync.Client` endpoint selection is
driven by its `Channel.Run` owner.

Relay freshness is instance-wide: any member service awaiting a verified,
evaluator-installed copy makes the downstream channel report `syncing`. This
keeps a temporarily withheld service from being mistaken for an authoritative
removal. Other healthy services can still evaluate locally on that relay.
Only endpoint-level data-request failures trigger upstream rotation; local
cache, policy-compilation, integrity and service-not-found errors stay isolated.

Endpoint selection serializes with an in-flight synchronization pass so one
pass cannot combine endpoints. Request stall timeouts remain in force; a
continuously progressing large transfer can delay selection until that pass
finishes. No total download-duration or payload-size limit is introduced.
