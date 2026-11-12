# Speedle Cloud 문서

Speedle Cloud는 오픈 소스 Speedle 정책 엔진을 기반으로 구축된 개발자용 호스팅형 접근 제어 서비스입니다. 자체 인가 서비스를 직접 운영할 필요 없이 SPDL 인가 정책 및 역할 정책을 관리하고, 플레이그라운드에서 결정을 테스트하며, 애플리케이션 서버에서 인가 API를 호출할 수 있도록 설계되었습니다.

**Speedle Cloud는 출시 예정입니다.** Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시되어 있으며, [cloud.speedle.io](https://cloud.speedle.io/ko/)에서의 프로덕션 로그인은 아직 시작되지 않았습니다. 여기에 설명된 콘솔 워크플로 및 인증된 인가 결정 서비스(ADS)는 공개된 서비스 계약의 프리뷰입니다. 인증된 ADS가 보류 중인 동안 토큰을 발급하더라도 서버 측 인가 호출이 활성화되지 않습니다. 선택한 단계에서의 배포 및 ID 공급자의 가용성을 확인하십시오.

무료 공개 베타는 시험 사용, 학습 및 평가를 목적으로 합니다. SLA는 제공되지 않으며 프로덕션 환경에서의 사용은 권장되지 않습니다. 또한 기능과 제한 사항은 변경될 수 있습니다.

## 문서

- [빠른 시작](https://engine.speedle.io/ko/docs/cloud/getting-started/): 로그인한 뒤 콘솔에서 서비스와 정책, API 키를 생성하고 애플리케이션에서 인가를 확인합니다.
- [애플리케이션 토큰](https://engine.speedle.io/ko/docs/cloud/tokens/): 최근 재인증 요구 사항, 1회 한정 토큰 공개, 메타데이터 페이지네이션, 교체 및 취소.
- [인가 API 프리뷰](https://engine.speedle.io/ko/docs/cloud/authorization-api/): IsAllowed 요청 및 응답 계약, Node.js 22 클라이언트 예제, 정책 스냅샷 및 통합 제약 사항.
- [제한 및 오류](https://engine.speedle.io/ko/docs/cloud/limits-and-errors/): 계정 쿼터, 요청 제한, 오류 응답 구조, 레이트 및 쿼터 헤더, 재시도 지침.

## 다운로드

- [한국어 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ko.md): Cloud 장을 모은 오프라인 문서.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml): 저장소에 체크인된 정식 Cloud API 사양.
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json): 동일한 YAML 소스 바이트에서 생성.
- [제공 빌드 버전](https://cloud.speedle.io/build.json): 서비스를 제공하는 빌드를 식별(ID 공급자나 엔드포인트의 가용성을 증명하지는 않음).

사양에는 4가지 세션 플레이그라운드 작업과 인증된 ADS 계약이 포함됩니다. 사양 공개나 가이드 다운로드가 보류 중인 엔드포인트를 활성화하거나 로그인을 시작하지는 않습니다. YAML 및 생성된 JSON은 해당 봉인된 Cloud 빌드와 함께 버전 관리, 프로모션 및 복원됩니다. 한국어, 일본어, 영어, 중국어 Markdown 다운로드는 이 Engine 문서 페이지와 함께 유지 관리됩니다.

Cloud의 [이용약관](https://cloud.speedle.io/ko/terms) 및 [개인정보 처리방침](https://cloud.speedle.io/ko/privacy)을 읽어보십시오.

## Speedle Cloud 빠른 시작

**Speedle Cloud는 출시 예정입니다.** 이 가이드는 프로덕션 로그인과 인증된 인가 API가 제공될 때의 사용 절차를 설명합니다. 예제는 실제 Cloud 환경에서 실행하지 않았습니다.

### GitHub 또는 Google을 통한 로그인(출시 예정)

브라우저에서 [Cloud 콘솔](https://cloud.speedle.io/ko/)을 열고 사용 가능한 공급자로 로그인합니다. 아직 사용되지 않은 인증된 이메일로 처음 로그인하는 경우 **계정 생성**을 확인합니다. 이메일이 이미 사용 중이라면 콘솔에 표시된 연결된 공급자로 로그인하십시오.

### 서비스와 정책 생성

[서비스 워크스페이스](https://cloud.speedle.io/ko/service)를 열고 `books`를 입력한 뒤 **서비스 생성**을 선택합니다. 기존 정책이 결과에 영향을 주지 않도록 새 서비스로 예제를 실행하십시오. 기존 데이터를 덮어쓰지 마십시오.

**새 정책**을 선택하고 이름을 `read-books`로 지정한 뒤 다음 SPDL을 입력합니다.

```text
grant user alice from corp get /books
```

이 정책은 ID 도메인 `corp`의 `alice`가 `/books`에 대해 `get`을 실행하도록 허용합니다. `from`이 없으면 모든 도메인의 같은 이름을 가진 사용자와 일치합니다. 신뢰할 수 있는 인증 데이터에서 도메인을 가져와 각 요청의 `idd`로 전송하십시오. [ID 도메인](https://engine.speedle.io/ko/docs/idd/)을 참조하십시오.

**정책 검증**을 선택한 다음 **정책 저장**을 선택합니다. 검증은 구문만 확인하며 초안을 저장하지 않습니다. 저장한 정책이 인가 API에 반영되기까지 약 60초가 걸립니다.

### API 키 생성

[API 키](https://cloud.speedle.io/ko/tokens) 페이지를 열고 `books-app`과 같은 이름을 입력한 뒤 `ads` 범위(인가 확인)를 선택하고 **API 키 생성**을 누릅니다. 본인 인증을 요청받으면 인증을 완료한 뒤 양식을 다시 제출합니다. 비밀 값은 한 번만 표시되므로 표시될 때 복사하십시오. 현재 만료 기간은 선택할 수 없습니다. 더 이상 필요하지 않은 키는 폐기하십시오. [API 키 관리](https://engine.speedle.io/ko/docs/cloud/tokens/)를 참조하십시오.

키를 서버의 비밀 저장소에 보관하고 `SPEEDLE_API_KEY` 환경 변수로 전달합니다. 브라우저 코드나 소스 저장소에 넣지 마십시오.

### 애플리케이션에서 인가 확인하기

애플리케이션 서버에서 다음 `isAllowed` 요청을 보내 저장한 정책을 확인합니다. 이 정책에는 속성이 필요하지 않으므로 `attributes`는 빈 배열입니다.

```bash
curl --fail-with-body --silent --show-error --max-time 10 \
  'https://cloud.speedle.io/authz-check/v1/is-allowed' \
  -H "Authorization: Bearer $SPEEDLE_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[]}'
```

예제 정책만 저장되어 있다면 예상 응답은 다음과 같습니다.

```json
{"allowed":true,"reason":0}
```

`action`을 `delete`로 바꾸어 다시 전송합니다. 이 작업을 허용하는 정책이 없으므로 예상 응답은 다음과 같습니다.

```json
{"allowed":false,"reason":3}
```

두 결정 모두 HTTP 200을 반환합니다. `allowed`가 `true`일 때만 보호된 작업을 실행하십시오. 애플리케이션은 결과를 적용하고 오류, 잘못된 응답 또는 시간 초과가 발생하면 접근을 거부해야 합니다.

자세한 내용은 [인가 API](https://engine.speedle.io/ko/docs/cloud/authorization-api/)와 [SDK](https://engine.speedle.io/ko/docs/sdks/)를 참조하십시오. [Node.js/TypeScript](https://engine.speedle.io/ko/docs/sdks/node/), [Python](https://engine.speedle.io/ko/docs/sdks/python/), [Java](https://engine.speedle.io/ko/docs/sdks/java/), [Go](https://engine.speedle.io/ko/docs/sdks/go/) 등을 지원합니다.

### 플레이그라운드에서 저장된 정책 테스트하기

선택 사항으로 서비스의 **Playground**를 열고 **Is allowed**를 선택합니다. **Principals (JSON)**에 `[{"type":"user","name":"alice","idd":"corp"}]`, **Resource**에 `/books`, **Action**에 `get`, **Attributes (JSON)**에 `[]`를 입력한 뒤 **Run test**를 선택합니다.

**Is allowed**는 결정 결과를, **Diagnose**는 정책이 결정에 미친 영향을, **All granted roles**는 주체의 역할을, **All granted permissions**는 허용된 리소스와 작업을 보여 줍니다. 테스트에는 저장된 정책만 사용되며 저장하지 않은 초안은 사용되지 않습니다. 입력이나 정책을 변경하면 다시 실행하십시오. 애플리케이션은 API 키를 사용하여 [ADS 인가 API](https://engine.speedle.io/ko/docs/cloud/authorization-api/)를 호출합니다.

다음 단계: 프로그램으로 관리하려면 [API 키](https://engine.speedle.io/ko/docs/cloud/tokens/)와 [PMS API 또는 spctl의 Speedle Cloud 사용법](https://engine.speedle.io/ko/docs/pms/policy-mgmt/#speedle-cloud에서-정책-관리)을 참조하십시오.

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

오류는 평면 JSON으로 반환합니다. `429 {"error":"rate_limited"}`의 경우 `Retry-After`(이 버스트 제한에서는 60초)를 따르고, 상한이 있는 백오프로 재시도하십시오.

### 재시도 지침

일시적인 네트워크 오류나 `429 Too Many Requests`의 경우 지수 백오프와 지터를 동반한 재시도를 구현하십시오.
