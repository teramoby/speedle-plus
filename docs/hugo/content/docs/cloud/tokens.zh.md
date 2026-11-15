+++
title = "Speedle Cloud API 密钥"
linkTitle = "API 密钥"
description = "创建、保护、列出和撤销账号级 API 密钥，选择固定权限范围和有效期。"
weight = 20
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

API 密钥属于账号，按权限范围访问该账号下的服务。它认证的是应用，不是最终用户。管理 API 密钥、会话和关联身份始终需要已登录的控制台会话。

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

## 创建 API 密钥

打开 [API 密钥](https://cloud.speedle.io/zh-CN/tokens)，输入去除首尾空白后为 1–128 个字符的名称，选择权限范围和有效期（`7d`、`30d`、`90d`、`180d` 或 `365d`；控制台默认 3 个月），点击 **创建**。创建需要在最近 10 分钟内通过已关联的身份提供方认证。收到 `403 reauth_required` 时，重新认证后再次点击 **创建**；从身份提供方返回不会自动创建密钥。

每个密钥只有一个权限范围，创建后不可修改：

| 权限范围 | 可访问的功能 |
| --- | --- |
| `ads`（默认） | 仅通过 `/authz-check/v1/*` 检查授权。 |
| `pms:read` | 通过 `/policy-mgmt/v1` 读取账号下的服务、授权策略和角色策略。 |
| `pms:write` | 通过 `/policy-mgmt/v1` 读取、创建和删除账号下的服务、授权策略和角色策略。 |

管理权限的密钥不能调用 ADS；`ads` 不能管理策略。权限范围不符时返回 `403 insufficient_scope`。登录尚未开放时，申请管理权限返回 `403 management_scope_disabled`。

控制台使用已登录会话发送请求：

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app","scope":"ads","expiresIn":"90d"}
```

请求接受 `name`、必填的 `expiresIn` 和可选的 `scope`（默认 `ads`）。`expiresIn` 只能是 `7d`、`30d`、`90d`、`180d` 或 `365d`，对应 1 周、1 个月、3 个月、6 个月或 1 年。一天按 24 小时计算。控制台的 **有效期** 默认是 3 个月，提交前显示 **预计到期时间（UTC）**。没有永不过期的密钥。不要发送服务选择、`expiresAt` 或认证证明。缺少或无法识别的 `expiresIn`，以及客户端自行提供的 `expiresAt`，都会返回 `400 invalid_request`。只包含 `name` 和 `scope` 的请求也会返回该错误。服务端不会代填 `expiresIn`。请求需要会话 cookie 和 CSRF 请求头；API 密钥不能代替会话。

## 保护密钥值

创建成功返回 HTTP 201。响应的 `token` 字段包含 API 密钥值，只显示一次：

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

每个密钥都会到期。响应包含服务端计算的 `expiresAt`（Unix 毫秒）。到达该时刻或之后，授权检查和策略管理调用返回 `401 {"error":"api_key_expired"}`。请创建新密钥；再次发送已到期的密钥不会成功。不再使用时可以提前撤销。在必须设置有效期之前创建的密钥现在也有到期时间。如果创建后 365 个 24 小时仍在未来，密钥就在那时到期，即使距离该要求生效只有很短的时间。如果这一年已经过去，密钥在该要求生效后再过 30 个 24 小时到期。列表中的每个密钥都包含 `expiresAt`。请在到期前查看并创建新密钥。

这只是格式说明，不能作为实际密钥使用。请立即保存到服务端的密钥存储中。不要放入浏览器存储、前端代码、URL、源码、截图或日志。列表只返回元数据，丢失的密钥值无法找回。

每个账号所有权限范围合计最多 **10 个有效 API 密钥**。继续创建会返回 `409 {"error":"quota_exceeded","limit":10}`。撤销不再使用的密钥，或等它到期，可释放名额。名额不会按固定周期重置。已撤销和已到期的密钥仍保留在列表中，不占有效名额。

## 列出 API 密钥

`GET /api/tokens` 按最新在前列出有效、已到期和已撤销的密钥。默认每页 25 条，`limit` 可设为 1–100。将上次响应的 `nextCursor` 作为 `cursor` 继续请求，直到 `nextCursor` 为 `null`。每条记录都包含 Unix 毫秒 `expiresAt`。控制台显示 **到期时间（UTC）**。7 天内到期的密钥标为 **即将到期**。已到期的密钥列在 **已到期** 分组中。`lastUsedAt` 尽力更新，通常每小时最多一次，不能据此断定密钥已无人使用。

## 撤销 API 密钥

选择 **撤销**，核对密钥名称和 ID 后确认。撤销无需最近重新认证。控制台发送：

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

首次撤销返回空的 HTTP 204。ID 未知、不属于当前账号或已撤销时返回 `404 not_found`。先前已通过验证的请求仍可能在最长 60 秒内使用该密钥，因此撤销不会在所有位置立即生效。

可用操作是 **创建** 和 **撤销**。如需更换密钥值、权限范围或有效期，请创建另一个密钥，更新调用方后撤销旧密钥。达到上限且所有密钥仍在使用时，撤销前需安排调用中断。删除服务不会撤销账号的 API 密钥。

创建或撤销的响应丢失时，先刷新列表再操作。同名记录不能找回丢失的密钥值。参见[限额与错误](/zh/docs/cloud/limits-and-errors/)和[授权 API](/zh/docs/cloud/authorization-api/)。
