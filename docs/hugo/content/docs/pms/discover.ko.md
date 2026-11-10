+++
title = "정책 발견"
description = "격리된 테스트 환경에서 접근 요청을 기록하고 인가 정책을 생성 및 검토"
weight = 4
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["policy", "core", "discovery"]
categories = ["docs"]
bref = "접근 요청을 기록하고 인가 정책을 생성하여 일반 인가 확인으로 복귀합니다."
lastmod = 2026-10-29T11:32:30+08:00
+++

## 정책 발견의 역할

발견 모드는 요청을 기록하고 기록 작업이 성공하면 이유 5와 함께 `allowed=true`를 반환합니다. 이는 기존 정책이 해당 작업을 인가했음을 의미하지 않습니다. 격리된 테스트 환경에서만 사용하고 생성된 권한을 검토한 후 일반적인 `is-allowed` 강제로 복원하십시오. 어설션 또는 저장소 오류는 여전히 적절하게 처리해야 하며 오류를 인가로 간주해서는 안 됩니다.

## 재현 가능한 로컬 예제 시작

[빠른 시작](/quick-start/)의 설명에 따라 바이너리를 빌드하고 체크아웃 루트에서 모든 터미널을 실행합니다. 시작하기 전에 새 파일을 사용하고 이전 페이지의 픽스처를 중지하십시오.

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

터미널 1:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

터미널 2:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

두 서비스 모두 실행 상태를 유지합니다. 터미널 3에서 하나의 요청을 기록합니다:

<!-- docs-check: discover-record -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/discover
```

캡처된 요청을 검사하고 제안된 서비스 정의를 생성합니다:

<!-- docs-check: discover-list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover request --service-name observed
```
<!-- docs-check: discover-export -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover policy --service-name observed > discovered-service.json
```

생성된 역할, ID 도메인 및 리소스를 포함하여 `discovered-service.json`을 검토합니다. 이 새로운 예제에서는 `observed`라는 기존 서비스가 없으므로 직접 가져올 수 있습니다. 기존 서비스의 경우 무조건 다시 생성하지 말고 정책을 조정하십시오.

<!-- docs-check: discover-import -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file discovered-service.json
```

## 일반 인가 확인으로 복귀 및 검증

다시 `is-allowed`를 호출합니다. ADS가 정책 업데이트를 로드한 후 Alice는 허용되고 Bob은 거부되어야 합니다:

<!-- docs-check: discover-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: discover-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

두 서비스 모두 Ctrl+C로 중지합니다. gRPC에서는 생성된 클라이언트에서 `Discover`/`IsAllowed`를 사용하고, 임베디드 Go에서는 `evaluator.Discover(ctx)`/`evaluator.IsAllowed(ctx)`를 사용합니다. CLI 옵션에 대해서는 `./bin/spctl discover --help`를 실행하십시오. 실제 프로덕션 애플리케이션이 `/discover`를 계속 가리키도록 두지 마십시오.
