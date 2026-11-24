#!/bin/sh
# Ensure a safe, unprivileged system group and user `speedle` exist before unpack.
# Refuses to silently adopt an incompatible pre-existing account or privileged ID.
set -eu

root="${DPKG_ROOT:-}"
root_args=""
if [ -n "${root}" ]; then
  root_args="--root ${root}"
fi

fail() {
  printf 'speedle preinstall error: %s\n' "$1" >&2
  exit 1
}

is_uint() {
  case "${1:-}" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

is_nologin_shell() {
  case "${1:-}" in
    /usr/sbin/nologin|/sbin/nologin|/bin/false|/usr/bin/false) return 0 ;;
    *) return 1 ;;
  esac
}

pick_nologin_shell() {
  for candidate in /usr/sbin/nologin /sbin/nologin /usr/bin/false /bin/false; do
    if [ -x "${root}${candidate}" ] || [ -z "${root}" ] && [ -x "${candidate}" ]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  printf '%s\n' "/usr/sbin/nologin"
}

group_entry=$(getent group speedle 2>/dev/null || true)
if [ -n "${group_entry}" ]; then
  gid=$(printf '%s\n' "${group_entry}" | cut -d: -f3)
  is_uint "${gid}" || fail "unable to parse numeric GID for existing group 'speedle'"
  [ "${gid}" -ne 0 ] || fail "refusing existing group 'speedle' with privileged GID 0"
else
  # shellcheck disable=SC2086
  groupadd ${root_args} --system speedle
  group_entry=$(getent group speedle 2>/dev/null || true)
  [ -n "${group_entry}" ] || fail "created group 'speedle' could not be resolved via getent"
  gid=$(printf '%s\n' "${group_entry}" | cut -d: -f3)
  is_uint "${gid}" || fail "unable to parse numeric GID for created group 'speedle'"
  [ "${gid}" -ne 0 ] || fail "created group 'speedle' unexpectedly resolved to GID 0"
fi

passwd_entry=$(getent passwd speedle 2>/dev/null || true)
if [ -n "${passwd_entry}" ]; then
  uid=$(printf '%s\n' "${passwd_entry}" | cut -d: -f3)
  user_gid=$(printf '%s\n' "${passwd_entry}" | cut -d: -f4)
  home_dir=$(printf '%s\n' "${passwd_entry}" | cut -d: -f6)
  login_shell=$(printf '%s\n' "${passwd_entry}" | cut -d: -f7)

  is_uint "${uid}" || fail "unable to parse numeric UID for existing user 'speedle'"
  [ "${uid}" -ne 0 ] || fail "refusing existing user 'speedle' with privileged UID 0"
  is_uint "${user_gid}" || fail "unable to parse primary GID for existing user 'speedle'"
  [ "${user_gid}" = "${gid}" ] || fail "refusing existing user 'speedle': primary GID ${user_gid} does not match group 'speedle' (${gid})"
  [ "${home_dir}" = "/var/lib/speedle" ] || fail "refusing existing user 'speedle': home directory '${home_dir}' is not /var/lib/speedle"
  is_nologin_shell "${login_shell}" || fail "refusing existing user 'speedle': shell '${login_shell}' must be non-interactive (nologin/false)"
else
  nologin_shell=$(pick_nologin_shell)
  # shellcheck disable=SC2086
  useradd ${root_args} --system --gid speedle --home-dir /var/lib/speedle \
    --no-create-home --shell "${nologin_shell}" \
    --comment "Speedle authorization service" speedle
fi
