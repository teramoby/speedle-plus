+++
title = "Go SDK"
description = "添加 Speedle Go SDK，向本地 Speedle 发起授权检查。"
weight = 30
aliases = ["/zh/docs/api/sdk-go/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 go get 添加 github.com/teramoby/speedle-sdk/go v0.2.0，再向本地 Speedle 调用 IsAllowed 与 Check。"
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


## 错误处理

授权请求返回 Deny 并不代表调用出错：`IsAllowed` 返回 `false`，`Check` 返回 `Allowed` 为 false 的结果及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，调用返回非 nil 的 `error`（配置无效时 `NewClient` 也是如此）。示例向标准错误输出 `Speedle error: <消息>`，并以状态码 1 退出。
