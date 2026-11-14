# Release notes

## Unreleased

- Removed the experimental policy-distribution feature introduced in v1.7.x.
  The Engine continues to provide policy decisions, local policy stores,
  management and decision APIs, spctl and SDK integration. The shipped
  `speedle-ads` and `speedle-pms` binaries reject the removed `policy-source`
  store at startup with a v1.8.0 migration error. A host that registers its own
  builder with `store.Register("policy-source", ...)` can still select it; see
  [Extending the decision service](docs/hugo/content/docs/extending-decision-service.md).
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

  **Removed Go API:** v1.8.0 is intentionally source-incompatible for consumers
  of the experimental `pkg/adsync`, `pkg/adsproxy`, `pkg/svcs/adsgate`,
  `pkg/store/policysource` and `testutil/adsyncfake` packages, `cfg.Secret`,
  `cfg.RedactedSecret` and `(*flags.Parameters).DownstreamListenerConfig`.
  Pin v1.7.x until those imports and calls are migrated; no distribution stubs
  remain. These distribution APIs have no built-in replacement. An external
  implementation can use `pkg/ads.Run` for server startup, `pkg/svcs/gate` and
  the REST/gRPC `WithGate` options for request routing, and the retained
  `store.Register`/`eval.NewWithStore` hooks for its store. See
  [Extending the decision service](docs/hugo/content/docs/extending-decision-service.md).
  `(*flags.Parameters).GRPCServerOptions` remains available.
  The retained `adsrest.NewRESTService`, `adsrest.NewRESTServiceWithEvaluator`,
  `adsrest.NewRouter` and `adsgrpc.NewGRPCService` constructors now accept
  variadic `...Option` arguments. Existing direct calls still compile. Code
  assigning a constructor to a function value with its old signature must wrap
  the call in a function with that signature and omit the optional arguments.

- Removed the Cloud getting-started guides, API and Swagger reference, SDK
  examples and `cloud-public.yaml` specification from the Engine site in all
  five languages. The old documentation and `/openapi/cloud-public.yaml` URLs
  are not redirected. This repository no longer publishes replacement Cloud
  documentation; the shared `cloud.speedle.io` product links remain.

- Source builds now require Go 1.27.4 or newer. CI reads the root module's
  Go version; its executable-guide matrix uses Go 1.27.4. Older published
  releases retain their own toolchain requirements.
- Log rotation retains the existing lumberjack dependency and exported
  `logging.LogConfig.RotationConfig` type for source compatibility. The v2
  migration is deferred to the logging redesign in #389.
- Updated etcd client/server modules to v3.7.3 and their required indirect
  dependencies. Upstream client creation no longer honors the deprecated
  `grpc.WithBlock` option. Speedle does not use that option; ADS still loads
  its initial policy snapshot and establishes any configured watch before
  opening listeners. Embedded etcd no longer supports legacy v2 discovery or v2
  requests; deployments must follow etcd's 3.6-to-3.7 upgrade guidance. This
  dependency update does not upgrade an externally managed etcd server.
  logrus v1.9.4 and zap v1.27.1 are required module selections from etcd;
  the logging implementation is unchanged.
- Updated Gorilla mux/handlers to v1.8.1/v1.5.2, fsnotify to v1.10.1 and
  pflag to v1.1.0. Gorilla uses request contexts instead of gorilla/context;
  file watching includes upstream rename/watch fixes. pflag corrects typed
  flag validation and error/help text, so scripts should rely on exit status
  rather than matching complete flag-error messages.
- Kubernetes API, machinery and client libraries use v0.37.2. The webhook
  sample's resource paths and label/owner metadata decoding have a loopback
  API regression test; this does not certify a live Kubernetes cluster.
- Updated gRPC to v1.85.0 and protobuf to v1.36.12. gRPC rejects malformed
  HTTP/2 requests more strictly, reports truncated messages as unexpected EOF,
  and fixes forceful shutdown while graceful shutdown is waiting. The
  OpenTelemetry SDK/trace modules use v1.47.0 in both the root module and the
  archived benchmark harness; its OTLP exporters retain v1.46.0. This is an
  SDK/trace update, not a coordinated upgrade of every telemetry module.
  Benchmark engine baselines, recorded results and
  executed-source archives are unchanged; dependency maintenance is not a new
  benchmark measurement.
