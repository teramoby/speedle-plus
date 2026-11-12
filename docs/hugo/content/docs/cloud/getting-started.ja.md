+++
title = "Speedle Cloud クイックスタート"
description = "Speedle Cloud のサインイン、SPDL ポリシーの保存、API キーによるアプリケーションからの認可チェック、Playground でのテスト。"
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

## GitHub または Google によるサインイン（まもなく開始）

ブラウザーで [Cloud コンソール](https://cloud.speedle.io/ja/)を開き、利用可能なプロバイダーでサインインします。未使用の確認済みメールアドレスで初めて利用する場合は、**アカウント作成**を確認します。メールアドレスが使用済みの場合は、コンソールに表示された連携済みプロバイダーでサインインしてください。

## サービスとポリシーを作成する

[サービスワークスペース](https://cloud.speedle.io/ja/service)を開き、`books` と入力して**サービス作成**を選択します。既存ポリシーが結果に影響しないよう、新しいサービスで試してください。既存データは上書きしないでください。

**新しいポリシー**を選択し、名前を `read-books` にして、次の SPDL を入力します。

```text
grant user alice from corp get /books
```

このポリシーは、ID ドメイン `corp` の `alice` が `/books` に対して `get` を実行することを許可します。`from` を省略すると、すべてのドメインの同名ユーザーに一致します。信頼できる認証データからドメインを取得し、各リクエストの `idd` に設定してください。[ID ドメイン](/ja/docs/idd/)を参照してください。

**ポリシーの検証**、**ポリシーを保存**の順に選択します。検証は構文を確認するだけで、下書きを保存しません。保存したポリシーが認可 API に反映されるまで約 60 秒かかります。

## API キーを作成する

[API キー](https://cloud.speedle.io/ja/tokens)を開き、`books-app` などの名前を入力し、`ads` スコープ（認可チェック）を選択して、**API キーを作成**を押します。本人確認を求められた場合は、確認を完了してからフォームを再送信します。シークレットは一度しか表示されないため、その場でコピーしてください。現在、有効期限は選択できません。不要になったキーは失効させてください。[API キーの管理](/ja/docs/cloud/tokens/)を参照してください。

キーをサーバーのシークレットストアに保存し、環境変数 `SPEEDLE_API_KEY` で渡します。ブラウザーのコードやソース管理には含めないでください。

## アプリケーションから認可を確認する

保存したポリシーに対して、アプリケーションのサーバーから次の `isAllowed` リクエストを送信します。このポリシーは属性を必要としないため、`attributes` は空です。

```bash
curl --fail-with-body --silent --show-error --max-time 10 \
  'https://cloud.speedle.io/authz-check/v1/is-allowed' \
  -H "Authorization: Bearer $SPEEDLE_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[]}'
```

サンプルのポリシーだけが保存されている場合、期待されるレスポンスは次のとおりです。

```json
{"allowed":true,"reason":0}
```

`action` を `delete` に変更して再送信します。このアクションを許可するポリシーがないため、期待されるレスポンスは次のとおりです。

```json
{"allowed":false,"reason":3}
```

どちらの判定も HTTP 200 を返します。`allowed` が `true` の場合だけ保護対象の操作を実行してください。アプリケーションは判定を適用し、エラー、不正なレスポンス、タイムアウト時にはアクセスを拒否する必要があります。

詳細は[認可 API](/ja/docs/cloud/authorization-api/)と [SDK](/ja/docs/sdks/)（[Node.js/TypeScript](/ja/docs/sdks/node/)、[Python](/ja/docs/sdks/python/)、[Java](/ja/docs/sdks/java/)、[Go](/ja/docs/sdks/go/) など）を参照してください。

## Playground で保存済みポリシーをテストする

必要に応じてサービスの **Playground** を開き、**Is allowed** を選択します。**Principals (JSON)** に `[{"type":"user","name":"alice","idd":"corp"}]`、**Resource** に `/books`、**Action** に `get`、**Attributes (JSON)** に `[]` を入力し、**Run test** を選択します。

**Is allowed** は判定結果、**Diagnose** はポリシーの寄与、**All granted roles** はサブジェクトのロール、**All granted permissions** は許可されたリソースとアクションを表示します。テストは保存済みポリシーを使用し、未保存の下書きは使用しません。入力やポリシーを変更したら再実行してください。アプリケーションは API キーを使用して [ADS 認可 API](/ja/docs/cloud/authorization-api/)を呼び出します。

次のステップ：プログラムから管理する場合は、`pms:write` スコープ（読み取りのみなら `pms:read`）のキーを別に作成し、[PMS API または spctl（Speedle Cloud）](/ja/docs/pms/policy-mgmt/#speedle-cloud)を使用してください。前の手順で作成した `ads` キーではポリシーを管理できません。
