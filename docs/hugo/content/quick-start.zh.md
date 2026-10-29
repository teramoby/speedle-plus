+++
title = "Speedle 快速入门"
description = "使用当前 Go 工具链编译 Speedle+，验证策略管理、直接授权和拒绝结果。"
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "从源码编译 Speedle+，使用 SPDL 和 spctl 创建策略，再通过 REST API 验证授权结果。本教程使用本地文件存储策略。"
icon = "9. Getting Started.svg"
+++

## 先在 Playground 中试用

**无需安装，即可先体验 Speedle 的基本功能。** 打开 [Playground 策略实验室](../playground/)，编辑 SPDL 策略、设置用户、资源和动作，即可在浏览器中查看真实的 Allow/Deny（允许／拒绝）授权结果，无需安装 Go 或启动服务。

准备在本机编译和运行 Speedle 时，再继续下面的步骤。

## 准备环境

准备 Linux 或 macOS、Bash、Git、curl，以及 **Go 1.26 或更新版本**。Speedle+ 支持最近的两个 Go 大版本；本教程已在 **Go 1.26.10 和 Go 1.27.3** 上验证，推荐使用稳定版 Go 1.27.3 完成本教程。

按照 [Go 官方安装说明](https://go.dev/doc/install)安装，然后检查版本：

<!-- quick-start: version -->
```bash
go version
```

Go 会自动配置 `GOROOT` 和默认的 `GOPATH`，无需手动指定。下面将程序编译到源码目录下的 `bin` 目录。

## 第一步：下载源码并编译

<!-- quick-start: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

输出目录中应有三个程序：

```text
spctl  speedle-ads  speedle-pms
```

- `spctl`：命令行工具。
- `speedle-pms`：授权策略管理服务（PMS）。
- `speedle-ads`：授权决策服务（ADS）。

新版 Go 使用 `go build` 或 `go install` 构建可执行程序；`go get` 用于管理模块依赖。

## 第二步：启动策略管理服务

本教程用于本机开发演示。`--insecure=true` 为下面的 REST 示例启用明文 HTTP。gRPC 服务默认关闭，需要时用 `--grpc-endpoint` 开启，例如 PMS 用 `--grpc-endpoint 127.0.0.1:50001`，ADS 用 `--grpc-endpoint 127.0.0.1:50002`；gRPC 与 REST 使用相同的 TLS 设置。生产环境的传输安全和授权配置请参阅[安全指南](../docs/security/)。

在第一个终端中，进入 `speedle-plus` 源码目录后执行：

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

保持服务运行，等待 REST 服务启动后再继续。PMS 会在当前源码目录创建 `quick-start-policies.json`；PMS 和 ADS 必须使用同一个文件。

## 第三步：创建和查看策略

打开第二个终端，进入同一个 `speedle-plus` 源码目录。

以图书馆为例，创建 `library` 服务：

<!-- quick-start: service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service library
```

添加“张三可以借书”的策略：

<!-- quick-start: policies -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user ZhangSan borrow book" --service-name=library
```

查看服务及其策略：

<!-- quick-start: list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

输出中应包含 `library` 服务及刚创建的授权策略。

## 第四步：启动授权决策服务

打开第三个终端，进入同一个 `speedle-plus` 源码目录：

<!-- quick-start: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

保持服务运行，等待 REST 服务启动后再继续。

## 第五步：验证授权结果

在第二个终端中，询问张三是否可以借书：

<!-- quick-start: allow-direct -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"ZhangSan"}]},"serviceName":"library","resource":"book","action":"borrow"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

响应中应包含 `"allowed":true`。

再询问没有获得授权的李四是否可以借书：

<!-- quick-start: deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"LiSi"}]},"serviceName":"library","resource":"book","action":"borrow"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

响应中应包含 `"allowed":false`。

使用结束后，在 PMS 和 ADS 的终端分别按 **Ctrl+C** 停止服务。策略文件会保留在源码目录中；再次执行创建命令时，请使用新的策略文件。

下一步可阅读 [SPDL 语言](../docs/spdl/)、[TLS 配置](../docs/security/#message-security-tls)和[部署指南](../docs/deployment/)，了解 ACL、RBAC、ABAC 及其他部署方式。
