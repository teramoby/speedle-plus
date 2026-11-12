+++
title = "Speedle Cloud API キー"
linkTitle = "API キー"
description = "アカウント API キーの作成、保護、一覧表示、失効と固定スコープ。"
weight = 20
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

API キーはアカウントに属し、スコープに応じてそのアカウントのサービスにアクセスします。アプリケーションを認証するもので、エンドユーザーの認証ではありません。API キー、セッション、連携 ID の管理にはログイン済みのコンソールセッションが必要です。

## API キーを作成する

[API キー](https://cloud.speedle.io/ja/tokens) を開き、前後の空白を除いて 1–128 文字の名前とスコープを指定し、**作成**を選択します。作成には過去 10 分以内の連携プロバイダーによる認証が必要です。`403 reauth_required` の場合は再認証後、再び**作成**を選択してください。プロバイダーから戻るだけでは作成されません。

各キーには作成後に変更できないスコープが 1 つあります。

| スコープ | アクセス |
| --- | --- |
| `ads`（既定） | `/authz-check/v1/*` による認可チェックのみ。 |
| `pms:read` | `/policy-mgmt/v1` によるアカウント内のサービス、認可ポリシー、ロールポリシーの読み取り。 |
| `pms:write` | `/policy-mgmt/v1` によるアカウント内のサービス、認可ポリシー、ロールポリシーの読み取り、作成、削除。 |

管理スコープは ADS を呼び出せず、`ads` はポリシーを管理できません。不適切なスコープは `403 insufficient_scope` です。サインインが閉じている場合の管理スコープの発行は `403 management_scope_disabled` です。

コンソールはセッション認証付きのリクエストを送信します。

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app","scope":"ads"}
```

入力は `name` と任意の `scope`（既定は `ads`）だけです。サービス指定、有効期限、認証証明は送信しません。セッション Cookie と CSRF ヘッダーが必要で、API キーではセッションを代替できません。

## シークレットを保護する

作成成功時は HTTP 201 を返します。レスポンスの `token` フィールドが API キーの値で、表示は一度だけです。

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

<!-- TODO: Update API key expiry after https://github.com/teramoby/cloud.speedle.io/issues/427 merges. -->

これは形式の説明であり、使用可能なキーではありません。すぐにサーバー側のシークレットストアへ保存してください。ブラウザストレージ、フロントエンド、URL、ソース管理、スクリーンショット、ログに含めないでください。一覧はメタデータのみを返し、紛失した値は復元できません。

アカウント全体で、全スコープ合計 **10 個の有効な API キー**を持てます。上限を超える作成は `409 {"error":"quota_exceeded","limit":10}` です。未使用のキーを失効させて枠を空けてください。時間によるリセットはありません。失効済みのキーは一覧に残りますが上限には含まれません。

## API キーを一覧表示する

`GET /api/tokens` は有効および失効済みのキーを新しい順に返します。既定は 25 件で、`limit` は 1–100 です。レスポンスの `nextCursor` を次の `cursor` に指定し、`null` になるまで進みます。`lastUsedAt` はベストエフォートで通常は 1 時間に最大 1 回の更新なので、未使用の証明にはなりません。

## API キーを失効させる

**失効**を選び、名前と ID を確認して確定します。最近の再認証は不要です。コンソールは次を送信します。

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

最初の失効は本文なしの HTTP 204 です。不明、別アカウント、失効済みの ID は `404 not_found` です。確認済みの認証情報が最大 60 秒間有効な場合があり、すべての場所で即座に反映されるわけではありません。

操作は**作成**と**失効**です。値やスコープを変更するには別のキーを作成し、呼び出し元を更新してから古いキーを失効させます。上限時にすべて使用中の場合、失効前に中断を計画してください。サービスを削除してもアカウントの API キーは失効しません。

応答が失われた場合は一覧を更新してから操作してください。同じ名前でも紛失した値は復元できません。[制限とエラー](/ja/docs/cloud/limits-and-errors/)と[認可 API](/ja/docs/cloud/authorization-api/)も参照してください。
