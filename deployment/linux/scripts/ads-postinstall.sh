#!/bin/sh
# Configure speedle-ads permissions and state directory without recursive chown
# or auto-starting unconfigured listeners.
set -eu

root="${DPKG_ROOT:-}"
etc_dir="${root}/etc/speedle"
cfg_file="${etc_dir}/ads.json"
env_file="${root}/etc/default/speedle-ads"
state_parent="${root}/var/lib"
state_dir="${state_parent}/speedle"
policy_file="${state_dir}/policies.json"

fail() {
  printf 'speedle-ads postinstall error: %s\n' "$1" >&2
  exit 1
}

[ ! -L "${etc_dir}" ] || fail "refusing symlink at /etc/speedle"
[ -d "${etc_dir}" ] || fail "missing directory /etc/speedle"
etc_uid=$(stat -c '%u' "${etc_dir}")
[ "${etc_uid}" = "0" ] || fail "refusing unsafe ownership on /etc/speedle: expected uid 0, got ${etc_uid}"
chown root:speedle "${etc_dir}"
chmod 0750 "${etc_dir}"

for f in "${cfg_file}" "${env_file}"; do
  [ ! -L "${f}" ] || fail "refusing symlink at ${f#"${root}"}"
  [ -f "${f}" ] || fail "missing regular configuration file ${f#"${root}"}"
  f_uid=$(stat -c '%u' "${f}")
  [ "${f_uid}" = "0" ] || fail "refusing unsafe ownership on ${f#"${root}"}: expected uid 0, got ${f_uid}"
  chown root:speedle "${f}"
  chmod 0640 "${f}"
done

[ ! -L "${state_dir}" ] || fail "refusing symlink at /var/lib/speedle"
if [ ! -e "${state_dir}" ]; then
  [ ! -L "${state_parent}" ] || fail "refusing symlink at /var/lib"
  [ -d "${state_parent}" ] || fail "missing parent directory /var/lib"
  install -d -m 0750 -o speedle -g speedle "${state_dir}"
else
  [ -d "${state_dir}" ] || fail "/var/lib/speedle exists and is not a directory"
  state_meta=$(stat -c '%U:%G:%a' "${state_dir}")
  state_owner=$(printf '%s\n' "${state_meta}" | cut -d: -f1)
  state_group=$(printf '%s\n' "${state_meta}" | cut -d: -f2)
  state_mode=$(printf '%s\n' "${state_meta}" | cut -d: -f3)
  [ "${state_owner}" = "speedle" ] && [ "${state_group}" = "speedle" ] || \
    fail "refusing preexisting /var/lib/speedle with ownership ${state_owner}:${state_group} (expected speedle:speedle)"
  case "${state_mode}" in
    700|750|770|2750|2770) ;;
    *) fail "refusing preexisting /var/lib/speedle with unsafe mode ${state_mode} (expected 0700/0750/0770)" ;;
  esac
fi

[ ! -L "${policy_file}" ] || fail "refusing symlink at /var/lib/speedle/policies.json"
if [ ! -e "${policy_file}" ]; then
  tmp_policy=$(mktemp "${state_dir}/.policies.init.XXXXXX")
  chown speedle:speedle "${tmp_policy}"
  chmod 0640 "${tmp_policy}"
  printf '{}\n' > "${tmp_policy}"
  ln "${tmp_policy}" "${policy_file}" 2>/dev/null || true
  rm -f "${tmp_policy}"
fi

[ ! -L "${policy_file}" ] || fail "refusing symlink at /var/lib/speedle/policies.json"
[ -f "${policy_file}" ] || fail "/var/lib/speedle/policies.json is not a regular file"
policy_meta=$(stat -c '%U:%G:%a' "${policy_file}")
policy_owner=$(printf '%s\n' "${policy_meta}" | cut -d: -f1)
policy_group=$(printf '%s\n' "${policy_meta}" | cut -d: -f2)
policy_mode=$(printf '%s\n' "${policy_meta}" | cut -d: -f3)
[ "${policy_owner}" = "speedle" ] && [ "${policy_group}" = "speedle" ] || \
  fail "refusing /var/lib/speedle/policies.json with ownership ${policy_owner}:${policy_group} (expected speedle:speedle)"
case "${policy_mode}" in
  600|640|660) ;;
  *) fail "refusing /var/lib/speedle/policies.json with unsafe mode ${policy_mode} (expected 0600/0640/0660)" ;;
esac

if [ -z "${root}" ] && [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
  # On Debian upgrades ($1 = configure, $2 = previous version), restart only if active.
  # Fresh installs never enable or start the service automatically.
  if [ "${1:-}" = "configure" ] && [ -n "${2:-}" ]; then
    systemctl try-restart speedle-ads.service >/dev/null 2>&1 || true
  fi
fi
