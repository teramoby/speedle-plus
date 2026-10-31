+++
title = "存储扩展"
description = "Bring your own persistence store!"
weight = 130
draft = false
toc = true
layout = "single"
linksection = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = ""
+++

## 内置存储与扩展接口

独立服务在 `pkg/store` 下注册 file、etcd、MongoDB、MySQL 实现。可用不代表具有相同的运维保证，部署前应分别审查后端认证、持久化、事务与 watch 行为。下方配置说明属性名称，本地示例不涉及启动 etcd/MongoDB 集群。

完整后端实现 [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go)：服务、授权策略、角色策略、函数管理，以及快照读取与 watch/stop。ADS 接受较窄的只读/watch 接口 `PolicyStoreManagerADS`。实现新后端时应以实际接口为准，不能照搬历史文档中不完整的方法列表。

逐步部署教程：[MySQL 配置指南](/docs/store/mysql/)、[etcd 配置指南](/docs/store/etcd/)与 [MongoDB 配置指南](/docs/store/mongodb/)。

## 存储故障与缓存授权

file、etcd、MySQL、MongoDB 使用相同的 ADS 故障策略。
`storeFailureGracePeriod` 默认是 `"0s"`：检测到存储故障后，授权返回
`allowed:false` 和求值错误。ADS 保持运行并持续重试；连接恢复后，还必须成功
加载完整策略快照，才能恢复授权。启用 watch 时，还必须先重新建立故障的订阅；
watch 未恢复时，成功读取数据不会重置故障或宽限期的截止时间。文件被删除时会持续重试，不再退出 ADS。

如果部署方明确接受撤权延迟，可在 ADS JSON 配置的顶层设置有限时长，例如
`"storeFailureGracePeriod": "5s"`。对应参数为
`--store-failure-grace-period=5s`，环境变量为
`SPDL_STORE_FAILURE_GRACE_PERIOD`，继续遵循“显式参数 → 环境变量 → 配置文件 → 默认值”。
重试失败不会延长宽限期。宽限期内按最后一份完整快照授权，因此撤权要等同步恢复
才能生效。缓存部分更新失败和授权计算 panic 不适用宽限期，仍然立即拒绝。

内置存储约每秒执行一次轻量健康检查，单次检查超时为一秒；watch 可能更早报告
故障，正在执行的存储读取也受各自后端超时限制。故障检测和正常策略传播均为异步，
不提供瞬时撤权保证。设置 `enableWatch:false` 时，请求仍检查存储健康，普通策略
变更需要显式刷新；故障恢复时，即使关闭 watch，也必须加载完整快照。首次加载
失败时没有可用缓存，不适用宽限期。自定义存储可以实现 `pms.StoreHealthChecker`
并发送 `pms.STORE_UNAVAILABLE` 事件，接入相同故障策略。

## MySQL 8.4 策略存储

按照 [MySQL 配置指南](/docs/store/mysql/)逐步建库、创建 PMS/ADS 独立账号、配置并启动服务，最后验证策略同步和授权结果。

`mysql` 后端完整实现 PMS 与 ADS 所需的策略存储接口，首版不支持 Discover 访问请求收集和策略生成。使用纯 Go 的 `go-sql-driver/mysql` 驱动；数据库需要预先创建。

```json
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle:replace-me@tcp(db.example.com:3306)/speedle?tls=true",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
```

| 配置属性 | 命令行参数 | 默认值 |
| --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | 必填，包含数据库名 |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `10s` |

环境变量沿用相同映射，例如 `SPDL_MYSQLSTORE_DSN`。时间间隔必须是正的时长字符串；`MySQLInitSchema` 接受 JSON 布尔值或布尔字符串。优先级依次为显式参数、非空环境变量、配置文件、默认值。嵌入使用时导入 `pkg/store/mysql`，直接将上述属性传给 `store.NewStore`，使用完毕后关闭返回的 `*mysql.Store`。通过 `eval.NewFromConfig` 创建的 evaluator 可通过 `io.Closer` 关闭；关闭 evaluator 也会关闭支持关闭操作的传入 store。

### 表结构与初始化

一个数据库对应一个策略存储，使用五张固定名称的 InnoDB 表：`speedle_services`、`speedle_policies`、`speedle_role_policies`、`speedle_functions` 和 `speedle_store_meta`。复杂数组与 metadata 使用 JSON 字段。两张策略表通过外键关联服务，并随服务级联删除。服务名、函数名和策略 ID 区分大小写、保留尾部空格，最长 255 个 Unicode 字符；策略名称允许重复。

默认启动时使用 [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql) 创建缺失的表，不创建数据库，也不修改已有表结构。预建表部署时，先使用初始化账号对选定数据库执行同一份 SQL，再将 `MySQLInitSchema` 设置为 `false`。启动会检查结构版本、必要字段与 InnoDB 引擎；不支持的结构版本会导致启动失败，未来升级需要显式执行迁移。

初始化需要建表和写入初始数据的权限。预建表后，PMS 运行账号需要策略数据库上的 `SELECT`、`INSERT`、`UPDATE`、`DELETE` 权限；ADS 可使用独立的只读 `SELECT` 账号，并关闭自动建表。远程数据库连接通过驱动 DSN 配置 TLS。生产密码不要提交到配置仓库或写入命令行历史。

### 事务、同步与恢复

每次写入都锁定唯一的修订号记录，将数据修改与修订号递增一同提交，因此多个 PMS 实例的策略写入会串行完成。包含嵌套策略的服务创建、级联删除和整库替换均保持原子性。导入保留已有策略 ID，为缺失 ID 生成新值。快照读取使用同一个可重复读事务。

启用 `enableWatch` 后，ADS 轮询修订号，变化时接收完整一致快照。开始监听时也发送快照，覆盖首次读取与建立监听之间的更新。传播是异步的，通常需要一个轮询周期加上查询和缓存重建时间。数据库不可用时，ADS 保留数据但默认暂停缓存授权，持续重试读取，成功加载完整快照后才恢复授权；适用上文统一的故障与宽限期策略。直接执行 SQL 而不更新修订号的修改不会自动触发同步；策略写入应通过 PMS 或存储接口完成。

操作超时也覆盖连接池等待和事务锁等待。写入失败会回滚；若提交时连接中断，结果可能无法确认，重试非幂等创建前应先检查存储状态。`StopWatch` 和 `Close` 均可重复调用，释放监听资源；`Close` 还会关闭连接池。

## 实现并注册 builder

[store.StoreBuilder](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/store.go)提供 `NewStore(map[string]interface{}) (pms.PolicyStoreManager, error)` 与 `GetStoreParams() map[string]string`。后者把 flag 名称映射到配置属性名。在 `init()` 或调用 `store.NewStore` 之前注册唯一的存储类型，重复注册会报错。

以下完整示例复用文件后端，演示工厂接口与注册流程，并不声称实现了一个新数据库：

<!-- docs-check: store -->
```bash
go run ./website/tests/examples/store
```


<!-- docs-source: website/tests/examples/store/main.go -->
```go
package main

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
	"os"
	"path/filepath"
)

// This example delegates to the file backend to demonstrate registration.
// A new backend must implement the complete PolicyStoreManager contract.
type Builder struct{ file.FileStoreBuilder }

func main() {
	dir, err := os.MkdirTemp("", "speedle-store-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store.Register("myfile", Builder{})
	policies, err := store.NewStore("myfile", map[string]interface{}{"FileLocation": filepath.Join(dir, "policies.json")})
	if err != nil {
		log.Fatal(err)
	}
	snapshot, err := policies.ReadPolicyStore()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Registered myfile store; services=%d\n", len(snapshot.Services))
}
```


## 正确合并配置

独立命令的优先级为显式 flag、非空环境变量、配置文件属性、默认值。环境变量名称的规则是添加 `SPDL_`、将 `-` 替换为 `_`，再转为大写；例如 `etcdstore-endpoint` 对应 `SPDL_ETCDSTORE_ENDPOINT`。flag/env 可能提供字符串，JSON 可能提供带类型的值，builder 必须明确验证并转换。

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "localhost:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": true,
      "EmbeddedEtcdDataDir": "./speedle.etcd"
    }
  },
  "enableWatch": true
}
```

自定义 flag 需在解析前通过 `pflag` 定义，并由 `GetStoreParams` 返回 flag 到属性的映射。嵌入式调用直接把配置 map 交给 `store.NewStore` 或 `eval.NewFromConfig`；工厂本身不会自动读取命令行或环境变量。

## 链接后端并处理更新

若在 `init()` 中注册，独立 PMS、ADS 或嵌入式应用都要通过 side-effect import 链接实现：

```go
import (
    _ "github.com/teramoby/speedle-plus/pkg/store/etcd"
    _ "github.com/teramoby/speedle-plus/pkg/store/file"
    _ "github.com/teramoby/speedle-plus/pkg/store/mongodb"
    _ "github.com/teramoby/speedle-plus/pkg/store/mysql"
)
```

`Watch()` 通过 `pms.StorageChangeChannel` 返回正确的快照/变更事件，`StopWatch()` 应能释放监听资源。ADS 异步更新运行时缓存。需测试 watch 断连、漏事件、恢复、写入失败与并发读取；不能仅因后端支持 watch，就承诺策略会瞬间传播。
