+++
title = "策略管理 API"
description = "策略管理 API 参考：管理服务、函数、授权策略和角色策略。"
tags = ["v0.1.0", "api", "guide", "core", "pms"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

已配置 PMS 的接口参考。请求与响应使用 JSON，策略 ID 为字符串，响应为扁平对象。

请配置[安全连接与端点访问控制](/docs/security/)。本文只说明接口约定，speedle.io 不提供托管授权后端。

{{< swagger-spec url="/swagger/policy-manage.zh.yaml" >}}
