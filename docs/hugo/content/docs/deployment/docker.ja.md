+++
title = "Docker イメージ"
description = "公開されている Speedle イメージから speedle-pms、speedle-ads、spctl を実行"
weight = 20
aliases = ["/ja/docs/sdks/docker/"]
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Docker Hub と GHCR 上の同一イメージが、共有ファイルストアを使用してポリシーマネージャー、決定サービス、spctl を実行します。"
lastmod = 2026-11-14T10:30:00+08:00
+++

Speedle は、`speedle-pms`、`speedle-ads`、および `spctl` を含む 1 つのイメージを公開しています。イメージは `FROM scratch` でビルドされ、UID 65532 として実行され、シェルは含まれていません。最初の引数でプログラムを選択します。

両方のレジストリに同一のイメージインデックスがあります:

- Docker Hub: `docker.io/speedle/speedle`
- GHCR: `ghcr.io/teramoby/speedle`

リリース 1.7.0 では、`linux/amd64` および `linux/arm64` 向けに、両方のレジストリでタグ `1.7.0`、`1.7`、および `latest` が公開されています。

## イメージのプル

```bash
docker pull docker.io/speedle/speedle:1.7.0
docker pull ghcr.io/teramoby/speedle:1.7.0
```

`docker pull` はインデックスダイジェストを出力します。1.7.0 の場合、そのダイジェストは両方のレジストリで `sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0` です。

## タグの選択

`1.7.0` はこのリリースを指定します。より新しい 1.7 パッチが公開されると `1.7` が移動します。`master` の最新コミットから新しいリリースが公開されると `latest` が移動します。リリースの再試行で同じバージョンダグが再度公開される可能性があるため、タグは不変のビルドではありません。

インデックスダイジェストを確認するには:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0 --format '{{.Manifest.Digest}}'
```

1.7.0 リリースの場合、両方のレジストリ上の `1.7.0`、`1.7`、および `latest` はすべて以下を出力します:

```text
sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0
```

`1.7` または `latest` をデプロイする前に、ダイジェストを再度確認してください。単一のビルドに固定するには、イメージ名に `@` とそのダイジェストを組み合わせて使用します（例: `docker.io/speedle/speedle@sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`）。

## イメージ内のコマンド

引数なしで実行すると、イメージは 3 つのプログラム名を出力します:

```bash
docker run --rm docker.io/speedle/speedle:1.7.0
```

```text
speedle-pms
speedle-ads
spctl
```

`docker run --rm ghcr.io/teramoby/speedle:1.7.0` も同じ行を出力します。以下のコマンドでは Docker Hub の名前を使用しています。GHCR のコピーを使用する場合は GHCR の名前に置き換えてください。イメージ内にシェルはないため、`docker exec` では `/spctl`、`/speedle-pms`、または `/speedle-ads` を絶対パスで呼び出す必要があります。

## PMS と ADS の実行

名前付きボリュームを作成し、ポリシーマネージャーを起動します。サーバーはコンテナ内の `0.0.0.0:6733` でリッスンします。`127.0.0.1:6733` を公開することで、未認証の管理 API をこのマシン内のみに制限します。

```bash
docker volume create speedle-policies
docker run -d --name speedle-pms \
  -p 127.0.0.1:6733:6733 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
```

新しい名前付きボリュームには、UID 65532 が書き込み可能なイメージの `/data` ディレクトリの内容が入力されます。`/data/policies.json` が見つからない場合、PMS が作成します。イメージには公開 CA 証明書バンドルが含まれています。1.7.0 と 1.7.1 はこのバンドルが追加される前に公開されたため、これらのタグで TLS エンドポイントに接続する場合は CA ファイルを指定してください。`--insecure=true` はこのローカル HTTP サンプル用です。TLS については [セキュリティ](/ja/docs/security/) を参照してください。

Docker は公開されたポートを受け入れ、プロセスがリッスンを開始する前に接続をリセットする場合があります。そのリセットを含め、すべてのエラーで再試行してください:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
```

最初の試行で `curl: (56) Recv failure: Connection reset by peer` が出力されることがあります。その後の試行でレスポンスボディが返されます。空のストアは以下を応答します:

```text
[]
```

同じボリューム上で ADS を起動します。ADS は PMS が書き込んだファイル（その後の更新を含む）を読み取ります。

```bash
docker run -d --name speedle-ads \
  -p 127.0.0.1:6734:6734 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

## ヘルスチェックと準備状態の確認

PMS は `/healthz` を提供しません。上記のように `GET /policy-mgmt/v1/service` が成功することは、管理 API がリクエストを受け付けていることを意味します。

ADS は liveness と readiness を提供します:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
```

```text
{"status":"ok"}
{"ready":true,"servicesTotal":0,"servicesUsable":0}
```

プロセスが実行されている間、`/healthz` は `200 {"status":"ok"}` を返します。`/readyz` は、ADS がリクエストを評価できる場合に `200` を返します。ファイルストアには分散状態がないため、サービスカウントはゼロのままになります。確認すべきフィールドは `ready` です。イメージにはシェルも `curl` もないため、これらのプローブはホスト上またはオーケストレーター内で公開されたポートに対して実行します。`http://localhost:6734/healthz` でも同じ公開ポートに到達します。

## spctl によるポリシーの作成

実行中の PMS コンテナ内の `/spctl` は、そのコンテナ自身のポートで PMS と通信します:

```bash
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

```text
service created
{"name":"docs","type":"application","metadata":{"createtime":"2026-11-10T03:30:03Z"}}
policy created
{"id":"zhtbsgm54iiuwky4zgmq","name":"","effect":"grant","permissions":[{"resource":"/docs/public","actions":["get","list"]}],"principals":[["user:alice"]],"metadata":{"createtime":"2026-11-10T03:30:04Z"}}
```

PMS が `createtime` とポリシー `id` を割り当てるため、これらの値は実行ごとに異なります。これは [SDK 概要](/ja/docs/sdks/#example-policy) と同じサービスおよびポリシーです。

同じ `spctl` バイナリは、PMS のネットワーク名前空間を共有する 2 つ目のコンテナでも実行できます:

```bash
docker run --rm --network container:speedle-pms \
  docker.io/speedle/speedle:1.7.0 \
  spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

## ADS への認可決定リクエスト

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":true,"reason":0}
```

`reason` `0` は `GRANT_POLICY_FOUND` です。同じポリシーは `delete` を許可していません:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"delete"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":false,"reason":3}
```

`reason` `3` は `NO_APPLICABLE_POLICIES` です。ADS がすでに実行されている間に PMS 経由で書き込まれたポリシーは、次のリクエストで反映されます。リクエストフィールドについては [認可決定](/ja/docs/decisions/) を参照してください。

## SDK から ADS への接続

SDK ページでは `http://localhost:6734` を呼び出します。上記でポートを公開しているため、そのアドレスはこの ADS を指します。以下の Node.js クライアントは、コンテナが公開しているアドレスである `http://127.0.0.1:6734` を使用します。まずサンプルディレクトリを作成し、そのディレクトリにクライアントを `main.mjs` として保存します:

```bash
mkdir speedle-example
cd speedle-example
```

```javascript
import { Auth, SpeedleClient } from "@speedle/sdk";

const speedle = new SpeedleClient({
  backend: "speedle",
  endpoint: "http://127.0.0.1:6734",
  auth: Auth.none(),
  serviceName: "docs",
});
try {
  const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
  console.log(`alice get /docs/public: allowed=${allowed}`);
} finally {
  await speedle.close();
}
```

`speedle-example` に公開パッケージをインストールし、そこで `main.mjs` を実行します。インストールは [Node.js SDK](/ja/docs/sdks/node/) ページと同じバージョンに固定されています:

```bash
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
node main.mjs
```

```text
alice get /docs/public: allowed=true
```

他の言語のページでも同じエンドポイントとサービス名を使用します。SDK はループバックホストに対してのみ平文の `http://` を受け入れます。

## Compose による両サービスの実行

このファイルを `compose.yaml` として保存します。これは、同じ 2 つのプロセスと同じボリュームを実行するもう 1 つの方法です。前のセクションのコンテナがまだ実行されている場合は、ポート 6733 と 6734 が解放されるように先に停止してください。そのボリュームを削除すると、そのポリシーファイルも削除されます。

```yaml
services:
  pms:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-pms
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6733
      - --insecure=true
    ports:
      - 127.0.0.1:6733:6733
    volumes:
      - speedle-policies:/data
  ads:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-ads
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6734
      - --insecure=true
    ports:
      - 127.0.0.1:6734:6734
    volumes:
      - speedle-policies:/data
volumes:
  speedle-policies:
```

```bash
docker compose -f compose.yaml up -d
```

`up -d` は PMS がリッスンする前に戻ります。何かを作成する前に、PMS サービスリストと ADS の `/healthz` および `/readyz` を待機します。上記で説明したように、最初の PMS 試行では接続がリセットされる場合があります。

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

同じ `is-allowed` リクエストを送信します。ADS は再起動なしで新しいポリシーを認識します。

```bash
docker compose -f compose.yaml down -v
```

`down -v` は Compose ボリュームとその中に保存されているポリシーを削除します。

## プロベナンスの検証

公開ビルドにはプロベナンスと SBOM が添付されています（`docker buildx build --provenance=mode=max --sbom=true`）。インデックスを確認します:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0
```

インデックスには `linux/amd64`、`linux/arm64`、および 2 つの `unknown/unknown` マニフェストが一覧表示されます。これらの未知のプラットフォームはアテステーションマニフェストです。それぞれ `vnd.docker.reference.type` を `attestation-manifest` に設定し、`vnd.docker.reference.digest` を説明対象のプラットフォームイメージに設定しています。`docker buildx imagetools inspect ghcr.io/teramoby/speedle:1.7.0` も同じプラットフォームおよびアテステーションのダイジェストを出力します。

## コンテナの停止

```bash
docker rm -f speedle-pms speedle-ads
docker volume rm speedle-policies
```

`docker volume rm` はポリシーファイルを削除します。プロセスを Compose で起動した場合は、代わりに `docker compose -f compose.yaml down -v` を使用してください。
