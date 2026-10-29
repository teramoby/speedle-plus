+++
title = "策略发现"
description = "Discover policies on the fly!"
weight = 4
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["policy", "core", "discovery"]
categories = ["docs"]
bref = ""
+++

## 策略发现的作用

发现模式记录请求，成功记录时返回 `allowed=true`、reason 5，不代表既有策略曾授权该操作。仅在隔离测试环境使用，审核生成的授权，并在结束后恢复正常的 `is-allowed` 检查。断言或存储错误仍需单独处理，不能把错误当作授权成功。

## 启动可复现的本地示例

先按[快速入门](/quick-start/)编译二进制。下面各终端均从源码根目录运行，使用新的策略文件，并先停止其他页面示例的服务。

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

终端一：

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

终端二：

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

保持两个服务运行。在终端三记录一个请求：

<!-- docs-check: discover-record -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/discover
```

检查记录的请求，再生成待审核的服务定义：

<!-- docs-check: discover-list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover request --service-name observed
```
<!-- docs-check: discover-export -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover policy --service-name observed > discovered-service.json
```

审核 `discovered-service.json` 中的角色、身份域和资源。本例尚不存在名为 `observed` 的策略服务，因此可以直接导入；已有同名服务时应合并和审核其策略，而不是直接重复创建。

<!-- docs-check: discover-import -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file discovered-service.json
```

## 恢复并验证正常权限检查

重新调用 `is-allowed`。ADS 加载策略更新后，alice 应被允许，bob 仍应被拒绝：

<!-- docs-check: discover-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: discover-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

结束后使用 Ctrl+C 停止两个服务。gRPC 使用生成客户端的 `Discover`/`IsAllowed`，嵌入式 Go 使用 `evaluator.Discover(ctx)`/`evaluator.IsAllowed(ctx)`。CLI 参数可通过 `./bin/spctl discover --help` 查看。不要让正式应用继续调用 `/discover`。
