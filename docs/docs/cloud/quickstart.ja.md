# Speedle Cloud ドキュメント

Speedle Cloud は、オープンソースの Speedle ポリシーエンジンをベースにした開発者向けのホスト型アクセス制御サービスです。SPDL 認可ポリシーおよびロールポリシーの管理、Playground での決定テスト、アプリケーションサーバーからの認可 API 呼び出しを、独自の認可サービスを運用することなく行えるよう設計されています。

**Speedle Cloud はまもなく公開されます。** Cloud ヘッダーには **Coming soon（まもなく公開）** バッジが表示されており、[cloud.speedle.io](https://cloud.speedle.io/ja/) での本番サインインはまだ開始されていません。ここで説明されているコンソールワークフローおよび認証付き認可決定サービス (ADS) は、公開されているサービス契約のプレビューです。認証付き ADS が保留中である間、トークンを発行してもサーバー側の認可呼び出しは有効になりません。選択したステージでのデプロイメントと ID プロバイダーの可用性を確認してください。

無料パブリックベータは、試用、学習、評価を目的としています。SLA は提供されず、本番環境での使用は推奨されません。また、機能や制限は変更される場合があります。

## ドキュメント

- [クイックスタート](https://engine.speedle.io/ja/docs/cloud/getting-started/): GitHub または Google でのサインイン（まもなく開始）、アカウント管理、サービス、SPDL ポリシー、セッション管理リクエスト、および 4 つのセッション Playground 操作。
- [アプリケーショントークン](https://engine.speedle.io/ja/docs/cloud/tokens/): 最近の認証要件、1 回限りのトークン開示、メタデータのページネーション、ローテーション、および失効。
- [認可 API プレビュー](https://engine.speedle.io/ja/docs/cloud/authorization-api/): IsAllowed リクエストとレスポンスの契約、Node.js 22 のサンプル、ポリシースナップショット、および統合の制約。
- [API リファレンス](https://engine.speedle.io/ja/docs/cloud/api-reference/): 公開 Cloud API の読み取り専用 Swagger UI。Try it out は無効なので、アプリケーション トークンをブラウザーに入力しません。
- [制限とエラー](https://engine.speedle.io/ja/docs/cloud/limits-and-errors/): アカウントクォータ、リクエスト制限、エラーレスポンス、レートおよびクォータヘッダー、再試行のガイダンス。

## ダウンロード

- [日本語 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ja.md): 4 つの Cloud の章をまとめたオフラインドキュメント。
- [API リファレンス](https://engine.speedle.io/ja/docs/cloud/api-reference/): レンダリングされた公開仕様。下の生ファイルも引き続き利用できます。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): リポジトリにチェックインされた正規の Cloud API 仕様。
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): 同じ YAML ソースバイトから生成。
- [提供ビルドバージョン](https://cloud.speedle.io/build.json): サービスを提供しているビルドを識別（ID プロバイダーやエンドポイントの可用性を証明するものではありません）。

仕様には 4 つのセッション Playground 操作と認証付き ADS 契約が含まれます。仕様の公開やガイドのダウンロードを行っても、保留中のエンドポイントが有効になったりサインインが開始されたりすることはありません。YAML および生成された JSON は、対応する封印された Cloud ビルドとともにバージョン管理、プロモート、復元されます。日本語、英語、中国語の Markdown ダウンロードは、これらの Engine ドキュメントページとともに保守されます。

Cloud の[利用規約](https://cloud.speedle.io/ja/terms)および[プライバシーポリシー](https://cloud.speedle.io/ja/privacy)をお読みください。

## Speedle Cloud クイックスタート

**Speedle Cloud はまもなく公開されます。** cloud.speedle.io の本番環境ではまだサインインが開始されておらず、Cloud ヘッダーには **Coming soon（まもなく公開）** バッジが表示されます。サインインが閉じられている間、サインインページには近日公開予定である旨が表示されます。本文では公開されている使用フローを説明していますが、本番コンソールが現在開かれていることを意味するものではありません。

### 本番環境

アカウントセッション、管理リクエスト、およびアプリケーショントークンは `https://cloud.speedle.io` を使用します。アカウント、リソース、トークンはそのオリジンに属します。

### 最初の認可ポリシーの作成

エディターに次の SPDL ステートメントを入力します:

```text
grant user alice from corp get /books
```

`from corp` は、この認可を `corp` ID ドメイン内に制限します。`corp` を、アプリケーションが信頼できる認証データから導出するドメインに置き換え、以下の Playground の例のように、各リクエストでプリンシパルの `idd` として同じ値を送信してください。`from` のない認可は、すべてのドメインの `alice` という名前のユーザーに一致します。ドメイン間で名前が衝突する可能性のない、単一の ID ソースを持つサービスの場合にのみ、この句を省略してください。ルールと対応するリクエストフィールドについては [ID ドメイン](/ja/docs/idd/) を参照してください。

### Playground で認可決定をテスト

Playground を開いてポリシーをテストします。

リクエストのサブジェクトにプリンシパルを入力します:

```json
{"type":"user","name":"alice","idd":"corp"}
```

アクションに `get`、リソースに `/books` を入力します。

## Speedle Cloud アプリケーショントークン

アプリケーショントークンを使用すると、バックエンドアプリケーションから Speedle Cloud の認可 API を認証して呼び出すことができます。

## Speedle Cloud 認可 API プレビュー

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

一時的なネットワークエラーや `429 Too Many Requests` の場合は、指数バックオフとジッターを伴う再試行を実装してください。
