+++
title = "認可決定 API"
description = "認可決定 API リファレンス: アクセス権限のチェック、付与されたロールと権限の照会、ポリシーの検出と評価プロセスの診断。"
tags = ["v0.1.0", "api", "guide", "core", "ads"]
categories = ["docs"]
customcss = true
customcssfile = "swagger-custom.css"
+++

構成された ADS のインターフェースリファレンス。リクエスト属性は型付き配列、診断レスポンス属性はオブジェクトです。

[セキュリティとエンドポイント構成](/ja/docs/security/)。本文はインターフェース仕様を説明するものであり、speedle.io はホスト型バックエンドを提供しません。

{{< swagger-spec url="/swagger/policy-check.yaml" >}}

## ヘルスチェックエンドポイント

`GET /healthz` はプロセスの稼働状態を報告します。プロセス実行中は常に `200 {"status":"ok"}` を返します。

`GET /readyz` はインスタンスが認可リクエストに応答可能かどうかを報告します。準備完了の場合は `200`、それ以外は `503` を返します。このエンドポイントは認証を必要とせず、集計カウントのみを返します。
