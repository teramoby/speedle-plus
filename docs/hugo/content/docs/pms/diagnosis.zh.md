+++
title = "授权诊断"
description = "解释授权评估的策略与属性明细"
date = 2019-01-18T15:46:59+08:00
weight = 3
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## 授权诊断提供什么？

`Diagnose` 用于解释一次授权评估：决策、原因、有效角色、属性及参与评估的策略。其请求上下文与 `IsAllowed` 相同；REST 路径为 `/authz-check/v1/diagnose`，Go/gRPC 方法均名为 `Diagnose`。

诊断会暴露策略定义和请求上下文，必须通过[API 安全配置](/docs/security/)限制为可信运维人员访问，不能作为公开调试端点。

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


## 创建示例策略

[测试文件](https://github.com/teramoby/speedle-plus/blob/master/samples/docs/diagnosis.json)允许 `user1` 对 `^/api/v1/example/.*$` 下的资源执行 `get`，但显式拒绝 `/api/v1/example/res1`。

<!-- docs-check: diagnosis-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file samples/docs/diagnosis.json
```


## 比较决策与诊断

<!-- docs-check: diagnosis-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


预期 `allowed=false`，`reason=1`，表示匹配显式拒绝策略。

<!-- docs-check: diagnose -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/diagnose
```


诊断响应的 `allowed` 是布尔值，`reason` 是 `DENY_POLICY_FOUND` 等字符串，`attributes` 是按名称索引的对象，`request_month` 是数字月份。`requestContext.subject.principals` 为数组；授权策略中的 `principals` 是二维数组，表示多组 AND 主体之间的 OR 关系。

| 字段 | 含义 |
| --- | --- |
| `requestContext` | 实际评估的请求；这里的 REST 属性仍是带类型的数组 |
| `attributes` | 请求属性与内置属性，以属性名作为键 |
| `grantedRoles` | 存在时返回有效角色 |
| `rolePolicies`、`policies` | 存在时返回角色策略与授权策略的评估明细 |
| `condition` | 条件表达式及结果；无条件策略可返回空对象 |

策略状态包括 `takeEffect`（生效）、`conditionFailed`（条件不满足）、`ignored`（最终结果已确定，无需再评估）。顺序可能变化，应根据 ID 与状态解释结果，不要逐字匹配历史响应。完整响应结构见[决策 API](/docs/api/decision_api/)。
