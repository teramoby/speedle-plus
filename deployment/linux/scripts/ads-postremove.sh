#!/bin/sh
# Reload systemd after removal or upgrade. On RPM upgrades ($1 >= 1), restart
# speedle-ads if active. User data (/var/lib/speedle, policies.json) and the
# `speedle` system account are always preserved on remove and purge.
set -eu

root="${DPKG_ROOT:-}"
action="${1:-}"

if [ -z "${root}" ] && [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
  case "${action}" in
    ''|*[!0-9]*) ;;
    *)
      if [ "${action}" -ge 1 ]; then
        systemctl try-restart speedle-ads.service >/dev/null 2>&1 || true
      fi
      ;;
  esac
fi
