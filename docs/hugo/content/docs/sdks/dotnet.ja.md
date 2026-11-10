+++
title = "C#/.NET SDK"
description = "Speedle .NET SDK を追加し、ローカルの Speedle または Speedle Cloud に対して認可チェックを実行"
weight = 40
aliases = ["/ja/docs/api/sdk-dotnet/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "dotnet add package で Speedle.Sdk 0.2.0 を追加し、ローカルの Speedle または Speedle Cloud に対して IsAllowedAsync と CheckAsync を呼び出します。"
lastmod = 2026-11-07T19:43:15+08:00
+++

.NET SDK は .NET 8 以降を対象とし、依存関係はありません。パッケージ: NuGet の [`Speedle.Sdk`](https://www.nuget.org/packages/Speedle.Sdk) 0.2.0、名前空間 `Speedle.Sdk`。リファレンス: [speedle-sdk/dotnet](https://github.com/teramoby/speedle-sdk/tree/main/dotnet)。

サンプルでは、[SDK 概要](/ja/docs/sdks/#example-policy) で説明されているポリシーをチェックします。

## 依存関係の追加

新しいコンソールプロジェクトを作成します:

```bash
dotnet new console -o SpeedleExample
cd SpeedleExample
dotnet add package Speedle.Sdk --version 0.2.0
```

## ローカルの Speedle の呼び出し

[SDK 概要](/ja/docs/sdks/#example-policy) の説明に従って Speedle を起動し、サンプルポリシーを作成します。`Program.cs` を以下に置き換えます:

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

実行します:

```bash
dotnet run
```

出力:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

## Speedle Cloud の呼び出し

> **Speedle Cloud はまもなく公開されます。**

プログラムは `SPEEDLE_SERVICE` からサービス名を、`SPEEDLE_TOKEN` からサービスのトークンを読み取ります。`SPEEDLE_ENDPOINT` はデフォルトのエンドポイント `https://cloud.speedle.io` を上書きします。`Program.cs` を以下に置き換えます:

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

環境変数を設定して実行します。`read -rs` を使用すると、トークンがシェルの履歴や画面に表示されません:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
dotnet run
```

サービスにサンプルポリシーが存在する場合の出力:

```text
alice get /docs/public: allowed=True
alice delete /docs/public: allowed=False reason=NoApplicablePolicies
```

トークンはサーバー側のシークレットです。ブラウザ、モバイルアプリ、またはデスクトップクライアントには絶対に配布しないでください。

## エラー処理

拒否されたリクエストはエラーではありません。上記の `delete` 呼び出しのように、`IsAllowedAsync` は `false` を返し、`CheckAsync` は `Allowed` が false で理由を含む `Decision` を返します。デフォルト設定では、呼び出しが決定を返せない場合、SDK は `SpeedleException` をスローします。サンプルではそれをキャッチし、標準エラー出力に `Speedle error: <message>` を出力して終了コード 1 を返します。
