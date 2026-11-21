+++
title = "Speedle Cloud"
description = "托管授权、工作区、控制台手册、API 密钥、请求限额与接入指南。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-21T00:00:00+08:00
+++

Speedle Cloud 是基于开源 Speedle 引擎的托管授权服务。你可以管理工作区、服务、SPDL 授权策略和角色策略，在策略试运行（Playground）中测试判定，并从服务端调用 API。

从 Speedle Cloud 同步策略使用 Speedle Cloud ADS，即 `speedle-cloud` 二进制和镜像。自托管 Speedle Engine 的 `speedle-ads` 二进制不再提供 `policy-source` 存储。

**Speedle Cloud 已开放公开测试版。** 本文介绍线上服务的使用流程。

使用控制台和创建 API 密钥需要登录账号。免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境。功能和限额可能调整。

## 文档导航

- [快速入门](/zh/docs/cloud/getting-started/)：登录控制台，创建服务和策略，创建 API 密钥，从应用发起授权检查。
- [控制台手册](/zh/docs/cloud/console/)：面向日常任务的 Speedle Cloud 控制台（SCC）指南，涵盖工作区、成员、服务与策略、策略试运行、API key、自定义函数、ADS 舰队、审计日志和账号设置。
- [API 密钥](/zh/docs/cloud/tokens/): 工作区级凭据、固定权限范围、必须选择有效期、创建与撤销、一次性展示和分页。
- [全局角色](/zh/docs/cloud/global-roles/)：为工作区内所有服务分配一次角色，并在各服务中授予相应权限。
- [授权 API](/zh/docs/cloud/authorization-api/): IsAllowed 请求、调用方身份、Node.js 22 和传输限制。
- [API 参考](/zh/docs/cloud/api-reference/)：使用 API key 进行授权检查和策略管理。请求从你的服务器发出；本页只读。
- [限额与错误](/zh/docs/cloud/limits-and-errors/): 工作区上限、大小限制、突发限流、每日额度、错误代码和重置时间。

## 下载

- [API 参考（Swagger）](/zh/docs/cloud/api-reference/)：在线浏览所有接口、参数和返回示例。
- [中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md)：四个 Cloud 章节合并的离线指南。
- OpenAPI 规范：[YAML](https://cloud.speedle.io/openapi.public.yaml) · [JSON](https://cloud.speedle.io/openapi.public.json)，供代码生成器和 API 客户端使用。

请阅读 Cloud 的 [服务条款](https://cloud.speedle.io/zh-CN/terms) / [隐私政策](https://cloud.speedle.io/zh-CN/privacy)。
