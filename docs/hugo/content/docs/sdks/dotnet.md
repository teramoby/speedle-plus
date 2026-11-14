+++
title = "C#/.NET SDK"
description = "Add the Speedle .NET SDK and check authorization against a local Speedle."
weight = 40
aliases = ["/docs/api/sdk-dotnet/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Add Speedle.Sdk 0.2.0 with dotnet add package, then call IsAllowedAsync and CheckAsync against a local Speedle."
lastmod = 2026-11-07T19:43:15+08:00
+++

The .NET SDK targets .NET 8 or later and has no dependencies. Package: [`Speedle.Sdk`](https://www.nuget.org/packages/Speedle.Sdk) 0.2.0 on NuGet, namespace `Speedle.Sdk`. Reference: [speedle-sdk/dotnet](https://github.com/teramoby/speedle-sdk/tree/main/dotnet).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

In a new console project:

```bash
dotnet new console -o SpeedleExample
cd SpeedleExample
dotnet add package Speedle.Sdk --version 0.2.0
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Replace `Program.cs` with:

```csharp
using Speedle.Sdk;

try
{
    await using var speedle = new SpeedleClient(new SpeedleClientOptions
    {
        Backend = Backend.Speedle,
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


## Error handling

A denied request is not an error: `IsAllowedAsync` returns `false`, and `CheckAsync` returns a `Decision` with `Allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, the SDK throws `SpeedleException`. The examples catch it, print `Speedle error: <message>` to standard error and return exit code 1.
