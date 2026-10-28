+++
title = "系统架构"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PAP", "policy administration point", "策略管理点", "PDP", "policy decision point", "策略决策点", "PEP", "policy enforcement point", "策略执行点", "PIP", "policy information point", "策略信息点", "centralized authorization", "集中式授权", "authorization policy management", "授权策略管理", "authorization decision service", "授权决策服务", "API authorization", "API 授权", "API 权限控制", "microservices authorization", "微服务授权", "微服务权限控制", "cloud-native authorization", "云原生授权"]
description = "Speedle 的组件、职责与可信边界"
weight = 10
draft = false
toc = false
tocheading = "h2"
tocsidebar = false
bref = "Speedle 系统架构"
+++

## 组成模块

Speedle 是授权引擎，包含三个主要组件：

- **策略管理服务（PMS）**：管理服务、授权策略、角色策略和自定义函数。
- **策略仓库**：通过已注册的 file、etcd、MongoDB 等存储持久化这些对象。
- **授权决策服务（ADS）**：根据适用策略评估主体、资源、操作和可信属性。

<img src="/img/speedle/spdlarch.jpg" />

历史示意图把决策组件标为 **ARS**，它就是当前文档和 API 中的 **ADS**。

(1) 管理员通过 PMS 管理策略。<br>
(2) PMS 把策略对象持久化到仓库。<br>
(3) ADS 加载策略，并异步观察存储支持的变更。<br>
(4) 应用提供可信请求上下文，调用 ADS，执行决策，并在出错时拒绝操作。

## 职责与可信边界

按 XACML 术语，PMS 对应策略管理点（PAP），ADS 或嵌入式评估器对应策略决策点（PDP），应用/网关对应策略执行点（PEP）。属性来源承担信息提供（PIP）职责。这是职责对照，不代表支持 XACML 协议。

管理策略、评估权限与认证 API 调用方是不同的职责。独立服务的 REST 与 gRPC 都需要适当的[安全配置](/docs/security/)。[令牌断言服务](/docs/assertor/)可以验证已配置的身份令牌，但一次策略决策并不能自动认证所有调用方，也不能代替应用阻止操作。

[授权与访问控制](/docs/access-control/)介绍 RBAC、ABAC、PBAC；[身份与 IAM](/docs/access-control/identity-iam/)说明身份边界；[OES](/docs/access-control/oes/) 与 [XACML](/docs/access-control/xacml/)提供架构比较；[嵌入模式](/docs/deployment-embeded/)演示 Go 进程内集成。
