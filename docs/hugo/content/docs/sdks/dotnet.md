+++
title = "C#/.NET SDK"
description = "Add the Speedle .NET SDK and check authorization against a local Speedle or Speedle Cloud."
weight = 40
aliases = ["/docs/api/sdk-dotnet/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Add Speedle.Sdk 0.1.0 with dotnet add package, then call IsAllowedAsync and CheckAsync against a local Speedle or Speedle Cloud."
+++

The .NET SDK targets .NET 8 or later and has no dependencies. Package: [`Speedle.Sdk`](https://www.nuget.org/packages/Speedle.Sdk) 0.1.0 on NuGet, namespace `Speedle.Sdk`. Reference: [speedle-sdk/dotnet](https://github.com/teramoby/speedle-sdk/tree/main/dotnet).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

In a new console project:

```bash
dotnet new console -o SpeedleExample
cd SpeedleExample
dotnet add package Speedle.Sdk --version 0.1.0
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Replace `Program.cs` with:

```csharp
using Speedle.Sdk;

try
{
    await using var speedle = new SpeedleClient(new SpeedleClientOptions
    {
        Backend = Backend.SpeedlePlus,
        Endpoint = new Uri("http://localhost:6734"),
        Auth = Auth.None(),
        ServiceName = "docs",
    });

    bool allowed = await speedle.IsAllowedAsync("alice", "get", "/docs/public");
    Console.WriteLine($"alice get /docs/public: allowed={allowed}");

    Decision d = await speedle.CheckAsync("alice", "delete", "/docs/public");
    Console.WriteLine($"alice delete /docs/public: allowed={d.Allowed} reason={d.Reason}");
}
catch (SpeedleException e)
{
    Console.Error.WriteLine($"Speedle error: {e.Message}");
    return 1;
}
return 0;
```

Run it:

```bash
dotnet run
```

Output:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

## Call Speedle Cloud

> **Speedle Cloud is coming soon.**

The program reads the service name from `SPEEDLE_SERVICE` and the service's token from `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` overrides the default endpoint `https://cloud.speedle.io`. Replace `Program.cs` with:

```csharp
using Speedle.Sdk;

try
{
    await using var speedle = new SpeedleClient(new SpeedleClientOptions
    {
        Backend = Backend.SpeedleCloud,
        Endpoint = new Uri(Environment.GetEnvironmentVariable("SPEEDLE_ENDPOINT") ?? "https://cloud.speedle.io"),
        Auth = Auth.Bearer(Environment.GetEnvironmentVariable("SPEEDLE_TOKEN") ?? ""),
        ServiceName = Environment.GetEnvironmentVariable("SPEEDLE_SERVICE") ?? "",
    });

    bool allowed = await speedle.IsAllowedAsync("alice", "get", "/docs/public");
    Console.WriteLine($"alice get /docs/public: allowed={allowed}");

    Decision d = await speedle.CheckAsync("alice", "delete", "/docs/public");
    Console.WriteLine($"alice delete /docs/public: allowed={d.Allowed} reason={d.Reason}");
}
catch (SpeedleException e)
{
    Console.Error.WriteLine($"Speedle error: {e.Message}");
    return 1;
}
return 0;
```

Set the environment and run it. `read -rs` keeps the token out of your shell history and screen:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
dotnet run
```

Output, when the service has the example policy:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

The token is a server-side secret. Never ship it to browsers, mobile apps or desktop clients.

## Error handling

A denied request is not an error: `IsAllowedAsync` returns `false`, and `CheckAsync` returns a `Decision` with `Allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, the SDK throws `SpeedleException`. The examples catch it, print `Speedle error: <message>` to standard error and return exit code 1.
