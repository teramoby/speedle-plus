+++
title = "決定サービスの拡張"
description = "独自のポリシーストアやサービスアダプターを使用します。"
weight = 310
lastmod = 2026-11-14T18:30:00+08:00
draft = false
toc = false
categories = ["docs"]
bref = "独自のポリシーストアやサービスアダプターを使用します。"
+++

`store.StoreBuilder` を実装し、アプリケーションに必要な `pms.PolicyStoreManager` インターフェースを満たすストアを返します。パッケージの `init` 関数で `store.Register` を呼び、一意の名前で登録して実行プログラムからインポートします。標準プログラムは `cmd/speedle-ads/stores.go` と `cmd/speedle-pms/stores.go` でストアをインポートします。`GetStoreParams` はコマンドラインフラグをストアのプロパティに対応付けます。

組み込みサービスでは、ストアを `eval.NewWithStore` に渡し、得られた評価器を `adsrest.NewRouter` または `adsgrpc.NewGRPCService` に渡します。変更イベントの購読が必要なストアは `store.WatchRequirement` を実装できます。`(*eval.PolicyEvalImpl).DistributionStore()` は外部アダプター用のストアアクセサーとして維持されます。エンジンは独自のゲートウェイインターフェースを解釈しません。ゲートウェイやルーティングは独自の実行プログラムで組み合わせてください。

[ストアのインターフェース](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)と[組み込みデプロイ](/ja/docs/deployment/embedded/)を参照してください。
