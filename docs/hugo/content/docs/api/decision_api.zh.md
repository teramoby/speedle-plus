+++
title = "授权决策 API"
description = "授权决策 API 参考：检查访问权限、查询已授予的角色和权限、发现策略并诊断评估过程。"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

已配置 ADS 的接口参考。请求属性是带类型的数组，诊断响应属性是对象。

请配置[安全连接与端点访问控制](/docs/security/)。本文只说明接口约定，speedle.io 不提供托管授权后端。试用已关闭：规范未写服务器主机，否则请求会发到本说明站点。

{{< swagger-spec url="/swagger/policy-check.zh.yaml" tryitout="false" >}}

## 健康检查端点

`GET /healthz` 报告进程存活；进程运行期间始终返回 `200 {"status":"ok"}`。

`GET /readyz` 无需认证，本地决策服务返回 `200 {"ready":true,"servicesTotal":0,"servicesUsable":0}`。两个计数始终为零，该探针不枚举存储中的服务。
