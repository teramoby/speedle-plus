+++
title = "Speedle Cloud 제한과 오류"
linkTitle = "제한과 오류"
description = "워크스페이스 한도, 오류 코드, 초기화 시각과 재시도 안내."
weight = 40
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-21T00:00:00+08:00
+++

**Speedle Cloud는 공개 베타로 제공 중입니다.** 이 가이드는 실제 서비스에서의 사용 절차를 설명합니다.

별도 표시가 없으면 한도는 워크스페이스 전체에서 공유합니다. 같은 워크스페이스에서 API 키를 바꿔도 일일 할당량은 늘지 않습니다.

## 제한

| 대상 | 한도 | 도달 시 처리 |
| --- | --- | --- |
| 워크스페이스 서비스 | 100개 | `409 quota_exceeded`. 미사용 서비스 삭제. 시간에 따른 초기화 없음. |
| 모든 서비스의 인가 정책과 역할 정책 합계 | 워크스페이스당 200개 | `409 quota_exceeded`. 정책 삭제. 시간에 따른 초기화 없음. |
| 정책 하나의 전체 SPDL 소스 | UTF-8 1,024바이트 | 콘솔 정책 쓰기는 `400 invalid_request`. 소스 축소. 문자 수가 아닌 바이트 수. 시간에 따른 초기화 없음. |
| 모든 범위의 활성 API 키 합계 | 워크스페이스당 10개 | `409 quota_exceeded`, `limit: 10`. 미사용 키를 폐기하거나 만료를 기다립니다. 한도는 주기로 초기화되지 않습니다. 폐기 및 만료된 키는 제외. |
| 정책 이름 / API 키 이름 | Unicode 코드 포인트 128개 / 앞뒤 공백 제외 1–128자 | `400 invalid_request`. 이름 수정. 시간에 따른 초기화 없음. |
| 과금 대상 정책 관리 및 인가 호출 | 워크스페이스당 UTC 일일 10,000회 | `429 daily_quota_exceeded`. 00:00 UTC 초기화. `Retry-After` 준수. |
| API 키 단기 버스트 | Cloud 위치별 키당 분당 약 600회 | `429 rate_limited`, `Retry-After: 60`. 기다린 뒤 재시도. 처리량 보장이나 추가 일일 할당량이 아님. |
| 일반 요청 본문 | 65,536바이트 (64 KiB) | `413 request_too_large`. 본문 축소. 시간에 따른 초기화 없음. |
| 원자적 JSON 가져오기 본문 (`POST /api/workspaces/<workspaceId>/services/<serviceId>/import`만) | 5,100,000바이트 | `413 request_too_large`. 데이터 축소. 정책과 워크스페이스 한도도 적용. 시간에 따른 초기화 없음. |
| 입력 속성 | 50개, 정규 JSON의 UTF-8 16,384바이트 | `400 SPDL-0004`. 속성 수나 크기 축소. 시간에 따른 초기화 없음. |

범위, 만료 기간, 생성, 폐기는 [API 키](/ko/docs/cloud/tokens/)를 참조하세요.

## 일일 할당량

읽기, 쓰기, 검증, Playground, 가져오기, 내보내기, 인가 검사가 공유합니다. 정책 거부나 잘못된 업무 입력도 계산합니다. 인증, 세션 작업, `GET /api/me`, `GET /api/workspaces/<workspaceId>/usage`는 제외합니다. 인증, 범위, 라우팅, CSRF, 본문 크기, 버스트 한도, 일일 할당량으로 거부된 요청도 제외합니다.

Cloud 자체 장애는 워크스페이스당 UTC 일일 최대 1,000회까지 할당량을 환급할 수 있습니다. 환급 확인에 실패하거나 요청 자체가 원인인 오류는 계속 과금됩니다. 카운터가 일시적으로 중단되면 인가 호출이 저속으로 계속될 수 있으며 워크스페이스당 UTC 일일 최대 10,000회의 추가 허용량을 사용합니다. 일반 할당량과 별개이고 보장되지 않으며 환급 한도와 함께 00:00 UTC에 초기화됩니다. 허용할 수 없으면 `503 quota_unavailable`입니다.

예약 후 사용하지 않은 용량 때문에 실제 사용 가능 횟수는 10,000회보다 적을 수 있습니다. `GET /api/workspaces/<workspaceId>/usage`는 워크스페이스 사용량과 초기화 시각을 반환합니다(`GET /api/me`는 개인 워크스페이스를 반환). 연속 재시도나 키 교체로 한도를 우회하지 마세요.

## 오류와 응답 헤더

오류는 `{"error":"rate_limited"}` 같은 평면 JSON입니다. 유효한 인가 결과를 얻지 못하면 거부하세요.

| 상태와 오류 | 처리 |
| --- | --- |
| `400 invalid_request` 또는 Speedle 오류 | 요청 수정. 그대로 재시도하지 않음. |
| `401 unauthorized` | API 키나 세션 확인. 세션 쿠키만으로 ADS 호출 불가. |
| `401 api_key_expired` | API 키가 만료 시각에 도달했거나 지났습니다. 새 키를 만드세요. 이 키를 다시 시도해도 성공하지 않습니다. 만료가 필수가 되기 전에 만든 키에도 만료 시각이 있습니다. 언제 만료되는지는 API 키 페이지에 있습니다. `expiresAt`을 확인하고 그 전에 새 키를 만드세요. |
| `403 reauth_required` | 키 생성 전 연결된 제공자로 재인증. |
| `403 insufficient_scope` | 필요한 범위의 키 사용. |
| `403 csrf_failed` | 세션, Origin, CSRF 헤더 확인. |
| `404 service_not_found` (ADS) / `404 not_found` (관리) | 자신의 계정에 있는 서비스 선택. 알 수 없거나 다른 계정 또는 폐기된 키 ID도 `404 not_found`. |
| `409 quota_exceeded` | 쓰지 않는 서비스나 정책을 삭제해 자리를 확보합니다. 시간으로 이 두 한도는 초기화되지 않습니다. API 키 자리는 키를 폐기하거나 만료를 기다려 확보합니다. |
| `409 revision_exhausted` | 서비스의 리비전이 상한에 도달했지만 계속 읽고 내보낼 수 있습니다. [복구 절차](#revision-exhausted)를 따르세요. |
| `413 request_too_large` | 본문 축소. |
| `429 rate_limited` / `429 daily_quota_exceeded` | `Retry-After`와 제한된 백오프 준수. 일일 한도는 UTC 자정 초기화. |
| `503 quota_unavailable` / `503 snapshot_unavailable` / `500 internal_error` | 거부하고 제한된 백오프로 재시도. `Retry-After`가 있으면 준수. |

검증된 API 키 요청의 `X-RateLimit-Limit`은 600입니다. 정확한 `X-RateLimit-Remaining`은 없습니다. `X-Quota-Daily-Remaining`은 계정 잔량의 추정치이며 동시 호출과 미사용 예약으로 실제와 다를 수 있습니다. 다음 요청의 성공을 보장하지 않고 조기 거부에서는 생략됩니다.

ADS는 `Cache-Control: no-store`를 사용합니다. 판정과 자격 증명을 공유 캐시에 넣지 마세요. POST를 사용하세요. 다른 메서드는 `405 method_not_allowed` 또는 그보다 앞선 비 JSON 차단 응답을 받을 수 있습니다. [인가 API](/ko/docs/cloud/authorization-api/)를 참조하세요.

<a id="revision-exhausted"></a>
## `409 revision_exhausted`

이 서비스의 리비전이 상한에 도달했습니다. 이 서비스는 계속 읽고 내보낼 수 있습니다.

복구 절차:

1. 정책을 내보내세요.
2. 새 서비스를 만드세요.
3. 내보낸 파일을 새 서비스로 가져오세요.
4. 호출자를 새 서비스 이름으로 전환하세요.
5. 이후 필요하면 기존 서비스를 삭제하세요.

계정의 서비스 및 정책 한도는 이 절차에도 적용됩니다. 2단계와 3단계 전에 여유 서비스 자리 하나와 가져오기에 충분한 여유 정책 자리가 있는지 확인하세요. 필요하면 사용하지 않는 서비스나 정책을 삭제하세요. 다른 자리를 확보할 수 없으면 내보낸 파일을 확인한 후 기존 서비스를 삭제하고 2–4단계를 수행하세요. 새 서비스가 준비되기 전에 기존 서비스를 삭제하면 해당 호출자의 접근이 중단됩니다.
