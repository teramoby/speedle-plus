+++
title = "MongoDB Setup"
description = "Set up MongoDB policy storage for PMS and ADS, step by step."
weight = 30
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mongodb"]
categories = ["docs"]
bref = "Run a MongoDB replica set, connect PMS and ADS, verify live policy updates, and save a backup."
+++

This walkthrough uses **MongoDB 7.0.16** as a single-node replica set with a persistent Docker volume. PMS writes policies and ADS follows the same database through a change stream. The example listens on loopback without authentication for local development. Production authentication, TLS and replica set configuration follow the walkthrough.

MongoDB must run as a **replica set or sharded cluster**. ADS watches the database with a change stream, and replacing the whole policy store runs in a transaction; a standalone `mongod` supports neither. A single-node replica set is enough for development.

## 1. Build Speedle

Prepare Bash, Git, curl, Docker and Go 1.26.10 or Go 1.27.3. Start Docker, then build the native services:

<!-- mongodb-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Run subsequent commands from this checkout. Keep local ports 27017, 6733 and 6734 available, or update the commands and configuration together.

## 2. Start MongoDB

The named volume holds MongoDB's data. `--replSet` starts `mongod` as a member of the replica set `speedle-rs`; only the database port is published, on loopback.

<!-- mongodb-step: database -->
```bash
docker run --name speedle-mongodb --detach \
  --publish 127.0.0.1:27017:27017 \
  --volume speedle-mongodb-data:/data/db \
  mongo:7.0.16 \
  --replSet speedle-rs --bind_ip_all
```

Wait for the server to accept commands:

<!-- mongodb-step: ready -->
```bash
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.adminCommand({ping: 1}).ok' >/dev/null 2>&1
do
  sleep 1
done
```

If you already run a MongoDB replica set, skip the container commands and step 3, and use its connection string below. See the [official Docker image](https://hub.docker.com/_/mongo) for container details.

## 3. Initialize the replica set

Initialize the replica set once, with this server as its only member, and wait until it becomes primary:

<!-- mongodb-step: replica-set -->
```bash
docker exec speedle-mongodb mongosh --quiet --eval \
  'rs.initiate({_id: "speedle-rs", members: [{_id: 0, host: "127.0.0.1:27017"}]})'
until docker exec speedle-mongodb mongosh --quiet \
  --eval 'db.hello().isWritablePrimary' | grep -qx true
do
  sleep 1
done
```

The member address `127.0.0.1:27017` is what clients are told to connect to, so it must be reachable from PMS and ADS. It works here because the port is published on the same loopback address. The configuration is stored in the volume; do not run `rs.initiate` again after a restart.

## 4. Configure the shared store

Create one configuration file for this local PMS/ADS pair:

<!-- mongodb-step: configuration -->
```bash
mkdir -p mongodb-demo
cat > mongodb-demo/config.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://127.0.0.1:27017/?replicaSet=speedle-rs",
      "MongoDatabase": "speedle_demo"
    }
  },
  "enableWatch": true
}
JSON
```

PMS and ADS must use the **same deployment and database**. Use a separate database for each independent policy store; the store keeps services and functions in its `services` and `functions` collections. MongoDB creates the database and collections on the first write, so no schema initialization is needed. `enableWatch: true` enables ADS updates through the change stream; no polling interval is configured.

## 5. Start PMS

In terminal A, start PMS and leave it running:

<!-- mongodb-step: pms -->
```bash
./bin/speedle-pms --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6733 --insecure=true
```

Wait for its REST listener to start. Plain HTTP is bound to loopback for this demo; see [Security](/docs/security/) for Speedle's own TLS and API authorization.

## 6. Create a policy

In terminal B, from the same checkout, grant Alice permission to read `book`:

<!-- mongodb-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mongodb-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

PMS returns the service and generated policy ID. The policy is now in MongoDB.

## 7. Start ADS and verify authorization

In terminal C, start ADS with the same file and leave it running:

<!-- mongodb-step: ads -->
```bash
./bin/speedle-ads --config-file mongodb-demo/config.json --endpoint 127.0.0.1:6734 --insecure=true
```

Wait for startup, then run these checks in terminal B. Alice is allowed:

<!-- mongodb-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

Expected: `{"allowed":true,"reason":0}`. Bob has no matching grant:

<!-- mongodb-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mongodb-demo","resource":"book","action":"read"}'
```

Expected: `{"allowed":false,"reason":3}`.

## 8. Verify stored documents and automatic updates

Inspect the service document; the service name is its `_id`:

<!-- mongodb-step: documents -->
```bash
docker exec speedle-mongodb mongosh speedle_demo --quiet --eval \
  'db.services.find({}, {"policies.name": 1, "policies.effect": 1}).toArray()'
```

The output includes `_id: 'mongodb-demo'` with the `alice-read` grant. Treat the documents as backend-managed data and make policy changes through PMS.

Add a deny while ADS remains running:

<!-- mongodb-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mongodb-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

Repeat Alice's request from step 7. After the asynchronous change-stream update, it should return `{"allowed":false,"reason":1}`. Repeat the request if synchronization is still in progress. ADS does not need a restart for this normal policy update.

## 9. Save a backup and verify persistence

Dump the policy database to an archive and copy it outside the container:

<!-- mongodb-step: backup -->
```bash
docker exec speedle-mongodb mongodump --quiet \
  --db speedle_demo --archive=/tmp/speedle-demo.archive
docker cp speedle-mongodb:/tmp/speedle-demo.archive ./mongodb-demo/speedle-demo.archive
```

The archive contains only the `speedle_demo` database. Store backups on separate protected storage. To restore it, stop policy writers and run `mongorestore --drop --archive=...` against the primary; see [Back up and restore with MongoDB tools](https://www.mongodb.com/docs/manual/tutorial/backup-and-restore-tools/).

Stop PMS and ADS with Ctrl+C before this restart check:

<!-- mongodb-step: restart -->
```bash
docker restart speedle-mongodb
```

Run both wait loops in steps 2 and 3, skipping `rs.initiate`, restart PMS and ADS with the same configuration, then repeat Alice's request. It should still return `{"allowed":false,"reason":1}`. The named volume retains the data and the replica set configuration across the container restart.

After a restore, restart ADS so it reads the restored store before serving decisions. Recheck authorization after recovery; a previously running cache is not proof that it contains the restored policy set.

## Production: authentication, TLS and replica sets

Enable access control, give PMS and ADS separate users, and connect over TLS. PMS needs `readWrite` on the policy database. ADS only reads and watches, so the built-in `read` role is enough, and its user cannot change policies:

```javascript
use speedle
db.createUser({user: "speedle-pms", pwd: passwordPrompt(), roles: [{role: "readWrite", db: "speedle"}]})
db.createUser({user: "speedle-ads", pwd: passwordPrompt(), roles: [{role: "read", db: "speedle"}]})
```

Then give each service its own connection string:

```json
{
  "storeConfig": {
    "storeType": "mongodb",
    "storeProps": {
      "MongoURI": "mongodb://speedle-ads:replace-me@mongo1.example.com:27017,mongo2.example.com:27017,mongo3.example.com:27017/?replicaSet=speedle-rs&authSource=speedle&tls=true&tlsCAFile=/etc/speedle/mongodb/ca.pem",
      "MongoDatabase": "speedle"
    }
  },
  "enableWatch": true
}
```

The CA path is on the PMS/ADS machine. Credentials in a URI must be percent-encoded, and a configuration file containing them must be readable only by the service account; you can supply the URI through `SPDL_MONGOSTORE_URI` instead. Every option of the [MongoDB connection string](https://www.mongodb.com/docs/manual/reference/connection-string/) is passed to the Go driver unchanged, including client certificates (`tlsCertificateKeyFile`) and other authentication mechanisms. TLS to MongoDB is configured separately from TLS on Speedle's REST/gRPC listeners.

Access control on a replica set also requires authentication between members, with a key file or X.509 certificates. See the official guides to [deploy a self-managed replica set with keyfile authentication](https://www.mongodb.com/docs/manual/tutorial/deploy-replica-set-with-keyfile-access-control/) and [configure TLS](https://www.mongodb.com/docs/manual/tutorial/configure-ssl/).

For high availability, use at least three data-bearing members and list several of them in the connection string. The single-node demo cannot tolerate loss of its only member. While a new primary is elected, writes through PMS can fail; retry them after the election.

## Configuration reference and troubleshooting

| Property | Flag | Environment variable | Default |
| --- | --- | --- | --- |
| `MongoURI` | `--mongostore_uri` | `SPDL_MONGOSTORE_URI` | `mongodb://localhost:27017` |
| `MongoDatabase` | `--mongostore_database` | `SPDL_MONGOSTORE_DATABASE` | `speedleplus` |

Explicit flags override nonempty environment variables, then configuration files, then defaults. Select the backend with `--store-type mongodb` or `SPDL_STORE_TYPE=mongodb`. For embedded use, import `pkg/store/mongodb` and pass these properties to `store.NewStore`.

Behaviour to know when operating the store:

- Replacing the whole store (`WritePolicyStore`) runs in one transaction. It keeps supplied policy IDs and generates missing ones. On a standalone server it fails without changing existing data.
- Deleting all services or functions keeps the collections, so the change stream stays open and ADS removes the cached entries.
- Reading the whole store reads services and functions separately, not in one snapshot.
- If the change stream ends with an error, ADS keeps its last loaded policies and does not reopen the stream. Restart ADS after the database recovers.
- Discover request collection and policy generation are not supported; they are available with the file and etcd stores.

| Symptom | Check |
| --- | --- |
| Server selection timeout at startup | Replica set initialized and primary, the member host in `rs.conf()` reachable from PMS/ADS, URI host and port, firewall. |
| ADS exits with `The $changeStream stage is only supported on replica sets` | The deployment is a standalone `mongod`; use a replica set or sharded cluster. |
| PMS writes but ADS has no service | Same URI deployment and `MongoDatabase` name in both configurations. |
| Changes do not reach ADS | Watch enabled, change stream errors in ADS logs; restart ADS after a recovery. |
| Authentication failed | User, password encoding, `authSource` and the role on the policy database. |
| TLS error | `tls=true`, CA file path and certificate host names. |

Return to [Pluggable Storage](/docs/store/) for backend interfaces and configuration precedence.
