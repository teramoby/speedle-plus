#!/usr/bin/env bash
set -euo pipefail
command -v docker >/dev/null
test "$(docker info --format '{{.OSType}}')" = linux
printf 'Deployment host: %s; runner=%s\n' "$(uname -s)/$(uname -m)" "${RUNNER_NAME:-local}"
docker version --format 'Docker client={{.Client.Version}} server={{.Server.Version}}'
docker info --format 'Engine arch={{.Architecture}} CPUs={{.NCPU}} memory={{.MemTotal}}'
printf 'Existing container count: '
docker ps -q | wc -l
