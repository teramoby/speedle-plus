+++
title = "Policy Management API"
description = "Swagger documentation for the management endpoint. CRUD on services, functions, policies and role-policies. Handle your simple and complex usecases using a simple declarative way!"
tags = ["v0.1.0", "api", "guide", "core", "pms"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

已配置 PMS 的接口参考。请求与响应使用 JSON，策略 ID 为字符串，响应为扁平对象。

请配置[安全连接与端点访问控制](/docs/security/)。这是接口契约，speedle.io 不托管授权后端。

{{< swagger-spec url="/swagger/policy-manage.zh.yaml" >}}
