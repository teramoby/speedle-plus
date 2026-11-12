+++
title = "Speedle Cloud"
description = "托管授权、账号 API 密钥、请求限额与接入指南。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T21:00:00+08:00
+++

Speedle Cloud 是基于开源 Speedle 引擎的托管授权服务。你可以管理服务、SPDL 授权策略和角色策略，在 Playground 中测试判定，并从服务端调用 API。

**Speedle Cloud 即将上线。** 本文介绍生产环境开放登录和授权 API 后的使用流程，示例尚未在线上 Cloud 环境执行。

使用控制台和创建 API 密钥需要账号登录功能可用。免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境。功能和限额可能调整。

## 文档导航

- [快速入门](/zh/docs/cloud/getting-started/)：登录控制台，创建服务和策略，创建 API 密钥，从应用发起授权检查。
- [API 密钥](/zh/docs/cloud/tokens/): 账号级凭据、固定权限范围、创建与撤销、一次性展示和分页。
- [授权 API](/zh/docs/cloud/authorization-api/): IsAllowed 请求、调用方身份、Node.js 22 和传输限制。
- [API 参考](/zh/docs/cloud/api-reference/)：使用 API key 进行授权检查和策略管理。请求从你的服务器发出；本页只读。
- [限额与错误](/zh/docs/cloud/limits-and-errors/): 账号上限、大小限制、突发限流、每日额度、错误代码和重置时间。

## 下载

- [中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md): 四个 Cloud 章节合并的离线指南。
- [API 参考](/zh/docs/cloud/api-reference/)：在本站阅读。下面仍保留原始文件。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

API 规范描述请求与响应合约；下载规范不会开放账号登录。

请阅读 Cloud 的 [服务条款](https://cloud.speedle.io/zh-CN/terms) / [隐私政策](https://cloud.speedle.io/zh-CN/privacy).
