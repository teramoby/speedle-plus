#!/usr/bin/env bash
# Validation-only harness: own a temporary mongod, never touch resident services.
set -euo pipefail
cd "$(dirname "$0")/../.."
test "${RUNNER_NAME}" = fiona-speedle
printf 'MongoDB validation runner: %s\n' "${RUNNER_NAME}"
validation_dir=$(mktemp -d "${RUNNER_TEMP:-/tmp}/speedle-mongo.XXXXXXXX")
mongo_pid=''
cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "${mongo_pid}" ]]; then
    kill "${mongo_pid}" 2>/dev/null || true
    wait "${mongo_pid}" || true
  fi
  if (( status != 0 )); then tail -100 "${validation_dir}/mongod.log" 2>/dev/null || true; fi
  rm -rf "${validation_dir}"
  exit "${status}"
}
trap cleanup EXIT
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64)
    archive=mongodb-macos-arm64-7.0.16.tgz
    download_base=https://fastdl.mongodb.org/osx
    ;;
  Linux/x86_64)
    archive=mongodb-linux-x86_64-ubuntu2204-7.0.16.tgz
    download_base=https://fastdl.mongodb.org/linux
    ;;
  *) echo 'Unsupported MongoDB validation platform.' >&2; exit 1 ;;
esac
curl --fail --silent --show-error --location --retry 3 "${download_base}/${archive}" -o "${validation_dir}/${archive}"
curl --fail --silent --show-error --location --retry 3 "${download_base}/${archive}.sha256" -o "${validation_dir}/checksum"
expected=$(awk 'NR == 1 {print $1}' "${validation_dir}/checksum")
[[ "${expected}" =~ ^[a-fA-F0-9]{64}$ ]]
printf '%s  %s\n' "${expected}" "${validation_dir}/${archive}" | shasum -a 256 --check -
tar -xzf "${validation_dir}/${archive}" -C "${validation_dir}" --strip-components=1
mongo_port=$(python3 - <<'PY'
import socket
with socket.socket() as s:
    s.bind(('127.0.0.1', 0))
    print(s.getsockname()[1])
PY
)
mkdir "${validation_dir}/data"
"${validation_dir}/bin/mongod" --dbpath "${validation_dir}/data" --bind_ip 127.0.0.1 --port "${mongo_port}" --replSet speedle-validation --nounixsocket --wiredTigerCacheSizeGB 0.25 --logpath "${validation_dir}/mongod.log" &
mongo_pid=$!
export SPEEDLE_MONGO_URI="mongodb://127.0.0.1:${mongo_port}/?replicaSet=speedle-validation"
export SPEEDLE_REQUIRE_MONGO=1
cat > "${validation_dir}/init.go" <<'GO'
package main

import (
    "context"
    "log"
    "os"
    "net/url"
    "time"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "go.mongodb.org/mongo-driver/mongo/readpref"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()
    bootstrapURL, err := url.Parse(os.Getenv("SPEEDLE_MONGO_URI"))
    if err != nil { log.Fatal(err) }
    // Do not require replicaSet membership before replSetInitiate has run.
    // This direct bootstrap connection must also allow a non-primary node.
    bootstrapURI := "mongodb://" + bootstrapURL.Host + "/"
    client, err := mongo.Connect(ctx, options.Client().ApplyURI(bootstrapURI).SetDirect(true).SetReadPreference(readpref.Nearest()).SetServerSelectionTimeout(time.Second))
    if err != nil { log.Fatal(err) }
    defer client.Disconnect(context.Background())
    for client.Ping(ctx, readpref.Nearest()) != nil {
        if ctx.Err() != nil { log.Fatal(ctx.Err()) }
        time.Sleep(200*time.Millisecond)
    }
    // The process was just started, so initialize this instance only.
    address := bootstrapURL.Host
    command := bson.D{{Key:"replSetInitiate", Value:bson.D{
        {Key:"_id", Value:"speedle-validation"},
        {Key:"members", Value:bson.A{bson.D{{Key:"_id", Value:0}, {Key:"host", Value:address}}}},
    }}}
    if err := client.Database("admin").RunCommand(ctx, command).Err(); err != nil { log.Fatal(err) }
    for {
        var state struct { Primary bool `bson:"isWritablePrimary"` }
        err := client.Database("admin").RunCommand(ctx, bson.D{{Key:"hello", Value:1}}).Decode(&state)
        if err == nil && state.Primary { return }
        if ctx.Err() != nil { log.Fatal(ctx.Err()) }
        time.Sleep(200*time.Millisecond)
    }
}
GO
go run "${validation_dir}/init.go"
unformatted=$(gofmt -l pkg/store/mongodb/mongodbStore.go pkg/store/mongodb/regression_test.go)
if [[ -n "${unformatted}" ]]; then
  printf 'Go files need formatting:\n%s\n' "${unformatted}" >&2
  exit 1
fi
go test -race -v -count=1 -timeout 300s ./pkg/store/mongodb
