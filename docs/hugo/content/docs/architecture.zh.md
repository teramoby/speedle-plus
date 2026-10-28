+++
title = "系统架构"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PAP", "policy administration point", "策略管理点", "PDP", "policy decision point", "策略决策点", "PEP", "policy enforcement point", "策略执行点", "PIP", "policy information point", "策略信息点", "centralized authorization", "集中式授权", "authorization policy management", "授权策略管理", "authorization decision service", "授权决策服务", "API authorization", "API 授权", "API 权限控制", "microservices authorization", "微服务授权", "微服务权限控制", "cloud-native authorization", "云原生授权"]
description = "Speedle building blocks"
weight = 10
draft = false
toc = false
tocheading = "h2"
tocsidebar = false
bref = "Speedle architecture"
+++

## Modules

Speedle is an authorization engine comprised of these components:

- Policy Management Service (PMS) API - Manages the authorization and role policies, and the objects from which they are created.

- Policy repository - Stores all the policy artifacts. The policy repository can be a json file, or a persistent store such as a database or etcd.

- Authorization Decision Service (ADS) API - Evaluates the authorization requests against the applicable policies and returns GRANT/DENY decisions.

This diagram shows the interaction between these components.

<img src="/img/speedle/spdlarch.jpg" />

(1) Users create/manage policies through the Policy Management Service API.  
(2) The Policy Management Service persists the policies in the policy repository.  
(3) The policies are provisioned from the policy repository to the Authorization Decision Service API.  
(4) Users systems invoke the Authorization Decision Service API for authorization checks.

If you are familiar with the XACML model, the Policy Management Service (PMS) serves as the Policy Administration Point (PAP), and the Authorization Decision Service (ADS) serves as the Policy Decision Point (PDP).

[授权与访问控制](/docs/access-control/)提供 RBAC、ABAC 和 PBAC 示例；[身份与 IAM 集成](/docs/access-control/identity-iam/)说明身份边界；[OES](/docs/access-control/oes/) 与 [XACML](/docs/access-control/xacml/) 专题介绍架构比较和迁移注意事项。
