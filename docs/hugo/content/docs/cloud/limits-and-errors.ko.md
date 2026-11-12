+++
title = "Speedle Cloud 제한과 오류"
linkTitle = "제한과 오류"
description = "계정 한도, 오류 코드, 초기화 시각과 재시도 안내."
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud는 출시 예정입니다.** cloud.speedle.io의 프로덕션 환경에서는 아직 로그인이 시작되지 않았으며 Cloud 헤더에는 **Coming soon(출시 예정)** 배지가 표시됩니다. 로그인이 닫혀 있는 동안 로그인 페이지에는 곧 출시될 예정이라는 안내가 표시됩니다.

별도 표시가 없으면 한도는 계정 전체에서 공유합니다. API 키를 바꿔도 일일 할당량은 늘지 않습니다.

## 제한

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

범위, 생성, 폐기는 [API 키](/ko/docs/cloud/tokens/)를 참조하세요.

## 일일 할당량

읽기, 쓰기, 검증, Playground, 가져오기, 내보내기, 인가 검사가 공유합니다. 정책 거부나 잘못된 업무 입력도 계산합니다. 인증, 세션 작업, `GET /api/me`는 제외합니다. 인증, 범위, 라우팅, CSRF, 본문 크기, 버스트 한도, 일일 할당량으로 거부된 요청도 제외합니다.

Cloud 자체 장애는 계정당 UTC 일일 최대 1,000회까지 할당량을 환급할 수 있습니다. 환급 확인에 실패하거나 요청 자체가 원인인 오류는 계속 과금됩니다. 카운터가 일시적으로 중단되면 인가 호출이 저속으로 계속될 수 있으며 계정당 UTC 일일 최대 10,000회의 추가 허용량을 사용합니다. 일반 할당량과 별개이고 보장되지 않으며 환급 한도와 함께 00:00 UTC에 초기화됩니다. 허용할 수 없으면 `503 quota_unavailable`입니다.

예약 후 사용하지 않은 용량 때문에 실제 사용 가능 횟수는 10,000회보다 적을 수 있습니다. `GET /api/me`는 사용량과 초기화 시각을 반환합니다. 연속 재시도나 키 교체로 한도를 우회하지 마세요.

## 오류와 응답 헤더

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

ADS는 `Cache-Control: no-store`를 사용합니다. 판정과 자격 증명을 공유 캐시에 넣지 마세요. POST를 사용하세요. 다른 메서드는 `405 method_not_allowed` 또는 그보다 앞선 비 JSON 차단 응답을 받을 수 있습니다. [인가 API](/ko/docs/cloud/authorization-api/)를 참조하세요.
