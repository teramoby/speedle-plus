+++
title = "Speedle Cloud"
description = "Speedle Cloud ドキュメント: クイックスタート、アカウントとポリシーの管理、アプリケーショントークン、認可 API プレビュー、制限、エラー、ダウンロード。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T20:00:00+08:00
+++

Speedle Cloud は、オープンソースの Speedle ポリシーエンジンをベースにした開発者向けのホスト型アクセス制御サービスです。SPDL 認可ポリシーおよびロールポリシーの管理、Playground での決定テスト、アプリケーションサーバーからの認可 API 呼び出しを、独自の認可サービスを運用することなく行えるよう設計されています。

**Speedle Cloud はまもなく公開されます。** Cloud ヘッダーには **Coming soon（まもなく公開）** バッジが表示されており、[cloud.speedle.io](https://cloud.speedle.io/ja/) での本番サインインはまだ開始されていません。ここで説明されているコンソールワークフローおよび認証付き認可決定サービス (ADS) は、公開されているサービス契約のプレビューです。認証付き ADS が保留中である間、トークンを発行してもサーバー側の認可呼び出しは有効になりません。選択したステージでのデプロイメントと ID プロバイダーの可用性を確認してください。

無料パブリックベータは、試用、学習、評価を目的としています。SLA は提供されず、本番環境での使用は推奨されません。また、機能や制限は変更される場合があります。

## ドキュメント

- [クイックスタート](/ja/docs/cloud/getting-started/): GitHub または Google でのサインイン（まもなく開始）、アカウント管理、サービス、SPDL ポリシー、セッション管理リクエスト、および 4 つのセッション Playground 操作。
- [アプリケーショントークン](/ja/docs/cloud/tokens/): 最近の認証要件、1 回限りのトークン開示、メタデータのページネーション、ローテーション、および失効。
- [認可 API プレビュー](/ja/docs/cloud/authorization-api/): IsAllowed リクエストとレスポンスの契約、Node.js 22 のサンプル、ポリシースナップショット、および統合の制約。
- [API リファレンス](/ja/docs/cloud/api-reference/): API key による認可チェックとポリシー管理。リクエストはサーバーから送り、このページは読み取り専用です。
- [制限とエラー](/ja/docs/cloud/limits-and-errors/): アカウントクォータ、リクエスト制限、エラーレスポンス、レートおよびクォータヘッダー、再試行のガイダンス。

## ダウンロード

- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): 4 つの Cloud の章をまとめたオフラインドキュメント。
- [API リファレンス](/ja/docs/cloud/api-reference/): このサイトで読めます。下の生ファイルも引き続き利用できます。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): リポジトリにチェックインされた正規の Cloud API 仕様。
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): 同じ YAML ソースバイトから生成。
- [提供ビルドバージョン](https://cloud.speedle.io/build.json): サービスを提供しているビルドを識別（ID プロバイダーやエンドポイントの可用性を証明するものではありません）。

仕様には 4 つのセッション Playground 操作と認証付き ADS 契約が含まれます。仕様の公開やガイドのダウンロードを行っても、保留中のエンドポイントが有効になったりサインインが開始されたりすることはありません。YAML および生成された JSON は、対応する封印された Cloud ビルドとともにバージョン管理、プロモート、復元されます。日本語、英語、中国語の Markdown ダウンロードは、これらの Engine ドキュメントページとともに保守されます。

Cloud の[利用規約](https://cloud.speedle.io/ja/terms)および[プライバシーポリシー](https://cloud.speedle.io/ja/privacy)をお読みください。
