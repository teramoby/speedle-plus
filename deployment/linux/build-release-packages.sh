#!/usr/bin/env bash
# Build deterministic .deb/.rpm archives and Windows Scoop/winget release manifests
# from checksum-verified release binaries in DIST_DIR, then verify .deb metadata
# and update CHECKSUMS idempotently.
set -euo pipefail

version="${VERSION:?VERSION is required}"
dist_dir="${DIST_DIR:?DIST_DIR is required}"
checksums="${CHECKSUMS:?CHECKSUMS is required}"
license_path="${LICENSE_PATH:?LICENSE_PATH is required}"
source_epoch="${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH is required}"

[[ "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "VERSION must look like v1.2.3." >&2; exit 1; }
[[ "${source_epoch}" =~ ^[0-9]+$ ]] || { echo "SOURCE_DATE_EPOCH must be a non-negative integer." >&2; exit 1; }
[[ -f "${license_path}" && ! -L "${license_path}" ]] || { echo "LICENSE_PATH must be a regular file from the release commit: ${license_path}" >&2; exit 1; }

script_dir=$(dirname -- "$0")
script_dir=$(cd -- "${script_dir}" >/dev/null && pwd)
packaging_root=$(cd -- "${script_dir}/.." >/dev/null && pwd)

nfpm_bin="${NFPM_BIN:-}"
if [[ -z "${nfpm_bin}" ]]; then
  nfpm_tmp=$(mktemp -d "${TMPDIR:-/tmp}/speedle-nfpm-bin.XXXXXX")
  trap 'rm -rf "${nfpm_tmp}"' EXIT
  NFPM_INSTALL_DIR="${nfpm_tmp}" bash "${script_dir}/install-nfpm.sh"
  nfpm_bin="${nfpm_tmp}/nfpm"
fi

python3 -I "${script_dir}/build-packages.py" \
  --version "${version}" \
  --dist-dir "${dist_dir}" \
  --checksums "${checksums}" \
  --license "${license_path}" \
  --output-dir "${dist_dir}" \
  --nfpm-bin "${nfpm_bin}" \
  --source-date-epoch "${source_epoch}" \
  --update-checksums

if command -v dpkg-deb >/dev/null 2>&1; then
  bare_version="${version#v}"
  for pkg in spctl speedle-pms speedle-ads; do
    for deb_arch in amd64 arm64; do
      deb_file="${dist_dir}/${pkg}_${bare_version}-1_${deb_arch}.deb"
      dpkg-deb --info "${deb_file}" >/dev/null
      dpkg-deb --contents "${deb_file}" >/dev/null
    done
  done
fi

python3 -I "${packaging_root}/winget/update-windows-manifests.py" \
  --version "${version}" \
  --checksums "${checksums}" \
  --binaries-dir "${dist_dir}" \
  --scoop-template "${packaging_root}/scoop/spctl.json.tmpl" \
  --output-dir "${dist_dir}" \
  --source-date-epoch "${source_epoch}" \
  --update-checksums

cat "${checksums}"
