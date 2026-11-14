# Cascade failover and freshness

Design for #385 (builds on #380 and #383).

- Start at the first HTTPS upstream. Connection, TLS, protocol, authentication,
  stale or syncing failures advance through the ordered list; exhausting it
  uses bounded reconnect backoff. After one healthy minute on an alternate,
  retry the preferred upstream. Direct API-key revocation behavior is unchanged.
- Keep the channel candidate separate from the admitted data/authorization
  endpoint. Before admission, validate the complete ack, effective heartbeat
  cadence, ancestor chain, freshness and retained revision/hash floors,
  including disk-cache floors after restart. An unadmitted or rejected candidate
  cannot receive data or forwarded authorization requests. Endpoint selection
  is serialized with store sync passes. Old in-flight forwarding failures are
  bound to their originating selection generation; service-level errors do not
  trigger node failover. Data failures are classified at the HTTP boundary, so
  local cache/compilation/integrity failures never rotate the endpoint. An admitted
  endpoint must stay healthy for a heartbeat interval before outage backoff resets. Retry hints remain associated with their upstream URL,
  including reconnects; obsolete generations cannot overwrite them. Epoch changes
  retain the full-resynchronization contract.
- Relay acks carry `upstreamState` (`live`, `syncing`, `stale`) and
  `upstreamFreshForMs`, the minimum remaining direct-ack and inherited freshness
  budget. Cascade hello/heartbeat frames carry `seq`; acks echo the latest
  received sequence as `ackSeq`. Children conservatively subtract the entire
  monotonic RTT, including holds and queues, without synchronized wall clocks.
  Keep that absolute deadline across validation and callbacks; installation
  waits never renew it, and the topology commit rechecks expiry under its locks. Unsolicited acks age the same reference until another
  frame arrives. Unknown/reordered sequences fail validation. Local stale
  windows can only tighten the budget. Exhausted freshness rejects data with
  `upstream_stale`; a syncing ancestor yields `upstream_syncing`. Channels
  propagate these states without applying withheld mirrors as service removals.
- Every relay ack advertises `chain`: ancestor UUIDs followed by its own UUID.
  A direct root starts with its own UUID. Validate uniqueness and UUID syntax,
  and reject the receiving client's own UUID before installing the upstream
  chain plus self as the downstream server's runtime `Config.Chain`. Reject
  ancestor/self hellos and polls with `chain_loop`; topology updates also close
  existing ancestor sessions and release held polls.
- Heartbeats report distinct direct-child sessions. Any valid client certificate
  gets all relay services. No fan-out, size, count, depth or CRL limit and no
  content signing are added.

Validation: ordered failover/recovery, rollback (including restart), both
transports, delayed acks over three hops, blocked topology callbacks, admission,
three-level evaluator/store synchronization, freshness recovery, loop guards,
child counts, fail-closed missing freshness providers, affected-package race tests and the full Go gate (>=90.0%).
