+++
title = "Go SDK"
description = "Add the Speedle Go SDK and check authorization against a local Speedle."
weight = 30
aliases = ["/docs/api/sdk-go/"]
draft = false
toc = true
tocheading = "h2"
tags = ["api", "sdk"]
categories = ["docs"]
bref = "Add github.com/teramoby/speedle-sdk/go v0.2.0 with go get, then call IsAllowed and Check against a local Speedle."
lastmod = 2026-11-07T19:43:15+08:00
+++

The Go SDK supports the two latest Go releases and uses only the standard library. Module: [`github.com/teramoby/speedle-sdk/go`](https://pkg.go.dev/github.com/teramoby/speedle-sdk/go) v0.2.0, package `speedle`. Reference: [speedle-sdk/go](https://github.com/teramoby/speedle-sdk/tree/main/go).

The examples check the policy described in the [SDKs overview](/docs/sdks/#example-policy).

## Add the dependency

In a new module:

```bash
go mod init example.com/speedle-example
go get github.com/teramoby/speedle-sdk/go@v0.2.0
```

## Call a local Speedle

Start Speedle and create the example policy as described in the [SDKs overview](/docs/sdks/#example-policy). Save this program as `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	speedle "github.com/teramoby/speedle-sdk/go"
)

func main() {
	client, err := speedle.NewClient(speedle.Config{
		Backend:     speedle.BackendSpeedle,
		Endpoint:    "http://localhost:6734",
		Auth:        speedle.NoAuth(),
		ServiceName: "docs",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	allowed, err := client.IsAllowed(ctx, speedle.User("alice"), "get", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice get /docs/public: allowed=%t\n", allowed)

	d, err := client.Check(ctx, speedle.User("alice"), "delete", "/docs/public", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Speedle error:", err)
		os.Exit(1)
	}
	fmt.Printf("alice delete /docs/public: allowed=%t reason=%s\n", d.Allowed, d.Reason)
}
```

Run it:

```bash
go run .
```

Output:

```text
alice get /docs/public: allowed=true
alice delete /docs/public: allowed=false reason=NO_APPLICABLE_POLICIES
```


## Error handling

A denied request is not an error: `IsAllowed` returns `false`, and `Check` returns a decision with `Allowed` false and the reason, as in the `delete` call above. With the default settings, when a call cannot return a decision, it returns a non-nil `error` (`NewClient` does the same for an invalid configuration). The examples print `Speedle error: <message>` to standard error and exit with status 1.
