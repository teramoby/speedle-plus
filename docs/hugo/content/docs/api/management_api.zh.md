+++
title = "策略管理 API"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PAP", "policy administration point", "策略管理点", "authorization policy management", "授权策略管理", "API authorization", "API 授权", "API 权限控制"]
description = "管理服务、函数、授权策略与角色策略的 JSON API"
tags = ["v0.1.0", "api", "guide", "core", "pms"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

已配置 PMS 的接口参考。请求与响应使用 JSON，策略 ID 为字符串，响应为扁平对象。

请配置[安全连接与端点访问控制](/docs/security/)。这是接口契约，speedle.io 不托管授权后端。

{{< swagger-spec url="/swagger/policy-manage.zh.yaml" >}}
