+++
title = "Pluggable Storage"
description = "Bring your own persistence store!"
weight = 130
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = "Speedle includes file, etcd, MongoDB and MySQL policy stores. This document describes their configuration and how to implement or register a different store."
+++

## Built-in stores and extension contracts

The standalone binaries register file, etcd, MongoDB and MySQL implementations under `pkg/store`. Availability does not mean identical operational guarantees; review backend authentication, persistence, transaction and watch behavior for your deployment. The configuration below illustrates property names; running etcd/MongoDB deployments is outside this local example.

A full backend implements [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go): service, authorization-policy, role-policy and function management, snapshot reads and watch/stop. ADS accepts the narrower `PolicyStoreManagerADS` read/watch interface. Check the actual interfaces when implementing a backend, rather than copying a partial historical method list.

## MySQL 8.4 policy store

The `mysql` backend implements the complete policy-store interface for PMS and ADS. Discover request collection and policy generation are not supported. It uses the pure-Go `go-sql-driver/mysql` driver; the database must already exist.

```json
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle:replace-me@tcp(db.example.com:3306)/speedle?tls=true",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
```

| Property | Flag | Default |
| --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | Required, including database name |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `10s` |

Environment variables use the same mapping, for example `SPDL_MYSQLSTORE_DSN`. Durations must be positive strings; `MySQLInitSchema` accepts a JSON boolean or a boolean string. Explicit flags override nonempty environment variables, then configuration-file values, then defaults. For embedded use, import `pkg/store/mysql`, pass these properties directly to `store.NewStore`, and close the returned `*mysql.Store` when finished. An evaluator created through `eval.NewFromConfig` can be closed through `io.Closer`; closing an evaluator also closes a closable supplied store.

### Schema and provisioning

One database is one policy store, using five fixed InnoDB tables: `speedle_services`, `speedle_policies`, `speedle_role_policies`, `speedle_functions`, and `speedle_store_meta`. Complex arrays and metadata use JSON columns. Policy tables reference the service with cascading deletion. Service/function names and policy IDs use case-sensitive, non-padding comparisons and support up to 255 Unicode characters; policy names need not be unique.

By default, startup creates missing tables using [schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql). It does not create a database or alter an existing schema. For a prebuilt deployment, run that same SQL against the selected database using a provisioning account, then set `MySQLInitSchema` to `false`. Startup checks the schema version, required columns and InnoDB engines. An unsupported schema version fails startup; future upgrades require an explicit migration.

Provisioning requires table-creation and initialization privileges. With a prebuilt schema, the PMS account needs `SELECT`, `INSERT`, `UPDATE`, and `DELETE` on the policy database; ADS can use a separate account with only `SELECT` and automatic schema creation disabled. Use driver DSN TLS options for remote databases. Do not put production passwords in committed configuration or command-line history.

### Transactions, synchronization and recovery

Each write locks the single revision row and commits data changes together with a revision increment. This serializes policy writes across PMS instances. Service creation with nested policies, cascading deletion, and whole-store replacement are atomic. Import preserves supplied policy IDs and assigns IDs where absent. Snapshot reads use one repeatable-read transaction.

With `enableWatch`, ADS polls the revision and receives a complete consistent snapshot when it changes. Startup also sends a snapshot to cover updates between the initial read and watch registration. Propagation is asynchronous, normally within a polling interval plus query/cache-rebuild time. During a database outage ADS retains its last loaded policies, retries reads, and catches up after recovery. Changes made directly in SQL without incrementing the revision are not discovered automatically; use PMS or the store interface for policy writes.

The operation timeout also bounds connection-pool waits and transaction lock waits. Failed writes are rolled back; a connection loss during commit can leave the outcome uncertain, so callers should inspect stored state before retrying a non-idempotent create. `StopWatch` and `Close` are idempotent and release watch resources; `Close` also closes the connection pool.

## Implement and register a builder

A [store.StoreBuilder](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/store.go) provides `NewStore(map[string]interface{}) (pms.PolicyStoreManager, error)` and `GetStoreParams() map[string]string`. The latter maps flag names to configuration property names. Register a unique store type in `init()` or before calling `store.NewStore`; duplicate registration is an error.

This complete example delegates to the existing file backend to demonstrate the factory contract. It does not claim to implement a new database:

<!-- docs-check: store -->
```bash
go run ./website/tests/examples/store
```


<!-- docs-source: website/tests/examples/store/main.go -->
```go
package main

import (
	"fmt"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
	"log"
	"os"
	"path/filepath"
)

// This example delegates to the file backend to demonstrate registration.
// A new backend must implement the complete PolicyStoreManager contract.
type Builder struct{ file.FileStoreBuilder }

func main() {
	dir, err := os.MkdirTemp("", "speedle-store-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store.Register("myfile", Builder{})
	policies, err := store.NewStore("myfile", map[string]interface{}{"FileLocation": filepath.Join(dir, "policies.json")})
	if err != nil {
		log.Fatal(err)
	}
	snapshot, err := policies.ReadPolicyStore()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Registered myfile store; services=%d\n", len(snapshot.Services))
}
```


## Merge configuration correctly

For standalone commands, explicit flags override nonempty environment variables, then configuration-file properties, then defaults. Convert the flag name by adding `SPDL_`, replacing `-` with `_`, and uppercasing: `etcdstore-endpoint` becomes `SPDL_ETCDSTORE_ENDPOINT`. Values may arrive as strings from flags/env or typed JSON values; validate and convert deliberately.

```json
{
  "storeConfig": {
    "storeType": "etcd",
    "storeProps": {
      "EtcdEndpoint": "localhost:2379",
      "EtcdKeyPrefix": "/speedle_ps/",
      "IsEmbeddedEtcd": true,
      "EmbeddedEtcdDataDir": "./speedle.etcd"
    }
  },
  "enableWatch": true
}
```

Define custom flags with `pflag` before parsing and return their flag-to-property mapping from `GetStoreParams`. Embedded callers pass the configuration map directly to `store.NewStore`/`eval.NewFromConfig`; the factory alone does not read command-line or environment configuration.

## Link the backend and handle updates

Use side-effect imports in both standalone binaries (or your embedded application) when registration happens in `init()`:

```go
import (
    _ "github.com/teramoby/speedle-plus/pkg/store/etcd"
    _ "github.com/teramoby/speedle-plus/pkg/store/file"
    _ "github.com/teramoby/speedle-plus/pkg/store/mongodb"
    _ "github.com/teramoby/speedle-plus/pkg/store/mysql"
)
```

Implement `Watch()` so it returns the correct snapshot/change events through `pms.StorageChangeChannel`; support `StopWatch()` without leaking watch resources. ADS updates its runtime cache asynchronously. Test missed/disconnected watches, recovery, failure during a write, and concurrent readers; do not promise instantaneous propagation simply because the backend supports watch.

## MongoDB deployment requirements

MongoDB change streams (`Watch`) and atomic snapshot replacement
(`WritePolicyStore`) require a replica set or sharded cluster. A single-node
replica set is sufficient for local testing. Standalone MongoDB can serve ordinary
CRUD operations, but rejects snapshot replacement without deleting existing data.
Snapshot replacement preserves supplied policy IDs and generates missing IDs.
Bulk service/function deletion keeps the collections and emits document deletion
events so ADS can remove cached entries. Call `StopWatch()` to cancel the stream
and close its event channel before releasing a store.
