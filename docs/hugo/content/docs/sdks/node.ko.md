+++
title = "Node.js/TypeScript SDK"
description = "Speedle Node.js/TypeScript SDK를 추가하고 로컬 Speedle 또는 Speedle Cloud에 대해 인가 확인 수행"
weight = 60
aliases = ["/ko/docs/api/sdk-node/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "npm으로 @speedle/sdk를 설치하고 로컬 Speedle 또는 Speedle Cloud에 대해 isAllowed 및 check를 호출합니다."
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

## Speedle Cloud 호출

> **Speedle Cloud는 출시 예정입니다.**

프로그램은 `SPEEDLE_SERVICE`에서 서비스 이름을, `SPEEDLE_TOKEN`에서 서비스 토큰을 읽습니다. `SPEEDLE_ENDPOINT`는 기본 엔드포인트 `https://cloud.speedle.io`를 재정의합니다. `main.ts`를 다음과 같이 대체합니다:

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

환경 변수를 설정하고 실행합니다. `read -rs`를 사용하면 토큰이 셸 기록이나 화면에 노출되지 않습니다:

```bash
export SPEEDLE_SERVICE=docs
read -rs SPEEDLE_TOKEN && export SPEEDLE_TOKEN
node main.ts
```

서비스에 예제 정책이 있는 경우 출력:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NoApplicablePolicies
```

토큰은 서버 측 비밀 값입니다. 브라우저, 모바일 앱 또는 데스크톱 클라이언트에 절대 배포하지 마십시오.

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
