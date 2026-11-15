+++
title = "策略管理"
description = "管理服务、授权策略、角色策略和自定义函数的完整生命周期。"
weight = 1
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pms", "policy", "core"]
categories = ["docs"]
bref = "策略管理基础"
lastmod = 2026-11-09T07:57:54+08:00
+++

## 什么是 Speedle 策略？

策略管理服务（PMS）管理服务、授权策略、角色策略与自定义函数，由已注册的[存储后端](/docs/store/)持久化这些对象。

<img src="/img/speedle/policystore.png" width="1942" height="540" loading="lazy"/>

## 服务与授权策略

服务限定策略的作用范围。授权策略允许或拒绝主体对资源执行操作，并可附带条件。除[全局角色策略](/docs/pms/global-policy/)外，一个服务的策略不会自动在另一个服务中生效。

<img src="/img/speedle/authzpolicy.png" width="1770" height="444" loading="lazy"/>

```text
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
```

此正则只匹配 `c1/default/core/pods/` 下的资源，不匹配 `c1/default/core/podssecret` 或带额外前缀的路径。`expr:` 使用正则表达式，不是 shell 通配符：任意字符使用 `.*`，需要匹配完整标识符时加 `^`、`$`。

## 角色策略

<img src="/img/speedle/rolepolicy.png" width="1598" height="436" loading="lazy"/>

```text
grant user alan manager on res1
grant role manager update res1
```

第一行把指定资源上的角色分配给 Alan，第二行才为该角色授予操作权限。仅分配角色不会自动产生权限。适用的显式拒绝优先于允许。

## 主体与条件

主体可以是用户、组、实体或角色。实体表示服务等非人身份，集成代码必须从可信来源获取这些身份。

<img src="/img/speedle/principal.png" width="1769" height="505" loading="lazy"/>

AND 主体组要求组内所有身份均匹配，多组之间为 OR。以下例子要求两个角色都有效：

<img src="/img/speedle/andprincipal.png" width="1584" height="349" loading="lazy"/>

```text
grant (role designer, role dba) update db_design_doc
```

资源指需要保护的对象，操作是应用定义的动作。条件是由可信属性、常量、运算符和受支持函数构成的布尔表达式。

<img src="/img/speedle/resource.png" width="1732" height="349" loading="lazy"/>

语法见 [SPDL](/docs/spdl/)，完整决策示例见 [RBAC](/docs/overview/) 与 [ABAC](/docs/overview/)。

## 运行示例前

先按[快速入门](/quick-start/)编译仓库根目录下的 `./bin/spctl`、`./bin/speedle-pms` 和 `./bin/speedle-ads`，需要 Go 1.27.4（也可使用同一大版本的更新补丁版本）、Bash、curl 和 Python 3。下面各终端都从仓库根目录运行。为本页使用一个新的策略文件；仅在两个服务都停止后重建该文件。

初始化空策略文件：

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

> **macOS：** `/usr/sbin/spctl` 是 Apple 的 Gatekeeper 工具，直接运行 `spctl` 时可能会选中它。请使用 Speedle CLI 的完整路径，或在其所在目录中用 `./spctl` 调用；下面的示例统一使用 `./bin/spctl`。运行 `command -v spctl` 可查看 shell 实际选择的可执行程序。如果下载的发布版本包含 `speedle-spctl_<os>_<arch>` 文件，它就是同一个 CLI，只是名称不会与 Apple 的工具冲突。

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


## 使用 spctl 管理服务与策略

在新存储上按顺序执行。ID 从实际查询响应中取得，不使用历史输出里的固定值。所有依赖服务的操作完成后，再删除服务。

<!-- docs-check: policy-management -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy policy1 -c "grant user alan read book" --service-name test
policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="policy1"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy rolepolicy01 -c "grant user alan manager on res1" --service-name test
role_policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="rolepolicy01"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete service test
```


## Speedle Cloud

使用账号 API 密钥，通过 `spctl` 管理 Cloud 服务和策略。管理端点为：

```text
https://cloud.speedle.io/policy-mgmt/v1
```

账号 API 密钥只能选择一个固定权限范围。`pms:read` 可读取服务和策略；`pms:write` 还可创建和删除服务与策略。两者都适用于账号下的服务。`ads` 密钥不能管理策略，会收到 `403 insufficient_scope`。创建和撤销规则见 [API 密钥](/zh/docs/cloud/tokens/)。

通过全局命令行参数指定端点，并从 `SPCTL_TOKEN` 环境变量读取API 密钥。在交互式 shell 中，以下方式可避免将 secret 写入命令参数和 shell 历史：

<!-- deployment-template: cloud-token-environment -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
export SPCTL_TOKEN
./bin/spctl --pms-endpoint https://cloud.speedle.io/policy-mgmt/v1 get service --all
unset SPCTL_TOKEN
```

`--token` 参数仍为兼容现有用法而保留，但环境变量可避免通过命令参数暴露API 密钥。每个全局参数都有对应的环境变量：添加 `SPCTL_` 前缀，将参数名转为大写，并把连字符替换为下划线。例如，`SPCTL_PMS_ENDPOINT` 对应 `pms-endpoint`，`SPCTL_SKIPVERIFY` 对应 `skipverify`。

如需保存配置，请使用 `~/.speedle`。文件每行包含一个 `名称=值` 配置项，名称与命令行参数一致：

```ini
pms-endpoint=https://cloud.speedle.io/policy-mgmt/v1
token=<api-key>
```

以下方式可在不把API 密钥写入命令参数的情况下创建或更新此文件：

<!-- deployment-template: cloud-token-config -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
printf '%s\n' "$SPCTL_TOKEN" | ./bin/spctl config token --stdin
unset SPCTL_TOKEN
```

`spctl config token --stdin` 会以 `0600` 权限写入 `~/.speedle`。手动编辑此文件时，先用 `install -m 600 /dev/null ~/.speedle` 创建文件，或在编辑后运行 `chmod 0600 ~/.speedle`。如果 `spctl` 从允许 group 或其他用户访问的文件中读取API 密钥，会向标准错误输出警告。`spctl config --list` 会把已配置的API 密钥显示为 `********`。

显式命令行参数优先于环境变量，环境变量优先于两个配置文件。在配置文件中，`~/.speedle` 优先于 `~/.spctlconfig`。通过 `spctl config` 设置 `token` 以外的配置项时，新值写入 `~/.spctlconfig`，同时删除 `~/.speedle` 中会覆盖新值的同名配置。

API 密钥仅在启用证书验证的 HTTPS 连接中以 `Authorization: Bearer` 凭据发送。只有 `127.0.0.1`、`::1` 或 `localhost` 等回环主机可以在携带API 密钥时使用纯 HTTP；对于远程 HTTP 端点以及从 HTTPS 降级到 HTTP 的重定向，`spctl` 会在发送凭据前拒绝请求。对于 HTTPS 端点，`spctl` 也拒绝在 `--skipverify=true` 时发送API 密钥；对于私有或自签名证书颁发机构，请使用 `--cacert` 配置信任。正常命令输出和错误信息不会包含API 密钥。

Cloud 返回错误时，命令以非零状态退出。遇到 `401` 请检查 API 密钥，遇到 `403 insufficient_scope` 请检查 scope。`404` 表示资源不存在，`409 service_exists` 和 `409 policy_exists` 表示资源重复。遇到 `429` 时，请遵循响应中的 `Retry-After`，并采用有界退避。遇到 `429 daily_quota_exceeded`，按 `Retry-After` 等待至 00:00 UTC。服务和策略上限返回 `409 quota_exceeded`，不会随时间重置；请删除不用的项目。撤销 API 密钥，或等它到期，可释放密钥名额。参见[限额与错误](/zh/docs/cloud/limits-and-errors/)。


## 其他管理接口

[PMS REST API](/docs/api/management_api/)使用 JSON，返回包含字符串 ID 的扁平策略对象。Go 应用可直接使用 [PMS 接口](https://github.com/teramoby/speedle-plus/tree/master/api/pms)，gRPC 客户端使用[公开的 PMS proto](/protobuf/pms.proto)。管理操作必须配置[传输安全与调用方访问控制](/docs/security/)。
