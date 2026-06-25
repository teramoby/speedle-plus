<p align="center">
    <img src="/docs/images/sp_logo.png" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ">
        <img src="https://img.shields.io/badge/slack-speedle-red.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/tags">
        <img src="https://img.shields.io/github/v/tag/teramoby/speedle-plus">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml">
        <img src="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml/badge.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/teramoby/speedle-plus">
        <img src="https://goreportcard.com/badge/github.com/teramoby/speedle-plus">
    </a>
    <a href="https://pkg.go.dev/github.com/teramoby/speedle-plus">
        <img src="https://pkg.go.dev/badge/github.com/teramoby/speedle-plus.svg">
    </a>
    <a href="https://996.icu/#/zh_CN">
        <img src="https://img.shields.io/badge/link-996.icu-red.svg">
    </a>
</p>

<p align="right">
<a href="README.md">English Version</a>
</p>

# Speedle+

Speedle+ 是一个通用授权引擎。用户可以使用它非常易用的策略定义语言构建自己的授权模型，并能在数毫秒之内得到授权结果。Speedle 非常易用，非常高效，而且可扩展性能力非常强。用户可以在 Speedle 里面管理百万规模级的授权策略。

作为一个开源项目，Speedle 包括策略定义语言（SPDL），策略管理模块，授权决策模块，命令行工具，以及数个和流行系统集成的示例。

Speedle+ 项目基于 Speedle 项目 <https://github.com/oracle/speedle>，两个项目都遵守 UPL 协议。

## 我们是谁

我们是 Speedle 项目的创始团队。因为大家知道的某些原因，我们都离开了 Oracle，现在也不是 Speedle 项目的贡献者了。但是我们还在继续维护这个项目，所以建立了一个新项目 Speedle+。

---

## 目录

- [系统架构](#系统架构)
- [快速开始](#快速开始)
- [SPDL — 安全策略定义语言](#spdl--安全策略定义语言)
- [策略管理](#策略管理)
- [授权决策](#授权决策)
- [高级功能](#高级功能)
  - [策略发现](#策略发现)
  - [策略诊断](#策略诊断)
  - [全局策略](#全局策略)
  - [身份域](#身份域)
  - [令牌断言](#令牌断言)
  - [自定义函数](#自定义函数)
- [存储](#存储)
- [安全与 TLS](#安全与-tls)
- [日志](#日志)
- [部署](#部署)
- [集成](#集成)
- [API 参考](#api-参考)
- [配置文件](#配置文件)
- [构建](#构建)
- [测试](#测试)
- [应用场景](#应用场景)
- [社区与贡献](#社区与贡献)

---

## 系统架构

Speedle+ 由三个核心组件组成：

```
                    +-----------+
                    |   spctl   |  命令行工具，用于管理策略
                    +-----+-----+
                          |
              +-----------+-----------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      |     PMS      |        |     ADS      |
      |  策略管理服务 |  推送  |  授权决策服务 |
      |              +------->+              |
      +-------+------+        +-------+------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      |   策略存储   |        |   策略存储   |
      | (file/etcd/  |<-------+   (只读)     |
      |  mongodb)    |  监听  |              |
      +--------------+        +--------------+
```

- **spctl**: 命令行工具，用于创建、查询、更新和删除策略、服务和函数。通过 REST API 与 PMS 通信。
- **PMS (策略管理服务)**: 提供 REST/gRPC API，管理策略生命周期，将策略写入后端存储。
- **ADS (授权决策服务)**: 实时评估授权请求。将策略加载到内存缓存中，并监听存储变更以保持同步。

如果你熟悉 XACML 模型，PMS 相当于策略管理点 (PAP)，ADS 相当于策略决策点 (PDP)。

**服务端口:**

| 服务 | 二进制文件 | 默认端口 |
|------|-----------|---------|
| 策略管理 | `speedle-pms` | 6733 |
| 授权决策 | `speedle-ads` | 6734 |
| 命令行工具 | `spctl` | N/A（连接 PMS） |

## 快速开始

以下步骤演示如何在本地使用基于文件的策略存储运行 Speedle+。

### 前期准备

- Go 1.22 或更高版本: <https://go.dev/doc/install>

### 1. 构建二进制文件

```bash
git clone https://github.com/teramoby/speedle-plus.git
cd speedle-plus
make build
ls $(go env GOPATH)/bin
# 预期输出: spctl  speedle-ads  speedle-pms
```

或使用 `go install` 直接安装：

```bash
go install github.com/teramoby/speedle-plus/cmd/spctl@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
```

然后将 `$(go env GOPATH)/bin` 添加到 `PATH` 环境变量中，或使用二进制文件的完整路径。

### 2. 启动策略管理服务 (PMS)

在一个终端中运行：

```bash
speedle-pms --store-type file --insecure true
```

默认策略存储文件将创建在 `/tmp/speedle-test-file-store.json`。

### 3. 使用 spctl 创建策略

在另一个终端中运行：

```bash
# 创建服务
spctl create service mysvc

# 授予 user1 对 res1 的访问权限
spctl create policy -c "grant user user1 get,del res1" --service-name=mysvc

# 授予 role2 对 res2 的访问权限
spctl create policy -c "grant role role2 get,del res2" --service-name=mysvc

# 将 user2 分配到 role2，作用于 res2
spctl create rolepolicy -c "grant user user2 role2 on res2" --service-name=mysvc
```

### 4. 启动授权决策服务 (ADS)

在另一个终端中运行：

```bash
speedle-ads --store-type file --insecure true
```

### 5. 验证授权决策

在又一个终端中，使用 `curl` 测试：

```bash
# user1 应该被允许访问 res1
curl -s -X POST \
  -H "Content-Type: application/json" \
  --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
# 预期: {"allowed":true,"reason":0}

# user2 应该被允许访问 res2（通过角色）
curl -s -X POST \
  -H "Content-Type: application/json" \
  --data '{"subject":{"principals":[{"type":"user","name":"user2"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
# 预期: {"allowed":true,"reason":0}

# user1 不应该被允许访问 res2
curl -s -X POST \
  -H "Content-Type: application/json" \
  --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
# 预期: {"allowed":false,"reason":3}
```

> **注意：** `--insecure true` 标志会禁用传输安全，仅适用于本地开发。
> 调用 ADS REST API 时必须添加 `Content-Type: application/json` 请求头。
> 生产环境的 TLS 配置请参见[安全与 TLS](#安全与-tls)章节。

---

## SPDL — 安全策略定义语言

SPDL 是 Speedle 易读易写的策略定义语言。

### 关键字（保留，不区分大小写）

`role`、`user`、`group`、`entity`、`grant`、`deny`、`if`、`in`、`on`、`from`

### 策略语法

```
POLICY = EFFECT SUBJECT ACTION RESOURCE [if CONDITION]
EFFECT = grant | deny
SUBJECT = PRINCIPAL_TYPE PRINCIPAL_NAME [, PRINCIPAL_TYPE PRINCIPAL_NAME]*
PRINCIPAL_TYPE = user | group | entity | role
ACTION = ACTION_NAME [, ACTION_NAME]*
RESOURCE = RESOURCE_NAME
```

**示例：**

```sh
# 允许 "employee" 组用户读取书籍
spctl create policy readBooks --service-name=bookstore \
    --pdl-command "grant group employee read book"

# 禁止用户 "bob" 删除书籍
spctl create policy bobCannotDelete --service-name=bookstore \
    --pdl-command "deny user bob delete book"

# 使用正则表达式匹配资源（expr: 前缀）
spctl create policy podAccess --service-name=k8s \
    --pdl-command "grant group Administrators list,watch,get expr:c1/default/core/pods/*"

# 使用 AND 主体（两个角色必须同时匹配）
spctl create policy designerDBA --service-name=myapp \
    --pdl-command "grant role (designer, dba) update db_design_doc"
```

### 角色策略语法

```
ROLE_POLICY = EFFECT SUBJECT ROLE [on RESOURCE] [if CONDITION]
```

```sh
# 将 "reader" 角色授予 "intern" 组，作用于 "book" 资源
spctl create rolepolicy readerRole --service-name=bookstore \
    --pdl-command "grant group intern reader on book"

# 授予角色，不限定资源
spctl create rolepolicy managerRole --service-name=bookstore \
    --pdl-command "grant user alan manager"
```

### 条件表达式

条件是运行时评估的布尔表达式。支持的数据类型：`string`、`numeric`、`bool`、`datetime`、`array`。

**内置属性：**

| 属性 | 类型 | 描述 |
|------|------|------|
| `request_user` | string | 请求访问的用户 |
| `request_groups` | []string | 请求用户所属的组 |
| `request_entity` | string | 发起请求的服务/实体 |
| `request_resource` | string | 被访问的资源 |
| `request_action` | string | 被执行的操作 |
| `request_time` | datetime | 请求时间戳 |
| `request_year` | int | 请求年份 |
| `request_month` | int | 请求月份 (1–12) |
| `request_day` | int | 请求日期 (1–31) |
| `request_hour` | int | 请求小时 (0–23) |
| `request_weekday` | string | 星期几 ("Sunday"–"Saturday") |

**内置函数：** `Sqrt`、`Max`、`Min`、`Sum`、`Avg`、`IsSubSet`

**条件示例：**

```sh
# 基于时间的访问控制
spctl create policy weekdayAccess --service-name=bookstore \
    --pdl-command "grant user alice read book if request_weekday != \"Saturday\" && request_weekday != \"Sunday\""

# 正则表达式匹配
spctl create policy regexAccess --service-name=bookstore \
    --pdl-command "grant user bob read book if request_resource =~ \"/books/.*\""

# 使用内置函数
spctl create policy subsetCheck --service-name=bookstore \
    --pdl-command "grant user alice read book if IsSubSet(request_groups, ('managers', 'admins'))"

# 组合条件
spctl create policy complexCondition --service-name=bookstore \
    --pdl-command "grant user alice read book if request_year==2024 && request_month==12"
```

### 自定义属性

在授权请求中传递自定义属性：

```json
{
  "subject": {"principals": [{"type": "user", "name": "alice"}]},
  "serviceName": "bookstore",
  "resource": "book",
  "action": "read",
  "attributes": {
    "clearance_level": {"type": "string", "value": "top_secret"}
  }
}
```

然后在条件中使用：`grant user alice read book if clearance_level == "top_secret"`

### "拒绝优先" 算法

当授权和拒绝策略同时匹配时，拒绝策略优先。

---

## 策略管理

### 服务

服务是授权策略和角色策略的顶层容器。每个服务定义独立的策略作用域。

```bash
# 创建服务
spctl create service myapp
spctl create service myapp --service-type=k8s

# 列出所有服务
spctl get service --all

# 获取特定服务
spctl get service myapp

# 删除服务
spctl delete service myapp
```

### 授权策略

定义谁可以对哪些资源执行哪些操作：

```bash
# 使用 PDL 创建策略
spctl create policy myPolicy \
    -c "grant user alan read,write book" \
    --service-name=myapp

# 从 JSON 文件创建策略
spctl create policy --json-file ./policy.json --service-name=myapp

# 列出服务中的策略
spctl get policy --service-name=myapp --all

# 按 ID 获取策略
spctl get policy <policy-id> --service-name=myapp

# 删除策略
spctl delete policy <policy-id> --service-name=myapp
```

**策略 JSON 结构：**

```json
{
  "id": "ao3olis24hrzchwjduea",
  "name": "myPolicy",
  "effect": "grant",
  "permissions": [{"resource": "book", "actions": ["read", "write"]}],
  "principals": [["user:alan"]],
  "condition": "clearance_level == \"top_secret\""
}
```

### 角色策略

将角色分配给主体：

```bash
# 创建角色策略
spctl create rolepolicy rp01 \
    -c "grant user alan manager on res1" \
    --service-name=myapp

# 列出角色策略
spctl get rolepolicy --service-name=myapp --all

# 删除角色策略
spctl delete rolepolicy <rolepolicy-id> --service-name=myapp
```

### 策略元素

| 元素 | 描述 |
|------|------|
| **效果 (Effect)** | `grant` 或 `deny`。DENY 优先于 GRANT |
| **主体 (Principal)** | 身份：`user`、`group`、`entity` 或 `role` |
| **AND 主体** | 用括号括起来的逗号分隔主体：`(designer, dba)` — 必须全部匹配 |
| **资源 (Resource)** | 受保护对象。支持正则表达式（`expr:` 前缀） |
| **操作 (Action)** | 对资源执行的操作（任意字符串） |
| **条件 (Condition)** | 控制策略何时生效的布尔表达式 |

---

## 授权决策

ADS 实时评估授权请求并返回 GRANT/DENY 决策。

### 决策原因码

| 原因 | 码值 | 含义 |
|------|------|------|
| GRANT_POLICY_FOUND | 0 | 匹配到了授权策略 |
| DENY_POLICY_FOUND | 1 | 匹配到了拒绝策略 |
| SERVICE_NOT_FOUND | 2 | 指定的服务不存在 |
| NO_APPLICABLE_POLICIES | 3 | 没有策略匹配请求 |
| ERROR_IN_EVALUATION | 4 | 评估过程发生错误 |
| DISCOVER_MODE | 5 | 发现模式（始终返回允许） |

### REST API

**is-allowed** — 检查主体是否可以对资源执行操作：

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/is-allowed \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "Alan"}]},
    "action": "download",
    "resource": "/books/HarryPotter",
    "serviceName": "onlineBookStore"
  }'
# 响应: {"allowed":true,"reason":0}
```

**all-granted-roles** — 获取授予主体的所有角色：

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/all-granted-roles \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "Alan"}]},
    "serviceName": "onlineBookStore"
  }'
# 响应: ["role1", "role2"]
```

**all-granted-permissions** — 获取授予主体的所有权限：

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/all-granted-permissions \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "Alan"}]},
    "serviceName": "onlineBookStore"
  }'
# 响应: [{"resource":"/books/HarryPotter","actions":["download","read"]}, ...]
```

### 评估模式

Speedle 支持两种部署模式：

- **作为服务** — ADS 作为独立服务器，通过 REST/gRPC 访问
- **嵌入式（进程内）** — ADS 作为 Go 库在应用程序内部运行

---

## 高级功能

### 策略发现

发现模式帮助你了解系统发起了哪些授权请求，并从中自动生成策略——无需手动编写。

**工作流程：**

1. 将 `is-allowed` 调用替换为 `discover` 调用（替换端点 URL）
2. 运行测试套件或访问受保护资源
3. 从发现的请求生成策略：
   ```bash
   spctl discover policy --service-name=YOUR_SERVICE_NAME > service.json
   ```
4. 导入生成的策略：
   ```bash
   spctl create service --json-file service.json
   ```
5. 从 `discover` 切换回 `is-allowed`

**发现命令：**

```bash
# 列出所有服务的请求详情
spctl discover request

# 列出特定服务的请求
spctl discover request --service-name="foo"

# 持续监听最新请求
spctl discover request --last --service-name="foo" -f

# 从发现的请求生成策略
spctl discover policy --service-name="foo"

# 重置/清理所有请求
spctl discover reset
spctl discover reset --service-name="foo"
```

### 策略诊断

当授权决策不符合预期时，使用诊断 API 查看具体评估了哪些策略以及原因。

将 URL 中的 `is-allowed` 替换为 `diagnose`（保持请求体不变）：

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/diagnose \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "user1"}]},
    "serviceName": "srv1",
    "action": "get",
    "resource": "/api/v1/example/res1"
  }'
```

响应包含：
- 已评估的策略及其 `status`（`takeEffect` 或 `ignored`）
- 请求时解析的内置属性
- 最终决策和原因

### 全局策略

在特殊的 `global` 服务中定义的策略会在**所有**服务中生效，避免重复配置。

```bash
# 创建全局服务
spctl create service global

# 创建全局角色策略（对所有服务生效）
spctl create rolepolicy -c "grant user Emma AdminRole" --service-name=global

# 创建普通服务，引用全局角色
spctl create service library
spctl create policy -c "grant role AdminRole borrow books" --service-name=library

# 全局角色策略现在在 "library" 服务范围内生效
```

> **注意：** 目前仅支持全局**角色**策略，不支持全局授权策略。

### 身份域

Speedle 支持来自多个身份域的用户身份。使用 `from` 关键字将策略限定到特定身份域。

```bash
# 限定到 github 身份域的策略
spctl create policy -c "grant user user1 from github read book" --service-name=booksvc

# 限定到 IDCS 特定租户的策略
spctl create policy -c "grant user user1 from IDCS.tenant01 read book" --service-name=booksvc

# 匹配任何身份域的策略（没有 from 子句）
spctl create policy -c "grant user user1 rent book" --service-name=booksvc
```

**评估规则：**
- 带身份域的策略 → 身份域必须严格匹配
- 不带身份域的策略 → 匹配来自任何域的主体

### 令牌断言

当授权请求携带身份令牌（JWT、OAuth 等）时，Speedle 可以调用可配置的 webhook 来验证令牌并提取用户身份 —— 你的服务无需处理令牌验证。

**在 config.json 中配置：**

```json
{
  "asserterWebhookConfig": {
    "endpoint": "http://localhost:8080/v1/assert",
    "clientCert": "",
    "clientKey": "",
    "caCert": ""
  }
}
```

断言服务必须实现 [令牌断言插件 API](https://github.com/teramoby/speedle-plus/tree/master/api/asserter)。

**带令牌的请求示例：**

```bash
curl -X POST -H "Content-Type: application/json" \
  http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -d '{
    "subject": {"token": "githubtoken", "tokenType": "github"},
    "serviceName": "booksvc",
    "resource": "book",
    "action": "read"
  }'
```

### 自定义函数

当内置函数不够用时，你可以编写自己的函数并在策略条件中使用。

1. **将函数实现为 REST 端点**，接受 POST 请求，JSON 格式 `{"params": [...]}` ，返回 `{"result": ..., "error": ""}`
2. **在 Speedle 中注册函数**：
   ```bash
   spctl create function isValid \
     --func-url=https://localhost:23456/func/isValid \
     --cachable=true \
     --cache-ttl=300
   ```
3. **在条件中使用**：
   ```bash
   spctl create policy -c "grant user Ally access library if isValid(attr1)" --service-name=service1
   ```

---

## 存储

Speedle 支持可插拔的策略存储。开箱即用的存储：

| 存储 | 适用场景 |
|------|---------|
| **File** | 本地开发和测试 |
| **etcd** | 生产环境、多节点部署 |
| **MongoDB** | 生产环境、文档型存储 |

### 文件存储

```bash
speedle-pms --store-type file --filestore-loc /path/to/policies.json
```

### etcd 存储

```bash
speedle-pms --store-type etcd --etcdstore-endpoint localhost:2379
```

启用 TLS 的 etcd：

```bash
speedle-pms --store-type etcd \
  --etcdstore-endpoint https://etcd-cluster:2379 \
  --etcdstore-tls-cert /path/to/etcd-client.crt \
  --etcdstore-tls-key /path/to/etcd-client.key \
  --etcdstore-tls-ca /path/to/etcd-ca.crt
```

### 实现自定义存储

1. 实现 `PolicyStoreManager` 接口（包括用于变更通知的 `Watch` 函数）
2. 实现 `StoreBuilder` 接口
3. 在 `init()` 函数中通过 `store.Register()` 注册
4. 在 `cmd/speedle-pms/stores.go` 和 `cmd/speedle-ads/stores.go` 中导入存储包

参见 [etcd 存储](pkg/store/etcd/) 作为参考实现。

> **注意：** 数据存储必须支持 `watch` 功能，以便 ADS 能接收实时策略更新。

---

## 安全与 TLS

### TLS 配置

Speedle 使用 TLS 保护 API 通信。TLS **默认启用**。

**服务器参数：**

| 参数 | 描述 | 默认值 |
|------|------|--------|
| `--insecure` | 禁用 TLS (`true` / `false`) | `false` |
| `--cert` | TLS 证书路径 | — |
| `--key` | TLS 私钥路径 | — |
| `--client-cert` | 客户端 CA 证书 | — |
| `--force-client-cert` | 要求双向 TLS | `false` |

**启用 TLS 启动（生产环境）：**

```bash
speedle-pms --store-type file \
  --insecure=false \
  --key=/etc/speedle/server.key \
  --cert=/etc/speedle/server.crt \
  --client-cert=/etc/speedle/client-ca.crt

speedle-ads --store-type file \
  --insecure=false \
  --force-client-cert=true \
  --key=/etc/speedle/server.key \
  --cert=/etc/speedle/server.crt \
  --client-cert=/etc/speedle/client-ca.crt
```

**使用 cfssl 生成自签名证书（测试用）：**

```bash
# 安装 cfssl
go get -u github.com/cloudflare/cfssl/cmd/cfssl
go get -u github.com/cloudflare/cfssl/cmd/cfssljson

# 生成 CA
echo '{"CN":"CA","key":{"algo":"rsa","size":2048}}' | cfssl gencert -initca - | cfssljson -bare ca -

# 生成服务器证书
echo '{"signing":{"default":{"expiry":"43800h","usages":["signing","key encipherment","server auth","client auth"]}}}' > ca-config.json
export ADDRESS=localhost,127.0.0.1
export NAME=server
echo '{"CN":"'$NAME'","hosts":[""],"key":{"algo":"rsa","size":2048}}' | cfssl gencert -config=ca-config.json -ca=ca.pem -ca-key=ca-key.pem -hostname="$ADDRESS" - | cfssljson -bare $NAME

# 生成客户端证书
export ADDRESS=
export NAME=client
echo '{"CN":"'$NAME'","hosts":[""],"key":{"algo":"rsa","size":2048}}' | cfssl gencert -config=ca-config.json -ca=ca.pem -ca-key=ca-key.pem -hostname="$ADDRESS" - | cfssljson -bare $NAME
```

**带 TLS 的 spctl 配置：**

```bash
spctl config skipverify false \
  cacert /path/to/server-ca.crt \
  cert /path/to/client.crt \
  key /path/to/client.key \
  pms-endpoint "https://localhost:6733/policy-mgmt/v1/"
```

**带 TLS 的 curl：**

```bash
curl --cacert /path/to/server-ca.crt \
  --cert /path/to/client.crt \
  --key /path/to/client.key \
  -X POST -H "Content-Type: application/json" \
  -d '{"subject":{...},"serviceName":"mysvc","resource":"res1","action":"get"}' \
  https://localhost:6734/authz-check/v1/is-allowed
```

### 管理端点的授权

PMS 管理端点**不内置认证**。生产环境建议：
- 将 PMS 绑定到本地：`--endpoint=127.0.0.1:6733`
- 使用带认证的反向代理（例如 API Gateway + Token）
- 通过防火墙限制网络访问

---

## 日志

Speedle 使用 [Logrus](https://github.com/sirupsen/logrus)（结构化日志）+ [Lumberjack](https://github.com/natefinch/lumberjack)（日志轮转）。

### 参数

| 参数 | 描述 | 默认值 |
|------|------|--------|
| `--log-level` | panic, fatal, error, warn, info, debug | `info` |
| `--log-formatter` | text, json | `text` |
| `--log-reportcaller` | 包含文件名/行号/函数名 | `false` |
| `--log-filename` | 写入文件（否则 stderr） | — |
| `--log-maxsize` | 轮转前最大 MB | `100` |
| `--log-maxbackups` | 保留的旧文件最大数量 | `0`（无限制） |
| `--log-maxage` | 保留的最大天数 | `0`（无限制） |
| `--log-compress` | 压缩轮转后的文件 | `false` |

### 配置文件

```json
{
  "logConfig": {
    "level": "info",
    "formatter": "json",
    "rotationConfig": {
      "filename": "/var/log/speedle/speedle.log",
      "maxSize": 10,
      "maxBackups": 5,
      "maxAge": 30
    }
  }
}
```

### 审计日志

使用 `--auditlog-*` 参数进行独立的审计日志配置（选项同日志参数，前缀为 `auditlog-`）。

### Docker / 容器日志

在 Docker 中运行时，建议使用 Docker 的 `json-file` 日志驱动代替 Lumberjack：

```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "20m", "max-file": "10" }
}
```

---

## 部署

### 作为 Go 库（嵌入式模式）

将 Speedle 导入你的 Go 应用，实现进程内授权：

```go
import (
    "github.com/teramoby/speedle-plus/api/pms"
    "github.com/teramoby/speedle-plus/api/ads"
)
```

### 在 Kubernetes 上

使用 Helm 或手动部署。两种模式：

- **开发模式** — 基于文件存储，单 Deployment
- **生产模式** — etcd 后端，PMS/ADS 独立部署（各 3 副本）

**开发模式 (Helm)：**

```bash
helm install -n speedle-dev deployment/helm/speedle-dev
kubectl port-forward svc/speedle-dev 6733:6733 6734:6734
```

**生产模式 (Helm)：**

```bash
# 安装 etcd operator
helm install stable/etcd-operator --name my-release
kubectl create -f example-etcd-cluster.yaml

# 部署 Speedle
helm install -n speedle deployment/helm/speedle-prod \
  --set store.etcd.endpoint=http://example-etcd-cluster-client:2379
```

手动部署 YAML 文件：[`deployment/k8s/`](deployment/k8s/)

---

## 集成

### Kubernetes 授权 Webhook

Speedle 可以作为 Kubernetes [授权 webhook](https://kubernetes.io/docs/reference/access-authn-authz/webhook/)，对 Kubernetes API 请求进行策略评估。

参见 [`samples/integration/kubernetes-integration`](samples/integration/kubernetes-integration/) 了解 webhook 实现。

**保护 Pod 的策略示例：**

```bash
spctl create service kubernetes

# 允许用户 "joe" 获取带有 component=etcd 标签的 pod
spctl create policy joe-pod --service-name=kubernetes \
  -c "grant user joe get expr:/pods/.* if labels_component == \"etcd\""
```

### Istio Mixer 适配器

将 Speedle 与 Istio 的策略执行层集成。参见 [Istio 集成文档](https://speedle.io/integrations/istio/)。

### Docker 授权插件

使用 Speedle 作为 Docker 授权插件控制容器操作。参见 [Docker 集成文档](https://speedle.io/integrations/docker/)。

---

## API 参考

Speedle 提供三种 API 接口：

### REST API

| API | 端点 | Swagger |
|-----|------|---------|
| 策略管理 | `http://host:6733/policy-mgmt/v1/` | [`swagger/policy-manage.yaml`](swagger/policy-manage.yaml) |
| 授权决策 | `http://host:6734/authz-check/v1/` | [`swagger/policy-check.yaml`](swagger/policy-check.yaml) |
| 令牌断言 | (用户自定义) | [`swagger/asserter.yaml`](swagger/asserter.yaml) |

### gRPC API

| API | Proto |
|-----|-------|
| 策略管理 | [`protobuf/pms.proto`](protobuf/pms.proto) |
| 授权决策 | [`protobuf/ads.proto`](protobuf/ads.proto) |

### Go API（嵌入式）

| API | 包 |
|-----|-----|
| 策略管理 | [`api/pms`](api/pms/) |
| 授权决策 | [`api/ads`](api/ads/) |

---

## 配置文件

Speedle 使用 JSON 配置文件：

```json
{
    "storeConfig": {
        "storeType": "file",
        "storeProps": {
            "FileLocation": "/etc/speedle/policies.json"
        }
    },
    "enableWatch": true,
    "serverConfig": {
        "endpoint": "0.0.0.0:6733",
        "insecure": "true"
    },
    "asserterWebhookConfig": {
        "endpoint": "http://localhost:8080/v1/assert",
        "clientCert": "",
        "clientKey": "",
        "caCert": ""
    },
    "logConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/speedle.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 30
        }
    },
    "auditLogConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/audit.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 90
        }
    }
}
```

配置值可以从三个来源获取（按优先级排序）：
1. **命令行参数**（最高）
2. **环境变量**（前缀 `SPDL_`，连字符 → 下划线，大写）
3. **配置文件**（最低）

---

## 构建

### 前期准备

- Go 1.22 或更高版本: <https://go.dev/doc/install>

### 从源码构建

```bash
git clone https://github.com/teramoby/speedle-plus.git
cd speedle-plus
make build
```

### 通过 go install

```bash
go install github.com/teramoby/speedle-plus/cmd/spctl@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
```

## 测试

```bash
cd speedle-plus
make test
```

---

## 应用场景

### 应用级策略

开发者在构建 Go 应用程序时可以嵌入 Speedle 进行授权，无需重复造轮子。

### 集中化策略执行

管理数十个内部系统的安全团队可以使用 Speedle 作为统一的授权引擎：
- 集中化策略管理
- 一个灵活的适用于所有系统的策略模型
- 易于与操作系统、中间件和应用程序集成

### 跨技术栈的统一授权

提供 Linux、Docker、Kubernetes 和 Istio 的云平台可以在 Speedle 中定义统一的授权模型，并在所有层面一致执行。

---

## 社区与贡献

### 获取帮助

- Slack：加入 [#speedle-users](https://join.slack.com/t/speedleproject/shared_invite/zt-72fgiyuo-QKJAhHAqVbn17KRFbd7aZw)
- 邮件列表：speedle-users@googlegroups.com
- QQ 群：643201591

### 参与贡献

1. 在 [GitHub](https://github.com/teramoby/speedle-plus) 为项目点颗星
2. 告诉我们你如何在项目中使用 Speedle+
3. 在 [Issues](https://github.com/teramoby/speedle-plus/issues) 提交 Bug 或功能请求
4. 贡献代码 — 参见 [CONTRIBUTING.md](CONTRIBUTING.md)

### 行为准则

遵守[黄金法则](https://en.wikipedia.org/wiki/Golden_Rule)。参见[贡献者公约](https://www.contributor-covenant.org/version/1/4/code-of-conduct.html)。

---

## 文档

- 最新文档: <https://speedle.io/docs>
- Go 包参考: <https://pkg.go.dev/github.com/teramoby/speedle-plus>

更详细的内容可以查阅[这里](https://github.com/teramoby/speedle-plus/tree/master/docs/%E4%B8%AD%E6%96%87%E8%B5%84%E6%96%99)。
