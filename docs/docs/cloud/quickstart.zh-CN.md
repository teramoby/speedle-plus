# Speedle Cloud 文档

Speedle Cloud 是基于开源 Speedle 策略引擎的托管访问控制服务，面向开发者。你可以用它管理 SPDL 授权策略与角色策略、在 Playground 中测试授权判定，再从应用服务端调用授权 API，无需自行部署和运维授权服务。

**Speedle Cloud 即将上线。** Cloud 页眉显示 **Coming soon（即将上线）** 徽章，[cloud.speedle.io](https://cloud.speedle.io/zh-CN/) 的生产环境尚未开放登录。本文介绍的控制台流程和带认证的授权决策服务（ADS）均为已发布服务合约的预览。带认证的 ADS 尚未提供时，签发令牌并不会启用服务端授权调用。所选环境的部署情况和登录供应商可用性仍需分别确认。

免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境，功能和限额可能调整。

## 文档导航

- [快速入门](https://engine.speedle.io/zh/docs/cloud/getting-started/)：使用 GitHub 或 Google 登录（即将开放）、账号管理、service、SPDL 策略、会话管理请求，以及四种会话 Playground 操作。
- [应用令牌](https://engine.speedle.io/zh/docs/cloud/tokens/)：最近认证要求、令牌的一次性展示、元数据分页、轮换和撤销。
- [授权 API 预览](https://engine.speedle.io/zh/docs/cloud/authorization-api/)：IsAllowed 请求与响应合约、Node.js 22 示例、策略快照和接入限制。
- [限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)：账号额度、请求限制、错误响应、限流与额度响应头，以及重试建议。

## 下载

- [简体中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md)：汇集全部四章 Cloud 文档，便于离线阅读。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)：Cloud API 的权威仓库规范。
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)：由同一份 YAML 原文生成。
- [当前构建版本](https://cloud.speedle.io/build.json)：用于识别正在提供服务的构建，不证明登录供应商或接口当前可用。

规范包含四种会话 Playground 操作和带认证的 ADS 合约。发布规范或下载指南并不会启用尚未提供的接口，也不会开放登录。Cloud 的 YAML 与生成的 JSON 随对应的封存构建一起记录版本、晋升或恢复；中英文 Markdown 下载则随这些 Engine 文档页面一起维护。

请阅读 Cloud 的[服务条款](https://cloud.speedle.io/zh-CN/terms)和[隐私政策](https://cloud.speedle.io/zh-CN/privacy)。

## Speedle Cloud 快速入门

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。本文介绍已发布的使用流程，不代表生产控制台目前已经开放；生产环境开放登录后会补充实际截图。

### 生产环境

账号会话、管理请求和应用令牌使用 `https://cloud.speedle.io`。账号、资源和令牌归属于该 origin。本文示例仅用于说明，本次未向线上环境执行。请确认所选环境已经部署，并确认准备使用的登录供应商在该环境可用。

### 使用 GitHub 或 Google 登录（即将开放）

即使登录尚未开放，登录页仍会显示 **使用 GitHub 登录** 和 **使用 Google 登录** 两个选项。生产环境开放登录后，请选择当前可用的供应商。新身份的已验证邮箱未被占用时，需要主动确认 **创建账号**。邮箱已被占用时，先用控制台提示的已关联供应商登录，再到 **账号设置** 连接第二个身份；系统不会自动合并账号。

账号设置可以查看已连接身份，并连接或断开当前可用的供应商；至少保留一个已连接身份。**退出登录** 位于页眉语言切换旁，账号菜单提供 **在所有设备上退出**。退出成功后返回登录页，并清空工作区和一次性令牌展示。审计日志展示账号活动。

删除账号要求最近的供应商认证，并输入 `DELETE`。随后会永久删除账号所属 service、策略、令牌和身份，并结束所有会话。删除前请先从服务工作区导出需要保留的策略。

### 创建 service 和策略

新账号打开[服务工作区](https://cloud.speedle.io/zh-CN/service)，在服务名称中输入 `books`，然后选择 **创建服务**。每个账号只有一个 service。如果账号已有 service 或策略，请先核对并使用合适的现有配置，不要为了运行示例而覆盖或删除已有数据。

选择 **新建策略**，在策略名称中输入 `read-books`，并在 SPDL 编辑器中输入以下授权策略：

```text
grant user alice get /books
```

选择 **验证策略**，确认控制台显示 **识别为：授权策略**，再选择 **保存策略**。验证会识别策略类型并检查语法，但不会保存草稿；保存需要单独手动操作。你也可以在工作区导入已有策略，无需逐条手动输入。

Speedle Cloud 支持授权策略和角色策略，不支持外部自定义函数。策略列表按两种类型分组并显示数量。角色分配使用以下形式：

- 全局角色：`grant user alice editor`
- 限定资源的角色：`grant user alice editor on /books`

`grant user alice role editor` 同时符合两类语法，会被拒绝。角色分配请省略可选的 `role` 前缀；授权策略请写成 `grant (user alice) role editor`。编辑会保留已存储的策略类型；如需改变类型，请删除原策略后新建。

遇到冲突或响应结果不明确时，请保留草稿，先刷新已保存数据，再决定是否执行另一项修改。切换语言会保留草稿。Monaco 内置菜单沿用编辑器首次加载时的语言，重新加载页面后才跟随新选择。

删除 service 时，需要输入服务名称、完成最近的供应商认证，并在验证后再次确认。

### 发送管理会话请求

策略管理服务（PMS）使用账号会话。请让受支持的同源客户端管理安全的会话 cookie，不要导出浏览器 cookie，也不要通过测试供应商初始化账号。修改请求必须携带与当前环境完全一致的 `Origin`、`X-SC-CSRF: 1` 和 JSON 内容类型。ADS Bearer 令牌不能用于 PMS 认证。客户端不得提交账号或 service 归属选择参数，也不得提交认证时间戳。

上述 service 和策略示例等价于以下请求：

| 操作 | 会话请求 | 成功状态 |
| --- | --- | --- |
| 创建账号的 service | `POST /api/service`，正文为 `{"name":"books"}` | 201 |
| 添加授权策略 | `POST /api/service/policies`，正文如下 | 201 |

```json
{"name":"read-books","spdl":"grant user alice get /books"}
```

使用控制台时，只把 `spdl` 的值粘贴到编辑器中；这段 JSON 正文用于管理 API。

### 验证和导出策略

使用 `POST /api/service/validate` 和 `{spdl}` 正文校验 SPDL 并识别类型。旧的 `{kind: "authz", spdl}` 形式仍指定授权策略解析器。**下载 SPDL** 和 **下载 JSON** 分别调用 `GET /api/service/export?format=spdl` 和 `GET /api/service/export?format=json`。

会话 Playground 使用当前账号在 D1 中的最新数据。后续带认证的 ADS 版本使用发布后的策略快照，传播约需 60 秒。因此，Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。

### 在 Playground 中测试已保存策略

在服务工作区找到 **Playground**，选择 **是否允许**。在 **主体（JSON）** 中输入 `[{"type":"user","name":"alice"}]`，在 **资源** 中输入 `/books`，在 **动作** 中输入 `get`，在 **属性（JSON）** 中输入 `[]`，然后选择 **运行测试**。

其他操作包括 **诊断**、**全部已授予角色** 和 **全部已授予权限**。Playground 只使用已保存的策略，不包含编辑器中的未保存草稿。修改输入、操作或语言不会自动发起请求。输入或已保存策略变化后，请再次运行；页面会把上次结果标记为已过期，并在后续请求失败时保留此前结果。

### 使用会话 Playground API

`POST /api/service/test` 实现全部四种操作，与其他管理请求使用相同的 Origin/CSRF 规则和账号每日额度。它读取当前账号 D1 策略的一致快照，不发布或保存策略，也不接受账号或 service ID。页面展示的是提交请求时工作区已知的 revision，不能证明服务端实际求值的快照版本。

```http
POST /api/service/test
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"op":"isAllowed","request":{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3}]}}
```

| `request.op` | 成功正文 | 已发布的控制台与会话 API 合约 |
| --- | --- | --- |
| `isAllowed` | Speedle IsAllowedResponse：`allowed`、`reason` 和可选的 `errorMessage` | 已包含 |
| `diagnose` | Speedle EvaluationDebugResponse：reason 名称、`requestContext`、`attributes`、`policies` | 已包含 |
| `allGrantedRoles` | `string[]`；无结果时为 `[]` | 已包含 |
| `allGrantedPermissions` | `{resource, actions}[]`；无结果时为 `[]` | 已包含 |

成功正文保持上游原始形状，不增加 `op` 或 `result` 包裹。两种 all-granted 结果均可能为 `null` 或空数组；这两种形状都不代表授权允许。ADS 不提供公开 Diagnose 接口。

接下来请阅读[应用令牌](https://engine.speedle.io/zh/docs/cloud/tokens/)、[授权 API 预览](https://engine.speedle.io/zh/docs/cloud/authorization-api/)和[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)。

## Speedle Cloud 应用令牌

应用令牌用于向 Speedle Cloud ADS 认证应用，并且只能访问令牌所属的 service。令牌不代表最终用户身份，也不能用于管理 API 认证。令牌控制台随账号登录开放；带认证的 ADS 尚未提供时，签发令牌不会启用服务端授权调用。

### 签发令牌

账号已有 service 时，打开[应用令牌](https://cloud.speedle.io/zh-CN/tokens)，输入 1–128 个字符的令牌名称，然后选择 **创建令牌**。如果要求验证，请选择当前可用的已关联供应商。从供应商页面返回后，重新输入名称并再次选择 **创建令牌**；返回页面不会自动签发令牌。

控制台会发送以下会话请求：

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app"}
```

示例使用生产环境 origin。客户端沿用原有的安全 cookie 处理方式携带会话，因此这段内容不是可单独执行的匿名请求。

创建令牌要求在最近 10 分钟内完成正常供应商认证，包括登录或重新认证。服务端依据持久化会话和自己的时钟检查。恰好 10 分钟仍有效；认证时间缺失、在未来或已经过期时，返回 `403 {"error":"reauth_required"}`。普通账号操作不会刷新认证时间。

请求只接受 `name`。service ID、scope、过期时间和认证证明均不是客户端可提交的参数。

### 保护一次性秘密值

创建成功时返回 201，响应包含令牌元数据和仅返回一次的 `token` 字段，格式如下：

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

上面只是格式说明，不是可用凭据。令牌前缀中的 ID 与返回的令牌 ID 相同。请立即将签发值存入应用服务端的秘密管理设施。

不要把令牌嵌入前端代码或持久化到浏览器存储，也不要放入 URL、源代码、截图、issue、遥测或日志。正常管理客户端可以短暂持有并展示新签发值，供一次性披露和用户主动复制；不得自动复制、下载、存储或记录秘密值。

令牌列表只返回元数据，之后无法再次取回明文。v1 令牌没有内置过期时间。每个 service 最多同时保留两个有效令牌，已撤销的历史记录不占用这两个名额。

### 查看令牌元数据

`GET /api/tokens` 按 ID 降序返回有效及已撤销令牌的元数据分页。默认每页 25 条，`limit` 为 1–100。cursor 是不包含边界项的令牌 ID；跟随 `nextCursor` 直至 `null`。接口永不返回秘密值。

使用 **刷新最新令牌** 和 **下一页** 查看元数据。`lastUsedAt` 尽力更新且最多每小时更新一次，不能据此证明调用方已经停止使用某个令牌。

### 轮换和撤销令牌

带认证的 ADS 可用后，先通过正常的最近认证创建第二个令牌，切换所有应用和秘密引用，并确认新凭据能按预期策略工作，然后再停用旧凭据。

在 **应用令牌** 页面选择旧令牌的 **撤销** 入口，核对名称与 ID，再确认 **撤销令牌**。管理会话会发送：

```http
DELETE /api/tokens/<old token id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

首次撤销属于自己的有效令牌时，返回无正文的 204。未知、不属于自己或已经撤销的 ID 均返回 404。验证缓存的授权期限从权威查询开始计算，最长固定为 60 秒，因此不能保证撤销立即在全球生效。

如果两个有效名额都已占用，先盘点并切换合法调用方，再停用凭据；不要仅为腾出名额而盲目撤销仍在生产使用的令牌。

如果创建或撤销响应丢失，先核对刷新后的元数据，再主动决定下一步操作。控制台不会自动重试修改请求；同名记录不能恢复或识别丢失的一次性秘密值。

请求限额和管理错误处理见[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)；在授权请求中使用令牌的方法见[授权 API 预览](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。

## Speedle Cloud 授权 API 预览

**带认证的 ADS 尚未提供。** 本页请求和示例描述后续版本的已发布合约，本次未向 Cloud 线上环境执行。签发应用令牌并不会启用这些调用。请明确使用获准环境的 URL，并在接入前确认该环境已经提供带认证的 ADS。

### 从应用服务端调用 IsAllowed

将签发的令牌放入一个 `Authorization: Bearer` 请求头，并发送 POST 请求：

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

请求正文就是 Speedle REST 请求，字段和类型与自托管 Speedle ADS 相同。`attributes` 是由 `{name, type, value}` 组成的数组。属性类型支持 `string`、`numeric`、`bool` 和采用 RFC 3339 格式的 `datetime`。

每个请求都必须填写 `serviceName`，系统不会自动取默认值。请填写令牌所属 service 的名称。名称缺失或为空时，得到与 Speedle 相同的 `SERVICE_NOT_FOUND` 结果；名称不一致时，返回 `403 {"error":"service_mismatch"}`。

[快速入门中的策略](https://engine.speedle.io/zh/docs/cloud/getting-started/#创建-service-和策略)会允许上述示例请求：

```json
{"allowed":true,"reason":0}
```

### 理解判定结果和调用方身份

策略拒绝同样返回 HTTP 200，但 `allowed` 为 `false`。应用必须检查 `allowed`，再执行受保护的操作；遇到异常或无效响应时应默认拒绝。

应用通过自身的用户认证确定调用方的 principals。API 令牌识别的是应用和 service，不是最终用户身份。不要相信匿名浏览器自行提交的 principal 名称。

Speedle Cloud 不校验 subject token。与没有 token asserter 的自托管 Speedle 一样，它接受并忽略 `subject.token` 和 `tokenType`，判定只使用 principals。

会话 Playground 使用当前 D1 策略数据，带认证的 ADS 使用发布后的快照，传播约需 60 秒。Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。ADS 不提供公开 Diagnose 接口。

### Node.js 22 示例

Node.js 22 无需 SDK 即可调用 ADS：

```js
export async function canReadBooks(authenticatedUserName) {
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
      subject: { principals: [{ type: "user", name: authenticatedUserName }] },
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

通过服务端秘密管理设施注入 `SPEEDLE_ADS_TOKEN`，切勿将值写入代码。传入的 principal 必须来自已认证用户。调用方遇到异常时必须拒绝执行受保护操作，不能把失败转换为允许。

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

已过期或被搁置的预留 lease 不会重新发放，因此实际可用次数可能少于 10,000。`GET /api/me` 返回当天已预留用量和重置时间，本身不扣每日额度。

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

其他 isolate 的并发调用可能使该值高估可用次数，最多高估授予时的全部未预留余额，并非仅 32 次；未消费或搁置的预留可能使其低估。发布或回滚期间，旧分配器会保守回退为本地 lease 余量。此响应头仅供参考，不改变额度执行方式。提前拒绝的响应不带此头。

已匹配的应用 ADS 响应还使用 `Cache-Control: no-store`，不要将判定结果或凭据放入共享缓存。

请求示例和接入限制见[授权 API 预览](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。
