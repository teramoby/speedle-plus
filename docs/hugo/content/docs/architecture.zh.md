+++
title = "系统架构"
description = "了解 Speedle 的策略管理服务、策略存储和授权决策服务，以及各组件如何协作。"
weight = 10
draft = false
toc = false
tocheading = "h2"
tocsidebar = false
bref = "Speedle 架构与组件协作流程"
lastmod = 2026-11-08T11:21:16+08:00
+++

<a id="modules"></a>
## 核心组件

Speedle 授权引擎由以下组件组成：

- **策略管理服务（PMS）API**：管理授权策略、角色策略及其依赖的对象。

- **策略存储**：保存所有策略数据，可以使用 JSON 文件，也可以使用数据库或 etcd 等持久化存储。

- **授权决策服务（ADS）API**：根据适用的策略评估授权请求，返回 GRANT 或 DENY。

下图展示了这些组件如何协作。

<img src="/img/speedle/spdlarch.jpg" width="524" height="241" loading="lazy" />

1. 用户通过策略管理服务 API 创建和管理策略。  
2. 策略管理服务把策略写入策略存储。  
3. 策略从存储同步到授权决策服务。  
4. 用户系统调用授权决策服务 API，检查请求是否允许访问。

熟悉 XACML 的话，可以把策略管理服务（PMS）看作策略管理点（PAP），把授权决策服务（ADS）看作策略决策点（PDP）。
