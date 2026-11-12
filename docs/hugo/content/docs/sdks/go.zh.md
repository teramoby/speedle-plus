+++
title = "Go SDK"
description = "添加 Speedle Go SDK，向本地 Speedle 或 Speedle Cloud 发起授权检查。"
weight = 30
aliases = ["/zh/docs/api/sdk-go/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 go get 添加 github.com/teramoby/speedle-sdk/go v0.2.0，再向本地 Speedle 或 Speedle Cloud 调用 IsAllowed 与 Check。"
lastmod = 2026-11-08T11:21:16+08:00
+++

Go SDK 支持最新的两个 Go 版本，只使用标准库。模块：[`github.com/teramoby/speedle-sdk/go`](https://pkg.go.dev/github.com/teramoby/speedle-sdk/go) v0.2.0，包名 `speedle`。参考文档：[speedle-sdk/go](https://github.com/teramoby/speedle-sdk/tree/main/go)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

## 添加依赖

在新模块中执行：

```bash
go mod init example.com/speedle-example
go get github.com/teramoby/speedle-sdk/go@v0.2.0
```

## 调用本地 Speedle

按 [SDK 概览](/docs/sdks/#示例策略)中的说明启动 Speedle 并创建示例策略。将以下程序保存为 `main.go`：

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

运行：

```bash
go run .
```

输出：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

## 调用 Speedle Cloud

> **Speedle Cloud 即将上线。**

程序从 `SPEEDLE_SERVICE` 读取服务名，从 `SPEEDLE_TOKEN` 读取具有 `ads` 权限范围的账号 API 密钥。`SPEEDLE_ENDPOINT` 可覆盖默认端点 `https://cloud.speedle.io`。将 `main.go` 替换为：

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

设置环境变量后运行。`read -rs` 不会把API 密钥写入 shell 历史，也不会显示在屏幕上：

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
go run .
```

服务中已有示例策略时，输出为：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```

API 密钥是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。

## 错误处理

授权请求返回 Deny 并不代表调用出错：`IsAllowed` 返回 `false`，`Check` 返回 `Allowed` 为 false 的结果及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，调用返回非 nil 的 `error`（配置无效时 `NewClient` 也是如此）。示例向标准错误输出 `Speedle error: <消息>`，并以状态码 1 退出。
