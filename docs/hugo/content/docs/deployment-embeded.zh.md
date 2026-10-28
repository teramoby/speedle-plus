+++
title = "Speedle嵌入模式"
description = ""
weight = 12
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
+++

## 什么是嵌入模式？

Go 应用可以在自身进程中运行策略引擎，直接调用 API 决策。应用仍需提供可信身份与属性，并执行返回的允许或拒绝结果。也可以先用 [Playground](/playground/) 了解策略语义，无需启动服务器。

## 支持范围与依赖

当前 `go.mod` 要求 Go 1.26 或更高版本。在自己的应用中，先执行 `go mod init`，再用 `go get github.com/teramoby/speedle-plus@<reviewed-tag-or-commit>` 添加依赖；将占位符替换为已审查的标签或提交。历史 v2.0 发布包与当前 master 的依赖和行为不同，见[版本说明](/docs/versions/)。

`eval.NewFromFile(location, watch)` 用于 JSON 或 SPDL 文件。嵌入模式还可以通过 `eval.NewFromConfig` 或 `eval.NewWithStore` 使用实现 ADS 接口的已注册存储。只接受文件是 `NewFromFile` 的限制，不是整个嵌入模式的限制。需要导入对应实现，例如 `pkg/store/file`，完成 builder 注册。

## 运行完整示例

从仓库根目录运行。示例读取 [policies.json](https://github.com/teramoby/speedle-plus/blob/master/samples/docs/embedded/policies.json)：在 `library` 服务中允许 Alice 对 `book` 执行 `read`，Bob 没有匹配的允许策略。

<!-- docs-check: embedded -->
```bash
go run ./samples/docs/embedded
```


预期输出包含 `alice: allowed=true` 和 `bob: allowed=false`。

<!-- docs-source: samples/docs/embedded/main.go -->
```go
package main

import (
	"flag"
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
)

func main() {
	policies := flag.String("policies", "samples/docs/embedded/policies.json", "Policy file")
	flag.Parse()
	evaluator, err := eval.NewFromConfig(&cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: "file", StoreProps: map[string]interface{}{"FileLocation": *policies}}, EnableWatch: false})
	if err != nil {
		log.Fatal(err)
	}
	defer evaluator.Close()
	for _, user := range []string{"alice", "bob"} {
		allowed, reason, err := evaluator.IsAllowed(ads.RequestContext{Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: user}}}, ServiceName: "library", Resource: "book", Action: "read"})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: allowed=%t reason=%s\n", user, allowed, reason.String())
	}
}
```


## 处理结果与生命周期

`IsAllowed(ads.RequestContext)` 返回 `(allowed bool, reason ads.Reason, err error)`。只要 `err != nil` 或 `allowed == false`，应用就应拒绝操作；引擎返回决策，并不会自动阻止业务操作。还可通过 `GetAllGrantedRoles`、`GetAllGrantedPermissions` 与 `Diagnose` 查询评估信息。

示例使用 `NewFromConfig`，其返回值提供 `Close()`。使用完应关闭评估器，停止后台任务并释放存储。需要观察策略更新时设置 `EnableWatch: true`；更新是异步的，应确认目标版本到达后再依赖新策略。更多内容见[策略存储](/docs/store/)和[嵌入式费用报销示例](https://github.com/teramoby/speedle-plus/tree/master/samples/embedded/expenses)。
