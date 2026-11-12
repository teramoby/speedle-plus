+++
title = "SDK"
description = "Speedle 授权检查的客户端库，支持 Java、Python、Go、.NET、Rust 和 Node.js/TypeScript。"
weight = 305
draft = false
toc = true
linksection = true
tocheading = "h2"
tags = ["sdk"]
categories = ["docs"]
bref = "Speedle 决策 API 的六种语言客户端库，既可用于自托管的 Speedle ADS，也可用于 Speedle Cloud（即将上线）。"
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle SDK 是决策 API `POST /authz-check/v1/is-allowed` 的客户端库，既可用于自托管的 Speedle ADS，也可用于 Speedle Cloud。每个语言页面说明如何添加软件包，并分别给出两种场景下经过实测的完整程序。源代码和完整参考见 [teramoby/speedle-sdk](https://github.com/teramoby/speedle-sdk)。这些示例要调用的 PMS 和 ADS，见[部署](/docs/deployment/)和[Docker 镜像](/docs/deployment/docker/)。

## 软件包

Java、Python、Go、.NET 和 Rust 的版本均为 0.2.0。`@speedle/sdk` 已在 npm 发布。安装命令见各语言页面。

- [Java](/docs/sdks/java/)：Maven Central `io.speedle:speedle-sdk`，已发布。
- [Python](/docs/sdks/python/)：PyPI `speedle`，已发布。
- [Go](/docs/sdks/go/)：Go 模块 `github.com/teramoby/speedle-sdk/go`，已发布。
- [C#/.NET](/docs/sdks/dotnet/)：NuGet `Speedle.Sdk`，已发布。
- [Rust](/docs/sdks/rust/)：crates.io `speedle`，已发布。
- [Node.js / TypeScript](/docs/sdks/node/)：npm `@speedle/sdk`，已发布。

## 功能

| 功能 | 状态 |
| --- | --- |
| `isAllowed`：返回授权结果 | 六个 SDK 均支持 |
| `check`：返回授权结果及原因，例如没有适用的策略 | 六个 SDK 均支持 |
| 自托管 Speedle ADS | 六个 SDK 均支持 |
| Speedle Cloud | 即将上线 |

方法名遵循各语言的命名习惯：Python 和 Rust 中为 `is_allowed` 与 `check`，Go 中为 `IsAllowed` 与 `Check`，.NET 中为 `IsAllowedAsync` 与 `CheckAsync`。

其他操作（如查询已授予的权限、诊断和策略管理）请使用 REST API：[授权决策 API](/docs/api/decision_api/) 和[策略管理 API](/docs/api/management_api/)。

## 示例策略

所有示例都检查服务 `docs` 中的用户 `alice` 能否对资源 `/docs/public` 执行 `get` 和 `delete`。要在本地 Speedle 上得到文档中的结果，先按[快速入门](/quick-start/)构建 Speedle，在 6733 端口启动 PMS、在 6734 端口启动 ADS，然后创建服务和策略：

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

本地示例以无凭据方式连接 `http://localhost:6734`。SDK 只对回环地址接受明文 `http://`；连接其他 ADS 时请使用 HTTPS。

Speedle Cloud 示例连接 `https://cloud.speedle.io`，并从环境变量 `SPEEDLE_SERVICE` 和 `SPEEDLE_TOKEN` 读取服务名及具有 `ads` 权限范围的账号 API 密钥。在该服务中创建同样的策略即可得到相同的结果。API 密钥是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。
