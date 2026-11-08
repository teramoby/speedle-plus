+++
title = "Speedle Cloud 授权 API 预览"
linkTitle = "授权 API"
description = "预览 Speedle Cloud ADS 的 IsAllowed 请求与响应、Node.js 22 接入示例、策略快照和传输限制。"
date = 2026-11-08T00:00:00+08:00
lastmod = 2026-11-08T00:00:00+08:00
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "后续带认证版本的服务端 ADS 已发布合约。"
+++

**带认证的 ADS 尚未提供。** 本页的请求和示例描述后续版本的已发布合约，并未在 Cloud 线上环境执行。签发应用令牌并不会启用这些调用。请明确使用获准环境的 URL，并在接入前确认该环境已经提供带认证的 ADS。

## 从应用服务端调用 IsAllowed

将签发的令牌放入一个 `Authorization: Bearer` 请求头，并发送 POST 请求：

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

请求正文就是 Speedle REST 请求，字段和类型与自托管 Speedle ADS 相同。`attributes` 是由 `{name, type, value}` 组成的数组。属性类型支持 `string`、`numeric`、`bool` 和采用 RFC 3339 格式的 `datetime`。

`idd` 是主体所属的身份域，也就是应用在哪个命名空间里完成对调用方的认证，例如某个身份提供方或某个租户。它必须由服务端根据可信的认证结果填写，不能取自浏览器提交的值。

快速入门中的策略 `grant user alice from corp get /books` 只在 `corp` 身份域中匹配上述请求。不带 `from` 的策略匹配任意身份域中的同名主体。主体不带 `idd` 时同样匹配任意身份域，一旦应用接受多个身份来源，两个同名用户就无法区分。多租户接入必须在每个请求中发送 `idd`。

每个请求都必须填写 `serviceName`，系统不会自动取默认值。请填写令牌所属 service 的名称。名称缺失或为空时，会得到 Speedle 的 `SERVICE_NOT_FOUND` 响应；名称不一致时，返回 `403 {"error":"service_mismatch"}`。

[快速入门中的策略](/zh/docs/cloud/getting-started/#创建-service-和策略)会允许上述示例请求：

```json
{"allowed":true,"reason":0}
```

## 理解判定结果和调用方身份

策略拒绝同样返回 HTTP 200，但 `allowed` 为 `false`。应用必须检查 `allowed`，再执行受保护的操作；遇到异常或无效响应时应默认拒绝。

应用通过自身的用户认证确定调用方的 principals。API 令牌识别的是应用和 service，不是最终用户身份。不要相信匿名浏览器自行提交的 principal 名称。

Speedle Cloud 不校验 subject token。与没有 token asserter 的自托管 Speedle 一样，它接受 `subject.token` 和 `tokenType`，但不做校验就丢弃，因此非空的 token 字段永远不能作为已经通过认证的证据，判定只使用 principals。不要把最终用户的令牌转发过来，并把它当成已完成断言。

会话 Playground 使用当前 D1 策略数据，带认证的 ADS 使用发布后的快照，传播约需 60 秒。Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。ADS 不提供公开 Diagnose 接口。

## Node.js 22 示例

Node.js 22 无需 SDK 即可调用 ADS：

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiToken = process.env.SPEEDLE_ADS_TOKEN;
  if (!apiToken) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      subject: {
        principals: [
          { type: "user", name: authenticatedUserName, idd: authenticatedUserIdd },
        ],
      },
      serviceName: "books",
      resource: "/books",
      action: "get",
    }),
  });
  if (!response.ok) throw new Error(`Speedle HTTP ${response.status}`);
  const result = await response.json();
  if (
    typeof result !== "object" || result === null || Array.isArray(result) ||
    typeof result.allowed !== "boolean" || !Number.isInteger(result.reason) ||
    result.reason < 0 || result.reason > 6
  ) {
    throw new Error("Invalid Speedle decision");
  }
  return result.allowed;
}
```

通过服务端的秘密存储注入 `SPEEDLE_ADS_TOKEN`，切勿将值写入代码。传入的主体名称和身份域必须由服务端根据已认证用户得出。示例在组装请求正文之前就拒绝空的主体名称或身份域，否则 `JSON.stringify` 会丢掉 `idd`，发出不带身份域的主体。只有在 service 仅有一个身份来源、名称不可能跨域重复时，才可以省略 `idd`。调用方遇到异常时必须拒绝执行受保护操作，不能把失败转换为允许。

这个最小示例没有重试循环。生产应用应区分文档列出的错误，遵守 `Retry-After`，并使用有界重试。示例中的 `redirect: "error"` 防止 fetch 携带凭据跟随重定向；改写示例时应保留这一行为。

## 传输和缓存限制

ADS 请使用 POST。非 POST 请求可能被公开边缘规则拦截并返回 Cloudflare 的 403，不应将其当成应用授权判定。系统不提供让浏览器携带秘密令牌直接调用 ADS 的 CORS 流程。

已匹配的应用 ADS 响应使用 `Cache-Control: no-store`，不要将判定结果或凭据放入共享缓存。

如需 Java、Python、Go、.NET、Rust 或 Node.js/TypeScript 客户端库，请参阅 [SDK 文档](/zh/docs/sdks/)。失败、额度和响应头说明见[限额与错误](/zh/docs/cloud/limits-and-errors/)。API 规范和 Markdown 下载链接见 [Speedle Cloud 概览](/zh/docs/cloud/#下载)。
