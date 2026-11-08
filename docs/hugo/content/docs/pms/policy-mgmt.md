+++
title = "Policy Management"
description = "Manage policy lifecycle "
weight = 1
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["pms", "policy", "core"]
categories = ["docs"]
bref = "Basics of policy management"
+++

## What is a Speedle policy?

The Policy Management Service (PMS) manages services, authorization policies, role policies and custom functions. A policy store persists those objects using a registered [storage backend](/docs/store/).

<img src="/img/speedle/policystore.png" width="1942" height="540" loading="lazy"/>

## Services and authorization policies

A service scopes its policies. An authorization policy grants or denies a principal permission to perform an action on a resource, optionally subject to a condition. Except for [global role policies](/docs/pms/global-policy/), policies do not automatically apply in another service.

<img src="/img/speedle/authzpolicy.png" width="1770" height="444" loading="lazy"/>

```text
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
```

This regular expression matches resources under `c1/default/core/pods/`. It does not match `c1/default/core/podssecret` or an extra prefix before `c1`. `expr:` uses regular-expression matching, not shell globs: use `.*` for arbitrary characters and `^`/`$` when the complete identifier must match.

## Role policies

<img src="/img/speedle/rolepolicy.png" width="1598" height="436" loading="lazy"/>

```text
grant user alan manager on res1
grant role manager update res1
```

The first statement assigns Alan a resource-scoped role; the second grants that role an operation. Assigning a role alone does not create permissions. An applicable explicit deny overrides a grant.

## Principals and conditions

A principal may be a user, group, entity or role. An entity represents a non-human identity, such as a service. Integrations must derive these identities from trusted sources.

<img src="/img/speedle/principal.png" width="1769" height="505" loading="lazy"/>

An AND group requires every listed principal; separate groups are alternatives. For example, both roles below must apply:

<img src="/img/speedle/andprincipal.png" width="1584" height="349" loading="lazy"/>

```text
grant (role designer, role dba) update db_design_doc
```

A resource names the protected object; an action names an application operation. A condition is a Boolean expression evaluated from trusted attributes, constants, operators and supported functions.

<img src="/img/speedle/resource.png" width="1732" height="349" loading="lazy"/>

See [SPDL](/docs/spdl/) for grammar and [RBAC](/docs/overview/)/[ABAC](/docs/overview/) for complete decision examples.

## Before running the examples

Build `./bin/spctl`, `./bin/speedle-pms` and `./bin/speedle-ads` using the [quick start](/quick-start/). Use Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch), Bash, curl and Python 3. Run each terminal from the repository root. Use a fresh policy file for this page; reset it only after stopping both services.

Initialize an empty policy file:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

> **macOS:** `/usr/sbin/spctl` is Apple's Gatekeeper tool and may be selected when you run a bare `spctl` command. Call Speedle's CLI by its full path, or use `./spctl` from the directory containing it; the examples below use `./bin/spctl`. Run `command -v spctl` to check which executable your shell selects. If the release you download includes a `speedle-spctl_<os>_<arch>` asset, it is the same CLI under a name that does not collide with Apple's tool.

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


## Manage services and policies with spctl

Run this sequence against a fresh store. IDs come from the live query responses; they are not fixed values copied from an older run. Deletion happens after all operations that need the service.

<!-- docs-check: policy-management -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy policy1 -c "grant user alan read book" --service-name test
policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="policy1"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy rolepolicy01 -c "grant user alan manager on res1" --service-name test
role_policy_id=$(./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy --all --service-name test | python3 -c 'import json,sys; print(next(p["id"] for p in json.load(sys.stdin) if p.get("name")=="rolepolicy01"))')
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete policy "$policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete rolepolicy "$role_policy_id" --service-name test
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ delete service test
```


## Speedle Cloud

Speedle Cloud sign-in opens soon. When API tokens become available, `spctl` can manage Cloud services and policies through the same commands used with a self-hosted PMS. Use the management endpoint:

```text
https://cloud.speedle.io/policy-mgmt/v1
```

Choose an API token with the scopes required by your commands. `pms:read` covers `get` and `discover` reads; `pms:write` covers `create`, `delete` and discovery resets. A token that only has ADS scopes cannot manage policies and is rejected with `403 insufficient_scope`.

Pass the endpoint as a global flag and read the token from `SPCTL_TOKEN`. In an interactive shell, this keeps the secret out of command arguments and shell history:

```bash
read -rsp 'API token: ' SPCTL_TOKEN; echo
export SPCTL_TOKEN
./bin/spctl --pms-endpoint https://cloud.speedle.io/policy-mgmt/v1 get service --all
unset SPCTL_TOKEN
```

The `--token` flag remains available for compatibility, but the environment variable avoids exposing the token through command arguments. Every global flag also has an environment variable: prefix `SPCTL_`, uppercase the flag name and replace hyphens with underscores. For example, `SPCTL_PMS_ENDPOINT` sets `pms-endpoint` and `SPCTL_SKIPVERIFY` sets `skipverify`.

For a saved configuration, use `~/.speedle`. Its format is one `name=value` pair per line, with the same flag names as the command line:

```ini
pms-endpoint=https://cloud.speedle.io/policy-mgmt/v1
token=<api-token>
```

Create or update this file without putting the token in command arguments:

```bash
read -rsp 'API token: ' SPCTL_TOKEN; echo
printf '%s\n' "$SPCTL_TOKEN" | ./bin/spctl config token --stdin
unset SPCTL_TOKEN
```

`spctl config token --stdin` writes `~/.speedle` with mode `0600`. When editing the file manually, create it with `install -m 600 /dev/null ~/.speedle` or run `chmod 0600 ~/.speedle` afterwards. `spctl` warns on standard error if it reads a token from a file whose permissions allow group or other access. `spctl config --list` masks a configured token as `********`.

Explicit command-line flags override environment variables, which override both configuration files. Within the files, `~/.speedle` overrides `~/.spctlconfig`. Setting a non-token key with `spctl config` writes it to `~/.spctlconfig` and removes any shadowing copy from `~/.speedle`.

The token is sent as an `Authorization: Bearer` credential only over HTTPS with certificate verification enabled. Plain HTTP is allowed with a token only for a loopback host such as `127.0.0.1`, `::1` or `localhost`; `spctl` rejects remote HTTP endpoints and HTTPS-to-HTTP downgrade redirects before sending the credential. It also refuses to send a token with `--skipverify=true`; use `--cacert` to trust a private or self-signed certificate authority instead. Tokens are not included in normal command output or error messages.

Cloud errors produce a non-zero exit status. Check the token after a `401`, and its scopes after a `403 insufficient_scope`. A `404` identifies a missing resource, while `409 service_exists` and `409 policy_exists` identify duplicates. For `429`, honor the returned `Retry-After` value and use bounded backoff. Daily or policy quota errors require waiting for the reset or reducing usage; do not retry them in a tight loop.


## Other interfaces

The [PMS REST API](/docs/api/management_api/) uses JSON and returns flat policy objects with string IDs. Go applications can call the [PMS interfaces](https://github.com/teramoby/speedle-plus/tree/master/api/pms) directly; gRPC clients use the [published PMS proto](/protobuf/pms.proto). Apply [transport and caller access controls](/docs/security/) to administrative operations.
