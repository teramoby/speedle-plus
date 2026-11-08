+++
title = "关于 Speedle"
icon = "Speedle_logo_about.svg"
+++

Speedle 是开源的细粒度访问控制引擎，为云原生应用处理授权。应用把授权决策交给 Speedle，由它根据 RBAC/ABAC 策略判断已登录用户，或代表用户执行操作的进程，能否访问指定资源。Speedle 自带的策略语言叫 SPDL（Speedle Policy Definition Language，Speedle 策略定义语言）。

Speedle 由 Oracle Entitlements Server（OES）原班团队开发，设计上沿用了团队在 OES、BEA ALES/WLES、CrossLogix 等授权产品中积累的 30 年经验。

Speedle 开源版包含：

- SPDL（Speedle Policy Definition Language，Speedle 策略定义语言）
- 策略管理服务（Policy Management Service）
- 授权决策服务（Authorization Decision Service）
- 策略发现与诊断接口（Policy Discovery and Diagnosis endpoints）
- 命令行工具（spctl）
- 集成示例

Speedle 采用 Universal Permissive License v1.0 许可证，用户可以自由地把 Speedle 集成到自己的系统中。
