+++
title = "授权决策 API"
description = "授权决策 API 参考：检查访问权限、查询已授予的角色和权限、发现策略并诊断评估过程。"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

已配置 ADS 的接口参考。请求属性是带类型的数组，诊断响应属性是对象。

请配置[安全连接与端点访问控制](/docs/security/)。本文只说明接口约定，speedle.io 不提供托管授权后端。

{{< swagger-spec url="/swagger/policy-check.zh.yaml" >}}
