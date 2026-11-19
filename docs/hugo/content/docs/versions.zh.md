+++
title = "版本"
description = "Speedle 发布版本与语义化版本规范"
weight = 400
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["releases", "versions"]
categories = ["docs"]
bref = "发布版本 — Speedle 遵循语义化版本规范"
lastmod = 2026-11-10T14:45:43+08:00
+++

## Speedle 发布版本

已发布版本及发行说明以 [GitHub Releases](https://github.com/teramoby/speedle-plus/releases) 为准。

- [v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0)：自定义函数失败会使决策失败；函数可声明参数与返回值类型；MySQL 存储使用 schema 2；js/wasm 构建支持宿主注册的自定义函数调用器；应用与审计日志迁移到 Go `log/slog`。
- [v1.8.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.8.0)：移除实验性的策略分发功能；源码构建需要 Go 1.27.4 或更新版本；容器映像包含公共 CA 证书包；文档新增日语、韩语和西班牙语。
- [v1.7.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.7.0)：MySQL 策略存储、离线 `spctl check`、命令行帮助列出全部选项、在 panic 和存储故障时默认拒绝，以及 Docker Hub 和 GHCR 上的单一多架构映像。

[v1.9.0](https://github.com/teramoby/speedle-plus/releases/tag/v1.9.0) 是当前嵌入式 Go 示例固定使用的版本。为确保模块依赖可复现，应指定已发布版本；安装方式见[嵌入模式](../deployment/embedded/)。

## Oracle Speedle 历史版本

0.1.0 是原项目首次开源的版本，包含策略管理、授权决策、命令行工具及集成示例。历史文档中的 0.2.0 规划不是当前 Speedle 的发布路线图。
