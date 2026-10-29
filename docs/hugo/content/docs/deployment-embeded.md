+++
title = "Speedle embedded mode"
description = "Embed Speedle in a Go application using Go modules and verify Allow/Deny decisions."
weight = 12
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
+++

## What is embedded mode?

Embedded mode runs the Speedle policy decision engine inside your Go application. Call the evaluator directly without starting PMS or ADS servers. Your application authenticates users and supplies the subject, resource and action for each decision.

To try basic policies before writing code, open the [Playground](../../playground/). To run Speedle as standalone services, follow [Quick Start](../../quick-start/).

## Prerequisites

Use Linux or macOS with a shell and **Go 1.26.10 or Go 1.27.3** (or a newer patch on the same release branch), matching [Quick Start](../../quick-start/). This walkthrough uses Go modules and the published **Speedle+ v1.6.0** release. Go 1.27.3 is the recommended toolchain; the example is verified with Go 1.26.10 and Go 1.27.3.

Install Go using the [official installation instructions](https://go.dev/doc/install), then check your version:

<!-- embedded: version -->
```bash
go version
```

Go configures `GOROOT` and its default `GOPATH` automatically; no custom values are needed.

## Create an application module

Start in a writable directory outside the Speedle source checkout. Use a new directory for this walkthrough:

<!-- embedded: setup -->
```bash
mkdir speedle-embedded-demo
cd speedle-embedded-demo
go mod init example.com/speedle-embedded-demo
go get github.com/teramoby/speedle-plus/pkg/eval@v1.6.0
```

In an existing Go module, run only the final `go get` command from that module's directory.

Here, `go get` adds a versioned **library dependency** to `go.mod` and `go.sum`; it does not install a program. This differs from Quick Start, which uses `git clone` and `go build` to build the standalone service executables. Both use Go modules. Keep `go.mod` and `go.sum` in version control.

## Create a policy file

In `speedle-embedded-demo`, save this content as `policies.spdl`. It grants `alice` permission to `get` resource `res1` in service `mysvc`:

<!-- embedded: policies -->
```ini
[service.mysvc]
[policy]
grant user alice get res1
```

The `eval.NewFromFile` constructor loads a local file containing SPDL or JSON policies. This walkthrough uses SPDL.

## Initialize the evaluator and request decisions

Save the following complete program as `main.go` in the same directory:

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

The blank import `_ "github.com/teramoby/speedle-plus/pkg/store/file"` registers the file-store driver. Omitting it causes an `unknown store type "file"` error.

`eval.NewFromFile(loc, isWatch)` returns an `ads.PolicyEvaluator` and an error. Check the error before using the evaluator. With `isWatch=false`, policies are loaded once; setting it to `true` enables file watching for a long-running application.

Call `evaluator.IsAllowed(ctx)` on that instance. Its `ads.RequestContext` contains the subject's principals, `ServiceName`, `Resource`, `Action` and optional `Attributes`. It returns `(allowed bool, reason ads.Reason, err error)`. An evaluation error must be handled separately from a successful `allowed=false` decision.

## Build and run

From `speedle-embedded-demo`, resolve dependencies and build your application:

<!-- embedded: build -->
```bash
go mod tidy
go build -o ./bin/embedded-demo .
```

Run it from the same directory so that `./policies.spdl` resolves correctly:

<!-- embedded: run -->
```bash
./bin/embedded-demo
```

The decision output is:

```text
alice allowed=true
bob allowed=false
```

`alice` matches the grant; `bob` has no matching grant and is denied. No network services or database are needed for this example.
