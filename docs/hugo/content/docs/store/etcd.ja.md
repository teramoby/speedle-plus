+++
title = "etcd セットアップ"
description = "PMS および ADS 向けの etcd ポリシーストアの設定手順"
weight = 20
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "etcd"]
categories = ["docs"]
bref = "独立した etcd サーバーを実行し、PMS と ADS を接続してライブポリシーの更新を確認し、バックアップを保存します。"
lastmod = 2026-10-31T22:36:36+08:00
+++

このウォークスルーでは、永続的な Docker ボリュームを備えた**外部の etcd v3.6.14** サーバーを使用します。PMS はポリシーを書き込み、ADS は同じキープレフィックスを監視（watch）します。シングルノードのサンプルはローカル開発用にループバックでリッスンし、平文の HTTP を使用します。本番環境の TLS とクラスター構成はウォークスルーの後に説明します。

## 1. Speedle のビルド

Bash、Git、curl、Docker、および Go 1.26.10 または Go 1.27.3 を準備します。Docker を起動し、ネイティブサービスをビルドします:

<!-- etcd-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

後続のコマンドはこのチェックアウトから実行します。ローカルポート 2379、6733、6734 を利用可能にしておくか、コマンドと構成を同時に更新してください。

## 2. 外部 etcd サーバーの起動

名前付きボリュームが etcd のデータを保持します。ホストにはクライアントポートのみが公開され、ピアポートはこのシングルノードコンテナ内に留まります。

<!-- etcd-step: database -->
```bash
docker run --name speedle-etcd --detach \
  --publish 127.0.0.1:2379:2379 \
  --volume speedle-etcd-data:/etcd-data \
  gcr.io/etcd-development/etcd:v3.6.14 \
  /usr/local/bin/etcd \
  --name speedle-local \
  --data-dir /etcd-data \
  --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://0.0.0.0:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster speedle-local=http://127.0.0.1:2380 \
  --initial-cluster-token speedle-local-demo \
  --initial-cluster-state new
```

v3 API が正常（healthy）になるのを待ちます:

<!-- etcd-step: ready -->
```bash
until docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 endpoint health
do
  sleep 1
done
```

すでに etcd を実行している場合は、コンテナコマンドをスキップし、以下のクライアントエンドポイントを使用してください。Docker ネットワークの詳細については、[公式コンテナガイド](https://etcd.io/docs/v3.6/op-guide/container/) を参照してください。

## 3. 共有ストアの構成

このローカルの PMS/ADS ペア用の設定ファイルを 1 つ作成します:

<!-- etcd-step: configuration -->
```bash
mkdir -p etcd-demo
cat > etcd-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "http://127.0.0.1:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false
    }
  },
  "enableWatch": true
}
JSON
```

PMS と ADS は**同じエンドポイントとキープレフィックス**を使用する必要があります。`/speedle_ps/` の末尾のスラッシュを保持してください。独立したポリシーストアごとに個別のプレフィックスを使用します。`IsEmbeddedEtcd: false` は外部サーバーに接続します。`enableWatch: true` は etcd watch イベントによる ADS の更新を有効にします。ポーリング間隔は設定されません。

## 4. PMS の起動

ターミナル A で PMS を起動し、実行したままにします:

<!-- etcd-step: pms -->
```bash
./bin/speedle-pms --config-file etcd-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

REST リスナーが起動するのを待ちます。このデモでは平文の HTTP がループバックにバインドされています。Speedle 独自の TLS および API 認可については [セキュリティ](/ja/docs/security/) を参照してください。

## 5. ポリシーの作成

ターミナル B で、同じチェックアウトから、Alice に `book` の読み取り権限を付与します:

<!-- etcd-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"etcd-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS はサービスと生成されたポリシー ID を返します。ポリシーは etcd 内に保存されました。SQL スキーマや手動のキー初期化は不要です。

## 6. ADS の起動と認可の確認

ターミナル C で、同じファイルを使用して ADS を起動し、実行したままにします:

<!-- etcd-step: ads -->
```bash
./bin/speedle-ads --config-file etcd-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

起動を待ってから、ターミナル B で以下のチェックを実行します。Alice は許可されます:

<!-- etcd-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

期待値: `{"allowed":true,"reason":0}`。Bob には一致するグラントがありません:

<!-- etcd-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

期待値: `{"allowed":false,"reason":3}`。

## 7. 保存されたキーと自動更新の検証

このポリシーストア内のキーのみを検査します:

<!-- etcd-step: keys -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 get /speedle_ps/ --prefix --keys-only
```

出力には `/speedle_ps/services/` 配下の `etcd-demo` のキーが含まれます。キーはバックエンドが管理するデータとして扱い、ポリシーの変更は PMS を通じて行ってください。

ADS を実行したまま、拒否（deny）を追加します:

<!-- etcd-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/etcd-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

ステップ 6 の Alice のリクエストを繰り返します。非同期の watch 更新後、`{"allowed":false,"reason":1}` が返されるはずです。同期がまだ進行中の場合はリクエストを再試行してください。この通常のポリシー更新のために ADS を再起動する必要はありません。

## 8. スナップショットの保存と永続化の確認

ライブスナップショットを取得し、確認して、コンテナの外部にコピーします:

<!-- etcd-step: snapshot -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 snapshot save /etcd-data/speedle-snapshot.db
docker exec speedle-etcd /usr/local/bin/etcdutl \
  snapshot status /etcd-data/speedle-snapshot.db --write-out=table
docker cp speedle-etcd:/etcd-data/speedle-snapshot.db ./etcd-demo/speedle-snapshot.db
```

スナップショットには、他のプレフィックスを含む **etcd クラスター全体のキースペース**が含まれます。バックアップは独立した保護されたストレージに保存してください。リストアとメンバーシップの手順については、[ディザスタリカバリ](https://etcd.io/docs/v3.6/op-guide/recovery/) を参照してください。

この再起動チェックの前に、PMS と ADS を Ctrl+C で停止します:

<!-- etcd-step: restart -->
```bash
docker restart speedle-etcd
```

ステップ 2 のヘルスチェックを実行し、同じ構成で PMS と ADS を再起動して、Alice のリクエストを再度実行します。引き続き `{"allowed":false,"reason":1}` が返されるはずです。名前付きボリュームは、コンテナの再起動後もデータを保持します。

ディザスタリカバリの場合は、ポリシーライターを停止し、etcd の説明に従って新しいデータディレクトリにスナップショットをリストアし、決定を提供する前にリストアされたストアを読み込むように ADS を再起動します。リカバリ後に認可を再確認してください。以前実行されていたキャッシュは、復元されたポリシーセットを含んでいることの証明にはなりません。

## 本番環境: TLS、認証、およびクラスター

信頼できるサーバー証明書、エンドポイントホスト名と一致する証明書 SAN、およびクライアント証明書検証を使用して etcd をデプロイします。その後、HTTPS とクライアント認証情報を使用するように PMS/ADS を構成します:

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "https://etcd.example.com:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false,
      "EtcdTLSTrustedCAFile": "/etc/speedle/etcd/ca.crt",
      "EtcdTLSCertFile": "/etc/speedle/etcd/client.crt",
      "EtcdTLSKeyFile": "/etc/speedle/etcd/client.key",
      "EtcdTLSServerName": "etcd.example.com",
      "EtcdTLSInsecureSkipVerify": false
    }
  },
  "enableWatch": true
}
```

これらのパスは PMS/ADS マシン上に存在します。秘密鍵へのアクセスはサービスアカウントのみに制限してください。etcd への TLS は、Speedle の REST/gRPC リスナー上の TLS とは個別に構成されます。

アダプターは証明書オプションを公開しており、etcd のユーザー名/パスワードプロパティは公開していません。etcd RBAC の場合は、`--client-cert-auth=true` と認証を有効にして、クライアント証明書の Common Name をユーザーアイデンティティとして使用します。PMS にはそのプレフィックスへの読み取り/書き込みアクセスを許可し、ADS には有効な機能が必要とするアクセス許可を付与します。サーバー構成とロールについては、公式の [TLS](https://etcd.io/docs/v3.6/op-guide/security/) および [RBAC](https://etcd.io/docs/v3.6/op-guide/authentication/rbac/) ガイドを参照してください。キープレフィックスはデータをグループ化し、アクセス制御には RBAC が必要です。

高可用性を確保するには、公式の [3 ノードクラスターの例](https://etcd.io/docs/v3.6/op-guide/container/#running-a-3-node-etcd-cluster) に従い、クライアントとピアのトラフィックを保護してください。シングルノードのデモは、唯一のメンバーの喪失に耐えることができません。現在の `EtcdEndpoint` プロパティは 1 つのエンドポイント文字列を受け入れます。デプロイメントに到達可能なクライアント URL を指定してください。

## 構成リファレンスとトラブルシューティング

| プロパティ | フラグ | 環境変数 | デフォルト |
| --- | --- | --- | --- |
| `EtcdEndpoint` | `--etcdstore-endpoint` | `SPDL_ETCDSTORE_ENDPOINT` | `localhost:2379` |
| `EtcdKeyPrefix` | `--etcdstore-keyprefix` | `SPDL_ETCDSTORE_KEYPREFIX` | `/speedle_ps/` |
| `IsEmbeddedEtcd` | `--etcdstore-isembedded` | `SPDL_ETCDSTORE_ISEMBEDDED` | `false` |
| `EmbeddedEtcdDataDir` | `--etcdstore-embeddedDataDir` | `SPDL_ETCDSTORE_EMBEDDEDDATADIR` | 空（組み込みモードのみ） |
| `EtcdTLSTrustedCAFile` | `--etcdstore-tls-ca` | `SPDL_ETCDSTORE_TLS_CA` | 空 |
| `EtcdTLSCertFile` | `--etcdstore-tls-cert` | `SPDL_ETCDSTORE_TLS_CERT` | 空 |
| `EtcdTLSKeyFile` | `--etcdstore-tls-key` | `SPDL_ETCDSTORE_TLS_KEY` | 空 |
| `EtcdTLSServerName` | `--etcdstore-tls-serverName` | `SPDL_ETCDSTORE_TLS_SERVERNAME` | 空 |
| `EtcdTLSInsecureSkipVerify` | `--etcdstore-tls-insecureSkipVerify` | `SPDL_ETCDSTORE_TLS_INSECURESKIPVERIFY` | `false` |

明示的なフラグは空でない環境変数をオーバーライドし、次に設定ファイル、最後にデフォルト値の順で優先されます。バックエンドを選択するには `--store-type etcd` または `SPDL_STORE_TYPE=etcd` を使用します。追加の TLS オプションには `EtcdTLSAllowedCN` (`--etcdstore-tls-allowedCN`) および `EtcdTLSCRLFile` (`--etcdstore-tls-crlFile`) があります。

組み込みモード (`IsEmbeddedEtcd: true`) はローカルの開発/テスト用であり、固定されたローカル etcd ポートを使用します。このガイドでは、データディレクトリとライフサイクルを明示的に管理できるように、etcd を PMS/ADS から独立させています。

| 症状 | 確認事項 |
| --- | --- |
| 接続が拒否される、または起動が停止する | etcd エンドポイントの健全性、クライアントポート、ホスト/コンテナのアドレス、ファイアウォール。 |
| PMS は書き込むが ADS にサービスがない | 末尾のスラッシュを含め、同じエンドポイントとキープレフィックスであるか。 |
| 変更が ADS に届かない | watch が有効か、ADS ログに接続および watch エラーがないか。リカバリ後に ADS を再起動して再読み込みしてください。 |
| TLS ホスト名または CA エラー | HTTPS エンドポイント、証明書 SAN、信頼できる CA パス、サーバー名設定。 |
| 権限拒否（Permission denied） | クライアント証明書のアイデンティティ、有効な RBAC ロール、プレフィックスのアクセス許可。 |

バックエンドインターフェースと構成の優先順位については、[プラグイン可能なストレージ](/ja/docs/store/) に戻って参照してください。

ストアの障害は、すべての組み込みストアと同じポリシーに従います。ADS は利用不能を検出した後にデフォルトでエラー付きの拒否を行い、実行を継続して再試行し、完全な再読み込みが成功した後に再開します。オプションの有限の猶予期間および失効遅延については、[ストア障害とキャッシュされた認可](/ja/docs/store/#store-failures-and-cached-authorization) を参照してください。
