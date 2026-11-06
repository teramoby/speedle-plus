+++
title = "API"
description = "Open API Specification Docs"
weight = 300
draft = false
toc = false
tocheading = "h4"
tocsidebar = false
tags = ["v0.1.0", "api", "guide", "core"]
categories = ["docs"]
bref = "Open API Specification Docs. Use the API to model your simple and complex policies in a declarative way; get decisions, discover policies dynamically and debug policy evaluation."
+++

## SDK

决策 API `POST /authz-check/v1/is-allowed` 的客户端库既可用于自托管的 Speedle ADS，也可用于 Speedle Cloud。每个页面说明如何添加软件包，并分别给出两种场景下经过实测的完整程序。

| 语言 | 软件包 | 版本 |
| --- | --- | --- |
| [Java](/docs/api/sdk-java/) | Maven Central `io.speedle:speedle-sdk` | 0.1.0 |
| [Python](/docs/api/sdk-python/) | PyPI `speedle` | 0.1.0 |
| [Go](/docs/api/sdk-go/) | `github.com/teramoby/speedle-sdk/go` | 0.1.0 |
| [C#/.NET](/docs/api/sdk-dotnet/) | NuGet `Speedle.Sdk` | 0.1.0 |
| [Rust](/docs/api/sdk-rust/) | crates.io `speedle` | 0.1.0 |
| [Node.js/TypeScript](/docs/api/sdk-node/) | npm `@speedle/sdk` | 0.1.0，发布后可用 |

SDK 调用的是 `is-allowed`：`isAllowed` 返回授权结果，`check` 还会返回原因。其他决策操作（如查询已授予的权限、诊断）请使用 [REST API](/docs/api/decision_api/)。源代码和完整参考见 [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk)。

所有示例都检查服务 `docs` 中的用户 `alice` 能否对资源 `/docs/public` 执行 `get` 和 `delete`。要在本地 Speedle 上得到文档中的结果，先按[快速入门](/quick-start/)构建 Speedle，在 6733 端口启动 PMS、在 6734 端口启动 ADS，然后创建服务和策略：

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

本地示例以无凭据方式连接 `http://localhost:6734`。SDK 只对回环地址接受明文 `http://`；连接其他 ADS 时请使用 HTTPS。

**Speedle Cloud 即将上线。** Speedle Cloud 示例连接 `https://cloud.speedle.io`，并从环境变量 `SPEEDLE_SERVICE` 和 `SPEEDLE_TOKEN` 读取服务名及该服务专属的令牌。在该服务中创建同样的策略即可得到相同的结果。令牌是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。

## REST API

这些接口说明面向**你配置的 PMS、ADS 或断言服务端点**。`speedle.io` 提供文档与浏览器 Playground，不托管 PMS/ADS 后端。发送 JSON 时必须设置 `Content-Type: application/json`，并配置 [TLS/mTLS 与调用方访问控制](/docs/security/)。

- [策略管理 API](/docs/api/management_api/)
- [授权决策 API](/docs/api/decision_api/)
- [令牌断言 webhook](/docs/api/asserter_api/)

策略和角色策略的 ID 是字符串，PMS 返回扁平对象；授权权限字段为复数 `permissions`。REST 决策请求使用带类型的属性数组，诊断响应及断言 webhook 的属性则为对象。

## gRPC API

- [PMS proto](/protobuf/pms.proto)：`pb.PolicyManager`
- [ADS proto](/protobuf/ads.proto)：`pb.Evaluator`

当前 proto 必须与生成客户端配套使用。PMS 的服务名为 `pb.PolicyManager`，ADS 为 `pb.Evaluator`。这两份历史协议重用了消息名，应分别生成和加载，不要合并进同一 descriptor set。默认不启用 gRPC；需通过 `--grpc-endpoint` 显式开启，其 TLS 配置与 REST 相同。

历史 gRPC 的 `ContextRequest.attributes` 是字符串 map，不是 REST 的带类型属性数组，也不提供同样的数值、布尔、日期类型。条件需要这些类型时，应使用 REST 或嵌入式 Go，除非你的 gRPC 适配层明确实现了相应转换。

## 嵌入式 Go API

- [策略管理接口](https://github.com/teramoby/speedle-plus/tree/master/api/pms)
- [授权决策接口](https://github.com/teramoby/speedle-plus/tree/master/api/ads)

完整应用示例见[嵌入模式](/docs/deployment-embeded/)。
