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
lastmod = 2026-11-07T01:07:23+08:00
+++

## 사용자 정의 함수를 사용하는 이유

기본 제공 연산자나 함수로 규칙을 표현할 수 없는 경우 SPDL 조건에서 구성된 함수를 호출할 수 있습니다. Speedle은 해당 REST 엔드포인트에 `POST {"params":[...]}`를 전송하고 `{"result":...}` 또는 `{"error":"..."}` 반환을 기대합니다. 매개변수 순서, 타입 및 반환 타입은 조건과 일치해야 합니다. 함수 오류는 평가 오류이며 애플리케이션은 기본적으로 접근을 거부(fail closed)해야 합니다.

## 로컬 예제 시작

이 예제는 100에서 50000 사이의 숫자 값을 수락합니다. 매개변수 타입이 잘못되었거나 매개변수 목록이 비어 있는 경우 HTTP 오류를 반환합니다. 루프백 IP에서만 일반 텍스트를 사용할 수 있으며 다른 네트워크 인터페이스에는 TLS를 적용해야 합니다.

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
