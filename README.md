<p align="center">
    <img src="/docs/images/sp_logo.png" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://discord.gg/uaY2xVsbbT">
        <img src="https://img.shields.io/badge/Discord-Speedle-5865F2?logo=discord&amp;logoColor=white">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/tags">
        <img src="https://img.shields.io/github/tag/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/workflows/Speedle%20Plus%20CI/badge.svg">
        <img src="https://github.com/teramoby/speedle-plus/workflows/Speedle%20Plus%20CI/badge.svg">
    </a>
</p>

<p align="right">
<a href="README.zh-cn.md">中文版</a>
</p>

# Speedle+

Speedle+ is an open-source authorization engine written in Go. Applications ask it whether a subject may perform an action on a resource, and it answers allow or deny in milliseconds from centrally managed policies, so authorization logic stays out of application code. It supports role-based (RBAC) and attribute-based (ABAC) access control; deny policies override grants, and anything not granted is denied.

Speedle+ consists of the Speedle Policy Definition Language (SPDL), the Policy Management Service (PMS, the policy administration point), the Authorization Decision Service (ADS, the policy decision point, which can also be embedded in Go applications), the `spctl` command-line tool, and integration samples for Docker, Kubernetes and Istio. See the [overview](https://speedle.io/docs/overview/) for how these map to common authorization terms.

Speedle+ is based on Speedle open source project which is hosted at https://github.com/oracle/speedle under UPL.

## Who are we

We are the founding members of Speedle project. Now we are not Oracle employees and consequently not contributors of Speedle project on GitHub anymore. But we still stay with Speedle project. We create a new repo under https://github.com/teramoby/speedle-plus and maintain the new project here now.

## Documentation

Latest documentations are available at <https://speedle.io/docs>.

## Get Started

See Getting Started at <https://speedle.io/quick-start/>.

## Embed in a Go application

The Authorization Decision Service can run inside a Go process. Save the policies as `policies.spdl`:

```text
[service.library]
[policy]
grant role librarian read,update book
[rolepolicy]
grant user alice librarian
```

Add the module with `go get github.com/teramoby/speedle-plus`, then evaluate a request:

```go
package main

import (
	"fmt"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file" // registers the file store
)

func main() {
	evaluator, err := eval.NewFromFile("policies.spdl", false)
	if err != nil {
		panic(err)
	}
	for _, user := range []string{"alice", "bob"} {
		allowed, reason, err := evaluator.IsAllowed(ads.RequestContext{
			Subject:     &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: user}}},
			ServiceName: "library",
			Resource:    "book",
			Action:      "update",
		})
		fmt.Println(user, allowed, reason, err)
	}
}
```

Output:

```text
alice true GRANT_POLICY_FOUND <nil>
bob false NO_APPLICABLE_POLICIES <nil>
```

See [Embedded mode](https://speedle.io/docs/deployment-embeded/) for details.

## Install

With Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch):

```
go install github.com/teramoby/speedle-plus/cmd/...@latest
```

This installs `spctl`, `speedle-pms` and `speedle-ads` into `$(go env GOPATH)/bin`, or `$GOBIN` if it is set.

## Build from source

### Prerequisites

-   Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch) <https://go.dev/doc/install>. Speedle+ supports patched Go 1.26 and Go 1.27 releases. Go sets `GOROOT` and `GOPATH` itself.

### Steps

```
git clone https://github.com/teramoby/speedle-plus.git
cd speedle-plus
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

The output is `spctl  speedle-ads  speedle-pms`.

## Test

Run the unit tests, which need no external services:

```
go test $(go list ./pkg/... ./cmd/... ./api/... | grep -v -e adsgrpc -e pmsgrpc -e pmsrest)
```

## Get Help

-   Join us on Discord: [Speedle community](https://discord.gg/uaY2xVsbbT)
-   Mailing List: speedle-users@googlegroups.com
-   Contact: [contact@speedle.io](mailto:contact@speedle.io)

## Get Involved

-   Learn how to [contribute](CONTRIBUTING.md)
-   See [issues](https://github.com/teramoby/speedle-plus/issues) for issues you can help with
