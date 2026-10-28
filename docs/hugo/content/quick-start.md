+++
title = "Quick Start"
description = "Build Speedle+ with a current Go toolchain and verify policy management, direct grants, role grants and denials."
weight = 10
draft = false
toc = false
copycommands = true
customcss = true
customcssfile = "command-copy.css"
tocheading = "h2"
bref = "Build Speedle+ from source, create policies with SPDL and spctl, and test authorization decisions using the REST API. This guide uses a local file store."
icon = "9. Getting Started.svg"
+++

## Try Speedle in the Playground first

**You can try Speedle's basic features before installing anything.** Open the [Playground](../playground/) to edit SPDL policies, set a user, resource and action, and see real Allow/Deny decisions entirely in your browser. No Go installation or running server is required.

Continue below when you are ready to build and run Speedle locally.

## Prerequisites

Use Linux or macOS with Bash, Git, curl and **Go 1.26.3 or newer**. The module requires Go 1.26; this guide is verified with **Go 1.26.10 and Go 1.27.3**. Go 1.27.3 is the recommended stable release for this walkthrough.

Install Go using the [official installation instructions](https://go.dev/doc/install), then check the installed version:

<!-- quick-start: version -->
```bash
go version
```

Go configures `GOROOT` and its default `GOPATH` automatically. No custom values are needed. The commands below build into the checkout's `bin` directory.

## Download Source Code and Build Speedle

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

Run this walkthrough on a local development machine. `--insecure=true` enables plain HTTP for the REST examples; the binaries also open gRPC ports 50001 and 50002. For production transport and authorization settings, see [Security](../docs/security/).

### 1. Start the Policy Management Service

In the first terminal, from the `speedle-plus` directory:

<!-- quick-start: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./quick-start-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

Leave it running. PMS creates `quick-start-policies.json` in this checkout. Wait for the REST server to start before continuing. Both services must use this same file.

### 2. Create policies using spctl

Open a second terminal in the same `speedle-plus` directory.

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

Open a third terminal in the same `speedle-plus` directory:

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

When finished, press **Ctrl+C** in the PMS and ADS terminals. The policy file remains in the checkout; use a fresh file for another run of the creation commands.

For TLS configuration, see [Message security](../docs/security/#message-security-tls). For other deployment types, see [Deployment](../docs/deployment/).
