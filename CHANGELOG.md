# Release notes

## Unreleased

- Source builds now require Go 1.27.4 or newer. CI reads the root module's
  Go version; its executable-guide matrix uses Go 1.27.4. Older published
  releases retain their own toolchain requirements.
- Log rotation uses `gopkg.in/natefinch/lumberjack.v2` v2.2.1. Existing JSON
  `rotationConfig` fields are unchanged. Go applications constructing
  `logging.LogConfig.RotationConfig` must import the v2 package because the
  exported field now uses its `Logger` type. Newly created log files default
  to `0600` instead of `0644`; rotation preserves an existing file's mode.
  Deployments with a separate log-collector account must check that account's
  read access, or provision the log file with appropriate ownership and mode
  before startup. Speedle retains the stricter upstream creation default.
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
  archived benchmark harness. Benchmark engine baselines, recorded results and
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
