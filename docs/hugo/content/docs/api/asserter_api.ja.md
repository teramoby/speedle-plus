+++
title = "トークンアサーション API"
description = "トークンアサーション Webhook を実装し、トークンを検証して認可リクエストにサブジェクト ID を提供。"
tags = ["v0.1.0", "api", "guide","core","asserter","pluggable"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
lastmod = 2026-11-12T10:00:00+08:00
+++

トークンアサーション Webhook リファレンス。errCode は整数、attributes はオブジェクトです。実際のエンドポイントは ADS アサーター構成で指定されます。

[セキュリティとエンドポイント構成](/ja/docs/security/)。本文はインターフェース仕様を説明するものであり、speedle.io はホスト型バックエンドを提供しません。Try it out は無効です。仕様にサーバーホストがないため、有効にするとリクエストはこのドキュメントサイトへ送られます。

{{< swagger-spec url="/swagger/asserter.yaml" tryitout="false" >}}
