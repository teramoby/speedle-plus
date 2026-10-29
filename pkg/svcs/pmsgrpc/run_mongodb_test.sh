#!/bin/bash

shell_dir=$(dirname $0)

set -ex
source ${GOPATH}/src/github.com/teramoby/speedle-plus/setTestEnv.sh

go clean -testcache

#Reconfig spctl
${GOPATH}/bin/spctl config ads-endpoint http://localhost:6734/authz-check/v1/
${GOPATH}/bin/spctl config pms-endpoint http://localhost:6733/policy-mgmt/v1/


startPMS mongodb --config-file ${shell_dir}/../pmsrest/config_mongodb.json

sleep 5
${GOPATH}/bin/spctl delete service --all

go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc -run=TestMats
