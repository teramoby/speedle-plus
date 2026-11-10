+++
title = "ID ドメイン"
description = "複数の ID ドメインからのユーザー ID をサポート"
date = 2019-01-17T13:20:07+08:00
weight = 40
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["identity domain", "identity"]
+++

## ID ドメインとは？

ID ドメイン（`idd`）は、プリンシパルの明示的な名前空間です。同名の2人の `user1` が異なるドメインに属することができます。ドメイン文字列は、信頼できる統合コードまたは構成済みのトークンアサーターによって提供されます。これ自体は認証の証明ではありません。Speedle はドメイン名のみからプロバイダーのテナントモデルを推論または検証することはありません。

## ポリシーとリクエストにおけるドメインの表現

```text
grant user user1 from github read book
grant user user1 from IDCS.tenant01 read book
```

プリンシパル名の直後に `from <identity-domain>` を使用します。PMS JSON ポリシーでは、同じプリンシパルを `idd=github:user:user1` としてエンコードします。決定リクエストでは、このプリンシパルを次のように表現します:

```json
{
  "type": "user",
  "name": "user1",
  "idd": "github"
}
```

ドメインが指定されていないプリンシパルは、その名前のすべてのドメインおよびドメインのないプリンシパルに一致します。

```text
grant user user1 read book
```

上記の認可は、`from github`、`from IDCS.tenant01`、または ID ドメインのない `user1` に一致します。

## ローカル検証手順

<!-- docs-check: setup -->
```bash
mkdir -p ./tmp && rm -f ./tmp/idd.json
```

PMS を起動します:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./tmp/idd.json --endpoint 127.0.0.1:6733 --insecure=true
```

ADS を起動します:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./tmp/idd.json --endpoint 127.0.0.1:6734 --insecure=true
```

テスト用のサービスとポリシーを作成します:

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service bookstore
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 from github read book" --service-name=bookstore
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user2 read book" --service-name=bookstore
```

`github` ドメインの `user1` は許可されます:

<!-- docs-check: idd-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"bookstore","resource":"book","action":"read"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

ドメインのない `user1` は拒否されます:

<!-- docs-check: idd-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"bookstore","resource":"book","action":"read"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

`user2` のポリシーには `from` 句がないため、任意のドメインの `user2` が一致します:

<!-- docs-check: idd-any -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user2","idd":"corp"}]},"serviceName":"bookstore","resource":"book","action":"read"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

認可されていないアクションは拒否されます:

<!-- docs-check: idd-write-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"bookstore","resource":"book","action":"write"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```
