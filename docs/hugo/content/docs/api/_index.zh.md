+++
title = "API"
description = "PMS、ADS 与令牌断言的当前接口契约"
weight = 300
draft = false
toc = false
tocheading = "h4"
tocsidebar = false
tags = ["v0.1.0", "api", "guide", "core"]
categories = ["docs"]
bref = "使用 REST、gRPC 与嵌入式 Go 接口管理策略、查询决策并诊断评估"
+++

## REST API

这些接口说明面向**你配置的 PMS、ADS 或断言服务端点**。`speedle.io` 提供文档与浏览器 Playground，不托管 PMS/ADS 后端。发送 JSON 时必须设置 `Content-Type: application/json`，并配置 [TLS/mTLS 与调用方访问控制](/docs/security/)。

- [策略管理 API](/docs/api/management_api/)
- [授权决策 API](/docs/api/decision_api/)
- [令牌断言 webhook](/docs/api/asserter_api/)

策略和角色策略的 ID 是字符串，PMS 返回扁平对象；授权权限字段为复数 `permissions`。REST 决策请求使用带类型的属性数组，诊断响应及断言 webhook 的属性则为对象。

## gRPC API

- [PMS proto](/protobuf/pms.proto)：`pms_pb.PolicyManager`
- [ADS proto](/protobuf/ads.proto)：`ads_pb.Evaluator`

当前 proto 必须与生成客户端配套使用，旧客户端的 `/pb.*` 方法路径不匹配服务端名称。生成的 Go 绑定保留既有包 API；维护者可按 `scripts/generate-protobuf.sh` 中固定的 protoc/generator 版本重新生成。gRPC 与 REST 使用相同的传输安全配置，明文通道无法调用 TLS 端点。

历史 gRPC 的 `ContextRequest.attributes` 是字符串 map，不是 REST 的带类型属性数组，也不提供同样的数值、布尔、日期类型。条件需要这些类型时，应使用 REST 或嵌入式 Go，除非你的 gRPC 适配层明确实现了相应转换。

## 嵌入式 Go API

- [策略管理接口](https://github.com/teramoby/speedle-plus/tree/master/api/pms)
- [授权决策接口](https://github.com/teramoby/speedle-plus/tree/master/api/ads)

完整应用与资源生命周期示例见[嵌入模式](/docs/deployment-embeded/)。
