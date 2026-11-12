+++
title = "Speedle Cloud 認可 API"
linkTitle = "認可 API"
description = "アカウント API キーによる IsAllowed、呼び出し元の ID と Node.js 22 の例。"
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

アカウント API キーを使い、サーバーから認可を確認します。キー作成にはサインインが利用可能である必要があります。管理スコープでは ADS を呼び出せません。`ads` 以外は `403 insufficient_scope` です。

## サーバーから IsAllowed を呼び出す

`ads` スコープの API キーを 1 つの `Authorization: Bearer` ヘッダーに入れ、POST で送信します。

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

本文は自ホストの Speedle ADS と同じ REST 形式です。`attributes` は `{name, type, value}` の配列で、型は `string`、`numeric`、`bool`、RFC 3339 の `datetime` です。

`idd` はアプリケーションが認証した ID ドメインです。信頼できるサーバー側の認証情報から設定してください。`grant user alice from corp get /books` は `corp` のみで一致します。`from` のないポリシーは全ドメインの同名ユーザーに一致します。`idd` を省略した principal も全ドメインに一致するため、複数の ID ソースでは必ず送信してください。

`serviceName` は必須で既定値はありません。キーのアカウント内のサービスを選びます。未指定、空、不明、別アカウントの場合は `404 {"error":"service_not_found"}` です。キーは単一サービスには結び付きません。[入門ポリシー](/ja/docs/cloud/getting-started/)の例では `books` を指定します。

## 判定とユーザー ID

成功時は `allowed` が真偽値、`reason` が数値です。ポリシーによる拒否も HTTP 200 になり、`allowed` は `false` です。保護対象の操作前に `allowed` を確認し、例外や無効な応答は拒否してください。

API キーはアカウントと API スコープを認証し、エンドユーザーはアプリケーションで認証します。ブラウザから渡された principal を信用しないでください。Cloud は `subject.token` と `tokenType` を検証せず破棄します。principal のみで判定するので、これらのフィールドをユーザー認証の証拠にしないでください。

Playground は現在の保存済みポリシー、ADS は公開済みスナップショットを使用します。Playground の成功はすべての ADS 拠点への反映を保証しません。公開 ADS Diagnose API はありません。

## Node.js 22 の例

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiKey = process.env.SPEEDLE_API_KEY;
  if (!apiKey) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      subject: {
        principals: [
          { type: "user", name: authenticatedUserName, idd: authenticatedUserIdd },
        ],
      },
      serviceName: "books",
      resource: "/books",
      action: "get",
    }),
  });
  if (!response.ok) throw new Error(`Speedle HTTP ${response.status}`);
  const result = await response.json();
  if (
    typeof result !== "object" || result === null || Array.isArray(result) ||
    typeof result.allowed !== "boolean" || !Number.isInteger(result.reason) ||
    result.reason < 0 || result.reason > 6
  ) {
    throw new Error("Invalid Speedle decision");
  }
  return result.allowed;
}
```

`SPEEDLE_API_KEY` はサーバーのシークレットストアから設定します。ユーザー名と ID ドメインはサーバーの認証結果を渡します。空の値は送信前に拒否し、例外を許可に変換しないでください。例には再試行ループがありません。必要なら `Retry-After` に従う上限付き再試行を追加し、認証情報付きリダイレクトを防ぐ `redirect: "error"` を維持します。

## 通信とキャッシュ

POST を使用します。他のメソッドでは `405 method_not_allowed` または早期の非 JSON のブロック応答を受ける場合があります。秘密の API キーをブラウザから直接送る CORS フローはありません。ADS は `Cache-Control: no-store` を使用し、判定とキーを共有キャッシュに保存しないでください。

[SDK](/ja/docs/sdks/)、[制限とエラー](/ja/docs/cloud/limits-and-errors/)、[ダウンロード](/ja/docs/cloud/)も参照してください。
