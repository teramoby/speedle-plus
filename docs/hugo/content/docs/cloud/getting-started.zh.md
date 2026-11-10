+++
title = "Speedle Cloud 快速入门"
linkTitle = "快速入门"
description = "了解 Speedle Cloud 的登录与账号管理、service 与 SPDL 策略管理、会话 API 请求和 Playground 测试。"
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "已发布的 Speedle Cloud 控制台流程，涵盖登录、账号设置、已保存策略和 Playground 判定。"
lastmod = 2026-11-08T21:11:24+08:00
+++

**Speedle Cloud 即将上线。** cloud.speedle.io 的生产环境尚未开放登录，Cloud 页眉显示 **Coming soon（即将上线）** 徽章。登录关闭时，登录页会提示即将开放。本文介绍已发布的使用流程，不代表生产控制台目前已经开放；生产环境开放登录后会补充实际截图。

## 生产环境

账号会话、管理请求和应用令牌使用 `https://cloud.speedle.io`。账号、资源和令牌归属于该 origin。本文示例仅用于说明，本次未向线上环境执行。请确认所选环境已经部署，并确认准备使用的登录供应商在该环境可用。

## 使用 GitHub 或 Google 登录（即将开放）

即使登录尚未开放，登录页仍会显示 **使用 GitHub 登录** 和 **使用 Google 登录** 两个选项。生产环境开放登录后，请选择当前可用的供应商。新身份的已验证邮箱未被占用时，需要主动确认 **创建账号**。邮箱已被占用时，先用控制台提示的已关联供应商登录，再到 **账号设置** 连接第二个身份；系统不会自动合并账号。

账号设置可以查看已连接身份，并连接或断开当前可用的供应商；至少保留一个已连接身份。**退出登录** 位于页眉语言切换旁，账号菜单提供 **在所有设备上退出**。退出成功后返回登录页，并清空工作区和一次性令牌展示。审计日志展示账号活动。

删除账号需要完成最近的供应商认证，并输入 `DELETE`。随后会永久删除账号所属 service、策略、令牌和身份，并结束所有会话。删除前请先从服务工作区导出需要保留的策略。

## 创建 service 和策略

新账号打开[服务工作区](https://cloud.speedle.io/zh-CN/service)，在服务名称中输入 `books`，然后选择 **创建服务**。每个账号只有一个 service。如果账号已有 service 或策略，请先核对并使用合适的现有配置，不要为了运行示例而覆盖或删除已有数据。

选择 **新建策略**，在策略名称中输入 `read-books`，并在 SPDL 编辑器中输入以下授权策略：

```text
grant user alice from corp get /books
```

`from corp` 将这条授权限制在 `corp` 身份域内。请将 `corp` 替换为应用根据可信认证结果得出的身份域，并在每个请求中把相同的值作为主体的 `idd` 发送，如下面的 Playground 示例所示。不带 `from` 的授权会匹配任意身份域中名为 `alice` 的用户；只有 service 只有一个身份来源、名称不可能跨域重复时，才可以省略该子句。规则和请求中对应的字段见[身份域](/zh/docs/idd/)。

选择 **验证策略**，确认控制台显示 **识别为：授权策略**，再选择 **保存策略**。验证会识别策略类型并检查语法，但不会保存草稿；保存需要单独手动操作。你也可以在工作区导入已有策略，无需逐条手动输入。

Speedle Cloud 支持授权策略和角色策略，不支持外部自定义函数。策略列表按两种类型分组并显示数量。角色分配使用以下形式：

- 全局角色：`grant user alice editor`
- 限定资源的角色：`grant user alice editor on /books`

`grant user alice role editor` 同时符合两类语法，会被拒绝。角色分配请省略可选的 `role` 前缀；授权策略请写成 `grant (user alice) role editor`。编辑会保留已存储的策略类型；如需改变类型，请删除原策略后新建。

遇到冲突或响应结果不明确时，请保留草稿，先刷新已保存数据，再决定是否执行另一项修改。切换语言会保留草稿。Monaco 内置菜单沿用编辑器首次加载时的语言，重新加载页面后才跟随新选择。

删除 service 时，需要输入服务名称、完成最近的供应商认证，并在验证后再次确认。

## 发送管理会话请求

策略管理服务（PMS）使用账号会话。请让受支持的同源客户端管理安全的会话 cookie，不要导出浏览器 cookie，也不要通过测试供应商初始化账号。修改请求必须携带与当前环境完全一致的 `Origin`、`X-SC-CSRF: 1` 和 JSON 内容类型。ADS Bearer 令牌不能用于 PMS 认证。客户端不得提交账号或 service 归属选择参数，也不得提交认证时间戳。

上述 service 和策略示例等价于以下请求：

| 操作 | 会话请求 | 成功状态 |
| --- | --- | --- |
| 创建账号的 service | `POST /api/service`，正文为 `{"name":"books"}` | 201 |
| 添加授权策略 | `POST /api/service/policies`，正文如下 | 201 |

```json
{"name":"read-books","spdl":"grant user alice from corp get /books"}
```

使用控制台时，只把 `spdl` 的值粘贴到编辑器中；这段 JSON 正文用于管理 API。

## 验证和导出策略

使用 `POST /api/service/validate` 和 `{spdl}` 正文校验 SPDL 并识别类型。旧的 `{kind: "authz", spdl}` 形式仍指定授权策略解析器。**下载 SPDL** 和 **下载 JSON** 分别调用 `GET /api/service/export?format=spdl` 和 `GET /api/service/export?format=json`。

会话 Playground 使用当前账号在 D1 中的最新数据。后续带认证的 ADS 版本使用发布后的策略快照，传播约需 60 秒。因此，Playground 测试成功并不表示所有 ADS 节点都已拿到新快照。

## 在 Playground 中测试已保存策略

在服务工作区找到 **Playground**，选择 **是否允许**。在 **主体（JSON）** 中输入 `[{"type":"user","name":"alice","idd":"corp"}]`，在 **资源** 中输入 `/books`，在 **动作** 中输入 `get`，在 **属性（JSON）** 中输入 `[]`，然后选择 **运行测试**。

其他操作包括 **诊断**、**全部已授予角色** 和 **全部已授予权限**。Playground 只使用已保存的策略，不包含编辑器中的未保存草稿。修改输入、操作或语言不会自动发起请求。输入或已保存策略变化后，请再次运行；页面会把上次结果标记为已过期，并在后续请求失败时保留此前结果。

## 使用会话 Playground API

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

接下来请阅读[应用令牌](/zh/docs/cloud/tokens/)、[授权 API 预览](/zh/docs/cloud/authorization-api/)和[限额与错误](/zh/docs/cloud/limits-and-errors/)。
