# Speedle Cloud 文档

Speedle Cloud 是基于开源 Speedle 策略引擎的托管访问控制服务，面向开发者。你可以用它管理 SPDL 授权策略与角色策略、在 Playground 中测试授权判定，再从应用服务端调用授权 API，无需自行部署和运维授权服务。

**Speedle Cloud 即将上线。** Cloud 页眉显示 **Coming soon（即将上线）** 徽章，[cloud.speedle.io](https://cloud.speedle.io/zh-CN/) 的生产环境尚未开放登录。本文介绍的控制台流程和带认证的授权决策服务（ADS）均为已发布服务合约的预览。带认证的 ADS 尚未提供时，签发令牌并不会启用服务端授权调用。所选环境的部署情况和登录供应商可用性仍需分别确认。

免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境，功能和限额可能调整。

## 文档导航

- [快速入门](https://engine.speedle.io/zh/docs/cloud/getting-started/)：登录控制台，创建服务和策略，创建 API 密钥，从应用发起授权检查。
- [应用令牌](https://engine.speedle.io/zh/docs/cloud/tokens/)：最近认证要求、令牌的一次性展示、元数据分页、轮换和撤销。
- [授权 API 预览](https://engine.speedle.io/zh/docs/cloud/authorization-api/)：IsAllowed 请求与响应合约、Node.js 22 示例、策略快照和接入限制。
- [API 参考](https://engine.speedle.io/zh/docs/cloud/api-reference/)：使用 API key 进行授权检查和策略管理。请求从你的服务器发出；本页只读。
- [限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)：账号额度、请求限制、错误响应、限流与额度响应头，以及重试建议。

## 下载

- [简体中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md)：汇集 Cloud 文档各章，便于离线阅读。
- [API 参考](https://engine.speedle.io/zh/docs/cloud/api-reference/)：在本站阅读。下面仍保留原始文件。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)：Cloud API 的权威仓库规范。
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)：由同一份 YAML 原文生成。
- [当前构建版本](https://cloud.speedle.io/build.json)：用于识别正在提供服务的构建，不证明登录供应商或接口当前可用。

规范包含四种会话 Playground 操作和带认证的 ADS 合约。发布规范或下载指南并不会启用尚未提供的接口，也不会开放登录。Cloud 的 YAML 与生成的 JSON 随对应的封存构建一起记录版本、晋升或恢复；中英文 Markdown 下载则随这些 Engine 文档页面一起维护。

请阅读 Cloud 的[服务条款](https://cloud.speedle.io/zh-CN/terms)和[隐私政策](https://cloud.speedle.io/zh-CN/privacy)。

## Speedle Cloud 快速入门

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

### 使用 GitHub 或 Google 登录（即将开放）

在浏览器中打开 [Cloud 控制台](https://cloud.speedle.io/zh-CN/)，选择可用的登录方式。新身份的已验证邮箱尚未被使用时，确认 **创建账号**。如果邮箱已被使用，请使用控制台提示的已关联登录方式登录。

### 创建 service 和策略

打开[服务工作区](https://cloud.speedle.io/zh-CN/service)，输入 `books`，选择 **创建服务**。请用新服务运行示例，避免已有策略影响结果，不要覆盖已有数据。

选择 **新建策略**，命名为 `read-books`，输入以下 SPDL：

```text
grant user alice from corp get /books
```

这条策略允许 `corp` 身份域中的 `alice` 对 `/books` 执行 `get`。不带 `from` 的授权会匹配所有身份域中同名的用户。应用应根据可信的认证结果确定身份域，并在每个请求的 `idd` 字段中发送相同的值。详见[身份域](https://engine.speedle.io/zh/docs/idd/)。

选择 **验证策略**，然后 **保存策略**。验证只检查语法，不会保存草稿。保存后，策略约需 60 秒才会在授权 API 中生效。

### 创建 API 密钥

打开 [API 密钥](https://cloud.speedle.io/zh-CN/tokens)页面，输入名称（如 `books-app`），选择 `ads` 权限范围（授权检查），然后选择 **创建 API 密钥**。如需验证身份，请完成验证后再次提交表单。复制仅展示一次的密钥值。目前不支持选择密钥有效期，不再使用时请撤销。详见 [API 密钥管理](https://engine.speedle.io/zh/docs/cloud/tokens/)。

将密钥保存在服务端的密钥存储中，通过环境变量 `SPEEDLE_API_KEY` 提供给应用，不要写入浏览器代码或源码仓库。

### 从应用发起授权检查

在应用服务端发送以下 `isAllowed` 请求，检查上一步保存的策略。这条策略不需要属性，因此 `attributes` 为空数组：

```bash
curl --fail-with-body --silent --show-error --max-time 10 \
  'https://cloud.speedle.io/authz-check/v1/is-allowed' \
  -H "Authorization: Bearer $SPEEDLE_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[]}'
```

如果只保存了示例策略，预期响应为：

```json
{"allowed":true,"reason":0}
```

将 `action` 改为 `delete` 后再次发送。没有策略授予该动作的权限，预期响应为：

```json
{"allowed":false,"reason":3}
```

允许和拒绝都返回 HTTP 200。只有 `allowed` 为 `true` 时，应用才能执行受保护的操作；应用必须落实判定结果，遇到错误、无效响应或超时一律拒绝访问。

接入细节见[授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/) 和 [SDK](https://engine.speedle.io/zh/docs/sdks/)，包括 [Node.js/TypeScript](https://engine.speedle.io/zh/docs/sdks/node/)、[Python](https://engine.speedle.io/zh/docs/sdks/python/)、[Java](https://engine.speedle.io/zh/docs/sdks/java/) 和 [Go](https://engine.speedle.io/zh/docs/sdks/go/)。

### 在 Playground 中测试已保存策略

也可以打开服务的 **Playground**，选择 **是否允许**。在 **主体（JSON）** 中输入 `[{"type":"user","name":"alice","idd":"corp"}]`，在 **资源** 中输入 `/books`，在 **动作** 中输入 `get`，在 **属性（JSON）** 中输入 `[]`，然后选择 **运行测试**。

**是否允许**显示判定结果，**诊断**解释策略如何影响判定，**全部已授予角色**列出主体的角色，**全部已授予权限**列出获准访问的资源和动作。测试只使用已保存的策略，不包含未保存的草稿。修改输入或策略后，请再次运行测试。应用通过 [ADS 授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/) 携带 API 密钥检查权限。

下一步：如需通过程序管理策略，请使用 [API 密钥](https://engine.speedle.io/zh/docs/cloud/tokens/)，参阅 [PMS API 或 spctl 的 Speedle Cloud 用法](https://engine.speedle.io/zh/docs/pms/policy-mgmt/#speedle-cloud)。

## Speedle Cloud 授权 API 预览

**带认证的 ADS 尚未提供。** 本页的请求和示例描述后续版本的已发布合约，并未在 Cloud 线上环境执行。签发应用令牌并不会启用这些调用。请明确使用获准环境的 URL，并在接入前确认该环境已经提供带认证的 ADS。

### 从应用服务端调用 IsAllowed

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

[快速入门中的策略](https://engine.speedle.io/zh/docs/cloud/getting-started/#创建-service-和策略)会允许上述示例请求：

```json
{"allowed":true,"reason":0}
```

### 理解判定结果和调用方身份

策略拒绝同样返回 HTTP 200，但 `allowed` 为 `false`。应用必须检查 `allowed`，再执行受保护的操作；遇到异常或无效响应时应默认拒绝。

应用通过自身的用户认证确定调用方的 principals。API 令牌识别的是应用和 service，不是最终用户身份。不要相信匿名浏览器自行提交的 principal 名称。

Speedle Cloud 不校验 subject token。与没有 token asserter 的自托管 Speedle 一样，它接受 `subject.token` 和 `tokenType`，但不做校验就丢弃，因此非空的 token 字段永远不能作为已经通过认证的证据，判定只使用 principals。不要把最终用户的令牌转发过来，并把它当成已完成断言。

会话 Playground 使用当前 D1 策略数据，带认证的 ADS 使用发布后的快照，传播约需 60 秒。Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。ADS 不提供公开 Diagnose 接口。

### Node.js 22 示例

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

### 传输和缓存限制

ADS 请使用 POST。非 POST 请求可能被公开边缘规则拦截并返回 Cloudflare 的 403，不应将其当成应用授权判定。系统不提供让浏览器携带秘密令牌直接调用 ADS 的 CORS 流程。

已匹配的应用 ADS 响应使用 `Cache-Control: no-store`，不要将判定结果或凭据放入共享缓存。

如需 Java、Python、Go、.NET、Rust 或 Node.js/TypeScript 客户端库，请参阅 [SDK 文档](https://engine.speedle.io/zh/docs/sdks/)。失败、额度和响应头说明见[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)。API 规范和 Markdown 下载链接见 [Speedle Cloud 概览](https://engine.speedle.io/zh/docs/cloud/#下载)。

## Speedle Cloud 限额与错误

以下 ADS 错误与响应头描述带认证的 ADS 合约，其运行时尚未提供。管理合约已经定义账号限额、会话与 CSRF 检查及令牌管理错误，供账号访问开放后使用。下载规范本身不会启用尚未提供的接口。

### 限额

| 限额 | 含义 |
| --- | --- |
| 每日 10,000 次 | 同一账号每 UTC 日进入已匹配 PMS 和 ADS handler 的合计次数，由所有会话、令牌和 service 共用；00:00 UTC 重置。 |
| 200 条策略 | 每个账号的授权策略和角色策略合计。 |
| 每条 SPDL 4 KiB | 完整策略最多 4,096 个 UTF-8 字节，不是 4,096 个字符。 |
| 2 个有效令牌 | 每个 service；已撤销的历史记录不占此名额。 |
| 每令牌每分钟 600 次 | 每个节点的近似突发限制，与严格的账号每日总额分开。 |
| 请求正文 64 KiB | 解析前的全局上限。可选输入 attributes 仍受 50 个属性、规范化 UTF-8 JSON 16,384 字节的限制。 |

每个账号只有一个 service。令牌名称为 1–128 个字符。凭据生命周期规则见[应用令牌](https://engine.speedle.io/zh/docs/cloud/tokens/)。

### 每日额度计算

进入已匹配的 handler 即扣除一次每日额度，包括策略拒绝、无效业务输入和内部错误。认证与会话接口不消耗此额度。在进入 handler 前被认证、路由、CSRF、正文大小、突发限流或额度预留检查拒绝的请求，也不消耗每日额度。

已过期或被放弃的预留 lease 不会重新发放，因此实际可用次数可能少于 10,000。

更换令牌不会增加账号额度。严格的账号每日总额与近似的节点级令牌突发限额分别执行。

### HTTP 错误

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

### 限流和额度响应头

完成验证后，`X-RateLimit-Limit` 为 600。ADS 不提供精确的 `X-RateLimit-Remaining` 值。

完成扣额的 handler 响应若带 `X-Quota-Daily-Remaining`，它估计账号今天剩余的调用次数：授予 lease 时的未预留余额，加上本 isolate 尚余的 lease 次数。该估计在授予时偏保守，随后最多缓存至有效的 30 秒 lease 结束，到期或 UTC 换日即作废。

其他 isolate 的并发调用可能使该值高估可用次数，最多高估授予时的全部未预留余额，并非仅 32 次；未使用或被放弃的预留则可能使其低估。发布或回滚期间，旧分配器会保守回退为本地 lease 余量。此响应头仅供参考，不改变额度执行方式。提前拒绝的响应不带此头。

已匹配的应用 ADS 响应还使用 `Cache-Control: no-store`，不要将判定结果或凭据放入共享缓存。

请求示例和接入限制见[授权 API 预览](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。
