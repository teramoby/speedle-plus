#!/bin/bash
set -ex

#Reconfig spctl
${GOPATH}/bin/spctl config pms-endpoint http://localhost:6733/policy-mgmt/v1/
source ${GOPATH}/src/github.com/teramoby/speedle-plus/setTestEnv.sh

startPMS file --config-file pkg/svcs/pmsrest/config_file.json
go test ${TEST_OPTS} github.com/teramoby/speedle-plus/cmd/spctl/command -run=TestMats

