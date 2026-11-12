# Speedle Cloud 文档

Speedle Cloud 是基于开源 Speedle 引擎的托管授权服务。你可以管理服务、SPDL 授权策略和角色策略，在 Playground 中测试判定，并从服务端调用 API。

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

使用控制台和创建 API 密钥需要账号登录功能可用。免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境。功能和限额可能调整。

## 文档导航

- [快速入门](https://engine.speedle.io/zh/docs/cloud/getting-started/)：登录控制台，创建服务和策略，创建 API 密钥，从应用发起授权检查。
- [API 密钥](https://engine.speedle.io/zh/docs/cloud/tokens/): 账号级凭据、固定权限范围、必须选择有效期、创建与撤销、一次性展示和分页。
- [授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/): IsAllowed 请求、调用方身份、Node.js 22 和传输限制。
- [API 参考](https://engine.speedle.io/zh/docs/cloud/api-reference/)：使用 API key 进行授权检查和策略管理。请求从你的服务器发出；本页只读。
- [限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/): 账号上限、大小限制、突发限流、每日额度、错误代码和重置时间。

## 下载

- [中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md): 四个 Cloud 章节合并的离线指南。
- [API 参考](https://engine.speedle.io/zh/docs/cloud/api-reference/)：在本站阅读。下面仍保留原始文件。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

API 规范描述请求与响应合约；下载规范不会开放账号登录。

请阅读 Cloud 的 [服务条款](https://cloud.speedle.io/zh-CN/terms) / [隐私政策](https://cloud.speedle.io/zh-CN/privacy).

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

打开 [API 密钥](https://cloud.speedle.io/zh-CN/tokens)页面，输入名称（如 `books-app`），选择 `ads` 权限范围（授权检查），选择有效期（控制台默认 3 个月，并显示 UTC 时间），然后选择 **创建 API 密钥**。如需验证身份，请完成验证后再次提交表单。复制仅展示一次的密钥值。每个密钥都会到期；不再使用时请提前撤销。详见 [API 密钥管理](https://engine.speedle.io/zh/docs/cloud/tokens/)。

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

下一步：如需通过程序管理策略，请单独创建具有 `pms:write` 权限范围的密钥（只读时使用 `pms:read`），再使用 [PMS API 或 spctl 的 Speedle Cloud 用法](https://engine.speedle.io/zh/docs/pms/policy-mgmt/#speedle-cloud)。上一步创建的 `ads` 密钥不能管理策略。

## Speedle Cloud API 密钥

API 密钥属于账号，按权限范围访问该账号下的服务。它认证的是应用，不是最终用户。管理 API 密钥、会话和关联身份始终需要已登录的控制台会话。

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

### 创建 API 密钥

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

### 保护密钥值

创建成功返回 HTTP 201。响应的 `token` 字段包含 API 密钥值，只显示一次：

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

每个密钥都会到期。响应包含服务端计算的 `expiresAt`（Unix 毫秒）。到达该时刻或之后，授权检查和策略管理调用返回 `401 {"error":"api_key_expired"}`。请创建新密钥；再次发送已到期的密钥不会成功。不再使用时可以提前撤销。在必须设置有效期之前创建的密钥现在也有到期时间。如果创建后 365 个 24 小时仍在未来，密钥就在那时到期，即使距离该要求生效只有很短的时间。如果这一年已经过去，密钥在该要求生效后再过 30 个 24 小时到期。列表中的每个密钥都包含 `expiresAt`。请在到期前查看并创建新密钥。

这只是格式说明，不能作为实际密钥使用。请立即保存到服务端的密钥存储中。不要放入浏览器存储、前端代码、URL、源码、截图或日志。列表只返回元数据，丢失的密钥值无法找回。

每个账号所有权限范围合计最多 **10 个有效 API 密钥**。继续创建会返回 `409 {"error":"quota_exceeded","limit":10}`。撤销不再使用的密钥，或等它到期，可释放名额。名额不会按固定周期重置。已撤销和已到期的密钥仍保留在列表中，不占有效名额。

### 列出 API 密钥

`GET /api/tokens` 按最新在前列出有效、已到期和已撤销的密钥。默认每页 25 条，`limit` 可设为 1–100。将上次响应的 `nextCursor` 作为 `cursor` 继续请求，直到 `nextCursor` 为 `null`。每条记录都包含 Unix 毫秒 `expiresAt`。控制台显示 **到期时间（UTC）**。7 天内到期的密钥标为 **即将到期**。已到期的密钥列在 **已到期** 分组中。`lastUsedAt` 尽力更新，通常每小时最多一次，不能据此断定密钥已无人使用。

### 撤销 API 密钥

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

创建或撤销的响应丢失时，先刷新列表再操作。同名记录不能找回丢失的密钥值。参见[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)和[授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。

## Speedle Cloud 授权 API

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

从应用服务端使用账号 API 密钥调用授权接口。创建密钥需要账号登录功能可用。管理权限的密钥不能调用 ADS；请使用 `ads`，否则返回 `403 insufficient_scope`。

### 从应用服务端调用 IsAllowed

将具有 `ads` 权限范围的账号 API 密钥放入一个 `Authorization: Bearer` 请求头，并发送 POST 请求：

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

请求正文就是 Speedle REST 请求，字段和类型与自托管 Speedle ADS 相同。`attributes` 是由 `{name, type, value}` 组成的数组。属性类型支持 `string`、`numeric`、`bool` 和采用 RFC 3339 格式的 `datetime`。

`idd` 是主体所属的身份域，也就是应用在哪个命名空间里完成对调用方的认证，例如某个身份提供方或某个租户。它必须由服务端根据可信的认证结果填写，不能取自浏览器提交的值。

快速入门中的策略 `grant user alice from corp get /books` 只在 `corp` 身份域中匹配上述请求。不带 `from` 的策略匹配任意身份域中的同名主体。主体不带 `idd` 时同样匹配任意身份域，一旦应用接受多个身份来源，两个同名用户就无法区分。多租户接入必须在每个请求中发送 `idd`。

每个请求都必须填写 `serviceName`，系统不会自动取默认值。请选择 API 密钥所属账号中的服务。服务名缺失、为空、不存在或属于其他账号时，统一返回 `404 {"error":"service_not_found"}`。账号 API 密钥不绑定单个服务。

[快速入门中的策略](https://engine.speedle.io/zh/docs/cloud/getting-started/#创建-service-和策略)会允许上述示例请求：

```json
{"allowed":true,"reason":0}
```

### 理解判定结果和调用方身份

策略拒绝同样返回 HTTP 200，但 `allowed` 为 `false`。应用必须检查 `allowed`，再执行受保护的操作；遇到异常或无效响应时应默认拒绝。

应用通过自身的用户认证确定调用方的 principals。API 密钥标识账号和允许访问的 API 范围，不是最终用户身份。不要相信匿名浏览器自行提交的 principal 名称。

Speedle Cloud 不校验 subject token。与没有 token asserter 的自托管 Speedle 一样，它接受 `subject.token` 和 `tokenType`，但不做校验就丢弃，因此非空的 token 字段永远不能作为已经通过认证的证据，判定只使用 principals。不要把最终用户的令牌转发过来，并把它当成已完成断言。

会话 Playground 使用当前保存的策略，带认证的 ADS 使用发布后的快照。Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。ADS 不提供公开 Diagnose 接口。

### Node.js 22 示例

Node.js 22 无需 SDK 即可调用 ADS：

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiKey = process.env.SPEEDLE_API_KEY;
  if (!apiKey) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiKey}`,
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

通过服务端的秘密存储注入 `SPEEDLE_API_KEY`，切勿将值写入代码。传入的主体名称和身份域必须由服务端根据已认证用户得出。示例在组装请求正文之前就拒绝空的主体名称或身份域，否则 `JSON.stringify` 会丢掉 `idd`，发出不带身份域的主体。只有在 service 仅有一个身份来源、名称不可能跨域重复时，才可以省略 `idd`。调用方遇到异常时必须拒绝执行受保护操作，不能把失败转换为允许。

这个最小示例没有重试循环。生产应用应区分文档列出的错误，遵守 `Retry-After`，并使用有界重试。示例中的 `redirect: "error"` 防止 fetch 携带凭据跟随重定向；改写示例时应保留这一行为。

### 传输和缓存限制

ADS 请使用 POST。非 POST 请求可能被公开边缘规则拦截并返回 Cloudflare 的 403，不应将其当成应用授权判定。系统不提供让浏览器携带秘密 API 密钥直接调用 ADS 的 CORS 流程。

已匹配的应用 ADS 响应使用 `Cache-Control: no-store`，不要将判定结果或凭据放入共享缓存。

如需 Java、Python、Go、.NET、Rust 或 Node.js/TypeScript 客户端库，请参阅 [SDK 文档](https://engine.speedle.io/zh/docs/sdks/)。失败、额度和响应头说明见[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)。API 规范和 Markdown 下载链接见 [Speedle Cloud 概览](https://engine.speedle.io/zh/docs/cloud/#下载)。

## Speedle Cloud 限额与错误

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

除表中另有说明外，限额由整个账号共用。API 密钥属于账号；更换密钥不会增加每日额度。

### 限额

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

权限范围、有效期、创建和撤销规则见 [API 密钥](https://engine.speedle.io/zh/docs/cloud/tokens/)。

### 每日额度

读取、写入、校验、Playground 测试、导入、导出和授权检查共用每日额度。策略拒绝或业务输入无效仍计费。认证、会话操作和 `GET /api/me` 不计费。被认证、权限范围、路由、CSRF、正文大小、突发限流或每日额度检查拒绝的请求不计费。

Cloud 自身故障可能退还额度，每账号每 UTC 日最多退还 1,000 次；无法确认退还时仍计费。请求本身导致的错误仍计费。额度计数暂时不可用时，授权调用可能在降速后继续执行，每账号每 UTC 日额外放行最多 10,000 次。这份额度与正常额度独立，不保证一定可用，和退还次数一样在 00:00 UTC 重置。无法放行时返回 `503 quota_unavailable`。

部分每日额度可能已预留但未使用，因此实际可用次数可能少于 10,000。`GET /api/me` 返回用量和重置时间。不要持续重试或通过更换密钥绕过账号限额。

### 错误与响应头

错误使用扁平 JSON，例如 `{"error":"rate_limited"}`。错误不是授权通过；无法取得有效判定时应默认拒绝。

| 状态与错误 | 处理方式 |
| --- | --- |
| `400 invalid_request` 或 Speedle 错误消息 | 修正请求，不要原样重试。 |
| `401 unauthorized` | 检查 API 密钥或登录会话。仅有会话 cookie 不能调用 ADS。 |
| `401 api_key_expired` | API 密钥已到或已过到期时间。请创建新密钥；重试该密钥不会成功。 |
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

ADS 响应使用 `Cache-Control: no-store`，不要共享缓存判定或凭据。请使用 POST；其他方法可能收到 `405 method_not_allowed`，或更早被拦截并返回非 JSON 响应。参见[授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。
