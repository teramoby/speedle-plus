+++
title = "Speedle Cloud"
description = "Speedle Cloud 文档中心：入门、账号与策略管理、应用令牌、授权 API 预览、限额、错误和下载。"
weight = 306
draft = false
toc = false
linksection = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T20:00:00+08:00
+++

Speedle Cloud 是基于开源 Speedle 策略引擎的托管访问控制服务，面向开发者。你可以用它管理 SPDL 授权策略与角色策略、在 Playground 中测试授权判定，再从应用服务端调用授权 API，无需自行部署和运维授权服务。

**Speedle Cloud 即将上线。** Cloud 页眉显示 **Coming soon（即将上线）** 徽章，[cloud.speedle.io](https://cloud.speedle.io/zh-CN/) 的生产环境尚未开放登录。本文介绍的控制台流程和带认证的授权决策服务（ADS）均为已发布服务合约的预览。带认证的 ADS 尚未提供时，签发令牌并不会启用服务端授权调用。所选环境的部署情况和登录供应商可用性仍需分别确认。

免费公开测试版用于试用、学习和评估，不提供 SLA，不建议用于生产环境，功能和限额可能调整。

## 文档导航

- [快速入门](/zh/docs/cloud/getting-started/)：使用 GitHub 或 Google 登录（即将开放）、账号管理、service、SPDL 策略、会话管理请求，以及四种会话 Playground 操作。
- [应用令牌](/zh/docs/cloud/tokens/)：最近认证要求、令牌的一次性展示、元数据分页、轮换和撤销。
- [授权 API 预览](/zh/docs/cloud/authorization-api/)：IsAllowed 请求与响应合约、Node.js 22 示例、策略快照和接入限制。
- [API 参考](/zh/docs/cloud/api-reference/)：使用 API key 进行授权检查和策略管理。请求从你的服务器发出；本页只读。
- [限额与错误](/zh/docs/cloud/limits-and-errors/)：账号额度、请求限制、错误响应、限流与额度响应头，以及重试建议。

## 下载

- [简体中文 Markdown](https://engine.speedle.io/docs/cloud/quickstart.zh-CN.md)：汇集全部四章 Cloud 文档，便于离线阅读。
- [API 参考](/zh/docs/cloud/api-reference/)：在本站阅读。下面仍保留原始文件。
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)：Cloud API 的权威仓库规范。
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)：由同一份 YAML 原文生成。
- [当前构建版本](https://cloud.speedle.io/build.json)：用于识别正在提供服务的构建，不证明登录供应商或接口当前可用。

规范包含四种会话 Playground 操作和带认证的 ADS 合约。发布规范或下载指南并不会启用尚未提供的接口，也不会开放登录。Cloud 的 YAML 与生成的 JSON 随对应的封存构建一起记录版本、晋升或恢复；中英文 Markdown 下载则随这些 Engine 文档页面一起维护。

请阅读 Cloud 的[服务条款](https://cloud.speedle.io/zh-CN/terms)和[隐私政策](https://cloud.speedle.io/zh-CN/privacy)。
