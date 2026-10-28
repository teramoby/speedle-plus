+++
title = "Versions"
description = "What's New? Speedle versions and changelog! "
weight = 400
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["releases", "versions"]
categories = ["docs"]
bref = "Releases - Speedle follows sematic versioning."
+++

## Current development documentation

These pages describe the current `master` source, not a frozen historical release. The current module requires Go 1.26 or later. The quick-start workflow verifies Go 1.26.10 and 1.27.3; those tested patch versions are not a promise that an old release has the same requirements.

When upgrading, review the chosen commit and dependencies and repeat the integration checks for your store, transport and identity provider. Current standalone gRPC uses the same TLS/mTLS settings as REST and inherits its listening host unless `--grpc-endpoint` is supplied. Its service namespaces are `pms_pb.PolicyManager` and `ads_pb.Evaluator`; use the [current downloadable protos](/docs/api/) and generated clients together.

## Historical releases

The repository publishes [v2.0 (July 2020)](https://github.com/teramoby/speedle-plus/releases/tag/v2.0) and the [baseline tag](https://github.com/teramoby/speedle-plus/releases/tag/baseline). Their assets and source snapshots are historical. See the [release list](https://github.com/teramoby/speedle-plus/releases) for published artifacts; a tag name alone does not imply current dependency or security compatibility.

The old 0.1.0 notes described policy management, authorization runtime, CLI and integration samples. The former “0.2.0 planning” placeholder was not a current release roadmap.

## Reproducible use

Choose a release tag or immutable commit for application dependencies. Record that revision alongside policy fixtures and expected decisions. Follow the [quick start](/quick-start/) for a native build, [embedded mode](/docs/deployment-embeded/) for a library integration, and [security](/docs/security/) for current connection behavior.
