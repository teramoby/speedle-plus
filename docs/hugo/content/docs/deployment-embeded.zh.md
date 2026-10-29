+++
title = "Speedle嵌入模式"
description = "使用 Go modules 将 Speedle 嵌入 Go 应用，并验证允许与拒绝的授权结果。"
weight = 12
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
+++

## 什么是 Speedle 嵌入模式

嵌入模式将 Speedle 策略决策引擎运行在你的 Go 应用进程中，通过 evaluator 直接决策，无需启动 PMS 或 ADS 服务。应用负责认证用户，并为每次决策提供主体、资源和操作。

编写代码前，可以先用 [Playground](../../playground/) 体验基本策略功能。如果希望以独立服务运行 Speedle，请参阅[快速入门](../../quick-start/)。

## 前提条件

使用 Linux 或 macOS、命令行 shell，以及 **Go 1.26.10 或 Go 1.27.3**（也可使用同一大版本的更新补丁版本），安全补丁版本要求与[快速入门](../../quick-start/)一致。本例使用 Go modules 和已发布的 **Speedle+ v1.6.0**，推荐使用 Go 1.27.3；示例已在 Go 1.26.10 和 Go 1.27.3 下验证。

按照 [Go 官方安装说明](https://go.dev/doc/install)安装 Go，然后检查版本：

<!-- embedded: version -->
```bash
go version
```

Go 会自动配置 `GOROOT` 和默认 `GOPATH`，无需手工设置。

## 创建应用模块

在 Speedle 源码目录之外，选择一个有写入权限的目录，从新目录开始本例：

<!-- embedded: setup -->
```bash
mkdir speedle-embedded-demo
cd speedle-embedded-demo
go mod init example.com/speedle-embedded-demo
go get github.com/teramoby/speedle-plus/pkg/eval@v1.6.0
```

如果已有包含 `go.mod` 的应用，只需在该应用的模块目录中执行最后一条 `go get` 命令。

这里的 `go get` 将指定版本的**库依赖**加入 `go.mod` 和 `go.sum`，不会安装程序。快速入门使用 `git clone` 和 `go build` 构建独立服务的可执行文件，两种用法均使用 Go modules。请将 `go.mod` 和 `go.sum` 纳入版本控制。

## 创建策略文件

在 `speedle-embedded-demo` 目录中，将以下内容保存为 `policies.spdl`。它允许 `alice` 对服务 `mysvc` 中的资源 `res1` 执行 `get` 操作：

<!-- embedded: policies -->
```ini
[service.mysvc]
[policy]
grant user alice get res1
```

`eval.NewFromFile` 从本地文件加载策略，支持 SPDL 和 JSON 两种格式。本例使用 SPDL。

## 初始化 evaluator 并请求决策

将以下完整程序保存为同一目录下的 `main.go`：

<!-- embedded: program -->
```go
package main

import (
	"fmt"
	"log"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func main() {
	evaluator, err := eval.NewFromFile("./policies.spdl", false)
	if err != nil {
		log.Fatal(err)
	}

	for _, user := range []string{"alice", "bob"} {
		allowed, _, err := evaluator.IsAllowed(ads.RequestContext{
			Subject: &ads.Subject{
				Principals: []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: user}},
			},
			ServiceName: "mysvc",
			Resource:    "res1",
			Action:      "get",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s allowed=%t\n", user, allowed)
	}
}
```

空白导入 `_ "github.com/teramoby/speedle-plus/pkg/store/file"` 用于注册文件存储驱动；省略它会出现 `unknown store type "file"` 错误。

`eval.NewFromFile(loc, isWatch)` 返回 `ads.PolicyEvaluator` 实例和错误。使用实例前必须检查错误。`isWatch=false` 表示只加载一次策略；长期运行的应用可设为 `true`，启用文件变更监听。

在该实例上调用 `evaluator.IsAllowed(ctx)`。参数 `ads.RequestContext` 包含主体的 principals、`ServiceName`、`Resource`、`Action` 和可选的 `Attributes`。返回值为 `(allowed bool, reason ads.Reason, err error)`。请区分决策失败的错误与正常返回 `allowed=false` 的拒绝结果。

## 构建并运行

在 `speedle-embedded-demo` 目录中，整理依赖并构建应用：

<!-- embedded: build -->
```bash
go mod tidy
go build -o ./bin/embedded-demo .
```

仍在同一目录中运行，确保 `./policies.spdl` 指向刚才创建的文件：

<!-- embedded: run -->
```bash
./bin/embedded-demo
```

决策输出如下：

```text
alice allowed=true
bob allowed=false
```

`alice` 匹配授权策略；`bob` 没有匹配的授权策略，因此被拒绝。本例无需启动网络服务或数据库。
