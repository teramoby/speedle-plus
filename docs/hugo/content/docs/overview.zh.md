+++
title = "Speedle 概览：面向细粒度访问控制的授权引擎"
linktitle = "概览"
description = "Speedle 与常见授权术语的对应关系：PAP、PDP、PEP、PIP、RBAC、ABAC、PBAC、XACML、零信任、身份与 IAM。"
weight = 2
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["core", "pdp", "identity", "spdl"]
categories = ["docs"]
bref = "Speedle 在授权与访问控制术语中的位置"
+++

Speedle 是一个开源的授权引擎（authorization engine）。应用向它提出“这个用户能否对这个资源执行这个操作”这样的问题，Speedle 根据集中管理的策略回答允许或拒绝。本页把常见的授权术语和 Speedle 中实现它们的组件对应起来，并链接到相应的文档。

下面的示例使用名为 `library` 的服务。每条策略都通过策略管理服务的 REST API 创建，每个决策都由授权决策服务的 REST API 返回。reason 代码的含义见[授权查询](../decisions/)。

## 授权引擎（Authorization engine）

授权引擎根据策略评估访问请求并返回决策，使授权逻辑独立于应用代码。Speedle 由[策略管理服务（PMS）、策略存储和授权决策服务（ADS）](../architecture/)组成。ADS 可以作为独立服务运行，也可以[嵌入 Go 应用](../deployment-embeded/)。

## 授权与访问控制（Authorization and access control）

认证（authentication）确认调用者是谁；授权（authorization）决定调用者能做什么；访问控制（access control）则是执行这些决策。Speedle 只负责授权：它接收已经认证过的主体，返回决策，由调用它的应用或网关执行。参见[授权查询](../decisions/)。

## 细粒度访问控制（Fine-grained access control，FGA）

细粒度访问控制按用户、资源和操作逐一决定访问权限，还可以结合请求上下文，而不是对整个应用一次性放行。Speedle 的策略写明主体、操作和资源，并可以附加条件；角色策略可以只在某一个资源上授予角色。参见 [SPDL](../spdl/)。

## PAP：策略管理点（Policy Administration Point）

PAP 是创建和管理策略的地方。在 Speedle 中，PAP 就是 PMS，可以通过 [REST API](../api/management_api/)、gRPC API 或命令行工具 `spctl` 使用。参见[策略管理](../pms/policy-mgmt/)。

## PDP：策略决策点（Policy Decision Point）

PDP 根据策略评估请求并返回决策。在 Speedle 中，PDP 是 ADS；Speedle 嵌入 Go 进程运行时，则是嵌入式求值器。一个决策请求如下：

```bash
curl --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"library","resource":"book","action":"update"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

在下文 RBAC 一节的策略下，返回 `{"allowed":true,"reason":0}`。参见[决策 API](../api/decision_api/)。

## PEP：策略执行点（Policy Enforcement Point）

PEP 拦截访问请求，向 PDP 询问决策并执行。Speedle 不提供通用的 PEP；PEP 是调用 ADS 的应用或网关。[Docker、Kubernetes 和 Istio 集成](../../integrations/)就是 PEP 的示例。

## PIP：策略信息点（Policy Information Point）

PIP 提供评估策略所需的额外信息。在 Speedle 中，这些信息来自请求属性和[内置属性](../spdl/#2-2-1-内置属性-built-in-attributes)，来自把身份令牌转换为主体的[令牌断言器](../assertor/)，以及在求值时调用外部 REST 接口的[自定义函数](../custom-function/)。

## XACML

XACML（eXtensible Access Control Markup Language）是 OASIS 标准，既定义了策略语言，也定义了 PAP/PDP/PEP/PIP 架构。Speedle 采用相同的角色划分，但策略语言是 SPDL 而不是 XACML，也不能导入 XACML 策略。

## RBAC：基于角色的访问控制（Role-based access control）

在 RBAC 中，权限授予角色，角色再分配给用户或组。Speedle 用策略把权限授予角色，用角色策略分配角色：

```text
grant role librarian read,update book
grant user alice librarian
```

`alice` 可以 update `book`（reason 0，找到授权策略）；没有角色的 `bob` 被拒绝（reason 3，没有适用的策略）。角色也可以只在某一个资源上分配，参见[策略管理](../pms/policy-mgmt/)和[全局角色策略](../pms/global-policy/)。

## ABAC：基于属性的访问控制（Attribute-based access control）

在 ABAC 中，决策取决于主体、资源、操作或环境的属性。在 Speedle 中，任何策略或角色策略都可以带一个基于属性的 `if` 条件：

```text
grant user bob read report if department == 'finance'
```

`bob` 的请求带字符串属性 `department`，值为 `finance` 时允许（reason 0）；值为 `sales` 或不带该属性时拒绝（reason 3）。参见 [Condition](../spdl/#条件-condition)。

## PBAC：基于策略的访问控制（Policy-based access control）

PBAC 通过集中定义的策略管理访问，而不是把权限写死在各个应用里。Speedle 的所有策略都保存在 PMS 中，并按两条固定规则评估：拒绝优先于授权；没有适用的授权策略时默认拒绝。

```text
grant group staff read,delete journal
deny user carol delete journal
```

`carol` 属于 `staff` 组，但对 `journal` 的 `delete` 被拒绝（reason 1，找到拒绝策略）；同属 `staff` 的 `erin` 被允许（reason 0）。

## 身份与 IAM（Identity and IAM）

身份与访问管理（IAM）既包括证明用户是谁，也包括决定用户能做什么。Speedle 不是身份提供者（IdP），不负责认证用户。它使用身份：主体可以限定在某个[身份域](../idd/)中，身份令牌可以由[令牌断言器](../assertor/)解析。

```text
grant user dave from github read code
```

来自 `github` 身份域的 `dave` 可以 read `code`（reason 0）；来自 `corp` 或不带身份域的 `dave` 被拒绝（reason 3）。

## 零信任（Zero trust）

零信任不因网络位置而授予任何访问，对每一个请求都做检查。Speedle 在其中担任决策服务：ADS 对收到的每个请求做出决策，凡是没有明确授权的都拒绝。认证、设备可信和网络控制不在 Speedle 的范围内。

## SPDL：Speedle Policy Definition Language

SPDL 的全称是 Speedle Policy Definition Language（Speedle 策略定义语言）。它是编写 Speedle 策略和角色策略的语言，例如 `grant role librarian read,update book`。参见 [SPDL 参考](../spdl/)。

## 集成（Integrations）

Speedle 提供 [Docker](../../integrations/docker/)（Docker 授权插件）、[Kubernetes](../../integrations/kubernetes/)（授权 webhook）和 [Istio](../../integrations/istio/)（Mixer 适配器）的集成示例。其他应用可以通过 REST 或 gRPC 调用 ADS，或在 Go 中嵌入求值器，参见[部署](../deployment/)。
