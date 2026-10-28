+++
title = "XACML、SPDL 与授权架构"
description = "对照 XACML（可扩展访问控制标记语言）与 Speedle+：PAP、PDP、PEP 和属性来源、SPDL 语法、拒绝优先及策略迁移边界。"
weight = 60
keywords = ["XACML", "eXtensible Access Control Markup Language", "可扩展访问控制标记语言", "授权", "策略访问控制", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PAP", "policy administration point", "策略管理点", "PDP", "policy decision point", "策略决策点", "PEP", "policy enforcement point", "策略执行点", "PIP", "policy information point", "策略信息点"]
bref = "对应架构概念，区分策略格式与协议兼容性"
+++

## Speedle 与 XACML 有什么关系？

可扩展访问控制标记语言（eXtensible Access Control Markup Language，XACML）是 OASIS 访问控制标准，定义策略及请求、响应等概念。Speedle+ 使用自己的Speedle 策略定义语言（SPDL）与 API。XACML 的行为以 [OASIS XACML 3.0 标准](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html)为准；本文说明架构职责的对应关系，不代表 XACML 标准符合性声明。

## 对应组件职责

| XACML 概念 | 职责 | Speedle+ 中的对应方式 |
| --- | --- | --- |
| PAP：策略管理点 | 管理授权策略 | 策略管理服务（PMS） |
| PDP：策略决策点 | 评估授权请求 | 授权决策服务（ADS）或嵌入式引擎 |
| PEP：策略执行点 | 对实际操作执行决策 | 调用 Speedle 的应用、网关或集成组件 |
| PIP：策略信息点 | 提供决策使用的属性 | 可信应用数据及配置的身份、属性集成；不代表内置 XACML PIP 接口 |

现有[架构文档](/docs/architecture/)已经介绍 PAP/PDP 的对应关系。决策拒绝或评估失败时，执行点仍需阻止实际操作。

## 策略语法与决策语义

SPDL 授权语句指定效果、主体、操作和资源，并可附加条件：

```text
grant user alice read book if clearance >= 3
```

在 [Playground](/playground/) 中，用户为 `alice`、操作为 `read`、资源为 `book`、数值属性 `clearance = 4` 时，该语句允许访问；值为 `2` 时不满足条件。

XACML 策略可以指定组合算法，并使用其他标准构造。Speedle 中，适用的显式拒绝优先于允许。不能假定任意 XACML 组合算法、义务（obligation）、建议（advice）或属性类别映射在转换后仍保持原语义；也不能把 Speedle 评估错误静默视为允许，或直接等同于某个 XACML 响应码。

## 迁移前评估转换

先确认实际使用的策略构造和配置规范，再映射主体及身份域、操作、资源、属性名称与类型，以及决策和错误处理。使用代表性请求比较预期结果，覆盖拒绝和属性缺失。对于不支持的构造，应明确记录，不能在转换时直接丢弃。

本文不提供 XACML XML 策略导入器、XACML 协议端点或符合性认证。设计集成前，请阅读 [SPDL 参考](/docs/spdl/)、[决策 API](/docs/api/decision_api/)、[PBAC 指南](/docs/access-control/pbac/)和 [OES 比较](/docs/access-control/oes/)。
