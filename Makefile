.PHONY: all build test lint coverage release clean image imagePms imageAds push

gopath := $(shell go env GOPATH)
gitCommit := $(shell git rev-parse --short HEAD)
buildDate := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Extract version from git tag, fallback to VERSION file, then to "0.1"
version := $(shell git describe --tags --always --dirty 2>/dev/null || (cat VERSION 2>/dev/null || echo "0.1"))

# go version output is "go version go1.11.2 linux/amd64"
goVersion := $(word 3,$(shell go version))
goLDFlags := -trimpath -ldflags "-s -w -X main.gitCommit=${gitCommit} -X main.productVersion=${version} -X main.goVersion=${goVersion} -X main.buildDate=${buildDate}"

pmsImageRepo ?= speedle-pms
pmsImageTag ?= dev
adsImageRepo ?= speedle-ads
adsImageTag ?= dev
containerGoVersion ?= 1.26.10

all: build

build: buildPms buildAds buildSpctl

buildPms:
	go build ${goLDFlags} -o ${gopath}/bin/speedle-pms github.com/teramoby/speedle-plus/cmd/speedle-pms

buildAds:
	go build ${goLDFlags} -o ${gopath}/bin/speedle-ads github.com/teramoby/speedle-plus/cmd/speedle-ads

buildSpctl:
	go build ${goLDFlags} -o ${gopath}/bin/spctl  github.com/teramoby/speedle-plus/cmd/spctl

image: imagePms imageAds

imagePms:
	docker build -f deployment/docker/speedle-pms/Dockerfile --build-arg GO_VERSION=${containerGoVersion} --build-arg GIT_COMMIT=${gitCommit} --build-arg VERSION=${pmsImageTag} -t ${pmsImageRepo}:${pmsImageTag} .

imageAds:
	docker build -f deployment/docker/speedle-ads/Dockerfile --build-arg GO_VERSION=${containerGoVersion} --build-arg GIT_COMMIT=${gitCommit} --build-arg VERSION=${adsImageTag} -t ${adsImageRepo}:${adsImageTag} .

push:
	docker push ${pmsImageRepo}:${pmsImageTag}
	docker push ${adsImageRepo}:${adsImageTag}

test: testAll

testAll: speedleUnitTests testSpeedleRest testSpeedleGRpc testSpctl testSpeedleRestADSCheck testSpeedleGRpcADSCheck testSpeedleTls

speedleUnitTests:
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/cfg
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/eval
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/store/file
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/store/etcd
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/store/mongodb
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/pdl
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/suid
	go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/assertion
	go clean -testcache
	STORE_TYPE=etcd go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/eval
	go clean -testcache
	STORE_TYPE=mongodb go test ${TEST_OPTS} github.com/teramoby/speedle-plus/pkg/eval

testSpeedleRest:
	pkg/svcs/pmsrest/run_file_test.sh
	pkg/svcs/pmsrest/run_etcd_test.sh
	pkg/svcs/pmsrest/run_mongodb_test.sh

testSpeedleGRpc:
	pkg/svcs/pmsgrpc/run_file_test.sh
	pkg/svcs/pmsgrpc/run_etcd_test.sh
	pkg/svcs/pmsgrpc/run_mongodb_test.sh

testSpeedleRestADSCheck:
	pkg/svcs/adsrest/run_file_test.sh
	pkg/svcs/adsrest/run_etcd_test.sh
	pkg/svcs/adsrest/run_mongodb_test.sh

testSpeedleGRpcADSCheck:
	pkg/svcs/adsgrpc/run_file_test.sh
	pkg/svcs/adsgrpc/run_etcd_test.sh
	pkg/svcs/adsgrpc/run_mongodb_test.sh

testSpctl:
	cmd/spctl/command/run_file_test.sh
	cmd/spctl/command/run_etcd_test.sh
	cmd/spctl/command/run_mongodb_test.sh

testSpeedleTls:
	pkg/svcs/pmsrest/tls_test.sh
	pkg/svcs/pmsrest/tls_test-force-client-cert.sh

# New targets

lint:
	golangci-lint run ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Multi-platform release build
release:
	GOOS=linux GOARCH=amd64 go build ${goLDFlags} -o ${gopath}/bin/speedle-pms-linux-amd64 github.com/teramoby/speedle-plus/cmd/speedle-pms
	GOOS=linux GOARCH=amd64 go build ${goLDFlags} -o ${gopath}/bin/speedle-ads-linux-amd64 github.com/teramoby/speedle-plus/cmd/speedle-ads
	GOOS=linux GOARCH=amd64 go build ${goLDFlags} -o ${gopath}/bin/spctl-linux-amd64 github.com/teramoby/speedle-plus/cmd/spctl
	GOOS=darwin GOARCH=amd64 go build ${goLDFlags} -o ${gopath}/bin/speedle-pms-darwin-amd64 github.com/teramoby/speedle-plus/cmd/speedle-pms
	GOOS=darwin GOARCH=amd64 go build ${goLDFlags} -o ${gopath}/bin/speedle-ads-darwin-amd64 github.com/teramoby/speedle-plus/cmd/speedle-ads
	GOOS=darwin GOARCH=amd64 go build ${goLDFlags} -o ${gopath}/bin/spctl-darwin-amd64 github.com/teramoby/speedle-plus/cmd/spctl
	GOOS=darwin GOARCH=arm64 go build ${goLDFlags} -o ${gopath}/bin/speedle-pms-darwin-arm64 github.com/teramoby/speedle-plus/cmd/speedle-pms
	GOOS=darwin GOARCH=arm64 go build ${goLDFlags} -o ${gopath}/bin/speedle-ads-darwin-arm64 github.com/teramoby/speedle-plus/cmd/speedle-ads
	GOOS=darwin GOARCH=arm64 go build ${goLDFlags} -o ${gopath}/bin/spctl-darwin-arm64 github.com/teramoby/speedle-plus/cmd/spctl

clean:
	rm -rf ${gopath}/pkg/linux_amd64/github.com/teramoby/speedle-plus
	rm -f ${gopath}/bin/speedle-pms
	rm -f ${gopath}/bin/speedle-ads
	rm -f ${gopath}/bin/spctl
