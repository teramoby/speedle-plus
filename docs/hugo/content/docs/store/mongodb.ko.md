+++
title = "MongoDB 설정 가이드"
description = "PMS 및 ADS용 MongoDB 정책 저장소 구성 단계"
weight = 30
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mongodb"]
categories = ["docs"]
bref = "MongoDB 복제본 세트를 실행하고 PMS와 ADS를 연결하여 실시간 정책 업데이트를 확인하고 백업을 생성합니다."
lastmod = 2026-10-31T22:36:36+08:00
+++

이 가이드에서는 영구 Docker 볼륨을 사용하는 단일 노드 복제본 세트로서 **MongoDB 7.0.16**을 사용합니다. PMS는 정책을 작성하고 ADS는 변경 스트림(change stream)을 통해 동일한 데이터베이스를 추적합니다. 로컬 예제에서는 개발 환경을 위해 인증 없이 루프백 주소에서 수신 대기합니다. 프로덕션 환경의 인증, TLS 및 복제본 세트 구성은 가이드 후반부에서 설명합니다.

MongoDB는 **복제본 세트 또는 샤딩된 클러스터**로 실행해야 합니다. ADS는 변경 스트림으로 데이터베이스를 감시하고 전체 정책 저장소 교체는 트랜잭션 내에서 실행됩니다. 독립 실행형 `mongod`는 두 기능 모두 지원하지 않습니다. 개발용으로는 단일 노드 복제본 세트로 충분합니다.

## 1. Speedle 빌드

Bash, Git, curl, Docker, 그리고 Go 1.26.10 또는 Go 1.27.3을 준비합니다. Docker를 시작하고 네이티브 서비스를 빌드합니다:

<!-- mongodb-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

이후 명령은 이 체크아웃 디렉터리에서 실행합니다. 로컬 포트 27017, 6733, 6734를 사용할 수 있는지 확인하거나 명령과 구성을 동시에 업데이트하십시오.

## 2. MongoDB 시작

이름이 지정된 볼륨에 MongoDB 데이터를 보관합니다. `--replSet`은 `speedle-rs` 복제본 세트의 구성원으로 `mongod`를 시작합니다. 루프백의 데이터베이스 포트만 노출됩니다.

<!-- mongodb-step: database -->
```bash
docker run --name speedle-mongodb --detach \
  --publish 127.0.0.1:27017:27017 \
  --volume speedle-mongodb-data:/data/db \
  mongo:7.0.16 \
  --replSet speedle-rs --bind_ip_all
```

서버가 명령을 수락할 때까지 대기합니다:

<!-- mongodb-step: ready -->
```bash
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.adminCommand({ping: 1}).ok' >/dev/null 2>&1
do
  sleep 1
done
```

이미 MongoDB 복제본 세트를 실행 중인 경우 컨테이너 명령과 3단계를 건너뛰고 아래의 연결 문자열을 사용하십시오. 컨테이너에 대한 자세한 내용은 [공식 Docker 이미지](https://hub.docker.com/_/mongo)를 참조하십시오.

## 3. 복제본 세트 초기화

이 서버를 유일한 구성원으로 하여 복제본 세트를 한 번 초기화하고 프라이머리가 될 때까지 대기합니다:

<!-- mongodb-step: replica-set -->
```bash
docker exec speedle-mongodb mongosh --quiet --eval \
  'rs.initiate({_id: "speedle-rs", members: [{_id: 0, host: "127.0.0.1:27017"}]})'
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.hello().isWritablePrimary' | grep -qx true
do
  sleep 1
done
```

구성원 주소 `127.0.0.1:27017`은 클라이언트에 알리는 연결 대상 주소이므로 PMS 및 ADS에서 접근 가능해야 합니다. 여기서는 포트가 동일한 루프백 주소로 노출되므로 정상적으로 작동합니다. 구성은 볼륨에 보관됩니다. 재시작 후 `rs.initiate`를 다시 실행하지 마십시오.

## 4. 공유 저장소 구성

로컬 PMS/ADS 쌍을 위한 구성 파일을 하나 생성합니다:

<!-- mongodb-step: configuration -->
```bash
mkdir -p mongodb-demo
cat > mongodb-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://127.0.0.1:27017/?replicaSet=speedle-rs",
      "MongoDatabase": "speedle_demo"
    }
  },
  "enableWatch": true
}
JSON
```

PMS와 ADS는 **동일한 배포 환경과 데이터베이스**를 사용해야 합니다. 독립적인 정책 저장소마다 개별 데이터베이스를 사용합니다. 저장소는 서비스와 함수를 `services` 및 `functions` 컬렉션에 보관합니다. MongoDB는 첫 번째 쓰기 시 데이터베이스와 컬렉션을 자동으로 생성합니다.

## 5. PMS 시작

터미널 A에서 PMS를 시작하고 실행 상태를 유지합니다:

<!-- mongodb-step: pms -->
```bash
./bin/speedle-pms --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

REST 리스너가 시작될 때까지 대기합니다. 이 데모는 루프백에 일반 텍스트 HTTP를 바인딩합니다. 프로덕션 환경의 전송 보안에 대해서는 [보안 가이드](/docs/security/)를 참조하십시오.

## 6. 정책 생성

터미널 B에서 동일한 체크아웃 디렉터리로부터 Alice에게 `book`에 대한 읽기 권한을 부여합니다:

<!-- mongodb-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mongo-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

```text
{"name":"mongo-demo","type":"application","policies":[{"id":"...","name":"alice-read",...}]}
```

MongoDB에서 문서 확인:

<!-- mongodb-step: documents -->
```bash
docker exec speedle-mongodb mongosh --quiet speedle_demo \
  --eval 'db.services.find({ServiceName: "mongo-demo"}, {ServiceName: 1, "Policies.Name": 1, _id: 0})'
```

## 7. ADS 시작

터미널 C에서 ADS를 시작하고 실행 상태를 유지합니다:

<!-- mongodb-step: ads -->
```bash
./bin/speedle-ads --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

ADS는 데이터베이스에서 기존 정책을 읽고 새 변경 사항에 대한 변경 스트림을 엽니다.

## 8. 인가 확인

터미널 B에서 Alice(허용)와 Bob(거부)의 권한을 확인합니다:

<!-- mongodb-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mongo-demo","resource":"book","action":"read"}'
```

```text
{"allowed":true,"reason":0}
```

<!-- mongodb-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mongo-demo","resource":"book","action":"read"}'
```

```text
{"allowed":false,"reason":3}
```

## 9. 실시간 정책 업데이트 확인

재시작하지 않고 Bob에게 읽기 권한을 부여합니다:

<!-- mongodb-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mongo-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"bob-read","effect":"grant","principals":[["user:bob"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

ADS는 변경 스트림을 통해 업데이트를 수신합니다. 이제 Bob의 결정이 허용됩니다:

<!-- mongodb-step: verify-update -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mongo-demo","resource":"book","action":"read"}'
```

```text
{"allowed":true,"reason":0}
```

## 10. 영속성 및 백업 확인

PMS와 ADS를 중지(Ctrl-C)한 다음 `mongodump`로 정책 데이터베이스를 백업합니다:

<!-- mongodb-step: backup -->
```bash
docker exec speedle-mongodb mongodump \
  --db=speedle_demo --archive=/data/db/speedle_demo.archive
```

아카이브는 볼륨 내부에 남아 있습니다. MongoDB 컨테이너를 다시 시작해도 이전 정책은 그대로 유지됩니다.

## 11. 정리

세 터미널의 모든 프로세스를 중지하고 데모 컨테이너 및 볼륨을 제거합니다:

<!-- mongodb-step: cleanup -->
```bash
docker rm -f speedle-mongodb
docker volume rm speedle-mongodb-data
rm -rf mongodb-demo
```

## 프로덕션 환경 권장 사항

프로덕션 배포 시에는 **SCRAM 또는 X.509 인증과 TLS**를 사용하십시오. 최소 권한 원칙을 따르십시오. PMS는 정책 데이터베이스에 대한 읽기 및 쓰기 권한이 필요하며, ADS는 `read` 및 `changeStream` 권한이 필요합니다. 연결 문자열에 사용자 이름, 비밀번호, `authSource` 및 `tls=true`를 지정하십시오. MongoDB에 대한 TLS는 Speedle의 자체 리스너 TLS와 별도로 구성됩니다.

복제본 세트 접근 제어에는 키 파일 또는 X.509 인증서를 사용한 멤버 간 인증도 필요합니다. 공식 [키 파일 접근 제어를 사용한 자체 관리 복제본 세트 배포](https://www.mongodb.com/docs/manual/tutorial/deploy-replica-set-with-keyfile-access-control/) 및 [TLS 구성](https://www.mongodb.com/docs/manual/tutorial/configure-ssl/) 가이드를 참조하십시오.

고가용성을 보장하려면 최소 3개의 데이터 보유 멤버를 사용하고 연결 문자열에 여러 멤버를 나열하십시오. 단일 노드 데모는 유일한 멤버의 장애를 견딜 수 없습니다. 새 프라이머리가 선출되는 동안 PMS를 통한 쓰기가 실패할 수 있으며 선출 후에 재시도하십시오.

## 구성 참조 및 문제 해결

| 속성 | 플래그 | 환경 변수 | 기본값 |
| --- | --- | --- | --- |
| `MongoURI` | `--mongostore_uri` | `SPDL_MONGOSTORE_URI` | `mongodb://localhost:27017` |
| `MongoDatabase` | `--mongostore_database` | `SPDL_MONGOSTORE_DATABASE` | `speedleplus` |

명시적인 플래그는 비어 있지 않은 환경 변수를 재정의하고 그다음 구성 파일, 마지막으로 기본값 순으로 우선순위가 적용됩니다. 백엔드를 선택하려면 `--store-type mongodb` 또는 `SPDL_STORE_TYPE=mongodb`를 사용하십시오. 임베디드 용도의 경우 `pkg/store/mongodb`를 가져오고 이러한 속성을 `store.NewStore`에 전달하십시오.

저장소 운영 시 알아두어야 할 동작:

- 전체 저장소 교체(`WritePolicyStore`)는 단일 트랜잭션으로 실행됩니다. 지정된 정책 ID를 유지하고 누락된 ID를 생성합니다. 독립형 서버에서는 기존 데이터를 수정하지 않고 실패합니다.
- 모든 서비스나 함수를 삭제해도 컬렉션은 보존되므로 변경 스트림이 계속 열려 있고 ADS는 캐시된 항목을 제거합니다.
- 전체 저장소 읽기 시에는 복제본 세트 또는 샤딩된 클러스터에서 단일 스냅샷으로 서비스와 함수를 읽습니다.
- 변경 스트림이 종료되면 ADS는 최대 30초의 백오프로 다시 열고 전체 저장소를 다시 로드하므로 그 사이에 발생한 변경 사항은 유실되지 않습니다. 그전까지는 마지막으로 로드된 정책을 계속 제공합니다.
- 발견 요청 수집 및 정책 생성은 지원되지 않습니다. 이러한 기능은 파일 저장소 및 etcd 저장소에서 사용할 수 있습니다.

| 증상 | 확인 사항 |
| --- | --- |
| 시작 시 서버 선택 타임아웃 | 복제본 세트가 초기화되었고 프라이머리인지, `rs.conf()`의 멤버 호스트가 PMS/ADS에서 접근 가능한지, URI 호스트 및 포트, 방화벽. |
| ADS가 `The $changeStream stage is only supported on replica sets`로 종료됨 | 배포 환경이 독립 실행형 `mongod`입니다. 복제본 세트 또는 샤딩된 클러스터를 사용하십시오. |
| PMS는 작성하지만 ADS에 서비스가 없음 | 두 구성 모두 동일한 URI 배포 환경 및 `MongoDatabase` 이름을 사용하는지 확인. |
| 변경 사항이 ADS에 반영되지 않음 | watch가 활성화되었는지, ADS 로그에 변경 스트림 및 재오픈 오류가 없는지 확인(ADS가 데이터베이스에 다시 도달할 때까지 반복됨). |
| 인증 실패(Authentication failed) | 사용자 이름, 비밀번호 인코딩, `authSource`, 정책 데이터베이스에 대한 역할. |
| TLS 오류 | `tls=true`, CA 파일 경로, 인증서의 호스트 이름. |

백엔드 인터페이스 및 구성 우선순위에 대해서는 [플러그형 저장소](/docs/store/)를 다시 참조하십시오.

저장소 장애는 모든 기본 내장 저장소와 동일한 정책을 따릅니다. ADS는 사용 불가를 감지한 후 기본적으로 오류와 함께 거부를 반환하고 계속 실행하여 재시도하며, 완전한 재로드가 성공한 후에 재개합니다. 선택적인 유한한 유예 기간 및 취소 지연에 대해서는 [저장소 장애 및 캐시된 인가](/docs/store/#store-failures-and-cached-authorization)를 참조하십시오.
