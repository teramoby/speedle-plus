+++
title = "授权与访问控制"
description = "了解 Speedle+ 如何通过 RBAC、ABAC 和 PBAC 实现授权与细粒度访问控制，接入身份与 IAM 系统，以及与 OES、XACML 的关系。"
weight = 12
bref = "授权概念、可运行策略与集成边界"
+++

## Speedle 授权什么？

Speedle+ 是开源授权引擎（authorization engine），用于判断某个主体能否在指定服务中，对某项资源执行某种操作。应用负责执行这一判断。将访问控制（access control）独立为策略，可以让多个服务共享授权规则，减少业务代码中的重复权限判断。

身份认证确定“是谁”，授权确定“可以做什么”。Speedle 可以使用可信应用提供的身份，或通过已配置的令牌断言服务解析身份；它并不替代身份与访问管理（Identity and Access Management，IAM）系统的全部组件。

## 选择访问控制模型

| 专题 | 在 Speedle+ 中的用途 |
| --- | --- |
| [RBAC：基于角色的访问控制（role-based access control）](/docs/access-control/rbac/) | 给用户或组分配角色，再向角色授予权限。 |
| [ABAC：基于属性的访问控制（attribute-based access control）](/docs/access-control/abac/) | 使用可信基础设施提供的属性编写条件。 |
| [PBAC：基于策略的访问控制（policy-based access control）](/docs/access-control/pbac/) | 集中管理授权规则，通过决策服务或嵌入式引擎执行判断。 |
| [身份与 IAM](/docs/access-control/identity-iam/) | 对接身份提供方、区分身份域并明确可信边界。 |
| [OES 与 Speedle](/docs/access-control/oes/) | 比较 Oracle Entitlements Server 的架构概念并评估迁移。 |
| [XACML 与 SPDL](/docs/access-control/xacml/) | 对照 PAP/PDP/PEP 的职责，了解策略语言之间的差异。 |

RBAC 和 ABAC 可以同时用于同一个策略体系。这些术语描述访问控制的不同方面，采用 PBAC 并不意味着放弃角色或属性。

## 第一条授权策略

将以下 SPDL 策略粘贴到 [Playground](/playground/)：

```text
grant user alice read book
deny user alice delete book
```

用户填 `alice`，资源填 `book`，操作填 `read`，预期为 Allow；改成 `delete`，预期为 Deny；改成 `write`，由于没有匹配的允许策略，默认拒绝。上线前应同时验证允许与拒绝的请求。

在实际服务中，[授权决策服务](/docs/decisions/)负责评估请求，应用必须执行决策并处理评估错误。浏览器 Playground 在本地运行真实的 Go 授权引擎，但不会连接身份提供方或外部自定义函数。

继续阅读[架构](/docs/architecture/)、[SPDL 参考](/docs/spdl/)和[快速入门](/quick-start/)。
