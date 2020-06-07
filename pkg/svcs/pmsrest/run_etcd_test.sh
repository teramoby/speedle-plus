#!/bin/bash

shell_dir=$(dirname $0)

set -ex
#source ${shell_dir}/start_etcd.sh
source ${GOPATH}/src/github.com/teramoby/speedle-plus/setTestEnv.sh

go clean -testcache

startPMS etcd --config-file ${shell_dir}/config_etcd.json

go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/svcs/pmsrest $*
