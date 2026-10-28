#!/usr/bin/env bash
set -euo pipefail
printf 'Deployment host: %s; runner=%s\n' "$(uname -s)/$(uname -m)" "${RUNNER_NAME:-local}"
for tool in docker podman sudo rootlesskit newuidmap slirp4netns; do command -v "$tool" || true; done
if command -v sudo >/dev/null; then
  if sudo -n true 2>/dev/null; then echo 'Noninteractive administrative setup is available.'; fi
fi
if [[ "$(uname -s)" == Linux ]]; then
  cat /etc/os-release | head -n 3
  test ! -f /sys/fs/cgroup/cgroup.controllers || echo 'cgroup v2 is available.'
  df -h /tmp | tail -n 1
  free -m | head -n 2
fi
command -v docker >/dev/null || { echo 'Docker is not installed on this runner.' >&2; exit 1; }
test "$(docker info --format '{{.OSType}}')" = linux
docker version --format 'Docker client={{.Client.Version}} server={{.Server.Version}}'
docker info --format 'Engine arch={{.Architecture}} CPUs={{.NCPU}} memory={{.MemTotal}}'
printf 'Existing container count: '
docker ps -q | wc -l
