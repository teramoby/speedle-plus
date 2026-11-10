+++
title = "MySQL セットアップ"
description = "PMS および ADS 向けの MySQL 8.4 ポリシーストアの設定手順"
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mysql"]
categories = ["docs"]
bref = "データベースを作成し、PMS と ADS を構成して、認可と自動ポリシー同期を確認します。"
+++

このガイドでは、**MySQL 8.4 LTS** と 2 つのネイティブ Speedle プロセスを使用します。PMS はポリシーを書き込み、ADS は個別の読み取り専用アカウントで同じデータベースを読み取ります。ローカルの例では Docker とループバック上の平文 HTTP を使用します。リモートデプロイの場合は、既存の MySQL サーバーと [セキュリティガイド](/ja/docs/security/) を使用してください。

## 1. MySQL バックエンドを含む Speedle のビルド

Git、Bash、curl、Docker、およびサポートされている Go ツールチェーン（Go 1.26.10 または Go 1.27.3）をインストールします。Docker が実行されている必要があります。両方のバイナリに `mysql` ストアが含まれるように、最新のソースからビルドします。以前インストールされた古いリリースには含まれていない可能性があります。

<!-- mysql-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

残りのコマンドはこの `speedle-plus` ディレクトリから実行します。ポート 3306、6733、6734 を解放しておくか、以下のすべてのコマンドおよび構成で対応するアドレスを変更してください。

## 2. MySQL 8.4 の起動

以下の固定パスワードは**ローカルデモ用の認証情報**です。デプロイ時には独自の認証情報とシークレット配信メカニズムを使用してください。名前付き Docker ボリュームは、このコンテナが停止または再作成された場合でもデータベースを保持します。

<!-- mysql-step: database -->
```bash
docker run --name speedle-mysql --detach \
  --publish 127.0.0.1:3306:3306 \
  --volume speedle-mysql-data:/var/lib/mysql \
  --env MYSQL_ROOT_PASSWORD=SpeedleDemoRoot84 \
  mysql:8.4
```

初期化が完了するのを待ちます:

<!-- mysql-step: ready -->
```bash
until docker exec speedle-mysql mysqladmin ping \
  --host=127.0.0.1 --user=root --password=SpeedleDemoRoot84 --silent
do
  sleep 1
done
```

すでに MySQL 8.4 を実行している場合は、これらの Docker コマンドをスキップし、データベース管理者のクライアントを使用して次のステップの SQL を実行してください。Docker イメージは、データディレクトリが空の場合にのみ認証情報を初期化します。環境変数を変更しても、既存のボリューム内のパスワードはリセットされません。

## 3. データベースとサービスアカウントの作成

Speedle はデフォルトでテーブルを作成しますが、**データベースは作成しません**。管理者を使用して `speedle` データベースと 2 つのアカウントを作成します:

<!-- mysql-step: accounts -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 <<'SQL'
CREATE DATABASE speedle CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
CREATE USER 'speedle_pms'@'%' IDENTIFIED BY 'SpeedleDemoPms84';
CREATE USER 'speedle_ads'@'%' IDENTIFIED BY 'SpeedleDemoAds84';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, REFERENCES ON speedle.* TO 'speedle_pms'@'%';
GRANT SELECT ON speedle.* TO 'speedle_ads'@'%';
SQL
```

PMS には、自動スキーマ初期化のための `CREATE` と `REFERENCES`、およびポリシー管理のための `SELECT`、`INSERT`、`UPDATE`、`DELETE` が必要です。ADS には `SELECT` のみが必要であり、自動スキーマ初期化を無効にする必要があります。アカウントホストの `%` は、Docker 経由でホストからの接続を許可します。リモートデプロイの場合は、アカウントホストを実際の PMS/ADS マシンに制限してください。

## 4. PMS および ADS 設定ファイルの作成

両方の DSN は**同じサーバーとデータベース**を選択する必要があります。DSN はドライバーの構文 `user:password@tcp(host:port)/database` に従います。

<!-- mysql-step: configuration -->
```bash
mkdir -p mysql-demo
cat > mysql-demo/pms.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_pms:SpeedleDemoPms84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  }
}
JSON
cat > mysql-demo/ads.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_ads:SpeedleDemoAds84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": false,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
JSON
chmod 600 mysql-demo/pms.json mysql-demo/ads.json
```

`MySQLInitSchema: false` は、起動時に DDL を実行せずに既存のスキーマをチェックします。`enableWatch: true` は、ADS がコミットされた変更に追従できるようにします。これらの認証情報を含む設定ファイルをコミットしないでください。

## 5. PMS、続いて ADS の起動

ターミナル A で、テーブルを作成できるようにまず PMS を起動します:

<!-- mysql-step: pms -->
```bash
./bin/speedle-pms --config-file mysql-demo/pms.json --endpoint 127.0.0.1:6733 --insecure=true
```

PMS を実行したままにします。ターミナル B で、同じチェックアウトから初期化を確認します:

<!-- mysql-step: tables -->
```bash
docker exec speedle-mysql mysql --user=speedle_ads --password=SpeedleDemoAds84 speedle -e 'SHOW TABLES;'
```

5 つのテーブルが表示されるはずです:

| テーブル | 内容 |
| --- | --- |
| `speedle_services` | サービスおよびサービスメタデータ |
| `speedle_policies` | 認可ポリシー |
| `speedle_role_policies` | ロールポリシー |
| `speedle_functions` | カスタム関数定義 |
| `speedle_store_meta` | スキーマバージョンおよびコミットされたポリシーリビジョン |

次に、ターミナル B で ADS を起動し、実行したままにします:

<!-- mysql-step: ads -->
```bash
./bin/speedle-ads --config-file mysql-demo/ads.json --endpoint 127.0.0.1:6734 --insecure=true
```

続行する前に、各サービスの REST リスナーが起動するのを待ちます。`--grpc-endpoint` を渡さない限り、gRPC サーバーは停止したままです。

## 6. ポリシーの作成と認可の確認

ターミナル C で、Alice に `book` の読み取り権限を付与するポリシーを持つサービスを作成します:

<!-- mysql-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mysql-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

ADS が変更を読み込むまで、最大でポーリング間隔（デフォルトで 1 秒）にクエリ/キャッシュ再構築時間を加えた時間を見込みます。同期がまだ進行中の場合はリクエストを再試行してください:

<!-- mysql-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

期待されるレスポンス: `{"allowed":true,"reason":0}`。Bob には一致するグラントがありません:

<!-- mysql-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

期待されるレスポンス: `{"allowed":false,"reason":3}`。

## 7. 自動更新と永続化の確認

ADS を再起動することなく、PMS 経由で Alice に対する明示的な拒否を追加します:

<!-- mysql-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mysql-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

同期後、ステップ 6 の Alice のリクエストを繰り返します。明示的な拒否が優先されるため、今度は `{"allowed":false,"reason":1}` が返されるはずです。PMS と ADS を Ctrl+C で停止し、ステップ 5 のコマンドで再起動して、同じリクエストを繰り返すことができます。保存されたポリシーと拒否設定は MySQL 内に永続化されます。

ポリシーの変更は、データとリビジョンが同じトランザクションで進むように、PMS またはストアインターフェースを経由する必要があります。SQL で直接編集しても、ADS の再読み込みは自動的にトリガーされません。データベース停止中、ADS は実行を継続し同期を再試行しますが、障害が検出されるとデフォルトで `allowed:false` とエラーを返します。認可が再開される前に、完全なスナップショットが正常に読み込まれる必要があります。共有の [ストア障害と猶予期間ポリシー](/ja/docs/store/#store-failures-and-cached-authorization) を参照してください。ADS の起動には依然として到達可能なデータベースが必要です。

## ランタイム DDL 権限なしでのスキーマ事前作成

自動テーブル作成の代替手段として、いずれかのサービスを開始する前に、プロビジョニングアカウントを使用してリポジトリの [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql) を実行します:

<!-- mysql-step: schema -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 speedle < pkg/store/mysql/schema.sql
```

**両方**の設定ファイルで `MySQLInitSchema` を `false` に設定します。上記のデモに従った場合は、追加の初期化権限を取り消します:

<!-- mysql-step: revoke-ddl -->
```bash
docker exec speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 -e "REVOKE CREATE, REFERENCES ON speedle.* FROM 'speedle_pms'@'%';"
```

PMS は `SELECT`、`INSERT`、`UPDATE`、`DELETE` で実行され、ADS は読み取り専用のままになります。構成を変更した後は両方のサービスを再起動してください。スキーマ初期化は既存のテーブルを移行しません。サポートされていないスキーマバージョンは起動に失敗します。

## 構成リファレンスとトラブルシューティング

| プロパティ | フラグ | 環境変数 | デフォルト |
| --- | --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | `SPDL_MYSQLSTORE_DSN` | 必須（データベース名を含む） |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `SPDL_MYSQLSTORE_INIT_SCHEMA` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `SPDL_MYSQLSTORE_POLL_INTERVAL` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `SPDL_MYSQLSTORE_QUERY_TIMEOUT` | `10s` |

選択フラグは `--store-type mysql` (`SPDL_STORE_TYPE=mysql`) です。明示的なフラグは空でない環境変数をオーバーライドし、次に設定ファイル、最後にデフォルト値の順で優先されます。リモート MySQL の場合は、信頼できるサーバー証明書を持つホスト名を使用し、DSN に `?tls=true` を追加します。カスタム CA の構成は [ドライバー TLS ドキュメント](https://github.com/go-sql-driver/mysql/tree/v1.10.1#tls) に従います。

| 症状 | 確認事項 |
| --- | --- |
| 不明な `mysql` ストアまたはフラグ | `pkg/store/mysql` を含むソースから両方のバイナリを再ビルドしてください。 |
| 接続が拒否される / アクセス拒否 | データベースの準備状態、DSN のホスト/ポート、パスワード、および MySQL アカウントのホスト制限。 |
| 不明なデータベース | Speedle はテーブルのみを作成するため、先にデータベースを作成してください。 |
| テーブルがない、または DDL が拒否される | PMS を先に起動するか、スキーマを事前作成してください。読み取り専用の ADS では `MySQLInitSchema` を `false` に設定する必要があります。 |
| ポリシーの変更が反映されない | 両方の DSN で同じデータベースであるか、ADS watch が有効か、ポーリング遅延、オーバーライドする `SPDL_*` 変数がないか。 |

MySQL は検出リクエストの収集やポリシー生成をサポートしていません。トランザクション、スナップショット、およびリカバリの詳細については、[プラグイン可能なストレージ](/ja/docs/store/) を参照してください。
