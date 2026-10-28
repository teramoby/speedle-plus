+++
title = "使用场景"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "centralized authorization", "集中式授权", "authorization policy management", "授权策略管理", "API authorization", "API 授权", "API 权限控制", "microservices authorization", "微服务授权", "微服务权限控制", "cloud-native authorization", "云原生授权"]
description = "授权集成场景与性能适用范围"
weight = 1
draft = false
toc = true
tocheading = "h2"
tags = ["guide", "usecase"]
categories = ["docs"]
bref = "理解不同应用如何集成策略管理与授权决策"
+++

## 嵌入到 GO 语言编写的应用程序

张先生是一个程序员，他用 GO 语言开发了一套应用系统，现在他希望在这套系统中增加权限控制功能。他可以在程序中直接调用 Speedle 的 GO API 轻松完成任务。

## 利用 Speedle 管理应用系统的权限

H 公司正在开发一套远程医疗系统。健康数据属于个人隐私，根据合规的需要，H 公司必须提供严格的权限控制，确保每位用户只能接触到自己的数据。该系统还需定期接受第三方机构的审计。H 公司使用 Speedle 来达成一下目标：

1. 构建 RBAC/ABAC 兼容的授权模型
2. 自然语言定义的授权策略，方便审计部门分析审计
3. 详尽的审计数据，可以跟踪用户每一次访问

## 利用 Speedle 集中管理组织内所有信息系统

陈先生是某大型跨国公司的信息安全负责人。在过去的十年里该公司飞速发展，并购了数十家中小型公司。这给陈先生和他的团队出了个难题，被并购的公司都有自己的信息系统，实现这些信息系统的技术各不相同。有基于 JavaEE 的，有基于 NodeJS 的，有 Ruby on Rails，还有基于 CORBA 的遗留系统。这些系统的权限管理机制也不一致，有的是硬编码的权限管理模块，有的基于 JaaS，有的基于 OASIS XACML，有些策略模型是 RBAC，有些是 ABAC，有些是普通的 ACL，甚至有些是原始的黑名单白名单。陈先生和他的团队为了管理这些系统不得不学习数十种权限模型和语言，每次修改授权策略需要在数十个系统上做不同的操作。非常繁琐也容易出现错误。

如何解决这个问题？Speedle 能帮上忙。

1. Speedle 可以表达 RBAC、ABAC 和部分 ACL 场景，具体规则应根据所需语义验证。
2. Speedle 提供 REST、gRPC 和 Go API；集成工作包括可信身份/属性映射、错误处理、结果执行与接口安全。
3. 可承载的策略数量取决于策略结构、角色关系、存储与实例资源，应使用实际数据集验证。
4. 延迟与吞吐应以所用版本、接口、硬件、数据集和缓存条件下的测试结果为准，不能据此承诺不会影响业务。
5. 当前 REST 服务默认限流为每秒 1000 请求、突发 200；这不是 Go 嵌入式引擎的性能基准。扩容还需验证存储容量、策略传播与负载分配。

基于以上特性，陈先生可以启动一组 Speedle 服务，把各个系统的权限控制模块指向 Speedle 服务。这样就可以用统一个授权模型和语言来管理这些系统了。
