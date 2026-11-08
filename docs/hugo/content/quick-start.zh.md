+++
title = "Speedle 快速入门"
description = "使用 go install 或源码编译安装 Speedle，验证策略管理、直接授权以及带条件（ABAC）的允许与拒绝结果。"
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "使用 go install 安装 Speedle 或从源码编译，使用 SPDL 和 spctl 创建策略，再通过 REST API 验证授权结果。本教程使用本地文件存储策略。"
icon = "9. Getting Started.svg"
+++

## 先在 Playground 中试用

**无需安装，即可先体验 Speedle 的基本功能。** 打开 [Playground 策略实验室](../playground/)，编辑 SPDL 策略、设置用户、资源和动作，即可在浏览器中查看真实的 Allow/Deny（允许／拒绝）授权结果，无需安装 Go 或启动服务。

准备在本机安装和运行 Speedle 时，再继续下面的步骤。

## 准备环境

准备 Linux 或 macOS、Bash、Git、curl，以及 **Go 1.26.10 或 Go 1.27.3**（也可使用同一大版本的更新补丁版本）。Speedle 支持 Go 1.26 和 Go 1.27 的安全补丁版本；本教程已在 **Go 1.26.10 和 Go 1.27.3** 上验证，推荐使用稳定版 Go 1.27.3 完成本教程。

按照 [Go 官方安装说明](https://go.dev/doc/install)安装，然后检查版本：

<!-- quick-start: version -->
```bash
go version
```

Go 会自动配置 `GOROOT` 和默认的 `GOPATH`，无需手动指定。下面的两种方式都会把三个可执行程序放到 `bin` 目录。

## 安装 Speedle

两种方式任选一种：方式 A 安装最新发布版本，方式 B 编译当前源码。后续命令都使用 `./bin/...`，两种方式通用。

### 方式 A：安装最新发布版本

<!-- quick-start: install -->
```bash
GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@v1.6.0
ls bin
```

`GOBIN` 把可执行程序放到当前目录的 `bin` 中；不设置时，Go 会把它们写入 `$(go env GOPATH)/bin`。`v1.6.0` 是撰写本文时的最新发布标签；`go install` 使用该标签的源码编译，并通过 Go 校验和数据库验证。

### 方式 B：下载源码并编译

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

> **macOS：** `/usr/sbin/spctl` 是 Apple 的 Gatekeeper 工具，直接运行 `spctl` 时可能会选中它。请使用 Speedle CLI 的完整路径，或在其所在目录中用 `./spctl` 调用；本指南统一使用 `./bin/spctl`。运行 `command -v spctl` 可查看 shell 实际选择的可执行程序。

新版 Go 使用 `go build` 或 `go install` 构建可执行程序；`go get` 用于管理模块依赖。

## 运行 Speedle

本教程用于本机开发演示。`--insecure=true` 为下面的 REST 示例启用明文 HTTP。gRPC 服务默认关闭，需要时用 `--grpc-endpoint` 开启，例如 PMS 用 `--grpc-endpoint 127.0.0.1:50001`，ADS 用 `--grpc-endpoint 127.0.0.1:50002`；gRPC 与 REST 使用相同的 TLS 设置。生产环境的传输安全和授权配置请参阅[安全指南](../docs/security/)。

## 第一步：启动策略管理服务

在第一个终端中，进入包含 `bin/` 的目录（方式 B 为 `speedle-plus` 源码目录）后执行：

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

保持服务运行，等待 REST 服务启动后再继续。PMS 会在当前目录创建 `quick-start-policies.json`；PMS 和 ADS 必须使用同一个文件。

## 第二步：创建和查看策略

打开第二个终端，进入同一个目录。

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

## 第三步：启动授权决策服务

打开第三个终端，进入同一个目录：

<!-- quick-start: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

保持服务运行，等待 REST 服务启动后再继续。

## 第四步：验证授权结果

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

`reason` 字段说明决策原因：`0`（`GRANT_POLICY_FOUND`）表示找到授权策略；`3`（`NO_APPLICABLE_POLICIES`）表示没有适用的策略，Speedle 默认拒绝。完整原因码见[授权决策](../docs/decisions/)。

## 第五步：添加带条件（ABAC）的策略

创建一条带条件的授权：仅当请求携带 `clearance` 属性且其值不小于 3 时，`ZhangSan` 才能 `borrow` 资源 `ebook`：

<!-- quick-start: abac-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user ZhangSan borrow ebook if clearance >= 3" --service-name=library
```

PEP（策略执行点）负责传入请求属性，条件只根据请求中的属性求值。下文的 `clearance` 值仅用于演示请求格式：真实的 PEP 必须从可信来源获取属性值，例如已认证的令牌声明或可信存储，不能直接信任客户端在待授权请求中自行填写的值。请求不带 `clearance` 时，该条件不匹配。

`clearance` 为 4 时允许：

<!-- quick-start: abac-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"ZhangSan"}]},"serviceName":"library","resource":"ebook","action":"borrow","attributes":[{"name":"clearance","type":"numeric","value":4}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

响应中应包含 `"allowed":true` 和 `"reason":0`。

`clearance` 为 2 时条件不匹配，没有适用的策略，请求返回 Deny：

<!-- quick-start: abac-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"ZhangSan"}]},"serviceName":"library","resource":"ebook","action":"borrow","attributes":[{"name":"clearance","type":"numeric","value":2}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

响应中应包含 `"allowed":false` 和 `"reason":3`。

使用结束后，在 PMS 和 ADS 的终端分别按 **Ctrl+C** 停止服务。策略文件会保留在当前目录中；再次执行创建命令时，请使用新的策略文件。

下一步可阅读 [SPDL 语言](../docs/spdl/)、[TLS 配置](../docs/security/#同时保护两种-api-传输)和[部署指南](../docs/deployment/)，了解 ACL、RBAC、ABAC 及其他部署方式。
