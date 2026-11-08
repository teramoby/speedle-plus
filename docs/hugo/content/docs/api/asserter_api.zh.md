+++
title = "令牌断言 API"
description = "实现令牌断言 webhook，验证令牌并向授权请求提供主体身份。"
tags = ["v0.1.0", "api", "guide","core","asserter","pluggable"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

令牌断言 webhook 参考。errCode 为整数，attributes 为对象。实际端点由 ADS 断言配置指定。

请配置[安全连接与端点访问控制](/docs/security/)。本文只说明接口约定，speedle.io 不提供托管授权后端。

{{< swagger-spec url="/swagger/asserter.zh.yaml" >}}
