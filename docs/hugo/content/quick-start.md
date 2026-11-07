+++
title = "Quick Start"
description = "Install Speedle with go install or from source, then verify policy management, direct grants, role grants, conditional (ABAC) decisions and denials."
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "Install Speedle with go install or build it from source, create policies with SPDL and spctl, and test authorization decisions using the REST API. This guide uses a local file store."
icon = "9. Getting Started.svg"
+++

## Try Speedle in the Playground first

**You can try Speedle's basic features before installing anything.** Open the [Playground](../playground/) to edit SPDL policies, set a user, resource and action, and see real Allow/Deny decisions entirely in your browser. No Go installation or running server is required.

Continue below when you are ready to install and run Speedle locally.

## Prerequisites

Use Linux or macOS with Bash, Git, curl and **Go 1.26.10 or Go 1.27.3** (or a newer patch on the same release branch). Speedle supports patched Go 1.26 and Go 1.27 releases; this guide is verified with **Go 1.26.10 and Go 1.27.3**. Go 1.27.3 is the recommended stable release for this walkthrough.

Install Go using the [official installation instructions](https://go.dev/doc/install), then check the installed version:

<!-- quick-start: version -->
```bash
go version
```

Go configures `GOROOT` and its default `GOPATH` automatically. No custom values are needed. Both routes below place the three executables in a `bin` directory.

## Install Speedle

Choose one route: route A installs the latest release, route B builds the current source. The rest of the guide uses `./bin/...` and works with either route.

### Route A: install the latest release

<!-- quick-start: install -->
```bash
GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@v1.6.0
ls bin
```

`GOBIN` keeps the executables in this directory's `bin` folder; without it, Go writes them to `$(go env GOPATH)/bin`. `v1.6.0` is the latest release tag at the time of writing; `go install` builds it from the tagged source and verifies it against the Go checksum database.

### Route B: clone and build

<!-- quick-start: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
ls bin
```

The output directory contains:

```text
spctl  speedle-ads  speedle-pms
```

- `spctl`: the command line interface.
- `speedle-pms`: the Policy Management Service (PMS).
- `speedle-ads`: the Authorization Decision Service (ADS).

Modern Go uses `go build` or `go install` to build executables; `go get` manages module dependencies.

## Run Speedle

Run this walkthrough on a local development machine. `--insecure=true` enables plain HTTP for the REST examples. The gRPC servers are off unless you start them with `--grpc-endpoint`, for example `--grpc-endpoint 127.0.0.1:50001` for PMS and `--grpc-endpoint 127.0.0.1:50002` for ADS; gRPC uses the same TLS settings as REST. For production transport and authorization settings, see [Security](../docs/security/).

### 1. Start the Policy Management Service

In the first terminal, from the directory that contains `bin/` (the checkout for route B):

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

Leave it running. PMS creates `quick-start-policies.json` in this directory. Wait for the REST server to start before continuing. Both services must use this same file.

### 2. Create policies using spctl

Open a second terminal in the same directory.

Create a service container:

<!-- quick-start: service -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service mysvc
```

Grant `user1` permission to get or delete `res1`, and grant `role2` the same actions on `res2`:

<!-- quick-start: policies -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get,del res1" --service-name=mysvc
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role role2 get,del res2" --service-name=mysvc
```

Assign `role2` to `user2` on `res2`:

<!-- quick-start: roles -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user user2 role2 on res2" --service-name=mysvc
```

List the service and its policies:

<!-- quick-start: list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
```

The output includes `mysvc`, two authorization policies and one role policy.

### 3. Start the Authorization Decision Service

Open a third terminal in the same directory:

<!-- quick-start: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

Leave it running and wait for its REST server to start.

### 4. Verify authorization decisions

Run these requests in the second terminal. The first request checks the direct grant to `user1`:

<!-- quick-start: allow-direct -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":true`.

The next request checks the role grant to `user2`:

<!-- quick-start: allow-role -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user2"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":true`.

A user without a matching grant is denied:

<!-- quick-start: deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user3"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":false`.

The `reason` field explains each decision. `0` (`GRANT_POLICY_FOUND`) means a grant policy matched; `3` (`NO_APPLICABLE_POLICIES`) means no policy applied, so Speedle denies by default. The [Authorization Decisions](../docs/decisions/) page lists every reason code.

### 5. Add a conditional (ABAC) policy

Create a grant that applies only when the request carries a `clearance` attribute of at least 3:

<!-- quick-start: abac-policy -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant user user1 get res3 if clearance >= 3" --service-name=mysvc
```

The policy enforcement point (PEP) supplies request attributes, and a condition is evaluated only against the attributes in the request. The `clearance` values below demonstrate the request format only: a real PEP must obtain attribute values from trusted sources, such as authenticated token claims or a trusted store, and never from the request being authorized. A request that omits `clearance` does not match this grant.

With `clearance` set to 4, the request is allowed:

<!-- quick-start: abac-allow -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":4}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":true` and `"reason":0`.

With `clearance` set to 2, the condition does not match, so no policy applies and the request is denied:

<!-- quick-start: abac-deny -->
```bash
curl --fail --silent --show-error --header 'Content-Type: application/json' --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res3","action":"get","attributes":[{"name":"clearance","type":"numeric","value":2}]}' http://127.0.0.1:6734/authz-check/v1/is-allowed
```

The response contains `"allowed":false` and `"reason":3`.

When finished, press **Ctrl+C** in the PMS and ADS terminals. The policy file remains in this directory; use a fresh file for another run of the creation commands.

For TLS configuration, see [Message security](../docs/security/#protect-both-api-transports). For other deployment types, see [Deployment](../docs/deployment/).
