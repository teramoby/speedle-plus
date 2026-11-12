+++
title = "Go SDK"
description = "Speedle Go SDK を追加し、ローカルの Speedle または Speedle Cloud に対して認可チェックを実行"
weight = 30
aliases = ["/ja/docs/api/sdk-go/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "go get で github.com/teramoby/speedle-sdk/go v0.2.0 を追加し、ローカルの Speedle または Speedle Cloud に対して IsAllowed と Check を呼び出します。"
lastmod = 2026-11-07T19:43:15+08:00
+++

Go SDK は最新の 2 つの Go リリースをサポートし、標準ライブラリのみを使用します。モジュール: [`github.com/teramoby/speedle-sdk/go`](https://pkg.go.dev/github.com/teramoby/speedle-sdk/go) v0.2.0、パッケージ名 `speedle`。リファレンス: [speedle-sdk/go](https://github.com/teramoby/speedle-sdk/tree/main/go)。

サンプルでは、[SDK 概要](/ja/docs/sdks/#example-policy) で説明されているポリシーをチェックします。

## 依存関係の追加

新しいモジュールで以下を実行します:

```bash
go mod init example.com/speedle-example
go get github.com/teramoby/speedle-sdk/go@v0.2.0
```

## ローカルの Speedle の呼び出し

[SDK 概要](/ja/docs/sdks/#example-policy) の説明に従って Speedle を起動し、サンプルポリシーを作成します。このプログラムを `main.go` として保存します:

```go
package main

import (
	"context"
	"fmt"
	"os"

	speedle "github.com/teramoby/speedle-sdk/go"
)

func main() {
	client, err := speedle.NewClient(speedle.Config{
		Backend:     speedle.BackendSpeedle,
		Endpoint:    "http://localhost:6734",
		Auth:        speedle.NoAuth(),
		ServiceName: "docs",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	allowed, err := client.IsAllowed(ctx, speedle.User("alice"), "get", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice get /docs/public: allowed=%t\n", allowed)

	d, err := client.Check(ctx, speedle.User("alice"), "delete", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice delete /docs/public: allowed=%t reason=%s\n", d.Allowed, d.Reason)
}
```

実行します:

```bash
go run .
```

出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## Speedle Cloud の呼び出し

> **Speedle Cloud はまもなく公開されます。**

プログラムは `SPEEDLE_SERVICE` からサービス名を、`SPEEDLE_TOKEN` から`ads` スコープのアカウント API キーを読み取ります。`SPEEDLE_ENDPOINT` はデフォルトのエンドポイント `https://cloud.speedle.io` を上書きします。`main.go` を以下に置き換えます:

```go
package main

import (
	"context"
	"fmt"
	"os"

	speedle "github.com/teramoby/speedle-sdk/go"
)

func main() {
	endpoint := os.Getenv("SPEEDLE_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://cloud.speedle.io"
	}
	client, err := speedle.NewClient(speedle.Config{
		Backend:     speedle.BackendSpeedleCloud,
		Endpoint:    endpoint,
		Auth:        speedle.BearerToken(os.Getenv("SPEEDLE_TOKEN")),
		ServiceName: os.Getenv("SPEEDLE_SERVICE"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	allowed, err := client.IsAllowed(ctx, speedle.User("alice"), "get", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice get /docs/public: allowed=%t\n", allowed)

	d, err := client.Check(ctx, speedle.User("alice"), "delete", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice delete /docs/public: allowed=%t reason=%s\n", d.Allowed, d.Reason)
}
```

環境変数を設定して実行します。`read -rs` を使用すると、API キーがシェルの履歴や画面に表示されません:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
go run .
```

サービスにサンプルポリシーが存在する場合の出力:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

API キーはサーバー側のシークレットです。ブラウザ、モバイルアプリ、またはデスクトップクライアントには絶対に配布しないでください。

## エラー処理

拒否されたリクエストはエラーではありません。上記の `delete` 呼び出しのように、`IsAllowed` は `false` を返し、`Check` は `Allowed` が false で理由を含む決定を返します。デフォルト設定では、呼び出しが決定を返せない場合に non-nil の `error` を返します（無効な構成の場合は `NewClient` も同様です）。サンプルでは標準エラー出力に `Speedle error: <message>` を出力し、終了ステータス 1 で終了します。
