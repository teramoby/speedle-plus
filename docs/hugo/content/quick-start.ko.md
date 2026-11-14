+++
title = "빠른 시작"
description = "go install 또는 소스 코드로부터 Speedle을 설치하고, 정책 관리, 직접 인가, 역할 인가, 조건부(ABAC) 결정 및 거부를 검증합니다."
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "go install 또는 소스 빌드로 Speedle을 설치하고, SPDL과 spctl로 정책을 생성하며, REST API로 인가 결정을 테스트합니다. 이 가이드는 로컬 파일 저장소를 사용합니다."
icon = "9. Getting Started.svg"
lastmod = 2026-11-10T17:04:32+08:00
+++

## 먼저 플레이그라운드에서 체험

**별도의 설치 없이 Speedle의 기본 기능을 바로 사용해 볼 수 있습니다.** [플레이그라운드](../playground/)를 열어 SPDL 정책을 편집하고 사용자, 리소스, 작업을 설정하면 브라우저 내에서 직접 실제 허용/거부(Allow/Deny) 결정을 확인할 수 있습니다. Go 설치나 서버 실행이 필요하지 않습니다.

로컬 환경에 Speedle을 설치하고 실행할 준비가 되면 아래 단계를 진행하십시오.

## 사전 요구 사항

Linux 또는 macOS, Bash, Git, curl과 **Go 1.27.4**(또는 Go 1.27의 최신 패치 버전)을 준비합니다. 현재 소스를 빌드하려면 Go 1.27.4 이상이 필요하며, 이미 배포된 릴리스에는 각 릴리스의 툴체인 요구 사항이 적용됩니다.

[공식 설치 지침](https://go.dev/doc/install)에 따라 Go를 설치하고 버전을 확인합니다:

<!-- quick-start: version -->
```bash
go version
```

Go는 `GOROOT` 및 기본 `GOPATH`를 자동으로 구성하므로 별도의 사용자 지정 설정이 필요하지 않습니다. 아래 두 경로 중 하나를 선택하면 세 개의 실행 파일이 `bin` 디렉터리에 생성됩니다.

## Speedle 설치

다음 중 하나의 경로를 선택하십시오: 경로 A는 최신 릴리스를 설치하고 경로 B는 현재 소스 코드에서 직접 빌드합니다. 가이드의 나머지 부분에서는 `./bin/...`을 사용하며 두 경로 모두 동일하게 작동합니다.

### 경로 A: 최신 릴리스 설치

<!-- quick-start: install -->
```bash
GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@v1.7.1
ls bin
```

`GOBIN`을 지정하면 실행 파일이 현재 디렉터리의 `bin` 폴더 내에 유지됩니다. 지정하지 않으면 Go는 `$(go env GOPATH)/bin`에 작성합니다. `v1.7.1`은 작성 시점의 최신 릴리스 태그입니다. `go install`은 태그가 지정된 소스에서 빌드하고 Go 체크섬 데이터베이스와 대조하여 검증합니다.

### 경로 B: 클론 후 빌드

<!-- quick-start: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

출력 디렉터리에는 다음 파일이 포함됩니다:

```text
spctl  speedle-ads  speedle-pms
```

- `spctl`: 명령줄 인터페이스
- `speedle-pms`: 정책 관리 서비스(PMS)
- `speedle-ads`: 인가 결정 서비스(ADS)

> **macOS:** `/usr/sbin/spctl`은 Apple의 Gatekeeper 도구이며 단순히 `spctl`을 실행할 때 해당 도구가 선택될 수 있습니다. Speedle의 CLI를 호출할 때는 전체 경로를 지정하거나 파일이 포함된 디렉터리에서 `./spctl`을 사용하십시오. 이 가이드에서는 `./bin/spctl`을 사용합니다. 셸이 어떤 실행 파일을 선택하는지 확인하려면 `command -v spctl`을 실행하십시오. 다운로드한 릴리스에 `speedle-spctl_<os>_<arch>` 자산이 포함되어 있는 경우 이는 Apple의 도구와 충돌하지 않도록 명명된 동일한 CLI입니다.

최신 Go에서는 실행 파일 빌드에 `go build` 또는 `go install`을 사용합니다. `go get`은 모듈 의존성 관리에 사용됩니다.

## Speedle 실행

로컬 개발 머신에서 이 가이드를 진행합니다. `--insecure=true`를 지정하면 REST 예제에서 일반 텍스트 HTTP가 활성화됩니다. `--grpc-endpoint`를 지정하여 시작하지 않는 한 gRPC 서버는 비활성 상태로 유지됩니다(예: PMS용으로 `--grpc-endpoint 127.0.0.1:50001`, ADS용으로 `--grpc-endpoint 127.0.0.1:50002`). gRPC는 REST와 동일한 TLS 설정을 사용합니다. 프로덕션 환경의 전송 및 인가 구성에 대해서는 [보안](../docs/security/)을 참조하십시오.

### 1. 정책 관리 서비스 시작

첫 번째 터미널에서 `bin/`을 포함하는 디렉터리(경로 B의 체크아웃 디렉터리)에서 실행합니다:

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

실행 상태를 유지합니다. PMS는 이 디렉터리에 `quick-start-policies.json`을 생성합니다. 계속 진행하기 전에 REST 서버가 시작될 때까지 대기합니다. 두 서비스 모두 동일한 파일을 사용해야 합니다.

### 2. spctl을 사용하여 정책 생성

동일한 디렉터리에서 두 번째 터미널을 엽니다.

서비스 컨테이너를 생성합니다:

<!-- quick-start: service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
```

`user1`에게 `res1`에 대한 get 또는 delete 권한을 부여하고, `role2`에게 `res2`에 대한 동일한 작업 권한을 부여합니다:

<!-- quick-start: policies -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role role2 get,del res2" --service-name=mysvc
```

`res2`에서 `user2`에게 `role2`를 할당합니다:

<!-- quick-start: roles -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user user2 role2 on res2" --service-name=mysvc
```

서비스와 해당 정책을 나열합니다:

<!-- quick-start: list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

출력에는 `mysvc`, 두 개의 인가 정책 및 하나의 역할 정책이 포함됩니다.

### 3. 인가 결정 서비스 시작

동일한 디렉터리에서 세 번째 터미널을 엽니다:

<!-- quick-start: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

실행 상태를 유지하고 REST 서버가 시작될 때까지 대기합니다.

### 4. 인가 결정 검증

두 번째 터미널에서 다음 요청을 실행합니다. 첫 번째 요청은 `user1`에 대한 직접 인가를 확인합니다:

<!-- quick-start: allow-direct -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

응답에는 `"allowed":true`가 포함됩니다.

다음 요청은 `user2`에 대한 역할 인가를 확인합니다:

<!-- quick-start: allow-role -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user2"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

응답에는 `"allowed":true`가 포함됩니다.

일치하는 인가가 없는 사용자는 거부됩니다:

<!-- quick-start: deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user3"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

응답에는 `"allowed":false`가 포함됩니다.

`reason` 필드는 각 결정의 이유를 설명합니다. `0`(`GRANT_POLICY_FOUND`)은 인가 정책과 일치했음을 의미합니다. `3`(`NO_APPLICABLE_POLICIES`)은 적용 가능한 정책이 없었음을 의미하며 Speedle은 기본적으로 거부합니다. [인가 결정](../docs/decisions/) 페이지에 모든 이유 코드가 나열되어 있습니다.

### 5. 조건부(ABAC) 정책 추가

요청에 3 이상의 `clearance` 속성이 포함된 경우에만 적용되는 인가를 생성합니다:

<!-- quick-start: abac-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get res3 if clearance >= 3" --service-name=mysvc
```

정책 적용 지점(PEP)이 요청 속성을 제공하며 조건은 요청 내부의 속성에 대해서만 평가됩니다. 아래의 `clearance` 값은 요청 형식을 보여주기 위한 것입니다. 실제 PEP는 인가 대상 요청이 아니라 인증된 토큰 클레임이나 신뢰할 수 있는 저장소와 같은 신뢰할 수 있는 소스에서 속성 값을 가져와야 합니다. `clearance`가 생략된 요청은 이 인가와 일치하지 않습니다.

`clearance`를 4로 설정하면 요청이 허용됩니다:

<!-- quick-start: abac-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":4}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

응답에는 `"allowed":true` 및 `"reason":0`이 포함됩니다.

`clearance`를 2로 설정하면 조건이 일치하지 않으므로 적용 가능한 정책이 없어 요청이 거부됩니다:

<!-- quick-start: abac-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":2}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

응답에는 `"allowed":false` 및 `"reason":3`이 포함됩니다.

완료되면 PMS 및 ADS 터미널에서 **Ctrl+C**를 누릅니다. 정책 파일은 이 디렉터리에 유지됩니다. 생성 명령을 다시 실행하려면 새 파일을 사용하십시오.

TLS 구성에 대해서는 [통신 보안](../docs/security/#두-api-전송-계층-모두-보호)을 참조하십시오. 기타 배포 유형에 대해서는 [배포](../docs/deployment/)를 참조하십시오.
