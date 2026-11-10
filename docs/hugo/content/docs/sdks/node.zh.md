+++
title = "Node.js/TypeScript SDK"
description = "添加 Speedle Node.js/TypeScript SDK，向本地 Speedle 或 Speedle Cloud 发起授权检查。"
weight = 60
aliases = ["/zh/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 npm 安装 @speedle/sdk，再向本地 Speedle 或 Speedle Cloud 调用 isAllowed 与 check。"
+++

Node.js SDK 需要 Node.js 22 或更高版本，同时提供 ESM 和 CommonJS 构建及 TypeScript 类型，没有运行时依赖。软件包：npm 上已发布的 [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk)。参考文档：[speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

## 添加依赖

新建 ES 模块包并安装：

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk
```

该命令安装 npm 上当前已发布的 `@speedle/sdk`。下面的程序调用该软件包的客户端 API。

## 调用本地 Speedle

按 [SDK 概览](/docs/sdks/#示例策略)中的说明启动 Speedle 并创建示例策略。将以下程序保存为 `main.ts`：

```typescript
import { Auth, Reason, SpeedleClient, SpeedleError } from "@speedle/sdk";

try {
  const speedle = new SpeedleClient({
    backend: "speedle",
    endpoint: "http://localhost:6734",
    auth: Auth.none(),
    serviceName: "docs",
  });
  try {
    const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
    console.log(`alice get /docs/public: allowed=${allowed}`);

    const d = await speedle.check("alice", "delete", "/docs/public");
    console.log(`alice delete /docs/public: allowed=${d.allowed} reason=${Reason[d.reason] ?? d.reason}`);
  } finally {
    await speedle.close();
  }
} catch (e) {
  if (!(e instanceof SpeedleError)) throw e;
  console.error(`Speedle error: ${e.message}`);
  process.exitCode = 1;
}
```

运行。Node.js 22.18 或更高版本可以直接运行 TypeScript 文件：

```bash
node main.ts
```

输出：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

## 调用 Speedle Cloud

> **Speedle Cloud 即将上线。**

程序从 `SPEEDLE_SERVICE` 读取服务名，从 `SPEEDLE_TOKEN` 读取该服务的令牌。`SPEEDLE_ENDPOINT` 可覆盖默认端点 `https://cloud.speedle.io`。将 `main.ts` 替换为：

```typescript
import { Auth, Reason, SpeedleClient, SpeedleError } from "@speedle/sdk";

try {
  const speedle = new SpeedleClient({
    backend: "speedle-cloud",
    endpoint: process.env.SPEEDLE_ENDPOINT ?? "https://cloud.speedle.io",
    auth: Auth.bearer(process.env.SPEEDLE_TOKEN ?? ""),
    serviceName: process.env.SPEEDLE_SERVICE ?? "",
  });
  try {
    const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
    console.log(`alice get /docs/public: allowed=${allowed}`);

    const d = await speedle.check("alice", "delete", "/docs/public");
    console.log(`alice delete /docs/public: allowed=${d.allowed} reason=${Reason[d.reason] ?? d.reason}`);
  } finally {
    await speedle.close();
  }
} catch (e) {
  if (!(e instanceof SpeedleError)) throw e;
  console.error(`Speedle error: ${e.message}`);
  process.exitCode = 1;
}
```

设置环境变量后运行。`read -rs` 不会把令牌写入 shell 历史，也不会显示在屏幕上：

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
node main.ts
```

服务中已有示例策略时，输出为：

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

令牌是服务端密钥，切勿放进浏览器、移动应用或桌面客户端。

## Speedle MCP

[`@speedle/mcp`](https://www.npmjs.com/package/@speedle/mcp) 已在 npm 发布。Speedle MCP（预览）是一个软件包，包含 `check` 与 `manage` 两种模式。用 npm 安装：

```bash
npm install @speedle/mcp
```

或直接运行 `speedle-mcp` 二进制：

```bash
npx @speedle/mcp check
```

不带模式运行 `speedle-mcp` 会打印用法，并以非零状态退出。参考文档：[speedle-mcp](https://github.com/teramoby/speedle-mcp)。

## 错误处理

授权请求返回 Deny 并不代表调用出错：`isAllowed` 解析为 `false`，`check` 解析为 `allowed` 为 false 的结果及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，Promise 以 `SpeedleError` 拒绝。示例捕获该错误，向标准错误输出 `Speedle error: <消息>`，并设置 `process.exitCode = 1`；其他错误会重新抛出。
