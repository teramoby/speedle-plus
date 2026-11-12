# Speedle Cloud ドキュメント

Speedle Cloud は、オープンソースの Speedle ポリシーエンジンをベースにした開発者向けのホスト型アクセス制御サービスです。SPDL 認可ポリシーおよびロールポリシーの管理、Playground での決定テスト、アプリケーションサーバーからの認可 API 呼び出しを、独自の認可サービスを運用することなく行えるよう設計されています。

**Speedle Cloud はまもなく公開されます。** Cloud ヘッダーには **Coming soon（まもなく公開）** バッジが表示されており、[cloud.speedle.io](https://cloud.speedle.io/ja/) での本番サインインはまだ開始されていません。ここで説明されているコンソールワークフローおよび認証付き認可決定サービス (ADS) は、公開されているサービス契約のプレビューです。認証付き ADS が保留中である間、トークンを発行してもサーバー側の認可呼び出しは有効になりません。選択したステージでのデプロイメントと ID プロバイダーの可用性を確認してください。

無料パブリックベータは、試用、学習、評価を目的としています。SLA は提供されず、本番環境での使用は推奨されません。また、機能や制限は変更される場合があります。

## ドキュメント

- [クイックスタート](https://engine.speedle.io/ja/docs/cloud/getting-started/)：サインインし、コンソールでサービスとポリシー、API キーを作成して、アプリケーションから認可を確認します。
- [アプリケーショントークン](https://engine.speedle.io/ja/docs/cloud/tokens/): 最近の認証要件、1 回限りのトークン開示、メタデータのページネーション、ローテーション、および失効。
- [認可 API プレビュー](https://engine.speedle.io/ja/docs/cloud/authorization-api/): IsAllowed リクエストとレスポンスの契約、Node.js 22 のサンプル、ポリシースナップショット、および統合の制約。
- [制限とエラー](https://engine.speedle.io/ja/docs/cloud/limits-and-errors/): アカウントクォータ、リクエスト制限、エラーレスポンス、レートおよびクォータヘッダー、再試行のガイダンス。

## ダウンロード

- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): Cloud の各章をまとめたオフラインドキュメント。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): リポジトリにチェックインされた正規の Cloud API 仕様。
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): 同じ YAML ソースバイトから生成。
- [提供ビルドバージョン](https://cloud.speedle.io/build.json): サービスを提供しているビルドを識別（ID プロバイダーやエンドポイントの可用性を証明するものではありません）。

仕様には 4 つのセッション Playground 操作と認証付き ADS 契約が含まれます。仕様の公開やガイドのダウンロードを行っても、保留中のエンドポイントが有効になったりサインインが開始されたりすることはありません。YAML および生成された JSON は、対応する封印された Cloud ビルドとともにバージョン管理、プロモート、復元されます。日本語、英語、中国語の Markdown ダウンロードは、これらの Engine ドキュメントページとともに保守されます。

Cloud の[利用規約](https://cloud.speedle.io/ja/terms)および[プライバシーポリシー](https://cloud.speedle.io/ja/privacy)をお読みください。

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

[API キー](https://cloud.speedle.io/ja/tokens)を開き、`books-app` などの名前を入力し、`ads` スコープ（認可チェック）を選択して、**API キーを作成**を押します。本人確認を求められた場合は、確認を完了してからフォームを再送信します。シークレットは一度しか表示されないため、その場でコピーしてください。現在、有効期限は選択できません。不要になったキーは失効させてください。[API キーの管理](https://engine.speedle.io/ja/docs/cloud/tokens/)を参照してください。

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

次のステップ：プログラムから管理する場合は、[API キー](https://engine.speedle.io/ja/docs/cloud/tokens/)と [PMS API または spctl（Speedle Cloud）](https://engine.speedle.io/ja/docs/pms/policy-mgmt/#speedle-cloud)を参照してください。

## Speedle Cloud 認可 API プレビュー

Speedle Cloud は、高速で低レイテンシのアクセス決定のための認可 API を提供します。

### リクエスト契約

アプリケーションは、認可決定サービス (ADS) に対して `POST /authz-check/v1/is-allowed` を呼び出します。

### Node.js 22 の例

以下の Node.js の例は、ベアラートークンを使用して認可リクエストを送信する方法を示しています:

```javascript
import { request } from 'node:https';

export async function checkAccess(token, authenticatedUserId, authenticatedUserIdd, resource, action) {
  if (!authenticatedUserId || !authenticatedUserIdd) {
    throw new Error('principal name and identity domain are required');
  }

  const payload = JSON.stringify({
    subject: {
      principals: [
        {
          type: 'user',
          name: authenticatedUserId,
          idd: authenticatedUserIdd
        }
      ]
    },
    serviceName: 'default',
    resource: resource,
    action: action
  });

  return new Promise((resolve, reject) => {
    const req = request('https://cloud.speedle.io/authz-check/v1/is-allowed', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload)
      }
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          resolve(JSON.parse(data));
        } else {
          reject(new Error(`Authorization failed: ${res.statusCode} ${data}`));
        }
      });
    });
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}
```

ポリシー例 `grant user alice from corp get /books` に対して、この関数を呼び出すと期待どおりの認可決定が返されます。

## Speedle Cloud の制限とエラー

### レート制限とクォータ

無料パブリックベータ期間中、Speedle Cloud では公正な利用を確保するためにアカウントごとのクォータが適用されます。

### エラーレスポンス

エラーレスポンスは標準の HTTP ステータスコードと JSON エラーオブジェクトを返します:

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Too many requests. Please retry later."
  }
}
```

### 再試行のガイダンス

一時的なネットワークエラーや `429 Too Many Requests` の場合は、指数バックオフとジッターを伴う再試行を実装してください。
