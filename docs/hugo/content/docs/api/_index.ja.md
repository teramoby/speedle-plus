+++
title = "API"
description = "Open API 仕様ドキュメント"
weight = 300
draft = false
toc = false
tocheading = "h4"
tocsidebar = false
tags = ["v0.1.0", "api", "guide", "core"]
categories = ["docs"]
bref = "Open API 仕様ドキュメント。API を使用して、シンプルおよび複雑なポリシーを宣言的にモデル化し、決定を取得し、ポリシーを動的に検出し、ポリシー評価をデバッグします。"
+++

Java、Python、Go、.NET、Rust、Node.js/TypeScript 向けのクライアントライブラリは [SDK](/ja/docs/sdks/) にあります。

## REST API

これらは **構成された PMS/ADS またはアサーターエンドポイント** の参照仕様です。`speedle.io` はドキュメントとブラウザ Playground を配信しており、ホスト型の PMS/ADS サービスではありません。`Content-Type: application/json` で JSON を提供し、[TLS/mTLS とアクセス制御](/ja/docs/security/) を構成してください。

- [ポリシー管理 API](/ja/docs/api/management_api/)
- [認可決定 API](/ja/docs/api/decision_api/)
- [トークンアサーション Webhook](/ja/docs/api/asserter_api/)

ポリシーおよびロールポリシーの ID は文字列です。PMS のレスポンスはフラットなオブジェクトであり、認可権限には `permissions`（複数形）を使用します。REST の決定リクエストは型付き属性配列を使用しますが、診断レスポンス属性および Webhook 属性はオブジェクトです。

## gRPC API

- [PMS proto](/protobuf/pms.proto): `pb.PolicyManager`
- [ADS proto](/protobuf/ads.proto): `pb.Evaluator`

現在の proto と生成されたクライアントを一緒に使用してください。PMS は `pb.PolicyManager` を使用し、ADS は `pb.Evaluator` を使用します。gRPC はサーバーが `--grpc-endpoint` で起動されない限り無効であり、REST と同じ TLS 設定を使用します。

## 組み込み Go API

- [ポリシー管理インターフェース](https://github.com/teramoby/speedle-plus/tree/master/api/pms)
- [認可決定インターフェース](https://github.com/teramoby/speedle-plus/tree/master/api/ads)

完全なアプリケーションとリソースのライフサイクルの例については、[組み込みモード](/ja/docs/deployment/embedded/) を参照してください。
