+++
title = "Speedle Cloud 快速入门"
linkTitle = "快速入门"
description = "了解 Speedle Cloud 的登录、SPDL 策略创建、使用 API 密钥从应用发起授权检查，以及 Playground 测试流程。"
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "登录控制台，保存策略，创建 API 密钥，从应用发起授权检查。"
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

## 使用 GitHub 或 Google 登录（即将开放）

在浏览器中打开 [Cloud 控制台](https://cloud.speedle.io/zh-CN/)，选择可用的登录方式。新身份的已验证邮箱尚未被使用时，确认 **创建账号**。如果邮箱已被使用，请使用控制台提示的已关联登录方式登录。

## 创建 service 和策略

打开[服务工作区](https://cloud.speedle.io/zh-CN/service)，输入 `books`，选择 **创建服务**。请用新服务运行示例，避免已有策略影响结果，不要覆盖已有数据。

选择 **新建策略**，命名为 `read-books`，输入以下 SPDL：

```text
grant user alice from corp get /books
```

这条策略允许 `corp` 身份域中的 `alice` 对 `/books` 执行 `get`。不带 `from` 的授权会匹配所有身份域中同名的用户。应用应根据可信的认证结果确定身份域，并在每个请求的 `idd` 字段中发送相同的值。详见[身份域](/zh/docs/idd/)。

选择 **验证策略**，然后 **保存策略**。验证只检查语法，不会保存草稿。保存后，策略约需 60 秒才会在授权 API 中生效。

## 创建 API 密钥

打开 [API 密钥](https://cloud.speedle.io/zh-CN/tokens)页面，输入名称（如 `books-app`），选择 `ads` 权限范围（授权检查），选择有效期（控制台默认 3 个月，并显示 UTC 时间），然后选择 **创建 API 密钥**。如需验证身份，请完成验证后再次提交表单。复制仅展示一次的密钥值。每个密钥都会到期；不再使用时请提前撤销。详见 [API 密钥管理](/zh/docs/cloud/tokens/)。

将密钥保存在服务端的密钥存储中，通过环境变量 `SPEEDLE_API_KEY` 提供给应用，不要写入浏览器代码或源码仓库。

## 从应用发起授权检查

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

接入细节见[授权 API](/zh/docs/cloud/authorization-api/) 和 [SDK](/zh/docs/sdks/)，包括 [Node.js/TypeScript](/zh/docs/sdks/node/)、[Python](/zh/docs/sdks/python/)、[Java](/zh/docs/sdks/java/) 和 [Go](/zh/docs/sdks/go/)。

## 在 Playground 中测试已保存策略

也可以打开服务的 **Playground**，选择 **是否允许**。在 **主体（JSON）** 中输入 `[{"type":"user","name":"alice","idd":"corp"}]`，在 **资源** 中输入 `/books`，在 **动作** 中输入 `get`，在 **属性（JSON）** 中输入 `[]`，然后选择 **运行测试**。

**是否允许**显示判定结果，**诊断**解释策略如何影响判定，**全部已授予角色**列出主体的角色，**全部已授予权限**列出获准访问的资源和动作。测试只使用已保存的策略，不包含未保存的草稿。修改输入或策略后，请再次运行测试。应用通过 [ADS 授权 API](/zh/docs/cloud/authorization-api/) 携带 API 密钥检查权限。

下一步：如需通过程序管理策略，请单独创建具有 `pms:write` 权限范围的密钥（只读时使用 `pms:read`），再使用 [PMS API 或 spctl 的 Speedle Cloud 用法](/zh/docs/pms/policy-mgmt/#speedle-cloud)。上一步创建的 `ads` 密钥不能管理策略。
