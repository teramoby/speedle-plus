+++
title = "Node.js/TypeScript SDK"
description = "Speedle Node.js/TypeScript SDK를 추가하고 로컬 Speedle에 대해 인가 확인 수행"
weight = 60
aliases = ["/ko/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "npm으로 @speedle/sdk를 설치하고 로컬 Speedle에 대해 isAllowed 및 check를 호출합니다."
lastmod = 2026-11-10T09:48:45+08:00
+++

Node.js SDK는 Node.js 22 이상이 필요하며 TypeScript 타입 정의를 포함한 ESM 및 CommonJS 빌드를 제공하고 런타임 의존성이 없습니다. 패키지: npm의 [`@speedle/sdk`](https://www.npmjs.com/package/@speedle/sdk). 참조 문서: [speedle-sdk/node](https://github.com/teramoby/speedle-sdk/tree/main/node).

예제에서는 [SDK 개요](/docs/sdks/#example-policy)에서 설명한 정책을 확인합니다.

## 의존성 추가

새 ES 모듈 패키지를 생성하고 설치합니다:

```bash
mkdir speedle-example
cd speedle-example
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
```

## 로컬 Speedle 호출

[SDK 개요](/docs/sdks/#example-policy)의 설명에 따라 Speedle을 시작하고 예제 정책을 생성합니다. 이 프로그램을 `main.ts`로 저장합니다:

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

실행합니다. Node.js 22.18 이상에서는 TypeScript 파일을 직접 실행할 수 있습니다:

```bash
node main.ts
```

출력:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```


## Speedle MCP

[`@speedle/mcp`](https://www.npmjs.com/package/@speedle/mcp)는 npm에 공개되어 있습니다. Speedle MCP(출시 프리뷰)는 `check` 및 `manage` 모드를 제공하는 단일 패키지입니다. npm으로 설치합니다:

```bash
npm install @speedle/mcp@0.2.1
```

또는 `speedle-mcp` 바이너리를 직접 실행합니다:

```bash
npx @speedle/mcp@0.2.1 check
```

모드를 지정하지 않고 `speedle-mcp`를 실행하면 사용법을 출력하고 0이 아닌 상태 코드로 종료합니다. 참조 문서: [speedle-mcp](https://github.com/teramoby/speedle-mcp).

## 오류 처리

거부된 요청은 오류가 아닙니다. 위의 `delete` 호출과 같이 `isAllowed`는 `false`로 해결되고, `check`는 `allowed`가 false이며 이유를 포함하는 결정으로 해결됩니다. 기본 구성에서 호출이 결정을 반환할 수 없는 경우 Promise는 `SpeedleError`로 거부됩니다. 예제에서는 이를 포착하여 표준 오류로 `Speedle error: <message>`를 출력하고 `process.exitCode = 1`을 설정합니다. 기타 오류는 다시 발생합니다.
