+++
title = "Speedle服务的部署方式"
description = "Recommended deployment topology for Speedle"
weight = 14
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## 选择部署方式

通过[嵌入模式](../deployment-embeded/)可以在 Go 进程内运行决策引擎；如果应用通过 REST 或 gRPC 调用 Speedle，则运行独立的 PMS 和 ADS。[快速入门](../../quick-start/)提供了完整的本地策略管理和授权验证步骤。

## 使用当前 Go 版本构建

在 Linux 或 macOS 上准备 Go 1.26.10 或 Go 1.27.3（也可使用同一大版本的更新补丁版本）（推荐 Go 1.27.3）、Git 和命令行 shell。使用 Go modules，源码可放在任意可写目录，无需手工配置 `GOROOT`、`GOPATH`、`dep` 或 GOPATH 源码路径。

<!-- deployment-check: source-build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

生成的可执行文件为 `bin/spctl`、`bin/speedle-pms` 和 `bin/speedle-ads`。从源码根目录运行下面的本地示例，两个服务共享同一策略文件：

<!-- deployment-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

保持 PMS 运行，在同一源码目录的另一个终端启动 ADS：

<!-- deployment-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

使用新的策略文件；文件不存在时 PMS 会创建它。创建策略和验证 Allow/Deny 的完整请求见[快速入门](../../quick-start/)，结束后用 Ctrl+C 停止两个服务。默认关闭 gRPC，只有设置 `--grpc-endpoint` 才启用。此处明文 HTTP 仅用于本机示例，TLS 和客户端认证见[安全配置](../security/)。

## 构建容器映像

如果需要部署到 Linux/amd64 容器，在源码根目录先构建静态 Linux 二进制：

<!-- deployment-check: linux-build -->
```bash
mkdir -p ./bin/linux-amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/linux-amd64/ ./cmd/speedle-pms ./cmd/speedle-ads
```

将下面内容保存为 `bin/linux-amd64/Dockerfile.pms`，再将其中两处 `speedle-pms` 替换成 `speedle-ads`，另存为同目录下的 `Dockerfile.ads`。映像直接运行二进制，不依赖历史映像的入口脚本：

<!-- deployment-template: image -->
```dockerfile
FROM scratch
COPY speedle-pms /speedle-pms
USER 65532:65532
ENTRYPOINT ["/speedle-pms"]
```

准备支持 BuildKit 的当前 Docker Engine 和可推送的映像仓库。将 `PMS_IMAGE`、`ADS_IMAGE` 设置为自己的完整映像名称与标签，登录该仓库，然后构建并推送：

<!-- deployment-template: image-build -->
```bash
docker build --platform linux/amd64 -f ./bin/linux-amd64/Dockerfile.pms -t "$PMS_IMAGE" ./bin/linux-amd64
docker build --platform linux/amd64 -f ./bin/linux-amd64/Dockerfile.ads -t "$ADS_IMAGE" ./bin/linux-amd64
docker push "$PMS_IMAGE"
docker push "$ADS_IMAGE"
```

这些映像面向 Linux/amd64 节点；如果节点架构不同，应构建对应架构。使用此最小映像连接 TLS 端点时，需要明确提供 CA 文件。

## Kubernetes 开发示例

使用当前 Kubernetes 集群和兼容的 `kubectl`。将以下内容保存为 `speedle-dev.yaml`。它使用 `apps/v1`，在同一 Pod 内让 PMS/ADS 共享文件，以非 root 用户运行应用，并通过内部 ClusterIP 服务访问。这是隔离开发环境示例：Pod 被替换后 `emptyDir` 中的策略会丢失，HTTP 端点需要集群访问控制。私有映像仓库需要时应配置 imagePullSecrets。

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
          image: registry.example.com/speedle-pms:demo
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
          image: registry.example.com/speedle-ads:demo
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

保持 `PMS_IMAGE`、`ADS_IMAGE` 指向刚才推送的映像。先在本地替换清单中的映像，再应用；使用快速入门客户端时保持端口转发运行：

<!-- deployment-template: apply -->
```bash
kubectl set image --local -f speedle-dev.yaml pms="$PMS_IMAGE" ads="$ADS_IMAGE" -o yaml | kubectl apply -f -
kubectl rollout status deployment/speedle-dev
kubectl port-forward service/speedle-dev 6733:6733 6734:6734
```

## 生产部署

生产环境应使用持久化策略存储，保护管理与决策端点，并为客户端和策略后端配置 TLS/mTLS。当前存储配置见[可插拔存储](../store/)，传输参数见[安全配置](../security/)。请按当前 etcd 文档部署和运维 etcd，不要继续使用已停止维护的 etcd-operator 示例。

仓库中保留了历史 Helm/Kubernetes 示例。旧的 Helm 2 `helm install -n ...`、`stable/etcd-operator`、`apps/v1beta2` 和 `r.authz.fun` 映像说明均不是本页的新部署流程。容器构建推送、集群准入、仓库访问、存储与 rollout 依赖具体环境，应在目标集群中验证后再用于生产。
