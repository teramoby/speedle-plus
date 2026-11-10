+++
title = "カスタム関数"
description = "SPDL 条件でカスタム関数を呼び出し、認可ルールの計算能力を拡張。"
date = 2019-01-21T10:43:02+08:00
weight = 600
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## なぜカスタム関数を使用するのか？

組み込みの演算や関数でルールを表現できない場合、SPDL 条件で構成済みの関数を呼び出すことができます。Speedle はその REST エンドポイントに `POST {"params":[...]}` を送信し、`{"result":...}` または `{"error":"..."}` の返却を期待します。パラメータの順序、型、および戻り値の型は条件と一致している必要があります。関数の失敗は評価エラーであり、アプリケーションはアクセスを拒否する必要があります。

## ローカルサンプルの起動

サンプルは 100 から 50000 の間の値を受け付けます。パラメータの型が不正であるかリストが空の場合は HTTP エラーを返します。ループバック IP のみがプレーンテキストを使用でき、その他のリッスンアドレスには TLS を提供する必要があります。

独立したターミナルで実行します:

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

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	flag.Parse()

	http.HandleFunc("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(req.Params) == 0 {
			http.Error(w, "empty parameters", http.StatusBadRequest)
			return
		}
		val, ok := req.Params[0].(float64)
		if !ok || val < 100 || val > 50000 {
			http.Error(w, "invalid parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response{Result: val > 1000})
	})

	server := &http.Server{
		Addr:              *addr,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Listening on %s", ln.Addr())
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
```

## ローカルでの検証

<!-- docs-check: setup -->
```bash
mkdir -p ./tmp && rm -f ./tmp/function.json
```

PMS を起動します:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./tmp/function.json --endpoint 127.0.0.1:6733 --insecure=true
```

ADS を起動します:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./tmp/function.json --endpoint 127.0.0.1:6734 --insecure=true
```

サービスとポリシーを作成します:

<!-- docs-check: function-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service bookstore
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 read book if custom_func(amount)" --service-name=bookstore
```

金額が 1000 を超える場合は許可されます:

<!-- docs-check: function-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"bookstore","resource":"book","action":"read","attributes":[{"name":"amount","type":"numeric","value":2000}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

金額が 1000 以下の場合は拒否されます:

<!-- docs-check: function-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"bookstore","resource":"book","action":"read","attributes":[{"name":"amount","type":"numeric","value":500}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```
