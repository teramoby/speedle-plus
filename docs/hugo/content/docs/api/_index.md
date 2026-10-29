+++
title = "API"
description = "Open API Specification Docs"
weight = 300
draft = false
toc = false
tocheading = "h4"
tocsidebar = false
tags = ["v0.1.0", "api", "guide", "core"]
categories = ["docs"]
bref = "Open API Specification Docs. Use the API to model your simple and complex policies in a declarative way; get decisions, discover policies dynamically and debug policy evaluation."
+++

## REST APIs

These are reference contracts for **your configured PMS/ADS or asserter endpoint**. `speedle.io` serves documentation and the browser Playground; it is not a hosted PMS/ADS service. Supply JSON with `Content-Type: application/json`, and configure [TLS/mTLS and caller access controls](/docs/security/).

- [Policy management API](/docs/api/management_api/)
- [Authorization decision API](/docs/api/decision_api/)
- [Token assertion webhook](/docs/api/asserter_api/)

Policy and role-policy IDs are strings. PMS responses are flat objects; authorization permissions use `permissions` (plural). REST decision requests use a typed attribute array, whereas diagnostic response attributes and webhook attributes are objects.

## gRPC APIs

- [PMS proto](/protobuf/pms.proto): `pb.PolicyManager`
- [ADS proto](/protobuf/ads.proto): `pb.Evaluator`

Use the current proto and generated client together. PMS uses `pb.PolicyManager`, and ADS uses `pb.Evaluator`. The two historical contracts reuse message names and must be generated/loaded separately; do not combine them in one descriptor set. gRPC is off unless the server is started with `--grpc-endpoint`, and uses the same TLS settings as REST.

The legacy gRPC `ContextRequest.attributes` map contains strings. It is not the REST typed attribute-array format and does not offer its numeric/bool/date typing. Use REST or embedded Go for conditions requiring those types unless your gRPC adapter explicitly implements the required conversion.

## Embedded Go APIs

- [Policy management interfaces](https://github.com/teramoby/speedle-plus/tree/master/api/pms)
- [Authorization decision interfaces](https://github.com/teramoby/speedle-plus/tree/master/api/ads)

See [embedded mode](/docs/deployment-embeded/) for a complete application and resource lifecycle example.
