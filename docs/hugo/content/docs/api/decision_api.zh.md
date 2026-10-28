+++
title = "授权决策 API"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PDP", "policy decision point", "策略决策点", "authorization decision service", "授权决策服务", "API authorization", "API 授权", "API 权限控制"]
description = "授权决策、角色权限查询、发现与诊断接口"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

已配置 ADS 的接口参考。请求属性是带类型的数组，诊断响应属性是对象。

请配置[安全连接与端点访问控制](/docs/security/)。这是接口契约，speedle.io 不托管授权后端。

{{< swagger-spec url="/swagger/policy-check.zh.yaml" >}}
