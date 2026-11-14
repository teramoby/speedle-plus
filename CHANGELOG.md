# Release notes

## Unreleased

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
