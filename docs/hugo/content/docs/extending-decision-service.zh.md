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

实现 `store.StoreBuilder`，用 `store.Register` 注册唯一名称，并在自己的可执行程序中导入该包。`GetStoreParams` 将附加命令行参数映射到存储属性。先解析自己的参数，再启动服务；库不解析参数，也不安装信号处理器。

从 `github.com/teramoby/speedle-plus/pkg/ads` 调用 `ads.Run(ctx, params, conf, options...)`，复用标准 ADS 启动流程。`params` 为 `*flags.Parameters`，`conf` 为 `*cfg.Config`；取消 `ctx` 会停止两个服务。`ads.WithGate(g)` 覆盖两个协议的门控发现；`ads.WithGate(nil)` 禁用门控。`ads.WithListeners(rest, rpc)` 接受预先绑定的 `net.Listener`（nil 使用配置的端点）。Run 接管并关闭这些监听器，启动失败时也会关闭。REST 保留配置的 TLS，gRPC 保留服务端凭证。`ads.WithHTTPServer(func(*http.Server))` 可在服务启动前包装处理器或添加路由。`ads.WithGRPCOptions(...)` 添加服务端选项；附加拦截器请用 `grpc.ChainUnaryInterceptor(...)`，以保留标准的 panic 恢复拦截器。

与协议无关的 `pkg/svcs/gate.Gate` 有两个方法：`Route(serviceName string) gate.Route` 选择 `gate.Local`、`gate.Fallback` 或 `gate.Unavailable`；`Forward(context.Context, gate.Operation, *ads.RequestContext) (*gate.Response, error)` 提供有类型的后备结果。Local 继续本地求值。Fallback 可返回允许、拒绝、角色或权限；失败时绝不转回本地求值。Diagnose 和 Discover 在选择 Fallback 时拒绝处理。`*gate.UnavailableError` 携带公开原因码；其他错误及 nil 响应映射为 `fallback_unavailable`。REST 返回 503，包含 `authorization_unavailable`、原因、`Cache-Control: no-store` 和 `Retry-After: 30`；gRPC 返回 Unavailable。响应的 `ErrorMessage` 表示求值错误：REST IsAllowed 返回正常的 200 错误体，其他 REST 操作返回 500，gRPC 返回 Internal。后备成功响应携带 `X-Speedle-Decision-Source: upstream`（gRPC 元数据使用小写键）。可选的 `gate.Auditor.Audit(ctx, event)` 对每个非本地路由接收一次事件，不包含主体、令牌或请求体；服务仍写入标准审计日志。门控实现须支持并发请求，自行管理后备认证和资源。

`gate.Response.Metadata`（`map[string][]string`）可为成功的后备决策提供响应元数据，包括拒绝结果和角色/权限列表。REST 将允许的值复制为响应头；gRPC 将相同键转为小写，作为响应头元数据发送，保留重复值。两个协议均过滤逐跳头（包括 `Connection` 列出的名称）、`Set-Cookie`、`Content-Length`、`Content-Type`，以及服务自行设置的头，例如 `Cache-Control`、`Retry-After`、`X-Speedle-Decision-Source`、`Speedle-Principals`、`WWW-Authenticate`、`Date` 和 `Server`。保留的 `grpc-*` 键、二进制 `*-bin` 键、无效文本键及不可打印或非 ASCII 值也被过滤。REST 保留中间件已设置的响应头。元数据为空时行为不变；求值错误和转发失败不复制元数据。请求处理期间，复用的映射须保持不可变。

存储可实现 `gate.GateProvider.RequestGate() gate.Gate`，REST 和 gRPC 会通过求值器的底层存储访问器发现它。嵌入式调用者可用 `eval.NewWithStore`，再调用 `adsrest.NewRouter(evaluator, adsrest.WithGate(g))` 或 `adsgrpc.NewGRPCService(evaluator, adsgrpc.WithGate(g))`。省略选项会发现存储提供的门控；显式 nil 禁用门控。没有门控时，本地行为不变。

明确存储生命周期：`store.WatchRequirement.RequireWatch()` 即使在配置关闭监听时也会强制订阅。可选的 `ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error)` 提供快照和确认回调，只有安装成功后才调用回调。`(*eval.PolicyEvalImpl).DistributionStore()` 为适配器提供底层存储。关闭求值器会停止监听并关闭可关闭的存储；`ads.Run` 负责此清理。显式传入的门控由调用者管理。

取消上下文时先停止 gRPC，再给 HTTP 请求五秒宽限期，之后关闭活动连接。`ads.WithShutdownTimeout(d)` 可设置正的宽限期；自定义处理器和门控应响应请求上下文的取消。两个协议均在本地令牌断言之前分流：后备收到原始令牌和主体信息，自行负责认证；本地路径保留正常断言。求值错误始终将 `Allowed` 清为 false，`Reason` 设为 `ERROR_IN_EVALUATION`，并审计 `evaluation_error`，即使后备响应声称允许。

参见[仅供测试的外部存储与门控示例](https://github.com/teramoby/speedle-plus/blob/master/pkg/ads/extensions_test.go)、[存储接口](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)和[嵌入式部署](/zh/docs/deployment/embedded/)。
