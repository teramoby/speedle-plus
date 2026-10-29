+++
title = "MySQL 配置指南"
description = "逐步配置 MySQL 8.4 策略存储，启动 PMS 与 ADS，并验证策略同步。"
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mysql"]
categories = ["docs"]
bref = "从建库、创建账号到配置 PMS/ADS，再验证授权、策略自动同步与持久化。"
+++

本指南使用 **MySQL 8.4 LTS**，分别启动 PMS 和 ADS：PMS 写入策略，ADS 使用独立的只读账号读取同一个数据库。本地演示使用 Docker，HTTP 服务只监听本机回环地址。远程部署可使用已有的 MySQL，并参考[安全配置](/docs/security/)。

## 1. 编译包含 MySQL 后端的 Speedle

准备 Git、Bash、curl、已启动的 Docker，以及支持的 Go 工具链（Go 1.26.10 或 Go 1.27.3）。从当前源码编译，确保两个服务都包含 `mysql` 后端；已安装的旧版本可能没有该后端。

<!-- mysql-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

后续命令都在这个 `speedle-plus` 目录执行。确认 3306、6733、6734 端口未被占用；若修改端口，需同步修改下文的命令和配置。

## 2. 启动 MySQL 8.4

下列固定密码仅为**本地演示凭据**。部署时请使用自己的凭据与密钥交付方式。命名卷会在容器停止或重建后保留数据库。

<!-- mysql-step: database -->
```bash
docker run --name speedle-mysql --detach \
  --publish 127.0.0.1:3306:3306 \
  --volume speedle-mysql-data:/var/lib/mysql \
  --env MYSQL_ROOT_PASSWORD=SpeedleDemoRoot84 \
  mysql:8.4
```

等待 MySQL 初始化完成：

<!-- mysql-step: ready -->
```bash
until docker exec speedle-mysql mysqladmin ping \
  --host=127.0.0.1 --user=root --password=SpeedleDemoRoot84 --silent
do
  sleep 1
done
```

已有 MySQL 8.4 时，跳过 Docker 启动命令，使用管理员客户端执行下一步 SQL。Docker 镜像只在数据目录为空时初始化账号；对已有数据卷修改环境变量不会重置密码。

## 3. 创建数据库和服务账号

Speedle 默认自动建表，但**不会创建数据库**。先以管理员身份创建 `speedle` 数据库及两个独立账号：

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

PMS 自动建表需要 `CREATE`、`REFERENCES`，管理策略还需要 `SELECT`、`INSERT`、`UPDATE`、`DELETE`。ADS 只需 `SELECT`，并必须关闭自动建表。示例中的账号主机 `%` 允许宿主机通过 Docker 连接；远程部署应限制为实际 PMS/ADS 主机。

## 4. 写入 PMS 与 ADS 配置文件

两个 DSN 必须指向**同一个 MySQL 服务和数据库**。DSN 格式为 `用户:密码@tcp(主机:端口)/数据库`。

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

`MySQLInitSchema: false` 表示只检查已有表结构，不执行 DDL。`enableWatch: true` 让 ADS 自动同步已提交的变更。配置文件包含密码，请勿提交到代码仓库。

## 5. 先启动 PMS，再启动 ADS

在终端 A 中启动 PMS，由它完成建表：

<!-- mysql-step: pms -->
```bash
./bin/speedle-pms --config-file mysql-demo/pms.json --endpoint 127.0.0.1:6733 --insecure=true
```

保持 PMS 运行。在终端 B 中进入同一个代码目录，确认表已创建：

<!-- mysql-step: tables -->
```bash
docker exec speedle-mysql mysql --user=speedle_ads --password=SpeedleDemoAds84 speedle -e 'SHOW TABLES;'
```

应看到下面五张表：

| 表名 | 保存内容 |
| --- | --- |
| `speedle_services` | 服务及服务元数据 |
| `speedle_policies` | 授权策略 |
| `speedle_role_policies` | 角色策略 |
| `speedle_functions` | 自定义函数定义 |
| `speedle_store_meta` | 结构版本与已提交的策略修订号 |

然后在终端 B 中启动 ADS，并保持运行：

<!-- mysql-step: ads -->
```bash
./bin/speedle-ads --config-file mysql-demo/ads.json --endpoint 127.0.0.1:6734 --insecure=true
```

等两个服务的 REST 监听启动后再继续。未传入 `--grpc-endpoint` 时，gRPC 服务默认关闭。

## 6. 创建策略并检查授权

在终端 C 创建服务，授予 Alice 读取 `book` 的权限：

<!-- mysql-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mysql-demo","type":"application","policies":[{"id":"alice-read","name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

等待 ADS 同步。默认轮询间隔为 1 秒，还需加上查询和缓存重建时间；如果请求发生在同步完成之前，稍后重试：

<!-- mysql-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

预期返回 `{"allowed":true,"reason":0}`。Bob 没有匹配的授权策略：

<!-- mysql-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

预期返回 `{"allowed":false,"reason":3}`。

## 7. 验证自动更新和持久化

通过 PMS 新增一条拒绝 Alice 的策略，无需重启 ADS：

<!-- mysql-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mysql-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"id":"alice-deny","name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

同步后，再执行第 6 步 Alice 的请求，应返回 `{"allowed":false,"reason":1}`，表示显式 deny 优先。也可以按 Ctrl+C 停止 PMS 和 ADS，再用第 5 步命令启动。再次请求仍应拒绝 Alice，说明策略已保存在 MySQL 中。

策略变更应通过 PMS 或存储接口完成，确保数据与修订号在同一事务内提交。直接修改 SQL 不会自动触发 ADS 重载。数据库断线时，已运行的 ADS 保留最后加载的策略并重试同步；ADS 启动时仍需要数据库可连接。

## 可选：预先建表，运行时不授予 DDL 权限

如果不允许服务账号自动建表，可在启动服务前，用初始化账号执行仓库中的同一份 [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql)：

<!-- mysql-step: schema -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 speedle < pkg/store/mysql/schema.sql
```

将**两个**配置文件中的 `MySQLInitSchema` 都设置为 `false`。如果沿用了本指南前面的演示账号，可撤销 PMS 的初始化权限：

<!-- mysql-step: revoke-ddl -->
```bash
docker exec speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 -e "REVOKE CREATE, REFERENCES ON speedle.* FROM 'speedle_pms'@'%';"
```

此后 PMS 仅需 `SELECT`、`INSERT`、`UPDATE`、`DELETE`，ADS 保持只读。修改配置后重启两个服务。初始化 SQL 不会迁移已有表；不支持的结构版本会导致启动失败。

## 配置速查与故障排查

| 配置属性 | 命令行参数 | 环境变量 | 默认值 |
| --- | --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | `SPDL_MYSQLSTORE_DSN` | 必填，必须包含数据库名 |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `SPDL_MYSQLSTORE_INIT_SCHEMA` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `SPDL_MYSQLSTORE_POLL_INTERVAL` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `SPDL_MYSQLSTORE_QUERY_TIMEOUT` | `10s` |

选择后端的参数是 `--store-type mysql`，对应 `SPDL_STORE_TYPE=mysql`。配置优先级为：显式参数 → 非空环境变量 → 配置文件 → 默认值。连接远程数据库时，使用带可信服务端证书的主机名，并在 DSN 后添加 `?tls=true`；自定义 CA 的配置请参阅[驱动 TLS 文档](https://github.com/go-sql-driver/mysql/tree/v1.10.1#tls)。

| 现象 | 检查项 |
| --- | --- |
| 不识别 `mysql` 后端或参数 | 从包含 `pkg/store/mysql` 的源码重新编译两个服务。 |
| 连接被拒绝或访问被拒绝 | MySQL 是否就绪，DSN 地址、端口、密码及账号主机限制是否正确。 |
| 数据库不存在 | 先创建数据库，Speedle 只负责建表。 |
| 表不存在或 DDL 权限不足 | 先启动 PMS 或手动建表；只读 ADS 必须设置 `MySQLInitSchema: false`。 |
| 修改策略后未生效 | 两个 DSN 是否同库、ADS 是否启用 watch、是否已等待同步，以及 `SPDL_*` 环境变量是否覆盖了文件配置。 |

MySQL 不支持 Discover 访问请求收集和策略生成。事务、快照和恢复机制详见[存储扩展](/docs/store/)。
