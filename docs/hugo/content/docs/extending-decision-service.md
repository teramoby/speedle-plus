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

Implement `store.StoreBuilder` and return a store implementing the
`pms.PolicyStoreManager` interfaces needed by your application. Register the
builder under a unique name with `store.Register` in your package's `init`
function. Import that package in your executable; the standard binaries link
stores through `cmd/speedle-ads/stores.go` and `cmd/speedle-pms/stores.go`.
`GetStoreParams` maps command-line flags to store properties.

For an embedded service, pass your store to `eval.NewWithStore` and pass the
resulting evaluator to `adsrest.NewRouter` or `adsgrpc.NewGRPCService`. A store
that must receive change events can implement `store.WatchRequirement`.
`(*eval.PolicyEvalImpl).DistributionStore()` remains a backing-store accessor
for external adapters; the Engine does not interpret custom gateway interfaces.
Compose any custom gateway or routing in your own executable.

See the [store interfaces](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)
and [embedded deployment](/docs/deployment/embedded/).
