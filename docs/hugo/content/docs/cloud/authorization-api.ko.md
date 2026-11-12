+++
title = "Speedle Cloud 인가 API"
linkTitle = "인가 API"
description = "계정 API 키를 사용한 IsAllowed, 호출자 신원, Node.js 22 예제."
weight = 30
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud는 출시 예정입니다.** 이 가이드는 프로덕션 로그인과 인증된 인가 API가 제공될 때의 사용 절차를 설명합니다. 예제는 실제 Cloud 환경에서 실행하지 않았습니다.

계정 API 키로 서버에서 인가를 확인합니다. 키를 생성하려면 로그인이 가능해야 합니다. 관리 범위는 ADS를 호출할 수 없습니다. `ads`가 아니면 `403 insufficient_scope`입니다.

## 서버에서 IsAllowed 호출

`ads` 범위 API 키를 하나의 `Authorization: Bearer` 헤더에 넣고 POST로 보냅니다.

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

본문은 자체 호스팅 Speedle ADS와 같은 REST 형식입니다. `attributes`는 `{name, type, value}` 배열이며 `string`, `numeric`, `bool`, RFC 3339 `datetime`을 지원합니다.

`idd`는 애플리케이션이 사용자를 인증한 신원 도메인입니다. 신뢰할 수 있는 서버 인증 정보로 설정하세요. `grant user alice from corp get /books`는 `corp`에서만 일치합니다. `from`이 없는 정책은 모든 도메인의 같은 이름과 일치합니다. `idd`가 없는 principal도 모든 도메인과 일치하므로 여러 신원 소스를 사용하면 반드시 전송하세요.

`serviceName`은 필수이며 기본값이 없습니다. 키 계정의 서비스를 선택하세요. 누락, 빈 값, 알 수 없거나 다른 계정의 이름은 `404 {"error":"service_not_found"}`입니다. 키는 한 서비스에 묶이지 않습니다. [시작 정책](/ko/docs/cloud/getting-started/)에서는 `books`를 지정합니다.

## 판정과 사용자 신원

성공 응답에는 불리언 `allowed`와 숫자 `reason`이 있습니다. 정책 거부도 HTTP 200이며 `allowed`가 `false`입니다. 보호 작업 전에 `allowed`를 확인하고 예외나 잘못된 응답은 거부하세요.

API 키는 계정과 API 범위를 인증하고 최종 사용자는 애플리케이션이 인증합니다. 브라우저가 제공한 principal을 신뢰하지 마세요. Cloud는 `subject.token`과 `tokenType`을 검증하지 않고 버립니다. principal만 판정에 사용하므로 이 필드를 사용자 인증의 증거로 사용하지 마세요.

Playground는 현재 저장된 정책을, ADS는 게시된 스냅샷을 사용합니다. Playground 성공이 모든 ADS 위치에 반영되었음을 보장하지 않습니다. 공개 ADS Diagnose API는 없습니다.

## Node.js 22 예제

```js
export async function canReadBooks(authenticatedUserName, authenticatedUserIdd) {
  if (!authenticatedUserName || !authenticatedUserIdd) {
    throw new Error("principal name and identity domain are required");
  }
  const apiKey = process.env.SPEEDLE_API_KEY;
  if (!apiKey) throw new Error("Speedle credential is not configured");
  const response = await fetch("https://cloud.speedle.io/authz-check/v1/is-allowed", {
    method: "POST",
    redirect: "error",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      subject: {
        principals: [
          { type: "user", name: authenticatedUserName, idd: authenticatedUserIdd },
        ],
      },
      serviceName: "books",
      resource: "/books",
      action: "get",
    }),
  });
  if (!response.ok) throw new Error(`Speedle HTTP ${response.status}`);
  const result = await response.json();
  if (
    typeof result !== "object" || result === null || Array.isArray(result) ||
    typeof result.allowed !== "boolean" || !Number.isInteger(result.reason) ||
    result.reason < 0 || result.reason > 6
  ) {
    throw new Error("Invalid Speedle decision");
  }
  return result.allowed;
}
```

`SPEEDLE_API_KEY`는 서버 비밀 저장소에서 주입합니다. 서버에서 인증한 사용자 이름과 신원 도메인을 전달하세요. 빈 값은 전송 전에 거부하며 예외를 허용으로 바꾸지 마세요. 예제에는 재시도 루프가 없습니다. 필요하면 `Retry-After`를 지키는 제한된 재시도를 추가하고 자격 증명 리디렉션을 막는 `redirect: "error"`를 유지하세요.

## 전송과 캐시

POST를 사용하세요. 다른 메서드는 `405 method_not_allowed` 또는 그보다 앞선 비 JSON 차단 응답을 받을 수 있습니다. 비밀 API 키를 브라우저에서 직접 보내는 CORS 흐름은 없습니다. ADS는 `Cache-Control: no-store`를 사용하며 판정과 키를 공유 캐시에 넣지 마세요.

[SDK](/ko/docs/sdks/), [제한과 오류](/ko/docs/cloud/limits-and-errors/), [다운로드](/ko/docs/cloud/)를 참조하세요.
