+++
title = "Speedle Cloud"
description = "ホスト型認可、アカウント API キー、リクエスト制限と連携ガイド。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

Speedle Cloud はオープンソースの Speedle エンジンによるホスト型認可サービスです。サービス、SPDL 認可・ロールポリシーを管理し、Playground で判定をテストして、サーバーから API を呼び出せます。

コンソールの利用と API キー作成にはサインインが利用可能である必要があります。無料パブリックベータは試用、学習、評価用で SLA はなく、本番利用は推奨しません。機能と制限は変更される場合があります。

## ドキュメント

- [クイックスタート](/ja/docs/cloud/getting-started/): サインイン、アカウント、サービス、ポリシー、セッション Playground。
- [API キー](/ja/docs/cloud/tokens/): アカウントの認証情報、固定スコープ、作成と失効、一度だけの表示、ページング。
- [認可 API](/ja/docs/cloud/authorization-api/): IsAllowed、ユーザー ID、Node.js 22、通信の制約。
- [制限とエラー](/ja/docs/cloud/limits-and-errors/): アカウント上限、サイズ、バースト、日次枠、エラーコード、リセット時刻。

## ダウンロード

- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): Cloud の 4 章をまとめたオフラインガイド。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

API 仕様はリクエストとレスポンスの契約です。ダウンロードしてもサインインは開始されません。

Cloud の次の文書をご確認ください： [利用規約](https://cloud.speedle.io/ja/terms) / [プライバシーポリシー](https://cloud.speedle.io/ja/privacy).
