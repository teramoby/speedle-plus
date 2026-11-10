+++
title = "授权决策 API"
description = "授权决策 API 参考：检查访问权限、查询已授予的角色和权限、发现策略并诊断评估过程。"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-09T21:18:26+08:00
+++

已配置 ADS 的接口参考。请求属性是带类型的数组，诊断响应属性是对象。

请配置[安全连接与端点访问控制](/docs/security/)。本文只说明接口约定，speedle.io 不提供托管授权后端。

{{< swagger-spec url="/swagger/policy-check.zh.yaml" >}}

## 健康检查端点

`GET /healthz` 报告进程存活；进程运行期间始终返回 `200 {"status":"ok"}`，不会重启已停止的实例。

`GET /readyz` 报告实例当前能否响应授权请求：就绪返回 `200`，否则返回 `503`。该端点无需认证，只返回汇总计数，不暴露 service 标识、版本、停止原因或撤销状态。不支持策略分发的存储始终就绪。
