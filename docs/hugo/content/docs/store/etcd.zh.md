+++
title = "etcd 配置指南"
description = "逐步部署 etcd 策略存储，配置 PMS/ADS，并验证同步和持久化。"
weight = 20
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "etcd"]
categories = ["docs"]
bref = "启动独立 etcd，连接 PMS 与 ADS，验证策略自动更新，并创建备份。"
+++

本指南使用**独立的 etcd v3.6.14** 和 Docker 持久化卷。PMS 写入策略，ADS 监听同一个 key prefix。本地单节点示例通过回环地址提供 HTTP 服务；生产环境的 TLS 与集群配置见后文。

## 1. 编译 Speedle

准备 Bash、Git、curl、已启动的 Docker，以及 Go 1.26.10 或 Go 1.27.3，然后从源码编译：

<!-- etcd-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

后续命令均在这个代码目录执行。确认 2379、6733、6734 端口可用；修改端口时要同步修改命令和配置。

## 2. 启动独立 etcd 服务

命名卷保存 etcd 数据。这里只把客户端端口发布到宿主机，单节点内部通信使用的 peer 端口保留在容器内。

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

等待 v3 API 健康检查通过：

<!-- etcd-step: ready -->
```bash
until docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 endpoint health
do
  sleep 1
done
```

如果已有 etcd，可跳过容器启动命令，在下一步填入已有的客户端 endpoint。Docker 网络部署细节见[官方容器指南](https://etcd.io/docs/v3.6/op-guide/container/)。

## 3. 配置共享策略存储

为本地 PMS/ADS 创建一个共用配置文件：

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

PMS 与 ADS 必须使用**同一个 endpoint 和 key prefix**。保留 `/speedle_ps/` 末尾的斜杠；不同的独立策略存储应使用不同前缀。`IsEmbeddedEtcd: false` 表示连接已启动的独立 etcd。`enableWatch: true` 让 ADS 通过 etcd watch 事件更新，不需要配置轮询间隔。

## 4. 启动 PMS

在终端 A 中启动 PMS，并保持运行：

<!-- etcd-step: pms -->
```bash
./bin/speedle-pms --config-file etcd-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

等待 REST 监听启动。本地演示的 HTTP 服务只绑定回环地址；Speedle 自身的 TLS 和 API 授权配置见[安全配置](/docs/security/)。

## 5. 创建策略

在终端 B 中进入同一代码目录，授予 Alice 读取 `book` 的权限：

<!-- etcd-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"etcd-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS 返回服务及生成的策略 ID。此时策略已写入 etcd，无需建 SQL 表或手工初始化 key。

## 6. 启动 ADS 并验证授权

在终端 C 中使用同一个配置文件启动 ADS，并保持运行：

<!-- etcd-step: ads -->
```bash
./bin/speedle-ads --config-file etcd-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

等待启动完成，然后在终端 B 中检查授权。Alice 应被允许：

<!-- etcd-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

预期返回 `{"allowed":true,"reason":0}`。Bob 没有匹配的授权策略：

<!-- etcd-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

预期返回 `{"allowed":false,"reason":3}`。

## 7. 检查存储的 key 和策略自动更新

只查看本策略存储前缀下的 key：

<!-- etcd-step: keys -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 get /speedle_ps/ --prefix --keys-only
```

输出中应包含 `/speedle_ps/services/` 下 `etcd-demo` 的相关 key。这些 key 由存储后端管理，策略变更请通过 PMS 完成。

保持 ADS 运行，为 Alice 新增一条拒绝策略：

<!-- etcd-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/etcd-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

再次执行第 6 步 Alice 的请求。watch 异步更新后，应返回 `{"allowed":false,"reason":1}`。如果同步尚未完成，稍后重试；这种正常策略更新无需重启 ADS。

## 8. 创建快照并验证持久化

创建在线快照、查看快照信息，并复制到容器外：

<!-- etcd-step: snapshot -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 snapshot save /etcd-data/speedle-snapshot.db
docker exec speedle-etcd /usr/local/bin/etcdutl \
  snapshot status /etcd-data/speedle-snapshot.db --write-out=table
docker cp speedle-etcd:/etcd-data/speedle-snapshot.db ./etcd-demo/speedle-snapshot.db
```

快照包含**整个 etcd 集群的 keyspace**，也包含其他前缀的数据。备份应保存在独立且有访问控制的存储上。恢复数据与成员信息的步骤见[官方灾难恢复指南](https://etcd.io/docs/v3.6/op-guide/recovery/)。

先按 Ctrl+C 停止 PMS 和 ADS，再执行容器重启检查：

<!-- etcd-step: restart -->
```bash
docker restart speedle-etcd
```

执行第 2 步健康检查，再用原配置重新启动 PMS 与 ADS。重复 Alice 的请求，仍应返回 `{"allowed":false,"reason":1}`，说明命名卷在容器重启后保留了策略。

灾难恢复时，先停止策略写入，再按 etcd 官方流程把快照恢复到新的数据目录。启动 ADS，让它读取恢复后的策略，再对外提供授权服务。恢复后应重新验证授权结果；原来运行中的缓存不能证明已包含恢复后的策略集。

## 生产环境：TLS、身份认证与集群

先为 etcd 配置受信任的服务端证书、与 endpoint 主机名匹配的证书 SAN，以及客户端证书校验。再为 PMS/ADS 配置 HTTPS 和各自的客户端凭据：

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

这些路径位于 PMS/ADS 主机上；私钥应仅允许服务账号读取。连接 etcd 的 TLS 与 Speedle REST/gRPC 监听端的 TLS 分别配置。

当前适配器提供证书选项，没有 etcd 用户名/密码配置属性。使用 etcd RBAC 时，可开启 `--client-cert-auth=true` 和身份认证，将客户端证书的 Common Name 映射为 etcd 用户。PMS 对其前缀需要读写权限；ADS 的权限应覆盖启用的功能。服务端和角色配置详见官方 [TLS 指南](https://etcd.io/docs/v3.6/op-guide/security/)和 [RBAC 指南](https://etcd.io/docs/v3.6/op-guide/authentication/rbac/)。key prefix 用于组织数据，访问控制需要 RBAC。

高可用部署请参考官方[三成员集群示例](https://etcd.io/docs/v3.6/op-guide/container/#running-a-3-node-etcd-cluster)，同时保护客户端与 peer 通信。单节点演示无法承受唯一成员故障。当前 `EtcdEndpoint` 属性接收一个 endpoint 字符串，请为部署提供可达的客户端 URL。

## 配置速查与故障排查

| 配置属性 | 命令行参数 | 环境变量 | 默认值 |
| --- | --- | --- | --- |
| `EtcdEndpoint` | `--etcdstore-endpoint` | `SPDL_ETCDSTORE_ENDPOINT` | `localhost:2379` |
| `EtcdKeyPrefix` | `--etcdstore-keyprefix` | `SPDL_ETCDSTORE_KEYPREFIX` | `/speedle_ps/` |
| `IsEmbeddedEtcd` | `--etcdstore-isembedded` | `SPDL_ETCDSTORE_ISEMBEDDED` | `false` |
| `EmbeddedEtcdDataDir` | `--etcdstore-embeddedDataDir` | `SPDL_ETCDSTORE_EMBEDDEDDATADIR` | 空，仅用于内嵌模式 |
| `EtcdTLSTrustedCAFile` | `--etcdstore-tls-ca` | `SPDL_ETCDSTORE_TLS_CA` | 空 |
| `EtcdTLSCertFile` | `--etcdstore-tls-cert` | `SPDL_ETCDSTORE_TLS_CERT` | 空 |
| `EtcdTLSKeyFile` | `--etcdstore-tls-key` | `SPDL_ETCDSTORE_TLS_KEY` | 空 |
| `EtcdTLSServerName` | `--etcdstore-tls-serverName` | `SPDL_ETCDSTORE_TLS_SERVERNAME` | 空 |
| `EtcdTLSInsecureSkipVerify` | `--etcdstore-tls-insecureSkipVerify` | `SPDL_ETCDSTORE_TLS_INSECURESKIPVERIFY` | `false` |

配置优先级为：显式参数 → 非空环境变量 → 配置文件 → 默认值。使用 `--store-type etcd` 或 `SPDL_STORE_TYPE=etcd` 选择后端。其余 TLS 选项包括 `EtcdTLSAllowedCN`（`--etcdstore-tls-allowedCN`）和 `EtcdTLSCRLFile`（`--etcdstore-tls-crlFile`）。

内嵌模式（`IsEmbeddedEtcd: true`）用于本地开发和测试，并使用固定的本地 etcd 端口。本指南让 etcd 独立于 PMS/ADS 运行，明确管理其数据目录和生命周期。

| 现象 | 检查项 |
| --- | --- |
| 连接被拒绝或服务启动长时间等待 | etcd 健康状态、客户端端口、宿主机与容器地址以及防火墙。 |
| PMS 已写入，但 ADS 找不到服务 | endpoint 与 key prefix 是否一致，包括末尾斜杠。 |
| 策略变更未同步到 ADS | watch 是否开启、连接状态及 ADS 日志中的 watch 错误；恢复后可重启 ADS 重新加载。 |
| TLS 主机名或 CA 校验失败 | HTTPS endpoint、证书 SAN、可信 CA 路径及 server name。 |
| 权限不足 | 客户端证书身份、已启用的 RBAC 角色和前缀权限。 |

后端接口和配置合并规则详见[存储扩展](/docs/store/)。
