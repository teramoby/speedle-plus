+++
title = "PBAC：基于策略的访问控制"
description = "通过 Speedle+ 实现基于策略的访问控制（PBAC，policy-based access control）：使用 SPDL 独立授权规则，由 PMS 管理策略、ADS 决策并组合 RBAC 与 ABAC。"
weight = 30
keywords = ["PBAC", "policy-based access control", "基于策略的访问控制", "授权", "Speedle+"]
bref = "分离策略管理、授权决策与访问执行"
+++

## 这里的 PBAC 是什么

基于策略的访问控制（Policy-Based Access Control，PBAC）将授权规则表达为可管理的策略，减少分散在业务代码中的权限判断。Speedle+ 提供安全策略定义语言（SPDL）、策略管理服务（PMS），以及授权决策服务（ADS）或嵌入式评估引擎。

PBAC 描述规则如何表达和管理；[RBAC](/docs/access-control/rbac/)描述角色关系；[ABAC](/docs/access-control/abac/)描述属性条件。同一个 Speedle 策略体系可以同时使用角色与属性。

## 从策略到执行

1. 约定服务、主体名称、资源标识与操作名称。
2. 通过 [PMS](/docs/pms/policy-mgmt/)创建授权策略和角色策略。
3. 确认决策服务已接收到预期的策略版本。
4. 将当前主体、资源、操作及可信属性发送给 [ADS](/docs/decisions/)。
5. 由应用、网关或其他执行点落实决策，并明确处理超时和评估错误。

集中管理并不免除应用执行决策的职责，也不意味着每个引擎都能瞬间收到更新。应验证所选[策略存储](/docs/store/)的更新行为。

## 验证拒绝优先

在 [Playground](/playground/) 中输入：

```text
grant user alice read book
deny user alice read book
```

用户为 `alice`、资源为 `book`、操作为 `read` 时，两条策略都匹配，显式拒绝优先，结果为 Deny。删除第二行后，结果为 Allow；把用户改为 `bob`，没有适用的允许策略，默认拒绝。

## 像审查代码一样审查策略

将预期规则和代表性请求纳入版本控制，检查允许、显式拒绝、未匹配请求、角色范围和属性边界。记录验证的策略版本，以及它如何到达线上决策服务。浏览器本地演示不能证明生产环境的延迟或更新一致性。

[架构](/docs/architecture/)、[SPDL 参考](/docs/spdl/)和 [XACML 比较](/docs/access-control/xacml/)进一步说明管理与决策的边界。
