# Speedle Cloud ドキュメント

Speedle Cloud はオープンソースの Speedle エンジンによるホスト型認可サービスです。サービス、SPDL 認可・ロールポリシーを管理し、Playground で判定をテストして、サーバーから API を呼び出せます。

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

コンソールの利用と API キー作成にはサインインが利用可能である必要があります。無料パブリックベータは試用、学習、評価用で SLA はなく、本番利用は推奨しません。機能と制限は変更される場合があります。

## ドキュメント

- [クイックスタート](https://engine.speedle.io/ja/docs/cloud/getting-started/)：サインインし、コンソールでサービスとポリシー、API キーを作成して、アプリケーションから認可を確認します。
- [API キー](https://engine.speedle.io/ja/docs/cloud/tokens/): アカウントの認証情報、固定スコープ、必須の有効期限、作成と失効、一度だけの表示、ページング。
- [認可 API](https://engine.speedle.io/ja/docs/cloud/authorization-api/): IsAllowed、ユーザー ID、Node.js 22、通信の制約。
- [API リファレンス](https://engine.speedle.io/ja/docs/cloud/api-reference/): API key による認可チェックとポリシー管理。リクエストはサーバーから送り、このページは読み取り専用です。
- [制限とエラー](https://engine.speedle.io/ja/docs/cloud/limits-and-errors/): アカウント上限、サイズ、バースト、日次枠、エラーコード、リセット時刻。

## ダウンロード

- [API リファレンス（Swagger）](https://engine.speedle.io/ja/docs/cloud/api-reference/): すべてのエンドポイント、パラメーター、レスポンス例をオンラインで確認できます。
- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): Cloud の 4 章をまとめたオフラインガイド。
- OpenAPI 仕様: [YAML](https://cloud.speedle.io/openapi.public.yaml) · [JSON](https://cloud.speedle.io/openapi.public.json)、コード生成ツールや API クライアント向けです。

Cloud の次の文書をご確認ください： [利用規約](https://cloud.speedle.io/ja/terms) / [プライバシーポリシー](https://cloud.speedle.io/ja/privacy).

## Speedle Cloud クイックスタート

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

### GitHub または Google によるサインイン（まもなく開始）

ブラウザーで [Cloud コンソール](https://cloud.speedle.io/ja/)を開き、利用可能なプロバイダーでサインインします。未使用の確認済みメールアドレスで初めて利用する場合は、**アカウント作成**を確認します。メールアドレスが使用済みの場合は、コンソールに表示された連携済みプロバイダーでサインインしてください。

### サービスとポリシーを作成する

[サービスワークスペース](https://cloud.speedle.io/ja/service)を開き、`books` と入力して**サービス作成**を選択します。既存ポリシーが結果に影響しないよう、新しいサービスで試してください。既存データは上書きしないでください。

**新しいポリシー**を選択し、名前を `read-books` にして、次の SPDL を入力します。

```text
grant user alice from corp get /books
```

このポリシーは、ID ドメイン `corp` の `alice` が `/books` に対して `get` を実行することを許可します。`from` を省略すると、すべてのドメインの同名ユーザーに一致します。信頼できる認証データからドメインを取得し、各リクエストの `idd` に設定してください。[ID ドメイン](https://engine.speedle.io/ja/docs/idd/)を参照してください。

**ポリシーの検証**、**ポリシーを保存**の順に選択します。検証は構文を確認するだけで、下書きを保存しません。保存したポリシーが認可 API に反映されるまで約 60 秒かかります。

### API キーを作成する

[API キー](https://cloud.speedle.io/ja/tokens)を開き、`books-app` などの名前を入力し、`ads` スコープ（認可チェック）を選択し、有効期限を選び（コンソールの既定は 3 か月で、UTC の時刻を表示します）、**API キーを作成**を押します。本人確認を求められた場合は、確認を完了してからフォームを再送信します。シークレットは一度しか表示されないため、その場でコピーしてください。すべてのキーには有効期限があります。有効期限が必須になる前に作られたキーも含まれます。いつ期限切れになるかは [API キーの管理](https://engine.speedle.io/ja/docs/cloud/tokens/) に書いてあります。一覧の期限を確認し、その前に新しいキーを作成してください。不要になったキーは期限前に失効させてください。

キーをサーバーのシークレットストアに保存し、環境変数 `SPEEDLE_API_KEY` で渡します。ブラウザーのコードやソース管理には含めないでください。

### アプリケーションから認可を確認する

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

詳細は[認可 API](https://engine.speedle.io/ja/docs/cloud/authorization-api/)と [SDK](https://engine.speedle.io/ja/docs/sdks/)（[Node.js/TypeScript](https://engine.speedle.io/ja/docs/sdks/node/)、[Python](https://engine.speedle.io/ja/docs/sdks/python/)、[Java](https://engine.speedle.io/ja/docs/sdks/java/)、[Go](https://engine.speedle.io/ja/docs/sdks/go/) など）を参照してください。

### Playground で保存済みポリシーをテストする

必要に応じてサービスの **Playground** を開き、**Is allowed** を選択します。**Principals (JSON)** に `[{"type":"user","name":"alice","idd":"corp"}]`、**Resource** に `/books`、**Action** に `get`、**Attributes (JSON)** に `[]` を入力し、**Run test** を選択します。

**Is allowed** は判定結果、**Diagnose** はポリシーの寄与、**All granted roles** はサブジェクトのロール、**All granted permissions** は許可されたリソースとアクションを表示します。テストは保存済みポリシーを使用し、未保存の下書きは使用しません。入力やポリシーを変更したら再実行してください。アプリケーションは API キーを使用して [ADS 認可 API](https://engine.speedle.io/ja/docs/cloud/authorization-api/)を呼び出します。

次のステップ：プログラムから管理する場合は、`pms:write` スコープ（読み取りのみなら `pms:read`）のキーを別に作成し、[PMS API または spctl（Speedle Cloud）](https://engine.speedle.io/ja/docs/pms/policy-mgmt/#speedle-cloud)を使用してください。前の手順で作成した `ads` キーではポリシーを管理できません。

## Speedle Cloud API キー

API キーはアカウントに属し、スコープに応じてそのアカウントのサービスにアクセスします。アプリケーションを認証するもので、エンドユーザーの認証ではありません。API キー、セッション、連携 ID の管理にはログイン済みのコンソールセッションが必要です。

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

### API キーを作成する

[API キー](https://cloud.speedle.io/ja/tokens) を開き、前後の空白を除いて 1–128 文字の名前、スコープ、有効期限（`7d`、`30d`、`90d`、`180d`、`365d`。コンソールの既定は 3 か月）を指定し、**作成**を選択します。作成には過去 10 分以内の連携プロバイダーによる認証が必要です。`403 reauth_required` の場合は再認証後、再び**作成**を選択してください。プロバイダーから戻るだけでは作成されません。

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

{"name":"books-app","scope":"ads","expiresIn":"90d"}
```

入力は `name`、必須の `expiresIn`、任意の `scope`（既定は `ads`）です。`expiresIn` は `7d`、`30d`、`90d`、`180d`、`365d` のいずれかです（1 週間、1 か月、3 か月、6 か月、1 年）。1 日は 24 時間です。コンソールの **有効期間** は既定で 3 か月で、送信前に **有効期限のプレビュー (UTC)** を表示します。無期限のキーはありません。サービス指定、`expiresAt`、認証証明は送信しません。`expiresIn` がない、認識できない、またはクライアントが `expiresAt` を送った場合は `400 invalid_request` です。`name` と `scope` だけを送ったリクエストも同じエラーです。サーバーは `expiresIn` を補いません。セッション Cookie と CSRF ヘッダーが必要で、API キーではセッションを代替できません。

### シークレットを保護する

作成成功時は HTTP 201 を返します。レスポンスの `token` フィールドが API キーの値で、表示は一度だけです。

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

すべてのキーには有効期限があります。レスポンスの `expiresAt` は、サーバーが計算した Unix ミリ秒の期限です。その時点以降、認可チェックとポリシー管理の呼び出しは `401 {"error":"api_key_expired"}` を返します。新しいキーを作成してください。期限切れのキーを再送しても成功しません。期限前に失効させることもできます。有効期限が必須になる前に作られたキーにも期限があります。作成から 365 日（1 日 24 時間）後がまだ未来なら、必須化の直後でもその時刻に期限切れになります。その日がすでに過ぎていれば、必須化から 30 日（1 日 24 時間）後です。一覧のすべてのキーに `expiresAt` があります。その前に確認し、新しいキーを作成してください。

これは形式の説明であり、使用可能なキーではありません。すぐにサーバー側のシークレットストアへ保存してください。ブラウザストレージ、フロントエンド、URL、ソース管理、スクリーンショット、ログに含めないでください。一覧はメタデータのみを返し、紛失した値は復元できません。

アカウント全体で、全スコープ合計 **10 個の有効な API キー**を持てます。上限を超える作成は `409 {"error":"quota_exceeded","limit":10}` です。未使用のキーを失効させるか、期限切れを待って枠を空けてください。上限は周期では戻りません。失効済みおよび期限切れのキーは一覧に残りますが上限には含まれません。

### API キーを一覧表示する

`GET /api/tokens` は有効、期限切れ、失効済みのキーを新しい順に返します。既定は 25 件で、`limit` は 1–100 です。レスポンスの `nextCursor` を次の `cursor` に指定し、`null` になるまで進みます。各キーは Unix ミリ秒の `expiresAt` を含みます。コンソールは **有効期限 (UTC)** を表示します。7 日以内に期限を迎えるキーは **まもなく期限切れ** と表示されます。期限切れのキーは **期限切れ** のグループに入ります。`lastUsedAt` はベストエフォートで通常は 1 時間に最大 1 回の更新なので、未使用の証明にはなりません。

### API キーを失効させる

**失効**を選び、名前と ID を確認して確定します。最近の再認証は不要です。コンソールは次を送信します。

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

最初の失効は本文なしの HTTP 204 です。不明、別アカウント、失効済みの ID は `404 not_found` です。確認済みの認証情報が最大 60 秒間有効な場合があり、すべての場所で即座に反映されるわけではありません。

操作は**作成**と**失効**です。値、スコープ、有効期限を変更するには別のキーを作成し、呼び出し元を更新してから古いキーを失効させます。上限時にすべて使用中の場合、失効前に中断を計画してください。サービスを削除してもアカウントの API キーは失効しません。

応答が失われた場合は一覧を更新してから操作してください。同じ名前でも紛失した値は復元できません。[制限とエラー](https://engine.speedle.io/ja/docs/cloud/limits-and-errors/)と[認可 API](https://engine.speedle.io/ja/docs/cloud/authorization-api/)も参照してください。

## Speedle Cloud 認可 API

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

アカウント API キーを使い、サーバーから認可を確認します。キー作成にはサインインが利用可能である必要があります。管理スコープでは ADS を呼び出せません。`ads` 以外は `403 insufficient_scope` です。

### サーバーから IsAllowed を呼び出す

`ads` スコープの API キーを 1 つの `Authorization: Bearer` ヘッダーに入れ、POST で送信します。

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

本文は自ホストの Speedle ADS と同じ REST 形式です。`attributes` は `{name, type, value}` の配列で、型は `string`、`numeric`、`bool`、RFC 3339 の `datetime` です。

`idd` はアプリケーションが認証した ID ドメインです。信頼できるサーバー側の認証情報から設定してください。`grant user alice from corp get /books` は `corp` のみで一致します。`from` のないポリシーは全ドメインの同名ユーザーに一致します。`idd` を省略した principal も全ドメインに一致するため、複数の ID ソースでは必ず送信してください。

`serviceName` は必須で既定値はありません。キーのアカウント内のサービスを選びます。未指定、空、不明、別アカウントの場合は `404 {"error":"service_not_found"}` です。キーは単一サービスには結び付きません。[入門ポリシー](https://engine.speedle.io/ja/docs/cloud/getting-started/)の例では `books` を指定します。

### 判定とユーザー ID

成功時は `allowed` が真偽値、`reason` が数値です。ポリシーによる拒否も HTTP 200 になり、`allowed` は `false` です。保護対象の操作前に `allowed` を確認し、例外や無効な応答は拒否してください。

API キーはアカウントと API スコープを認証し、エンドユーザーはアプリケーションで認証します。ブラウザから渡された principal を信用しないでください。Cloud は `subject.token` と `tokenType` を検証せず破棄します。principal のみで判定するので、これらのフィールドをユーザー認証の証拠にしないでください。

Playground は現在の保存済みポリシー、ADS は公開済みスナップショットを使用します。Playground の成功はすべての ADS 拠点への反映を保証しません。公開 ADS Diagnose API はありません。

### Node.js 22 の例

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

### 通信とキャッシュ

POST を使用します。他のメソッドでは `405 method_not_allowed` または早期の非 JSON のブロック応答を受ける場合があります。秘密の API キーをブラウザから直接送る CORS フローはありません。ADS は `Cache-Control: no-store` を使用し、判定とキーを共有キャッシュに保存しないでください。

[SDK](https://engine.speedle.io/ja/docs/sdks/)、[制限とエラー](https://engine.speedle.io/ja/docs/cloud/limits-and-errors/)、[ダウンロード](https://engine.speedle.io/ja/docs/cloud/)も参照してください。

## Speedle Cloud 制限とエラー

**Speedle Cloud はまもなく公開されます。** このガイドは、本番環境のサインインと認証付き認可 API が利用可能になった際の手順を説明しています。例は実際の Cloud 環境では実行していません。

特記がない限り、制限はアカウント全体で共有します。API キーを変更しても日次クォータは増えません。

### 制限

| 対象 | 上限 | 到達時の対応 |
| --- | --- | --- |
| アカウントのサービス | 100 | `409 quota_exceeded`。未使用のサービスを削除。時間によるリセットなし。 |
| 全サービスの認可・ロールポリシー合計 | アカウントごとに 200 | `409 quota_exceeded`。ポリシーを削除。時間によるリセットなし。 |
| 1 ポリシーの完全な SPDL ソース | 1,024 UTF-8 バイト | コンソールの書き込みは `400 invalid_request`。ソースを短縮。文字数ではなくバイト数。時間によるリセットなし。 |
| 全スコープの有効な API キー | アカウントごとに 10 | `409 quota_exceeded`、`limit: 10`。未使用キーを失効するか、期限切れを待つ。上限は周期では戻らない。失効済みと期限切れは対象外。 |
| ポリシー名 / API キー名 | 128 Unicode コードポイント / 前後の空白を除いて 1–128 文字 | `400 invalid_request`。名前を修正。時間によるリセットなし。 |
| 課金対象のポリシー管理・認可呼び出し | アカウントごとに UTC 日あたり 10,000 | `429 daily_quota_exceeded`。00:00 UTC にリセット。`Retry-After` に従う。 |
| API キーの短時間のバースト | Cloud の各ロケーションでキーごとに毎分約 600 | `429 rate_limited`、`Retry-After: 60`。待ってから再試行。保証された処理量でも追加の日次枠でもない。 |
| 通常のリクエスト本文 | 65,536 バイト（64 KiB） | `413 request_too_large`。本文を縮小。時間によるリセットなし。 |
| アトミック JSON インポート本文（`POST /api/service/import` のみ） | 5,100,000 バイト | `413 request_too_large`。データを縮小。ポリシーとアカウントの上限も適用。時間によるリセットなし。 |
| 入力属性 | 50 属性、正規化 JSON の UTF-8 で 16,384 バイト | `400 SPDL-0004`。属性を減らすか縮小。時間によるリセットなし。 |

スコープ、有効期限、作成、失効は [API キー](https://engine.speedle.io/ja/docs/cloud/tokens/)を参照してください。

### 日次クォータ

読み取り、書き込み、検証、Playground、インポート、エクスポート、認可チェックで共有します。ポリシーによる拒否や不正な業務入力も数えます。認証、セッション操作、`GET /api/me` は対象外です。認証、スコープ、ルーティング、CSRF、本文サイズ、バースト制限、日次クォータで拒否されたリクエストも対象外です。

Cloud 自体の障害は、アカウントごとに UTC 日あたり最大 1,000 回までクォータを返還する場合があります。確認できない返還やリクエスト由来のエラーは課金対象のままです。カウンターが一時的に利用不能な間、認可呼び出しが低速で継続される場合があり、追加枠はアカウントごとに UTC 日あたり最大 10,000 回です。通常の枠とは別で保証されず、返還枠と同じく 00:00 UTC にリセットします。許可できなければ `503 quota_unavailable` です。

予約されたまま未使用の枠があるため、実際に使える回数は 10,000 未満の場合があります。`GET /api/me` は使用量とリセット時刻を返します。連続再試行やキーの変更で上限を回避しないでください。

### エラーとヘッダー

エラーは `{"error":"rate_limited"}` のようなフラットな JSON です。有効な認可結果が得られなければ拒否してください。

| ステータスとエラー | 対応 |
| --- | --- |
| `400 invalid_request` または Speedle エラー | リクエストを修正し、そのまま再試行しない。 |
| `401 unauthorized` | API キーまたはセッションを確認。セッション Cookie だけでは ADS を呼べない。 |
| `401 api_key_expired` | API キーが有効期限に達したか、それを過ぎています。新しいキーを作成してください。このキーの再試行は成功しません。有効期限が必須になる前に作られたキーにも期限があります。いつ切れるかは API キーのページにあります。`expiresAt` を確認し、その前に新しいキーを作成してください。 |
| `403 reauth_required` | キー作成前に連携プロバイダーで再認証。 |
| `403 insufficient_scope` | 必要なスコープのキーを使用。 |
| `403 management_scope_disabled` | サインイン停止中は管理スコープを発行できない。 |
| `403 csrf_failed` | セッション、Origin、CSRF ヘッダーを確認。 |
| `404 service_not_found`（ADS）/ `404 not_found`（管理） | 自分のアカウントのサービスを指定。不明、別アカウント、失効済みキー ID も `404 not_found`。 |
| `409 quota_exceeded` | 未使用のサービスまたはポリシーを削除して枠を空ける。時間ではこの二つの上限は戻らない。API キーの枠は、キーを失効するか期限切れを待って空ける。 |
| `413 request_too_large` | 本文を縮小。 |
| `429 rate_limited` / `429 daily_quota_exceeded` | `Retry-After` と上限付きバックオフに従う。日次枠は UTC 午前 0 時にリセット。 |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | 拒否し、上限付きバックオフで再試行。`Retry-After` があれば従う。 |

検証済み API キーの `X-RateLimit-Limit` は 600 です。正確な `X-RateLimit-Remaining` はありません。`X-Quota-Daily-Remaining` はアカウントの残量の推定値で、並行呼び出しや未使用の予約で実際と異なります。次のリクエストの成功を保証せず、早期拒否では省略されます。

ADS は `Cache-Control: no-store` を返します。結果や認証情報を共有キャッシュに保存しないでください。POST を使用し、他のメソッドでは `405 method_not_allowed` またはそれより前の非 JSON のブロック応答に対応してください。[認可 API](https://engine.speedle.io/ja/docs/cloud/authorization-api/)を参照してください。
