+++
title = "etcd Deployment"
description = "Deploy etcd policy storage for PMS and ADS, step by step."
weight = 20
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "etcd"]
categories = ["docs"]
bref = "Run an independent etcd server, connect PMS and ADS, verify live policy updates, and save a backup."
+++

This walkthrough uses an **external etcd v3.6.14** server with a persistent Docker volume. PMS writes policies and ADS watches the same key prefix. The single-node example listens on loopback and uses plain HTTP for local development. Production TLS and cluster configuration follow the walkthrough.

## 1. Build Speedle

Prepare Bash, Git, curl, Docker and Go 1.26.10 or Go 1.27.3. Start Docker, then build the native services:

<!-- etcd-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Run subsequent commands from this checkout. Keep local ports 2379, 6733 and 6734 available, or update the commands and configuration together.

## 2. Start an external etcd server

The named volume holds etcd's data. Only the client port is published to the host; the peer port remains inside this single-node container.

<!-- etcd-step: database -->
```bash
docker run --name speedle-etcd --detach \
  --publish 127.0.0.1:2379:2379 \
  --volume speedle-etcd-data:/etcd-data \
  gcr.io/etcd-development/etcd:v3.6.14 \
  /usr/local/bin/etcd \
  --name speedle-local \
  --data-dir /etcd-data \
  --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://127.0.0.1:2379 \
  --listen-peer-urls http://0.0.0.0:2380 \
  --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster speedle-local=http://127.0.0.1:2380 \
  --initial-cluster-token speedle-local-demo \
  --initial-cluster-state new
```

Wait for the v3 API to become healthy:

<!-- etcd-step: ready -->
```bash
until docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 endpoint health
do
  sleep 1
done
```

If you already run etcd, skip the container commands and use its client endpoint below. See the [official container guide](https://etcd.io/docs/v3.6/op-guide/container/) for Docker networking details.

## 3. Configure the shared store

Create one configuration file for this local PMS/ADS pair:

<!-- etcd-step: configuration -->
```bash
mkdir -p etcd-demo
cat > etcd-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "http://127.0.0.1:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false
    }
  },
  "enableWatch": true
}
JSON
```

PMS and ADS must use the **same endpoint and key prefix**. Keep the trailing slash in `/speedle_ps/`; use a separate prefix for each independent policy store. `IsEmbeddedEtcd: false` connects to your external server. `enableWatch: true` enables ADS updates through etcd watch events; no polling interval is configured.

## 4. Start PMS

In terminal A, start PMS and leave it running:

<!-- etcd-step: pms -->
```bash
./bin/speedle-pms --config-file etcd-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

Wait for its REST listener to start. Plain HTTP is bound to loopback for this demo; see [Security](/docs/security/) for Speedle's own TLS and API authorization.

## 5. Create a policy

In terminal B, from the same checkout, grant Alice permission to read `book`:

<!-- etcd-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"etcd-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS returns the service and generated policy ID. The policy is now in etcd; no SQL schema or manual key initialization is needed.

## 6. Start ADS and verify authorization

In terminal C, start ADS with the same file and leave it running:

<!-- etcd-step: ads -->
```bash
./bin/speedle-ads --config-file etcd-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

Wait for startup, then run these checks in terminal B. Alice is allowed:

<!-- etcd-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

Expected: `{"allowed":true,"reason":0}`. Bob has no matching grant:

<!-- etcd-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"etcd-demo","resource":"book","action":"read"}'
```

Expected: `{"allowed":false,"reason":3}`.

## 7. Verify stored keys and automatic updates

Inspect only the keys in this policy store:

<!-- etcd-step: keys -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 get /speedle_ps/ --prefix --keys-only
```

The output includes keys for `etcd-demo` under `/speedle_ps/services/`. Treat the keys as backend-managed data and make policy changes through PMS.

Add a deny while ADS remains running:

<!-- etcd-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/etcd-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

Repeat Alice's request from step 6. After the asynchronous watch update, it should return `{"allowed":false,"reason":1}`. Repeat the request if synchronization is still in progress. ADS does not need a restart for this normal policy update.

## 8. Save a snapshot and verify persistence

Take a live snapshot, inspect it, and copy it outside the container:

<!-- etcd-step: snapshot -->
```bash
docker exec speedle-etcd /usr/local/bin/etcdctl \
  --endpoints=http://localhost:2379 snapshot save /etcd-data/speedle-snapshot.db
docker exec speedle-etcd /usr/local/bin/etcdutl \
  snapshot status /etcd-data/speedle-snapshot.db --write-out=table
docker cp speedle-etcd:/etcd-data/speedle-snapshot.db ./etcd-demo/speedle-snapshot.db
```

A snapshot contains the **whole etcd cluster's keyspace**, including other prefixes. Store backups on separate protected storage. See [Disaster recovery](https://etcd.io/docs/v3.6/op-guide/recovery/) for restore and membership procedures.

Stop PMS and ADS with Ctrl+C before this restart check:

<!-- etcd-step: restart -->
```bash
docker restart speedle-etcd
```

Run the health check in step 2, restart PMS and ADS with the same configuration, then repeat Alice's request. It should still return `{"allowed":false,"reason":1}`. The named volume retains data across the container restart.

For a disaster restore, stop policy writers, restore the snapshot into new data directories as described by etcd, and restart ADS so it reads the restored store before serving decisions. Recheck authorization after recovery; a previously running cache is not proof that it contains the restored policy set.

## Production: TLS, authentication and clusters

Deploy etcd with a trusted server certificate, a certificate SAN matching the endpoint hostname, and client-certificate validation. Then configure PMS/ADS to use HTTPS and their client credentials:

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "https://etcd.example.com:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": false,
      "EtcdTLSTrustedCAFile": "/etc/speedle/etcd/ca.crt",
      "EtcdTLSCertFile": "/etc/speedle/etcd/client.crt",
      "EtcdTLSKeyFile": "/etc/speedle/etcd/client.key",
      "EtcdTLSServerName": "etcd.example.com",
      "EtcdTLSInsecureSkipVerify": false
    }
  },
  "enableWatch": true
}
```

These paths are on the PMS/ADS machine. Restrict private-key access to the service account. TLS to etcd is configured separately from TLS on Speedle's REST/gRPC listeners.

The adapter exposes certificate options and does not expose etcd username/password properties. For etcd RBAC, use client-certificate Common Names as user identities with `--client-cert-auth=true` and authentication enabled. Grant PMS read/write access to its prefix and ADS the permissions its enabled features require. See the official [TLS](https://etcd.io/docs/v3.6/op-guide/security/) and [RBAC](https://etcd.io/docs/v3.6/op-guide/authentication/rbac/) guides for server configuration and roles. A key prefix groups data; access control requires RBAC.

For high availability, follow the official [three-member cluster example](https://etcd.io/docs/v3.6/op-guide/container/#running-a-3-node-etcd-cluster) and protect client and peer traffic. The single-node demo cannot tolerate loss of its only member. The current `EtcdEndpoint` property accepts one endpoint string; provide a reachable client URL for your deployment.

## Configuration reference and troubleshooting

| Property | Flag | Environment variable | Default |
| --- | --- | --- | --- |
| `EtcdEndpoint` | `--etcdstore-endpoint` | `SPDL_ETCDSTORE_ENDPOINT` | `localhost:2379` |
| `EtcdKeyPrefix` | `--etcdstore-keyprefix` | `SPDL_ETCDSTORE_KEYPREFIX` | `/speedle_ps/` |
| `IsEmbeddedEtcd` | `--etcdstore-isembedded` | `SPDL_ETCDSTORE_ISEMBEDDED` | `false` |
| `EmbeddedEtcdDataDir` | `--etcdstore-embeddedDataDir` | `SPDL_ETCDSTORE_EMBEDDEDDATADIR` | Empty; embedded mode only |
| `EtcdTLSTrustedCAFile` | `--etcdstore-tls-ca` | `SPDL_ETCDSTORE_TLS_CA` | Empty |
| `EtcdTLSCertFile` | `--etcdstore-tls-cert` | `SPDL_ETCDSTORE_TLS_CERT` | Empty |
| `EtcdTLSKeyFile` | `--etcdstore-tls-key` | `SPDL_ETCDSTORE_TLS_KEY` | Empty |
| `EtcdTLSServerName` | `--etcdstore-tls-serverName` | `SPDL_ETCDSTORE_TLS_SERVERNAME` | Empty |
| `EtcdTLSInsecureSkipVerify` | `--etcdstore-tls-insecureSkipVerify` | `SPDL_ETCDSTORE_TLS_INSECURESKIPVERIFY` | `false` |

Explicit flags override nonempty environment variables, then configuration files, then defaults. Select the backend with `--store-type etcd` or `SPDL_STORE_TYPE=etcd`. Additional TLS options are `EtcdTLSAllowedCN` (`--etcdstore-tls-allowedCN`) and `EtcdTLSCRLFile` (`--etcdstore-tls-crlFile`).

Embedded mode (`IsEmbeddedEtcd: true`) is for local development/tests and uses the fixed local etcd port. This guide keeps etcd independent from PMS/ADS so its data directory and lifecycle are managed explicitly.

| Symptom | Check |
| --- | --- |
| Connection refused or startup stalls | etcd endpoint health, client port, host/container address and firewall. |
| PMS writes but ADS has no service | Same endpoint and key prefix, including the trailing slash. |
| Changes do not reach ADS | Watch enabled, connectivity and watch errors in ADS logs; restart ADS to reload after a recovery. |
| TLS hostname or CA error | HTTPS endpoint, certificate SAN, trusted CA path and server-name setting. |
| Permission denied | Client certificate identity, enabled RBAC roles and prefix permissions. |

Return to [Pluggable Storage](/docs/store/) for backend interfaces and configuration precedence.
