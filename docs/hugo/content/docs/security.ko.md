+++
title = "보안"
description = "Speedle의 REST 및 gRPC 엔드포인트에 TLS, 상호 TLS 및 접근 제어 구성"
weight = 310
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["security", "guide", "tls", "https"]
categories = ["docs"]
bref = ""
lastmod = 2026-10-29T11:53:01+08:00
+++

## 두 API 전송 계층 모두 보호

독립적으로 작동하는 PMS와 ADS는 **REST와 gRPC**를 노출합니다. 현재 두 서비스 모두 동일한 `--insecure`, `--cert`, `--key`, `--client-cert` 및 `--force-client-cert` 전송 설정을 사용합니다. 인증서나 신뢰 설정이 유효하지 않은 경우 시작 시 실패합니다. 보안 모드가 일반 텍스트로 폴백되지 않습니다.

TLS는 연결을 암호화하고 서버를 인증합니다. 인증된 클라이언트를 요구하려면 `--client-cert`로 클라이언트 CA를 제공하고 **반드시** `--force-client-cert=true`를 설정해야 합니다. 해당 플래그가 없는 경우 구성된 CA는 클라이언트가 인증서를 제공한 경우에만 클라이언트 인증서를 검증합니다. TLS 자체만으로는 관리자 인가나 Bearer 토큰 검증을 제공하지 않습니다. 관리, 진단 및 발견 엔드포인트에는 인증된 게이트웨이나 이에 상응하는 접근 제어를 사용하십시오. `--enable-authz` 플래그는 기본 내장 엔드포인트 인가 정책을 제공하지 않습니다.

## 수신 대기 주소 및 호환성

| 서비스 | 기본 REST 주소 | 기본 gRPC 주소 |
| --- | --- | --- |
| PMS | `0.0.0.0:6733` | `--grpc-endpoint`가 설정될 때까지 비활성화 |
| ADS | `0.0.0.0:6734` | `--grpc-endpoint`가 설정될 때까지 비활성화 |

REST와 gRPC 수신 대기 주소는 개별적으로 구성됩니다. `--endpoint`만 설정한다고 해서 gRPC가 활성화되지는 않습니다. PMS용으로는 `--grpc-endpoint 127.0.0.1:50001`, ADS용으로는 `--grpc-endpoint 127.0.0.1:50002`를 명시적으로 지정하십시오. `127.0.0.1:0`은 격리 테스트를 위해 임시 포트를 요청합니다. 필요한 경우 IPv6 주소를 명시적으로 지정하십시오.

구성 우선순위는 명령줄 인수, `SPDL_*` 환경 변수, JSON 구성, 기본값 순입니다. gRPC 재정의는 `--grpc-endpoint`, `SPDL_GRPC_ENDPOINT` 및 `serverConfig.grpcEndpoint`입니다. RPC 네임스페이스와 클라이언트 생성에 대해서는 [현재 proto 계약](/docs/api/)을 참조하십시오.

## 로컬 테스트용 인증서 생성

[빠른 시작](/quick-start/)의 설명에 따라 바이너리를 빌드합니다. 저장소 루트에서 Go 1.26.10 또는 Go 1.27.3(또는 동일 릴리스 브랜치의 최신 패치 버전)을 사용하여 로컬 CA와 개별 서버/클라이언트 인증서를 생성합니다. 서버 인증서에는 `localhost`, `127.0.0.1` 및 `::1`의 SAN이 포함됩니다. 개인 키 권한은 0600입니다. 이는 **24시간 유효한 데모용 인증서**이며 프로덕션용 자격 증명이 아닙니다. 생성기는 기존 파일 덮어쓰기를 거부합니다. 다시 생성하려면 새 출력 디렉터리를 선택하십시오.

<!-- docs-check: certificates -->
```bash
go run ./website/tests/examples/certificates --out ./tls
```

PMS와 ADS용으로 빈 저장소 파일을 준비합니다:

<!-- docs-check: setup -->
```bash
printf '{}\n' > tls-policies.json
```

## mTLS를 사용하여 PMS 시작

클라이언트 인증서를 필수로 요구하는 TLS로 PMS를 시작합니다:

<!-- docs-check: secure-pms -->
```bash
./bin/speedle-pms \
  --store-type file \
  --filestore-loc ./tls-policies.json \
  --endpoint 127.0.0.1:6733 \
  --grpc-endpoint 127.0.0.1:0 \
  --cert ./tls/server.crt \
  --key ./tls/server.key \
  --client-cert ./tls/ca.crt \
  --force-client-cert=true
```

## mTLS를 사용하여 ADS 시작

클라이언트 인증서를 필수로 요구하는 TLS로 ADS를 시작합니다:

<!-- docs-check: secure-ads -->
```bash
./bin/speedle-ads \
  --store-type file \
  --filestore-loc ./tls-policies.json \
  --endpoint 127.0.0.1:6734 \
  --grpc-endpoint 127.0.0.1:0 \
  --cert ./tls/server.crt \
  --key ./tls/server.key \
  --client-cert ./tls/ca.crt \
  --force-client-cert=true
```

## spctl로 연결 테스트

spctl을 사용하여 서비스를 생성합니다:

<!-- docs-check: secure-cli -->
```bash
./bin/spctl \
  --pms-endpoint https://127.0.0.1:6733/policy-mgmt/v1/ \
  --cacert ./tls/ca.crt \
  --cert ./tls/client.crt \
  --key ./tls/client.key \
  create service bookstore
```

## curl로 연결 테스트

curl을 사용하여 인가 결정을 요청합니다:

<!-- docs-check: secure-curl -->
```bash
curl --fail --silent --show-error \
  --cacert ./tls/ca.crt \
  --cert ./tls/client.crt \
  --key ./tls/client.key \
  --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"bookstore","resource":"book","action":"read"}' \
  https://127.0.0.1:6734/authz-check/v1/is-allowed
```
