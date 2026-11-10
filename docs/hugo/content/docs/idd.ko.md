+++
title = "ID 도메인"
description = "여러 ID 도메인에서 제공되는 사용자 ID 지원"
date = 2019-01-17T13:20:07+08:00
weight = 40
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["identity domain", "identity"]
lastmod = 2026-10-29T11:53:01+08:00
+++

## ID 도메인이란 무엇인가

ID 도메인(`idd`)은 주체의 명시적인 네임스페이스입니다. 동일한 이름을 가진 두 명의 `user1`이 서로 다른 도메인에 속할 수 있습니다. 도메인 문자열은 신뢰할 수 있는 통합 코드 또는 구성된 토큰 어서터에 의해 제공됩니다. 이것 자체가 인증의 증거는 아닙니다. Speedle은 도메인 이름만으로 공급자의 테넌트 모델을 추론하거나 검증하지 않습니다.

## 정책과 요청에서의 도메인 표현

```text
grant user user1 from github read book
grant user user1 from IDCS.tenant01 read book
```

주체 이름 바로 뒤에 `from <identity-domain>`을 사용합니다. PMS JSON 정책에서는 동일한 주체를 `idd=github:user:user1`로 인코딩합니다. 결정 요청에서는 이 주체를 다음과 같이 표현합니다:

```json
{
  "type": "user",
  "name": "user1",
  "idd": "github"
}
```

도메인이 지정되지 않은 주체는 해당 이름의 모든 도메인 및 도메인이 없는 주체와 일치합니다.

```text
grant user user1 read book
```

위의 인가는 `from github`, `from IDCS.tenant01` 또는 ID 도메인이 없는 `user1`과 일치합니다.

## 로컬 검증 절차

<!-- docs-check: setup -->
```bash
mkdir -p ./tmp && rm -f ./tmp/idd.json
```

PMS를 시작합니다:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./tmp/idd.json --endpoint 127.0.0.1:6733 --insecure=true
```

ADS를 시작합니다:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./tmp/idd.json --endpoint 127.0.0.1:6734 --insecure=true
```

테스트용 서비스와 정책을 생성합니다:

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service bookstore
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 from github read book" --service-name=bookstore
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user2 read book" --service-name=bookstore
```

`github` 도메인의 `user1`은 허용됩니다:

<!-- docs-check: idd-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"bookstore","resource":"book","action":"read"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

도메인이 없는 `user1`은 거부됩니다:

<!-- docs-check: idd-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"bookstore","resource":"book","action":"read"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

`user2`의 정책에는 `from` 절이 없으므로 임의의 도메인을 가진 `user2`가 일치합니다:

<!-- docs-check: idd-any -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user2","idd":"corp"}]},"serviceName":"bookstore","resource":"book","action":"read"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

인가되지 않은 작업은 거부됩니다:

<!-- docs-check: idd-write-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"bookstore","resource":"book","action":"write"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```
