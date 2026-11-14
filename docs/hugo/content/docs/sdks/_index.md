+++
title = "SDKs"
description = "Client libraries for Speedle authorization checks in Java, Python, Go, .NET, Rust and Node.js/TypeScript."
weight = 305
draft = false
toc = true
linksection = true
tocheading = "h2"
tags = ["sdk"]
categories = ["docs"]
bref = "Client libraries for the Speedle decision API in six languages. They work with a self-hosted Speedle ADS."
lastmod = 2026-11-10T14:45:43+08:00
+++

The Speedle SDKs are client libraries for the decision API `POST /authz-check/v1/is-allowed`. They work with a self-hosted Speedle ADS. Each language page shows how to add the package and gives a complete, tested program for a self-hosted ADS. The source and the full reference are in [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk). To run PMS and ADS for these examples, see [Deployment](/docs/deployment/) and the [Docker image](/docs/deployment/docker/).

## Packages

Java, Python, Go, .NET and Rust are version 0.2.0. `@speedle/sdk` is published on npm. Each language page shows the install command.

- [Java](/docs/sdks/java/): Maven Central `io.speedle:speedle-sdk`, published.
- [Python](/docs/sdks/python/): PyPI `speedle`, published.
- [Go](/docs/sdks/go/): Go module `github.com/teramoby/speedle-sdk/go`, published.
- [C#/.NET](/docs/sdks/dotnet/): NuGet `Speedle.Sdk`, published.
- [Rust](/docs/sdks/rust/): crates.io `speedle`, published.
- [Node.js / TypeScript](/docs/sdks/node/): npm `@speedle/sdk`, published.

## Features

| Feature | Status |
| --- | --- |
| `isAllowed`: returns the decision | All six SDKs |
| `check`: returns the decision and the reason, for example that no policy applies | All six SDKs |
| Self-hosted Speedle ADS | All six SDKs |

The method names follow each language's conventions: `is_allowed` and `check` in Python and Rust, `IsAllowed` and `Check` in Go, `IsAllowedAsync` and `CheckAsync` in .NET.

For the other operations, such as granted permissions, diagnosis and policy management, use the REST APIs: the [authorization decision API](/docs/api/decision_api/) and the [policy management API](/docs/api/management_api/).

## Example policy

Every example asks whether user `alice` may `get` and `delete` the resource `/docs/public` in the service `docs`. To get the documented answers from a local Speedle, build it and start PMS on port 6733 and ADS on port 6734 as in the [Quick Start](/quick-start/), then create the service and the policy:

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

The local examples connect to `http://localhost:6734` without credentials. The SDKs accept plain `http://` only for a loopback host; use HTTPS for any other ADS.
