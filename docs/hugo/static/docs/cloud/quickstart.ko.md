# Speedle Cloud 문서

Speedle Cloud는 오픈 소스 Speedle 엔진 기반의 호스팅 인가 서비스입니다. 서비스, SPDL 인가 정책과 역할 정책을 관리하고 Playground에서 판정을 테스트한 뒤 서버에서 API를 호출합니다.

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다.

콘솔 사용과 API 키 생성에는 로그인이 가능해야 합니다. 무료 공개 베타는 시험, 학습, 평가용이며 SLA가 없고 프로덕션 사용은 권장하지 않습니다. 기능과 한도는 변경될 수 있습니다.

## 문서

- [시작하기](https://engine.speedle.io/ko/docs/cloud/getting-started/): 로그인, 계정, 서비스, 정책과 세션 Playground.
- [API 키](https://engine.speedle.io/ko/docs/cloud/tokens/): 계정 자격 증명, 고정 범위, 생성과 폐기, 한 번의 비밀 표시와 페이지 조회.
- [인가 API](https://engine.speedle.io/ko/docs/cloud/authorization-api/): IsAllowed 요청, 호출자 신원, Node.js 22와 전송 제약.
- [제한과 오류](https://engine.speedle.io/ko/docs/cloud/limits-and-errors/): 계정 한도, 크기, 버스트, 일일 할당량, 오류 코드와 초기화 시각.

## 다운로드

- [한국어 Markdown](https://engine.speedle.io/docs/cloud/quickstart.ko.md): Cloud 네 장을 합친 오프라인 안내.
- [OpenAPI YAML](https://cloud.speedle.io/openapi.yaml)
- [OpenAPI JSON](https://cloud.speedle.io/openapi.json)

API 명세는 요청과 응답 계약을 설명합니다. 다운로드해도 로그인이 열리지는 않습니다.

Cloud 문서를 확인하세요: [이용 약관](https://cloud.speedle.io/ko/terms) / [개인정보 처리방침](https://cloud.speedle.io/ko/privacy).

## Speedle Cloud 빠른 시작

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다. 본문에서는 공개된 사용 흐름을 설명하지만 현재 프로덕션 콘솔이 열려 있음을 의미하지는 않습니다.

#### 프로덕션 환경

계정 세션, 관리 요청 및 API 키은 `https://cloud.speedle.io`를 사용합니다. 계정, 리소스, API 키은 해당 오리진에 속합니다.

#### GitHub 또는 Google을 통한 로그인(출시 예정)

로그인이 아직 시작되지 않았더라도 로그인 페이지에는 **GitHub로 로그인**과 **Google로 로그인**이라는 두 가지 옵션이 표시됩니다. 프로덕션 환경에서 로그인이 시작되면 현재 사용 가능한 공급자를 선택하십시오. 새 ID의 인증된 이메일 주소가 아직 사용되지 않은 경우 **계정 생성**을 명시적으로 확인해야 합니다.

계정 설정에서는 연결된 ID를 확인하고 공급자 연결 또는 연결 해제를 수행할 수 있습니다. 최소 하나의 연결된 ID를 유지해야 합니다.

#### 워크스페이스 및 서비스

서비스는 정책의 네임스페이스입니다. 새 계정에서는 [서비스 워크스페이스](https://cloud.speedle.io/service)를 열고 서비스 이름에 `books`를 입력한 뒤 **서비스 생성**을 선택합니다.

#### 첫 번째 인가 정책 생성

에디터에 다음 SPDL 문을 입력합니다:

```text
grant user alice from corp get /books
```

`from corp`는 이 인가를 `corp` ID 도메인 내로 제한합니다. `corp`를 애플리케이션이 신뢰할 수 있는 인증 데이터에서 파생하는 도메인으로 대체하고, 아래 플레이그라운드 예제처럼 각 요청에서 주체의 `idd`로 동일한 값을 전송하십시오. `"from"`이 없는 인가는 모든 도메인의 `alice`라는 이름을 가진 사용자와 일치합니다. 도메인 간에 이름 충돌 가능성이 없는 단일 ID 소스를 가진 서비스의 경우에만 이 절을 생략하십시오. 규칙 및 해당 요청 필드에 대해서는 [ID 도메인](https://engine.speedle.io/docs/idd/)을 참조하십시오.

**정책 검증**을 선택하고 콘솔에 **식별 결과: 인가 정책**으로 보고되는지 확인한 다음 **정책 저장**을 선택합니다. 검증에서는 정책 타입을 감지하고 구문을 확인하지만 초안은 저장되지 않습니다. 저장은 독립적인 수동 작업입니다.

#### 플레이그라운드에서 인가 결정 테스트

플레이그라운드를 열어 정책을 테스트합니다.

요청의 주체에 프린시펄을 입력합니다:

```json
{"type":"user","name":"alice","idd":"corp"}
```

작업에 `get`, 리소스에 `/books`를 입력합니다. **평가**를 클릭하면 결과는 `allowed: true`(이유 코드 0)가 됩니다.

프린시펄의 `idd`를 `other`로 변경하거나 리소스를 `/secret`으로 변경하면 요청은 거부됩니다(`allowed: false`).

## Speedle Cloud API 키

API 키는 계정에 속하며 범위에 따라 해당 계정의 서비스에 접근합니다. 최종 사용자가 아닌 애플리케이션을 인증합니다. API 키, 세션, 연결된 신원 관리에는 로그인한 콘솔 세션이 필요합니다.

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다.

### API 키 생성

[API 키](https://cloud.speedle.io/ko/tokens)에서 앞뒤 공백을 제외한 1–128자 이름과 범위를 지정하고 **생성**을 선택합니다. 생성하려면 최근 10분 이내에 연결된 제공자로 인증해야 합니다. `403 reauth_required`이면 다시 인증한 뒤 **생성**을 다시 선택하세요. 제공자에서 돌아오는 것만으로 생성되지 않습니다.

키마다 생성 후 변경할 수 없는 범위 하나가 있습니다.

| 범위 | 접근 권한 |
| --- | --- |
| `ads` (기본값) | `/authz-check/v1/*` 인가 검사만 가능. |
| `pms:read` | `/policy-mgmt/v1`으로 계정의 서비스, 인가 정책, 역할 정책 읽기. |
| `pms:write` | `/policy-mgmt/v1`으로 계정의 서비스, 인가 정책, 역할 정책 읽기, 생성, 삭제. |

관리 범위는 ADS를 호출할 수 없으며 `ads`는 정책을 관리할 수 없습니다. 범위가 맞지 않으면 `403 insufficient_scope`입니다. 로그인이 닫혀 있을 때 관리 범위를 요청하면 `403 management_scope_disabled`입니다.

콘솔은 세션 인증 요청을 전송합니다.

```http
POST /api/tokens
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{"name":"books-app","scope":"ads"}
```

요청은 `name`과 선택적 `scope` (기본값 `ads`)만 받습니다. 서비스 선택, 만료일, 인증 증명은 보내지 마세요. 세션 쿠키와 CSRF 헤더가 필요하며 API 키로 세션을 대체할 수 없습니다.

### 비밀 값 보호

생성에 성공하면 HTTP 201을 반환합니다. 응답의 `token` 필드는 API 키 비밀 값이며 한 번만 표시됩니다.

```text
spdl_<canonical uppercase ULID id>_<43 base62 digits>
```

API 키는 폐기할 때까지 유효하며 자동 만료는 없습니다.

사용 가능한 키가 아닌 형식 설명입니다. 즉시 서버의 비밀 저장소에 보관하세요. 브라우저 저장소, 프런트엔드 코드, URL, 소스 관리, 스크린샷, 로그에 넣지 마세요. 목록에는 메타데이터만 나오며 잃어버린 값은 복구할 수 없습니다.

계정의 모든 범위를 합쳐 **활성 API 키 10개**까지 가능합니다. 추가 생성은 `409 {"error":"quota_exceeded","limit":10}`을 반환합니다. 사용하지 않는 키를 폐기하여 자리를 확보하세요. 시간이 지나도 한도는 초기화되지 않습니다. 폐기된 키는 목록에 남지만 한도에 포함되지 않습니다.

### API 키 목록

`GET /api/tokens`는 활성 키와 폐기된 키를 최신순으로 반환합니다. 기본 페이지 크기는 25이며 `limit`은 1–100입니다. 응답의 `nextCursor`를 다음 `cursor`로 보내고 `null`까지 반복합니다. `lastUsedAt`은 최선형으로 보통 시간당 최대 한 번 갱신하므로 미사용의 증거가 아닙니다.

### API 키 폐기

**폐기**를 선택하고 이름과 ID를 확인합니다. 폐기에는 최근 재인증이 필요하지 않습니다. 콘솔 요청은 다음과 같습니다.

```http
DELETE /api/tokens/<API key id>
Origin: https://cloud.speedle.io
X-SC-CSRF: 1
Content-Type: application/json

{}
```

첫 폐기는 본문 없는 HTTP 204입니다. 알 수 없거나 다른 계정의 ID, 이미 폐기된 ID는 `404 not_found`입니다. 이미 검증된 인증 정보는 최대 60초간 유효할 수 있으므로 모든 위치에 즉시 반영되지는 않습니다.

지원 작업은 **생성**과 **폐기**입니다. 값이나 범위를 바꾸려면 새 키를 만들고 호출자를 변경한 다음 이전 키를 폐기하세요. 한도에 도달했고 모든 키가 사용 중이면 폐기 전에 중단을 계획하세요. 서비스를 삭제해도 계정 API 키는 폐기되지 않습니다.

응답이 유실되면 목록을 새로 조회한 뒤 조치하세요. 이름이 같아도 잃어버린 비밀 값을 복구할 수 없습니다. [제한과 오류](https://engine.speedle.io/ko/docs/cloud/limits-and-errors/) 및 [인가 API](https://engine.speedle.io/ko/docs/cloud/authorization-api/)를 참조하세요.

## Speedle Cloud 인가 API

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다.

계정 API 키로 서버에서 인가를 확인합니다. 키를 생성하려면 로그인이 가능해야 합니다. 관리 범위는 ADS를 호출할 수 없습니다. `ads`가 아니면 `403 insufficient_scope`입니다.

### 서버에서 IsAllowed 호출

`ads` 범위 API 키를 하나의 `Authorization: Bearer` 헤더에 넣고 POST로 보냅니다.

```http
POST /authz-check/v1/is-allowed
Authorization: Bearer <API key>
Content-Type: application/json

{"subject":{"principals":[{"type":"user","name":"alice","idd":"corp"}]},"serviceName":"books","resource":"/books","action":"get","attributes":[{"name":"level","type":"numeric","value":3},{"name":"when","type":"datetime","value":"2026-11-05T03:00:00Z"}]}
```

본문은 자체 호스팅 Speedle ADS와 같은 REST 형식입니다. `attributes`는 `{name, type, value}` 배열이며 `string`, `numeric`, `bool`, RFC 3339 `datetime`을 지원합니다.

`idd`는 애플리케이션이 사용자를 인증한 신원 도메인입니다. 신뢰할 수 있는 서버 인증 정보로 설정하세요. `grant user alice from corp get /books`는 `corp`에서만 일치합니다. `from`이 없는 정책은 모든 도메인의 같은 이름과 일치합니다. `idd`가 없는 principal도 모든 도메인과 일치하므로 여러 신원 소스를 사용하면 반드시 전송하세요.

`serviceName`은 필수이며 기본값이 없습니다. 키 계정의 서비스를 선택하세요. 누락, 빈 값, 알 수 없거나 다른 계정의 이름은 `404 {"error":"service_not_found"}`입니다. 키는 한 서비스에 묶이지 않습니다. [시작 정책](https://engine.speedle.io/ko/docs/cloud/getting-started/)에서는 `books`를 지정합니다.

### 판정과 사용자 신원

성공 응답에는 불리언 `allowed`와 숫자 `reason`이 있습니다. 정책 거부도 HTTP 200이며 `allowed`가 `false`입니다. 보호 작업 전에 `allowed`를 확인하고 예외나 잘못된 응답은 거부하세요.

API 키는 계정과 API 범위를 인증하고 최종 사용자는 애플리케이션이 인증합니다. 브라우저가 제공한 principal을 신뢰하지 마세요. Cloud는 `subject.token`과 `tokenType`을 검증하지 않고 버립니다. principal만 판정에 사용하므로 이 필드를 사용자 인증의 증거로 사용하지 마세요.

Playground는 현재 저장된 정책을, ADS는 게시된 스냅샷을 사용합니다. Playground 성공이 모든 ADS 위치에 반영되었음을 보장하지 않습니다. 공개 ADS Diagnose API는 없습니다.

### Node.js 22 예제

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

### 전송과 캐시

POST를 사용하세요. 다른 메서드는 `405 method_not_allowed` 또는 그보다 앞선 비 JSON 차단 응답을 받을 수 있습니다. 비밀 API 키를 브라우저에서 직접 보내는 CORS 흐름은 없습니다. ADS는 `Cache-Control: no-store`를 사용하며 판정과 키를 공유 캐시에 넣지 마세요.

[SDK](https://engine.speedle.io/ko/docs/sdks/), [제한과 오류](https://engine.speedle.io/ko/docs/cloud/limits-and-errors/), [다운로드](https://engine.speedle.io/ko/docs/cloud/)를 참조하세요.

## Speedle Cloud 제한과 오류

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다.

별도 표시가 없으면 한도는 계정 전체에서 공유합니다. API 키를 바꿔도 일일 할당량은 늘지 않습니다.

### 제한

| 대상 | 한도 | 도달 시 처리 |
| --- | --- | --- |
| 계정 서비스 | 100개 | `409 quota_exceeded`. 미사용 서비스 삭제. 시간에 따른 초기화 없음. |
| 모든 서비스의 인가 정책과 역할 정책 합계 | 계정당 200개 | `409 quota_exceeded`. 정책 삭제. 시간에 따른 초기화 없음. |
| 정책 하나의 전체 SPDL 소스 | UTF-8 1,024바이트 | 콘솔 정책 쓰기는 `400 invalid_request`. 소스 축소. 문자 수가 아닌 바이트 수. 시간에 따른 초기화 없음. |
| 모든 범위의 활성 API 키 합계 | 계정당 10개 | `409 quota_exceeded`, `limit: 10`. 미사용 키 폐기. 시간에 따른 초기화 없음. 폐기 이력은 제외. |
| 정책 이름 / API 키 이름 | Unicode 코드 포인트 128개 / 앞뒤 공백 제외 1–128자 | `400 invalid_request`. 이름 수정. 시간에 따른 초기화 없음. |
| 과금 대상 정책 관리 및 인가 호출 | 계정당 UTC 일일 10,000회 | `429 daily_quota_exceeded`. 00:00 UTC 초기화. `Retry-After` 준수. |
| API 키 단기 버스트 | Cloud 위치별 키당 분당 약 600회 | `429 rate_limited`, `Retry-After: 60`. 기다린 뒤 재시도. 처리량 보장이나 추가 일일 할당량이 아님. |
| 일반 요청 본문 | 65,536바이트 (64 KiB) | `413 request_too_large`. 본문 축소. 시간에 따른 초기화 없음. |
| 원자적 JSON 가져오기 본문 (`POST /api/service/import`만) | 5,100,000바이트 | `413 request_too_large`. 데이터 축소. 정책과 계정 한도도 적용. 시간에 따른 초기화 없음. |
| 입력 속성 | 50개, 정규 JSON의 UTF-8 16,384바이트 | `400 SPDL-0004`. 속성 수나 크기 축소. 시간에 따른 초기화 없음. |

범위, 생성, 폐기는 [API 키](https://engine.speedle.io/ko/docs/cloud/tokens/)를 참조하세요.

### 일일 할당량

읽기, 쓰기, 검증, Playground, 가져오기, 내보내기, 인가 검사가 공유합니다. 정책 거부나 잘못된 업무 입력도 계산합니다. 인증, 세션 작업, `GET /api/me`는 제외합니다. 인증, 범위, 라우팅, CSRF, 본문 크기, 버스트 한도, 일일 할당량으로 거부된 요청도 제외합니다.

Cloud 자체 장애는 계정당 UTC 일일 최대 1,000회까지 할당량을 환급할 수 있습니다. 환급 확인에 실패하거나 요청 자체가 원인인 오류는 계속 과금됩니다. 카운터가 일시적으로 중단되면 인가 호출이 저속으로 계속될 수 있으며 계정당 UTC 일일 최대 10,000회의 추가 허용량을 사용합니다. 일반 할당량과 별개이고 보장되지 않으며 환급 한도와 함께 00:00 UTC에 초기화됩니다. 허용할 수 없으면 `503 quota_unavailable`입니다.

예약 후 사용하지 않은 용량 때문에 실제 사용 가능 횟수는 10,000회보다 적을 수 있습니다. `GET /api/me`는 사용량과 초기화 시각을 반환합니다. 연속 재시도나 키 교체로 한도를 우회하지 마세요.

### 오류와 응답 헤더

오류는 `{"error":"rate_limited"}` 같은 평면 JSON입니다. 유효한 인가 결과를 얻지 못하면 거부하세요.

| 상태와 오류 | 처리 |
| --- | --- |
| `400 invalid_request` 또는 Speedle 오류 | 요청 수정. 그대로 재시도하지 않음. |
| `401 unauthorized` | API 키나 세션 확인. 세션 쿠키만으로 ADS 호출 불가. |
| `403 reauth_required` | 키 생성 전 연결된 제공자로 재인증. |
| `403 insufficient_scope` | 필요한 범위의 키 사용. |
| `403 management_scope_disabled` | 로그인이 닫힌 동안 관리 범위 발급 불가. |
| `403 csrf_failed` | 세션, Origin, CSRF 헤더 확인. |
| `404 service_not_found` (ADS) / `404 not_found` (관리) | 자신의 계정에 있는 서비스 선택. 알 수 없거나 다른 계정 또는 폐기된 키 ID도 `404 not_found`. |
| `409 quota_exceeded` | 서비스, 정책, API 키의 자리를 확보. 시간으로 초기화되지 않음. |
| `413 request_too_large` | 본문 축소. |
| `429 rate_limited` / `429 daily_quota_exceeded` | `Retry-After`와 제한된 백오프 준수. 일일 한도는 UTC 자정 초기화. |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | 거부하고 제한된 백오프로 재시도. `Retry-After`가 있으면 준수. |

검증된 API 키 요청의 `X-RateLimit-Limit`은 600입니다. 정확한 `X-RateLimit-Remaining`은 없습니다. `X-Quota-Daily-Remaining`은 계정 잔량의 추정치이며 동시 호출과 미사용 예약으로 실제와 다를 수 있습니다. 다음 요청의 성공을 보장하지 않고 조기 거부에서는 생략됩니다.

ADS는 `Cache-Control: no-store`를 사용합니다. 판정과 자격 증명을 공유 캐시에 넣지 마세요. POST를 사용하세요. 다른 메서드는 `405 method_not_allowed` 또는 그보다 앞선 비 JSON 차단 응답을 받을 수 있습니다. [인가 API](https://engine.speedle.io/ko/docs/cloud/authorization-api/)를 참조하세요.
