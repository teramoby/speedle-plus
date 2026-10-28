+++
title = "Speedle embedded mode"
description = ""
weight = 12
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
+++

## What is embedded mode?

An embedded Go application evaluates policies in its own process. It still supplies trusted identities and attributes and enforces the returned decision. Try the [Playground](/playground/) first to explore policy semantics without a server.

## Supported configurations

Use Go 1.26 or later, as required by the current `go.mod`. Within your own application module, add a dependency with `go get github.com/teramoby/speedle-plus@<reviewed-tag-or-commit>` after `go mod init`; replace the placeholder with the revision you intend to use. The historical v2.0 release and current master have different dependencies and behavior; see [versions](/docs/versions/).

`eval.NewFromFile(location, watch)` is a convenience API for JSON or SPDL files. Embedded mode can also use `eval.NewFromConfig` or `eval.NewWithStore` with a registered store implementing the ADS interface. The file-only restriction applies to `NewFromFile`, not every embedded evaluator. Import the store implementation, for example `pkg/store/file`, so its builder is registered.

## Run a complete example

From the checked-out repository root, this example reads [policies.json](https://github.com/teramoby/speedle-plus/blob/master/samples/docs/embedded/policies.json): it grants Alice `read` on `book` within `library`. Bob has no matching grant.

<!-- docs-check: embedded -->
```bash
go run ./samples/docs/embedded
```


Expected decisions: `alice: allowed=true` and `bob: allowed=false`.

<!-- docs-source: samples/docs/embedded/main.go -->
```go
package main

import (
	"flag"
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
)

func main() {
	policies := flag.String("policies", "samples/docs/embedded/policies.json", "Policy file")
	flag.Parse()
	evaluator, err := eval.NewFromConfig(&cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: "file", StoreProps: map[string]interface{}{"FileLocation": *policies}}, EnableWatch: false})
	if err != nil {
		log.Fatal(err)
	}
	defer evaluator.Close()
	for _, user := range []string{"alice", "bob"} {
		allowed, reason, err := evaluator.IsAllowed(ads.RequestContext{Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: user}}}, ServiceName: "library", Resource: "book", Action: "read"})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: allowed=%t reason=%s\n", user, allowed, reason.String())
	}
}
```


## Handle results and lifecycle

`IsAllowed(ads.RequestContext)` returns `(allowed bool, reason ads.Reason, err error)`. An application must reject an operation when `err != nil` or `allowed == false`; a decision does not enforce access by itself. `GetAllGrantedRoles`, `GetAllGrantedPermissions` and `Diagnose` provide additional evaluation APIs.

The example uses `NewFromConfig`, which returns an evaluator with `Close()`. Close it when finished to stop background work and release the store. Set `EnableWatch: true` when policy changes must be observed; updates are asynchronous, so verify the intended revision before relying on it. See [policy storage](/docs/store/) and the [embedded expenses sample](https://github.com/teramoby/speedle-plus/tree/master/samples/embedded/expenses) for larger integrations.
