+++
title = "Policy Diagnosis"
description = "Authorization policy diagnosis"
date = 2019-01-18T15:46:59+08:00
weight = 3
draft = false
bref = "This feature is used to diagnose the evaluation process of authorization. When a user isn't allowed to operate on a resource, then this feature is useful to find out the reason, i.e., what's the policy which denies the operation on the resource."
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## What does diagnosis show?

`Diagnose` explains an authorization evaluation: decision, reason, effective roles, attributes, and the policies examined. It accepts the same request context as `IsAllowed`; REST uses `/authz-check/v1/diagnose`, and both Go/gRPC use `Diagnose`.

Diagnosis exposes policy definitions and request context. Restrict it to trusted operators using the [API security controls](/docs/security/); it is not a public debugging endpoint.

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


## Create the example policies

The [fixture](https://github.com/teramoby/speedle-plus/blob/master/website/tests/examples/diagnosis.json) grants `get` under `^/api/v1/example/.*$` but explicitly denies `/api/v1/example/res1` for `user1`.

<!-- docs-check: diagnosis-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file website/tests/examples/diagnosis.json
```


## Compare decision and diagnosis

<!-- docs-check: diagnosis-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


Expected: `allowed=false`, `reason=1` (explicit deny).

<!-- docs-check: diagnose -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1"}]},"serviceName":"srv1","resource":"/api/v1/example/res1","action":"get"}' \
  http://127.0.0.1:6734/authz-check/v1/diagnose
```


The diagnostic response uses `allowed` as a Boolean, `reason` as a string such as `DENY_POLICY_FOUND`, and `attributes` as an object. `request_month` is a numeric month. `requestContext.subject.principals` is an array; authorization policies contain a two-dimensional `principals` array representing OR-of-AND principal groups.

| Field | Meaning |
| --- | --- |
| `requestContext` | The request used in evaluation; REST attributes remain a typed array here |
| `attributes` | Available request and built-in attributes, keyed by name |
| `grantedRoles` | Effective roles, when present |
| `rolePolicies`, `policies` | Evaluated role and authorization policy details, when present |
| `condition` | Expression and result; may be empty for unconditional policies |

Policy status values are `takeEffect`, `conditionFailed` and `ignored`. `ignored` means the final result was already determined, so that policy did not need evaluation. Ordering can vary; inspect IDs and status rather than matching a historical response verbatim. See the [decision schema](/docs/api/decision_api/) for the complete response contract.
