+++
title = "사용자 정의 함수"
description = "SPDL 조건에서 사용자 정의 함수를 호출하여 인가 규칙의 계산 기능 확장"
date = 2019-01-21T10:43:02+08:00
weight = 600
draft = false
bref = "정책이나 역할 정책의 조건이 표현식과 기본 제공 함수로 작성하기에 너무 복잡한 경우, 사용자 정의 함수를 작성하여 조건에 포함할 수 있습니다."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
lastmod = 2026-11-17T10:00:00+08:00
+++

## 사용자 정의 함수를 사용하는 이유

기본 제공 연산자나 함수로 규칙을 표현할 수 없는 경우 SPDL 조건에서 구성된 함수를 호출할 수 있습니다. Speedle은 해당 REST 엔드포인트에 `POST {"params":[...]}`를 전송하고 `{"result":...}` 또는 `{"error":"..."}` 반환을 기대합니다. 매개변수 순서, 타입 및 반환 타입은 조건과 일치해야 합니다. 함수 오류는 평가 오류이며 애플리케이션은 기본적으로 접근을 거부(fail closed)해야 합니다.

## 로컬 예제 시작

이 예제는 100에서 50000 사이의 숫자 값을 수락합니다. 매개변수 타입이 잘못되었거나 매개변수 목록이 비어 있는 경우 HTTP 오류를 반환합니다. 루프백 IP에서만 일반 텍스트를 사용할 수 있으며 다른 네트워크 인터페이스에는 TLS를 적용해야 합니다.

> **참고:** 이 페이지에서 사용하는 `website/tests/examples/` 파일은 저장소 복제본([빠른 시작](/quick-start/)의 경로 B)에 포함됩니다. `go install`에는 포함되지 않습니다.

별도의 터미널에서 실행합니다:

<!-- docs-check: function-server -->
```bash
go run ./website/tests/examples/custom-function
```

<!-- docs-source: website/tests/examples/custom-function/main.go -->
```go
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"time"
)

type request struct {
	Params []interface{} `json:"params"`
}

type response struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Params) != 1 {
		http.Error(w, "expected exactly 1 parameter", http.StatusBadRequest)
		return
	}
	val, ok := req.Params[0].(float64)
	if !ok {
		http.Error(w, "parameter must be numeric", http.StatusBadRequest)
		return
	}
	res := val > 1000
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{Result: res})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	flag.Parse()
	srv := &http.Server{
		Addr:         *addr,
		Handler:      http.HandlerFunc(handler),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("custom-function server listening on %s", ln.Addr())
	log.Fatal(srv.Serve(ln))
}
```

## 로컬에서 검증

<!-- docs-check: setup -->
```bash
mkdir -p ./tmp && rm -f ./tmp/function.json
```

PMS를 시작합니다:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./tmp/function.json --endpoint 127.0.0.1:6733 --insecure=true
```

ADS를 시작합니다:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./tmp/function.json --endpoint 127.0.0.1:6734 --insecure=true
```

서비스와 정책을 생성합니다:

<!-- docs-check: function-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service bookstore
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 read book if custom_func(amount)" --service-name=bookstore
```

금액이 1000을 초과하면 허용됩니다:

<!-- docs-check: function-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"bookstore","resource":"book","action":"read","attributes":[{"name":"amount","type":"numeric","value":2000}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

금액이 1000 이하이면 거부됩니다:

<!-- docs-check: function-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"bookstore","resource":"book","action":"read","attributes":[{"name":"amount","type":"numeric","value":500}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

## 함수 실패

사용자 정의 함수 호출은 실패할 수 있습니다. 시간 초과나 연결 실패, 2xx가 아닌 상태나 읽을 수 없는 응답, `{"error":"..."}`가 포함된 응답, 또는 선언된 타입과 일치하지 않는 값입니다. 이러한 경우는 모두 평가 오류이며 조건이 거짓이 된 것이 아닙니다.

`is-allowed`, `all-granted-roles`, `all-granted-permissions`는 결정 대신 오류를 반환합니다. REST 엔드포인트는 `{"error":"..."}` 본문과 함께 `500 Internal Server Error`를 반환하고, gRPC 메서드는 OK가 아닌 상태를 반환합니다. `diagnose` 엔드포인트는 오류 응답에 실패한 호출을 보고하며, Go `Diagnose` API는 평가 결과도 함께 반환합니다(이유는 `ERROR_IN_EVALUATION`). 실패한 호출은 캐시되지 않으므로 캐시 가능한 함수는 다음 요청에서 다시 호출됩니다.

그 밖의 동작은 바뀌지 않습니다. 특히 요청에 없는 속성을 참조하는 조건은 여전히 일치하지 않으며, 그런 조건을 가진 조건부 `deny`는 적용되지 않습니다.

## 선언된 매개변수 타입과 결과 타입

함수 정의는 인수와 결과의 타입을 선언할 수 있습니다:

```json
{"name":"isValid","funcURL":"http://127.0.0.1:23456/func/isValid","params":["numeric"],"result":"bool"}
```

`params`는 호출 순서대로 인수 타입을 나열하고 `result`는 반환 값의 타입입니다. 두 필드 모두 선택 사항입니다. 타입은 `string`, `numeric`, `bool`, `datetime` 또는 목록 형식인 `string[]`, `numeric[]`, `bool[]`, `datetime[]`입니다. PMS는 그 밖의 값을 거부합니다.

선언하면 Speedle은 호출 전에 인수를, 호출 후에 결과를 검사하며 불일치는 함수 실패입니다. `numeric`은 정수와 부동 소수점 값을 받고, `datetime`은 Unix 타임스탬프 또는 RFC 3339 문자열을 받습니다. 목록 매개변수는 목록 값 자체 또는 조건이 개별 인수로 함수에 전달하는 목록 값의 각 요소를 받습니다. 두 필드를 모두 선언하지 않은 함수는 이러한 검사 없이 호출됩니다.
