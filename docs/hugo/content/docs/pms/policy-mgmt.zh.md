+++
title = "策略管理"
description = "Manage policy lifecycle "
weight = 1
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pms", "policy", "core"]
categories = ["docs"]
bref = "Basics of policy management"
+++

## 什么是 Speedle 策略？

策略管理服务（PMS）管理服务、授权策略、角色策略与自定义函数，由已注册的[存储后端](/docs/store/)持久化这些对象。

<img src="/img/speedle/policystore.png"/>

## 服务与授权策略

服务限定策略的作用范围。授权策略允许或拒绝主体对资源执行操作，并可附带条件。除[全局角色策略](/docs/pms/global-policy/)外，一个服务的策略不会自动在另一个服务中生效。

<img src="/img/speedle/authzpolicy.png"/>

```text
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
```

此正则只匹配 `c1/default/core/pods/` 下的资源，不匹配 `c1/default/core/podssecret` 或带额外前缀的路径。`expr:` 使用正则表达式，不是 shell 通配符：任意字符使用 `.*`，需要匹配完整标识符时加 `^`、`$`。

## 角色策略

<img src="/img/speedle/rolepolicy.png"/>

```text
grant user alan manager on res1
grant role manager update res1
```

第一行把指定资源上的角色分配给 Alan，第二行才为该角色授予操作权限。仅分配角色不会自动产生权限。适用的显式拒绝优先于允许。

## 主体与条件

主体可以是用户、组、实体或角色。实体表示服务等非人身份，集成代码必须从可信来源获取这些身份。

<img src="/img/speedle/principal.png"/>

AND 主体组要求组内所有身份均匹配，多组之间为 OR。以下例子要求两个角色都有效：

<img src="/img/speedle/andprincipal.png"/>

```text
grant (role designer, role dba) update db_design_doc
```

资源是被保护的对象，操作是应用定义的动作。条件是由可信属性、常量、运算符与受支持函数构成的布尔表达式。

<img src="/img/speedle/resource.png"/>

语法见 [SPDL](/docs/spdl/)，完整决策示例见 [RBAC](/docs/overview/) 与 [ABAC](/docs/overview/)。

## 运行示例前

先按[快速入门](/quick-start/)编译仓库根目录下的 `./bin/spctl`、`./bin/speedle-pms` 和 `./bin/speedle-ads`，需要 Go 1.26.10 或 Go 1.27.3（也可使用同一大版本的更新补丁版本）、Bash、curl 和 Python 3。下面各终端都从仓库根目录运行。为本页使用一个新的策略文件；仅在两个服务都停止后重建该文件。

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


## 其他管理接口

[PMS REST API](/docs/api/management_api/)使用 JSON，返回包含字符串 ID 的扁平策略对象。Go 应用可直接使用 [PMS 接口](https://github.com/teramoby/speedle-plus/tree/master/api/pms)，gRPC 客户端使用[公开的 PMS proto](/protobuf/pms.proto)。管理操作必须配置[传输安全与调用方访问控制](/docs/security/)。
