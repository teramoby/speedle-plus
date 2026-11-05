+++
title = "SDKs"
description = "适用于 Speedle+ 和 Speedle Cloud 的客户端库"
weight = 350
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
bref = "适用于 Speedle+ 和 Speedle Cloud 的客户端库。"
+++

Speedle SDK 同时支持 Speedle+ 和 Speedle Cloud，六个 SDK 均采用 UPL-1.0 许可证。

## Node.js

**包名：** `@speedle/sdk`

**安装：**

```sh
npm install @speedle/sdk
```

**用法（摘自 `node/v0.1.0` README）：**

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

[注册中心](https://www.npmjs.com/package/@speedle/sdk) | [`node/v0.1.0` 源代码](https://github.com/teramoby/speedle-sdk/tree/node/v0.1.0/node)

## Python

**包名：** `speedle`

**安装：**

```sh
pip install speedle
```

**用法（摘自 `python/v0.1.0` README）：**

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

[注册中心](https://pypi.org/project/speedle/) | [`python/v0.1.0` 源代码](https://github.com/teramoby/speedle-sdk/tree/python/v0.1.0/python)

## Java

**包名：** `io.speedle:speedle-sdk`

**添加 Maven 依赖：**

```xml
<dependency>
  <groupId>io.speedle</groupId>
  <artifactId>speedle-sdk</artifactId>
  <version>0.1.0</version>
</dependency>
```

**用法（摘自 `java/v0.1.0` README）：**

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

[注册中心](https://central.sonatype.com/artifact/io.speedle/speedle-sdk) | [`java/v0.1.0` 源代码](https://github.com/teramoby/speedle-sdk/tree/java/v0.1.0/java)

## .NET

**包名：** `Speedle.Sdk`

**安装：**

```sh
dotnet add package Speedle.Sdk
```

**用法（摘自 `dotnet/v0.1.0` README）：**

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

[注册中心](https://www.nuget.org/packages/Speedle.Sdk) | [`dotnet/v0.1.0` 源代码](https://github.com/teramoby/speedle-sdk/tree/dotnet/v0.1.0/dotnet)

## Rust

**包名：** `speedle`

**安装：**

```sh
cargo add speedle
```

**用法（摘自 `rust/v0.1.0` README）：**

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

[注册中心](https://crates.io/crates/speedle) | [`rust/v0.1.0` 源代码](https://github.com/teramoby/speedle-sdk/tree/rust/v0.1.0/rust)

## Go

**包名：** `github.com/teramoby/speedle-sdk/go`

**安装：**

```sh
go get github.com/teramoby/speedle-sdk/go
```

**用法（摘自 `main` 分支 README）：**

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

[注册中心](https://pkg.go.dev/github.com/teramoby/speedle-sdk/go) | [`main` 分支源代码](https://github.com/teramoby/speedle-sdk/tree/main/go)
