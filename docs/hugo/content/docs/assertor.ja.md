+++
title = "トークンアサーター"
description = "ID トークンを検証し、Speedle 認可リクエストに信頼できるプリンシパルとアイデンティティドメインを提供"
date = 2019-01-21T09:28:30+08:00
weight = 60
draft = false
bref = "評価リクエストには、ユーザーを表す ID トークンを含めることができます。この場合、評価エンジンはアサーターサービスを呼び出して、ユーザーの明示的なアイデンティティを取得できます"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["Identity", "Token asserter"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## トークンアサーターの役割

設定されたアサーターはトークンを検証し、アイデンティティドメインを含む明示的なプリンシパルを返します。Speedle はそれらのアイデンティティを使用してポリシーを評価します。本番環境の実装では、トークンの署名またはイントロスペクション結果、発行者、対象者、有効期限、およびプロバイダー固有の要件を検証する必要があります。Webhook インターフェースがあること自体は、任意の OAuth/OIDC トークンとの自動互換性を意味するものではありません。

[トークンアサーション API](/ja/docs/api/asserter_api/) の `errCode` は整数型であり、オプションの属性は**オブジェクト**であり、型付き属性配列ではありません。`asserterWebhookConfig.endpoint` は完全な URL です。以下の例では `/v1/assert` を使用します。

## 確定的なローカルモックの実行

このモックは、`githubtoken`/`github`、`googletoken`/`google`、`gitlabtoken`/`gitlab` の 3 組の入力のみを認識し、それぞれ対応するドメインの `user1` を表します。トークンとプロバイダーのペアが一致しない場合は失敗します。**これはテスト用フィクスチャであり、実際のトークン検証ではありません。** トークンの値は記録されません。ループバックアドレスのみにバインドし、本番環境では実際のプロバイダー統合に置き換えてください。

リポジトリルートから別のターミナルで起動します:

<!-- docs-check: mock-asserter -->
```bash
go run ./website/tests/examples/asserter
```


<!-- docs-source: website/tests/examples/asserter/main.go -->
```go
// A deterministic mock for the token-assertion tutorial. Never use for real tokens.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Type string `json:"type"`
	Name string `json:"name"`
	IDD  string `json:"idd"`
}
type response struct {
	Principals []principal `json:"principals,omitempty"`
	ErrCode    int         `json:"errCode"`
	ErrMessage string      `json:"errMessage,omitempty"`
}

func assert(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		send(405, response{ErrCode: 405, ErrMessage: "GET required"})
		return
	}
	// Fixed fixtures model successful assertion. This is not signature validation.
	domains := map[string]string{"githubtoken": "github", "googletoken": "google", "gitlabtoken": "gitlab"}
	domain, ok := domains[r.Header.Get("x-token")]
	if !ok || domain != r.Header.Get("x-idp") {
		send(401, response{ErrCode: 401, ErrMessage: "invalid demonstration token or identity provider"})
		return
	}
	if allowed := r.Header.Get("x-allowedIDD"); allowed != "" {
		found := false
		for _, name := range strings.Split(allowed, ",") {
			found = found || strings.TrimSpace(name) == domain
		}
		if !found {
			send(403, response{ErrCode: 403, ErrMessage: "identity domain is not allowed"})
			return
		}
	}
	send(200, response{Principals: []principal{{Type: "user", Name: "user1", IDD: domain}}})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "Local demonstration endpoint")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assert", assert)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
```


## サンプル実行前の準備

[クイックスタート](/ja/quick-start/) の手順に従い、リポジトリルート下の `./bin/spctl`、`./bin/speedle-pms`、`./bin/speedle-ads` をビルドします。Go 1.27.4（または同じリリースブランチの新しいパッチバージョン）、Bash、curl、Python 3 が必要です。各ターミナルはリポジトリのルートから実行してください。このページ用に新しいポリシーファイルを使用します。両方のサービスを停止した後にのみファイルをリセットしてください。

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
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true --asserter-endpoint http://127.0.0.1:8080/v1/assert
```

残りのコマンドは 3 つ目のターミナルで実行します。PMS と ADS は同じファイルを共有し、変更は watch を通じて非同期に伝播します。新しいポリシーに依存する前に、更新が反映されていることを確認してください。`--insecure=true` はローカルのループバック用です。本番環境の接続については [セキュリティ](/ja/docs/security/) を参照してください。明示的な PMS アドレスの指定により、`~/.spctlconfig` を変更する必要はありません。完了したら、両方のサーバーターミナルで Ctrl-C を押して終了します。


## アイデンティティドメイン固有のテストポリシーの作成

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/idd.json
```


このフィクスチャは、GitHub の `user1` に読み取りアクセス、Google の `user1` に書き込みアクセス、すべてのドメインにレンタルアクセスを付与します。これは [アイデンティティドメインの例](/ja/docs/idd/) と同じルールです。

## トークンに基づく認可決定の検証

順番に、Allow、Deny、Allow を想定します。2 番目のリクエストは有効な **GitLab** アイデンティティであるため、GitHub 限定の読み取り権限は使用できません。

<!-- docs-check: token-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"githubtoken","tokenType":"github"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


無効なトークンはアサーションエラーであり、通常のポリシー Deny ではありません。現在の REST 実装は、HTTP 200 とともに `allowed=false` および空でない `errorMessage` を返す場合があります。HTTP ステータスとレスポンスボディの両方を確認してください。`reason` だけで評価が成功したと判断しないでください:

<!-- docs-check: token-invalid -->
```bash
curl --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"invalid-token","tokenType":"google"},"serviceName":"booksvc","resource":"book","action":"write"}' \
  -w '\nHTTP %{http_code}\n' http://127.0.0.1:6734/authz-check/v1/is-allowed
```


完了したら、モックと両方のサーバーで Ctrl-C を押して停止します。

## プロバイダー統合の設定

ローカルフラグの代わりに、完全な JSON 設定で以下を指定できます:

```json
{
  "asserterWebhookConfig": {
    "endpoint": "https://identity.example/v1/assert",
    "clientCert": "/absolute/path/client.crt",
    "clientKey": "/absolute/path/client.key",
    "caCert": "/absolute/path/issuer-ca.crt",
    "httpTimeout": 5
  }
}
```

これはサーバー設定のアサーター部分です。デプロイ時にはストレージとサーバーの設定も指定してください。`caCert` は Webhook サーバーを検証し、`clientCert` と `clientKey` はそのサーバーが mTLS を要求する場合に使用されます。プロバイダーのドキュメントに従って [OpenID Connect](https://openid.net/connect/) または [OAuth](https://oauth.net/2/) 経由でトークンを取得し、実際に受け入れられるトークンタイプを検証してください。PMS/ADS の呼び出し元は、[セキュリティ](/ja/docs/security/) で説明されているように個別に保護してください。
