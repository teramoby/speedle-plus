+++
title = "Speedle Cloud 빠른 시작"
description = "Speedle Cloud 로그인, SPDL 정책 저장, API 키를 사용한 애플리케이션 인가 확인 및 플레이그라운드 테스트"
weight = 10
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
lastmod = 2026-11-12T00:00:00+08:00
+++

**Speedle Cloud는 출시 예정입니다.** 이 가이드는 프로덕션 로그인과 인증된 인가 API가 제공될 때의 사용 절차를 설명합니다. 예제는 실제 Cloud 환경에서 실행하지 않았습니다.

## GitHub 또는 Google을 통한 로그인(출시 예정)

브라우저에서 [Cloud 콘솔](https://cloud.speedle.io/ko/)을 열고 사용 가능한 공급자로 로그인합니다. 아직 사용되지 않은 인증된 이메일로 처음 로그인하는 경우 **계정 생성**을 확인합니다. 이메일이 이미 사용 중이라면 콘솔에 표시된 연결된 공급자로 로그인하십시오.

## 서비스와 정책 생성

[서비스 워크스페이스](https://cloud.speedle.io/ko/service)를 열고 `books`를 입력한 뒤 **서비스 생성**을 선택합니다. 기존 정책이 결과에 영향을 주지 않도록 새 서비스로 예제를 실행하십시오. 기존 데이터를 덮어쓰지 마십시오.

**새 정책**을 선택하고 이름을 `read-books`로 지정한 뒤 다음 SPDL을 입력합니다.

```text
grant user alice from corp get /books
```

이 정책은 ID 도메인 `corp`의 `alice`가 `/books`에 대해 `get`을 실행하도록 허용합니다. `from`이 없으면 모든 도메인의 같은 이름을 가진 사용자와 일치합니다. 신뢰할 수 있는 인증 데이터에서 도메인을 가져와 각 요청의 `idd`로 전송하십시오. [ID 도메인](/ko/docs/idd/)을 참조하십시오.

**정책 검증**을 선택한 다음 **정책 저장**을 선택합니다. 검증은 구문만 확인하며 초안을 저장하지 않습니다. 저장한 정책이 인가 API에 반영되기까지 약 60초가 걸립니다.

## API 키 생성

[API 키](https://cloud.speedle.io/ko/tokens) 페이지를 열고 `books-app`과 같은 이름을 입력한 뒤 `ads` 범위(인가 확인)를 선택하고 만료 기간을 고릅니다(콘솔 기본값은 3개월이며 UTC 시각을 표시합니다). 그런 다음 **API 키 생성**을 누릅니다. 본인 인증을 요청받으면 인증을 완료한 뒤 양식을 다시 제출합니다. 비밀 값은 한 번만 표시되므로 표시될 때 복사하십시오. 모든 키는 만료됩니다. 더 이상 필요하지 않으면 그 전에 폐기하십시오. [API 키 관리](/ko/docs/cloud/tokens/)를 참조하십시오.

키를 서버의 비밀 저장소에 보관하고 `SPEEDLE_API_KEY` 환경 변수로 전달합니다. 브라우저 코드나 소스 저장소에 넣지 마십시오.

## 애플리케이션에서 인가 확인하기

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

자세한 내용은 [인가 API](/ko/docs/cloud/authorization-api/)와 [SDK](/ko/docs/sdks/)를 참조하십시오. [Node.js/TypeScript](/ko/docs/sdks/node/), [Python](/ko/docs/sdks/python/), [Java](/ko/docs/sdks/java/), [Go](/ko/docs/sdks/go/) 등을 지원합니다.

## 플레이그라운드에서 저장된 정책 테스트하기

선택 사항으로 서비스의 **Playground**를 열고 **Is allowed**를 선택합니다. **Principals (JSON)**에 `[{"type":"user","name":"alice","idd":"corp"}]`, **Resource**에 `/books`, **Action**에 `get`, **Attributes (JSON)**에 `[]`를 입력한 뒤 **Run test**를 선택합니다.

**Is allowed**는 결정 결과를, **Diagnose**는 정책이 결정에 미친 영향을, **All granted roles**는 주체의 역할을, **All granted permissions**는 허용된 리소스와 작업을 보여 줍니다. 테스트에는 저장된 정책만 사용되며 저장하지 않은 초안은 사용되지 않습니다. 입력이나 정책을 변경하면 다시 실행하십시오. 애플리케이션은 API 키를 사용하여 [ADS 인가 API](/ko/docs/cloud/authorization-api/)를 호출합니다.

다음 단계: 프로그램으로 관리하려면 `pms:write` 범위(읽기만 하려면 `pms:read`)의 키를 따로 만들고 [PMS API 또는 spctl의 Speedle Cloud 사용법](/ko/docs/pms/policy-mgmt/#speedle-cloud에서-정책-관리)을 사용하십시오. 앞 단계의 `ads` 키로는 정책을 관리할 수 없습니다.
