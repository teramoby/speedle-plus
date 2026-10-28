+++
title = "Custom Function"
description = "Write policy condition with your own functions"
date = 2019-01-21T10:43:02+08:00
weight = 600
draft = false
bref = "When the condition in a policy or role policy is too complicated to draft using an expression and built-in functions, you can write your own functions to include in the condition instead"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## Why use a custom function?

SPDL conditions can call a configured function when built-in operators or functions do not express the required rule. Speedle calls its REST endpoint with `POST {"params":[...]}` and expects `{"result":...}` or `{"error":"..."}`. Parameter order/types and the result type must match the condition. Function failures are evaluation errors; applications must fail closed.

## Start the local example

This example accepts numeric values from 100 to 50000. Wrong parameter types or an empty parameter list return an HTTP error. It uses plaintext only on a loopback IP; use TLS for other interfaces.

Start it in a separate terminal:

<!-- docs-check: function-server -->
```bash
go run ./samples/docs/custom-function
```


<!-- docs-source: samples/docs/custom-function/main.go -->
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

func isValid(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		send(405, response{Error: "POST required"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var input request
	if err := decoder.Decode(&input); err != nil || len(input.Params) == 0 {
		send(400, response{Error: "params must contain at least one number"})
		return
	}
	valid := true
	for _, param := range input.Params {
		value, ok := param.(float64)
		if !ok {
			send(400, response{Error: "all params must be numbers"})
			return
		}
		if value < 100 || value > 50000 {
			valid = false
		}
	}
	send(200, response{Result: valid})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:23456", "Function listen address")
	cert := flag.String("cert", "", "TLS server certificate")
	key := flag.String("key", "", "TLS server key")
	flag.Parse()
	if (*cert == "") != (*key == "") {
		log.Fatal("provide both --cert and --key")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/func/isValid", isValid)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	if *cert != "" {
		log.Fatal(server.ListenAndServeTLS(*cert, *key))
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("plaintext examples require a loopback IP; provide TLS certificates for other addresses")
	}
	log.Fatal(server.ListenAndServe())
}
```


## Before running the examples

Build `./bin/spctl`, `./bin/speedle-pms` and `./bin/speedle-ads` using the [quick start](/quick-start/). Use Go 1.26 or later, Bash, curl and Python 3. Run each terminal from the repository root. Use a fresh policy file for this page; reset it only after stopping both services.

Initialize an empty policy file:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

In terminal one, start PMS and leave it running:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

In terminal two, start ADS and leave it running:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

Run the remaining commands in a third terminal. PMS and ADS share the file; watch propagates changes asynchronously. Confirm the intended update has reached ADS before relying on it. `--insecure=true` is only for this loopback example; see [security](/docs/security/) for production connections. Explicit PMS addresses avoid changing `~/.spctlconfig`. Stop both server terminals with Ctrl-C when finished.


## Register and call the function

The definition below disables result caching so that decisions reflect each request. It creates the service before its policy.

<!-- docs-check: function-fixture -->
```bash
cat > function.json <<'JSON'
{"name":"isValid","funcURL":"http://127.0.0.1:23456/func/isValid","resultCachable":false}
JSON
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create function --json-file function.json
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service service1
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user Ally access library if isValid(attr1)" --service-name service1
```


The following requests return Allow (`200` is in range) and Deny (`50` is outside it):

<!-- docs-check: function-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"numeric","value":200}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: function-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Ally"}]},"serviceName":"service1","resource":"library","action":"access","attributes":[{"name":"attr1","type":"numeric","value":50}]}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


## TLS and caching

For a TLS endpoint, generate fresh [local test certificates](/docs/security/), then start the function with `go run ./samples/docs/custom-function --cert ./tls/server.crt --key ./tls/server.key`. Register `funcURL` as `https://localhost:23456/func/isValid` and set `ca` to the **PEM certificate contents** from `tls/ca.crt`, not its file path. Production certificates must cover the actual host. No expired hard-coded certificate or private key is required.

`resultCachable` defaults to false; `resultTTL` is the cached lifetime in seconds when caching is enabled. Enable caching only if the result remains correct for that period and parameter set. External state such as account status may change before the TTL expires. Use [PMS function management](/docs/api/management_api/) to manage definitions; external function invocation is not available in the browser Playground.
