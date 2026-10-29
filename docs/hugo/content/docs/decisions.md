+++
title = "Authorization Decisions"
description = "Get authorization decisions for your service interactions"
weight = 30
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pdp", "policy", "core"]
categories = ["docs"]
bref = "Get authorization decisions"
+++

## What is an authorization decision?

Speedle evaluates a subject, action and resource within a service. The application must enforce the result. Treat an evaluation error or timeout separately from a normal Deny and fail closed. REST, gRPC and embedded Go APIs have different wire representations; use their respective [contracts](/docs/api/).

## Before running the examples

Build `./bin/spctl`, `./bin/speedle-pms` and `./bin/speedle-ads` using the [quick start](/quick-start/). Use Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch), Bash, curl and Python 3. Run each terminal from the repository root. Use a fresh policy file for this page; reset it only after stopping both services.

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
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:0 --insecure=true
```

Run the remaining commands in a third terminal. PMS and ADS share the file; watch propagates changes asynchronously. Confirm the intended update has reached ADS before relying on it. `--insecure=true` is only for this loopback example; see [security](/docs/security/) for production connections. Explicit PMS addresses avoid changing `~/.spctlconfig`. Stop both server terminals with Ctrl-C when finished.


## Create a reproducible policy set

The [fixture](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/bookstore.json) assigns Alan `role1` and `role2`; `role1` may download `/books/HarryPotter`.

<!-- docs-check: bookstore-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/bookstore.json
```


## Get a decision

Alan is allowed; Bob is denied because no grant applies.

<!-- docs-check: decision-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


<!-- docs-check: decision-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


Successful decisions return `{"allowed":true,"reason":0}` or a corresponding Deny object. HTTP errors return an error response, not a successful grant. Reason codes are:

| Code | Meaning |
| --- | --- |
| 0 | GRANT_POLICY_FOUND |
| 1 | DENY_POLICY_FOUND |
| 2 | SERVICE_NOT_FOUND |
| 3 | NO_APPLICABLE_POLICIES |
| 4 | ERROR_IN_EVALUATION |
| 5 | DISCOVER_MODE |
| 6 | REASON_NOT_AVAILABLE |

`DISCOVER_MODE` is not a production authorization grant. `REASON_NOT_AVAILABLE` is the fallback reason value. Do not assume these numbers are HTTP status codes.

## Get granted roles

<!-- docs-check: decision-roles -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-roles
```


Expected roles are `role1` and `role2` (ordering is not significant). Include `resource` when querying resource-scoped role assignments.

## Get granted permissions

<!-- docs-check: decision-permissions -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  http://127.0.0.1:6734/authz-check/v1/all-granted-permissions
```


Expected permission: resource `/books/HarryPotter`, action `download`. This API describes effective granted permissions; enforce a particular operation with `is-allowed` rather than treating a permission-list response as a replacement for request evaluation.

## Attributes, diagnosis and token identity

REST request attributes are typed objects in an array. For example, an attribute item is:

```json
{
  "name": "clearance",
  "type": "numeric",
  "value": 4
}
```

Embedded Go attributes are `map[string]interface{}`; diagnostic response attributes are also an object, not that request array. See [SPDL](/docs/spdl/) for types and conditions, [diagnosis](/docs/pms/diagnosis/) for evaluation detail, and [token assertion](/docs/assertor/) for configured token validation.
