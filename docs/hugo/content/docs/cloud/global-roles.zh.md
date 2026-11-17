+++
title = "全局角色"
linkTitle = "全局角色"
description = "为 Speedle Cloud 账号内的所有服务分配一次角色，并在各服务中授予相应权限。"
date = 2026-11-17T00:00:00+08:00
lastmod = 2026-11-17T00:00:00+08:00
weight = 25
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "为账号内所有服务共享一次角色分配。"
+++

**Speedle Cloud 已开放公开测试版。** 本文介绍线上服务的使用流程。

角色分配通常只属于一个服务。当多个服务共享同一个角色分配时，把它定义在账号的 **全局角色** 服务中一次，再在各服务自己的授权策略中使用该角色。

全局角色只接受角色策略。全局角色分配本身不授予任何操作：每个服务仍然自己定义允许的权限。全局角色只属于一个 Cloud 账号，不会跨账号生效。

## 创建一次角色分配

在控制台的服务选择器顶部选择 **全局角色**，创建一条角色策略：

```text
grant user Emma AdminRole
```

角色 `AdminRole` 现在已在账号内的所有服务中分配给 Emma。

## 在服务中授予权限

切换到需要该权限的服务，例如 `library`，创建一条授权策略：

```text
grant role AdminRole borrow books
```

在每个需要允许该操作的服务中重复这一步。没有自己的授权策略的服务不授予任何权限，即使角色已分配给它。

## 检查结果

打开 `library` 的 Playground，以 `[{"type":"user","name":"Emma"}]` 作为主体、`books` 作为资源、`borrow` 作为动作运行测试。请求通过 `AdminRole` 被允许；来自 `Bob` 的同一请求被拒绝。

全局角色自己的 Playground 不同：直接对全局服务判定总是拒绝。全局角色在各服务的 Playground 中生效。

## 行为与限制

- 全局角色只接受角色策略。在全局服务中创建授权策略返回 `400 {"error":"global_authz_unsupported"}`。
- 全局服务计入账号的服务上限，其角色策略计入账号合并的策略上限。参见[限额与错误](/zh/docs/cloud/limits-and-errors/)。
- 控制台把该特殊服务显示为 **全局角色**；它不能手动创建、改名或删除。
- 账号的每个 Speedle Cloud ADS 实例都会收到全局角色策略，即使该实例只同步选定的服务集合。
- 修改与其他策略一样传播：通过已发布的快照约一分钟生效；Playground 直接读取当前状态，立即生效。
