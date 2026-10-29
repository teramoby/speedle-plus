+++
title = "存储扩展"
description = "Bring your own persistence store!"
weight = 130
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = ""
+++

## 内置存储与扩展接口

独立服务在 `pkg/store` 下注册 file、etcd、MongoDB 实现。可用不代表具有相同的运维保证，部署前应分别审查后端认证、持久化、事务与 watch 行为。下方配置说明属性名称，本地示例不涉及启动 etcd/MongoDB 集群。

完整后端实现 [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go)：服务、授权策略、角色策略、函数管理，以及快照读取与 watch/stop。ADS 接受较窄的只读/watch 接口 `PolicyStoreManagerADS`。实现新后端时应以实际接口为准，不能照搬历史文档中不完整的方法列表。

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
)
```

`Watch()` 通过 `pms.StorageChangeChannel` 返回正确的快照/变更事件，`StopWatch()` 应能释放监听资源。ADS 异步更新运行时缓存。需测试 watch 断连、漏事件、恢复、写入失败与并发读取；不能仅因后端支持 watch，就承诺策略会瞬间传播。
