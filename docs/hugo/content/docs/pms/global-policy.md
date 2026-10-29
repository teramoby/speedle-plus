+++
title = "Global Policy"
description = "Policies take effect globally"
date = 2019-01-18T21:19:44+08:00
weight = 2
draft = false
bref = "When there are policies that are common to all services, it's tedious and difficult to manage if the same policies are created in each service. In this scenario, you can use global policies to simplify your policy management tasks"
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
+++

## When should a role be global?

Ordinary roles are scoped to a service. When several services share a role assignment, define the assignment in the specially named `global` service and use that role in each service's own authorization policies.

Only global **role policies** are supported. A global role assignment does not itself grant every operation; each ordinary service still defines its permissions. Global authorization policies are not supported.

## Before running the examples

Build `./bin/spctl`, `./bin/speedle-pms` and `./bin/speedle-ads` using the [quick start](/quick-start/). Use Go 1.25 or later, Bash, curl and Python 3. Run each terminal from the repository root. Use a fresh policy file for this page; reset it only after stopping both services.

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


## Create a global role and a local permission

<!-- docs-check: global-fixture -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create rolepolicy -c "grant user Emma AdminRole" --service-name global
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service library
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy -c "grant role AdminRole borrow books" --service-name library
```


`global` must use exactly that service name. `library` is created before its policy. Both are managed through normal service APIs.

## Check the result

Emma is allowed through `AdminRole`; Bob is denied.

<!-- docs-check: global-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Emma"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


<!-- docs-check: global-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"library","resource":"books","action":"borrow"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```


A resource-scoped global role policy applies only to its resource scope. Keep cross-service role assignments under review, especially when identities span [different domains](/docs/idd/).
