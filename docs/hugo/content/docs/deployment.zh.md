+++
title = "Speedle服务的部署方式"
description = "使用 Docker 或 Kubernetes 部署 Speedle，配置文件存储、etcd 与 TLS/mTLS"
weight = 14
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## 选择部署方式

可以先在 [Playground](/playground/) 试用基本策略；本地进程部署见[快速入门](/quick-start/)，在 Go 中嵌入引擎见[嵌入式部署](/docs/deployment-embeded/)。

本页提供本地 Docker 示例、单 Pod 的 Kubernetes 文件存储示例，以及使用 etcd 的独立 PMS/ADS Pod 部署。选择一种部署方式完成后，再执行后面的授权检查。所有命令均在仓库根目录的 Bash 中执行。

### 前提条件

- 可用的 Docker 引擎与 BuildKit。镜像内部使用 Go 1.26.10 编译；证书示例需要本机 Go 1.26 或更高版本。
- Kubernetes 部署需要可访问的测试集群、kubectl，以及创建新 namespace 的权限。示例使用 Kubernetes/kubectl 1.36.4、Helm 4.3.1 和 etcd 3.5.35。etcd 示例的 1 GiB PVC 需要默认 StorageClass。
- curl，以及可用的本机 6733/6734 端口；也可通过下方的 `PMS_PORT`、`ADS_PORT` 覆盖。

自动化部署检查使用隔离的 [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) 0.33.0 集群，无需付费云集群。

## 构建和分发镜像

在同一个终端初始化以下变量。`IMAGE_PREFIX` 默认为 `speedle`；如果需要其他节点拉取镜像，请先将它设置为自己的镜像仓库前缀。`IMAGE_TAG` 用来选择固定版本或本地开发标签。`DEMO`、`NAMESPACE` 标识本页创建的资源，请使用新的 namespace。

<!-- deployment-check: setup -->
```bash
export IMAGE_PREFIX="${IMAGE_PREFIX:-speedle}"
export IMAGE_TAG="${IMAGE_TAG:-dev}"
export PMS_IMAGE="${IMAGE_PREFIX}-pms:${IMAGE_TAG}"
export ADS_IMAGE="${IMAGE_PREFIX}-ads:${IMAGE_TAG}"
export PMS_PORT="${PMS_PORT:-6733}"
export ADS_PORT="${ADS_PORT:-6734}"
export DEMO="${DEMO:-speedle-demo-$(date +%s)}"
export NAMESPACE="${NAMESPACE:-$DEMO}"
export DOCS_WORKDIR="${DOCS_WORKDIR:-$(mktemp -d)}"
export TLS_DIR="$DOCS_WORKDIR/tls"
```

从仓库根目录构建两个镜像，无需先将本机编译的二进制复制到 Docker 构建目录。

<!-- deployment-check: build -->
```bash
make image pmsImageRepo="${IMAGE_PREFIX}-pms" pmsImageTag="$IMAGE_TAG" \
  adsImageRepo="${IMAGE_PREFIX}-ads" adsImageTag="$IMAGE_TAG"
```

使用镜像仓库部署时，先登录选定仓库，再推送相同仓库名和标签。本机构建的镜像不会自动出现在 Kubernetes 节点中。

<!-- deployment-check: push -->
```bash
make push pmsImageRepo="${IMAGE_PREFIX}-pms" pmsImageTag="$IMAGE_TAG" \
  adsImageRepo="${IMAGE_PREFIX}-ads" adsImageTag="$IMAGE_TAG"
```

## 本地 Docker 示例

本示例仅将 API 发布到宿主机回环地址，并在示例网络内显式启用明文连接。两个容器共用策略卷，请保持该网络隔离。生产连接应使用[安全文档](/docs/security/)中的 TLS/mTLS 配置。

<!-- deployment-check: docker -->
```bash
docker network create "$DEMO"
docker volume create "$DEMO-policies"
docker run -d --name "$DEMO-pms" --network "$DEMO" \
  --cpus=0.5 --memory=256m -p "127.0.0.1:$PMS_PORT:6733" \
  --mount "type=volume,source=$DEMO-policies,target=/var/lib/speedle" \
  "$PMS_IMAGE" --insecure=true
docker run -d --name "$DEMO-ads" --network "$DEMO" \
  --cpus=0.5 --memory=256m -p "127.0.0.1:$ADS_PORT:6734" \
  --mount "type=volume,source=$DEMO-policies,target=/var/lib/speedle" \
  "$ADS_IMAGE" --insecure=true
```

等待两个容器均显示 `healthy` 后，再执行后面的授权检查。

<!-- deployment-check: docker-status -->
```bash
docker inspect --format '{{.Name}}: {{.State.Health.Status}}' "$DEMO-pms" "$DEMO-ads"
```

## Kubernetes 文件存储示例

创建新 namespace；如果名称已存在，请更换名称。先确保镜像已推送到节点可访问的仓库，或已加载到本地 kind 集群。

<!-- deployment-check: namespace -->
```bash
kubectl create namespace "$NAMESPACE"
```

使用 Helm 安装：

<!-- deployment-check: helm-dev -->
```bash
helm upgrade --install speedle-dev deployment/helm/speedle-dev \
  --namespace "$NAMESPACE" --set-string image.pms="$PMS_IMAGE" \
  --set-string image.ads="$ADS_IMAGE" --wait --timeout 180s
```

也可以直接使用独立 YAML，安装相同资源后显式设置镜像，并等待新版本就绪。此方式请使用新 namespace，不要与 Helm 同时管理同一组资源。

<!-- deployment-check: raw-dev -->
```bash
kubectl --namespace "$NAMESPACE" apply --dry-run=server -f deployment/k8s/speedle-dev.yaml
kubectl --namespace "$NAMESPACE" apply -f deployment/k8s/speedle-dev.yaml
kubectl --namespace "$NAMESPACE" set image deployment/speedle-dev pms="$PMS_IMAGE" ads="$ADS_IMAGE"
kubectl --namespace "$NAMESPACE" rollout status deployment/speedle-dev --timeout=180s
```

开发 chart 将 PMS、ADS 放在同一 Pod 中，共用文件卷。默认 `emptyDir` 的策略在 Pod 被替换时丢失；需要保留示例数据时，请预先创建 PVC 并设置 `storage.existingClaim`。该 chart 限定一个副本；多个独立 Pod 应使用 etcd chart。

在当前终端启动端口转发，再执行授权检查；检查期间保持转发进程运行。

<!-- deployment-check: forward-dev -->
```bash
kubectl --namespace "$NAMESPACE" port-forward --address 127.0.0.1 svc/speedle-dev \
  "$PMS_PORT:6733" "$ADS_PORT:6734" > "$DOCS_WORKDIR/forward.log" 2>&1 &
DEV_FORWARD_PID=$!
```

## 使用 etcd 与 mTLS 的独立 PMS/ADS Pod

使用另一个新 namespace（设置 `NAMESPACE` 后执行前面的创建命令）。生产部署应接入满足持久化、可用性、备份和访问控制要求的 etcd 集群。下面提供单成员、使用 PVC 的可复现测试示例；它在测试 namespace 内使用 HTTP 客户端连接。

<!-- deployment-check: etcd -->
```bash
kubectl --namespace "$NAMESPACE" apply --dry-run=server -f deployment/k8s/etcd-example.yaml
kubectl --namespace "$NAMESPACE" apply -f deployment/k8s/etcd-example.yaml
kubectl --namespace "$NAMESPACE" rollout status statefulset/speedle-etcd --timeout=180s
```

生成有效期 24 小时、包含 localhost/回环地址 SAN 的示例证书，再创建服务端 Secret。客户端私钥保留在本机。生产环境应使用可信 CA 为实际服务 DNS 名称签发的证书。重新生成时需选择新的输出目录。

<!-- deployment-check: certificates -->
```bash
go run ./samples/docs/certificates --out "$TLS_DIR"
kubectl --namespace "$NAMESPACE" create secret generic speedle-server-tls \
  --from-file=server.crt="$TLS_DIR/server.crt" \
  --from-file=server.key="$TLS_DIR/server.key" \
  --from-file=ca.crt="$TLS_DIR/ca.crt"
```

安装使用 etcd 的 chart。默认情况下，REST 和 gRPC 均要求 TLS 与可信客户端证书。

<!-- deployment-check: helm-prod -->
```bash
helm upgrade --install speedle-prod deployment/helm/speedle-prod \
  --namespace "$NAMESPACE" --set-string image.pms="$PMS_IMAGE" \
  --set-string image.ads="$ADS_IMAGE" \
  --set-string store.etcd.endpoint=http://speedle-etcd:2379 \
  --wait --timeout 180s
```

独立 YAML 方式使用相同的默认 Secret 和 etcd 端点：

<!-- deployment-check: raw-prod -->
```bash
kubectl --namespace "$NAMESPACE" apply --dry-run=server -f deployment/k8s/speedle-prod.yaml
kubectl --namespace "$NAMESPACE" apply -f deployment/k8s/speedle-prod.yaml
kubectl --namespace "$NAMESPACE" set image deployment/speedle-prod-pms pms="$PMS_IMAGE"
kubectl --namespace "$NAMESPACE" set image deployment/speedle-prod-ads ads="$ADS_IMAGE"
kubectl --namespace "$NAMESPACE" rollout status deployment/speedle-prod-pms --timeout=180s
kubectl --namespace "$NAMESPACE" rollout status deployment/speedle-prod-ads --timeout=180s
```

接入已有的 TLS etcd 集群时，创建包含 etcd 客户端证书、私钥和 CA 的 Secret，再配置以下 values。它与保护 Speedle API 的证书是两套独立配置：

```yaml
store:
  type: etcd
  etcd:
    endpoint: https://your-etcd-service:2379
    etcdClientCertSecret: etcd-client-tls
    certFile: client.crt
    keyFile: client.key
    trustedCAFile: ca.crt
```

将两个服务转发到本机：

<!-- deployment-check: forward-prod -->
```bash
kubectl --namespace "$NAMESPACE" port-forward --address 127.0.0.1 svc/speedle-prod-pms \
  "$PMS_PORT:6733" > "$DOCS_WORKDIR/pms-forward.log" 2>&1 &
PMS_FORWARD_PID=$!
kubectl --namespace "$NAMESPACE" port-forward --address 127.0.0.1 svc/speedle-prod-ads \
  "$ADS_PORT:6734" > "$DOCS_WORKDIR/ads-forward.log" 2>&1 &
ADS_FORWARD_PID=$!
```

## 验证授权

Docker 或文件存储示例使用 HTTP：

<!-- deployment-check: http-client -->
```bash
TLS_FLAGS=()
export PMS_URL="http://127.0.0.1:$PMS_PORT"
export ADS_URL="http://127.0.0.1:$ADS_PORT"
```

mTLS 部署使用 HTTPS，校验服务端证书并提供可信客户端证书：

<!-- deployment-check: https-client -->
```bash
TLS_FLAGS=(--cacert "$TLS_DIR/ca.crt" --cert "$TLS_DIR/client.crt" --key "$TLS_DIR/client.key")
export PMS_URL="https://127.0.0.1:$PMS_PORT"
export ADS_URL="https://127.0.0.1:$ADS_PORT"
```

只执行与当前部署对应的客户端配置。发送请求前，等待端口转发日志显示本机端口已监听。导入书店策略，授予 Alan 下载书籍的权限：

<!-- deployment-check: create -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -H 'Content-Type: application/json' \
  --data-binary @samples/docs/bookstore.json "$PMS_URL/policy-mgmt/v1/service"
```

策略通过 watch 异步传播。ADS 接收到更新后，Alan 返回 `allowed=true, reason=0`：

<!-- deployment-check: allow -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  "$ADS_URL/authz-check/v1/is-allowed"
```

Bob 返回 `allowed=false, reason=3`：

<!-- deployment-check: deny -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  "$ADS_URL/authz-check/v1/is-allowed"
```

读取并删除测试服务：

<!-- deployment-check: read -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" "$PMS_URL/policy-mgmt/v1/service/onlineBookStore"
```

<!-- deployment-check: delete -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -X DELETE "$PMS_URL/policy-mgmt/v1/service/onlineBookStore"
```

## 健康检查与清理

容器探针使用独立的回环 HTTP 监听：PMS 为 `127.0.0.1:8081`，ADS 为 `127.0.0.1:8082`。`/livez` 表示进程仍可响应；`/readyz` 在超时范围内检查后端可用性。后端不可用会导致 readiness 失败，避免立即重启仍存活的进程。业务 API 端口继续保持 TLS/mTLS；Service 不发布这些健康端口。参考 [Kubernetes 探针文档](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)。

使用完 Docker 示例后，停止并移除容器及其策略数据：

<!-- deployment-check: docker-cleanup -->
```bash
docker stop --time 15 "$DEMO-pms" "$DEMO-ads"
docker rm "$DEMO-pms" "$DEMO-ads"
docker volume rm "$DEMO-policies"
docker network rm "$DEMO"
```

Kubernetes 示例结束后，停止所选部署方式的端口转发进程：

<!-- deployment-check: forward-dev-cleanup -->
```bash
kill "$DEV_FORWARD_PID" 2>/dev/null || true
wait "$DEV_FORWARD_PID" || true
```

<!-- deployment-check: forward-prod-cleanup -->
```bash
kill "$PMS_FORWARD_PID" "$ADS_FORWARD_PID" 2>/dev/null || true
wait "$PMS_FORWARD_PID" "$ADS_FORWARD_PID" || true
```

删除本页创建的示例 namespace，同时删除其中 PVC 保存的测试数据。

<!-- deployment-check: namespace-cleanup -->
```bash
kubectl delete namespace "$NAMESPACE" --wait=true
```
