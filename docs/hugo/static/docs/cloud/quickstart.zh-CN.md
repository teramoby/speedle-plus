# Speedle Cloud 文档

Speedle Cloud 是基于开源 Speedle 引擎的托管授权服务。你可以管理服务、SPDL 授权策略和角色策略，在 Playground 中测试判定，并从服务端调用 API。

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。

使用控制台和创建 API 密钥需要账号登录功能可用。免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境。功能和限额可能调整。

## 文档导航

- [快速入门](https://engine.speedle.io/zh/docs/cloud/getting-started/): 登录、账号、服务、策略和会话 Playground。
- [API 密钥](https://engine.speedle.io/zh/docs/cloud/tokens/): 账号级凭据、固定权限范围、创建与撤销、一次性展示和分页。
- [授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/): IsAllowed 请求、调用方身份、Node.js 22 和传输限制。
- [限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/): 账号上限、大小限制、突发限流、每日额度、错误代码和重置时间。

## 下载

- [中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md): 四个 Cloud 章节合并的离线指南。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

API 规范描述请求与响应合约；下载规范不会开放账号登录。

请阅读 Cloud 的 [服务条款](https://cloud.speedle.io/zh-CN/terms) / [隐私政策](https://cloud.speedle.io/zh-CN/privacy).

## Speedle Cloud 快速入门

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。本文介绍已发布的使用流程，不代表生产控制台目前已经开放；生产环境开放登录后会补充实际截图。

### 生产环境

账号会话、管理请求和API 密钥使用 `https://cloud.speedle.io`。账号、资源和API 密钥归属于该 origin。本文示例仅用于说明，本次未向线上环境执行。请确认所选环境已经部署，并确认准备使用的登录供应商在该环境可用。

### 使用 GitHub 或 Google 登录（即将开放）

即使登录尚未开放，登录页仍会显示 **使用 GitHub 登录** 和 **使用 Google 登录** 两个选项。生产环境开放登录后，请选择当前可用的供应商。新身份的已验证邮箱未被占用时，需要主动确认 **创建账号**。邮箱已被占用时，先用控制台提示的已关联供应商登录，再到 **账号设置** 连接第二个身份；系统不会自动合并账号。

账号设置可以查看已连接身份，并连接或断开当前可用的供应商；至少保留一个已连接身份。**退出登录** 位于页眉语言切换旁，账号菜单提供 **在所有设备上退出**。退出成功后返回登录页，并清空工作区和一次性API 密钥展示。审计日志展示账号活动。

删除账号需要完成最近的供应商认证，并输入 `DELETE`。随后会永久删除账号所属 service、策略、API 密钥和身份，并结束所有会话。删除前请先从服务工作区导出需要保留的策略。

### 创建 service 和策略

新账号打开[服务工作区](https://cloud.speedle.io/zh-CN/service)，在服务名称中输入 `books`，然后选择 **创建服务**。每个账号最多可创建 100 个服务。如果账号已有 service 或策略，请先核对并使用合适的现有配置，不要为了运行示例而覆盖或删除已有数据。

选择 **新建策略**，在策略名称中输入 `read-books`，并在 SPDL 编辑器中输入以下授权策略：

```text
grant user alice from corp get /books
```

`from corp` 将这条授权限制在 `corp` 身份域内。请将 `corp` 替换为应用根据可信认证结果得出的身份域，并在每个请求中把相同的值作为主体的 `idd` 发送，如下面的 Playground 示例所示。不带 `from` 的授权会匹配任意身份域中名为 `alice` 的用户；只有 service 只有一个身份来源、名称不可能跨域重复时，才可以省略该子句。规则和请求中对应的字段见[身份域](https://engine.speedle.io/zh/docs/idd/)。

选择 **验证策略**，确认控制台显示 **识别为：授权策略**，再选择 **保存策略**。验证会识别策略类型并检查语法，但不会保存草稿；保存需要单独手动操作。你也可以在工作区导入已有策略，无需逐条手动输入。

Speedle Cloud 支持授权策略和角色策略，不支持外部自定义函数。策略列表按两种类型分组并显示数量。角色分配使用以下形式：

- 全局角色：`grant user alice editor`
- 限定资源的角色：`grant user alice editor on /books`

`grant user alice role editor` 同时符合两类语法，会被拒绝。角色分配请省略可选的 `role` 前缀；授权策略请写成 `grant (user alice) role editor`。编辑会保留已存储的策略类型；如需改变类型，请删除原策略后新建。

遇到冲突或响应结果不明确时，请保留草稿，先刷新已保存数据，再决定是否执行另一项修改。切换语言会保留草稿。Monaco 内置菜单沿用编辑器首次加载时的语言，重新加载页面后才跟随新选择。

删除 service 时，需要输入服务名称、完成最近的供应商认证，并在验证后再次确认。

### 发送管理会话请求

策略管理服务（PMS）使用账号会话。请让受支持的同源客户端管理安全的会话 cookie，不要导出浏览器 cookie，也不要通过测试供应商初始化账号。修改请求必须携带与当前环境完全一致的 `Origin`、`X-SC-CSRF: 1` 和 JSON 内容类型。ADS Bearer API 密钥不能用于 PMS 认证。客户端不得提交账号或 service 归属选择参数，也不得提交认证时间戳。

上述 service 和策略示例等价于以下请求：

| 操作 | 会话请求 | 成功状态 |
| --- | --- | --- |
| 创建账号的 service | `POST /api/service`，正文为 `{"name":"books"}` | 201 |
| 添加授权策略 | `POST /api/service/policies`，正文如下 | 201 |

```json
{"name":"read-books","spdl":"grant user alice from corp get /books"}
```

使用控制台时，只把 `spdl` 的值粘贴到编辑器中；这段 JSON 正文用于管理 API。

### 验证和导出策略

使用 `POST /api/service/validate` 和 `{spdl}` 正文校验 SPDL 并识别类型。旧的 `{kind: "authz", spdl}` 形式仍指定授权策略解析器。**下载 SPDL** 和 **下载 JSON** 分别调用 `GET /api/service/export?format=spdl` 和 `GET /api/service/export?format=json`。

会话 Playground 使用当前账号在 D1 中的最新数据。后续带认证的 ADS 版本使用发布后的策略快照，传播约需 60 秒。因此，Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。

### 在 Playground 中测试已保存策略

在服务工作区找到 **Playground**，选择 **是否允许**。在 **主体（JSON）** 中输入 `[{"type":"user","name":"alice","idd":"corp"}]`，在 **资源** 中输入 `/books`，在 **动作** 中输入 `get`，在 **属性（JSON）** 中输入 `[]`，然后选择 **运行测试**。

其他操作包括 **诊断**、**全部已授予角色** 和 **全部已授予权限**。Playground 只使用已保存的策略，不包含编辑器中的未保存草稿。修改输入、操作或语言不会自动发起请求。输入或已保存策略变化后，请再次运行；页面会把上次结果标记为已过期，并在后续请求失败时保留此前结果。

### 使用会话 Playground API

`POST /api/service/test` 实现全部四种操作，与其他管理请求使用相同的 Origin/CSRF 规则和账号每日额度。它读取当前账号 D1 策略的一致快照，不发布或保存策略，也不接受账号或 service ID。页面展示的是提交请求时工作区已知的 revision，不能证明服务端实际求值的快照版本。

```http
POST /api/service/test
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"op":"isAllowed","request":{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3}]}}
```

| `request.op` | 成功正文 | 已发布的控制台与会话 API 合约 |
| --- | --- | --- |
| `isAllowed` | Speedle IsAllowedResponse：`allowed`、`reason` 和可选的 `errorMessage` | 已包含 |
| `diagnose` | Speedle EvaluationDebugResponse：reason 名称、`requestContext`、`attributes`、`policies` | 已包含 |
| `allGrantedRoles` | `string[]`；无结果时为 `[]` | 已包含 |
| `allGrantedPermissions` | `{resource, actions}[]`；无结果时为 `[]` | 已包含 |

成功正文保持上游原始形状，不增加 `op` 或 `result` 包裹。两种 all-granted 结果均可能为 `null` 或空数组；这两种形状都不代表授权允许。ADS 不提供公开 Diagnose 接口。

接下来请阅读[API 密钥](https://engine.speedle.io/zh/docs/cloud/tokens/)、[授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/)和[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)。

## Speedle Cloud API 密钥

API 密钥属于账号，按权限范围访问该账号下的服务。它认证的是应用，不是最终用户。管理 API 密钥、会话和关联身份始终需要已登录的控制台会话。

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。

### 创建 API 密钥

打开 [API 密钥](https://cloud.speedle.io/zh-CN/tokens)，输入去除首尾空白后为 1–128 个字符的名称，选择权限范围，点击 **创建**。创建需要在最近 10 分钟内通过已关联的身份提供方认证。收到 `403 reauth_required` 时，重新认证后再次点击 **创建**；从身份提供方返回不会自动创建密钥。

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

{"name":"books-app","scope":"ads"}
```

请求只接受 `name` 和可选的 `scope`（默认 `ads`）。不要发送服务选择、到期时间或认证证明。请求需要会话 cookie 和 CSRF 请求头；API 密钥不能代替会话。

### 保护密钥值

创建成功返回 HTTP 201。响应的 `token` 字段包含 API 密钥值，只显示一次：

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

API 密钥在撤销前一直有效，不会自动过期。

这只是格式说明，不能作为实际密钥使用。请立即保存到服务端的密钥存储中。不要放入浏览器存储、前端代码、URL、源码、截图或日志。列表只返回元数据，丢失的密钥值无法找回。

每个账号所有权限范围合计最多 **10 个有效 API 密钥**。继续创建会返回 `409 {"error":"quota_exceeded","limit":10}`。撤销不再使用的密钥可释放名额；名额不会随时间重置。已撤销的密钥仍保留在列表中，不占有效名额。

### 列出 API 密钥

`GET /api/tokens` 按最新在前列出有效和已撤销的密钥。默认每页 25 条，`limit` 可设为 1–100。将上次响应的 `nextCursor` 作为 `cursor` 继续请求，直到 `nextCursor` 为 `null`。`lastUsedAt` 尽力更新，通常每小时最多一次，不能据此断定密钥已无人使用。

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

可用操作是 **创建** 和 **撤销**。如需更换密钥值或权限范围，请创建另一个密钥，更新调用方后撤销旧密钥。达到上限且所有密钥仍在使用时，撤销前需安排调用中断。删除服务不会撤销账号的 API 密钥。

创建或撤销的响应丢失时，先刷新列表再操作。同名记录不能找回丢失的密钥值。参见[限额与错误](https://engine.speedle.io/zh/docs/cloud/limits-and-errors/)和[授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。

## Speedle Cloud 授权 API

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。

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

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。

除表中另有说明外，限额由整个账号共用。API 密钥属于账号；更换密钥不会增加每日额度。

### 限额

| 限制对象 | 上限 | 达到后如何处理 |
| --- | --- | --- |
| 每个账号的服务 | 100 个 | `409 quota_exceeded`；删除不用的服务。不会定时重置。 |
| 所有服务的授权策略与角色策略合计 | 每账号 200 条 | `409 quota_exceeded`；删除策略。不会定时重置。 |
| 单条策略的完整 SPDL 源文本 | 1,024 个 UTF-8 字节 | 控制台策略写入返回 `400 invalid_request`；缩短源文本。按字节计数，不是按字符计数。不会定时重置。 |
| 所有权限范围的有效 API 密钥合计 | 每账号 10 个 | `409 quota_exceeded`，带 `limit: 10`；撤销不用的密钥。不会定时重置；已撤销历史不占名额。 |
| 策略名称 / API 密钥名称 | 128 个 Unicode 码点 / 去除首尾空白后 1–128 个字符 | `400 invalid_request`；缩短或修正名称。不会定时重置。 |
| 策略管理与授权调用的计费次数 | 每账号每 UTC 日 10,000 次 | `429 daily_quota_exceeded`；00:00 UTC 重置。遵守 `Retry-After`。 |
| API 密钥请求的短时突发 | 每个 Cloud 位置、每个密钥每分钟约 600 次 | `429 rate_limited`，带 `Retry-After: 60`；等待后重试。这是短时限制，不保证吞吐量，也不是额外的每日额度。 |
| 普通请求正文 | 65,536 字节（64 KiB） | `413 request_too_large`；缩小正文。不会定时重置。 |
| 原子 JSON 导入正文（仅 `POST /api/service/import`） | 5,100,000 字节 | `413 request_too_large`；缩小导入内容。仍须满足单条策略和账号限额。不会定时重置。 |
| 输入属性 | 50 个属性；规范化 JSON 的 UTF-8 长度为 16,384 字节 | `400 SPDL-0004`；减少属性或缩小属性值。不会定时重置。 |

权限范围、创建和撤销规则见 [API 密钥](https://engine.speedle.io/zh/docs/cloud/tokens/)。

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
| `403 reauth_required` | 创建密钥前，通过已关联的身份提供方重新认证。 |
| `403 insufficient_scope` | 使用具有所需权限范围的密钥。 |
| `403 management_scope_disabled` | 登录关闭时不能签发管理权限的密钥。 |
| `403 csrf_failed` | 使用控制台会话及所需的来源和 CSRF 请求头。 |
| `404 service_not_found`（ADS）/ `404 not_found`（管理） | 选择当前账号中存在的服务。密钥 ID 未知、属于其他账号或已撤销时也返回 `404 not_found`。 |
| `409 quota_exceeded` | 释放相应的服务、策略或 API 密钥名额；等待不会重置这些上限。 |
| `413 request_too_large` | 缩小正文。 |
| `429 rate_limited` / `429 daily_quota_exceeded` | 遵守 `Retry-After` 并使用有界退避；每日额度在 UTC 午夜重置。 |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | 默认拒绝，按有界退避重试；带有 `Retry-After` 时遵守它。 |

API 密钥验证通过后，`X-RateLimit-Limit` 为 600，不提供精确的 `X-RateLimit-Remaining`。如果有 `X-Quota-Daily-Remaining`，它只是账号级剩余额度的估计值；并发调用和未使用的预留可能使它偏离实际可用次数。它不保证下一次请求成功，提前拒绝的响应不带此头。

ADS 响应使用 `Cache-Control: no-store`，不要共享缓存判定或凭据。请使用 POST；其他方法可能收到 `405 method_not_allowed`，或更早被拦截并返回非 JSON 响应。参见[授权 API](https://engine.speedle.io/zh/docs/cloud/authorization-api/)。
