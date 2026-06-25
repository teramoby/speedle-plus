<p align="center">
    <img src="/docs/images/sp_logo.png" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ">
        <img src="https://img.shields.io/badge/slack-speedle-red.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/tags">
        <img src="https://img.shields.io/github/v/tag/teramoby/speedle-plus">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml">
        <img src="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml/badge.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/teramoby/speedle-plus">
        <img src="https://goreportcard.com/badge/github.com/teramoby/speedle-plus">
    </a>
    <a href="https://pkg.go.dev/github.com/teramoby/speedle-plus">
        <img src="https://pkg.go.dev/badge/github.com/teramoby/speedle-plus.svg">
    </a>
</p>

<p align="right">
<a href="README.zh-cn.md">中文版</a>
</p>

# Speedle+

Speedle+ is a general purpose authorization engine. It allows users to construct their policy model with user-friendly policy definition language and get authorization decision in milliseconds based on the policies. Speedle is very user-friendly, efficient, and extremely scalable. 

Speedle+ open source project consists of a policy definition language, policy management module, authorization runtime module, commandline tool, and integration samples with popular systems.

Speedle+ is based on Speedle open source project which is hosted at https://github.com/oracle/speedle under UPL.

## Who are we

We are the founding members of Speedle project. Now we are not Oracle employees and consequently not contributors of Speedle project on GitHub anymore. But we still stay with Speedle project. We create a new repo under https://github.com/teramoby/speedle-plus and maintain the new project here now.

## Architecture

Speedle+ consists of three main components that work together to provide policy-based authorization:

```
                    +-----------+
                    |   spctl   |  Command-line tool for managing policies
                    +-----+-----+
                          |
              +-----------+-----------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      |     PMS      |        |     ADS      |
      | Policy Mgmt  |  push  |  Authz Check |
      |   Service    +------->+   Service    |
      +-------+------+        +-------+------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      | Policy Store |        | Policy Store |
      | (file/etcd/  |<-------+ (read-only)  |
      |  mongodb)    |  watch |              |
      +--------------+        +--------------+
```

- **spctl**: Command-line interface for creating, reading, updating, and deleting policies, services, and functions. Communicates with PMS via REST API.
- **PMS (Policy Management Service)**: Provides the REST/gRPC API for policy lifecycle management. Writes policies to the backing store.
- **ADS (Authorization Decision Service)**: Evaluates authorization requests in real time. Loads policies into an in-memory cache and watches the store for changes.

## PDL Syntax Examples

Speedle uses a human-readable Policy Definition Language (PDL). A policy is written
as a single line and created with `spctl`.

**Policy** — `grant|deny <principals> <actions> <resource> [if <condition>]`:

```sh
# Allow users in group "employee" to read the resource "book"
spctl create policy readBooks --service-name=bookstore \
    --pdl-command "grant group employee read book"

# Deny user "bob" from deleting books
spctl create policy bobCannotDelete --service-name=bookstore \
    --pdl-command "deny user bob delete book"

# Match resources by regular expression with the expr: prefix
spctl create policy podAccess --service-name=k8s \
    --pdl-command "grant group Administrators list,watch,get expr:c1/default/core/pods/*"

# Policy with a condition expression
spctl create policy weekdayAccess --service-name=bookstore \
    --pdl-command "grant user alice read book if request_weekday != \"Saturday\" && request_weekday != \"Sunday\""
```

**Role policy** — `grant|deny <principals> <roles> [on <resources>] [in <service>] [if <condition>]`:

```sh
# Grant the "reader" role to group "intern" on resource "book"
spctl create rolepolicy readerRole --service-name=bookstore \
    --pdl-command "grant group intern reader on book"
```

Principals are written as `<type> <name>` pairs (`user alice`, `group employee`,
`role admin`), and multiple principals are separated by commas. Actions are a
comma-separated list. A resource is either a literal string or a regular
expression prefixed with `expr:`.

## Configuration

Speedle uses a JSON configuration file. Example:

```json
{
    "storeConfig": {
        "storeType": "file",
        "storeProps": {
            "FileLocation": "/etc/speedle/policies.json"
        }
    },
    "enableWatch": true,
    "serverConfig": {
        "endpoint": "0.0.0.0:6733",
        "insecure": "true"
    },
    "logConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/speedle.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 30
        }
    },
    "auditLogConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/audit.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 90
        }
    }
}
```

## Documentation

- Latest documentations: <https://speedle.io/docs>
- Go package reference: <https://pkg.go.dev/github.com/teramoby/speedle-plus>

## Quick Start

This guide walks you through running Speedle+ locally with a file-based policy store.

### Prerequisites

- Go 1.22 or greater: <https://go.dev/doc/install>

### 1. Build the binaries

```bash
git clone https://github.com/teramoby/speedle-plus.git
cd speedle-plus
make build
ls $(go env GOPATH)/bin
# Expected output: spctl  speedle-ads  speedle-pms
```

Or install directly with `go install`:

```bash
go install github.com/teramoby/speedle-plus/cmd/spctl@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
```

Then add `$(go env GOPATH)/bin` to your `PATH` or use the full path to the binaries.

### 2. Start the Policy Management Service (PMS)

In one terminal:

```bash
speedle-pms --store-type file --insecure true
```

A default policy store file is created at `/tmp/speedle-test-file-store.json`.

### 3. Create policies via spctl

In another terminal:

```bash
# Create a service
spctl create service mysvc

# Grant user1 access to res1
spctl create policy -c "grant user user1 get,del res1" --service-name=mysvc

# Grant role2 access to res2
spctl create policy -c "grant role role2 get,del res2" --service-name=mysvc

# Assign user2 to role2 on res2
spctl create rolepolicy -c "grant user user2 role2 on res2" --service-name=mysvc
```

### 4. Start the Authorization Decision Service (ADS)

In another terminal:

```bash
speedle-ads --store-type file --insecure true
```

### 5. Verify authorization decisions

In yet another terminal, test with `curl`:

```bash
# user1 should be allowed to get res1
curl -s -X POST \
  -H "Content-Type: application/json" \
  --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
# Expected: {"allowed":true,"reason":0}

# user2 should be allowed to get res2 (via role)
curl -s -X POST \
  -H "Content-Type: application/json" \
  --data '{"subject":{"principals":[{"type":"user","name":"user2"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
# Expected: {"allowed":true,"reason":0}

# user1 should be denied access to res2
curl -s -X POST \
  -H "Content-Type: application/json" \
  --data '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"mysvc","resource":"res2","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
# Expected: {"allowed":false,"reason":3}
```

### Service Ports

| Service | Binary | Default Port |
|---------|--------|---------------|
| Policy Management | `speedle-pms` | 6733 |
| Authorization Decision | `speedle-ads` | 6734 |
| CLI tool | `spctl` | N/A (connects to PMS) |

### Secure Mode (TLS)

The examples above use `--insecure true` for local testing. For production, configure TLS certificates
and omit the `--insecure` flag. See the [Configuration](#configuration) section for details.

> **Note:** The `--insecure true` flag disables transport security. Use it only for local development.
> The `Content-Type: application/json` header is required when calling the ADS REST API.

## Build

### Prerequisites

- Go 1.26 or greater <https://go.dev/doc/install>

### Step

```
$ go install github.com/teramoby/speedle-plus/cmd/spctl@latest
$ go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
$ go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
$ ls $(go env GOPATH)/bin
spctl  speedle-ads  speedle-pms
```

Or clone the repository and build from source:

```
$ git clone https://github.com/teramoby/speedle-plus.git
$ cd speedle-plus
$ make build
```

## Test

```
$ cd speedle-plus
$ make test
```

## Security

**Important:** The PMS (Policy Management Service) management endpoints have no built-in authentication.
In production, bind the PMS server to localhost only and use a reverse proxy with authentication:

```
$ speedle-pms --endpoint=127.0.0.1:6733
```

Alternatively, run the PMS behind a firewall that restricts access to authorized administrators only.
Exposing PMS management endpoints on a public network interface without authentication allows anyone
to create, modify, or delete all policies, services, and functions.

## Get Help

- Join us on Slack: [#speedle-users](https://join.slack.com/t/speedleproject/shared_invite/zt-72fgiyuo-QKJAhHAqVbn17KRFbd7aZw)
- Mailing List: speedle-users@googlegroups.com

## Get Involved

- Learn how to [contribute](CONTRIBUTING.md)
- See [issues](https://github.com/teramoby/speedle-plus/issues) for issues you can help with
