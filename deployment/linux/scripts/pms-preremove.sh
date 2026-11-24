#!/bin/sh
# Stop and disable speedle-pms only on final package removal (Debian `remove`, RPM `0`),
# never during an in-place package upgrade.
set -eu

root="${DPKG_ROOT:-}"
action="${1:-}"

if [ "${action}" = "remove" ] || [ "${action}" = "0" ]; then
  if [ -z "${root}" ] && [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
    systemctl --no-reload disable --now speedle-pms.service >/dev/null 2>&1 || true
  fi
fi
