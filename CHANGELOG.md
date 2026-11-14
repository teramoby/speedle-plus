# Release notes

## Unreleased

- Removed the experimental policy-distribution feature introduced in v1.7.x.
  The Engine continues to provide policy decisions, local policy stores,
  management and decision APIs, spctl and SDK integration. Configurations that
  select the removed store type fail at startup with a v1.8.0 migration error.
  Remove obsolete distribution command-line flags before using an old
  configuration; removed flags are rejected as unknown options during parsing.
  For a v1.7.x distribution deployment, migrate every dependent client together:
  the policy-source protocol and dedicated `downstream-*` mTLS listener are no
  longer served, so older clients cannot keep synchronizing with this binary.
  Replace the `policy-source` store configuration with a supported local store
  and remove its source URL/API key, `ClientCertFile`, `ClientKeyFile`, `CAFile`,
  `CRL*` and `CacheDir` settings. Retire the dedicated listener, its firewall or
  load-balancer route, and its client CA/certificates and keys when no remaining
  deployment uses them; revoke credentials that are no longer needed. After
  stopping the old distribution process and completing the migration, remove
  its unused `CacheDir`, including the persistent instance identity and retained
  CRL watermarks. Do not delete credentials or state still used by another
  service. The Engine does not clean up these files automatically.
