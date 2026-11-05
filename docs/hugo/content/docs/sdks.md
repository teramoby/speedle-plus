+++
title = "SDKs"
description = "Client libraries for Speedle+ and Speedle Cloud"
weight = 350
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
bref = "Client libraries for Speedle+ and Speedle Cloud."
+++

The Speedle SDKs work with Speedle+ and Speedle Cloud. All six SDKs are licensed under UPL-1.0.

## Node.js

**Package:** `@speedle/sdk`

**Install:**

```sh
npm install @speedle/sdk
```

**Usage (from the `node/v0.1.0` README):**

```ts
import { readFileSync } from "node:fs";
import { Auth, SpeedleClient } from "@speedle/sdk";

const speedle = new SpeedleClient({
  backend: "speedle-plus",
  endpoint: "https://ads.internal:6734",
  auth: Auth.none(), // or Auth.mutualTls({ cert, key }) with --force-client-cert
  caCert: readFileSync("ca.pem"), // extra trust anchor for a private CA
  serviceName: "books",
});

const decision = await speedle.check("alice", "get", "/books/123", { team: "eng", clearance: 4 });
// { allowed: false, reason: 3 }
```

[Registry](https://www.npmjs.com/package/@speedle/sdk) | [Source at `node/v0.1.0`](https://github.com/teramoby/speedle-sdk/tree/node/v0.1.0/node)

## Python

**Package:** `speedle`

**Install:**

```sh
pip install speedle
```

**Usage (from the `python/v0.1.0` README):**

```python
from speedle import AsyncClient, Auth

async with AsyncClient(
    backend="speedle-plus",
    endpoint="https://ads.internal:6734",
    auth=Auth.none(),  # or Auth.mutual_tls("client.pem", "client.key") with --force-client-cert
    ca_file="ca.pem",  # extra trust anchor for a private CA
    service_name="books",
) as speedle:
    decision = await speedle.check("alice", "get", "/books/123", {"team": "eng", "clearance": 4})
    # Decision(allowed=False, reason=<Reason.NO_APPLICABLE_POLICIES: 3>, ...)
```

[Registry](https://pypi.org/project/speedle/) | [Source at `python/v0.1.0`](https://github.com/teramoby/speedle-sdk/tree/python/v0.1.0/python)

## Java

**Package:** `io.speedle:speedle-sdk`

**Add the Maven dependency:**

```xml
<dependency>
  <groupId>io.speedle</groupId>
  <artifactId>speedle-sdk</artifactId>
  <version>0.1.0</version>
</dependency>
```

**Usage (from the `java/v0.1.0` README):**

```java
try (SpeedleClient speedle = SpeedleClient.builder()
    .backend(Backend.SPEEDLE_PLUS)
    .endpoint(URI.create("https://ads.internal.example:6734"))
    .auth(Auth.none())                  // or Auth.mutualTls(keyStore, password)
    .serviceName("books")
    .trustStore(myCaTrustStore)         // extra CA for a self-hosted server, optional
    .build()) {
  Decision d = speedle.check(Subject.of(Principal.user("alice"), Principal.group("eng")),
      "get", "/books/123", List.of());
  System.out.println(d.allowed() + " " + d.reason());
}
```

[Registry](https://central.sonatype.com/artifact/io.speedle/speedle-sdk) | [Source at `java/v0.1.0`](https://github.com/teramoby/speedle-sdk/tree/java/v0.1.0/java)

## .NET

**Package:** `Speedle.Sdk`

**Install:**

```sh
dotnet add package Speedle.Sdk
```

**Usage (from the `dotnet/v0.1.0` README):**

```csharp
await using var speedle = new SpeedleClient(new SpeedleClientOptions
{
    Backend = Backend.SpeedlePlus,
    Endpoint = new Uri("https://ads.internal.example:6734"),
    Auth = Auth.None(),                 // or Auth.MutualTls(certificateWithKey)
    ServiceName = "books",
    ExtraTrustedCertificates = new X509Certificate2Collection(myCa), // optional
});

Decision d = await speedle.CheckAsync(
    Subject.Of(Principal.User("alice"), Principal.Group("eng")), "get", "/books/123");
Console.WriteLine($"{d.Allowed} {d.Reason}");
```

[Registry](https://www.nuget.org/packages/Speedle.Sdk) | [Source at `dotnet/v0.1.0`](https://github.com/teramoby/speedle-sdk/tree/dotnet/v0.1.0/dotnet)

## Rust

**Package:** `speedle`

**Install:**

```sh
cargo add speedle
```

**Usage (from the `rust/v0.1.0` README):**

```rust
use speedle::{Auth, Backend, Client, Principal, Subject};

let ca_pem = std::fs::read("ads-ca.pem")?; // your server's CA, if not in the platform store
let client = Client::builder(Backend::SpeedlePlus, "https://ads.internal:6734", Auth::none(), "books")
    .add_root_certificate_pem(&ca_pem)
    .build()?;
let subject = Subject::of([Principal::new(Principal::USER, "bob"), Principal::new(Principal::GROUP, "readers")]);
let decision = client.check(&subject, "download", "/books/HarryPotter", &[]).await?;
println!("{} {:?} {:?}", decision.allowed, decision.reason, decision.error_message);
```

[Registry](https://crates.io/crates/speedle) | [Source at `rust/v0.1.0`](https://github.com/teramoby/speedle-sdk/tree/rust/v0.1.0/rust)

## Go

**Package:** `github.com/teramoby/speedle-sdk/go`

**Install:**

```sh
go get github.com/teramoby/speedle-sdk/go
```

**Usage (from the README on `main`):**

```go
client, err := speedle.NewClient(speedle.Config{
	Backend:     speedle.BackendSpeedlePlus,
	Endpoint:    "https://ads.internal:6734",
	Auth:        speedle.NoAuth(), // or speedle.MutualTLS(cert)
	ServiceName: "books",
	RootCAs:     pool, // your server's CA; nil means the system roots
})

d, err := client.Check(ctx, speedle.Subject{
	{Type: speedle.PrincipalUser, Name: "bob"},
	{Type: speedle.PrincipalGroup, Name: "readers"},
}, "download", "/books/HarryPotter", nil)
// d.Allowed, d.Reason, d.ErrorMessage
```

[Registry](https://pkg.go.dev/github.com/teramoby/speedle-sdk/go) | [Source on `main`](https://github.com/teramoby/speedle-sdk/tree/main/go)
