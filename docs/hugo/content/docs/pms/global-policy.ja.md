+++
title = "グローバルポリシー"
description = "複数のサービス間でロール割り当てを共有し、操作権限は各サービス内で個別に管理"
date = 2019-01-18T21:19:44+08:00
weight = 2
draft = false
bref = "複数のサービスに共通するロール定義がある場合、グローバルポリシーを使用することでポリシー管理を簡素化できます。"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## ロールをグローバルにするべき場合

通常のロールはサービススコープです。複数のサービスでロール割り当てを共有する場合、特別に予約された名前の `global` サービスでその割り当てを定義し、各サービス独自の認可ポリシー内でそのロールを使用します。

サポートされているのはグローバルな**ロールポリシー**のみです。グローバルなロール割り当て自体がすべての操作権限を付与するわけではありません。各通常サービスがそれぞれの権限を定義します。グローバルな認可ポリシーはサポートされていません。

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

## グローバルロールとローカル権限の作成

<!-- docs-check: global-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user Emma AdminRole" --service-name global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service library
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role AdminRole borrow books" --service-name library
```

`global` はサービス名として完全に一致している必要があります。`library` はそのポリシーの前に作成されます。どちらも通常のサービス API を通じて管理されます。

## 結果の検証

Emma は `AdminRole` を通じて許可されます。Bob は拒否されます。

<!-- docs-check: global-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Emma"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: global-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

リソーススコープのグローバルロールポリシーは、そのリソーススコープにのみ適用されます。特にアイデンティティが [異なるドメイン](/ja/docs/idd/) にまたがる場合は、サービス間のロール割り当てを定期的に確認してください。
