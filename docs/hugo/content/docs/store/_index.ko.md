+++
title = "플러그형 저장소"
description = "파일, etcd, MySQL 또는 MongoDB를 사용하여 정책을 저장하거나 인터페이스를 구현하여 사용자 정의 저장소 백엔드 연결"
weight = 130
draft = false
toc = true
layout = "single"
linksection = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = "Speedle에는 파일, etcd, MongoDB, MySQL 정책 저장소가 포함되어 있습니다. 이 문서에서는 해당 구성과 다른 저장소를 구현하거나 등록하는 방법을 설명합니다."
lastmod = 2026-10-31T22:36:36+08:00
+++

## 기본 내장 저장소와 확장 계약

독립 실행형 바이너리는 `pkg/store` 아래에 파일, etcd, MongoDB, MySQL 구현을 등록합니다. 사용 가능하다고 해서 동일한 운영상의 보장을 의미하는 것은 아닙니다. 배포하기 전에 백엔드의 인증, 영속성, 트랜잭션 및 감시(watch) 동작을 개별적으로 검토하십시오. 아래 구성 예제는 속성 이름을 설명하기 위한 것이며, 로컬 예제에서 etcd/MongoDB 클러스터를 실행하는 것은 범위에 포함되지 않습니다.

완전한 백엔드는 [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go)를 구현합니다(서비스, 인가 정책, 역할 정책, 함수 관리 및 스냅샷 읽기와 watch/stop). ADS는 더 제한적인 읽기/감시 인터페이스인 `PolicyStoreManagerADS`를 수락합니다. 새 백엔드를 구현할 때는 과거 문서의 불완전한 메서드 목록을 복사하지 말고 실제 인터페이스를 확인하십시오.

단계별 배포 가이드: [MySQL 설정 가이드](/docs/store/mysql/), [etcd 설정 가이드](/docs/store/etcd/), [MongoDB 설정 가이드](/docs/store/mongodb/).

<a id="store-failures-and-cached-authorization"></a>
## 저장소 장애 및 캐시된 인가

파일, etcd, MySQL, MongoDB는 동일한 ADS 장애 정책을 사용합니다. `storeFailureGracePeriod`의 기본값은 `"0s"`입니다. 저장소 장애가 감지되면 인가는 `allowed:false`와 평가 오류를 반환합니다. ADS는 계속 실행되며 재시도를 지속합니다. 연결이 다시 읽기 가능해지는 것만으로는 인가가 복원되지 않으며, 완전한 정책 스냅샷을 다시 로드하는 데 성공해야 합니다. 감시(watch)가 활성화된 경우 실패한 구독을 먼저 다시 설정해야 합니다. 감시가 끊어진 상태에서 데이터 읽기에 성공하더라도 장애/유예 기간의 마감 시간은 재설정되지 않습니다. 정책 파일이 삭제된 경우에도 ADS가 종료되지 않고 계속 재시도합니다.

권한 취소 지연을 명시적으로 수용하는 배포 환경에서는 ADS JSON 구성의 최상위 수준에서 유한한 기간(예: `"storeFailureGracePeriod": "5s"`)을 설정할 수 있습니다. 해당하는 플래그는 `--store-failure-grace-period=5s`이며 환경 변수는 `SPDL_STORE_FAILURE_GRACE_PERIOD`입니다. "명시적 플래그 → 환경 변수 → 구성 파일 → 기본값"의 우선순위가 적용됩니다. 장애가 반복되더라도 마감 시간은 연장되지 않습니다. 유예 기간 내의 결정은 마지막 완전한 스냅샷을 사용하므로 동기화가 복구될 때까지 권한 취소가 적용되지 않습니다. 부분적인 캐시 오류나 평가 패닉에는 유예 기간이 적용되지 않습니다.

기본 내장 저장소에는 경량 헬스 프로브가 포함되어 있어 1초의 프로브 제한 시간으로 약 1초마다 확인됩니다. 감시(watch) 기능은 장애를 더 빨리 보고할 수 있습니다. 실행 중인 저장소 읽기 작업에도 각 백엔드의 제한 시간이 적용됩니다. 장애 감지와 일반적인 정책 전파는 비동기적이므로 즉각적인 취소 보장은 없습니다. `enableWatch:false`인 경우에도 요청 시 저장소 헬스를 확인합니다. 일반적인 정책 업데이트에는 명시적인 새로고침이 필요합니다. 장애 발생 후 복구 시에는 해당 모드에서도 전체 스냅샷이 로드됩니다. 초기 로드 실패에는 유예 기간을 사용할 수 없습니다. 사용자 정의 저장소는 `pms.StoreHealthChecker`를 구현하고 `pms.STORE_UNAVAILABLE` 이벤트를 발생시켜 동일한 장애 정책에 참여할 수 있습니다.

파일 저장소의 PMS 쓰기 작업은 완전히 직렬화되고 동기화된 임시 파일을 게시하고 동일한 디렉터리 내에서 원자적 이름 변경을 수행합니다. 따라서 독립적인 ADS 리더는 항상 완전한 이전 스냅샷 또는 새 스냅샷을 참조합니다. 작성자는 상위 디렉터리에 대한 쓰기 권한이 있어야 합니다. 기존 권한 비트 및 구성된 심볼릭 링크는 유지됩니다. 교체 파일은 작성자 계정에 의해 생성되므로 PMS/ADS에서 서로 다른 사용자를 사용하는 경우 디렉터리의 그룹/ACL 상속을 구성하십시오. 컨테이너는 두 서비스에서 교체 파일이 표시될 수 있도록 정책 디렉터리를 공유해야 합니다. 직접 새로 생성된 정책 파일은 권한 `0600`을 사용합니다. 일반 파일 저장소 빌더는 이전과 마찬가지로 누락된 저장소를 초기화합니다.

## MySQL 8.4 정책 저장소

[단계별 MySQL 설정 가이드](/docs/store/mysql/)에 따라 데이터베이스를 생성하고, 개별 PMS/ADS 계정을 구성하며, 정책 동기화를 확인하십시오.

운영 배포 시에는 고정 스키마, 최소 권한 원칙(PMS에만 쓰기 권한, ADS에는 읽기 전용 권한 부여), 풀 크기 조정, TLS 암호화 연결 및 복제 지연 모니터링을 적용하십시오. 감시(watch) 기능은 폴링과 행 버전 번호를 사용하여 변경 사항을 감지합니다. 버전 번호가 증가하지 않고 데이터베이스에 직접 변경된 사항은 자동으로 감지되지 않을 수 있습니다. 정책을 수정할 때는 항상 PMS API를 사용하십시오.

## 빌더 구현 및 등록

[store.StoreBuilder](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/store.go)는 `NewStore(map[string]interface{}) (pms.PolicyStoreManager, error)` 및 `GetStoreParams() map[string]string`을 제공합니다. 후자는 플래그 이름을 구성 속성 이름에 매핑합니다. `init()` 내부 또는 `store.NewStore`를 호출하기 전에 고유한 저장소 유형을 등록하십시오. 중복 등록은 오류가 발생합니다.

이 완전한 예제는 팩토리 계약을 입증하기 위해 기존 파일 백엔드에 위임합니다. 새로운 데이터베이스를 완전히 구현한 것은 아닙니다:

<!-- docs-check: store -->
```bash
go run ./website/tests/examples/store
```

<!-- docs-source: website/tests/examples/store/main.go -->
```go
package main

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
	"os"
	"path/filepath"
)

// This example delegates to the file backend to demonstrate registration.
// A new backend must implement the complete PolicyStoreManager contract.
type Builder struct{ file.FileStoreBuilder }

func main() {
	dir, err := os.MkdirTemp("", "speedle-store-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store.Register("myfile", Builder{})
	policies, err := store.NewStore("myfile", map[string]interface{}{"FileLocation": filepath.Join(dir, "policies.json")})
	if err != nil {
		log.Fatal(err)
	}
	snapshot, err := policies.ReadPolicyStore()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Registered myfile store; services=%d\n", len(snapshot.Services))
}
```

## 올바른 구성 병합

독립 실행형 명령의 경우 명시적인 플래그가 비어 있지 않은 환경 변수를 재정의하고, 그다음 구성 파일 속성, 마지막으로 기본값 순으로 우선순위가 적용됩니다. 플래그 이름을 변환하려면 `SPDL_`을 추가하고 `-`를 `_`로 변경한 후 대문자로 바꿉니다(예: `etcdstore-endpoint`는 `SPDL_ETCDSTORE_ENDPOINT`가 됨). 값은 플래그나 환경 변수의 문자열 또는 형식화된 JSON 값으로 전달될 수 있습니다. 목적에 맞게 검증하고 변환하십시오.

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "localhost:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": true,
      "EmbeddedEtcdDataDir": "./speedle.etcd"
    }
  },
  "enableWatch": true
}
```

구문 분석 전에 `pflag`로 사용자 정의 플래그를 정의하고 `GetStoreParams`에서 플래그와 속성 매핑을 반환합니다. 임베디드 호출자는 구성 맵을 `store.NewStore`/`eval.NewFromConfig`에 직접 전달합니다. 팩토리 자체는 명령줄이나 환경 구성을 읽지 않습니다.

## 백엔드 링크 및 업데이트 처리

등록이 `init()` 내에서 발생하는 경우 두 독립 실행형 바이너리(또는 임베디드 애플리케이션)에서 부수 효과 import를 사용합니다:

```go
import (
    _ "github.com/teramoby/speedle-plus/pkg/store/etcd"
    _ "github.com/teramoby/speedle-plus/pkg/store/file"
    _ "github.com/teramoby/speedle-plus/pkg/store/mongodb"
    _ "github.com/teramoby/speedle-plus/pkg/store/mysql"
)
```

`pms.StorageChangeChannel`을 통해 올바른 스냅샷/변경 이벤트를 반환하도록 `Watch()`를 구현합니다. 감시 리소스를 누수하지 않고 `StopWatch()`를 지원합니다. ADS는 런타임 캐시를 비동기식으로 업데이트합니다. 감시 손실/연결 해제, 복구, 쓰기 중 장애 및 동시 리더를 테스트하십시오. 백엔드가 감시를 지원한다고 해서 즉각적인 전파를 보장해서는 안 됩니다.
