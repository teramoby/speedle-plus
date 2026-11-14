+++
title = "인가 진단"
description = "인가 평가를 진단하고 결정 이유, 유효한 역할 및 평가된 정책 확인"
date = 2019-01-18T15:46:59+08:00
weight = 3
draft = false
bref = "인가 평가 프로세스를 진단합니다. 사용자의 리소스 작업이 허용되지 않을 때 그 이유나 어떤 정책이 작업을 거부했는지 조사하는 데 도움이 됩니다."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## 인가 진단의 역할

`Diagnose`는 인가 평가를 상세히 설명합니다. 결정 결과, 이유, 유효한 역할, 속성 및 검증된 정책이 포함됩니다. `IsAllowed`와 동일한 요청 컨텍스트를 수신합니다. REST에서는 `/authz-check/v1/diagnose`를 사용하고 Go/gRPC에서는 `Diagnose`를 사용합니다.

진단 기능은 정책 정의와 요청 컨텍스트를 그대로 노출합니다. [API 보안 제어](/docs/security/)를 사용하여 신뢰할 수 있는 운영자에게만 접근을 제한하십시오. 공개 디버깅 엔드포인트로 사용해서는 안 됩니다.

## 예제 실행 전 준비

[빠른 시작](/quick-start/)의 설명에 따라 저장소 루트 아래의 `./bin/spctl`, `./bin/speedle-pms`, `./bin/speedle-ads`를 빌드합니다. Go 1.27.4(또는 동일 릴리스 브랜치의 최신 패치 버전), Bash, curl, Python 3이 필요합니다. 각 터미널은 저장소 루트에서 실행하십시오. 이 페이지를 위해 새 정책 파일을 사용합니다. 두 서비스가 모두 중지된 후에만 파일을 재설정하십시오.

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

나머지 명령은 세 번째 터미널에서 실행합니다. PMS와 ADS는 동일한 파일을 공유하며 변경 사항은 감시(watch)를 통해 비동기적으로 전파됩니다. 새 정책에 의존하기 전에 업데이트가 반영되었는지 확인하십시오. `--insecure=true`는 로컬 루프백 전용입니다. 프로덕션 환경 연결에 대해서는 [보안](/docs/security/)을 참조하십시오. 명시적인 PMS 주소 지정을 사용하므로 `~/.spctlconfig`를 수정할 필요가 없습니다. 완료되면 두 서버 터미널에서 Ctrl-C를 눌러 종료합니다.

## 예제 정책 생성

이 [픽스처](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/diagnosis.json)는 `^/api/v1/example/.*$` 하위의 `get`을 허용하지만, `user1`에 대해 `/api/v1/example/res1`을 명시적으로 거부합니다.

<!-- docs-check: diagnosis-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/diagnosis.json
```

## 인가 결정과 진단 결과 비교

<!-- docs-check: diagnosis-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

예상 결과: `allowed=false`, `reason=1`(명시적 거부).

<!-- docs-check: diagnose -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/diagnose
```

진단 응답에서 `allowed`는 부울 값, `reason`은 `DENY_POLICY_FOUND`와 같은 문자열, `attributes`는 객체로 표현됩니다. `request_month`는 숫자로 된 월입니다. `requestContext.subject.principals`는 배열입니다. 인가 정책에는 OR-of-AND 주체 그룹을 나타내는 2차원 `principals` 배열이 포함됩니다.

| 필드 | 의미 |
| --- | --- |
| `requestContext` | 평가에 사용된 요청. REST 속성은 여기서 형식화된 배열로 유지됩니다 |
| `attributes` | 사용 가능한 요청 속성 및 기본 내장 속성(이름을 키로 사용) |
| `grantedRoles` | 유효한 역할(있는 경우) |
| `rolePolicies`, `policies` | 평가된 역할 및 인가 정책의 상세 정보(있는 경우) |
| `condition` | 표현식과 결과. 무조건 정책의 경우 비어 있을 수 있습니다 |

정책 상태 값은 `takeEffect`, `conditionFailed`, `ignored`입니다. `ignored`는 최종 결과가 이미 확정되어 해당 정책의 평가가 불필요했음을 의미합니다. 순서는 달라질 수 있으므로 이전 응답과 완벽히 일치시키려 하지 말고 ID와 상태를 확인하십시오. 전체 응답 계약에 대해서는 [결정 스키마](/docs/api/decision_api/)를 참조하십시오.
