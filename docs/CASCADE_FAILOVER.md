# Cascade failover and freshness

Design for #385 (builds on #380 and #383).

- A cascade channel starts at the first configured HTTPS upstream. Connection,
  TLS, protocol, stale and syncing failures advance through the ordered list.
  Exhausting the list uses the existing bounded reconnect backoff. After one
  minute on a healthy alternate, retry the preferred upstream. Authentication
  failures are endpoint failures in cascade mode; direct API-key mode retains
  its existing terminal-revocation behavior. Data and authorization requests
  use the channel's selected endpoint. Selection is serialized with store sync
  passes so a pass cannot combine responses from different upstreams.
- Validate a candidate ack before changing membership, freshness or ancestry.
  Within an epoch, a head below a retained copy or known head is rejected, as
  is an equal-revision conflicting hash. Existing verified snapshot/change
  validation remains the data-integrity and rollback guard. Epoch changes keep
  the existing full-resynchronization contract.
- Relay acks carry `upstreamState` (`live`, `syncing`, `stale`) and
  `upstreamFreshForMs`, the remaining upstream freshness budget. A child keeps
  an absolute local deadline, capped by its own stale window; another ack can
  only renew the budget advertised by its parent. Relay output takes the
  minimum of its direct-ack and inherited deadlines. Syncing ancestors prevent
  data serving with `upstream_syncing`; exhausted freshness prevents serving
  with `upstream_stale`. Channels can report these states without advertising
  a temporarily withheld mirror as authoritative service removal.
- Every relay ack advertises `chain`, its ordered ancestor UUIDs followed by
  its own UUID. A direct root starts with its own UUID. A cascade client
  validates UUIDs, uniqueness and absence of its own UUID before accepting an
  ack, then updates its downstream server's runtime chain from that ack plus
  itself. A server rejects ancestor/self hellos and polls with `chain_loop`;
  ancestry updates also disconnect existing sessions that would close a loop.
  There are no chain-depth, upstream-count, fan-out, size or CRL limits.
- Heartbeats retain the existing distinct direct-child session count. Any valid
  client certificate receives all relay services; no content signing is added.

Validation: failover and rollback unit tests, both channel transports, runtime
chain updates, freshness deadline propagation and a three-level in-process
chain; affected-package race tests and the full Go gate (coverage >=90.0%).
