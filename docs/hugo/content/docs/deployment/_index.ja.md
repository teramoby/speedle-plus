+++
title = "デプロイ"
description = "Go アプリケーションへの組み込み、公開されたコンテナイメージからの実行、またはスタンドアロンの PMS および ADS サービスとして Speedle を実行"
weight = 304
draft = false
toc = true
linksection = true
tocheading = "h2"
categories = ["docs"]
bref = "組み込みモード、Docker イメージ、ソースからのビルド、およびスタンドアロンの PMS と ADS サービス。"
+++

Speedle は Go アプリケーション内への組み込み、1 つの公開コンテナイメージからの実行、またはスタンドアロンの PMS および ADS サービスとして実行できます。

- [組み込みモード](/ja/docs/deployment/embedded/) は Go プロセス内で評価エンジンを実行します。
- [Docker イメージ](/ja/docs/deployment/docker/) は `docker.io/speedle/speedle` または `ghcr.io/teramoby/speedle` から `speedle-pms`、`speedle-ads`、`spctl` を実行します。

## デプロイモードの選択

Go プロセス内で評価エンジンを実行するには [組み込みモード](/ja/docs/deployment/embedded/) を使用します。アプリケーションが REST または gRPC 経由で Speedle を呼び出す場合は、[Docker イメージ](/ja/docs/deployment/docker/) またはスタンドアロンの PMS と ADS を使用します。[クイックスタート](../../quick-start/) では、ローカルでのポリシー管理と認可の完全なウォークスルーを提供しています。

## 最新の Go でビルド

Linux または macOS 上で、Go 1.26.10 または Go 1.27.3（または同じリリースブランチの新しいパッチバージョン）（Go 1.27.3 推奨）、Git、およびシェルを使用します。Go modules を使用し、チェックアウト先は書き込み可能な任意のディレクトリで構いません。カスタムの `GOROOT`、`GOPATH`、`dep`、または GOPATH ソースレイアウトは不要です。

<!-- deployment-check: source-build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

実行ファイルは `bin/spctl`、`bin/speedle-pms`、`bin/speedle-ads` です。チェックアウトルートから、ローカルサンプルのために 1 つの共有ファイルを使用します:

<!-- deployment-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

PMS を実行したままにし、同じチェックアウトの別のターミナルで ADS を起動します:

<!-- deployment-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

新しいポリシーファイルを使用します。ファイルが存在しない場合は PMS が作成します。ポリシーの作成と Allow/Deny リクエストの手順については [クイックスタート](../../quick-start/) に従い、完了したら両方のサービスを Ctrl+C で停止します。`--grpc-endpoint` で明示的に有効にしない限り、gRPC は無効になっています。平文の HTTP はこのループバックサンプル専用です。TLS とクライアント認証については [セキュリティ](../security/) を参照してください。

## リリースバイナリとコンテナイメージ

リリースワークフローによって公開される各リリースには、macOS および Linux（amd64、arm64）向けの事前ビルド済み `spctl`、`speedle-pms`、`speedle-ads` バイナリ（ファイル名 `<コマンド>_<OS>_<アーキテクチャ>`）と `checksums.txt` ファイルが添付されます。[Releases ページ](https://github.com/teramoby/speedle-plus/releases) からダウンロードし、実行前にダウンロードした各ファイルの SHA-256 チェックサムを検証してください。チェックサムファイルは同じリリースに添付されているため、リリースの真正性確認ではなく、ダウンロードの不完全性や破損の検出を目的としています。バイナリとコンテナイメージはリリースワークフローによって生成されるため、ワークフローが追加された後の最初のリリースから利用可能です。

リリースワークフローは、Linux amd64 および arm64 向けの 2 つのマルチアーキテクチャコンテナイメージも公開します:

- `ghcr.io/teramoby/speedle-pms`
- `ghcr.io/teramoby/speedle-ads`

匿名プルにはパッケージがパブリックである必要があります。パッケージがプライベートな場合は、まず `docker login ghcr.io` で認証してください。

デプロイするリリースをプルします。`latest` は最新のリリースに追従します:

<!-- deployment-template: image-pull -->
```bash
docker pull ghcr.io/teramoby/speedle-pms:latest
docker pull ghcr.io/teramoby/speedle-ads:latest
```

ダイジェストでイメージを固定するには、`docker buildx imagetools inspect ghcr.io/teramoby/speedle-pms:<tag>` を実行して公開されたダイジェストを確認します。

両方のイメージは UID 65532 としてサービスを実行し、ファイルストア用の書き込み可能な `/data` ディレクトリを含んでいます。イメージ名の後にサーバーフラグを渡し、各コマンドを独自のターミナルで実行します:

<!-- deployment-template: image-run -->
```bash
docker run --rm -p 127.0.0.1:6733:6733 -v speedle-policies:/data ghcr.io/teramoby/speedle-pms:latest --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
docker run --rm -p 127.0.0.1:6734:6734 -v speedle-policies:/data ghcr.io/teramoby/speedle-ads:latest --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

PMS には組み込みの呼び出し元認証がありません。`127.0.0.1` へのバインドにより、未認証の管理 API をこのマシン内のみに留めます。これらのポートをホストインターフェースに公開したり、信頼できないネットワークに公開したりしないでください。

イメージには CA 証明書が含まれていません。TLS エンドポイント用の CA ファイルを提供してください。`--insecure=true` はこのローカルサンプル専用です。[セキュリティ](../security/) を参照してください。

## Kubernetes 開発サンプル

最新の Kubernetes クラスターと互換性のある `kubectl` を使用してください。以下を `speedle-dev.yaml` として保存します。`apps/v1` を使用し、1 つのポッド内で PMS/ADS が 1 つのファイルを共有し、非 root のアプリケーション UID と内部 ClusterIP サービスを使用します。これは分離された開発用サンプルです。ポッドが置換されると `emptyDir` 内のポリシーは失われ、HTTP エンドポイントにはクラスターアクセス制御が必要です。マニフェストは `<release-tag>` プレースホルダーを使用しています。デプロイするリリースタグまたは不変の固定用のダイジェストに置き換え、クラスターに必要な場合は imagePullSecrets を追加してください。

<!-- deployment-template: kubernetes -->
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: speedle-dev
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: speedle-dev
  template:
    metadata:
      labels:
        app: speedle-dev
    spec:
      initContainers:
        - name: policy-file
          image: busybox:1.37.0
          command: ["sh", "-c", "printf '{}\n' > /data/policies.json; chown 65532:65532 /data/policies.json; chmod 600 /data/policies.json"]
          volumeMounts:
            - name: policies
              mountPath: /data
      containers:
        - name: pms
          image: ghcr.io/teramoby/speedle-pms:<release-tag>
          args: ["--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6733", "--insecure=true"]
          ports:
            - containerPort: 6733
          readinessProbe:
            tcpSocket:
              port: 6733
          volumeMounts:
            - name: policies
              mountPath: /data
        - name: ads
          image: ghcr.io/teramoby/speedle-ads:<release-tag>
          args: ["--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6734", "--insecure=true"]
          ports:
            - containerPort: 6734
          readinessProbe:
            tcpSocket:
              port: 6734
          volumeMounts:
            - name: policies
              mountPath: /data
      volumes:
        - name: policies
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: speedle-dev
spec:
  type: ClusterIP
  selector:
    app: speedle-dev
  ports:
    - name: pms
      port: 6733
      targetPort: 6733
    - name: ads
      port: 6734
      targetPort: 6734
```

`PMS_IMAGE` と `ADS_IMAGE` を、デプロイする正確なイメージ（リリースタグやダイジェストなど）に設定します。適用前にローカルで置換し、クイックスタートのクライアントを使用している間はポートフォワードを実行したままにしてください:

<!-- deployment-template: apply -->
```bash
kubectl set image --local -f speedle-dev.yaml pms="$PMS_IMAGE" ads="$ADS_IMAGE" -o yaml | kubectl apply -f -
kubectl rollout status deployment/speedle-dev
kubectl port-forward service/speedle-dev 6733:6733 6734:6734
```

## 本番環境でのデプロイ

永続的なポリシーストレージを使用し、デプロイするリリースバージョンのコンテナイメージダイジェストを固定し（リリースの再試行でバージョンダグが再公開される可能性があるため）、管理および決定エンドポイントを保護し、クライアントとポリシーバックエンド向けに TLS/mTLS を設定します。[プラグイン可能なストレージ](../store/) では現在のストア設定を説明しており、[セキュリティ](../security/) では現在のトランスポートフラグを網羅しています。廃止された etcd-operator の例ではなく、最新の etcd ドキュメントに従って etcd デプロイを設計・運用してください。

リポジトリには過去の Helm/Kubernetes の例が含まれています。以前の Helm 2 `helm install -n ...`、`stable/etcd-operator`、`apps/v1beta2`、および `r.authz.fun` イメージの説明は、上記のデプロイ手順ではありません。コンテナのプル、クラスターの受け入れ、レジストリアクセス、ストレージ、およびロールアウトはご利用の環境によって異なるため、本番運用の前に対象クラスターで検証してください。
