+++
title = "Policy Discovery"
description = "Discover policies on the fly!"
weight = 4
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["policy", "core", "discovery"]
categories = ["docs"]
bref = "Speedle can be used for authorization decisions in systems with a large number of protected resources. Before those protected resources can be accessed, the Speedle authorization decision API (is-allowed API) is called. Defining the authorization policies for those protected resources is difficult if you don't know what actions are occuring on the protected resources, or how a protected resource is expressed in an authorization decision request. Policy discovery mode can make this task much easier"
+++

## What discovery does

Discovery records requests and returns `allowed=true` with reason 5 for a successful recording operation. It does not prove that an existing policy authorized the operation. Use it only in an isolated test environment, review generated grants, and restore normal `is-allowed` enforcement afterwards. Assertion or storage errors must still be handled; never treat an error as an authorization grant.

## Start a reproducible local example

Build the binaries using [Quick Start](/quick-start/), then run all terminals from the checkout root. Use a fresh file and stop previous page fixtures before starting.

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```

In terminal one:

<!-- docs-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

In terminal two:

<!-- docs-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

Keep both running. In terminal three, record one request:

<!-- docs-check: discover-record -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/discover
```

Inspect the captured request, then generate a proposed service definition:

<!-- docs-check: discover-list -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover request --service-name observed
```
<!-- docs-check: discover-export -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ discover policy --service-name observed > discovered-service.json
```

Review `discovered-service.json`, including generated roles, identity domains and resources. The fresh example has no existing service named `observed`, so it can be imported directly; for an existing service, reconcile its policies rather than blindly recreating it.

<!-- docs-check: discover-import -->
```bash
./bin/spctl --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service --json-file discovered-service.json
```

## Restore and verify normal enforcement

Call `is-allowed` again. Alice should be allowed after ADS has loaded the policy update; Bob must still be denied:

<!-- docs-check: discover-allow -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```
<!-- docs-check: discover-deny -->
```bash
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"observed","resource":"book","action":"read"}' \
  http://127.0.0.1:6734/authz-check/v1/is-allowed
```

Stop both services with Ctrl+C. For gRPC use `Discover`/`IsAllowed` on the generated client; embedded Go uses `evaluator.Discover(ctx)`/`evaluator.IsAllowed(ctx)`. For CLI options, run `./bin/spctl discover --help`. Do not leave a real application pointing at `/discover`.
