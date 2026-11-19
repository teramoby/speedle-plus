+++
title = "Node.js/TypeScript SDK"
description = "添加 Speedle Node.js/TypeScript SDK，向本地 Speedle 发起授权检查。"
weight = 60
aliases = ["/zh/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "用 npm 安装 @speedle/sdk，再向本地 Speedle 调用 isAllowed 与 check。"
lastmod = 2026-11-10T09:48:45+08:00
+++

Node.js SDK 需要 Node.js 22 或更高版本，支持过去三年内发布的所有 Node.js 版本，同时提供 ESM 和 CommonJS 构建及 TypeScript 类型，没有运行时依赖。软件包：npm 上已发布的 [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk)。参考文档：[speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node)。

示例使用的策略见 [SDK 概览](/docs/sdks/#示例策略)。

## 添加依赖

新建 ES 模块包并安装：

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@1.9.0
```

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


## Speedle MCP

[`@speedle/mcp`](https://www.npmjs.com/package/@speedle/mcp) 已在 npm 发布。Speedle MCP（预览）是一个软件包，包含 `check` 与 `manage` 两种模式。用 npm 安装：

```bash
npm install @speedle/mcp@1.9.0
```

或直接运行 `speedle-mcp` 二进制。`check` 模式从 `SPEEDLE_ENDPOINT` 读取 ADS 地址，从 `SPEEDLE_SERVICE_NAME` 读取服务名；下面的示例连接 [SDK 概览](/docs/sdks/#example-policy)中的本地 ADS：

```bash
SPEEDLE_ENDPOINT=http://127.0.0.1:6734 SPEEDLE_SERVICE_NAME=docs npx @speedle/mcp@1.9.0 check
```

未设置 `SPEEDLE_ENDPOINT` 时，命令以 `speedle-mcp: SPEEDLE_ENDPOINT is required (for example https://cloud.speedle.io or http://127.0.0.1:6734)` 退出。该命令启动 stdio MCP 服务器并等待客户端，因此成功启动时不会输出内容。不带模式运行 `speedle-mcp` 会打印用法，并以非零状态退出。参考文档：[speedle-mcp](https://github.com/teramoby/speedle-mcp)。

## 错误处理

授权请求返回 Deny 并不代表调用出错：`isAllowed` 解析为 `false`，`check` 解析为 `allowed` 为 false 的结果及原因，如上文的 `delete` 调用。在默认设置下，调用无法得出授权结果时，Promise 以 `SpeedleError` 拒绝。示例捕获该错误，向标准错误输出 `Speedle error: <消息>`，并设置 `process.exitCode = 1`；其他错误会重新抛出。
