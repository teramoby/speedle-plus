+++
title = "認可診断"
description = "認可評価を診断し、決定理由、有効なロール、評価されたポリシーを確認"
date = 2019-01-18T15:46:59+08:00
weight = 3
draft = false
bref = "認可評価プロセスを診断します。ユーザーによるリソースの操作が許可されない場合に、その理由やどのポリシーが操作を拒否したかを調査するのに役立ちます。"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## 認可診断の役割

`Diagnose` は認可評価を詳細に説明します。決定結果、理由、有効なロール、属性、および検証されたポリシーが含まれます。`IsAllowed` と同じリクエストコンテキストを受け取ります。REST では `/authz-check/v1/diagnose` を使用し、Go/gRPC では `Diagnose` を使用します。

診断機能はポリシー定義とリクエストコンテキストをそのまま開示します。[API セキュリティ制御](/ja/docs/security/) を使用して信頼できるオペレーターにのみ制限してください。パブリックなデバッグエンドポイントではありません。

## サンプル実行前の準備

[クイックスタート](/ja/quick-start/) の手順に従い、リポジトリルート下の `./bin/spctl`、`./bin/speedle-pms`、`./bin/speedle-ads` をビルドします。Go 1.26.10 または Go 1.27.3（または同じリリースブランチの新しいパッチバージョン）、Bash、curl、Python 3 が必要です。各ターミナルはリポジトリのルートから実行してください。このページ用に新しいポリシーファイルを使用します。両方のサービスを停止した後にのみファイルをリセットしてください。

空のポリシーファイルを初期化します:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

ターミナル 1 で PMS を起動し、実行したままにします:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

ターミナル 2 で ADS を起動し、実行したままにします:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

残りのコマンドは 3 つ目のターミナルで実行します。PMS と ADS は同じファイルを共有し、変更は watch を通じて非同期に伝播します。新しいポリシーに依存する前に、更新が反映されていることを確認してください。`--insecure=true` はローカルのループバック用です。本番環境の接続については [セキュリティ](/ja/docs/security/) を参照してください。明示的な PMS アドレスの指定により、`~/.spctlconfig` を変更する必要はありません。完了したら、両方のサーバーターミナルで Ctrl-C を押して終了します。

## サンプルポリシーの作成

この [フィクスチャ](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/diagnosis.json) は `^/api/v1/example/.*$` 配下の `get` を許可しますが、`user1` に対して `/api/v1/example/res1` を明示的に拒否します。

<!-- docs-check: diagnosis-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/diagnosis.json
```

## 認可決定と診断結果の比較

<!-- docs-check: diagnosis-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

期待値: `allowed=false`、`reason=1`（明示的拒否）。

<!-- docs-check: diagnose -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/diagnose
```

診断レスポンスでは、`allowed` はブール値、`reason` は `DENY_POLICY_FOUND` などの文字列、`attributes` はオブジェクトとして表現されます。`request_month` は数値の月です。`requestContext.subject.principals` は配列です。認可ポリシーには、OR-of-AND プリンシパルグループを表す 2 次元の `principals` 配列が含まれます。

| フィールド | 意味 |
| --- | --- |
| `requestContext` | 評価に使用されたリクエスト。REST 属性はここでは型付き配列のままです |
| `attributes` | 利用可能なリクエスト属性および組み込み属性（名前をキーとする） |
| `grantedRoles` | 有効なロール（存在する場合） |
| `rolePolicies`, `policies` | 評価されたロールおよび認可ポリシーの詳細（存在する場合） |
| `condition` | 式と結果。無条件ポリシーの場合は空になることがあります |

ポリシーのステータス値は `takeEffect`、`conditionFailed`、`ignored` です。`ignored` は最終結果がすでに確定していたため、そのポリシーの評価が不要だったことを意味します。順序は異なる場合があるため、過去のレスポンスと完全に一致させるのではなく、ID とステータスを確認してください。完全なレスポンスコントラクトについては [決定スキーマ](/ja/docs/api/decision_api/) を参照してください。
