+++
title = "Docker 이미지"
description = "공개된 Speedle 이미지로 speedle-pms, speedle-ads 및 spctl 실행"
weight = 20
aliases = ["/ko/docs/sdks/docker/"]
draft = false
toc = true
tocheading = "h2"
categories = ["docs"]
bref = "Docker Hub 및 GHCR의 단일 이미지로 공유 파일 저장소를 사용하는 정책 관리 서비스, 인가 결정 서비스 및 spctl을 실행합니다."
+++

Speedle은 `speedle-pms`, `speedle-ads`, `spctl`이 포함된 단일 이미지를 배포합니다. 이 이미지는 `FROM scratch`로 빌드되며 UID 65532로 실행되고 셸이 포함되어 있지 않습니다. 첫 번째 인수로 실행할 프로그램을 선택합니다.

두 레지스트리 모두 동일한 이미지 인덱스를 호스팅합니다:

- Docker Hub: `docker.io/speedle/speedle`
- GHCR: `ghcr.io/teramoby/speedle`

릴리스 1.7.0은 두 레지스트리 모두에 `linux/amd64` 및 `linux/arm64` 아키텍처용으로 `1.7.0`, `1.7`, `latest` 태그를 게시합니다.

## 이미지 풀

```bash
docker pull docker.io/speedle/speedle:1.7.0
docker pull ghcr.io/teramoby/speedle:1.7.0
```

`docker pull`은 인덱스 다이제스트를 출력합니다. 1.7.0의 경우 두 레지스트리 모두 다이제스트는 `sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`입니다.

## 태그 선택

`1.7.0`은 이번 릴리스를 가리킵니다. 상위 1.7 패치가 게시되면 `1.7` 태그가 이동합니다. `master`의 최신 커밋에서 상위 릴리스가 게시되면 `latest` 태그가 이동합니다. 릴리스 재시도로 동일한 버전 태그가 다시 게시될 수 있으므로 태그는 불변 빌드가 아닙니다.

인덱스 다이제스트 확인:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0 --format '{{.Manifest.Digest}}'
```

1.7.0 릴리스의 경우 두 레지스트리의 `1.7.0`, `1.7`, `latest` 모두 다음을 출력합니다:

```text
sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0
```

`1.7` 또는 `latest`를 배포하기 전에 다이제스트를 다시 확인하십시오. 특정 빌드를 고정하려면 이미지 이름 뒤에 `@`와 해당 다이제스트를 사용합니다(예: `docker.io/speedle/speedle@sha256:20bd3fe8169ec3ddadf1484285e34bd68004dbb457782223dc84a2a9f1dd91c0`).

## 이미지 내 명령

인수 없이 실행하면 이미지는 세 개의 프로그램을 출력합니다:

```bash
docker run --rm docker.io/speedle/speedle:1.7.0
```

```text
speedle-pms
speedle-ads
spctl
```

`docker run --rm ghcr.io/teramoby/speedle:1.7.0`도 동일한 내용을 출력합니다. 아래 명령은 Docker Hub 이름을 사용합니다. GHCR 복사본을 사용하려면 GHCR 이름으로 대체하십시오. 이미지에는 셸이 없으므로 `docker exec`는 절대 경로 `/spctl`, `/speedle-pms` 또는 `/speedle-ads`를 호출해야 합니다.

## PMS 및 ADS 실행

이름이 지정된 볼륨을 생성하고 정책 관리 서비스를 시작합니다. 서버는 컨테이너 내부의 `0.0.0.0:6733`에서 수신 대기합니다. `127.0.0.1:6733`으로 게시하면 인증되지 않은 관리 API가 로컬 머신에만 유지됩니다.

```bash
docker volume create speedle-policies
docker run -d --name speedle-pms \
  -p 127.0.0.1:6733:6733 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
```

새로 명명된 볼륨은 UID 65532가 쓸 수 있는 이미지의 `/data` 디렉터리로 채워집니다. PMS는 `/data/policies.json`이 없으면 새로 생성합니다. 이미지에는 CA 인증서가 없습니다. `--insecure=true`는 이 로컬 HTTP 예제를 위한 것입니다. TLS에 대해서는 [보안](/docs/security/)을 참조하십시오.

Docker는 포트를 게시한 후 프로세스가 수신 대기하기 전에 연결을 리셋할 수 있습니다. 해당 리셋을 포함하여 모든 오류에 대해 재시도하십시오:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
```

첫 번째 시도는 `curl: (56) Recv failure: Connection reset by peer`를 출력할 수 있습니다. 이후 시도에서 본문이 반환됩니다. 비어 있는 저장소는 다음을 응답합니다:

```text
[]
```

동일한 볼륨에서 ADS를 시작합니다. ADS는 PMS가 작성한 파일(이후 업데이트 포함)을 읽습니다.

```bash
docker run -d --name speedle-ads \
  -p 127.0.0.1:6734:6734 \
  -v speedle-policies:/data \
  docker.io/speedle/speedle:1.7.0 \
  speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

## 헬스 및 준비 상태 확인

PMS는 `/healthz`를 제공하지 않습니다. 위에서 본 성공적인 `GET /policy-mgmt/v1/service`는 관리 API가 요청을 수락하고 있음을 의미합니다.

ADS는 라이브니스 및 준비 상태를 제공합니다:

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
```

```text
{"status":"ok"}
{"ready":true,"servicesTotal":0,"servicesUsable":0}
```

프로세스가 실행 중인 동안 `/healthz`는 `200 {"status":"ok"}`를 응답합니다. ADS가 요청을 평가할 수 있을 때 `/readyz`는 `200`을 응답합니다. 파일 저장소에는 분산 상태가 없으므로 서비스 수는 0으로 유지됩니다. 확인해야 할 필드는 `ready`입니다. 이미지에는 셸과 `curl`이 없으므로 이러한 프로브는 호스트나 다른 컨테이너에서 실행됩니다.

## 정책 추가 및 확인

이 컨테이너에서 `spctl`을 실행하여 서비스와 정책을 추가합니다. `--insecure=true` PMS는 로컬 `http://` 주소를 사용합니다:

```bash
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker exec speedle-pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

게시된 포트를 통해 ADS를 직접 쿼리합니다. 사용자 `alice`에게 부여된 `get` 권한을 요청합니다:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":true,"reason":0}
```

`delete` 작업은 부여되지 않았으므로 정책과 일치하지 않습니다:

```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"docs","resource":"/docs/public","action":"delete"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

```text
{"allowed":false,"reason":3}
```

`reason` `3`은 `NO_APPLICABLE_POLICIES`입니다. ADS가 이미 실행 중인 상태에서 PMS를 통해 작성된 정책은 다음 요청에서 바로 반영됩니다. 요청 필드에 대해서는 [인가 결정](/docs/decisions/)을 참조하십시오.

## SDK에서 ADS 호출

SDK 문서 페이지들은 `http://localhost:6734`를 호출합니다. 위에서 게시된 포트를 사용하면 해당 주소가 바로 이 ADS입니다. 아래의 Node.js 클라이언트는 컨테이너가 게시하는 주소인 `http://127.0.0.1:6734`를 사용합니다. 먼저 예제 디렉터리를 생성한 다음 해당 디렉터리에 클라이언트를 `main.mjs`로 저장합니다:

```bash
mkdir speedle-example
cd speedle-example
```

```javascript
import { Auth, SpeedleClient } from "@speedle/sdk";

const speedle = new SpeedleClient({
  backend: "speedle",
  endpoint: "http://127.0.0.1:6734",
  auth: Auth.none(),
  serviceName: "docs",
});
try {
  const allowed = await speedle.isAllowed("alice", "get", "/docs/public");
  console.log(`alice get /docs/public: allowed=${allowed}`);
} finally {
  await speedle.close();
}
```

`speedle-example`에 공개된 패키지를 설치하고 `main.mjs`를 실행합니다. 설치는 [Node.js SDK](/docs/sdks/node/) 페이지에 설명된 것과 동일한 버전으로 고정됩니다:

```bash
npm init -y
npm pkg set type=module
npm install @speedle/sdk@0.2.1
node main.mjs
```

```text
alice get /docs/public: allowed=true
```

다른 언어 페이지도 동일한 엔드포인트와 서비스 이름을 사용합니다. SDK는 루프백 호스트에 대해서만 일반 `http://`를 허용합니다.

## Compose로 두 서비스 실행

이 파일을 `compose.yaml`로 저장합니다. 이는 동일한 두 프로세스와 동일한 볼륨을 실행하는 또 다른 방법입니다. 이전 섹션의 컨테이너가 여전히 실행 중인 경우 포트 6733과 6734가 비어 있도록 먼저 중지하십시오. 해당 볼륨을 제거하면 정책 파일도 함께 제거됩니다.

```yaml
services:
  pms:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-pms
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6733
      - --insecure=true
    ports:
      - 127.0.0.1:6733:6733
    volumes:
      - speedle-policies:/data
  ads:
    image: docker.io/speedle/speedle:1.7.0
    command:
      - speedle-ads
      - --store-type
      - file
      - --filestore-loc
      - /data/policies.json
      - --endpoint
      - 0.0.0.0:6734
      - --insecure=true
    ports:
      - 127.0.0.1:6734:6734
    volumes:
      - speedle-policies:/data
volumes:
  speedle-policies:
```

```bash
docker compose -f compose.yaml up -d
```

`up -d`는 PMS가 수신 대기하기 전에 반환됩니다. 리소스를 생성하기 전에 PMS 서비스 목록과 ADS의 `/healthz` 및 `/readyz`를 기다립니다. 위에서 설명한 대로 첫 번째 PMS 시도는 연결을 리셋할 수 있습니다.

```bash
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6733/policy-mgmt/v1/service
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/healthz
curl --fail --silent --show-error --retry 30 --retry-all-errors --retry-delay 1 --connect-timeout 2 \
  http://127.0.0.1:6734/readyz
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service docs
docker compose -f compose.yaml exec -T pms /spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user alice get,list /docs/public" --service-name=docs
```

동일한 `is-allowed` 요청을 전송합니다. ADS는 재시작 없이 새 정책을 인식합니다.

```bash
docker compose -f compose.yaml down -v
```

`down -v`는 Compose 볼륨과 그 안에 저장된 정책을 삭제합니다.

## 출처 및 SBOM 검증

배포 빌드에는 출처(Provenance) 및 SBOM(`docker buildx build --provenance=mode=max --sbom=true`)이 포함됩니다. 인덱스 검사:

```bash
docker buildx imagetools inspect docker.io/speedle/speedle:1.7.0
```

인덱스에는 `linux/amd64`, `linux/arm64` 및 두 개의 `unknown/unknown` 매니페스트가 나열됩니다. 이러한 unknown 플랫폼은 증명(attestation) 매니페스트입니다. 각각은 `vnd.docker.reference.type`을 `attestation-manifest`로 설정하고 `vnd.docker.reference.digest`를 해당 플랫폼 이미지로 설정합니다. `docker buildx imagetools inspect ghcr.io/teramoby/speedle:1.7.0`도 동일한 플랫폼 및 증명 다이제스트를 출력합니다.

## 컨테이너 중지

```bash
docker rm -f speedle-pms speedle-ads
docker volume rm speedle-policies
```

`docker volume rm`은 정책 파일을 삭제합니다. Compose로 프로세스를 시작한 경우 대신 `docker compose -f compose.yaml down -v`를 사용하십시오.
