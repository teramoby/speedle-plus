+++
title = "身份与 IAM 集成"
description = "Speedle+ 在身份与访问管理（IAM，Identity and Access Management）中的职责：可信身份、身份域、令牌断言与授权访问控制。"
weight = 40
keywords = ["identity", "IAM", "identity and access management", "身份", "身份与访问管理", "授权", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "zero trust", "零信任"]
bref = "衔接身份与授权，并明确各组件的职责"
+++

## Speedle 在 IAM 中的位置

身份与访问管理（Identity and Access Management，IAM）涉及身份建立、用户认证、账号生命周期和访问控制。Speedle+ 提供其中的授权引擎：判断已识别的用户、组或服务可以执行哪些操作。它不提供完整的身份目录、登录服务或账号开通系统。

| 职责 | 通常由谁负责 |
| --- | --- |
| 账号、凭据与登录 | 身份提供方与目录服务 |
| 可信主体名称、组及属性 | 应用集成组件或令牌断言服务 |
| 授权策略与角色策略 | Speedle 策略管理服务 |
| 访问决策 | Speedle 授权决策服务或嵌入式引擎 |
| 对实际资源执行决策 | 应用或网关 |

## 提供可信身份

可信应用可以在决策请求中直接提供主体身份。另一种方式是配置[令牌断言服务](/docs/assertor/)，验证并解析令牌，返回策略评估使用的身份。断言服务应针对实际身份提供方实现和配置；存在 webhook 接口不等于自动兼容所有令牌格式或协议。

不能让不可信调用者仅凭填写他人用户名就能冒充身份。应认证调用方，从可信来源获取身份和属性，并按[安全文档](/docs/security/)保护决策服务与管理服务。

## 区分身份域

不同身份域中可能存在同名用户。在 [Playground](/playground/) 中尝试：

```text
grant user alice from workforce read book
```

用户填 `alice`，身份域填 `workforce`，资源填 `book`，操作填 `read`，结果为允许；只把身份域改为 `partner`，结果为拒绝。身份域字符串是请求中的显式命名空间，本身不能证明已经完成身份认证。

对应的 Playground 请求为：

```json
{
  "subject": {"principals": [{"type": "user", "name": "alice", "idd": "workforce"}]},
  "resource": "book",
  "action": "read"
}
```

从策略中删去 `from workforce` 后，该用户名将可以跨身份域匹配。多租户集成时应仔细审查这一选择。浏览器演示允许直接填写身份进行实验，不会认证用户或调用令牌断言服务。

实现细节见[身份域](/docs/idd/)、[ABAC 属性](/docs/access-control/abac/)与[令牌断言 API](/docs/api/asserter_api/)。
