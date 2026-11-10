+++
title = "集成"
noindex = false
categories = ["docs"]
description = "将 Speedle 接入你的应用与基础设施。"
aliases = ["/zh/integrations/istio/", "/zh/integrations/kubernetes/", "/zh/integrations/docker/"]
+++

Speedle 负责授权决策。应用、网关或平台中的策略执行点（PEP）在每次访问时向 Speedle 请求决策，并执行返回的结果。下面是目前支持的接入方式。

## REST 与 gRPC API

在策略管理服务（PMS）中管理服务、策略和角色，向授权决策服务（ADS）请求运行时决策。两者都提供 REST API；启动任一服务时加上 `--grpc-endpoint` 即可启用对应的 gRPC API。

- [策略管理 API](/zh/docs/api/management_api/)
- [授权决策 API](/zh/docs/api/decision_api/)
- [令牌断言 webhook](/zh/docs/api/asserter_api/)
- [API 概览，包括 PMS 和 ADS 的 proto](/zh/docs/api/)

## 嵌入式 Go

在 Go 应用中直接运行求值器，不需要单独的 PMS 或 ADS 进程。[嵌入式模式指南](/zh/docs/deployment/embedded/)使用 Go modules 构建了一个完整示例。

## SDK

提供 Java、Go、Python、Node.js/TypeScript、Rust 和 C#/.NET 客户端 SDK。参见 [SDK](/zh/docs/sdks/)。

## 策略存储

可以使用内置的文件、etcd、MySQL 或 MongoDB 存储，也可以实现存储接口接入其他后端。参见[存储扩展](/zh/docs/store/)。

## 此前的指南

此前的 Istio、Kubernetes 和 Docker 示例指南已不再发布。新的集成请使用上面的接口。
