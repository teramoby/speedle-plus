+++
title = "C#/.NET SDK"
description = "添加 Speedle .NET SDK，向本地 Speedle 或 Speedle Cloud 发起授权检查。"
weight = 40
aliases = ["/zh/docs/api/sdk-dotnet/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 dotnet add package 添加 Speedle.Sdk 0.2.0，再向本地 Speedle 或 Speedle Cloud 调用 IsAllowedAsync 与 CheckAsync。"
lastmod = 2026-11-08T11:21:16+08:00
+++

.NET SDK 面向 .NET 8 或更高版本，没有依赖。软件包：NuGet 上的 [`Speedle.Sdk`](https://www.nuget.org/packages/Speedle.Sdk) 0.2.0，命名空间 `Speedle.Sdk`。参考文档：[speedle-sdk/dotnet](https://github.com/teramoby/speedle-sdk/tree/main/dotnet)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

## 添加依赖

新建控制台项目并添加软件包：

```bash
dotnet new console -o SpeedleExample
cd SpeedleExample
dotnet add package Speedle.Sdk --version 0.2.0
```

## 调用本地 Speedle

按 [SDK 概览](/docs/sdks/#示例策略)中的说明启动 Speedle 并创建示例策略。将 `Program.cs` 替换为：

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

运行：

```bash
dotnet run
```

输出：

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

## 调用 Speedle Cloud

> **Speedle Cloud 即将上线。**

程序从 `SPEEDLE_SERVICE` 读取服务名，从 `SPEEDLE_TOKEN` 读取该服务的令牌。`SPEEDLE_ENDPOINT` 可覆盖默认端点 `https://cloud.speedle.io`。将 `Program.cs` 替换为：

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

设置环境变量后运行。`read -rs` 不会把令牌写入 shell 历史，也不会显示在屏幕上：

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
dotnet run
```

服务中已有示例策略时，输出为：

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

令牌是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。

## 错误处理

授权请求返回 Deny 并不代表调用出错：`IsAllowedAsync` 返回 `false`，`CheckAsync` 返回 `Allowed` 为 false 的 `Decision` 及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，SDK 抛出 `SpeedleException`。示例捕获该异常，向标准错误输出 `Speedle error: <消息>`，并返回退出码 1。
