+++
title = "Node.js/TypeScript SDK"
description = "Add the Speedle Node.js/TypeScript SDK and check authorization against a local Speedle or Speedle Cloud."
weight = 60
aliases = ["/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Install @speedle/sdk 0.2.0 with npm when it is published, then call isAllowed and check against a local Speedle or Speedle Cloud."
+++

The Node.js SDK needs Node.js 22 or later, ships ESM and CommonJS builds with TypeScript types, and has no runtime dependencies. Package: [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk) 0.2.0 on npm, **available when published**. Reference: [speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

In a new ES module package (available when published):

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.0
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

## Call Speedle Cloud

> **Speedle Cloud is coming soon.**

The program reads the service name from `SPEEDLE_SERVICE` and the service's token from `SPEEDLE_TOKEN`. `SPEEDLE_ENDPOINT` overrides the default endpoint `https://cloud.speedle.io`. Replace `main.ts` with:

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

Set the environment and run it. `read -rs` keeps the token out of your shell history and screen:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
node main.ts
```

Output, when the service has the example policy:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

The token is a server-side secret. Never ship it to browsers, mobile apps or desktop clients.

## Error handling

A denied request is not an error: `isAllowed` resolves to `false`, and `check` resolves to a decision with `allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, the promise rejects with `SpeedleError`. The examples catch it, print `Speedle error: <message>` to standard error and set `process.exitCode = 1`; any other error is rethrown.
