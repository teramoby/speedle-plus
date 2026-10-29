+++
title = "Token Assertion Plugin API"
description = "Use the asserter interface to build your own token asserter for token assertion and verification for your decision payload"
tags = ["v0.1.0", "api", "guide","core","asserter","pluggable"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

令牌断言 webhook 参考。errCode 为整数，attributes 为对象。实际端点由 ADS 断言配置指定。

请配置[安全连接与端点访问控制](/docs/security/)。这是接口契约，speedle.io 不托管授权后端。

{{< swagger-spec url="/swagger/asserter.zh.yaml" >}}
