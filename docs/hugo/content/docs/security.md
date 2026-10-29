+++
title = "Security"
description = "Secure your Speedle instances"
weight = 310
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["security", "guide", "tls", "https"]
categories = ["docs"]
bref = ""
+++

## Protect both API transports

Speedle's standalone PMS and ADS expose **REST and gRPC**. Both now use the same `--insecure`, `--cert`, `--key`, `--client-cert` and `--force-client-cert` transport settings. Invalid certificates or trust configuration cause startup to fail; secure mode does not fall back to plaintext.

TLS encrypts the connection and authenticates the server. To require authenticated clients, provide a client CA with `--client-cert` **and** set `--force-client-cert=true`. Without that flag, a configured CA validates a client certificate only when the client supplies one. TLS alone does not authorize an administrator or provide bearer-token validation. Use an authenticated gateway or equivalent access controls for management, diagnosis and discovery endpoints. The `--enable-authz` flag does not provide a built-in endpoint authorization policy.

## Listen addresses and compatibility

| Service | Default REST address | Default gRPC address |
| --- | --- | --- |
| PMS | `0.0.0.0:6733` | disabled until `--grpc-endpoint` is set |
| ADS | `0.0.0.0:6734` | disabled until `--grpc-endpoint` is set |

REST and gRPC listen addresses are configured separately. Setting `--endpoint` alone does not enable gRPC. Explicitly use `--grpc-endpoint 127.0.0.1:50001` for PMS or `127.0.0.1:50002` for ADS. `127.0.0.1:0` requests an ephemeral port for isolated tests; specify an IPv6 address explicitly when needed.

Configuration precedence is command line, then `SPDL_*` environment variables, then JSON, then defaults. The gRPC overrides are `--grpc-endpoint`, `SPDL_GRPC_ENDPOINT`, and `serverConfig.grpcEndpoint`. See the [current proto contracts](/docs/api/) for RPC namespaces and client generation.

## Generate certificates for a local test

Build the binaries using the [quick start](/quick-start/). From the repository root, use Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch) to generate a local CA plus separate server/client certificates. The server certificate has SANs for `localhost`, `127.0.0.1` and `::1`; private keys use mode 0600. These **24-hour demonstration certificates** are not production credentials. The generator refuses to overwrite existing files: choose a new output directory when regenerating.

<!-- docs-check: certificates -->
```bash
go run ./website/tests/examples/certificates --out ./tls
```


## Start mutually authenticated services

Initialize a fresh policy file once, with both services stopped:

<!-- docs-check: setup -->
```bash
printf '{}\n' > docs-policies.json
```


Start PMS in one terminal:

<!-- docs-check: secure-pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json \
  --endpoint 127.0.0.1:6733 --grpc-endpoint 127.0.0.1:50001 \
  --cert ./tls/server.crt --key ./tls/server.key \
  --client-cert ./tls/ca.crt --force-client-cert=true
```


Start ADS in a second terminal:

<!-- docs-check: secure-ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json \
  --endpoint 127.0.0.1:6734 --grpc-endpoint 127.0.0.1:50002 \
  --cert ./tls/server.crt --key ./tls/server.key \
  --client-cert ./tls/ca.crt --force-client-cert=true
```


Run both terminals from the same repository root. Keep them open; stop each with Ctrl-C when finished. Production certificates must cover the actual DNS names/IPs and come from your organization's trusted CA.

## Connect with spctl and curl

<!-- docs-check: secure-cli -->
```bash
./bin/spctl --pms-endpoint https://127.0.0.1:6733/policy-mgmt/v1/ \
  --skipverify=false --cacert ./tls/ca.crt --cert ./tls/client.crt --key ./tls/client.key \
  get service --all
```


<!-- docs-check: secure-curl -->
```bash
curl --fail --silent --show-error --cacert ./tls/ca.crt \
  --cert ./tls/client.crt --key ./tls/client.key \
  https://127.0.0.1:6733/policy-mgmt/v1/service
```


An empty store returns `[]`. Omitting the client certificate fails the TLS handshake. `spctl` uses `--cacert` for server trust and `--cert`/`--key` for its own identity; `--client-cert` and `--force-client-cert` are server flags. Keep `--skipverify=false` (the default).

For gRPC, configure the generated client's channel with the same CA, a matching server name, and the client certificate/key. PMS uses `pb.PolicyManager`; ADS uses `pb.Evaluator`. An insecure channel must fail against these TLS endpoints.

## Local plaintext development

Only an explicit `--insecure=true` enables plaintext for both transports. Bind REST and gRPC to loopback for that mode. Do not publish these unauthenticated endpoints or trust principal names/attributes supplied by untrusted callers. The [token asserter](/docs/assertor/) validates a configured identity token; it does not authenticate every PMS/ADS caller or replace endpoint access controls.
