+++
title = "Identity Domain"
description = "Speedle supports user identities from multiple identity domains"
date = 2019-01-17T13:20:07+08:00
weight = 40
draft = false
bref = ""
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
tags = ["identity domain", "identity"]
+++

## What is an identity domain?

An identity domain (`idd`) is an explicit namespace for a principal. Two users named `user1` can belong to different domains. The string is supplied by trusted integration code or a configured token asserter; it is not proof of authentication. Speedle does not infer or validate a provider's tenant model merely from the domain name.

## Represent domains in policies and requests

```text
grant user user1 from github read book
grant user user1 from IDCS.tenant01 read book
```

Use `from <identity-domain>` immediately after the principal name. In a PMS JSON policy, encode the same principal as `idd=github:user:user1`. In a decision request, represent this principal as:

```json
{
  "type": "user",
  "name": "user1",
  "idd": "github"
}
```

A policy with `from github` matches that domain only. Omitting `from` matches that principal name across domains. Review this explicitly for multitenant applications; a domain string by itself does not validate a token. See [identity/IAM](/docs/access-control/identity-iam/) and [token assertion](/docs/assertor/).

## Before running the examples

Build `./bin/spctl`, `./bin/speedle-pms` and `./bin/speedle-ads` using the [quick start](/quick-start/). Use Go 1.26 or later, Bash, curl and Python 3. Run each terminal from the repository root. Use a fresh policy file for this page; reset it only after stopping both services.

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


## Load the test policies

The [fixture](https://github.com/teramoby/speedle-plus/blob/master/samples/docs/idd.json) grants `read` to GitHub's `user1`, `write` to Google's `user1`, and `rent` to `user1` from any domain. The lowercase domain values are demonstration namespaces.

<!-- docs-check: idd-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file samples/docs/idd.json
```


## Compare decisions

In order, expect Allow, Deny, Allow, Deny. A user in `notgoogle` cannot use Google's `write` grant.

<!-- docs-check: idd-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"github"}]},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"gitlab"}]},"serviceName":"booksvc","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-any -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"google"}]},"serviceName":"booksvc","resource":"book","action":"rent"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

<!-- docs-check: idd-write-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"user1","idd":"notgoogle"}]},"serviceName":"booksvc","resource":"book","action":"write"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


JSON requests require `Content-Type: application/json`. Management examples use string policy IDs and `permissions` (plural); see the [management API](/docs/api/management_api/).
