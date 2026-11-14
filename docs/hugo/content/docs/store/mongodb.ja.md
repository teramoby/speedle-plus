+++
title = "MongoDB セットアップ"
description = "PMS および ADS 向けの MongoDB ポリシーストアの設定手順"
weight = 30
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mongodb"]
categories = ["docs"]
bref = "MongoDB レプリカセットを実行し、PMS と ADS を接続してライブポリシーの更新を確認し、バックアップを保存します。"
lastmod = 2026-10-31T22:36:36+08:00
+++

このウォークスルーでは、永続的な Docker ボリュームを備えたシングルノードレプリカセットとして **MongoDB 7.0.16** を使用します。PMS はポリシーを書き込み、ADS はチェンジストリームを介して同じデータベースを追跡します。サンプルでは、ローカル開発用に認証なしでループバックでリッスンします。本番環境の認証、TLS、およびレプリカセットの構成はウォークスルーの後に説明します。

MongoDB は**レプリカセットまたはシャーディングされたクラスター**として実行する必要があります。ADS はチェンジストリームでデータベースを監視し、ポリシーストア全体の置換はトランザクション内で実行されます。スタンドアロンの `mongod` ではどちらもサポートされていません。開発用にはシングルノードのレプリカセットで十分です。

## 1. Speedle のビルド

Bash、Git、curl、Docker、および Go 1.27.4 を準備します。Docker を起動し、ネイティブサービスをビルドします:

<!-- mongodb-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

後続のコマンドはこのチェックアウトから実行します。ローカルポート 27017、6733、6734 を利用可能にしておくか、コマンドと構成を同時に更新してください。

## 2. MongoDB の起動

名前付きボリュームが MongoDB のデータを保持します。`--replSet` は `speedle-rs` レプリカセットのメンバーとして `mongod` を起動します。ループバック上のデータベースポートのみが公開されます。

<!-- mongodb-step: database -->
```bash
docker run --name speedle-mongodb --detach \
  --publish 127.0.0.1:27017:27017 \
  --volume speedle-mongodb-data:/data/db \
  mongo:7.0.16 \
  --replSet speedle-rs --bind_ip_all
```

サーバーがコマンドを受け付けるのを待ちます:

<!-- mongodb-step: ready -->
```bash
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.adminCommand({ping: 1}).ok' >/dev/null 2>&1
do
  sleep 1
done
```

すでに MongoDB レプリカセットを実行している場合は、コンテナコマンドとステップ 3 をスキップし、以下の接続文字列を使用してください。コンテナの詳細については [公式 Docker イメージ](https://hub.docker.com/_/mongo) を参照してください。

## 3. レプリカセットの初期化

このサーバーを唯一のメンバーとしてレプリカセットを 1 回初期化し、プライマリになるまで待ちます:

<!-- mongodb-step: replica-set -->
```bash
docker exec speedle-mongodb mongosh --quiet --eval \
  'rs.initiate({_id: "speedle-rs", members: [{_id: 0, host: "127.0.0.1:27017"}]})'
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.hello().isWritablePrimary' | grep -qx true
do
  sleep 1
done
```

メンバーアドレス `127.0.0.1:27017` はクライアントに接続先として通知されるものであるため、PMS および ADS から到達可能である必要があります。ここではポートが同じループバックアドレスで公開されているため機能します。構成はボリュームに保存されます。再起動後に `rs.initiate` を再度実行しないでください。

## 4. 共有ストアの構成

このローカルの PMS/ADS ペア用の設定ファイルを 1 つ作成します:

<!-- mongodb-step: configuration -->
```bash
mkdir -p mongodb-demo
cat > mongodb-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://127.0.0.1:27017/?replicaSet=speedle-rs",
      "MongoDatabase": "speedle_demo"
    }
  },
  "enableWatch": true
}
JSON
```

PMS と ADS は**同じデプロイメントとデータベース**を使用する必要があります。独立したポリシーストアごとに個別のデータベースを使用します。ストアはサービスと関数を `services` および `functions` コレクションに保持します。MongoDB は最初の書き込み時にデータベースとコレクションを作成するため、スキーマの初期化は不要です。`enableWatch: true` はチェンジストリームを介した ADS の更新を有効にします。ポーリング間隔は構成されません。

## 5. PMS の起動

ターミナル A で PMS を起動し、実行したままにします:

<!-- mongodb-step: pms -->
```bash
./bin/speedle-pms --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

REST リスナーが起動するのを待ちます。このデモでは平文の HTTP がループバックにバインドされています。Speedle 独自の TLS および API 認可については [セキュリティ](/ja/docs/security/) を参照してください。

## 6. ポリシーの作成

ターミナル B で、同じチェックアウトから、Alice に `book` の読み取り権限を付与します:

<!-- mongodb-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mongodb-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS はサービスと生成されたポリシー ID を返します。ポリシーは MongoDB に保存されました。

## 7. ADS の起動と認可の確認

ターミナル C で、同じファイルを使用して ADS を起動し、実行したままにします:

<!-- mongodb-step: ads -->
```bash
./bin/speedle-ads --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

起動を待ってから、ターミナル B で以下のチェックを実行します。Alice は許可されます:

<!-- mongodb-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

期待値: `{"allowed":true,"reason":0}`。Bob には一致するグラントがありません:

<!-- mongodb-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

期待値: `{"allowed":false,"reason":3}`。

## 8. 保存されたドキュメントと自動更新の検証

サービスドキュメントを検査します。サービス名は `_id` です:

<!-- mongodb-step: documents -->
```bash
docker exec speedle-mongodb mongosh speedle_demo --quiet --eval \
  'db.services.find({}, {"policies.name": 1, "policies.effect": 1}).toArray()'
```

出力には `alice-read` グラントを含む `_id: 'mongodb-demo'` が含まれます。ドキュメントはバックエンドが管理するデータとして扱い、ポリシーの変更は PMS を通じて行ってください。

ADS を実行したまま、拒否（deny）を追加します:

<!-- mongodb-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mongodb-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

ステップ 7 の Alice のリクエストを繰り返します。非同期のチェンジストリーム更新後、`{"allowed":false,"reason":1}` が返されるはずです。同期がまだ進行中の場合はリクエストを再試行してください。この通常のポリシー更新のために ADS を再起動する必要はありません。

## 9. バックアップの保存と永続化の確認

ポリシーデータベースをアーカイブにダンプし、コンテナの外部にコピーします:

<!-- mongodb-step: backup -->
```bash
docker exec speedle-mongodb mongodump --quiet \
  --db speedle_demo --archive=/tmp/speedle-demo.archive
docker cp speedle-mongodb:/tmp/speedle-demo.archive ./mongodb-demo/speedle-demo.archive
```

アーカイブには `speedle_demo` データベースのみが含まれます。バックアップは独立した保護されたストレージに保存してください。リストアするには、ポリシーライターを停止し、プライマリに対して `mongorestore --drop --archive=...` を実行します。[MongoDB ツールを使用したバックアップとリストア](https://www.mongodb.com/docs/manual/tutorial/backup-and-restore-tools/) を参照してください。

この再起動チェックの前に、PMS と ADS を Ctrl+C で停止します:

<!-- mongodb-step: restart -->
```bash
docker restart speedle-mongodb
```

ステップ 2 と 3 の両方の待機ループを実行し（`rs.initiate` はスキップ）、同じ構成で PMS と ADS を再起動して、Alice のリクエストを再度実行します。引き続き `{"allowed":false,"reason":1}` が返されるはずです。名前付きボリュームは、コンテナの再起動後もデータとレプリカセットの構成を保持します。

リストア後は、決定を提供する前にリストアされたストアを読み込むように ADS を再起動してください。リカバリ後に認可を再確認してください。以前実行されていたキャッシュは、復元されたポリシーセットを含んでいることの証明にはなりません。

## 本番環境: 認証、TLS、およびレプリカセット

アクセス制御を有効にし、PMS と ADS に別個のユーザーを付与して、TLS 経由で接続します。PMS にはポリシーデータベースに対する `readWrite` 権限が必要です。ADS は読み取りと監視のみを行うため、組み込みの `read` ロールで十分であり、そのユーザーはポリシーを変更できません:

```javascript
use speedle
db.createUser({user: "speedle-pms", pwd: passwordPrompt(), roles: [{role: "readWrite", db: "speedle"}]})
db.createUser({user: "speedle-ads", pwd: passwordPrompt(), roles: [{role: "read", db: "speedle"}]})
```

次に、各サービスに独自の接続文字列を指定します:

```json
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://speedle-ads:replace-me@mongo1.example.com:27017,mongo2.example.com:27017,mongo3.example.com:27017/?replicaSet=speedle-rs&authSource=speedle&tls=true&tlsCAFile=/etc/speedle/mongodb/ca.pem",
      "MongoDatabase": "speedle"
    }
  },
  "enableWatch": true
}
```

CA パスは PMS/ADS マシン上に存在します。URI 内の認証情報はパーセントエンコードする必要があり、それらを含む設定ファイルはサービスアカウントのみが読み取り可能である必要があります。代わりに `SPDL_MONGOSTORE_URI` 経由で URI を指定することもできます。[MongoDB 接続文字列](https://www.mongodb.com/docs/manual/reference/connection-string/) のすべてのオプション（クライアント証明書 `tlsCertificateKeyFile` やその他の認証メカニズムを含む）は変更されずに Go ドライバーに渡されます。MongoDB への TLS は、Speedle の REST/gRPC リスナー上の TLS とは個別に構成されます。

レプリカセットのアクセス制御には、キーファイルまたは X.509 証明書を使用したメンバー間の認証も必要です。[キーファイルアクセス制御を使用したセルフマネージドレプリカセットのデプロイ](https://www.mongodb.com/docs/manual/tutorial/deploy-replica-set-with-keyfile-access-control/) および [TLS の構成](https://www.mongodb.com/docs/manual/tutorial/configure-ssl/) に関する公式ガイドを参照してください。

高可用性を確保するには、少なくとも 3 つのデータ保持メンバーを使用し、接続文字列に複数のメンバーをリストします。シングルノードのデモは、唯一のメンバーの喪失に耐えることができません。新しいプライマリが選出されている間、PMS 経由の書き込みが失敗する可能性があります。選出後に再試行してください。

## 構成リファレンスとトラブルシューティング

| プロパティ | フラグ | 環境変数 | デフォルト |
| --- | --- | --- | --- |
| `MongoURI` | `--mongostore_uri` | `SPDL_MONGOSTORE_URI` | `mongodb://localhost:27017` |
| `MongoDatabase` | `--mongostore_database` | `SPDL_MONGOSTORE_DATABASE` | `speedleplus` |

明示的なフラグは空でない環境変数をオーバーライドし、次に設定ファイル、最後にデフォルト値の順で優先されます。バックエンドを選択するには `--store-type mongodb` または `SPDL_STORE_TYPE=mongodb` を使用します。組み込み用途の場合、`pkg/store/mongodb` をインポートし、これらのプロパティを `store.NewStore` に渡します。

ストア運用時に知っておくべき動作:

- ストア全体の置換（`WritePolicyStore`）は 1 つのトランザクションで実行されます。指定されたポリシー ID を保持し、欠落している ID を生成します。スタンドアロンサーバーでは、既存のデータを変更することなく失敗します。
- すべてのサービスまたは関数を削除してもコレクションは保持されるため、チェンジストリームは開いたままとなり、ADS はキャッシュされたエントリを削除します。
- ストア全体の読み取りでは、レプリカセットまたはシャーディングされたクラスター上で 1 つのスナップショットとしてサービスと関数が読み取られます。
- チェンジストリームが終了した場合、ADS は最大 30 秒のバックオフで再オープンし、ストア全体を再読み込みするため、その間に行われた変更は失われません。それまでは最後に読み込まれたポリシーを引き続き提供します。
- 検出リクエストの収集とポリシー生成はサポートされていません。これらはファイルストアおよび etcd ストアで利用可能です。

| 症状 | 確認事項 |
| --- | --- |
| 起動時のサーバー選択タイムアウト | レプリカセットが初期化されプライマリであるか、`rs.conf()` 内のメンバーホストが PMS/ADS から到達可能か、URI ホストとポート、ファイアウォール。 |
| ADS が `The $changeStream stage is only supported on replica sets` で終了する | デプロイメントがスタンドアロンの `mongod` です。レプリカセットまたはシャーディングされたクラスターを使用してください。 |
| PMS は書き込むが ADS にサービスがない | 両方の構成で同じ URI デプロイメントおよび `MongoDatabase` 名であるか。 |
| 変更が ADS に届かない | watch が有効か。ADS ログにチェンジストリームおよび再オープンエラーがないか（ADS がデータベースに再到達するまで繰り返されます）。 |
| 認証失敗（Authentication failed） | ユーザー名、パスワードのエンコーディング、`authSource`、ポリシーデータベース上のロール。 |
| TLS エラー | `tls=true`、CA ファイルパス、証明書のホスト名。 |

バックエンドインターフェースと構成の優先順位については、[プラグイン可能なストレージ](/ja/docs/store/) に戻って参照してください。

ストアの障害は、すべての組み込みストアと同じポリシーに従います。ADS は利用不能を検出した後にデフォルトでエラー付きの拒否を行い、実行を継続して再試行し、完全な再読み込みが成功した後に再開します。オプションの有限の猶予期間および失効遅延については、[ストア障害とキャッシュされた認可](/ja/docs/store/#store-failures-and-cached-authorization) を参照してください。
