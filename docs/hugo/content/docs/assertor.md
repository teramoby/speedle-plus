+++
title = "Token asserter"
description = "Token asserter"
date = 2019-01-21T09:28:30+08:00
weight = 60
draft = false
bref = "Evaluation requests can contain an identity token that represents a user. In this case, the evaluation engine can invoke the asserter service to obtain the explicit identities of the user"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["Identity", "Token asserter"]
+++

## What does a token asserter do?

A configured asserter validates a token and returns explicit principals, including their identity domains. Speedle can then evaluate policies using those identities. A production implementation must validate the token's signature or introspection result, issuer, audience, expiry and any provider-specific requirements. The webhook alone does not establish compatibility with arbitrary OAuth/OIDC tokens.

The [Token Assertion Plugin API](/docs/api/asserter_api/) uses integer `errCode` and an optional attribute **object**, not a typed attribute array. `asserterWebhookConfig.endpoint` is the complete URL; the sample below uses `/v1/assert`.

## Run a deterministic local mock

This mock recognizes only `githubtoken`/`github`, `googletoken`/`google`, and `gitlabtoken`/`gitlab`. Each represents `user1` in the named domain. Wrong token/provider pairs fail. **It is a test fixture, not real token validation.** It does not log tokens. Keep it on loopback and use a real provider integration in production.

Start it in a separate terminal from the repository root:

<!-- docs-check: mock-asserter -->
```bash
go run ./samples/docs/asserter
```


<!-- docs-source: samples/docs/asserter/main.go -->
```go
// A deterministic mock for the token-assertion tutorial. Never use for real tokens.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Type string `json:"type"`
	Name string `json:"name"`
	IDD  string `json:"idd"`
}
type response struct {
	Principals []principal `json:"principals,omitempty"`
	ErrCode    int         `json:"errCode"`
	ErrMessage string      `json:"errMessage,omitempty"`
}

func assert(w http.ResponseWriter, r *http.Request) {
	send := func(status int, value response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		send(405, response{ErrCode: 405, ErrMessage: "GET required"})
		return
	}
	// Fixed fixtures model successful assertion. This is not signature validation.
	domains := map[string]string{"githubtoken": "github", "googletoken": "google", "gitlabtoken": "gitlab"}
	domain, ok := domains[r.Header.Get("x-token")]
	if !ok || domain != r.Header.Get("x-idp") {
		send(401, response{ErrCode: 401, ErrMessage: "invalid demonstration token or identity provider"})
		return
	}
	if allowed := r.Header.Get("x-allowedIDD"); allowed != "" {
		found := false
		for _, name := range strings.Split(allowed, ",") {
			found = found || strings.TrimSpace(name) == domain
		}
		if !found {
			send(403, response{ErrCode: 403, ErrMessage: "identity domain is not allowed"})
			return
		}
	}
	send(200, response{Principals: []principal{{Type: "user", Name: "user1", IDD: domain}}})
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "Local demonstration endpoint")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assert", assert)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
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
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true --asserter-endpoint http://127.0.0.1:8080/v1/assert
```

Run the remaining commands in a third terminal. PMS and ADS share the file; watch propagates changes asynchronously. Confirm the intended update has reached ADS before relying on it. `--insecure=true` is only for this loopback example; see [security](/docs/security/) for production connections. Explicit PMS addresses avoid changing `~/.spctlconfig`. Stop both server terminals with Ctrl-C when finished.


## Create domain-specific test policies

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file samples/docs/idd.json
```


The fixture grants GitHub `user1` read access, Google `user1` write access, and all domains rent access. This is the same [identity-domain example](/docs/idd/).

## Exercise token-based decisions

In order, expect Allow, Deny, Allow. The second request is a valid **GitLab** identity, so it cannot use the GitHub-only read grant.

<!-- docs-check: token-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"githubtoken","tokenType":"github"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: token-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"gitlabtoken","tokenType":"gitlab"},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


An invalid token is an assertion error (non-2xx), not an ordinary policy Deny:

<!-- docs-check: token-invalid -->
```bash
curl --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"token":"invalid-token","tokenType":"google"},"serviceName":"booksvc","resource":"book","action":"write"}' \
  -w '\nHTTP %{http_code}\n' http://127.0.0.1:6734/authz-check/v1/is-allowed
```


Stop the mock and both servers with Ctrl-C when finished.

## Configure your provider integration

Instead of the local flag, a full JSON configuration may set:

```json
{
  "asserterWebhookConfig": {
    "endpoint": "https://identity.example/v1/assert",
    "clientCert": "/absolute/path/client.crt",
    "clientKey": "/absolute/path/client.key",
    "caCert": "/absolute/path/issuer-ca.crt",
    "httpTimeout": 5
  }
}
```

This is the asserter portion of the server configuration; supply the store/server settings for the deployed instance as well. `caCert` verifies the webhook server; `clientCert` and `clientKey` are used if that server requires mTLS. Collect tokens using the provider's documented [OpenID Connect](https://openid.net/connect/) or [OAuth](https://oauth.net/2/) integration, and validate the token type actually accepted by that provider. Protect PMS/ADS callers separately as described in [security](/docs/security/).
