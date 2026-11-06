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

## SDKs

Client libraries for the decision API `POST /authz-check/v1/is-allowed` work with a self-hosted Speedle ADS and with Speedle Cloud. Each page shows how to add the package and gives a complete, tested program for both.

| Language | Package | Version |
| --- | --- | --- |
| [Java](/docs/api/sdk-java/) | Maven Central `io.speedle:speedle-sdk` | 0.1.0 |
| [Python](/docs/api/sdk-python/) | PyPI `speedle` | 0.1.0 |
| [Go](/docs/api/sdk-go/) | `github.com/teramoby/speedle-sdk/go` | 0.1.0 |
| [C#/.NET](/docs/api/sdk-dotnet/) | NuGet `Speedle.Sdk` | 0.1.0 |
| [Rust](/docs/api/sdk-rust/) | crates.io `speedle` | 0.1.0 |
| [Node.js/TypeScript](/docs/api/sdk-node/) | npm `@speedle/sdk` | 0.1.0, available when published |

The SDKs call `is-allowed`: `isAllowed` returns the decision, and `check` also returns the reason. For the other decision operations, such as granted permissions and diagnosis, use the [REST API](/docs/api/decision_api/). The source and the full reference are in [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk).

Every example asks whether user `alice` may `get` and `delete` the resource `/docs/public` in the service `docs`. To get the documented answers from a local Speedle, build it and start PMS on port 6733 and ADS on port 6734 as in the [Quick Start](/quick-start/), then create the service and the policy:

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

The local examples connect to `http://localhost:6734` without credentials. The SDKs accept plain `http://` only for a loopback host; use HTTPS for any other ADS.

**Speedle Cloud is coming soon.** The Speedle Cloud examples connect to `https://cloud.speedle.io` and read the service name and its service-scoped token from the environment variables `SPEEDLE_SERVICE` and `SPEEDLE_TOKEN`. Create the same policy in that service to get the same answers. The token is a server-side secret: never ship it to browsers, mobile apps or desktop clients.

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
