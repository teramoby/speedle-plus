+++
title = "Speedle Cloud"
description = "ホスト型認可、アカウント API キー、リクエスト制限と連携ガイド。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T21:00:00+08:00
+++

Speedle Cloud はオープンソースの Speedle エンジンによるホスト型認可サービスです。サービス、SPDL 認可・ロールポリシーを管理し、Playground で判定をテストして、サーバーから API を呼び出せます。

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

コンソールの利用と API キー作成にはサインインが利用可能である必要があります。無料パブリックベータは試用、学習、評価用で SLA はなく、本番利用は推奨しません。機能と制限は変更される場合があります。

## ドキュメント

- [クイックスタート](/ja/docs/cloud/getting-started/)：サインインし、コンソールでサービスとポリシー、API キーを作成して、アプリケーションから認可を確認します。
- [API キー](/ja/docs/cloud/tokens/): アカウントの認証情報、固定スコープ、作成と失効、一度だけの表示、ページング。
- [認可 API](/ja/docs/cloud/authorization-api/): IsAllowed、ユーザー ID、Node.js 22、通信の制約。
- [API リファレンス](/ja/docs/cloud/api-reference/): API key による認可チェックとポリシー管理。リクエストはサーバーから送り、このページは読み取り専用です。
- [制限とエラー](/ja/docs/cloud/limits-and-errors/): アカウント上限、サイズ、バースト、日次枠、エラーコード、リセット時刻。

## ダウンロード

- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): Cloud の 4 章をまとめたオフラインガイド。
- [API リファレンス](/ja/docs/cloud/api-reference/): このサイトで読めます。下の生ファイルも引き続き利用できます。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

API 仕様はリクエストとレスポンスの契約です。ダウンロードしてもサインインは開始されません。

Cloud の次の文書をご確認ください： [利用規約](https://cloud.speedle.io/ja/terms) / [プライバシーポリシー](https://cloud.speedle.io/ja/privacy).
