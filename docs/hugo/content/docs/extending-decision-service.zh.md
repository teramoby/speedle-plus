+++
title = "扩展决策服务"
description = "使用自定义策略存储或服务适配器。"
weight = 310
lastmod = 2026-11-14T18:30:00+08:00
draft = false
toc = false
categories = ["docs"]
bref = "使用自定义策略存储或服务适配器。"
+++

实现 `store.StoreBuilder`，并返回满足应用所需 `pms.PolicyStoreManager` 接口的存储。在包的 `init` 函数中调用 `store.Register`，使用唯一名称注册构建器，再在可执行程序中导入该包。标准程序通过 `cmd/speedle-ads/stores.go` 和 `cmd/speedle-pms/stores.go` 导入存储。`GetStoreParams` 将命令行参数映射到存储属性。

嵌入式服务可将存储传给 `eval.NewWithStore`，再将返回的求值器传给 `adsrest.NewRouter` 或 `adsgrpc.NewGRPCService`。必须订阅变更事件的存储可实现 `store.WatchRequirement`。`(*eval.PolicyEvalImpl).DistributionStore()` 保留为供外部适配器访问底层存储的接口；引擎不解释自定义网关接口。请在自己的可执行程序中组合网关或路由。

参见[存储接口](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)和[嵌入式部署](/zh/docs/deployment/embedded/)。
