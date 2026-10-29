+++
title = "策略管理控制台"
description = "在浏览器中管理策略、分析角色关系、诊断授权请求"
weight = 65
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["console", "guide", "pms", "ads"]
categories = ["docs"]
bref = "speedle-console 是一个 Web 控制台，用于管理授权策略和角色策略、分析角色关系、诊断授权请求。用户通过令牌断言服务登录，能做什么由 Speedle 策略决定。"
+++

## 控制台能做什么

`speedle-console` 是一个独立的服务，提供网页，并把页面发出的请求转发给 PMS 和 ADS。功能包括：

- **服务、授权策略和角色策略**：列表、筛选、新建、修改、删除。修改时用 `PUT` 并带上对象的 `ETag`，如果期间有人改过同一项，会提示而不是直接覆盖。
- **角色关系分析**：用关系图展示谁直接或经由其他角色获得了哪些角色，可以包含 `global` 服务的角色策略。会标出循环、被引用却没有任何角色策略授予的角色，以及被授予却从未使用的角色。选择一个角色，可以查看谁持有它、经由哪些角色、是否只在满足条件或资源范围时才持有。
- **诊断**：向 ADS 发送一个请求，查看结果、原因、获得的角色，以及每条生效的角色策略和授权策略及其条件结果。

页面支持中文和英文。

## PMS 和 ADS 必须隔离

PMS 没有认证，并且直接根据调用方发送的 `Speedle-Principals` 请求头记录修改人。控制台会验证用户身份并检查权限，但任何能直接访问 PMS 的人都可以修改任意策略。**只对外开放控制台，PMS 和 ADS 只允许控制台和你的应用访问**：使用网络策略，或启用[双向 TLS](/docs/security/) 并设置 `forceClientCert`，给控制台配置客户端证书。

## 在本机试用

按[快速入门](/quick-start/)启动 PMS 和 ADS，然后以开发模式构建并运行控制台。开发模式无需登录，所有人都是管理员，因此只能监听回环地址。

```bash
go build -o ./bin/speedle-console ./cmd/speedle-console
./bin/speedle-console --dev-no-auth --insecure --endpoint 127.0.0.1:6735 --pms-url http://127.0.0.1:6733 --ads-url http://127.0.0.1:6734
```

打开 `http://127.0.0.1:6735/console/`。

## 登录与权限

用户使用令牌登录。控制台把令牌发给[令牌断言服务](/docs/assertor/)（与 ADS 使用的是同一个 webhook），并把返回的主体保存在签名的 `HttpOnly`、`SameSite=Strict` cookie 中，令牌本身不会保存。每次修改还需要会话专属的 CSRF 令牌，页面会自动带上。

用户能做什么由 Speedle 自己决定。每次 API 调用时，控制台都会询问 ADS：用户的主体能否在服务 `speedle-console`（可用 `--authz-service` 修改）中对资源 `console` 执行某个操作：

| 操作 | 允许 |
|---|---|
| `read` | 查看服务和策略、角色关系分析、诊断 |
| `write` | 新建、修改、删除服务、授权策略和角色策略 |

如果 ADS 无法应答，控制台会拒绝请求（HTTP 503）。判定结果会缓存 `--authz-cache-ttl`（默认 5 秒）。

## 写入第一批控制台策略

新安装时还没有人拥有 `write` 权限。先用 `--bootstrap-admins user:alice`（以逗号分隔的 `类型:名称` 主体列表）启动一次控制台，这些主体无需询问 ADS 即拥有全部权限，控制台会给出警告。然后在控制台中或用 spctl 写入控制台策略：

```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service speedle-console
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy console-admins -c "grant user alice console-admin" --service-name speedle-console
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy console-admin -c "grant role console-admin read,write console" --service-name speedle-console
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy console-readers -c "grant group auditors read console" --service-name speedle-console
```

去掉 `--bootstrap-admins` 后重启控制台。如果这些策略写错导致所有人都无法登录，再带上 `--bootstrap-admins` 重启一次进行修复。有人编辑 `speedle-console` 服务时，控制台会先给出警告。

## 参数

每个参数都可以用环境变量设置：`--pms-url` 对应 `SPEEDLE_CONSOLE_PMS_URL`。命令行参数优先。

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--endpoint` | `0.0.0.0:6735` | 控制台监听的地址 |
| `--insecure` | `false` | 使用 HTTP 而不是 HTTPS |
| `--cert`、`--key` | | 控制台的 TLS 证书和私钥 |
| `--secure-cookie` | `false` | 在终止 TLS 的代理之后以 HTTP 提供服务时，仍把会话 cookie 标记为 `Secure` |
| `--pms-url`、`--ads-url` | | PMS 和 ADS 的地址，例如 `https://pms:6733` |
| `--upstream-ca` | | 验证 PMS 和 ADS 证书的 CA |
| `--upstream-cert`、`--upstream-key` | | 访问 PMS 和 ADS 的客户端证书 |
| `--upstream-timeout` | `30s` | 调用 PMS 和 ADS 的超时时间 |
| `--asserter-endpoint` | | 令牌断言 webhook；除非使用 `--dev-no-auth`，否则必填 |
| `--asserter-ca`、`--asserter-cert`、`--asserter-key` | | 访问令牌断言服务的 TLS 设置 |
| `--session-key-file` | | 签名会话 cookie 的密钥文件，至少 32 字节。不提供时使用随机密钥：控制台重启后会话失效，并且只能部署一个副本 |
| `--session-ttl` | `8h` | 一次登录的有效期 |
| `--authz-service` | `speedle-console` | 存放控制台策略的服务 |
| `--authz-cache-ttl` | `5s` | 权限判定结果的缓存时间 |
| `--bootstrap-admins` | | 无需策略即拥有全部权限的主体 |
| `--dev-no-auth` | `false` | 无需登录、所有人都是管理员；只能监听回环地址 |

## 用 Helm 或 Docker 部署

`make buildConsole imageConsole` 会构建二进制文件和 `speedle-console` 镜像（端口 6735）。`deployment/helm` 下的两个 Helm chart 都有 `console` 配置段，默认关闭：

- `speedle-dev` 把控制台与 PMS、ADS 放在同一个 pod 中。设置 `console.enabled=true` 和 `console.asserterEndpoint` 即可。
- `speedle-prod` 把控制台部署为独立的 deployment 和 service。另外支持 `console.sessionKeySecret`（包含 `session.key` 的 secret，多副本时必需）、控制台自身证书 `console.tls`，以及访问 PMS 和 ADS 用的 CA 与客户端证书 `console.upstreamTLS`。

## 常见问题

| 现象 | 原因 |
|---|---|
| 登录提示“令牌未通过验证” | 令牌断言服务拒绝了该令牌，或没有返回任何主体 |
| “你已登录，但没有任何策略允许你读取控制台” | `speedle-console` 中没有策略把 `console` 上的 `read` 授予该用户的主体 |
| 503“授权服务不可用” | 控制台无法访问 ADS；在 ADS 恢复前会拒绝所有请求 |
| 502“无法连接策略服务” | 控制台无法访问 PMS |
| “打开后已有人修改了此项” | 加载之后该项被修改过；请重新加载后再修改 |
