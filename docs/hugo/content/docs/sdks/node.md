+++
title = "Node.js/TypeScript SDK"
description = "Add the Speedle Node.js/TypeScript SDK and check authorization against a local Speedle."
weight = 60
aliases = ["/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Install @speedle/sdk with npm, then call isAllowed and check against a local Speedle."
lastmod = 2026-11-10T09:48:45+08:00
+++

The Node.js SDK needs Node.js 22 or later, supports every Node.js release from the past three years, ships ESM and CommonJS builds with TypeScript types, and has no runtime dependencies. Package: [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk), published on npm. Reference: [speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

In a new ES module package:

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@1.9.0
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Save this program as `main.ts`:

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

Run it. Node.js 22.18 or later runs TypeScript files directly:

```bash
node main.ts
```

Output:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```


## Speedle MCP

[`@speedle/mcp`](https://www.npmjs.com/package/@speedle/mcp) is published on npm. Speedle MCP (launch preview) is one package with `check` and `manage` modes. Install it with npm:

```bash
npm install @speedle/mcp@1.9.0
```

Or run the `speedle-mcp` binary directly. The `check` mode reads the ADS URL from `SPEEDLE_ENDPOINT` and the service name from `SPEEDLE_SERVICE_NAME`; this example uses the local ADS from the [SDKs overview](/docs/sdks/#example-policy):

```bash
SPEEDLE_ENDPOINT=http://127.0.0.1:6734 SPEEDLE_SERVICE_NAME=docs npx @speedle/mcp@1.9.0 check
```

Without `SPEEDLE_ENDPOINT`, the command exits with `speedle-mcp: SPEEDLE_ENDPOINT is required (for example https://cloud.speedle.io or http://127.0.0.1:6734)`. The command starts a stdio MCP server and waits for a client, so a successful start prints nothing. Running `speedle-mcp` without a mode prints usage and exits with a non-zero status. Reference: [speedle-mcp](https://github.com/teramoby/speedle-mcp).

## Error handling

A denied request is not an error: `isAllowed` resolves to `false`, and `check` resolves to a decision with `allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, the promise rejects with `SpeedleError`. The examples catch it, print `Speedle error: <message>` to standard error and set `process.exitCode = 1`; any other error is rethrown.
