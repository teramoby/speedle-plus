+++
title = "Docker 镜像"
description = "用已发布的 Speedle 镜像运行 speedle-pms、speedle-ads 和 spctl。"
weight = 70
draft = false
toc = true
tocheading = "h2"
tags = ["sdk"]
categories = ["docs"]
bref = "Docker Hub 和 GHCR 上的同一个镜像运行策略管理服务、决策服务和 spctl，并共用一份文件存储。"
+++

Speedle 发布一个镜像，其中包含 `speedle-pms`、`speedle-ads` 和 `spctl`。镜像基于 `scratch` 构建，以 UID 65532 运行，里面没有 shell。第一个参数决定启动哪个程序。

两个仓库上的镜像索引相同：

- Docker Hub：`docker.io/speedle/speedle`
- GHCR：`ghcr.io/teramoby/speedle`

1.7.0 在两个仓库都发布了标签 `1.7.0`、`1.7` 和 `latest`，架构为 `linux/amd64` 和 `linux/arm64`。

## 拉取镜像

```bash
docker pull docker.io/speedle/speedle:1.7.0
docker pull ghcr.io/teramoby/speedle:1.7.0
```

`docker pull` 会打印索引摘要。1.7.0 在两个仓库上的摘要都是 `sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`。

## 选择标签

`1.7.0` 指向这一次发布。更高的 1.7 补丁发布后，`1.7` 会跟着移动。从 `master` 最新提交发布更高版本时，`latest` 会跟着移动。同一次版本可以再次发布，因此标签不是不可变的构建。

查看索引摘要：

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0 --format '{{.Manifest.Digest}}'
```

1.7.0 这次发布里，两个仓库上的 `1.7.0`、`1.7` 和 `latest` 都打印：

```text
sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0
```

部署 `1.7` 或 `latest` 之前请再查一次摘要。要固定在同一次构建上，把镜像名写成带 `@` 和该摘要的形式，例如 `docker.io/speedle/speedle@sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`。

## 镜像里的命令

不带参数时，镜像打印这三个程序：

```bash
docker run --rm docker.io/speedle/speedle:1.7.0
```

```text
speedle-pms
speedle-ads
spctl
```

`docker run --rm ghcr.io/teramoby/speedle:1.7.0` 打印同样的三行。下面的命令使用 Docker Hub 的名字。把名字换成 GHCR 即使用那一份副本。镜像里没有 shell，所以 `docker exec` 必须用绝对路径调用 `/spctl`、`/speedle-pms` 或 `/speedle-ads`。

## 运行 PMS 和 ADS

创建命名卷并启动策略管理服务。进程在容器内监听 `0.0.0.0:6733`。把端口发布到 `127.0.0.1:6733`，未认证的管理 API 就只留在本机。

```bash
docker volume create speedle-policies
docker run -d --name speedle-pms \
  -p 127.0.0.1:6733:6733 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
```

新建的命名卷会放入镜像里的 `/data` 目录，UID 65532 可以写入。策略文件不存在时，PMS 会创建 `/data/policies.json`。镜像不包含 CA 证书。`--insecure=true` 只用于这个本地 HTTP 示例。TLS 见[安全](/docs/security/)。

Docker 可能在进程开始监听之前就接受已发布的端口，然后重置连接。请重试所有错误，包括这种重置：

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
```

第一次尝试可能会打印 `curl: (56) Recv failure: Connection reset by peer`。随后的尝试返回正文。空存储的响应是：

```text
[]
```

在同一个卷上启动 ADS。它读取 PMS 写入的文件，也包括之后的更新。

```bash
docker run -d --name speedle-ads \
  -p 127.0.0.1:6734:6734 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

## 检查健康和就绪

PMS 不提供 `/healthz`。上面的 `GET /policy-mgmt/v1/service` 返回成功，就表示管理 API 已经在接受请求。

ADS 提供存活和就绪检查：

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

进程运行期间，`/healthz` 返回 `200 {"status":"ok"}`。ADS 当前能够做授权判断时，`/readyz` 返回 `200`。文件存储没有策略分发状态，所以服务计数保持为零；要看的字段是 `ready`。镜像里没有 shell，也没有 `curl`，因此这些探测在宿主机或编排器上对已发布的端口执行。`http://localhost:6734/healthz` 访问的是同一个已发布端口。

## 用 spctl 创建策略

在正在运行的 PMS 容器里，`/spctl` 通过该容器自己的端口访问 PMS：

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

PMS 会分配 `createtime` 和策略 `id`，所以每次运行这两个值都不同。这里的服务和策略与 [SDK 概览](/docs/sdks/#示例策略) 中的示例相同。

同一个 `spctl` 也可以在另一个容器里运行，只要它与 PMS 共用网络命名空间：

```bash
docker run --rm --network container:speedle-pms \
  docker.io/speedle/speedle:1.7.0 \
  spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

## 向 ADS 请求决策

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":true,"reason":0}
```

`reason` 为 `0` 表示 `GRANT_POLICY_FOUND`。同一条策略没有授予 `delete`：

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"delete"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":false,"reason":3}
```

`reason` 为 `3` 表示 `NO_APPLICABLE_POLICIES`。ADS 已经在运行时，通过 PMS 写入的新策略会在下一次请求中生效。请求字段见[授权决策](/docs/decisions/)。

## 让 SDK 连接 ADS

SDK 页面使用 `http://localhost:6734`。按上面的方式发布端口后，这个地址就是本页的 ADS。下面的 Node.js 客户端使用容器实际发布的 `http://127.0.0.1:6734`。把它存成 `main.mjs`：

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

在一个新的 ES 模块包里安装已发布的包并运行。安装命令与 [Node.js SDK](/docs/sdks/node/) 页面相同，并固定同一版本：

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
node main.mjs
```

```text
alice get /docs/public: allowed=true
```

其他语言页面使用同一个端点和服务名。SDK 只对回环地址接受明文 `http://`。

## 用 Compose 同时运行两个服务

把下面的内容存成 `compose.yaml`。这是运行同样两个进程和同一个卷的另一种方式。如果前面的容器还在运行，先停掉它们，让 6733 和 6734 空出来。删掉那个卷也会删掉里面的策略文件。

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
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

用上面的 curl 命令等待 PMS 的服务列表以及 ADS 的 `/healthz` 和 `/readyz`，再发送同一个 `is-allowed` 请求。ADS 不需要重启就能看到新策略。

```bash
docker compose -f compose.yaml down -v
```

`down -v` 会删除 Compose 卷以及保存在里面的策略。

## 核对来源证明

发布构建附带了来源证明和 SBOM（`docker buildx build --provenance=mode=max --sbom=true`）。查看索引：

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0
```

索引列出 `linux/amd64`、`linux/arm64`，以及两份 `unknown/unknown` 清单。这两份未知平台是证明清单。每一份都把 `vnd.docker.reference.type` 设为 `attestation-manifest`，并把 `vnd.docker.reference.digest` 设为它所描述的平台镜像。`docker buildx imagetools inspect ghcr.io/teramoby/speedle:1.7.0` 打印相同的平台摘要和证明摘要。

## 停止容器

```bash
docker rm -f speedle-pms speedle-ads
docker volume rm speedle-policies
```

`docker volume rm` 会删除策略文件。如果进程是用 Compose 启动的，改用 `docker compose -f compose.yaml down -v`。
