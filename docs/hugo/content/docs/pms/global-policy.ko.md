+++
title = "글로벌 정책"
description = "여러 서비스 간에 역할 할당을 공유하고 작업 권한은 각 서비스 내에서 개별 관리"
date = 2019-01-18T21:19:44+08:00
weight = 2
draft = false
bref = "여러 서비스에 공통된 역할 정의가 있는 경우 글로벌 정책을 사용하여 정책 관리를 단순화할 수 있습니다."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## 역할을 글로벌로 정의해야 하는 경우

일반적인 역할은 서비스 범위로 한정됩니다. 여러 서비스에서 역할 할당을 공유해야 하는 경우, 특별히 예약된 이름인 `global` 서비스에서 해당 할당을 정의하고 각 서비스 자체의 인가 정책 내에서 해당 역할을 참조합니다.

현재 지원되는 것은 글로벌 **역할 정책**뿐입니다. 글로벌 역할 할당 자체가 모든 작업 권한을 부여하는 것은 아닙니다. 각 일반 서비스는 자체 권한을 정의해야 합니다. 글로벌 인가 정책은 지원되지 않습니다.

## 예제 실행 전 준비

[빠른 시작](/quick-start/)의 설명에 따라 저장소 루트 아래의 `./bin/spctl`, `./bin/speedle-pms`, `./bin/speedle-ads`를 빌드합니다. Go 1.26.10 또는 Go 1.27.3(또는 동일 릴리스 브랜치의 최신 패치 버전), Bash, curl, Python 3이 필요합니다. 각 터미널은 저장소 루트에서 실행하십시오. 이 페이지를 위해 새 정책 파일을 사용합니다. 두 서비스가 모두 중지된 후에만 파일을 재설정하십시오.

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

## 글로벌 역할 및 로컬 권한 생성

<!-- docs-check: global-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user Emma AdminRole" --service-name global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service library
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role AdminRole borrow books" --service-name library
```

`global`은 서비스 이름과 정확히 일치해야 합니다. `library`는 해당 정책보다 먼저 생성됩니다. 둘 다 일반 서비스 API를 통해 관리됩니다.

## 결과 검증

Emma는 `AdminRole`을 통해 허용됩니다. Bob은 거부됩니다.

<!-- docs-check: global-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Emma"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: global-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

리소스 범위의 글로벌 역할 정책은 해당 리소스 범위에만 적용됩니다. 특히 ID가 [서로 다른 도메인](/docs/idd/)에 걸쳐 있는 경우 서비스 간 역할 할당을 정기적으로 검토하십시오.
