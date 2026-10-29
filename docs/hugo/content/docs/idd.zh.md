+++
title = "用户身份域"
description = "Speedle supports user identities from multiple identity domains"
date = 2019-01-17T13:20:07+08:00
weight = 40
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["identity domain", "identity"]
+++

## 什么是身份域？

身份域（`idd`）为主体提供显式命名空间。同名的 `user1` 可以属于不同的域。域字符串由可信应用或已配置的令牌断言服务提供，本身不代表完成了身份认证。Speedle 不会仅根据域名称推断或验证身份提供方的租户模型。

## 在策略与请求中表示身份域

```text
grant user user1 from github read book
grant user user1 from IDCS.tenant01 read book
```

在主体名后使用 `from <identity-domain>`。PMS JSON 策略中的等价编码为 `idd=github:user:user1`；决策请求中的主体表示为：

```json
{
  "type": "user",
  "name": "user1",
  "idd": "github"
}
```

带 `from github` 的策略只匹配该域；省略 `from` 则按主体名跨域匹配。多租户应用应明确审查这一选择；域字符串不能用于验证令牌。更多内容见[身份与 IAM](/docs/overview/)和[令牌断言](/docs/assertor/)。

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


## 导入测试策略

[测试文件](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/idd.json)允许 GitHub 的 `user1` 读取、Google 的 `user1` 写入，并允许任何域的 `user1` 租借。小写域名是演示命名空间。

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/idd.json
```


## 比较决策

依次预期为 Allow、Deny、Allow、Deny。`notgoogle` 中的用户不能使用授予 Google 身份的 `write` 权限。

<!-- docs-check: idd-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"gitlab"}]},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"google"}]},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-write-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"notgoogle"}]},"serviceName":"booksvc","resource":"book","action":"write"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


JSON 请求必须提供 `Content-Type: application/json`。管理接口使用字符串策略 ID 和复数形式的 `permissions` 字段，见[管理 API](/docs/api/management_api/)。
