# Speedle Cloud 문서

Speedle Cloud는 오픈 소스 Speedle 정책 엔진을 기반으로 구축된 개발자용 호스팅형 접근 제어 서비스입니다. 자체 인가 서비스를 직접 운영할 필요 없이 SPDL 인가 정책 및 역할 정책을 관리하고, 플레이그라운드에서 결정을 테스트하며, 애플리케이션 서버에서 인가 API를 호출할 수 있도록 설계되었습니다.

**Speedle Cloud는 출시 예정입니다.** Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시되어 있으며, [cloud.speedle.io](https://cloud.speedle.io/ko/)에서의 프로덕션 로그인은 아직 시작되지 않았습니다. 여기에 설명된 콘솔 워크플로 및 인증된 인가 결정 서비스(ADS)는 공개된 서비스 계약의 프리뷰입니다. 인증된 ADS가 보류 중인 동안 토큰을 발급하더라도 서버 측 인가 호출이 활성화되지 않습니다. 선택한 단계에서의 배포 및 ID 공급자의 가용성을 확인하십시오.

무료 공개 베타는 시험 사용, 학습 및 평가를 목적으로 합니다. SLA는 제공되지 않으며 프로덕션 환경에서의 사용은 권장되지 않습니다. 또한 기능과 제한 사항은 변경될 수 있습니다.

## 문서

- [빠른 시작](https://engine.speedle.io/ko/docs/cloud/getting-started/): GitHub 또는 Google 로그인(출시 예정), 계정 관리, 서비스, SPDL 정책, 세션 관리 요청 및 4가지 세션 플레이그라운드 작업.
- [애플리케이션 토큰](https://engine.speedle.io/ko/docs/cloud/tokens/): 최근 재인증 요구 사항, 1회 한정 토큰 공개, 메타데이터 페이지네이션, 교체 및 취소.
- [인가 API 프리뷰](https://engine.speedle.io/ko/docs/cloud/authorization-api/): IsAllowed 요청 및 응답 계약, Node.js 22 클라이언트 예제, 정책 스냅샷 및 통합 제약 사항.
- [제한 및 오류](https://engine.speedle.io/ko/docs/cloud/limits-and-errors/): 계정 쿼터, 요청 제한, 오류 응답 구조, 레이트 및 쿼터 헤더, 재시도 지침.

## 다운로드

- [한국어 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ko.md): 4개의 Cloud 장을 모은 오프라인 문서.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): 저장소에 체크인된 정식 Cloud API 사양.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): 동일한 YAML 소스 바이트에서 생성.
- [제공 빌드 버전](https://cloud.speedle.io/build.json): 서비스를 제공하는 빌드를 식별(ID 공급자나 엔드포인트의 가용성을 증명하지는 않음).

사양에는 4가지 세션 플레이그라운드 작업과 인증된 ADS 계약이 포함됩니다. 사양 공개나 가이드 다운로드가 보류 중인 엔드포인트를 활성화하거나 로그인을 시작하지는 않습니다. YAML 및 생성된 JSON은 해당 봉인된 Cloud 빌드와 함께 버전 관리, 프로모션 및 복원됩니다. 한국어, 일본어, 영어, 중국어 Markdown 다운로드는 이 Engine 문서 페이지와 함께 유지 관리됩니다.

Cloud의 [이용약관](https://cloud.speedle.io/ko/terms) 및 [개인정보 처리방침](https://cloud.speedle.io/ko/privacy)을 읽어보십시오.

## Speedle Cloud 빠른 시작

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다. 본문에서는 공개된 사용 흐름을 설명하지만 현재 프로덕션 콘솔이 열려 있음을 의미하지는 않습니다.

### 프로덕션 환경

계정 세션, 관리 요청 및 애플리케이션 토큰은 `https://cloud.speedle.io`를 사용합니다. 계정, 리소스, 토큰은 해당 오리진에 속합니다.

### 첫 번째 인가 정책 생성

에디터에 다음 SPDL 문을 입력합니다:

```text
grant user alice from corp get /books
```

`from corp`는 이 인가를 `corp` ID 도메인 내로 제한합니다. `corp`를 애플리케이션이 신뢰할 수 있는 인증 데이터에서 파생하는 도메인으로 대체하고, 아래 플레이그라운드 예제처럼 각 요청에서 주체의 `idd`로 동일한 값을 전송하십시오. `"from"`이 없는 인가는 모든 도메인의 `alice`라는 이름을 가진 사용자와 일치합니다. 도메인 간에 이름 충돌 가능성이 없는 단일 ID 소스를 가진 서비스의 경우에만 이 절을 생략하십시오. 규칙 및 해당 요청 필드에 대해서는 [ID 도메인](/docs/idd/)을 참조하십시오.

### 플레이그라운드에서 인가 결정 테스트

플레이그라운드를 열어 정책을 테스트합니다.

요청의 주체에 프린시펄을 입력합니다:

```json
{"type":"user","name":"alice","idd":"corp"}
```

작업에 `get`, 리소스에 `/books`를 입력합니다. **평가**를 클릭하면 결과는 `allowed: true`(이유 코드 0)가 됩니다.

프린시펄의 `idd`를 `other`로 변경하거나 리소스를 `/secret`으로 변경하면 요청은 거부됩니다(`allowed: false`).

## Speedle Cloud 애플리케이션 토큰

애플리케이션 토큰을 사용하면 백엔드 애플리케이션에서 Speedle Cloud의 인가 API를 인증하여 호출할 수 있습니다.

### 보안 요구 사항

토큰을 생성, 교체 또는 취소하려면 콘솔에서 최근 재인증이 필요합니다. 토큰 시크릿은 생성 시 **1회 한정**으로 표시됩니다. 안전한 시크릿 저장소에 즉시 저장하십시오. 분실 시 다시 표시할 수 없으므로 토큰을 다시 생성해야 합니다.

### 토큰 범위

토큰은 특정 계정 및 워크스페이스로 범위가 지정됩니다. 프로덕션 워크로드와 개발 워크로드에 별도의 토큰을 사용하십시오.

### 토큰 교체 및 취소

정기적인 토큰 교체를 권장합니다. 새 토큰을 생성하여 애플리케이션에 배포한 후 이전 토큰을 취소하여 다운타임을 방지하십시오.

## Speedle Cloud 인가 API 프리뷰

Speedle Cloud는 빠르고 지연 시간이 짧은 접근 결정을 위한 인가 API를 제공합니다.

### 요청 계약

애플리케이션은 인가 결정 서비스(ADS)에 대해 `POST /authz-check/v1/is-allowed`를 호출합니다.

### Node.js 22 예제

다음 Node.js 예제는 전달자(Bearer) 토큰을 사용하여 인가 요청을 보내는 방법을 보여줍니다:

```javascript
import { request } from 'node:https';

export async function checkAccess(token, authenticatedUserId, authenticatedUserIdd, resource, action) {
  if (!authenticatedUserId || !authenticatedUserIdd) {
    throw new Error('principal name and identity domain are required');
  }

  const payload = JSON.stringify({
    subject: {
      principals: [
        {
          type: 'user',
          name: authenticatedUserId,
          idd: authenticatedUserIdd
        }
      ]
    },
    serviceName: 'default',
    resource: resource,
    action: action
  });

  return new Promise((resolve, reject) => {
    const req = request('https://cloud.speedle.io/authz-check/v1/is-allowed', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload)
      }
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        if (res.statusCode === 200) {
          resolve(JSON.parse(data));
        } else {
          reject(new Error(`Authorization failed: ${res.statusCode} ${data}`));
        }
      });
    });
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}
```

예제 정책 `grant user alice from corp get /books`에 대해 이 함수를 호출하면 예상대로 인가 결정이 반환됩니다.

## Speedle Cloud 제한 및 오류

### 전송 속도 제한 및 쿼터

무료 공개 베타 기간 동안 Speedle Cloud에서는 공정한 사용을 보장하기 위해 계정별 쿼터가 적용됩니다.

### 오류 응답

오류 응답은 표준 HTTP 상태 코드와 JSON 오류 객체를 반환합니다:

```json
{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "Too many requests. Please retry later."
  }
}
```

### 재시도 지침

일시적인 네트워크 오류나 `429 Too Many Requests`의 경우 지수 백오프와 지터를 동반한 재시도를 구현하십시오.
