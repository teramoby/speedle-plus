+++
title = "授权与访问控制"
keywords = ["Speedle+", "authorization", "access control", "RBAC", "ABAC", "PBAC", "identity", "IAM", "OES", "XACML", "SPDL", "授权", "访问控制", "身份与访问管理", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PAP", "policy administration point", "策略管理点", "PDP", "policy decision point", "策略决策点", "PEP", "policy enforcement point", "策略执行点", "PIP", "policy information point", "策略信息点", "centralized authorization", "集中式授权", "authorization policy management", "授权策略管理", "authorization decision service", "授权决策服务", "API authorization", "API 授权", "API 权限控制", "microservices authorization", "微服务授权", "微服务权限控制", "cloud-native authorization", "云原生授权", "zero trust", "零信任"]
description = "了解 Speedle+ 如何通过 RBAC、ABAC 和 PBAC 实现授权与细粒度访问控制，接入身份与 IAM 系统，以及与 OES、XACML 的关系。"
weight = 12
bref = "授权概念、可运行策略与集成边界"
+++

## Speedle 授权什么？

Speedle+ 是开源授权引擎（authorization engine），用于判断某个主体能否在指定服务中，对某项资源执行某种操作。应用负责执行这一判断。将访问控制（access control）独立为策略，可以让多个服务共享授权规则，减少业务代码中的重复权限判断。

身份认证确定“是谁”，授权确定“可以做什么”。Speedle 可以使用可信应用提供的身份，或通过已配置的令牌断言服务解析身份；它并不替代身份与访问管理（Identity and Access Management，IAM）系统的全部组件。

## 细粒度访问控制（FGA）与零信任

细粒度访问控制（Fine-grained access control，FGA）针对每个请求，依据具体的主体、操作、资源和上下文作出决策，而不是按整个应用或网络一次性放行。Speedle+ 用 SPDL 策略评估每一个请求，因此决策可以取决于用户及其所属的组和角色、具体资源以及请求的属性。

零信任（zero trust）架构不因请求来自何处而信任它，对受保护资源的每次访问都要显式授权。Speedle+ 负责其中的授权决策。按 XACML 的术语，应用或网关是策略执行点（PEP），针对每个受保护的操作向授权决策服务——即策略决策点（PDP）——发起询问；策略通过策略管理服务——即策略管理点（PAP）——进行管理；可信的身份与属性来源则充当策略信息点（PIP），详见 [XACML 与 SPDL](/docs/access-control/xacml/)。Speedle+ 本身不提供认证、设备状态或网络层控制，零信任部署需要将它与这些组件结合使用。

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
