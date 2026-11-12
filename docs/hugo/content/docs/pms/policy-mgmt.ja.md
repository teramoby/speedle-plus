+++
title = "ポリシー管理"
description = "サービス、認可ポリシー、ロールポリシー、カスタム関数の完全なライフサイクルの管理"
weight = 1
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pms", "policy", "core"]
categories = ["docs"]
bref = "ポリシー管理の基本"
lastmod = 2026-11-09T07:57:54+08:00
+++

## Speedle ポリシーとは？

ポリシー管理サービス (PMS) は、サービス、認可ポリシー、ロールポリシー、およびカスタム関数を管理します。ポリシーストアは、登録された [ストレージバックエンド](/ja/docs/store/) を使用してそれらのオブジェクトを永続化します。

<img src="/img/speedle/policystore.png" width="1942" height="540" loading="lazy"/>

## サービスと認可ポリシー

サービスはそのポリシーのスコープを制限します。認可ポリシーは、プリンシパルに対してリソース上のアクションを実行する権限を付与または拒否し、オプションで条件を設定できます。[グローバルロールポリシー](/ja/docs/pms/global-policy/) を除き、あるサービスのポリシーが別のサービスで自動的に適用されることはありません。

<img src="/img/speedle/authzpolicy.png" width="1770" height="444" loading="lazy"/>

```text
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
```

この正規表現は `c1/default/core/pods/` 配下のリソースに一致します。`c1/default/core/podssecret` や `c1` の前に余計なプレフィックスがあるパスには一致しません。`expr:` はシェルのワイルドカードではなく正規表現を使用します。任意の文字には `.*` を使用し、完全な識別子に一致させる必要がある場合は `^`/`$` を使用します。

## ロールポリシー

<img src="/img/speedle/rolepolicy.png" width="1598" height="436" loading="lazy"/>

```text
grant user alan manager on res1
grant role manager update res1
```

最初のステートメントは特定のリソース上のロールを Alan に割り当て、2 番目のステートメントはそのロールに操作権限を付与します。ロールを割り当てるだけでは権限は生成されません。該当する明示的な拒否（deny）は許可（grant）をオーバーライドします。

## プリンシパルと条件

プリンシパルには、ユーザー、グループ、エンティティ、またはロールを指定できます。エンティティはサービスなどの人間以外のアイデンティティを表します。統合コードはこれらのアイデンティティを信頼できるソースから取得する必要があります。

<img src="/img/speedle/principal.png" width="1769" height="505" loading="lazy"/>

AND プリンシパルグループは、リスト内のすべてのプリンシパルが一致することを要求します。個別のグループ間は OR 条件になります。たとえば、以下の例では両方のロールが適用される必要があります:

<img src="/img/speedle/andprincipal.png" width="1584" height="349" loading="lazy"/>

```text
grant (role designer, role dba) update db_design_doc
```

リソースは保護対象のオブジェクトを指定し、アクションはアプリケーションの操作を指定します。条件は、信頼できる属性、定数、演算子、およびサポートされている関数から評価されるブール式です。

<img src="/img/speedle/resource.png" width="1732" height="349" loading="lazy"/>

文法については [SPDL](/ja/docs/spdl/) を、完全な決定例については [RBAC](/ja/docs/overview/)/[ABAC](/ja/docs/overview/) を参照してください。

## サンプル実行前の準備

[クイックスタート](/ja/quick-start/) の手順に従い、リポジトリルート下の `./bin/spctl`、`./bin/speedle-pms`、`./bin/speedle-ads` をビルドします。Go 1.26.10 または Go 1.27.3（または同じリリースブランチの新しいパッチバージョン）、Bash、curl、Python 3 が必要です。各ターミナルはリポジトリのルートから実行してください。このページ用に新しいポリシーファイルを使用します。両方のサービスを停止した後にのみファイルをリセットしてください。

空のポリシーファイルを初期化します:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

> **macOS:** `/usr/sbin/spctl` は Apple の Gatekeeper ツールであり、引数なしの `spctl` コマンドを実行した際に選択される場合があります。Speedle の CLI は完全なパスで呼び出すか、それを含むディレクトリから `./spctl` を使用してください。以下の例では `./bin/spctl` を使用しています。シェルがどの実行可能ファイルを選択するかを確認するには、`command -v spctl` を実行します。ダウンロードしたリリースに `speedle-spctl_<os>_<arch>` アセットが含まれている場合、それは Apple のツールと衝突しない名前の同じ CLI です。

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

## spctl によるサービスとポリシーの管理

新しいストアに対して以下のシーケンスを実行します。ID はライブクエリのレスポンスから取得され、古い実行からの固定値ではありません。削除はサービスを必要とするすべての操作の後に行われます。

<!-- docs-check: policy-management -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy policy1 -c "grant user alan read book" --service-name test
policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="policy1"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy rolepolicy01 -c "grant user alan manager on res1" --service-name test
role_policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="rolepolicy01"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete service test
```

## Speedle Cloud

アカウント API キーを使い、`spctl` で Cloud のサービスとポリシーを管理します。管理エンドポイントは次のとおりです。

```text
https://cloud.speedle.io/policy-mgmt/v1
```

アカウント API キーには固定スコープを 1 つ指定します。`pms:read` はサービスとポリシーの読み取り、`pms:write` はさらに作成と削除を許可します。アカウント内のサービスが対象です。`ads` キーは管理できず `403 insufficient_scope` になります。作成と失効は [API キー](/ja/docs/cloud/tokens/)を参照してください。

エンドポイントをグローバルフラグとして渡し、`SPCTL_TOKEN` からAPI キーを読み取ります。対話型シェルでは、これによりシークレットがコマンド引数やシェル履歴に残らなくなります:

<!-- deployment-template: cloud-token-environment -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
export SPCTL_TOKEN
./bin/spctl --pms-endpoint https://cloud.speedle.io/policy-mgmt/v1 get service --all
unset SPCTL_TOKEN
```

互換性のために `--token` フラグも引き続き利用できますが、環境変数を使用するとコマンド引数によるAPI キーの漏洩を防ぐことができます。すべてのグローバルフラグには環境変数も用意されています。プレフィックス `SPCTL_` を付け、フラグ名を大文字にしてハイフンをアンダースコアに置き換えます。たとえば、`SPCTL_PMS_ENDPOINT` は `pms-endpoint` を設定し、`SPCTL_SKIPVERIFY` は `skipverify` を設定します。

設定を保存するには `~/.speedle` を使用します。形式はコマンドラインと同じフラグ名を持つ、1 行あたり 1 つの `name=value` ペアです:

```ini
pms-endpoint=https://cloud.speedle.io/policy-mgmt/v1
token=<api-key>
```

コマンド引数にAPI キーを含めずにこのファイルを作成または更新します:

<!-- deployment-template: cloud-token-config -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
printf '%s\n' "$SPCTL_TOKEN" | ./bin/spctl config token --stdin
unset SPCTL_TOKEN
```

`spctl config token --stdin` はパーミッション `0600` で `~/.speedle` を書き込みます。手動でファイルを編集する場合は、`install -m 600 /dev/null ~/.speedle` で作成するか、後で `chmod 0600 ~/.speedle` を実行してください。パーミッションがグループまたはその他のアクセスを許可しているファイルからAPI キーを読み取ると、`spctl` は標準エラー出力に警告を出力します。`spctl config --list` は設定されたAPI キーを `********` としてマスクします。

明示的なコマンドラインフラグは環境変数をオーバーライドし、環境変数は両方の設定ファイルをオーバーライドします。ファイル間では、`~/.speedle` が `~/.spctlconfig` をオーバーライドします。`spctl config` で `token` 以外の設定項目を変更すると、それが `~/.spctlconfig` に書き込まれ、`~/.speedle` から対応するコピーが削除されます。

API キーは、証明書の検証が有効になっている HTTPS 経由でのみ `Authorization: Bearer` 認証情報として送信されます。API キーを使用した平文 HTTP は、`127.0.0.1`、`::1`、`localhost` などのループバックホストに対してのみ許可されます。`spctl` はリモートの HTTP エンドポイントおよび HTTPS から HTTP へのダウングレードリダイレクトに対しては認証情報を送信する前に拒否します。HTTPS エンドポイントの場合、`--skipverify=true` が指定されているとAPI キーの送信も拒否されます。代わりに `--cacert` を使用してプライベートまたは自己署名の認証局を信頼してください。API キーは通常のコマンド出力やエラーメッセージには含まれません。

Cloud のエラーはゼロ以外の終了ステータスを生成します。`401` の後はAPI キーを確認し、`403 insufficient_scope` の後はそのスコープを確認してください。`404` は存在しないリソースを示し、`409 service_exists` および `409 policy_exists` は重複を示します。`429` の場合は、返された `Retry-After` の値を尊重し、有界なバックオフを使用してください。`429 daily_quota_exceeded` では `Retry-After` に従い 00:00 UTC まで待ちます。サービスとポリシーの上限は `409 quota_exceeded` で、時間ではリセットされません。未使用の項目を削除してください。API キーの枠は、キーを失効するか期限切れを待って空きます。[制限とエラー](/ja/docs/cloud/limits-and-errors/)を参照してください。

## その他のインターフェース

[PMS REST API](/ja/docs/api/management_api/) は JSON を使用し、文字列 ID を持つフラットなポリシーオブジェクトを返します。Go アプリケーションは [PMS インターフェース](https://github.com/teramoby/speedle-plus/tree/master/api/pms) を直接呼び出すことができます。gRPC クライアントは [公開されている PMS proto](/protobuf/pms.proto) を使用します。管理操作には [トランスポートおよび呼び出し元のアクセス制御](/ja/docs/security/) を適用してください。
