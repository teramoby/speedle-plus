+++
title = "授权查询"
keywords = ["Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化", "PDP", "policy decision point", "策略决策点", "authorization decision service", "授权决策服务", "API authorization", "API 授权", "API 权限控制"]
description = "查询决策、角色与权限，并正确处理错误"
weight = 30
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pdp", "policy", "core"]
categories = ["docs"]
bref = ""
+++

## 什么是授权决策？

Speedle 在指定服务内评估主体、操作与资源，由应用执行结果。应区分评估错误、超时与正常的 Deny，并在出错时拒绝访问。REST、gRPC、嵌入式 Go 的数据表示不同，应分别遵循各自的 [API 契约](/docs/api/)。

## 运行示例前

先按[快速入门](/quick-start/)编译仓库根目录下的 `./bin/spctl`、`./bin/speedle-pms` 和 `./bin/speedle-ads`，需要 Go 1.26 或更高版本、Bash、curl 和 Python 3。下面各终端都从仓库根目录运行。为本页使用一个新的策略文件；仅在两个服务都停止后重建该文件。

初始化空策略文件：

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

终端一启动 PMS，并保持运行：

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

终端二启动 ADS，并保持运行：

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

在第三个终端运行后续命令。PMS/ADS 共享同一个文件，策略更新通过 watch 异步到达 ADS；先查询确认已更新，再依赖新策略做决策。`--insecure=true` 仅用于回环地址上的本地示例，生产连接见[安全配置](/docs/security/)。示例显式指定 PMS 地址，不修改 `~/.spctlconfig`。完成后在两个服务终端按 Ctrl-C。


## 创建可重复的测试策略

[测试文件](https://github.com/teramoby/speedle-plus/blob/master/samples/docs/bookstore.json)给 Alan 分配 `role1`、`role2`；其中 `role1` 可以下载 `/books/HarryPotter`。

<!-- docs-check: bookstore-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file samples/docs/bookstore.json
```


## 获取授权决策

Alan 的请求允许；Bob 没有适用的允许策略，请求被拒绝。

<!-- docs-check: decision-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


<!-- docs-check: decision-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


成功评估返回 `{"allowed":true,"reason":0}` 或对应的 Deny 对象。HTTP 错误返回错误响应，不能视为成功授权。原因码如下：

| 数值 | 含义 |
| --- | --- |
| 0 | GRANT_POLICY_FOUND |
| 1 | DENY_POLICY_FOUND |
| 2 | SERVICE_NOT_FOUND |
| 3 | NO_APPLICABLE_POLICIES |
| 4 | ERROR_IN_EVALUATION |
| 5 | DISCOVER_MODE |
| 6 | REASON_NOT_AVAILABLE |

`DISCOVER_MODE` 不表示生产授权通过；`REASON_NOT_AVAILABLE` 是回退原因值。这些数值不是 HTTP 状态码。

## 查询已授予角色

<!-- docs-check: decision-roles -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-roles
```


预期包含 `role1`、`role2`，顺序无意义。查询有资源范围的角色分配时，需要传入对应的 `resource`。

## 查询已授予权限

<!-- docs-check: decision-permissions -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-permissions
```


预期权限为资源 `/books/HarryPotter` 上的 `download` 操作。此接口列出已授予权限；执行具体业务操作仍应调用 `is-allowed`，不能用权限列表代替完整的请求评估。

## 属性、诊断与令牌身份

REST 请求属性是带类型的对象数组，例如 `{"name":"clearance","type":"numeric","value":4}`；嵌入式 Go 使用 `map[string]interface{}`。诊断响应的属性也是对象，不能与请求属性数组混淆。属性类型与条件见 [SPDL](/docs/spdl/)，过程明细见[授权诊断](/docs/pms/diagnosis/)，令牌校验见[令牌断言](/docs/assertor/)。
