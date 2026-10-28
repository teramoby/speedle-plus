+++
title = "ABAC：基于属性的访问控制"
description = "使用 Speedle+ 实现基于属性的访问控制（ABAC，attribute-based access control）：SPDL 条件、属性类型、可信请求上下文与密级示例。"
weight = 20
keywords = ["ABAC", "attribute-based access control", "基于属性的访问控制", "授权", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化"]
bref = "使用可信属性表达带条件的授权规则"
+++

## Speedle 如何实现 ABAC

基于属性的访问控制（Attribute-Based Access Control，ABAC）使用主体、资源或请求环境的属性判断访问权限。在 Speedle 中，SPDL 条件会结合身份、操作和资源评估运行时属性。应用或可信集成组件需要收集这些属性，并约定一致的名称与类型。

例如，角色确定 Alice 是读者，密级属性进一步决定她能否阅读某类信息。角色分配部分见 [RBAC](/docs/access-control/rbac/)。

## 验证密级条件

将下面的策略粘贴到 [Playground](/playground/)：

```text
grant user alice read book if clearance >= 3
```

用户填 `alice`，资源填 `book`，操作填 `read`。添加名为 `clearance` 的属性，选择数值类型，值填 `4`，结果为允许；将值改为 `2`，结果为拒绝。字符串 `4` 与数值 `4` 并不相同，应使用条件所要求的类型。

对应的 Playground 请求为：

```json
{
  "subject": {"principals": [{"type": "user", "name": "alice"}]},
  "resource": "book",
  "action": "read",
  "attributes": [{"name": "clearance", "type": "numeric", "value": 4}]
}
```

Playground 使用带类型的属性数组。对已部署 ADS 构造请求时，应以[决策 API 参考](/docs/api/decision_api/)为准，浏览器适配层不能替代服务 API 契约。

## 确保属性可信

密级、租户归属等属性应来自可信服务，而不是任由客户端填写。资源分类也需要显式映射到请求中，Speedle 不会自动读取应用数据库字段。不同身份域中的同名用户应通过[身份域](/docs/idd/)区分。

属性缺失可能引发评估错误。应用应区分错误与正常的拒绝响应，并在出错时拒绝访问。除了允许情况，还应测试缺失属性、类型错误和边界值。外部自定义函数需要服务端集成，浏览器 Playground 不提供这一能力。

继续阅读 [SPDL 条件](/docs/spdl/)、[授权决策](/docs/decisions/)和[身份与 IAM 集成](/docs/access-control/identity-iam/)。
