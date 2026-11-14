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

`store.StoreBuilder` を実装し、`store.Register` で一意の名前を登録して、実行プログラムからパッケージをインポートします。`GetStoreParams` は追加フラグをストアのプロパティに対応付けます。独自フラグは起動前に解析します。ライブラリはフラグ解析やシグナルハンドラーの登録を行いません。

`github.com/teramoby/speedle-plus/pkg/ads` の `ads.Run(ctx, params, conf, options...)` で標準 ADS 起動を再利用します。`params` は `*flags.Parameters`、`conf` は `*cfg.Config` です。`ctx` のキャンセルで両サーバーを停止します。`ads.WithGate(g)` は両プロトコルのゲート検出を上書きし、`ads.WithGate(nil)` は無効にします。`ads.WithListeners(rest, rpc)` はバインド済み `net.Listener` を受け取り、nil は設定済みエンドポイントを使います。Run は起動失敗時も含めてリスナーを所有し、閉じます。REST の TLS 設定と gRPC のサーバー認証情報は維持されます。`ads.WithHTTPServer(func(*http.Server))` でハンドラーをラップし、ルートを追加できます。`ads.WithGRPCOptions(...)` でオプションを追加します。標準の panic 回復を維持するため、追加インターセプターには `grpc.ChainUnaryInterceptor(...)` を使います。

プロトコルに依存しない `pkg/svcs/gate.Gate` は、`Route(serviceName string) gate.Route` で `gate.Local`、`gate.Fallback`、`gate.Unavailable` を選び、`Forward(context.Context, gate.Operation, *ads.RequestContext) (*gate.Response, error)` で型付き結果を返します。Local は通常の評価を続行します。Fallback は許可・拒否、ロール、権限を返せます。失敗時にローカル評価へ切り替わることはありません。Diagnose と Discover は Fallback では拒否されます。`*gate.UnavailableError` の公開理由コード以外のエラーと nil 応答は `fallback_unavailable` になります。REST は 503 と `authorization_unavailable`、理由、`Cache-Control: no-store`、`Retry-After: 30` を返し、gRPC は Unavailable を返します。応答の `ErrorMessage` は評価エラーを表し、REST IsAllowed は通常の 200 エラー本文、他の REST 操作は 500、gRPC は Internal を返します。成功した後備応答には `X-Speedle-Decision-Source: upstream` が付きます（gRPC メタデータのキーは小文字）。任意の `gate.Auditor.Audit(ctx, event)` は非ローカル経路ごとに一度呼ばれ、主体、トークン、リクエスト本文を含みません。標準監査ログも記録されます。ゲートは並行呼び出しに対応し、後備認証とリソースを管理します。

ストアが `gate.GateProvider.RequestGate() gate.Gate` を実装すると、REST と gRPC は評価器のストアアクセサーから検出します。組み込み用途では `eval.NewWithStore` の後に `adsrest.NewRouter(evaluator, adsrest.WithGate(g))` または `adsgrpc.NewGRPCService(evaluator, adsgrpc.WithGate(g))` を使います。オプション省略時はストアから検出し、明示的な nil は無効化します。ゲートなしのローカル動作は変わりません。

`store.WatchRequirement.RequireWatch()` は設定で無効でも購読を強制します。任意の `ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error)` はスナップショットと、インストール成功後にのみ呼ばれる確認コールバックを提供します。`(*eval.PolicyEvalImpl).DistributionStore()` はアダプター向けのストアアクセサーです。評価器を閉じると監視を停止し、閉じられるストアを閉じます。`ads.Run` はこの後始末を行います。明示的に渡したゲートの所有者は呼び出し側です。

キャンセル時は先に gRPC を停止し、HTTP に五秒の猶予を与えてから接続を閉じます。`ads.WithShutdownTimeout(d)` で正の猶予期間を指定できます。独自ハンドラーとゲートはリクエストコンテキストのキャンセルに従ってください。両プロトコルはローカルのトークン検証前に経路を選択し、後備側は元のトークンと主体の認証を担当します。ローカル経路の検証は維持されます。評価エラーでは、応答が許可を主張しても `Allowed` を false、`Reason` を `ERROR_IN_EVALUATION` にし、`evaluation_error` を監査に記録します。

[テスト専用ストアとゲート](https://github.com/teramoby/speedle-plus/blob/master/pkg/ads/extensions_test.go)、[ストアインターフェース](https://github.com/teramoby/speedle-plus/tree/master/pkg/store)、[組み込みデプロイ](/ja/docs/deployment/embedded/)を参照してください。
