+++
title = "인가 결정"
description = "서비스 접근을 위한 인가 결정을 쿼리하고 부여된 역할 및 권한 조회"
weight = 30
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pdp", "policy", "core"]
categories = ["docs"]
bref = "인가 결정을 쿼리합니다."
lastmod = 2026-11-08T19:23:52+08:00
+++

## 인가 결정이란 무엇인가

Speedle은 지정된 서비스 내에서 주체, 작업 및 리소스를 평가하며 애플리케이션이 그 결과를 적용합니다. 평가 오류나 타임아웃은 일반적인 Deny(거부)와 구분하여 처리하고 오류 발생 시 접근을 기본적으로 차단(fail closed)해야 합니다. REST, gRPC, 임베디드 Go API는 데이터 표현 방식이 다르므로 각각의 [API 계약](/docs/api/)을 따르십시오.

## 예제 실행 전 준비

[빠른 시작](/quick-start/)의 설명에 따라 저장소 루트에서 `./bin/spctl`, `./bin/speedle-pms`, `./bin/speedle-ads`를 빌드합니다. Go 1.27.4(또는 동일 릴리스 브랜치의 최신 패치 버전), Bash, curl, Python 3이 필요합니다. 각 터미널은 저장소 루트 디렉터리에서 실행하십시오. 이 페이지를 위해 새 정책 파일을 사용하고 두 서비스가 모두 중지된 후에만 재설정하십시오.

빈 정책 파일 초기화:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

터미널 1에서 PMS를 시작하고 실행 상태를 유지합니다:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

터미널 2에서 ADS를 시작하고 실행 상태를 유지합니다:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

나머지 명령은 터미널 3에서 실행합니다. PMS와 ADS는 파일을 공유하며 변경 사항은 비동기적으로 반영됩니다. 의도한 업데이트가 ADS에 도달했는지 확인한 후에 의존하십시오. `--insecure=true`는 이 루프백 예제를 위한 것입니다. 프로덕션 연결에 대해서는 [보안](/docs/security/)을 참조하십시오. 명시적인 PMS 주소를 지정하면 `~/.spctlconfig`를 수정할 필요가 없습니다. 완료되면 두 서버 터미널에서 Ctrl-C를 눌러 중지합니다.

<!-- docs-check: bookstore-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/bookstore.json
```

## 인가 결정 조회

Alan은 허용됩니다. Bob은 적용되는 인가가 없으므로 거부됩니다.

<!-- docs-check: decision-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: decision-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

성공적인 결정은 `{"allowed":true,"reason":0}` 또는 해당 거부 객체를 반환합니다. HTTP 오류는 성공적인 인가가 아닌 오류 응답을 반환합니다. 이유 코드는 다음과 같습니다:

| 코드 | 의미 |
| --- | --- |
| 0 | GRANT_POLICY_FOUND |
| 1 | DENY_POLICY_FOUND |
| 2 | SERVICE_NOT_FOUND |
| 3 | NO_APPLICABLE_POLICIES |
| 4 | ERROR_IN_EVALUATION |
| 5 | DISCOVER_MODE |
| 6 | REASON_NOT_AVAILABLE |

`DISCOVER_MODE`는 프로덕션 환경의 인가 결정이 아닙니다. `REASON_NOT_AVAILABLE`은 폴백 이유 값입니다. 이러한 숫자를 HTTP 상태 코드와 혼동하지 마십시오.

## 부여된 역할 조회

<!-- docs-check: decision-roles -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-roles
```

예상되는 역할은 `role1`과 `role2`입니다(순서는 상관없음). 리소스 범위의 역할 할당을 쿼리하는 경우 `resource`를 포함하십시오.

## 부여된 권한 조회

<!-- docs-check: decision-permissions -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-permissions
```

예상되는 권한: 리소스 `/books/HarryPotter`, 작업 `download`. 이 API는 유효한 부여 권한을 기술합니다. 특정 작업을 적용할 때 권한 목록으로 대체하지 말고 `is-allowed`로 판정을 수행하십시오.

## 속성, 진단 및 토큰 ID

REST 요청 속성은 배열 내의 형식화된 객체입니다. 예를 들어 속성 항목은 다음과 같습니다:

```json
{
  "name": "clearance",
  "type": "numeric",
  "value": 4
}
```

임베디드 Go의 속성은 `map[string]interface{}`입니다. 진단 응답 속성도 객체이며 요청 배열과는 다릅니다. 타입 및 조건에 대해서는 [SPDL](/docs/spdl/), 평가 세부 정보는 [진단](/docs/pms/diagnosis/), 구성된 토큰 검증은 [토큰 어설션](/docs/assertor/)을 참조하십시오.

정책 적용 지점(PEP)은 정책이 참조하는 모든 사용자 정의 속성을 제공해야 합니다(`request_user`나 `request_time`과 같은 기본 제공 속성은 ADS에서 제공함). PEP는 최종 사용자가 아닌 호출하는 신뢰할 수 있는 애플리케이션 또는 서비스이며 전달하는 속성 값에 책임을 집니다. 기본 제공 속성 이름으로 신뢰할 수 없는 최종 사용자 입력을 전달하지 마십시오. 요청에 존재하지 않는 속성의 조건은 일치하지 않으므로 속성이 존재하지 않는 경우 조건부 거부는 적용되지 않습니다. 거부 조건이 사용하는 모든 속성을 PEP가 확실히 제공하도록 하고 해당 요청을 테스트하십시오.
