+++
title = "Oracle Entitlements Server（OES）与 Speedle"
description = "比较 Oracle Entitlements Server（OES，Oracle 权限管理服务器）与 Speedle+ 的授权架构、策略管理及执行方式，并通过检查清单评估迁移。"
weight = 50
keywords = ["OES", "Oracle Entitlements Server", "Oracle 权限管理服务器", "授权", "访问控制", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "centralized authorization", "集中式授权", "authorization policy management", "授权策略管理"]
bref = "评估迁移前，先比较授权体系中各组件的职责"
+++

## 为什么比较 OES 与 Speedle？

Oracle Entitlements Server（OES，Oracle 权限管理服务器）与 Speedle+ 都面向外部化的细粒度授权。[Oracle 的 OES 架构文档](https://docs.oracle.com/cd/E27559_01/admin.1112/e27153/intro.htm)介绍集中式策略管理，以及评估和执行授权策略的安全模块。Speedle 则通过自己的服务与 API 提供策略管理和授权决策。

在审查既有 OES 集成或评估开源授权引擎时，可以比较二者的职责。职责相近并不意味着能够直接替换。应核对实际使用的 OES 版本文档；本文不宣称兼容 OES 策略导出格式、API 或安全模块。

## 比较组件职责

| 关注点 | OES 概念 | Speedle+ 方式 |
| --- | --- | --- |
| 策略管理 | 集中式策略管理 | 策略管理服务（PMS） |
| 授权评估 | 安全模块及其决策集成 | 授权决策服务（ADS）或嵌入式 Go 引擎 |
| 应用执行 | 与 OES 安全模块集成 | 应用或网关执行 Speedle 决策 |
| 策略表达 | OES 策略模型及版本相关格式 | SPDL 与 Speedle 管理 API 数据模型 |
| 身份上下文 | 配置的身份与属性集成 | 显式主体、身份域、属性和已配置的令牌断言服务 |

实际组件边界见 [Speedle 架构](/docs/architecture/)。表格对照的是职责，不代表二进制或通信协议兼容。

## 用实际请求评估迁移

1. 盘点应用使用的 OES 策略、角色、资源匹配、身份来源、自定义函数及执行集成。
2. 记录代表性请求与预期决策，覆盖显式拒绝、无匹配策略、属性失败和角色继承。
3. 使用 [SPDL](/docs/spdl/)表达规则，识别需要修改应用或不能直接表示的语义。
4. 在 [Playground](/playground/) 支持的范围内测试策略。令牌验证和外部函数需要服务端集成测试。
5. 切换执行路径前，用同一份可信上下文比较两套引擎，制定回滚方案并验证策略更新行为。

本文不提供自动 OES 导入器，也不承诺直接替换。Speedle 是否适合，取决于应用需要的语义和集成方式，而不是功能名称是否相同。

继续阅读 [RBAC](/docs/access-control/rbac/)、[ABAC](/docs/access-control/abac/)、[身份与 IAM](/docs/access-control/identity-iam/)以及 [XACML 概念](/docs/access-control/xacml/)。
