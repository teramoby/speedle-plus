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

---

## Table of Contents

- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [SPDL — Security Policy Definition Language](#spdl--security-policy-definition-language)
- [Policy Management](#policy-management)
- [Authorization Decisions](#authorization-decisions)
- [Advanced Features](#advanced-features)
  - [Policy Discovery](#policy-discovery)
  - [Policy Diagnosis](#policy-diagnosis)
  - [Global Policy](#global-policy)
  - [Identity Domain](#identity-domain)
  - [Token Asserter](#token-asserter)
  - [Custom Function](#custom-function)
- [Storage](#storage)
- [Security & TLS](#security--tls)
- [Logging](#logging)
- [Deployment](#deployment)
- [Integrations](#integrations)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Build](#build)
- [Test](#test)
- [Use Cases](#use-cases)
- [Community & Contributing](#community--contributing)

---

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

If you are familiar with the XACML model, PMS serves as the Policy Administration Point (PAP), and ADS serves as the Policy Decision Point (PDP).

**Service Ports:**

| Service | Binary | Default Port |
|---------|--------|---------------|
| Policy Management | `speedle-pms` | 6733 |
| Authorization Decision | `speedle-ads` | 6734 |
| CLI tool | `spctl` | N/A (connects to PMS) |

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

> **Note:** The `--insecure true` flag disables transport security. Use it only for local development.
> The `Content-Type: application/json` header is required when calling the ADS REST API.
> For production TLS setup, see [Security & TLS](#security--tls).

---

## SPDL — Security Policy Definition Language

SPDL is Speedle's human-readable policy definition language.

### Keywords (reserved, case-insensitive)

`role`, `user`, `group`, `entity`, `grant`, `deny`, `if`, `in`, `on`, `from`

### Policy Syntax

```
POLICY = EFFECT SUBJECT ACTION RESOURCE [if CONDITION]
EFFECT = grant | deny
SUBJECT = PRINCIPAL_TYPE PRINCIPAL_NAME [, PRINCIPAL_TYPE PRINCIPAL_NAME]*
PRINCIPAL_TYPE = user | group | entity | role
ACTION = ACTION_NAME [, ACTION_NAME]*
RESOURCE = RESOURCE_NAME
```

**Examples:**

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

# Policy with AND principals (both roles must match)
spctl create policy designerDBA --service-name=myapp \
    --pdl-command "grant role (designer, dba) update db_design_doc"
```

### Role Policy Syntax

```
ROLE_POLICY = EFFECT SUBJECT ROLE [on RESOURCE] [if CONDITION]
```

```sh
# Grant the "reader" role to group "intern" on resource "book"
spctl create rolepolicy readerRole --service-name=bookstore \
    --pdl-command "grant group intern reader on book"

# Grant role without resource constraint
spctl create rolepolicy managerRole --service-name=bookstore \
    --pdl-command "grant user alan manager"
```

### Conditions

Conditions are bool expressions evaluated at runtime. Supported data types: `string`, `numeric`, `bool`, `datetime`, `array`.

**Built-in Attributes:**

| Attribute | Type | Description |
|-----------|------|-------------|
| `request_user` | string | The user requesting access |
| `request_groups` | []string | Groups of the requesting user |
| `request_entity` | string | Service/entity making the request |
| `request_resource` | string | Resource being accessed |
| `request_action` | string | Action being performed |
| `request_time` | datetime | Request timestamp |
| `request_year` | int | Year of request |
| `request_month` | int | Month of request (1–12) |
| `request_day` | int | Day of request (1–31) |
| `request_hour` | int | Hour of request (0–23) |
| `request_weekday` | string | Day of week ("Sunday"–"Saturday") |

**Built-in Functions:** `Sqrt`, `Max`, `Min`, `Sum`, `Avg`, `IsSubSet`

**Condition Examples:**

```sh
# Time-based access
spctl create policy weekdayAccess --service-name=bookstore \
    --pdl-command "grant user alice read book if request_weekday != \"Saturday\" && request_weekday != \"Sunday\""

# Regular expression matching
spctl create policy regexAccess --service-name=bookstore \
    --pdl-command "grant user bob read book if request_resource =~ \"/books/.*\""

# Using built-in functions
spctl create policy subsetCheck --service-name=bookstore \
    --pdl-command "grant user alice read book if IsSubSet(request_groups, ('managers', 'admins'))"

# Combined conditions
spctl create policy complexCondition --service-name=bookstore \
    --pdl-command "grant user alice read book if request_year==2024 && request_month==12"
```

### Customer Attributes

Pass custom attributes in authorization requests:

```json
{
  "subject": {"principals": [{"type": "user", "name": "alice"}]},
  "serviceName": "bookstore",
  "resource": "book",
  "action": "read",
  "attributes": {
    "clearance_level": {"type": "string", "value": "top_secret"}
  }
}
```

Then use in conditions: `grant user alice read book if clearance_level == "top_secret"`

### "DENY overrides" Algorithm

When both grant and deny policies match, DENY takes precedence.

---

## Policy Management

### Service

A service is the top-level container for authorization and role policies. Each service defines an independent policy scope.

```bash
# Create a service
spctl create service myapp
spctl create service myapp --service-type=k8s

# List all services
spctl get service --all

# Get a specific service
spctl get service myapp

# Delete a service
spctl delete service myapp
```

### Authorization Policies

Define who can perform what actions on which resources:

```bash
# Create a policy using PDL
spctl create policy myPolicy \
    -c "grant user alan read,write book" \
    --service-name=myapp

# Create a policy from JSON file
spctl create policy --json-file ./policy.json --service-name=myapp

# List policies in a service
spctl get policy --service-name=myapp --all

# Get a specific policy by ID
spctl get policy <policy-id> --service-name=myapp

# Delete a policy
spctl delete policy <policy-id> --service-name=myapp
```

**Policy JSON structure:**

```json
{
  "id": "ao3olis24hrzchwjduea",
  "name": "myPolicy",
  "effect": "grant",
  "permissions": [{"resource": "book", "actions": ["read", "write"]}],
  "principals": [["user:alan"]],
  "condition": "clearance_level == \"top_secret\""
}
```

### Role Policies

Assign roles to principals:

```bash
# Create a role policy
spctl create rolepolicy rp01 \
    -c "grant user alan manager on res1" \
    --service-name=myapp

# List role policies
spctl get rolepolicy --service-name=myapp --all

# Delete a role policy
spctl delete rolepolicy <rolepolicy-id> --service-name=myapp
```

### Policy Elements

| Element | Description |
|---------|-------------|
| **Effect** | `grant` or `deny`. DENY overrides GRANT. |
| **Principal** | Identity: `user`, `group`, `entity`, or `role` |
| **AND Principal** | Comma-separated principals enclosed in parentheses: `(designer, dba)` — ALL must match |
| **Resource** | Protected object. Supports regex with `expr:` prefix |
| **Action** | Operation on the resource (arbitrary string) |
| **Condition** | Bool expression controlling when the policy takes effect |

---

## Authorization Decisions

The ADS evaluates authorization requests in real time and returns GRANT/DENY decisions.

### Decision Reasons

| Reason | Code | Meaning |
|--------|------|---------|
| GRANT_POLICY_FOUND | 0 | A grant policy was matched |
| DENY_POLICY_FOUND | 1 | A deny policy was matched |
| SERVICE_NOT_FOUND | 2 | The specified service does not exist |
| NO_APPLICABLE_POLICIES | 3 | No policy matched the request |
| ERROR_IN_EVALUATION | 4 | An error occurred during evaluation |
| DISCOVER_MODE | 5 | In discover mode (always returns allowed) |

### REST API

**is-allowed** — Check if a subject is allowed to perform an action on a resource:

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/is-allowed \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "Alan"}]},
    "action": "download",
    "resource": "/books/HarryPotter",
    "serviceName": "onlineBookStore"
  }'
# Response: {"allowed":true,"reason":0}
```

**all-granted-roles** — Get all roles granted to a subject:

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/all-granted-roles \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "Alan"}]},
    "serviceName": "onlineBookStore"
  }'
# Response: ["role1", "role2"]
```

**all-granted-permissions** — Get all permissions granted to a subject:

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/all-granted-permissions \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "Alan"}]},
    "serviceName": "onlineBookStore"
  }'
# Response: [{"resource":"/books/HarryPotter","actions":["download","read"]}, ...]
```

### Evaluation Modes

Speedle supports two deployment modes:

- **As a Service** — ADS runs as a standalone server accessed via REST/gRPC
- **Embedded (In-Process)** — ADS runs as a Go library inside your application

---

## Advanced Features

### Policy Discovery

Discover mode helps you understand what authorization requests your system makes, then generate policies from them — without writing policies manually.

**Workflow:**

1. Replace `is-allowed` calls with `discover` calls (swap the endpoint URL)
2. Run your test suite or access protected resources
3. Generate policies from discovered requests:
   ```bash
   spctl discover policy --service-name=YOUR_SERVICE_NAME > service.json
   ```
4. Import the generated policies:
   ```bash
   spctl create service --json-file service.json
   ```
5. Switch back from `discover` to `is-allowed`

**Discover commands:**

```bash
# List all request details for all services
spctl discover request

# List requests for a specific service
spctl discover request --service-name="foo"

# Continuously watch the latest request
spctl discover request --last --service-name="foo" -f

# Generate policies from discovered requests
spctl discover policy --service-name="foo"

# Reset/cleanup all requests
spctl discover reset
spctl discover reset --service-name="foo"
```

### Policy Diagnosis

When an authorization decision doesn't match your expectations, use the diagnose API to see exactly which policies were evaluated and why.

Replace `is-allowed` with `diagnose` in the REST URL (keep the same request body):

```bash
curl -X POST -H "Content-Type: application/json" \
  http://localhost:6734/authz-check/v1/diagnose \
  -d '{
    "subject": {"principals": [{"type": "user", "name": "user1"}]},
    "serviceName": "srv1",
    "action": "get",
    "resource": "/api/v1/example/res1"
  }'
```

The response includes:
- The evaluated policies with `status` (`takeEffect` or `ignored`)
- Built-in attributes resolved at request time
- The final decision and reason

### Global Policy

Policies defined in a special `global` service take effect across ALL services, avoiding duplication.

```bash
# Create the global service
spctl create service global

# Create a global role policy (applies to all services)
spctl create rolepolicy -c "grant user Emma AdminRole" --service-name=global

# Create an ordinary service with a policy that references the global role
spctl create service library
spctl create policy -c "grant role AdminRole borrow books" --service-name=library

# The global role policy now takes effect in the "library" service scope
```

> **Note:** Global authorization policies are not currently supported. Only global **role** policies work.

### Identity Domain

Speedle supports user identities from multiple identity domains (e.g., different IdPs). Use the `from` keyword to scope a policy to a specific identity domain.

```bash
# Policy scoped to github identity domain
spctl create policy -c "grant user user1 from github read book" --service-name=booksvc

# Policy scoped to a specific tenant in IDCS
spctl create policy -c "grant user user1 from IDCS.tenant01 read book" --service-name=booksvc

# Policy matching ANY identity domain (no 'from' clause)
spctl create policy -c "grant user user1 rent book" --service-name=booksvc
```

**Evaluation rules:**
- Policy WITH identity domain → IDD must strictly match
- Policy WITHOUT identity domain → matches principals from any domain

### Token Asserter

When authorization requests carry an identity token (JWT, OAuth, etc.), Speedle can invoke a configurable webhook to validate the token and extract user identities — your service doesn't need to handle token validation.

**Configuration in config.json:**

```json
{
  "asserterWebhookConfig": {
    "endpoint": "http://localhost:8080/v1/assert",
    "clientCert": "",
    "clientKey": "",
    "caCert": ""
  }
}
```

The asserter service must implement the [Token Assertion Plugin API](https://github.com/teramoby/speedle-plus/tree/master/api/asserter).

**Example request with token:**

```bash
curl -X POST -H "Content-Type: application/json" \
  http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -d '{
    "subject": {"token": "githubtoken", "tokenType": "github"},
    "serviceName": "booksvc",
    "resource": "book",
    "action": "read"
  }'
```

### Custom Function

When built-in functions aren't enough, you can write your own functions and use them in policy conditions.

1. **Implement your function as a REST endpoint** accepting POST with JSON body `{"params": [...]}` and returning `{"result": ..., "error": ""}`
2. **Register the function** in Speedle:
   ```bash
   spctl create function isValid \
     --func-url=https://localhost:23456/func/isValid \
     --cachable=true \
     --cache-ttl=300
   ```
3. **Use in conditions:**
   ```bash
   spctl create policy -c "grant user Ally access library if isValid(attr1)" --service-name=service1
   ```

---

## Storage

Speedle supports pluggable policy stores. OOTB stores:

| Store | Use Case |
|-------|----------|
| **File** | Local development and testing |
| **etcd** | Production, multi-node deployments |
| **MongoDB** | Production, document-oriented storage |

### File Store

```bash
speedle-pms --store-type file --filestore-loc /path/to/policies.json
```

### etcd Store

```bash
speedle-pms --store-type etcd --etcdstore-endpoint localhost:2379
```

TLS-enabled etcd:

```bash
speedle-pms --store-type etcd \
  --etcdstore-endpoint https://etcd-cluster:2379 \
  --etcdstore-tls-cert /path/to/etcd-client.crt \
  --etcdstore-tls-key /path/to/etcd-client.key \
  --etcdstore-tls-ca /path/to/etcd-ca.crt
```

### Implementing a Custom Store

1. Implement the `PolicyStoreManager` interface (including `Watch` for change notifications)
2. Implement the `StoreBuilder` interface
3. Register via `store.Register()` in an `init()` function
4. Import the store package in `cmd/speedle-pms/stores.go` and `cmd/speedle-ads/stores.go`

See the [etcd store](pkg/store/etcd/) for a reference implementation.

> **Note:** The data store must support the `watch` function so ADS can receive real-time policy updates.

---

## Security & TLS

### TLS Configuration

Speedle uses TLS to secure API communications. TLS is **enabled by default**.

**Server flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--insecure` | Disable TLS (`true` / `false`) | `false` |
| `--cert` | TLS certificate path | — |
| `--key` | TLS private key path | — |
| `--client-cert` | Client CA certificate | — |
| `--force-client-cert` | Require mutual TLS | `false` |

**Start with TLS enabled (production):**

```bash
speedle-pms --store-type file \
  --insecure=false \
  --key=/etc/speedle/server.key \
  --cert=/etc/speedle/server.crt \
  --client-cert=/etc/speedle/client-ca.crt

speedle-ads --store-type file \
  --insecure=false \
  --force-client-cert=true \
  --key=/etc/speedle/server.key \
  --cert=/etc/speedle/server.crt \
  --client-cert=/etc/speedle/client-ca.crt
```

**Generate self-signed certificates with cfssl (for testing):**

```bash
# Install cfssl
go get -u github.com/cloudflare/cfssl/cmd/cfssl
go get -u github.com/cloudflare/cfssl/cmd/cfssljson

# Generate CA
echo '{"CN":"CA","key":{"algo":"rsa","size":2048}}' | cfssl gencert -initca - | cfssljson -bare ca -

# Generate server cert
echo '{"signing":{"default":{"expiry":"43800h","usages":["signing","key encipherment","server auth","client auth"]}}}' > ca-config.json
export ADDRESS=localhost,127.0.0.1
export NAME=server
echo '{"CN":"'$NAME'","hosts":[""],"key":{"algo":"rsa","size":2048}}' | cfssl gencert -config=ca-config.json -ca=ca.pem -ca-key=ca-key.pem -hostname="$ADDRESS" - | cfssljson -bare $NAME

# Generate client cert
export ADDRESS=
export NAME=client
echo '{"CN":"'$NAME'","hosts":[""],"key":{"algo":"rsa","size":2048}}' | cfssl gencert -config=ca-config.json -ca=ca.pem -ca-key=ca-key.pem -hostname="$ADDRESS" - | cfssljson -bare $NAME
```

**spctl with TLS:**

```bash
spctl config skipverify false \
  cacert /path/to/server-ca.crt \
  cert /path/to/client.crt \
  key /path/to/client.key \
  pms-endpoint "https://localhost:6733/policy-mgmt/v1/"
```

**curl with TLS:**

```bash
curl --cacert /path/to/server-ca.crt \
  --cert /path/to/client.crt \
  --key /path/to/client.key \
  -X POST -H "Content-Type: application/json" \
  -d '{"subject":{...},"serviceName":"mysvc","resource":"res1","action":"get"}' \
  https://localhost:6734/authz-check/v1/is-allowed
```

### Authorization for Management Endpoints

PMS management endpoints have **no built-in authentication**. In production:
- Bind PMS to localhost: `--endpoint=127.0.0.1:6733`
- Use a reverse proxy with authentication (e.g., API Gateway with tokens)
- Restrict network access via firewall

---

## Logging

Speedle uses [Logrus](https://github.com/sirupsen/logrus) (structured logging) + [Lumberjack](https://github.com/natefinch/lumberjack) (log rotation).

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--log-level` | panic, fatal, error, warn, info, debug | `info` |
| `--log-formatter` | text, json | `text` |
| `--log-reportcaller` | Include file/line/function | `false` |
| `--log-filename` | Write to file (else stderr) | — |
| `--log-maxsize` | Max MB before rotation | `100` |
| `--log-maxbackups` | Max old files to keep | `0` (unlimited) |
| `--log-maxage` | Max days to retain | `0` (unlimited) |
| `--log-compress` | Gzip rotated files | `false` |

### Config file

```json
{
  "logConfig": {
    "level": "info",
    "formatter": "json",
    "rotationConfig": {
      "filename": "/var/log/speedle/speedle.log",
      "maxSize": 10,
      "maxBackups": 5,
      "maxAge": 30
    }
  }
}
```

### Audit Log

Separate audit log configuration with `--auditlog-*` flags (same options as log flags, prefixed with `auditlog-`).

### Docker / Container Logging

When running in Docker, use Docker's `json-file` logging driver instead of Lumberjack:

```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "20m", "max-file": "10" }
}
```

---

## Deployment

### As a Go Library (Embedded Mode)

Import Speedle into your Go application for in-process authorization:

```go
import (
    "github.com/teramoby/speedle-plus/api/pms"
    "github.com/teramoby/speedle-plus/api/ads"
)
```

### On Kubernetes

Deploy using Helm or manually. Two modes:

- **Dev mode** — file-based store, single deployment
- **Production mode** — etcd-backed, separate PMS/ADS deployments (3 replicas each)

**Dev mode with Helm:**

```bash
helm install -n speedle-dev deployment/helm/speedle-dev
kubectl port-forward svc/speedle-dev 6733:6733 6734:6734
```

**Production mode with Helm:**

```bash
# Install etcd operator
helm install stable/etcd-operator --name my-release
kubectl create -f example-etcd-cluster.yaml

# Deploy Speedle
helm install -n speedle deployment/helm/speedle-prod \
  --set store.etcd.endpoint=http://example-etcd-cluster-client:2379
```

Manual deployment YAML files: [`deployment/k8s/`](deployment/k8s/)

---

## Integrations

### Kubernetes Authorization Webhook

Speedle can serve as a Kubernetes [authorization webhook](https://kubernetes.io/docs/reference/access-authn-authz/webhook/), evaluating Kubernetes API requests against Speedle policies.

See [`samples/integration/kubernetes-integration`](samples/integration/kubernetes-integration/) for the webhook implementation.

**Example policy protecting pods:**

```bash
spctl create service kubernetes

# Allow user "joe" to get pods with label component=etcd
spctl create policy joe-pod --service-name=kubernetes \
  -c "grant user joe get expr:/pods/.* if labels_component == \"etcd\""
```

### Istio Mixer Adapter

Integrate Speedle with Istio's policy enforcement layer. See [Istio integration docs](https://speedle.io/integrations/istio/).

### Docker Authorization Plugin

Use Speedle as a Docker authorization plugin to control container operations. See [Docker integration docs](https://speedle.io/integrations/docker/).

---

## API Reference

Speedle provides three API surfaces:

### REST APIs

| API | Endpoint | Swagger |
|-----|----------|---------|
| Policy Management | `http://host:6733/policy-mgmt/v1/` | [`swagger/policy-manage.yaml`](swagger/policy-manage.yaml) |
| Authorization Decision | `http://host:6734/authz-check/v1/` | [`swagger/policy-check.yaml`](swagger/policy-check.yaml) |
| Token Assertion | (user-defined) | [`swagger/asserter.yaml`](swagger/asserter.yaml) |

### gRPC APIs

| API | Proto |
|-----|-------|
| Policy Management | [`protobuf/pms.proto`](protobuf/pms.proto) |
| Authorization Decision | [`protobuf/ads.proto`](protobuf/ads.proto) |

### Go APIs (Embedded)

| API | Package |
|-----|---------|
| Policy Management | [`api/pms`](api/pms/) |
| Authorization Decision | [`api/ads`](api/ads/) |

---

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
    "asserterWebhookConfig": {
        "endpoint": "http://localhost:8080/v1/assert",
        "clientCert": "",
        "clientKey": "",
        "caCert": ""
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

Configuration values can come from three sources (in priority order):
1. **Command-line flags** (highest)
2. **Environment variables** (prefixed with `SPDL_`, hyphens → underscores, uppercase)
3. **Config file** (lowest)

---

## Build

### Prerequisites

- Go 1.22 or greater: <https://go.dev/doc/install>

### From Source

```bash
git clone https://github.com/teramoby/speedle-plus.git
cd speedle-plus
make build
```

### Via `go install`

```bash
go install github.com/teramoby/speedle-plus/cmd/spctl@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
```

## Test

```bash
cd speedle-plus
make test
```

---

## Use Cases

### Application-Specific Policies

A developer building a Go application can embed Speedle for authorization rather than reinventing access control logic.

### Centralized Policy Enforcement

A security team managing dozens of internal systems can use Speedle as a single authorization engine with:
- Centralized policy management
- One flexible policy model for all systems
- Easy integration with OS, middleware, and applications

### Unified Authorization Across the Stack

A cloud platform offering Linux, Docker, Kubernetes, and Istio can define a single authorization model in Speedle and enforce it consistently across all layers.

---

## Community & Contributing

### Get Help

- Join us on Slack: [#speedle-users](https://join.slack.com/t/speedleproject/shared_invite/zt-72fgiyuo-QKJAhHAqVbn17KRFbd7aZw)
- Mailing List: speedle-users@googlegroups.com
- QQ Group (中文): 643201591

### Get Involved

1. Star the project on [GitHub](https://github.com/teramoby/speedle-plus)
2. Tell us how you use Speedle+ in your projects
3. File [issues](https://github.com/teramoby/speedle-plus/issues) for bugs or feature requests
4. Contribute code — see [CONTRIBUTING.md](CONTRIBUTING.md)

### Code of Conduct

Follow the [Golden Rule](https://en.wikipedia.org/wiki/Golden_Rule). See [Contributor Covenant](https://www.contributor-covenant.org/version/1/4/code-of-conduct.html).

---

## Documentation

- Latest documentation: <https://speedle.io/docs>
- Go package reference: <https://pkg.go.dev/github.com/teramoby/speedle-plus>
