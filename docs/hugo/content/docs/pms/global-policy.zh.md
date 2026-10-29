+++
title = "全局角色策略"
description = "Policies take effect globally"
date = 2019-01-18T21:19:44+08:00
weight = 2
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## 什么时候使用全局角色？

普通角色限定在某个服务中。当多个服务需要共享角色分配时，可以在特殊的 `global` 服务中定义角色策略，再在各服务自己的授权策略中引用该角色。

当前仅支持全局**角色策略**。全局角色分配本身并不授予任意操作，各普通服务仍需定义自己的权限；全局授权策略尚不支持。

## 运行示例前

先按[快速入门](/quick-start/)编译仓库根目录下的 `./bin/spctl`、`./bin/speedle-pms` 和 `./bin/speedle-ads`，需要 Go 1.25 或更新版本、Bash、curl 和 Python 3。下面各终端都从仓库根目录运行。为本页使用一个新的策略文件；仅在两个服务都停止后重建该文件。

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


## 创建全局角色与本地权限

<!-- docs-check: global-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user Emma AdminRole" --service-name global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service library
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role AdminRole borrow books" --service-name library
```


全局服务必须准确命名为 `global`，`library` 在创建其策略之前先创建。两个服务均通过普通管理 API 维护。

## 验证结果

Emma 通过 `AdminRole` 获得允许，Bob 被拒绝。

<!-- docs-check: global-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Emma"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


<!-- docs-check: global-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


有资源范围的全局角色策略只在其范围内生效。跨服务角色分配应经过审查，尤其需要区分[不同身份域](/docs/idd/)中的同名主体。
