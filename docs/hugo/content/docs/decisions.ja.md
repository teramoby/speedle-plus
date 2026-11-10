+++
title = "認可決定"
description = "サービスアクセスの認可決定を取得し、付与されたロールと権限を照会。"
weight = 30
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pdp", "policy", "core"]
categories = ["docs"]
bref = "認可決定を取得"
lastmod = 2026-11-08T19:23:52+08:00
+++

## 認可決定とは？

Speedle は指定されたサービス内でサブジェクト、アクション、およびリソースを評価し、アプリケーションがその結果を適用します。評価エラーやタイムアウトは通常の Deny（拒否）とは区別して扱い、エラー時はアクセスを拒否（fail closed）してください。REST、gRPC、組み込み Go の各 API はデータ表現が異なるため、それぞれの [API 契約](/ja/docs/api/) に従ってください。

## 実行前の準備

[クイックスタート](/ja/quick-start/) に従ってリポジトリルートで `./bin/spctl`、`./bin/speedle-pms`、`./bin/speedle-ads` をビルドします。Go 1.26.10 または Go 1.27.3（または同じリリースブランチの新しいパッチバージョン）、Bash、curl、Python 3 が必要です。各ターミナルはリポジトリのルートディレクトリから実行してください。このページ用には新しいポリシーファイルを使用し、両方のサービスを停止した後にのみリセットしてください。

空のポリシーファイルを初期化します:

<!-- docs-check: setup -->
```bash
printf '{}
' > docs-policies.json
```

ターミナル1で PMS を起動し、実行したままにします:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

ターミナル2で ADS を起動し、実行したままにします:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

残りのコマンドはターミナル3で実行します。PMS と ADS はファイルを共有し、変更は非同期に反映されます。意図した更新が ADS に到達したことを確認してから依存してください。`--insecure=true` はこのループバックサンプルのためのものです。本番接続については [セキュリティ](/ja/docs/security/) を参照してください。明示的な PMS アドレスを指定することで `~/.spctlconfig` の変更を回避できます。終了したら、両方のサーバーターミナルで Ctrl-C を押して停止します。

## 再現可能なポリシーセットの作成

[フィクスチャ](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/bookstore.json) は Alan に `role1` と `role2` を割り当て、`role1` は `/books/HarryPotter` のダウンロードが許可されます。

<!-- docs-check: bookstore-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/bookstore.json
```

## 認可決定の取得

Alan は許可されます。Bob は適用される認可がないため拒否されます。

<!-- docs-check: decision-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json'   -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}'   http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: decision-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json'   -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}'   http://127.0.0.1:6734/authz-check/v1/is-allowed
```

成功した決定は `{"allowed":true,"reason":0}` または対応する Deny オブジェクトを返します。HTTP エラーは成功した認可ではなくエラーレスポンスを返します。理由コードは以下のとおりです:

| コード | 意味 |
| --- | --- |
| 0 | GRANT_POLICY_FOUND |
| 1 | DENY_POLICY_FOUND |
| 2 | SERVICE_NOT_FOUND |
| 3 | NO_APPLICABLE_POLICIES |
| 4 | ERROR_IN_EVALUATION |
| 5 | DISCOVER_MODE |
| 6 | REASON_NOT_AVAILABLE |

`DISCOVER_MODE` は本番環境の認可決定ではありません。`REASON_NOT_AVAILABLE` はフォールバック理由値です。これらの数値を HTTP ステータスコードと混同しないでください。

## 付与されたロールの取得

<!-- docs-check: decision-roles -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json'   -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}'   http://127.0.0.1:6734/authz-check/v1/all-granted-roles
```

期待されるロールは `role1` と `role2` です（順序は関係ありません）。リソーススコープのロール割り当てを照会する場合は `resource` を含めてください。

## 付与された権限の取得

<!-- docs-check: decision-permissions -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json'   -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}'   http://127.0.0.1:6734/authz-check/v1/all-granted-permissions
```

期待される権限: リソース `/books/HarryPotter`、アクション `download`。この API は有効な付与権限を記述します。特定のアクションを適用する際は権限リストで代用せず、`is-allowed` で判定を行ってください。

## 属性、診断およびトークン ID

REST リクエスト属性は配列内の型付きオブジェクトです。たとえば、属性項目は次のようになります:

```json
{
  "name": "clearance",
  "type": "numeric",
  "value": 4
}
```

組み込み Go の属性は `map[string]interface{}` です。診断レスポンス属性もオブジェクトであり、リクエスト配列とは異なります。型と条件については [SPDL](/ja/docs/spdl/)、評価の詳細については [診断](/ja/docs/pms/diagnosis/)、構成されたトークン検証については [トークンアサーション](/ja/docs/assertor/) を参照してください。

ポリシー施行ポイント（PEP）は、ポリシーが参照するすべてのカスタム属性を提供する必要があります（`request_user` や `request_time` などの組み込み属性は ADS によって提供されます）。PEP はエンドユーザーではなく、呼び出し側の信頼できるアプリケーションまたはサービスであり、送信する属性値に責任を持ちます。組み込み属性名で信頼できないエンドユーザー入力を転送しないでください。リクエストに存在しない属性の条件は一致しないため、属性が存在しない場合、条件付き拒否は適用されません。拒否条件が使用するすべての属性を PEP が確実に提供するようにし、それらのリクエストをテストしてください。
