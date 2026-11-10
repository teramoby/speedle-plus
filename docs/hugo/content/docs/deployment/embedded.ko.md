+++
title = "Speedle 임베디드 모드"
description = "Go modules를 사용하여 Go 애플리케이션에 Speedle을 임베드하고 허용 및 거부 인가 결정 검증"
weight = 10
aliases = ["/ko/docs/deployment-embeded/"]
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
lastmod = 2026-11-10T14:45:43+08:00
+++

## Speedle 임베디드 모드란 무엇인가

임베디드 모드는 Speedle 정책 결정 엔진을 Go 애플리케이션 프로세스 내에서 직접 실행합니다. PMS 또는 ADS 서버를 시작하지 않고 평가 엔진을 직접 호출합니다. 애플리케이션은 사용자를 인증하고 각 결정에 대해 주체, 리소스 및 작업을 제공합니다.

코드를 작성하기 전에 기본 정책을 체험하려면 [플레이그라운드](/playground/)를 이용하십시오. Speedle을 독립 실행형 서비스로 실행하려면 [빠른 시작](/quick-start/)을 따르십시오.

## 사전 요구 사항

[빠른 시작](/quick-start/)과 동일하게 Linux 또는 macOS 환경과 명령줄 셸, 그리고 **Go 1.26.10 또는 Go 1.27.3**(또는 동일 릴리스 브랜치의 최신 패치 버전)을 준비합니다. 이 예제는 Go modules와 공개된 **Speedle v1.6.0** 릴리스를 사용합니다. Go 1.27.3이 권장 툴체인이며, 예제는 Go 1.26.10 및 Go 1.27.3에서 검증되었습니다.

[공식 설치 지침](https://go.dev/doc/install)에 따라 Go를 설치한 후 버전을 확인합니다:

<!-- embedded: version -->
```bash
go version
```

Go는 `GOROOT` 및 기본 `GOPATH`를 자동으로 구성하므로 별도의 사용자 지정 설정이 필요하지 않습니다.

## 애플리케이션 모듈 생성

Speedle 소스 체크아웃 외부의 쓰기 가능한 디렉터리에서 시작합니다. 이 예제를 위해 새 디렉터리를 사용합니다:

<!-- embedded: setup -->
```bash
mkdir speedle-embedded-demo
cd speedle-embedded-demo
go mod init example.com/speedle-embedded-demo
go get github.com/teramoby/speedle-plus/pkg/eval@v1.6.0
```

기존 Go 모듈이 있는 경우 해당 모듈의 디렉터리에서 마지막 `go get` 명령만 실행합니다.

여기서 `go get`은 `go.mod` 및 `go.sum`에 버전이 지정된 **라이브러리 의존성**을 추가하며 프로그램을 설치하지 않습니다. 이는 독립 실행형 서비스 실행 파일을 빌드하기 위해 `git clone` 및 `go build`를 사용하는 빠른 시작과 다릅니다. 둘 다 Go modules를 사용합니다. `go.mod` 및 `go.sum`을 버전 제어에 유지하십시오.

## 정책 파일 생성

`speedle-embedded-demo`에서 다음 내용을 `policies.spdl`로 저장합니다. 이 정책은 `mysvc` 서비스에서 사용자 `alice`에게 리소스 `res1`에 대한 `get` 권한을 부여합니다:

<!-- embedded: policies -->
```ini
[service.mysvc]
[policy]
grant user alice get res1
```

`eval.NewFromFile` 생성자는 SPDL 또는 JSON 정책이 포함된 로컬 파일을 로드합니다. 이 예제에서는 SPDL을 사용합니다.

## 평가 엔진 초기화 및 결정 요청

동일한 디렉터리에 다음 완성된 프로그램을 `main.go`로 저장합니다:

<!-- embedded: program -->
```go
package main

import (
	"fmt"
	"log"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func main() {
	evaluator, err := eval.NewFromFile("./policies.spdl", false)
	if err != nil {
		log.Fatal(err)
	}

	for _, user := range []string{"alice", "bob"} {
		allowed, _, err := evaluator.IsAllowed(ads.RequestContext{
			Subject: &ads.Subject{
				Principals: []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: user}},
			},
			ServiceName: "mysvc",
			Resource:    "res1",
			Action:      "get",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s allowed=%t\n", user, allowed)
	}
}
```

빈 import `_ "github.com/teramoby/speedle-plus/pkg/store/file"`은 파일 저장소 드라이버를 등록합니다. 이를 생략하면 `unknown store type "file"` 오류가 발생합니다.

`eval.NewFromFile(loc, isWatch)`는 `ads.PolicyEvaluator`와 error를 반환합니다. 평가 엔진을 사용하기 전에 error를 확인하십시오. `isWatch=false`의 경우 정책은 한 번 로드되며, 장기 실행 애플리케이션의 경우 `true`로 설정하면 파일 감시가 활성화됩니다.

해당 인스턴스에서 `evaluator.IsAllowed(ctx)`를 호출합니다. `ads.RequestContext`에는 주체의 프린시펄 목록, `ServiceName`, `Resource`, `Action` 및 선택적 `Attributes`가 포함됩니다. `(allowed bool, reason ads.Reason, err error)`를 반환합니다. 평가 오류는 성공적인 `allowed=false` 결정과 별도로 처리해야 합니다.

## 빌드 및 실행

`speedle-embedded-demo`에서 의존성을 정리하고 애플리케이션을 빌드합니다:

<!-- embedded: build -->
```bash
go mod tidy
go build -o ./bin/embedded-demo .
```

`./policies.spdl`이 올바르게 확인될 수 있도록 동일한 디렉터리에서 실행합니다:

<!-- embedded: run -->
```bash
./bin/embedded-demo
```

결정 출력 결과는 다음과 같습니다:

```text
alice allowed=true
bob allowed=false
```

`alice`는 부여된 권한과 일치하고, `bob`은 일치하는 권한이 없으므로 거부됩니다. 이 예제에는 네트워크 서비스나 데이터베이스가 필요하지 않습니다.
