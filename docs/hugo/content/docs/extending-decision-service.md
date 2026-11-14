+++
title = "Extending the decision service"
description = "Use a custom policy store or service adapter."
weight = 310
lastmod = 2026-11-14T18:30:00+08:00
draft = false
toc = false
categories = ["docs"]
bref = "Use a custom policy store or service adapter."
+++

Implement `store.StoreBuilder`, register a unique name with `store.Register`, and
import the package from your executable. `GetStoreParams` maps extra flags to
store properties. Parse your own flags before starting the server; the Engine
library does not parse flags or install signal handlers.

Use `ads.Run(ctx, params, conf, options...)` from
`github.com/teramoby/speedle-plus/pkg/ads` to reuse the standard ADS startup.
`params` is `*flags.Parameters`, `conf` is `*cfg.Config`, and cancelling `ctx`
stops both servers. `ads.WithGate(g)` overrides gate discovery on both transports;
`ads.WithGate(nil)` disables it. `ads.WithListeners(rest, rpc)` accepts pre-bound
`net.Listener` values (nil uses the configured endpoint). Run owns and closes
those listeners, even on startup failure. REST retains the configured TLS;
gRPC retains its server credentials. `ads.WithHTTPServer(func(*http.Server))`
can wrap the handler or add routes before serving. `ads.WithGRPCOptions(...)`
adds server options; use `grpc.ChainUnaryInterceptor(...)` for extra interceptors
to preserve the standard panic recovery interceptor.

The transport-independent `pkg/svcs/gate.Gate` has two methods:
`Route(serviceName string) gate.Route` chooses `gate.Local`, `gate.Fallback`, or
`gate.Unavailable`, and `Forward(context.Context, gate.Operation,
*ads.RequestContext) (*gate.Response, error)` supplies a typed fallback result.
Local continues normal evaluation. Fallback can allow or deny, return roles, or
return permissions; failures never switch to local evaluation. Diagnose and
Discover fail closed when fallback is selected. A `*gate.UnavailableError` carries
a public reason code; other failures and nil responses map to
`fallback_unavailable`. REST returns 503 with `authorization_unavailable`, a
reason, `Cache-Control: no-store`, and `Retry-After: 30`; gRPC returns Unavailable.
A response's `ErrorMessage` is an evaluation failure: REST IsAllowed returns its
normal 200 error body, other REST operations return 500, and gRPC returns Internal.
Successful fallback responses carry `X-Speedle-Decision-Source: upstream` (gRPC
metadata uses the lowercase key). An optional `gate.Auditor.Audit(ctx, event)`
receives one event per non-local route without subjects, tokens or request bodies;
the services also write their standard audit logs. Gate implementations must be
safe for concurrent requests and own their fallback authentication and resources.

A store can implement `gate.GateProvider.RequestGate() gate.Gate`; REST and gRPC
discover it through the evaluator's backing-store accessor. Embedded callers can
use `eval.NewWithStore`, then `adsrest.NewRouter(evaluator,
adsrest.WithGate(g))` or `adsgrpc.NewGRPCService(evaluator,
adsgrpc.WithGate(g))`. Omitting the option discovers the store gate; an explicit
nil disables it. Without a gate, existing local behavior is unchanged.

Keep store lifetimes explicit: `store.WatchRequirement.RequireWatch()` forces
subscription even if config disables watching. Optional
`ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error)` supplies a snapshot
and an acknowledgement callback, called only after successful installation.
`(*eval.PolicyEvalImpl).DistributionStore()` exposes the backing store for
adapters. Closing the evaluator stops its watch and closes a closable store;
`ads.Run` performs that cleanup. An explicitly supplied gate remains caller-owned.
See the [test-only external store and gate](https://github.com/teramoby/speedle-plus/blob/master/pkg/ads/extensions_test.go)
for REST/gRPC examples, the [store interfaces](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)
and [embedded deployment](/docs/deployment/embedded/).
