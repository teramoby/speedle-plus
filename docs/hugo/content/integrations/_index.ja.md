+++
title = "連携"
noindex = false
categories = ["docs"]
description = "Speedle をアプリケーションおよびインフラストラクチャに接続。"
aliases = ["/ja/integrations/istio/", "/ja/integrations/kubernetes/", "/ja/integrations/docker/"]
lastmod = 2026-11-10T14:45:43+08:00
+++

Speedle は認可決定を担当します。アプリケーション、ゲートウェイ、またはプラットフォーム内のポリシー施行ポイント（PEP）が決定ごとに Speedle を呼び出し、結果を適用します。現在、以下のインターフェースがサポートされています。

## REST および gRPC API

ポリシー管理サービス (PMS) を通じてサービス、ポリシー、ロールを管理し、認可決定サービス (ADS) にランタイム決定をリクエストします。どちらも REST API を公開しており、サーバー起動時に `--grpc-endpoint` を指定することで gRPC API を有効にできます。

- [ポリシー管理 API](/ja/docs/api/management_api/)
- [認可決定 API](/ja/docs/api/decision_api/)
- [トークンアサーション Webhook](/ja/docs/api/asserter_api/)
- [API 概要 (PMS および ADS の proto を含む)](/ja/docs/api/)

## 組み込み Go

個別の PMS または ADS プロセスなしで、Go アプリケーション内で直接評価エンジンを実行します。[組み込みモードガイド](/ja/docs/deployment/embedded/)では、Go モジュールを使用した完全なサンプルを構築します。

## SDK

Java、Go、Python、Node.js/TypeScript、Rust、C#/.NET 向けのクライアント SDK が利用可能です。[SDK](/ja/docs/sdks/) を参照してください。

## ポリシーストア

組み込みのファイル、etcd、MySQL、MongoDB ストアを使用するか、別のバックエンド用のストアインターフェースを実装します。[ストレージ拡張](/ja/docs/store/)を参照してください。

## 以前のガイド

以前の Istio、Kubernetes、Docker のサンプルガイドは公開を終了しました。新しい連携には上記のインターフェースを使用してください。
