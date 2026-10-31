+++
title = "MySQL Setup"
description = "Configure MySQL 8.4 policy storage for PMS and ADS, step by step."
weight = 10
draft = false
toc = true
tocheading = "h2"
tags = ["storage", "mysql"]
categories = ["docs"]
bref = "Create a database, configure PMS and ADS, then verify authorization and automatic policy synchronization."
+++

This guide uses **MySQL 8.4 LTS** and two native Speedle processes: PMS writes
policies, and ADS reads the same database with a separate read-only account.
The local example uses Docker and plain HTTP on loopback. For a remote deployment,
use your existing MySQL server and the [Security guide](/docs/security/).

## 1. Build Speedle with the MySQL backend

Install Git, Bash, curl, Docker and a supported Go toolchain (Go 1.26.10 or
Go 1.27.3). Docker must be running. Build from current source so both binaries
contain the `mysql` store; an older installed release may not contain it.

<!-- mysql-step: build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

Run the remaining commands from this `speedle-plus` directory. Keep ports 3306,
6733 and 6734 free, or change the corresponding addresses in every command and
configuration below.

## 2. Start MySQL 8.4

The fixed passwords below are **local demo credentials**. Use your own credentials
and secret delivery mechanism when deploying. The named Docker volume preserves
the database when this container is stopped or recreated.

<!-- mysql-step: database -->
```bash
docker run --name speedle-mysql --detach \
  --publish 127.0.0.1:3306:3306 \
  --volume speedle-mysql-data:/var/lib/mysql \
  --env MYSQL_ROOT_PASSWORD=SpeedleDemoRoot84 \
  mysql:8.4
```

Wait for initialization to finish:

<!-- mysql-step: ready -->
```bash
until docker exec speedle-mysql mysqladmin ping \
  --host=127.0.0.1 --user=root --password=SpeedleDemoRoot84 --silent
do
  sleep 1
done
```

If you already have MySQL 8.4, skip these Docker commands and run the SQL in the
next step using your database administrator's client. The Docker image initializes
credentials only when its data directory is empty; changing an environment
variable does not reset a password in an existing volume.

## 3. Create the database and service accounts

Speedle creates tables by default, but **does not create the database**. Use an
administrator to create `speedle` and two accounts:

<!-- mysql-step: accounts -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 <<'SQL'
CREATE DATABASE speedle CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;
CREATE USER 'speedle_pms'@'%' IDENTIFIED BY 'SpeedleDemoPms84';
CREATE USER 'speedle_ads'@'%' IDENTIFIED BY 'SpeedleDemoAds84';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, REFERENCES ON speedle.* TO 'speedle_pms'@'%';
GRANT SELECT ON speedle.* TO 'speedle_ads'@'%';
SQL
```

PMS needs `CREATE` and `REFERENCES` for automatic schema initialization, plus
`SELECT`, `INSERT`, `UPDATE` and `DELETE` for policy management. ADS needs only
`SELECT` and must disable automatic schema initialization. The `%` account host
allows connections from the host through Docker; restrict account hosts to your
actual PMS/ADS machines for a remote deployment.

## 4. Write the PMS and ADS configuration files

Both DSNs must select the **same server and database**. A DSN follows the driver
syntax `user:password@tcp(host:port)/database`.

<!-- mysql-step: configuration -->
```bash
mkdir -p mysql-demo
cat > mysql-demo/pms.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_pms:SpeedleDemoPms84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": true,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  }
}
JSON
cat > mysql-demo/ads.json <<'JSON'
{
  "storeConfig": {
    "storeType": "mysql",
    "storeProps": {
      "MySQLDSN": "speedle_ads:SpeedleDemoAds84@tcp(127.0.0.1:3306)/speedle",
      "MySQLInitSchema": false,
      "MySQLPollInterval": "1s",
      "MySQLQueryTimeout": "10s"
    }
  },
  "enableWatch": true
}
JSON
chmod 600 mysql-demo/pms.json mysql-demo/ads.json
```

`MySQLInitSchema: false` makes startup check the existing schema without executing
DDL. `enableWatch: true` lets ADS follow committed changes. Do not commit these
credential-bearing configuration files.

## 5. Start PMS, then ADS

In terminal A, start PMS first so it can create the tables:

<!-- mysql-step: pms -->
```bash
./bin/speedle-pms --config-file mysql-demo/pms.json --endpoint 127.0.0.1:6733 --insecure=true
```

Leave PMS running. In terminal B, from the same checkout, confirm initialization:

<!-- mysql-step: tables -->
```bash
docker exec speedle-mysql mysql --user=speedle_ads --password=SpeedleDemoAds84 speedle -e 'SHOW TABLES;'
```

You should see five tables:

| Table | Contents |
| --- | --- |
| `speedle_services` | Services and service metadata |
| `speedle_policies` | Authorization policies |
| `speedle_role_policies` | Role policies |
| `speedle_functions` | Custom function definitions |
| `speedle_store_meta` | Schema version and committed policy revision |

Then start ADS in terminal B and leave it running:

<!-- mysql-step: ads -->
```bash
./bin/speedle-ads --config-file mysql-demo/ads.json --endpoint 127.0.0.1:6734 --insecure=true
```

Wait for each service's REST listener to start before continuing. The gRPC servers
remain off unless you also pass `--grpc-endpoint`.

## 6. Create a policy and check authorization

In terminal C, create a service with a policy granting Alice permission to read
`book`:

<!-- mysql-step: policy -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service \
  -H 'Content-Type: application/json' \
  -d '{"name":"mysql-demo","type":"application","policies":[{"name":"alice-read","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}'
```

Allow up to a polling interval (1 second by default), plus query/cache rebuild
time, for ADS to load the change. Repeat the request if synchronization is still
in progress:

<!-- mysql-step: allow -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

Expected response: `{"allowed":true,"reason":0}`. Bob has no matching grant:

<!-- mysql-step: deny -->
```bash
curl -fsS -X POST http://127.0.0.1:6734/authz-check/v1/is-allowed \
  -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"mysql-demo","resource":"book","action":"read"}'
```

Expected response: `{"allowed":false,"reason":3}`.

## 7. Verify automatic updates and persistence

Add an explicit deny for Alice through PMS, without restarting ADS:

<!-- mysql-step: update -->
```bash
curl -fsS -X POST http://127.0.0.1:6733/policy-mgmt/v1/service/mysql-demo/policy \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice-deny","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}'
```

After synchronization, repeat Alice's request from step 6. It should now return
`{"allowed":false,"reason":1}`: the explicit deny takes priority. You can stop
PMS and ADS with Ctrl+C, restart them with the commands in step 5, and repeat the
same request. The stored policy and denial persist in MySQL.

Policy changes must go through PMS or the store interface so that data and the
revision advance in the same transaction. Direct SQL edits do not automatically
trigger an ADS reload. During a database outage, ADS stays running and retries synchronization but
defaults to `allowed:false` and an error once the failure is detected. A complete
snapshot must load successfully before authorization resumes. See the shared
[store failure and grace-period policy](/docs/store/#store-failures-and-cached-authorization).
Starting ADS still requires a reachable database.

## Pre-create the schema without runtime DDL privileges

As an alternative to automatic table creation, run the repository's
[schema.sql](https://github.com/teramoby/speedle-plus/blob/master/pkg/store/mysql/schema.sql)
with a provisioning account before starting either service:

<!-- mysql-step: schema -->
```bash
docker exec -i speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 speedle < pkg/store/mysql/schema.sql
```

Set `MySQLInitSchema` to `false` in **both** configuration files. If you followed
the demo above, remove the extra initialization privileges:

<!-- mysql-step: revoke-ddl -->
```bash
docker exec speedle-mysql mysql --user=root --password=SpeedleDemoRoot84 -e "REVOKE CREATE, REFERENCES ON speedle.* FROM 'speedle_pms'@'%';"
```

PMS then runs with `SELECT`, `INSERT`, `UPDATE`, `DELETE`; ADS stays read-only.
Restart both services after changing their configuration. Schema initialization
does not migrate existing tables; unsupported schema versions fail startup.

## Configuration reference and troubleshooting

| Property | Flag | Environment variable | Default |
| --- | --- | --- | --- |
| `MySQLDSN` | `--mysqlstore-dsn` | `SPDL_MYSQLSTORE_DSN` | Required, including database name |
| `MySQLInitSchema` | `--mysqlstore-init-schema` | `SPDL_MYSQLSTORE_INIT_SCHEMA` | `true` |
| `MySQLPollInterval` | `--mysqlstore-poll-interval` | `SPDL_MYSQLSTORE_POLL_INTERVAL` | `1s` |
| `MySQLQueryTimeout` | `--mysqlstore-query-timeout` | `SPDL_MYSQLSTORE_QUERY_TIMEOUT` | `10s` |

The selection flag is `--store-type mysql` (`SPDL_STORE_TYPE=mysql`). Explicit
flags override nonempty environment variables, then configuration files, then
defaults. For remote MySQL, use a hostname with a trusted server certificate and
add `?tls=true` to the DSN; custom CA configuration follows the
[driver TLS documentation](https://github.com/go-sql-driver/mysql/tree/v1.10.1#tls).

| Symptom | Check |
| --- | --- |
| Unknown `mysql` store or flag | Rebuild both binaries from source containing `pkg/store/mysql`. |
| Connection refused / access denied | Database readiness, DSN host/port, password and MySQL account host restrictions. |
| Unknown database | Create the database first; Speedle creates only tables. |
| Missing tables or DDL denied | Start PMS first, or pre-create the schema; read-only ADS must set `MySQLInitSchema` to `false`. |
| Policy change is not visible | Same database in both DSNs, ADS watch enabled, polling delay, and no overriding `SPDL_*` variables. |

MySQL does not support Discover request collection or policy generation. See
[Pluggable Storage](/docs/store/) for transaction, snapshot and recovery details.
