+++
title = "Node.js/TypeScript SDK"
description = "Speedle Node.js/TypeScript SDK を追加し、ローカルの Speedle または Speedle Cloud に対して認可チェックを実行"
weight = 60
aliases = ["/ja/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "npm で @speedle/sdk をインストールし、ローカルの Speedle または Speedle Cloud に対して isAllowed と check を呼び出します。"
lastmod = 2026-11-10T09:48:45+08:00
+++

Node.js SDK は Node.js 22 以降が必要で、TypeScript の型定義を含む ESM および CommonJS ビルドを提供し、ランタイム依存関係はありません。パッケージ: npm の [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk)。リファレンス: [speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node)。

サンプルでは、[SDK 概要](/ja/docs/sdks/#example-policy) で説明されているポリシーをチェックします。

## 依存関係の追加

新しい ES モジュールパッケージを作成します:

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
```

## ローカルの Speedle の呼び出し

[SDK 概要](/ja/docs/sdks/#example-policy) の説明に従って Speedle を起動し、サンプルポリシーを作成します。このプログラムを `main.ts` として保存します:

```typescript
import { Auth, Reason, SpeedleClient, SpeedleError } from "@speedle/sdk";

try {
  const speedle = new SpeedleClient({
    backend: "speedle",
    endpoint: "http://localhost:6734",
    auth: Auth.none(),
    serviceName: "docs",
  });
  try {
    const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
    console.log(`alice get /docs/public: allowed=${allowed}`);

    const d = await speedle.check("alice", "delete", "/docs/public");
    console.log(`alice delete /docs/public: allowed=${d.allowed} reason=${Reason[d.reason] ?? d.reason}`);
  } finally {
    await speedle.close();
  }
} catch (e) {
  if (!(e instanceof SpeedleError)) throw e;
  console.error(`Speedle error: ${e.message}`);
  process.exitCode = 1;
}
```

実行します。Node.js 22.18 以降では TypeScript ファイルを直接実行できます:

```bash
node main.ts
```

出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

## Speedle Cloud の呼び出し

> **Speedle Cloud はまもなく公開されます。**

プログラムは `SPEEDLE_SERVICE` からサービス名を、`SPEEDLE_TOKEN` からサービスのトークンを読み取ります。`SPEEDLE_ENDPOINT` はデフォルトのエンドポイント `https://cloud.speedle.io` を上書きします。`main.ts` を以下に置き換えます:

```typescript
import { Auth, Reason, SpeedleClient, SpeedleError } from "@speedle/sdk";

try {
  const speedle = new SpeedleClient({
    backend: "speedle-cloud",
    endpoint: process.env.SPEEDLE_ENDPOINT ?? "https://cloud.speedle.io",
    auth: Auth.bearer(process.env.SPEEDLE_TOKEN ?? ""),
    serviceName: process.env.SPEEDLE_SERVICE ?? "",
  });
  try {
    const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
    console.log(`alice get /docs/public: allowed=${allowed}`);

    const d = await speedle.check("alice", "delete", "/docs/public");
    console.log(`alice delete /docs/public: allowed=${d.allowed} reason=${Reason[d.reason] ?? d.reason}`);
  } finally {
    await speedle.close();
  }
} catch (e) {
  if (!(e instanceof SpeedleError)) throw e;
  console.error(`Speedle error: ${e.message}`);
  process.exitCode = 1;
}
```

環境変数を設定して実行します。`read -rs` を使用すると、トークンがシェルの履歴や画面に表示されません:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
node main.ts
```

サービスにサンプルポリシーが存在する場合の出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

トークンはサーバー側のシークレットです。ブラウザ、モバイルアプリ、またはデスクトップクライアントには絶対に配布しないでください。

## Speedle MCP

[`@speedle/mcp`](https://www.npmjs.com/package/@speedle/mcp) は npm で公開されています。Speedle MCP（ローンチプレビュー）は、`check` および `manage` モードを備えた 1 つのパッケージです。npm でインストールします:

```bash
npm install @speedle/mcp@0.2.1
```

または `speedle-mcp` バイナリを直接実行します:

```bash
npx @speedle/mcp@0.2.1 check
```

モードを指定せずに `speedle-mcp` を実行すると、使用方法を出力し、ゼロ以外のステータスで終了します。リファレンス: [speedle-mcp](https://github.com/teramoby/speedle-mcp)。

## エラー処理

拒否されたリクエストはエラーではありません。上記の `delete` 呼び出しのように、`isAllowed` は `false` に解決され、`check` は `allowed` が false で理由を含む決定に解決されます。デフォルト設定では、呼び出しが決定を返せない場合、Promise は `SpeedleError` で拒否されます。サンプルではそれをキャッチし、標準エラー出力に `Speedle error: <message>` を出力して `process.exitCode = 1` を設定します。その他のエラーは再スローされます。
