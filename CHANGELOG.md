# Changelog

All notable changes to the speedle-plus project will be documented in this file.

## [Unreleased] - wcai_issues branch

### Fixed
- Updated go directive in go.mod from 1.25.0 to 1.24 (conventional format)
- Added toolchain directive for Go 1.26.3 build compatibility
- Downgraded etcd dependencies from v3.6.7 to v3.5.18 for compatibility
- Downgraded k8s dependencies from v0.35.0 to v0.31.0
- Removed go.mod.backup and go.sum.backup files
- Replaced hardcoded MongoDB credentials with placeholder values
- Updated GitHub Actions CI workflow:
  - Updated actions/checkout from v2 to v4
  - Updated actions/setup-go from v2 to v5
  - Added matrix strategy for store types (file, etcd)
  - Added go test -coverprofile step
  - Added golangci-lint step
  - Added go mod tidy -diff check
  - Added govulncheck step
  - Removed DNS hack workaround
- Fixed Kubernetes deployment manifests:
  - Changed // comment to # in YAML files
  - Updated apiVersion from apps/v1beta2 to apps/v1
  - Added securityContext (runAsNonRoot, readOnlyRootFilesystem, allowPrivilegeEscalation: false)
  - Added resource limits and requests
  - Added livenessProbe and readinessProbe
  - Updated etcd image to v3.5.18
  - Added ServiceAccount
- Fixed Helm charts with same security and probe improvements
- Updated Dockerfiles with multi-stage builds, non-root USER, HEALTHCHECK, and OCI labels
- Added .dockerignore file
- Updated Makefile with ldflags improvements, lint/coverage/release targets, git-based versioning
- Removed abandoned wercker.yml CI configuration
