# Release notes

## Unreleased

- The policy-source store accepts a downstream mTLS configuration with a
  required persistent `CacheDir`. Its client verifies relay certificates and
  reloads TLS files on a 30-second poll or SIGHUP, reconnecting upstream
  connections when material changes. See [configuration](docs/POLICY_SOURCE.md).
- Removed the unreleased `DownstreamCredentialsFile` credential prototype and
  `Services` per-downstream subsets. Setting either fails startup with an error
  naming `ClientCertFile`, `ClientKeyFile`, `CAFile` and the relay's
  `downstreamClientCAPath`. Connection/depth limit properties `MaxDownstream`,
  `MaxChainDepth` and `MaxConnectionsPerDownstream` are also rejected. Engines
  without the policy-source store keep their existing behavior.
