+++
title = "Speedle Cloud 限额与错误"
linkTitle = "限额与错误"
description = "账号限额、错误代码、重置时间和重试建议。"
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

除表中另有说明外，限额由整个账号共用。API 密钥属于账号；更换密钥不会增加每日额度。

## 限额

| 限制对象 | 上限 | 达到后如何处理 |
| --- | --- | --- |
| 每个账号的服务 | 100 个 | `409 quota_exceeded`；删除不用的服务。不会定时重置。 |
| 所有服务的授权策略与角色策略合计 | 每账号 200 条 | `409 quota_exceeded`；删除策略。不会定时重置。 |
| 单条策略的完整 SPDL 源文本 | 1,024 个 UTF-8 字节 | 控制台策略写入返回 `400 invalid_request`；缩短源文本。按字节计数，不是按字符计数。不会定时重置。 |
| 所有权限范围的有效 API 密钥合计 | 每账号 10 个 | `409 quota_exceeded`，带 `limit: 10`；撤销不用的密钥，或等它到期。名额不会按固定周期重置。已撤销和已到期的记录不占名额。 |
| 策略名称 / API 密钥名称 | 128 个 Unicode 码点 / 去除首尾空白后 1–128 个字符 | `400 invalid_request`；缩短或修正名称。不会定时重置。 |
| 策略管理与授权调用的计费次数 | 每账号每 UTC 日 10,000 次 | `429 daily_quota_exceeded`；00:00 UTC 重置。遵守 `Retry-After`。 |
| API 密钥请求的短时突发 | 每个 Cloud 位置、每个密钥每分钟约 600 次 | `429 rate_limited`，带 `Retry-After: 60`；等待后重试。这是短时限制，不保证吞吐量，也不是额外的每日额度。 |
| 普通请求正文 | 65,536 字节（64 KiB） | `413 request_too_large`；缩小正文。不会定时重置。 |
| 原子 JSON 导入正文（仅 `POST /api/service/import`） | 5,100,000 字节 | `413 request_too_large`；缩小导入内容。仍须满足单条策略和账号限额。不会定时重置。 |
| 输入属性 | 50 个属性；规范化 JSON 的 UTF-8 长度为 16,384 字节 | `400 SPDL-0004`；减少属性或缩小属性值。不会定时重置。 |

权限范围、有效期、创建和撤销规则见 [API 密钥](/zh/docs/cloud/tokens/)。

## 每日额度

读取、写入、校验、Playground 测试、导入、导出和授权检查共用每日额度。策略拒绝或业务输入无效仍计费。认证、会话操作和 `GET /api/me` 不计费。被认证、权限范围、路由、CSRF、正文大小、突发限流或每日额度检查拒绝的请求不计费。

Cloud 自身故障可能退还额度，每账号每 UTC 日最多退还 1,000 次；无法确认退还时仍计费。请求本身导致的错误仍计费。额度计数暂时不可用时，授权调用可能在降速后继续执行，每账号每 UTC 日额外放行最多 10,000 次。这份额度与正常额度独立，不保证一定可用，和退还次数一样在 00:00 UTC 重置。无法放行时返回 `503 quota_unavailable`。

部分每日额度可能已预留但未使用，因此实际可用次数可能少于 10,000。`GET /api/me` 返回用量和重置时间。不要持续重试或通过更换密钥绕过账号限额。

## 错误与响应头

错误使用扁平 JSON，例如 `{"error":"rate_limited"}`。错误不是授权通过；无法取得有效判定时应默认拒绝。

| 状态与错误 | 处理方式 |
| --- | --- |
| `400 invalid_request` 或 Speedle 错误消息 | 修正请求，不要原样重试。 |
| `401 unauthorized` | 检查 API 密钥或登录会话。仅有会话 cookie 不能调用 ADS。 |
| `401 api_key_expired` | API 密钥已到或已过到期时间。请创建新密钥；重试该密钥不会成功。在必须设置有效期之前创建的密钥同样有到期时间。到期规则见 API 密钥页；请查看 `expiresAt`，并在到期前创建新密钥。 |
| `403 reauth_required` | 创建密钥前，通过已关联的身份提供方重新认证。 |
| `403 insufficient_scope` | 使用具有所需权限范围的密钥。 |
| `403 management_scope_disabled` | 登录关闭时不能签发管理权限的密钥。 |
| `403 csrf_failed` | 使用控制台会话及所需的来源和 CSRF 请求头。 |
| `404 service_not_found`（ADS）/ `404 not_found`（管理） | 选择当前账号中存在的服务。密钥 ID 未知、属于其他账号或已撤销时也返回 `404 not_found`。 |
| `409 quota_exceeded` | 删除不用的服务或策略以释放名额；等待不会重置这两类上限。撤销不用的 API 密钥，或等它到期，可释放密钥名额。 |
| `413 request_too_large` | 缩小正文。 |
| `429 rate_limited` / `429 daily_quota_exceeded` | 遵守 `Retry-After` 并使用有界退避；每日额度在 UTC 午夜重置。 |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | 默认拒绝，按有界退避重试；带有 `Retry-After` 时遵守它。 |

API 密钥验证通过后，`X-RateLimit-Limit` 为 600，不提供精确的 `X-RateLimit-Remaining`。如果有 `X-Quota-Daily-Remaining`，它只是账号级剩余额度的估计值；并发调用和未使用的预留可能使它偏离实际可用次数。它不保证下一次请求成功，提前拒绝的响应不带此头。

ADS 响应使用 `Cache-Control: no-store`，不要共享缓存判定或凭据。请使用 POST；其他方法可能收到 `405 method_not_allowed`，或更早被拦截并返回非 JSON 响应。参见[授权 API](/zh/docs/cloud/authorization-api/)。
