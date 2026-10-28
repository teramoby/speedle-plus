+++
title = "关于 Speedle"
icon = "Speedle_logo_about.svg"
+++

Speedle 是一个开源的细粒度访问控制引擎，用于满足云原生应用的授权需求。Speedle 提供策略驱动的访问控制，把授权决策从应用中剥离出来，交给策略引擎处理。策略引擎可以针对已登录的用户，或代表用户执行操作的进程，评估 RBAC/ABAC 策略。Speedle 包含 SPDL，即 Speedle Policy Definition Language（Speedle 策略定义语言）。

Speedle 由开发 Oracle Entitlements Server（OES）的同一团队打造，其设计建立在该团队在 OES、BEA ALES/WLES、CrossLogix 等前序授权产品上 30 年产品迭代的经验之上。

Speedle 开源版本包括：

- SPDL（Speedle Policy Definition Language，Speedle 策略定义语言）
- 策略管理服务（Policy Management Service）
- 授权决策服务（Authorization Decision Service）
- 策略发现与调试接口（Policy Discovery and Debug endpoints）
- 命令行工具（spctl）
- 集成示例

Speedle 采用 Universal Permissive License v1.0 许可证，用户可以自由地把 Speedle 集成到自己的系统中。
