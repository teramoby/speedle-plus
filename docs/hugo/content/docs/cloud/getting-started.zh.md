+++
title = "Speedle Cloud 快速入门"
linkTitle = "快速入门"
description = "了解 Speedle Cloud 的使用流程：登录、创建服务、编写 SPDL 策略、测试授权判定、创建 API 令牌并调用 IsAllowed。"
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "预览 Speedle Cloud 的使用流程，涵盖登录、SPDL 策略编写、Playground 测试和授权 API 调用。"
+++

**Speedle Cloud 即将上线。** cloud.speedle.io 尚未开放登录，带认证的授权 API 也仍在逐步交付。下面介绍计划中的生产环境使用流程。生产环境开放登录后会补充实际截图，本文不使用模拟界面图。

## 1. 使用 GitHub 或 Google 登录

打开 [Speedle Cloud](https://cloud.speedle.io/zh-CN/)。生产环境开放登录后，选择 **使用 GitHub 登录** 或 **使用 Google 登录**。如果你要创建新账号，还需要明确点击 **创建账号** 进行确认。

> **截图待补充**
>
> 此处将展示生产环境的登录页面。

## 2. 创建服务

打开服务工作区，输入服务名称（例如 `books`），然后点击 **创建服务**。服务用于集中管理应用的授权策略和角色策略。每个免费账号可以创建一个服务。

> **截图待补充**
>
> 此处将展示创建服务的页面。

## 3. 编写或导入 SPDL 策略

在服务工作区点击 **新建策略**，在控制台中编写 SPDL，点击 **验证策略** 检查语法，再点击 **保存策略**。验证语法不会自动保存策略。你也可以在工作区导入已有策略，无需逐条手动输入。

例如，下面这条授权策略允许用户 `alice` 对 `/books` 执行 `get` 操作：

`grant user alice get /books`

Speedle Cloud 支持授权策略和角色策略，不支持外部自定义函数。

> **截图待补充**
>
> 此处将展示策略编辑器和导入控件。

## 4. 在 Playground 中测试授权判定

在服务工作区找到 **Playground**，选择 **Is allowed**。在 Principals 中输入 `[{"type":"user","name":"alice"}]`，在 Resource 中输入 `/books`，在 Action 中输入 `get`，然后点击 **运行测试**。

Playground 使用已保存的策略进行判定，不会包含编辑器中尚未保存的草稿。修改策略或请求后，需要重新运行测试才能刷新结果。

> **截图待补充**
>
> 此处将展示 Playground 请求及其判定结果。

## 5. 创建 API 令牌

打开 **应用令牌**，输入令牌名称，然后点击 **创建令牌**。系统可能要求你先使用 GitHub 或 Google 完成身份验证。令牌只显示一次，请及时复制并存入应用服务端的密钥存储，之后无法再次查看。

令牌格式为 `spdl_<canonical lowercase UUIDv7 id>_<43 base62 digits>`。它是服务专属的 Bearer 凭据，用于授权检查，既不是最终用户的身份令牌，也不能用于调用管理 API。切勿将令牌放入浏览器代码、URL、源代码仓库、截图或日志。

> **截图待补充**
>
> 此处将展示令牌创建和一次性显示的页面。

## 6. 调用授权 API

带认证的授权调用开放后，应用服务端将向 `https://cloud.speedle.io/authz-check/v1/is-allowed` 发送 `POST` 请求，并在一个 `Authorization: Bearer` 请求头中携带签发的令牌：

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <issued token>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

应用必须自行完成用户认证，并直接传入用户的 principals。Speedle Cloud 不支持令牌断言。执行受保护的操作前，必须检查响应中的 `allowed` 值；发生错误时应默认拒绝。

如需 Java、Python、Go、.NET、Rust 或 Node.js/TypeScript 客户端库，请参阅 [SDK 文档](/docs/sdks/)。

> **截图待补充**
>
> 此处将展示生产环境的 API 接入流程。

## 测试版限额

| 限额 | 测试版额度 |
| --- | --- |
| 策略 | 每个账号最多 200 条策略，授权策略和角色策略合并计算。 |
| API 调用 | 每个账号每天最多 10,000 次 API 调用，管理调用和授权调用合并计算，每天 00:00 UTC 重置。 |

免费测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境。功能和限额可能调整。

## 测试版条款与隐私政策

请阅读 Speedle Cloud 的[测试版条款](https://cloud.speedle.io/zh-CN/terms)和[隐私政策](https://cloud.speedle.io/zh-CN/privacy)。
