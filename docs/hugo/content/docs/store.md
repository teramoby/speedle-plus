+++
title = "Pluggable Storage"
description = "Bring your own persistence store!"
weight = 130
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "pluggable"]
categories = ["docs"]
bref = "By default, Speedle supports etcd and file as persistence stores. However, It is easy to plug in a different persistence store, say, mongodb. This document describes how one could go about implementing/using a different store."
+++

## Built-in stores and extension contracts

The standalone binaries register file, etcd and MongoDB implementations under `pkg/store`. Availability does not mean identical operational guarantees; review backend authentication, persistence, transaction and watch behavior for your deployment. The configuration below illustrates property names; running etcd/MongoDB deployments is outside this local example.

A full backend implements [pms.PolicyStoreManager](https://github.com/teramoby/speedle-plus/blob/master/api/pms/interface.go): service, authorization-policy, role-policy and function management, snapshot reads, health checks, watch/stop, and close. ADS accepts the narrower `PolicyStoreManagerADS` read/watch interface. Check the actual interfaces when implementing a backend, rather than copying a partial historical method list.

## Implement and register a builder

A [store.StoreBuilder](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/store.go) provides `NewStore(map[string]interface{}) (pms.PolicyStoreManager, error)` and `GetStoreParams() map[string]string`. The latter maps flag names to configuration property names. Register a unique store type in `init()` or before calling `store.NewStore`; duplicate registration is an error.

This complete example delegates to the existing file backend to demonstrate the factory contract. It does not claim to implement a new database:

<!-- docs-check: store -->
```bash
go run ./samples/docs/store
```


<!-- docs-source: samples/docs/store/main.go -->
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
	defer policies.Close()
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
)
```

Implement `Watch()` so it returns the correct snapshot/change events through `pms.StorageChangeChannel`; support `StopWatch()` and `Close()` without leaking resources. ADS updates its runtime cache asynchronously. Test missed/disconnected watches, recovery, failure during a write, and concurrent readers; do not promise instantaneous propagation simply because the backend supports watch.
