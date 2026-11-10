+++
title = "Speedle Cloud 限额与错误"
linkTitle = "限额与错误"
description = "Speedle Cloud 账号额度、请求限制、扁平 JSON 错误、限流与额度响应头、每日额度计算和重试建议。"
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T17:11:18+08:00
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "合理规划账号级限额，并在 Cloud 无法返回有效授权判定时默认拒绝。"
+++

以下 ADS 错误与响应头描述带认证的 ADS 合约，其运行时尚未提供。管理合约已经定义账号限额、会话与 CSRF 检查及令牌管理错误，供账号访问开放后使用。下载规范本身不会启用尚未提供的接口。

## 限额

| 限额 | 含义 |
| --- | --- |
| 每日 10,000 次 | 同一账号每 UTC 日进入已匹配 PMS 和 ADS handler 的合计次数，由所有会话、令牌和 service 共用；00:00 UTC 重置。 |
| 200 条策略 | 每个账号的授权策略和角色策略合计。 |
| 每条 SPDL 4 KiB | 完整策略最多 4,096 个 UTF-8 字节，不是 4,096 个字符。 |
| 2 个有效令牌 | 每个 service；已撤销的历史记录不占此名额。 |
| 每令牌每分钟 600 次 | 每个节点的近似突发限制，与严格的账号每日总额分开。 |
| 请求正文 64 KiB | 解析前的全局上限。可选输入 attributes 仍受 50 个属性、规范化 UTF-8 JSON 16,384 字节的限制。 |

每个账号只有一个 service。令牌名称为 1–128 个字符。凭据生命周期规则见[应用令牌](/zh/docs/cloud/tokens/)。

## 每日额度计算

进入已匹配的 handler 即扣除一次每日额度，包括策略拒绝、无效业务输入和内部错误。认证与会话接口不消耗此额度。在进入 handler 前被认证、路由、CSRF、正文大小、突发限流或额度预留检查拒绝的请求，也不消耗每日额度。

已过期或被放弃的预留 lease 不会重新发放，因此实际可用次数可能少于 10,000。`GET /api/me` 返回当天已预留用量和重置时间，本身不扣每日额度。

更换令牌不会增加账号额度。严格的账号每日总额与近似的节点级令牌突发限额分别执行。

## HTTP 错误

错误使用扁平 JSON 响应。请按说明修正请求或凭据；其他情况下应默认拒绝，并采用有界退避，不要无限重试。

| HTTP 状态与错误 | 应用处理方式 |
| --- | --- |
| `401 {"error":"unauthorized"}` | 检查环境和凭据。ADS 带 Bearer challenge。匿名请求或只带会话 cookie 的请求不能授权 ADS。 |
| `403 {"error":"reauth_required"}` | 创建令牌前，使用已关联供应商重新认证。普通操作不会刷新认证时间。 |
| `403 {"error":"service_mismatch"}` | 修正 service 输入，不得改用其他租户的数据重试。 |
| `400 {"error":"SPDL-0004 …"}` | Speedle 拒绝了请求，例如属性值类型不符。error 是 Speedle 的原始消息。修正请求，不要原样重试。 |
| 令牌撤销返回 `404` | 令牌 ID 未知、不属于自己或已经撤销。先刷新元数据，再决定下一步操作。 |
| `413 {"error":"request_too_large"}` | 缩小请求正文。 |
| `429 {"error":"rate_limited"}` | 遵守 `Retry-After`，此突发限制为 60 秒，并使用有界退避。 |
| `429 {"error":"daily_quota_exceeded"}` | 按 `Retry-After` 等待至下一个 UTC 零点。更换令牌不会增加账号额度。 |
| `503 {"error":"quota_unavailable"}` 或 `snapshot_unavailable` | 默认拒绝，仅在有界退避后重试，不得回退到 demo 或 D1 策略判定。 |
| `500 {"error":"internal_error"}` | 默认拒绝。保留安全的 request ID 供排障，避免记录凭据。 |

对于 `503` 响应，`snapshot_unavailable` 在进入 handler 后已经扣额，`quota_unavailable` 未扣额。提前拒绝的响应不带下文所述的每日剩余额度响应头。

非 POST 的 ADS 请求也可能被公开边缘规则拦截并返回 Cloudflare 的 403，该响应不是应用授权判定。

## 限流和额度响应头

完成验证后，`X-RateLimit-Limit` 为 600。ADS 不提供精确的 `X-RateLimit-Remaining` 值。

完成扣额的 handler 响应若带 `X-Quota-Daily-Remaining`，它估计账号今天剩余的调用次数：授予 lease 时的未预留余额，加上本 isolate 尚余的 lease 次数。该估计在授予时偏保守，随后最多缓存至有效的 30 秒 lease 结束，到期或 UTC 换日即作废。

其他 isolate 的并发调用可能使该值高估可用次数，最多高估授予时的全部未预留余额，并非仅 32 次；未使用或被放弃的预留则可能使其低估。发布或回滚期间，旧分配器会保守回退为本地 lease 余量。此响应头仅供参考，不改变额度执行方式。提前拒绝的响应不带此头。

已匹配的应用 ADS 响应还使用 `Cache-Control: no-store`，不要将判定结果或凭据放入共享缓存。

请求示例和接入限制见[授权 API 预览](/zh/docs/cloud/authorization-api/)。
