+++
title = "MongoDB 配置指南"
description = "逐步部署 MongoDB 策略存储，配置 PMS/ADS，并验证同步和持久化。"
weight = 30
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mongodb"]
categories = ["docs"]
bref = "启动 MongoDB 副本集，连接 PMS 与 ADS，验证策略自动更新，并创建备份。"
+++

本指南使用 **MongoDB 7.0.16** 单节点副本集和 Docker 持久化卷。PMS 写入策略，ADS 通过 change stream 跟踪同一个数据库。本地示例只监听回环地址且不开启身份认证，用于开发环境；生产环境的身份认证、TLS 与副本集配置见后文。

MongoDB 必须以**副本集或分片集群**方式运行：ADS 使用 change stream 监听数据库，整体替换策略存储时使用事务，独立运行的 `mongod` 两者都不支持。开发环境使用单节点副本集即可。

## 1. 编译 Speedle

准备 Bash、Git、curl、已启动的 Docker，以及 Go 1.26.10 或 Go 1.27.3，然后从源码编译：

<!-- mongodb-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

后续命令均在这个代码目录执行。确认 27017、6733、6734 端口可用；修改端口时要同步修改命令和配置。

## 2. 启动 MongoDB

命名卷保存 MongoDB 数据。`--replSet` 让 `mongod` 作为副本集 `speedle-rs` 的成员启动；只把数据库端口发布到宿主机的回环地址。

<!-- mongodb-step: database -->
```bash
docker run --name speedle-mongodb --detach \
  --publish 127.0.0.1:27017:27017 \
  --volume speedle-mongodb-data:/data/db \
  mongo:7.0.16 \
  --replSet speedle-rs --bind_ip_all
```

等待服务开始接受命令：

<!-- mongodb-step: ready -->
```bash
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.adminCommand({ping: 1}).ok' >/dev/null 2>&1
do
  sleep 1
done
```

如果已有 MongoDB 副本集，可跳过容器命令和第 3 步，在下文使用它的连接字符串。容器用法详见[官方 Docker 镜像说明](https://hub.docker.com/_/mongo)。

## 3. 初始化副本集

以这台服务器作为唯一成员初始化副本集（只需执行一次），并等待它成为主节点：

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

成员地址 `127.0.0.1:27017` 会告知客户端去连接，因此 PMS 和 ADS 必须能访问这个地址。这里端口发布在相同的回环地址上，所以可以访问。副本集配置保存在数据卷中，容器重启后不要再次执行 `rs.initiate`。

## 4. 配置共享存储

为本地这一组 PMS/ADS 创建配置文件：

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

PMS 与 ADS 必须使用**同一个部署和同一个数据库**。每个独立的策略存储使用单独的数据库；服务和函数分别保存在 `services` 和 `functions` 集合中。MongoDB 在首次写入时自动创建数据库和集合，无需初始化 schema。`enableWatch: true` 让 ADS 通过 change stream 获取更新，无需配置轮询间隔。

## 5. 启动 PMS

在终端 A 启动 PMS，并保持运行：

<!-- mongodb-step: pms -->
```bash
./bin/speedle-pms --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

等待 REST 监听启动。本演示仅在回环地址使用 HTTP；Speedle 自身的 TLS 与 API 授权见[安全](/docs/security/)。

## 6. 创建策略

在终端 B 的同一代码目录中，授予 Alice 读取 `book` 的权限：

<!-- mongodb-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mongodb-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS 返回服务及生成的策略 ID，策略此时已写入 MongoDB。

## 7. 启动 ADS 并验证授权

在终端 C 使用同一个配置文件启动 ADS，并保持运行：

<!-- mongodb-step: ads -->
```bash
./bin/speedle-ads --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

等待启动完成，然后在终端 B 执行以下检查。Alice 应被允许：

<!-- mongodb-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

预期结果：`{"allowed":true,"reason":0}`。Bob 没有匹配的授权：

<!-- mongodb-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

预期结果：`{"allowed":false,"reason":3}`。

## 8. 验证存储的文档和自动更新

查看服务文档，服务名保存在 `_id` 中：

<!-- mongodb-step: documents -->
```bash
docker exec speedle-mongodb mongosh speedle_demo --quiet --eval \
  'db.services.find({}, {"policies.name": 1, "policies.effect": 1}).toArray()'
```

输出中应包含 `_id: 'mongodb-demo'` 及其 `alice-read` 授权。这些文档由存储后端管理，策略变更请通过 PMS 完成。

保持 ADS 运行，为 Alice 新增一条拒绝策略：

<!-- mongodb-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mongodb-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

再次执行第 7 步 Alice 的请求。change stream 异步更新后，应返回 `{"allowed":false,"reason":1}`。如果同步尚未完成，稍后重试；这种正常策略更新无需重启 ADS。

## 9. 创建备份并验证持久化

把策略数据库导出为归档文件，并复制到容器外：

<!-- mongodb-step: backup -->
```bash
docker exec speedle-mongodb mongodump --quiet \
  --db speedle_demo --archive=/tmp/speedle-demo.archive
docker cp speedle-mongodb:/tmp/speedle-demo.archive ./mongodb-demo/speedle-demo.archive
```

归档只包含 `speedle_demo` 数据库。备份应保存在独立且有访问控制的存储上。恢复时先停止策略写入，再对主节点执行 `mongorestore --drop --archive=...`，详见[官方备份与恢复指南](https://www.mongodb.com/docs/manual/tutorial/backup-and-restore-tools/)。

先按 Ctrl+C 停止 PMS 和 ADS，再执行容器重启检查：

<!-- mongodb-step: restart -->
```bash
docker restart speedle-mongodb
```

执行第 2 步和第 3 步中的两个等待循环（跳过 `rs.initiate`），再用原配置重新启动 PMS 与 ADS。重复 Alice 的请求，仍应返回 `{"allowed":false,"reason":1}`，说明命名卷在容器重启后保留了数据和副本集配置。

恢复数据后，重启 ADS，让它读取恢复后的策略，再对外提供授权服务。恢复后应重新验证授权结果；原来运行中的缓存不能证明已包含恢复后的策略集。

## 生产环境：身份认证、TLS 与副本集

开启访问控制，为 PMS 和 ADS 分别创建用户，并通过 TLS 连接。PMS 需要策略数据库的 `readWrite` 角色；ADS 只读取和监听，内置的 `read` 角色即可，其用户无法修改策略：

```javascript
use speedle
db.createUser({user: "speedle-pms", pwd: passwordPrompt(), roles: [{role: "readWrite", db: "speedle"}]})
db.createUser({user: "speedle-ads", pwd: passwordPrompt(), roles: [{role: "read", db: "speedle"}]})
```

再为每个服务配置各自的连接字符串：

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

CA 路径位于 PMS/ADS 主机上。URI 中的凭据需要进行百分号编码，包含凭据的配置文件应仅允许服务账号读取；也可以通过 `SPDL_MONGOSTORE_URI` 传入 URI。[MongoDB 连接字符串](https://www.mongodb.com/docs/manual/reference/connection-string/)的所有选项都会原样传给 Go 驱动，包括客户端证书（`tlsCertificateKeyFile`）和其他认证机制。连接 MongoDB 的 TLS 与 Speedle REST/gRPC 监听端的 TLS 分别配置。

副本集开启访问控制后，成员之间也需要通过 key file 或 X.509 证书认证。详见官方[使用 keyfile 认证部署副本集](https://www.mongodb.com/docs/manual/tutorial/deploy-replica-set-with-keyfile-access-control/)和[配置 TLS](https://www.mongodb.com/docs/manual/tutorial/configure-ssl/)。

高可用部署至少使用三个数据节点，并在连接字符串中列出多个成员。单节点演示无法承受唯一成员故障。选举新主节点期间，通过 PMS 的写入可能失败，选举完成后重试即可。

## 配置速查与故障排查

| 配置属性 | 命令行参数 | 环境变量 | 默认值 |
| --- | --- | --- | --- |
| `MongoURI` | `--mongostore_uri` | `SPDL_MONGOSTORE_URI` | `mongodb://localhost:27017` |
| `MongoDatabase` | `--mongostore_database` | `SPDL_MONGOSTORE_DATABASE` | `speedleplus` |

配置优先级为：显式参数 → 非空环境变量 → 配置文件 → 默认值。使用 `--store-type mongodb` 或 `SPDL_STORE_TYPE=mongodb` 选择后端。内嵌使用时，导入 `pkg/store/mongodb`，并把这些属性传给 `store.NewStore`。

运维时需要了解的行为：

- 整体替换策略存储（`WritePolicyStore`）在一个事务中完成，保留已提供的策略 ID 并为缺失的策略生成 ID；在独立运行的服务器上会失败，且不修改现有数据。
- 删除全部服务或函数时保留集合，change stream 保持打开，ADS 会移除对应的缓存。
- 读取整个存储时分别读取服务和函数，不是同一个快照。
- change stream 因错误结束后，ADS 保留最后加载的策略，不会重新打开 change stream；数据库恢复后请重启 ADS。
- 不支持 Discover 访问请求收集和策略生成；file 与 etcd 存储支持该功能。

| 现象 | 检查项 |
| --- | --- |
| 启动时 server selection 超时 | 副本集是否已初始化并有主节点、`rs.conf()` 中的成员地址能否从 PMS/ADS 访问、URI 主机与端口、防火墙。 |
| ADS 退出并报告 `The $changeStream stage is only supported on replica sets` | 当前是独立运行的 `mongod`；请使用副本集或分片集群。 |
| PMS 已写入，但 ADS 找不到服务 | 两份配置中的 URI 部署和 `MongoDatabase` 名称是否一致。 |
| 策略变更未同步到 ADS | watch 是否开启及 ADS 日志中的 change stream 错误；恢复后重启 ADS。 |
| 身份认证失败 | 用户名、密码编码、`authSource` 以及策略数据库上的角色。 |
| TLS 错误 | `tls=true`、CA 文件路径和证书主机名。 |

后端接口和配置合并规则详见[存储扩展](/docs/store/)。
