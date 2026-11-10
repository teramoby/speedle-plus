<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/speedle-engine-lockup-dark.svg">
    <img src="docs/images/speedle-engine-lockup-light.svg" alt="Speedle | Engine" height="52">
  </picture>
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
    <a href="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml">
        <img src="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml/badge.svg">
    </a>
</p>

<p align="right">
<a href="README.zh-cn.md">中文版</a>
</p>

# Speedle

Speedle is an open-source authorization engine written in Go. Applications ask it whether a subject may perform an action on a resource, and it answers allow or deny in milliseconds from centrally managed policies, so authorization logic stays out of application code. It supports role-based (RBAC) and attribute-based (ABAC) access control; deny policies override grants, and anything not granted is denied.

Speedle consists of the Speedle Policy Definition Language (SPDL), the Policy Management Service (PMS, the policy administration point), the Authorization Decision Service (ADS, the policy decision point, which can also be embedded in Go applications), the `spctl` command-line tool, and integration samples for Docker, Kubernetes and Istio. See the [overview](https://engine.speedle.io/docs/overview/) for how these map to common authorization terms.

This project is based on the original Speedle open source project, which is hosted at https://github.com/oracle/speedle under UPL.

## Who are we

We are the founding members of Speedle project. Now we are not Oracle employees and consequently not contributors of Speedle project on GitHub anymore. But we still stay with Speedle project. We create a new repo under https://github.com/teramoby/speedle-plus and maintain the new project here now.

## Documentation

Latest documentations are available at <https://engine.speedle.io/docs/>.

## Get Started

See Getting Started at <https://engine.speedle.io/quick-start/>.

Try the [Playground](https://engine.speedle.io/playground/) to write SPDL policies and test authorization decisions directly in your browser, without installing Speedle.

## Embed in a Go application

The Authorization Decision Service can run inside a Go process. Start a new Go module:

```bash
mkdir speedle-embedded-demo
cd speedle-embedded-demo
go mod init example.com/speedle-embedded-demo
```

If you already have a Go module, use its directory instead. Save the policies as `policies.spdl`:

```text
[service.library]
[policy]
grant role librarian read,update book
[rolepolicy]
grant user alice librarian
```

Save the following program as `main.go`:

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

Install the dependencies and run the example from the same directory:

```bash
go get github.com/teramoby/speedle-plus
go mod tidy
go run .
```

Output:

```text
alice true GRANT_POLICY_FOUND <nil>
bob false NO_APPLICABLE_POLICIES <nil>
```

See [Embedded mode](https://engine.speedle.io/docs/deployment-embeded/) for details.

## Install

With [Homebrew](https://brew.sh/) on macOS or Linux:

```
brew install teramoby/tap/spctl
```

The formula is named `spctl`. On macOS the command is `speedle-spctl`, so the install does not replace Apple Gatekeeper at `/usr/sbin/spctl`. On Linux the same program is also available as `spctl`.

`speedle-pms` and `speedle-ads` are installed the same way:

```
brew install teramoby/tap/speedle-pms
brew install teramoby/tap/speedle-ads
```

Install the command-line tool and both services together:

```
brew install teramoby/tap/speedle
```

The [Homebrew](https://engine.speedle.io/docs/deployment/homebrew/) page shows how to start the services and run a demo.

With Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch):

```
go install github.com/teramoby/speedle-plus/cmd/...@latest
```

This installs `spctl`, `speedle-pms` and `speedle-ads` into `$(go env GOPATH)/bin`, or `$GOBIN` if it is set.

The same three programs are published as one container image, `docker.io/speedle/speedle` and `ghcr.io/teramoby/speedle`, with tags `1.7.0`, `1.7` and `latest`. [Run Speedle from the Docker image](https://engine.speedle.io/docs/sdks/docker/).

## Build from source

### Prerequisites

-   Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch) <https://go.dev/doc/install>. Speedle supports patched Go 1.26 and Go 1.27 releases. Go sets `GOROOT` and `GOPATH` itself.

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
-   Security: see [SECURITY.md](SECURITY.md) to report a vulnerability privately

## Get Involved

-   Learn how to [contribute](CONTRIBUTING.md)
-   See [issues](https://github.com/teramoby/speedle-plus/issues) for issues you can help with
