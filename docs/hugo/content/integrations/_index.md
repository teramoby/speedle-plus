+++
title = "Integrations"
noindex = false
categories = ["docs"]
description = "Connect Speedle with your applications and infrastructure."
aliases = ["/integrations/istio/", "/integrations/kubernetes/", "/integrations/docker/"]
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle makes authorization decisions. A policy enforcement point (PEP) in your application, gateway or platform calls Speedle for each decision and enforces the result. The interfaces below are supported today.

## REST and gRPC APIs

Manage services, policies and roles through the Policy Management Service (PMS), and request runtime decisions from the Authorization Decision Service (ADS). Both expose REST APIs; start either server with `--grpc-endpoint` to enable its gRPC API.

- [Policy management API](/docs/api/management_api/)
- [Authorization decision API](/docs/api/decision_api/)
- [Token assertion webhook](/docs/api/asserter_api/)
- [API overview, including the PMS and ADS protos](/docs/api/)

## Embedded Go

Run the evaluator inside a Go application, without a separate PMS or ADS process. The [embedded mode guide](/docs/deployment/embedded/) builds a complete example with Go modules.

## SDKs

Client SDKs are available for Java, Go, Python, Node.js/TypeScript, Rust and C#/.NET. See [SDKs](/docs/sdks/).

## Policy stores

Use the built-in file, etcd, MySQL or MongoDB store, or implement the store interfaces for another backend. See [Pluggable storage](/docs/store/).

## Previous guides

The previous Istio, Kubernetes and Docker sample guides are no longer published. Use the interfaces above for new integrations.
