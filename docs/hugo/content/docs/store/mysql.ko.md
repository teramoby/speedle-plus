+++
title = "MySQL 설정 가이드"
description = "PMS 및 ADS용 MySQL 8.4 정책 저장소 구성 단계"
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mysql"]
categories = ["docs"]
bref = "데이터베이스를 생성하고 PMS와 ADS를 구성하여 인가 및 자동 정책 동기화를 확인합니다."
+++

이 가이드에서는 **MySQL 8.4 LTS**와 두 개의 네이티브 Speedle 프로세스를 사용합니다. PMS는 정책을 작성하고 ADS는 별도의 읽기 전용 계정으로 동일한 데이터베이스를 읽습니다. 로컬 예제에서는 Docker와 루프백 주소의 일반 텍스트 HTTP를 사용합니다. 원격 배포의 경우 기존 MySQL 서버와 [보안 가이드](/docs/security/)를 참조하십시오.

## 1. MySQL 백엔드를 포함한 Speedle 빌드

Git, Bash, curl, Docker 및 지원되는 Go 툴체인(Go 1.26.10 또는 Go 1.27.3)을 설치합니다. Docker가 실행 중이어야 합니다. 두 바이너리에 모두 `mysql` 저장소가 포함되도록 최신 소스에서 빌드합니다. 이전에 설치된 이전 릴리스에는 포함되지 않았을 수 있습니다.

<!-- mysql-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

나머지 명령은 이 `speedle-plus` 디렉터리에서 실행합니다. 포트 3306, 6733, 6734를 비워 두거나 아래의 모든 명령 및 구성에서 해당 주소를 변경하십시오.

## 2. MySQL 8.4 시작

아래의 고정 비밀번호는 **로컬 데모용 자격 증명**입니다. 배포 시에는 자체 자격 증명과 보안 전달 방식을 사용하십시오. 명명된 Docker 볼륨은 컨테이너가 중지되거나 다시 생성되더라도 데이터베이스를 보존합니다.

<!-- mysql-step: database -->
```bash
docker run --name speedle-mysql --detach \
  --publish 127.0.0.1:3306:3306 \
  --volume speedle-mysql-data:/var/lib/mysql \
  --env MYSQL_ROOT_PASSWORD=SpeedleDemoRoot84 \
  mysql:8.4
```

초기화가 완료될 때까지 대기합니다:

<!-- mysql-step: ready -->
```bash
until docker exec speedle-mysql mysqladmin ping \
  --host=127.0.0.1 --user=root --password=SpeedleDemoRoot84 --silent
do
  sleep 1
done
```

이미 MySQL 8.4를 실행 중인 경우 이러한 Docker 명령을 건너뛰고 데이터베이스 관리자 클라이언트를 사용하여 다음 단계의 SQL을 실행하십시오. Docker 이미지는 데이터 디렉터리가 비어 있는 경우에만 자격 증명을 초기화합니다. 환경 변수를 변경해도 기존 볼륨 내부의 비밀번호는 재설정되지 않습니다.

## 3. 데이터베이스 및 서비스 계정 생성

Speedle은 기본적으로 테이블을 생성하지만 **데이터베이스는 생성하지 않습니다**. 관리자 권한으로 `speedle` 데이터베이스와 두 개의 계정을 생성합니다:

<!-- mysql-step: accounts -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 <<'SQL'
CREATE DATABASE speedle CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
CREATE USER 'speedle_pms'@'%' IDENTIFIED BY 'SpeedleDemoPms84';
CREATE USER 'speedle_ads'@'%' IDENTIFIED BY 'SpeedleDemoAds84';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, REFERENCES ON speedle.* TO 'speedle_pms'@'%';
GRANT SELECT ON speedle.* TO 'speedle_ads'@'%';
SQL
```

PMS에는 자동 스키마 초기화를 위한 `CREATE` 및 `REFERENCES`와 정책 관리를 위한 `SELECT`, `INSERT`, `UPDATE`, `DELETE`가 필요합니다. ADS에는 `SELECT`만 필요하며 자동 스키마 초기화를 비활성화해야 합니다. 계정 호스트의 `%`는 Docker를 통한 호스트로부터의 연결을 허용합니다. 원격 배포의 경우 계정 호스트를 실제 PMS/ADS 머신으로 제한하십시오.

## 4. PMS 및 ADS 구성 파일 생성

두 DSN은 **동일한 서버와 데이터베이스**를 선택해야 합니다. DSN은 드라이버 구문 `user:password@tcp(host:port)/database`를 따릅니다.

<!-- mysql-step: configuration -->
```bash
mkdir -p mysql-demo
cat > mysql-demo/pms.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_pms:SpeedleDemoPms84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": true
    }
  }
}
JSON
cat > mysql-demo/ads.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_ads:SpeedleDemoAds84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": false,
      "MySQLPollInterval": "1s"
    }
  },
  "enableWatch": true
}
JSON
```

`MySQLInitSchema: true`는 PMS가 처음 시작될 때 누락된 테이블을 자동으로 생성하도록 지시합니다. `MySQLInitSchema: false`는 ADS가 읽기 전용 계정으로 DDL 작업을 시도하지 못하도록 방지합니다. ADS의 `enableWatch: true`는 폴링 기반 감시자(watcher)를 활성화합니다.

## 5. PMS 및 ADS 시작

터미널 A에서 PMS를 시작하고 스키마를 초기화합니다:

<!-- mysql-step: pms -->
```bash
./bin/speedle-pms --config-file mysql-demo/pms.json --endpoint 127.0.0.1:6733 --insecure=true
```

PMS가 수신 대기 상태가 되면 터미널 B에서 ADS를 시작합니다:

<!-- mysql-step: ads -->
```bash
./bin/speedle-ads --config-file mysql-demo/ads.json --endpoint 127.0.0.1:6734 --insecure=true
```

ADS는 데이터베이스에서 기존 정책을 로드하고 업데이트 폴링을 시작합니다. 이 로컬 데모는 루프백 주소에서 일반 텍스트 HTTP를 바인딩합니다. 프로덕션 환경의 REST/gRPC 전송 보안에 대해서는 [보안 가이드](/docs/security/)를 참조하십시오.

## 6. 정책 생성 및 인가 확인

터미널 C에서 서비스와 정책을 생성합니다:

<!-- mysql-step: create-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1 create service s1
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1 create policy \
  --service-name s1 -c "grant user alice read book"
```

ADS 폴링 간격(1초)을 기다린 후 권한을 확인합니다:

<!-- mysql-step: check-allow -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1 is-allowed \
  --service-name s1 --user alice --action read --resource book
```

```text
{"allowed":true,"reason":0}
```

부여되지 않은 작업에 대해 확인:

<!-- mysql-step: check-deny -->
```bash
./bin/spctl --ads-endpoint http://127.0.0.1:6734/authz-check/v1 is-allowed \
  --service-name s1 --user alice --action write --resource book
```

```text
{"allowed":false,"reason":3}
```

## 7. 업데이트 확인 및 거부 정책

명시적 거부 정책을 추가합니다:

<!-- mysql-step: update-policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/s1/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

동기화 후 6단계의 Alice 요청을 반복합니다. 명시적 거부가 우선하므로 이제 `{"allowed":false,"reason":1}`이 반환되어야 합니다. PMS와 ADS를 Ctrl+C로 중지하고 5단계의 명령으로 다시 시작하여 동일한 요청을 반복할 수 있습니다. 저장된 정책과 거부 설정은 MySQL 내에 영구 보존됩니다.

정책 변경은 데이터와 수정 버전이 동일한 트랜잭션에서 진행되도록 PMS 또는 저장소 인터페이스를 거쳐야 합니다. SQL로 직접 편집하더라도 ADS 재로드가 자동으로 트리거되지 않습니다. 데이터베이스가 중단되는 동안 ADS는 계속 실행되며 동기화를 재시도하지만, 장애가 감지되면 기본적으로 `allowed:false`와 오류를 반환합니다. 인가가 재개되기 전에 완전한 스냅샷이 정상적으로 로드되어야 합니다. 공유 [저장소 장애 및 유예 기간 정책](/docs/store/#store-failures-and-cached-authorization)을 참조하십시오. ADS 시작 시에는 여전히 연결 가능한 데이터베이스가 필요합니다.

## 런타임 DDL 권한 없이 스키마 사전 생성

자동 테이블 생성의 대안으로 서비스를 시작하기 전에 프로비저닝 계정을 사용하여 저장소의 [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql)을 실행합니다:

<!-- mysql-step: schema -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 speedle < pkg/store/mysql/schema.sql
```

**두** 구성 파일 모두에서 `MySQLInitSchema`를 `false`로 설정합니다. 위의 데모를 따른 경우 추가 초기화 권한을 취소합니다:

<!-- mysql-step: revoke-ddl -->
```bash
docker exec speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 -e "REVOKE CREATE, REFERENCES ON speedle.* FROM 'speedle_pms'@'%';"
```

PMS는 `SELECT`, `INSERT`, `UPDATE`, `DELETE` 권한으로 실행되며 ADS는 읽기 전용으로 유지됩니다. 구성을 변경한 후에는 두 서비스를 모두 다시 시작하십시오. 스키마 초기화는 기존 테이블을 마이그레이션하지 않습니다. 지원되지 않는 스키마 버전은 시작 시 실패합니다.

## 구성 참조 및 문제 해결

| 속성 | 플래그 | 환경 변수 | 기본값 |
| --- | --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | `SPDL_MYSQLSTORE_DSN` | 필수(데이터베이스 이름 포함) |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `SPDL_MYSQLSTORE_INIT_SCHEMA` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `SPDL_MYSQLSTORE_POLL_INTERVAL` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `SPDL_MYSQLSTORE_QUERY_TIMEOUT` | `10s` |

선택 플래그는 `--store-type mysql`(`SPDL_STORE_TYPE=mysql`)입니다. 명시적인 플래그는 비어 있지 않은 환경 변수를 재정의하고 그다음 구성 파일, 마지막으로 기본값 순으로 우선순위가 적용됩니다. 원격 MySQL의 경우 신뢰할 수 있는 서버 인증서가 있는 호스트 이름을 사용하고 DSN에 `?tls=true`를 추가하십시오. 사용자 지정 CA 구성은 [드라이버 TLS 문서](https://github.com/go-sql-driver/mysql/tree/v1.10.1#tls)를 따릅니다.

| 증상 | 확인 사항 |
| --- | --- |
| 알 수 없는 `mysql` 저장소 또는 플래그 | `pkg/store/mysql`을 포함한 최신 소스에서 두 바이너리를 다시 빌드하십시오. |
| 연결 거부 / 접근 거부 | 데이터베이스 준비 상태, DSN의 호스트/포트, 비밀번호 및 MySQL 계정 호스트 제한. |
| 알 수 없는 데이터베이스 | Speedle은 테이블만 생성하므로 먼저 데이터베이스를 생성하십시오. |
| 테이블 없음 또는 DDL 거부 | PMS를 먼저 시작하거나 스키마를 미리 생성하십시오. 읽기 전용 ADS에서는 `MySQLInitSchema`를 `false`로 설정해야 합니다. |
| 정책 변경이 반영되지 않음 | 두 DSN 모두 동일한 데이터베이스인지, ADS watch가 활성화되었는지, 폴링 지연, 재정의하는 `SPDL_*` 변수가 없는지 확인. |

MySQL은 발견 요청 수집이나 정책 생성을 지원하지 않습니다. 트랜잭션, 스냅샷 및 복구에 대한 자세한 내용은 [플러그형 저장소](/docs/store/)를 참조하십시오.
