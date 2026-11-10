+++
title = "etcd 설정 가이드"
description = "PMS 및 ADS용 etcd 정책 저장소 구성 단계"
weight = 20
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "etcd"]
categories = ["docs"]
bref = "독립 실행형 etcd 서버를 실행하고 PMS와 ADS를 연결하여 실시간 정책 업데이트를 확인하고 백업을 생성합니다."
+++

이 가이드에서는 영구 Docker 볼륨을 사용하는 **외부 etcd v3.6.14** 서버를 사용합니다. PMS는 정책을 작성하고 ADS는 동일한 키 접두사를 감시(watch)합니다. 단일 노드 예제는 로컬 개발을 위해 루프백 주소에서 수신 대기하고 일반 텍스트 HTTP를 사용합니다. 프로덕션 환경의 TLS 및 클러스터 구성은 가이드 후반부에서 설명합니다.

## 1. Speedle 빌드

Bash, Git, curl, Docker, 그리고 Go 1.26.10 또는 Go 1.27.3을 준비합니다. Docker를 시작하고 네이티브 서비스를 빌드합니다:

<!-- etcd-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

이후 명령은 이 체크아웃 디렉터리에서 실행합니다. 로컬 포트 2379, 6733, 6734를 사용할 수 있는지 확인하거나 명령과 구성을 동시에 업데이트하십시오.

## 2. 외부 etcd 서버 시작

이름이 지정된 볼륨에 etcd 데이터를 보관합니다. 호스트에는 클라이언트 포트만 노출되며 피어 포트는 이 단일 노드 컨테이너 내부에 유지됩니다.

<!-- etcd-step: database -->
```bash
docker run --name speedle-etcd --detach \
  --publish 127.0.0.1:2379:2379 \
  --volume speedle-etcd-data:/etcd-data \
  gcr.io/etcd-development/etcd:v3.6.14 \
  /usr/local/bin/etcd \
  --name speedle-local \
  --data-dir /etcd-data \
  --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://0.0.0.0:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster speedle-local=http://127.0.0.1:2380 \
  --initial-cluster-token speedle-local-demo \
  --initial-cluster-state new
```

v3 API가 정상(healthy) 상태가 될 때까지 대기합니다:

<!-- etcd-step: ready -->
```bash
until docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 endpoint health
do
  sleep 1
done
```

이미 etcd를 실행 중인 경우 컨테이너 명령을 건너뛰고 아래의 클라이언트 엔드포인트를 사용하십시오. Docker 네트워킹에 대한 자세한 내용은 [공식 컨테이너 가이드](https://etcd.io/docs/v3.6/op-guide/container/)를 참조하십시오.

## 3. 공유 저장소 구성

로컬 PMS/ADS 쌍을 위한 구성 파일을 하나 생성합니다:

<!-- etcd-step: configuration -->
```bash
mkdir -p etcd-demo
cat > etcd-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "http://127.0.0.1:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false
    }
  },
  "enableWatch": true
}
JSON
```

PMS와 ADS는 **동일한 엔드포인트와 키 접두사**를 사용해야 합니다. `/speedle_ps/`의 후행 슬래시를 유지하십시오. 독립적인 정책 저장소마다 개별 접두사를 사용합니다. `IsEmbeddedEtcd: false`는 외부 서버에 연결합니다. `enableWatch: true`는 etcd watch 이벤트를 통한 ADS 업데이트를 활성화합니다. 폴링 간격은 구성되지 않습니다.

## 4. PMS 시작

터미널 A에서 PMS를 시작하고 실행 상태를 유지합니다:

<!-- etcd-step: pms -->
```bash
./bin/speedle-pms --config-file etcd-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

REST 리스너가 시작될 때까지 기다립니다. 이 데모에서는 일반 텍스트 HTTP가 루프백에 바인딩됩니다. Speedle 자체 TLS 및 API 인가에 대해서는 [보안](/docs/security/)을 참조하십시오.

## 5. 정책 생성

터미널 B에서 동일한 체크아웃 디렉터리로부터 Alice에게 `book`에 대한 읽기 권한을 부여합니다:

<!-- etcd-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"etcd-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

```text
{"name":"etcd-demo","type":"application","policies":[{"id":"...","name":"alice-read",...}]}
```

etcd 내부의 키 확인:

<!-- etcd-step: keys -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 get --prefix /speedle_ps/ --keys-only
```

출력에는 서비스 및 정책 키(예: `/speedle_ps/services/etcd-demo` 및 `/speedle_ps/policies/etcd-demo/...`)가 나열됩니다.

## 6. ADS 시작

터미널 C에서 ADS를 시작하고 실행 상태를 유지합니다:

<!-- etcd-step: ads -->
```bash
./bin/speedle-ads --config-file etcd-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

ADS는 저장소에서 기존 정책을 읽고 새 변경 사항에 대한 watch를 시작합니다.

## 7. 인가 확인

터미널 B에서 Alice(허용)와 Bob(거부)의 권한을 확인합니다:

<!-- etcd-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

```text
{"allowed":true,"reason":0}
```

<!-- etcd-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

```text
{"allowed":false,"reason":3}
```

## 8. 실시간 정책 업데이트 확인

재시작하지 않고 Bob에게 읽기 권한을 부여합니다:

<!-- etcd-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/etcd-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"bob-read","effect":"grant","principals":[["user:bob"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

ADS는 etcd watch를 통해 변경 사항을 수신합니다. 이제 Bob의 결정이 허용됩니다:

<!-- etcd-step: verify-update -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

```text
{"allowed":true,"reason":0}
```

## 9. 영속성 및 백업 확인

PMS와 ADS를 중지(Ctrl-C)한 다음 etcd의 스냅샷을 생성합니다:

<!-- etcd-step: backup -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 snapshot save /etcd-data/backup.db
docker exec speedle-etcd /usr/local/bin/etcdctl \
  snapshot status /etcd-data/backup.db --write-out=table
```

스냅샷 파일은 볼륨 내부에 남아 있습니다. etcd 컨테이너를 다시 시작해도 이전 정책은 그대로 유지됩니다.

## 10. 정리

세 터미널의 모든 프로세스를 중지하고 데모 컨테이너 및 볼륨을 제거합니다:

<!-- etcd-step: cleanup -->
```bash
docker rm -f speedle-etcd
docker volume rm speedle-etcd-data
rm -rf etcd-demo
```

## 프로덕션 환경 권장 사항

프로덕션 배포 시에는 **클라이언트 및 피어 트래픽에 TLS**를 사용하십시오. CA 인증서, 클라이언트 인증서 및 키를 프로비저닝하고 HTTPS 엔드포인트를 전달하십시오. etcd에 대한 TLS는 Speedle의 REST/gRPC 리스너의 TLS와 별도로 구성됩니다.

어댑터는 인증서 옵션을 노출하며 etcd의 사용자 이름/비밀번호 속성은 노출하지 않습니다. etcd RBAC의 경우 `--client-cert-auth=true` 및 인증을 활성화하여 클라이언트 인증서의 Common Name을 사용자 ID로 사용합니다. PMS에는 해당 접두사에 대한 읽기/쓰기 접근 권한을 부여하고 ADS에는 필요한 기능에 해당하는 권한을 부여하십시오. 서버 구성 및 역할에 대해서는 공식 [TLS](https://etcd.io/docs/v3.6/op-guide/security/) 및 [RBAC](https://etcd.io/docs/v3.6/op-guide/authentication/rbac/) 가이드를 참조하십시오. 키 접두사는 데이터를 그룹화하며 접근 제어에는 RBAC가 필요합니다.

고가용성을 보장하려면 공식 [3개 노드 클러스터 예제](https://etcd.io/docs/v3.6/op-guide/container/#running-a-3-node-etcd-cluster)를 따르고 클라이언트 및 피어 트래픽을 보호하십시오. 단일 노드 데모는 유일한 멤버의 장애를 견딜 수 없습니다. 현재 `EtcdEndpoint` 속성은 단일 엔드포인트 문자열을 수락합니다. 배포 환경에서 연결 가능한 클라이언트 URL을 지정하십시오.

## 구성 참조 및 문제 해결

| 속성 | 플래그 | 환경 변수 | 기본값 |
| --- | --- | --- | --- |
| `EtcdEndpoint` | `--etcdstore-endpoint` | `SPDL_ETCDSTORE_ENDPOINT` | `localhost:2379` |
| `EtcdKeyPrefix` | `--etcdstore-keyprefix` | `SPDL_ETCDSTORE_KEYPREFIX` | `/speedle_ps/` |
| `IsEmbeddedEtcd` | `--etcdstore-isembedded` | `SPDL_ETCDSTORE_ISEMBEDDED` | `false` |
| `EmbeddedEtcdDataDir` | `--etcdstore-embeddedDataDir` | `SPDL_ETCDSTORE_EMBEDDEDDATADIR` | 비어 있음(임베디드 모드 전용) |
| `EtcdTLSTrustedCAFile` | `--etcdstore-tls-ca` | `SPDL_ETCDSTORE_TLS_CA` | 비어 있음 |
| `EtcdTLSCertFile` | `--etcdstore-tls-cert` | `SPDL_ETCDSTORE_TLS_CERT` | 비어 있음 |
| `EtcdTLSKeyFile` | `--etcdstore-tls-key` | `SPDL_ETCDSTORE_TLS_KEY` | 비어 있음 |
| `EtcdTLSServerName` | `--etcdstore-tls-serverName` | `SPDL_ETCDSTORE_TLS_SERVERNAME` | 비어 있음 |
| `EtcdTLSInsecureSkipVerify` | `--etcdstore-tls-insecureSkipVerify` | `SPDL_ETCDSTORE_TLS_INSECURESKIPVERIFY` | `false` |

명시적인 플래그는 비어 있지 않은 환경 변수를 재정의하고 그다음 구성 파일, 마지막으로 기본값 순으로 우선순위가 적용됩니다. 백엔드를 선택하려면 `--store-type etcd` 또는 `SPDL_STORE_TYPE=etcd`를 사용하십시오. 추가 TLS 옵션에는 `EtcdTLSAllowedCN`(`--etcdstore-tls-allowedCN`) 및 `EtcdTLSCRLFile`(`--etcdstore-tls-crlFile`)이 있습니다.

임베디드 모드(`IsEmbeddedEtcd: true`)는 로컬 개발/테스트용이며 고정된 로컬 etcd 포트를 사용합니다. 이 가이드에서는 데이터 디렉터리와 라이프사이클을 명시적으로 관리할 수 있도록 etcd를 PMS/ADS에서 독립시켰습니다.

| 증상 | 확인 사항 |
| --- | --- |
| 연결이 거부되거나 시작이 중단됨 | etcd 엔드포인트 헬스, 클라이언트 포트, 호스트/컨테이너 주소, 방화벽. |
| PMS는 작성하지만 ADS에 서비스가 없음 | 후행 슬래시를 포함하여 동일한 엔드포인트와 키 접두사를 사용하는지 확인. |
| 변경 사항이 ADS에 반영되지 않음 | watch가 활성화되었는지, ADS 로그에 연결 및 watch 오류가 없는지 확인. 복구 후 ADS를 재시작하여 다시 로드하십시오. |
| TLS 호스트 이름 또는 CA 오류 | HTTPS 엔드포인트, 인증서 SAN, 신뢰할 수 있는 CA 경로, 서버 이름 구성. |
| 권한 거부(Permission denied) | 클라이언트 인증서 ID, 유효한 RBAC 역할, 접두사 권한. |

백엔드 인터페이스 및 구성 우선순위에 대해서는 [플러그형 저장소](/docs/store/)를 다시 참조하십시오.

저장소 장애는 모든 기본 내장 저장소와 동일한 정책을 따릅니다. ADS는 사용 불가를 감지한 후 기본적으로 오류와 함께 거부를 반환하고, 계속 실행하여 재시도하며, 완전한 재로드가 성공한 후에 재개합니다. 선택적인 유한한 유예 기간 및 취소 지연에 대해서는 [저장소 장애 및 캐시된 인가](/docs/store/#store-failures-and-cached-authorization)를 참조하십시오.
