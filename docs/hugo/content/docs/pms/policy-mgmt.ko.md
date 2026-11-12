+++
title = "정책 관리"
description = "서비스, 인가 정책, 역할 정책 및 커스텀 함수의 전체 라이프사이클 관리"
weight = 1
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pms", "policy", "core"]
categories = ["docs"]
bref = "정책 관리 기본 사항"
lastmod = 2026-11-09T07:57:54+08:00
+++

## Speedle 정책이란 무엇인가

정책 관리 서비스(PMS)는 서비스, 인가 정책, 역할 정책 및 커스텀 함수를 관리합니다. 정책 저장소는 등록된 [저장소 백엔드](/docs/store/)를 사용하여 이러한 객체를 영구 보존합니다.

<img src="/img/speedle/policystore.png" width="1942" height="540" loading="lazy"/>

## 서비스와 인가 정책

서비스는 해당 정책의 범위를 제한합니다. 인가 정책은 주체에게 리소스에 대한 작업을 허용하거나 거부하며, 선택적으로 조건을 지정할 수 있습니다. [글로벌 역할 정책](/docs/pms/global-policy/)을 제외하고 특정 서비스의 정책이 다른 서비스에 자동으로 적용되지 않습니다.

<img src="/img/speedle/authzpolicy.png" width="1770" height="444" loading="lazy"/>

```text
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
```

이 정규식은 `c1/default/core/pods/` 하위의 리소스와 일치합니다. `c1/default/core/podssecret`이나 `c1` 앞에 추가 접두사가 있는 경로와는 일치하지 않습니다. `expr:`는 셸 와일드카드가 아닌 정규식을 사용합니다. 임의의 문자에는 `.*`를 사용하고, 전체 식별자와 일치해야 하는 경우 `^`/`$`를 사용하십시오.

## 역할 정책

<img src="/img/speedle/rolepolicy.png" width="1598" height="436" loading="lazy"/>

```text
grant user alan manager on res1
grant role manager update res1
```

첫 번째 문은 특정 리소스에 대해 Alan에게 역할을 할당하고, 두 번째 문은 해당 역할에 작업 권한을 부여합니다. 역할을 할당하는 것만으로는 권한이 발생하지 않습니다. 해당하는 명시적 거부(deny)는 허용(grant)보다 우선합니다.

## 주체와 조건

주체에는 사용자, 그룹, 엔티티 또는 역할을 지정할 수 있습니다. 엔티티는 서비스와 같은 사람이 아닌 식별자를 나타냅니다. 통합 코드는 이러한 식별자를 신뢰할 수 있는 소스에서 가져와야 합니다.

<img src="/img/speedle/principal.png" width="1769" height="505" loading="lazy"/>

AND 주체 그룹은 목록의 모든 주체가 일치해야 함을 요구합니다. 개별 그룹 간은 OR 조건이 됩니다. 예를 들어 다음 예제에서는 두 역할이 모두 적용되어야 합니다:

<img src="/img/speedle/andprincipal.png" width="1584" height="349" loading="lazy"/>

```text
grant (role designer, role dba) update db_design_doc
```

리소스는 보호 대상 객체를 지정하고, 작업은 애플리케이션의 동작을 지정합니다. 조건은 신뢰할 수 있는 속성, 상수, 연산자 및 지원되는 함수로 평가되는 부울 표현식입니다.

<img src="/img/speedle/resource.png" width="1732" height="349" loading="lazy"/>

문법에 대해서는 [SPDL](/docs/spdl/)을 참조하고, 완전한 결정 예제에 대해서는 [RBAC](/docs/overview/)/[ABAC](/docs/overview/)를 참조하십시오.

## 예제 실행 전 준비

[빠른 시작](/quick-start/)의 설명에 따라 저장소 루트 아래의 `./bin/spctl`, `./bin/speedle-pms`, `./bin/speedle-ads`를 빌드합니다. Go 1.26.10 또는 Go 1.27.3(또는 동일 릴리스 브랜치의 최신 패치 버전), Bash, curl, Python 3이 필요합니다. 각 터미널은 저장소 루트에서 실행하십시오. 이 페이지를 위해 새 정책 파일을 사용합니다. 두 서비스가 모두 중지된 후에만 파일을 재설정하십시오.

빈 정책 파일 초기화:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

> **macOS:** `/usr/sbin/spctl`은 Apple의 Gatekeeper 도구이며, 인수 없이 `spctl`을 실행할 때 해당 도구가 선택될 수 있습니다. Speedle의 CLI는 전체 경로로 호출하거나, 해당 디렉터리를 `PATH`의 맨 앞에 배치하여 호출하십시오.

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

터미널 3에서 아래 명령을 실행합니다.

## 서비스 생성

서비스를 생성합니다:

<!-- docs-check: create-service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service s1
```

```text
service s1 created
```

서비스 목록 확인:

<!-- docs-check: get-service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service s1
```

```text
{"name":"s1"}
```

## 인가 정책 생성

정책을 추가합니다:

<!-- docs-check: create-authz-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role manager update res1" --service-name=s1
```

출력되는 정책 ID를 기록해 둡니다:

```text
policy 12345678-1234-1234-1234-123456789abc created
```

결정 확인:

<!-- docs-check: eval-deny-unassigned -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --user alan --action update --resource res1 --service-name=s1
```

```text
{"allowed":false,"reason":3}
```

Alan에게 아직 `manager` 역할이 할당되지 않았으므로 결정은 거부됩니다.

## 역할 정책 생성

Alan에게 `manager` 역할을 할당합니다:

<!-- docs-check: create-role-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user alan manager on res1" --service-name=s1
```

동일한 인가 요청을 다시 평가합니다:

<!-- docs-check: eval-allow-role -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --user alan --action update --resource res1 --service-name=s1
```

```text
{"allowed":true,"reason":0}
```

## 정규식 리소스 매칭

정규식 리소스 일치 정책을 추가합니다:

<!-- docs-check: create-regex-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$" --service-name=s1
```

일치하는 리소스에 대한 요청 확인:

<!-- docs-check: eval-regex-match -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --group Administrators --action get --resource c1/default/core/pods/pod1 --service-name=s1
```

```text
{"allowed":true,"reason":0}
```

경로 외부의 리소스에 대한 요청 확인:

<!-- docs-check: eval-regex-no-match -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --group Administrators --action get --resource c1/default/core/podssecret --service-name=s1
```

```text
{"allowed":false,"reason":3}
```

## AND 주체 그룹

두 역할을 모두 요구하는 정책을 추가합니다:

<!-- docs-check: create-and-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant (role designer, role dba) update db_design_doc" --service-name=s1
```

하나의 역할만으로 요청 평가:

<!-- docs-check: eval-and-partial -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --role designer --action update --resource db_design_doc --service-name=s1
```

```text
{"allowed":false,"reason":3}
```

두 역할을 모두 포함하여 요청 평가:

<!-- docs-check: eval-and-full -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1/ is-allowed --role designer --role dba --action update --resource db_design_doc --service-name=s1
```

```text
{"allowed":true,"reason":0}
```

## Speedle Cloud에서 정책 관리

계정 API 키로 `spctl`에서 Cloud 서비스와 정책을 관리합니다.

계정 API 키에는 고정 범위 하나를 지정합니다. `pms:read`는 서비스와 정책 읽기를, `pms:write`는 추가로 생성과 삭제를 허용합니다. 계정 내 서비스에 적용됩니다. `ads` 키로는 관리할 수 없으며 `403 insufficient_scope`입니다. 생성과 폐기는 [API 키](/ko/docs/cloud/tokens/)를 참조하세요.

엔드포인트를 전역 플래그로 전달하고 `SPCTL_TOKEN`에서 API 키을 읽습니다. 대화형 셸에서 이를 사용하면 비밀 값이 명령 인수나 셸 기록에 남지 않습니다:

<!-- deployment-template: cloud-token-environment -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
export SPCTL_TOKEN
./bin/spctl --pms-endpoint https://cloud.speedle.io/policy-mgmt/v1 get service --all
unset SPCTL_TOKEN
```

호환성을 위해 `--token` 플래그도 계속 지원되지만 환경 변수를 사용하면 명령 인수를 통한 API 키 유출을 방지할 수 있습니다. 모든 전역 플래그에는 환경 변수도 제공됩니다. 접두사 `SPCTL_`을 붙이고 플래그 이름을 대문자로 변환한 뒤 하이픈을 밑줄로 대체합니다. 예를 들어 `SPCTL_PMS_ENDPOINT`는 `pms-endpoint`를 설정하고 `SPCTL_SKIPVERIFY`는 `skipverify`를 설정합니다.

구성을 저장하려면 `~/.speedle`을 사용합니다. 형식은 명령줄 플래그 이름과 동일한 한 줄당 하나의 `name=value` 쌍입니다:

```ini
pms-endpoint=https://cloud.speedle.io/policy-mgmt/v1
token=<api-key>
```

명령 인수에 API 키을 포함하지 않고 이 파일을 생성하거나 업데이트합니다:

<!-- deployment-template: cloud-token-config -->
```bash
read -rsp 'API key: ' SPCTL_TOKEN; echo
printf '%s\n' "$SPCTL_TOKEN" | ./bin/spctl config token --stdin
unset SPCTL_TOKEN
```

`spctl config token --stdin`은 권한 `0600`으로 `~/.speedle`을 작성합니다. 파일을 수동으로 편집하는 경우 `install -m 600 /dev/null ~/.speedle`로 생성하거나 나중에 `chmod 0600 ~/.speedle`을 실행하십시오. 그룹 또는 기타 접근을 허용하는 권한을 가진 파일에서 API 키을 읽으면 `spctl`은 표준 오류로 경고를 출력합니다. `spctl config --list`는 구성된 API 키을 `********`로 마스킹합니다.

명시적인 명령줄 플래그는 환경 변수를 재정의하고, 환경 변수는 두 구성 파일을 모두 재정의합니다. 파일 간에는 `~/.speedle`이 `~/.spctlconfig`를 재정의합니다. `spctl config`로 API 키 이외의 키를 설정하면 해당 키는 `~/.spctlconfig`에 작성되고 `~/.speedle`에서 해당 복사본이 제거됩니다.

API 키은 인증서 검증이 활성화된 HTTPS를 통해서만 `Authorization: Bearer` 자격 증명으로 전송됩니다. API 키을 사용한 일반 텍스트 HTTP는 `127.0.0.1`, `::1`, `localhost`와 같은 루프백 호스트에 대해서만 허용됩니다. `spctl`은 원격 HTTP 엔드포인트 및 HTTPS에서 HTTP로의 다운그레이드 리디렉션에 대해 자격 증명을 전송하기 전에 거부합니다. HTTPS 엔드포인트의 경우 `--skipverify=true`가 지정되면 API 키 전송도 거부됩니다. 대신 `--cacert`를 사용하여 사설 또는 자체 서명된 인증 기관을 신뢰하도록 설정하십시오. API 키은 일반 명령 출력이나 오류 메시지에 포함되지 않습니다.

Cloud 오류는 0이 아닌 종료 코드를 생성합니다. `401` 후에는 API 키을 확인하고, `403 insufficient_scope` 후에는 범위를 확인하십시오. `404`는 존재하지 않는 리소스를 나타내며, `409 service_exists` 및 `409 policy_exists`는 중복을 나타냅니다. `429`의 경우 반환된 `Retry-After` 값을 준수하고 유계 백오프를 사용하십시오. `429 daily_quota_exceeded`이면 `Retry-After`에 따라 00:00 UTC까지 기다리세요. 서비스와 정책 한도는 `409 quota_exceeded`이며 시간으로 초기화되지 않습니다. 미사용 항목을 삭제하세요. API 키 자리는 키를 폐기하거나 만료를 기다려 확보합니다. [제한과 오류](/ko/docs/cloud/limits-and-errors/)를 참조하세요.

## 기타 인터페이스

[PMS REST API](/docs/api/management_api/)는 JSON을 사용하며 문자열 ID를 가진 플랫 정책 객체를 반환합니다. Go 애플리케이션은 [PMS 인터페이스](https://github.com/teramoby/speedle-plus/tree/master/api/pms)를 직접 호출할 수 있습니다. gRPC 클라이언트는 [공개된 PMS proto](/protobuf/pms.proto)를 사용합니다. 관리 작업에는 [전송 계층 및 호출자 접근 제어](/docs/security/)를 적용하십시오.
