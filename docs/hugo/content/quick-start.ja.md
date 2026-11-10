+++
title = "クイックスタート"
description = "go install またはソースコードから Speedle をインストールし、ポリシー管理、直接認可、ロール認可、条件付き（ABAC）決定、拒否を検証します。"
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "go install またはソースビルドで Speedle をインストールし、SPDL と spctl でポリシーを作成し、REST API で認可決定をテストします。このガイドではローカルファイルストアを使用します。"
icon = "9. Getting Started.svg"
lastmod = 2026-11-10T17:04:32+08:00
+++

## まずは Playground で体験

**インストール不要で、Speedle の基本機能をすぐに試すことができます。** [Playground](../playground/) を開いて SPDL ポリシーを編集し、ユーザー、リソース、アクションを設定すると、ブラウザ上ですべて完結して実際の Allow/Deny（許可／拒否）決定を確認できます。Go のインストールやサーバーの起動は必要ありません。

ローカル環境に Speedle をインストールして実行する準備ができたら、以下の手順に進んでください。

## 前提条件

Linux または macOS 環境、Bash、Git、curl、および **Go 1.26.10 または Go 1.27.3**（または同じリリースブランチの新しいパッチバージョン）を使用してください。Speedle は Go 1.26 および Go 1.27 のパッチリリースをサポートしています。このガイドは **Go 1.26.10 および Go 1.27.3** で検証されています。このウォークスルーには Go 1.27.3 が推奨安定バージョンです。

[公式インストール手順](https://go.dev/doc/install) に従って Go をインストールし、バージョンを確認します:

<!-- quick-start: version -->
```bash
go version
```

Go は `GOROOT` とデフォルトの `GOPATH` を自動的に構成します。カスタム値は不要です。以下のどちらの手順でも、3つの実行可能ファイルが `bin` ディレクトリに配置されます。

## Speedle のインストール

どちらか1つのルートを選択してください: ルート A は最新リリースをインストールし、ルート B は現在のソースコードからビルドします。ガイドの残りの部分では `./bin/...` を使用し、どちらのルートでも同様に動作します。

### ルート A: 最新リリースのインストール

<!-- quick-start: install -->
```bash
GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@v1.7.1
ls bin
```

`GOBIN` により、実行可能ファイルがこのディレクトリの `bin` フォルダー内に保持されます。指定しない場合、Go は `$(go env GOPATH)/bin` に書き込みます。`v1.7.1` は執筆時点での最新リリースタグです。`go install` はタグ付けされたソースからビルドし、Go チェックサムデータベースと照合して検証します。

### ルート B: クローンしてビルド

<!-- quick-start: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

出力ディレクトリには以下が含まれます:

```text
spctl  speedle-ads  speedle-pms
```

- `spctl`: コマンドラインインターフェース
- `speedle-pms`: ポリシー管理サービス (PMS)
- `speedle-ads`: 認可決定サービス (ADS)

> **macOS:** `/usr/sbin/spctl` は Apple の Gatekeeper ツールであり、単に `spctl` を実行した際に選択される場合があります。Speedle の CLI を呼び出す際はフルパスを指定するか、ファイルが含まれるディレクトリから `./spctl` を使用してください。このガイドでは `./bin/spctl` を使用します。シェルがどの実行可能ファイルを選択するか確認するには `command -v spctl` を実行してください。ダウンロードしたリリースに `speedle-spctl_<os>_<arch>` アセットが含まれている場合、それは Apple のツールと衝突しない名前の同じ CLI です。

最新の Go では、実行可能ファイルのビルドに `go build` または `go install` を使用します。`go get` はモジュール依存関係の管理に使用されます。

## Speedle の実行

ローカルの開発マシンでこのウォークスルーを実行します。`--insecure=true` を指定すると、REST の例でプレーンな HTTP が有効になります。`--grpc-endpoint` を指定して起動しない限り gRPC サーバーは停止したままです（例: PMS 用に `--grpc-endpoint 127.0.0.1:50001`、ADS 用に `--grpc-endpoint 127.0.0.1:50002`）。gRPC は REST と同じ TLS 設定を使用します。本番環境のトランスポートおよび認可設定については、[セキュリティ](../docs/security/) を参照してください。

### 1. ポリシー管理サービスを起動

最初のターミナルで、`bin/` を含むディレクトリ（ルート B のチェックアウトディレクトリ）から実行します:

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

起動したままにしておきます。PMS はこのディレクトリに `quick-start-policies.json` を作成します。続行する前に REST サーバーが起動するのを待ちます。両方のサービスが同じファイルを使用する必要があります。

### 2. spctl を使用してポリシーを作成

同じディレクトリで2つ目のターミナルを開きます。

サービスコンテナを作成します:

<!-- quick-start: service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
```

`user1` に `res1` に対する get または delete の権限を付与し、`role2` に `res2` に対する同じアクションの権限を付与します:

<!-- quick-start: policies -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role role2 get,del res2" --service-name=mysvc
```

`res2` 上で `user2` に `role2` を割り当てます:

<!-- quick-start: roles -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user user2 role2 on res2" --service-name=mysvc
```

サービスとそのポリシーを一覧表示します:

<!-- quick-start: list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

出力には `mysvc`、2つの認可ポリシー、および1つのロールポリシーが含まれます。

### 3. 認可決定サービスを起動

同じディレクトリで3つ目のターミナルを開きます:

<!-- quick-start: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

起動したままにし、REST サーバーが開始されるのを待ちます。

### 4. 認可決定を検証

2つ目のターミナルでこれらのリクエストを実行します。最初のリクエストは `user1` への直接認可を確認します:

<!-- quick-start: allow-direct -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

レスポンスには `"allowed":true` が含まれます。

次のリクエストは `user2` へのロール認可を確認します:

<!-- quick-start: allow-role -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user2"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

レスポンスには `"allowed":true` が含まれます。

一致する認可がないユーザーは拒否されます:

<!-- quick-start: deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user3"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

レスポンスには `"allowed":false` が含まれます。

`reason` フィールドは各決定の理由を説明します。`0` (`GRANT_POLICY_FOUND`) は認可ポリシーに一致したことを意味します。`3` (`NO_APPLICABLE_POLICIES`) は適用されるポリシーがなかったことを意味し、Speedle はデフォルトで拒否します。[認可決定](../docs/decisions/) ページにすべての理由コードが記載されています。

### 5. 条件付き（ABAC）ポリシーを追加

リクエストに3以上の `clearance` 属性が含まれている場合にのみ適用される認可を作成します:

<!-- quick-start: abac-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get res3 if clearance >= 3" --service-name=mysvc
```

ポリシー施行ポイント（PEP）がリクエスト属性を提供し、条件はリクエスト内の属性に対してのみ評価されます。以下の `clearance` の値はリクエスト形式を示すためのものです。実際の PEP は、認可対象のリクエストからではなく、認証済みトークンクレームや信頼できるストアなどの信頼できる情報源から属性値を取得する必要があります。`clearance` を省略したリクエストはこの認可に一致しません。

`clearance` を 4 に設定すると、リクエストは許可されます:

<!-- quick-start: abac-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":4}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

レスポンスには `"allowed":true` と `"reason":0` が含まれます。

`clearance` を 2 に設定すると条件が一致しないため、適用されるポリシーがなくリクエストは拒否されます:

<!-- quick-start: abac-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":2}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

レスポンスには `"allowed":false` と `"reason":3` が含まれます。

完了したら、PMS および ADS のターミナルで **Ctrl+C** を押します。ポリシーファイルはこのディレクトリに残ります。作成コマンドを再度実行する場合は、新しいファイルを使用してください。

TLS 設定については、[通信セキュリティ](../docs/security/#protect-both-api-transports) を参照してください。その他のデプロイタイプについては、[デプロイ](../docs/deployment/) を参照してください。
