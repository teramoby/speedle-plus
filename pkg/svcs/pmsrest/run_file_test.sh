#!/bin/bash

shell_dir=$(dirname $0)

set -ex
source ${GOPATH}/src/github.com/teramoby/speedle-plus/setTestEnv.sh

startPMS file --config-file ${shell_dir}/config_file.json

go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/svcs/pmsrest $*
