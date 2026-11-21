+++
title = "Speedle Cloud"
description = "ホスト型認可、ワークスペース、コンソールマニュアル、API キー、リクエスト制限と連携ガイド。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-21T00:00:00+08:00
+++

Speedle Cloud はオープンソースの Speedle エンジンによるホスト型認可サービスです。ワークスペース、サービス、SPDL 認可・ロールポリシーを管理し、プレイグラウンドで判定をテストして、サーバーから API を呼び出せます。

Speedle Cloud からのポリシー同期には、Speedle Cloud ADS の `speedle-cloud` バイナリとイメージを使用します。セルフホスト型 Speedle Engine の `speedle-ads` バイナリは、`policy-source` ストアを提供しなくなりました。

**Speedle Cloud はパブリックベータを提供中です。** このガイドでは、稼働中のサービスでの手順を説明します。

コンソールの利用と API キーの作成にはサインインが必要です。無料パブリックベータは試用、学習、評価用で SLA はなく、本番利用は推奨しません。機能と制限は変更される場合があります。

## ドキュメント

- [クイックスタート](/ja/docs/cloud/getting-started/)：サインインし、コンソールでサービスとポリシー、API キーを作成して、アプリケーションから認可を確認します。
- [コンソールマニュアル](/ja/docs/cloud/console/)：Speedle Cloud Console（SCC）の実践ガイド：ワークスペース、メンバー、サービスとポリシー、プレイグラウンド、API キー、カスタム関数、ADS フリート、監査ログ、アカウント設定。
- [API キー](/ja/docs/cloud/tokens/): ワークスペースの認証情報、固定スコープ、必須の有効期限、作成と失効、一度だけの表示、ページング。
- [グローバルロール](/ja/docs/cloud/global-roles/): ワークスペース内のすべてのサービスに一度だけロールを割り当て、各サービスで権限を付与します。
- [認可 API](/ja/docs/cloud/authorization-api/): IsAllowed、ユーザー ID、Node.js 22、通信の制約。
- [API リファレンス](/ja/docs/cloud/api-reference/): API key による認可チェックとポリシー管理。リクエストはサーバーから送り、このページは読み取り専用です。
- [制限とエラー](/ja/docs/cloud/limits-and-errors/): ワークスペース上限、サイズ、バースト、日次枠、エラーコード、リセット時刻。

## ダウンロード

- [API リファレンス（Swagger）](/ja/docs/cloud/api-reference/): すべてのエンドポイント、パラメーター、レスポンス例をオンラインで確認できます。
- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): Cloud の 4 章をまとめたオフラインガイド。
- OpenAPI 仕様: [YAML](https://cloud.speedle.io/openapi.public.yaml) · [JSON](https://cloud.speedle.io/openapi.public.json)、コード生成ツールや API クライアント向けです。

Cloud の次の文書をご確認ください： [利用規約](https://cloud.speedle.io/ja/terms) / [プライバシーポリシー](https://cloud.speedle.io/ja/privacy)。
