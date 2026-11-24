#!/usr/bin/env bash
# Install a SHA-256-pinned nFPM v2.49.0 build for Linux amd64 or arm64.
# Set NFPM_INSTALL_DIR to install into a user-writable directory without sudo.
set -euo pipefail

nfpm_version=2.49.0
case "$(uname -m)" in
  x86_64|amd64)
    nfpm_arch=x86_64
    nfpm_checksum=2afaa5ffb5e3493402e31e7c67a5a1fb42133e3f66fc315ca0051934125561c7
    ;;
  aarch64|arm64)
    nfpm_arch=arm64
    nfpm_checksum=d4eee71839e92c68cbd3b101640bad32d2856fc551acae0635be9118e48c9d9c
    ;;
  *)
    echo "install-nfpm.sh supports Linux x86_64 and arm64; got $(uname -m)." >&2
    exit 1
    ;;
esac

install_dir="${NFPM_INSTALL_DIR:-/usr/local/bin}"
tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/speedle-nfpm.XXXXXX")
trap 'rm -rf "${tmpdir}"' EXIT

archive="nfpm_${nfpm_version}_Linux_${nfpm_arch}.tar.gz"
curl -fsSL -o "${tmpdir}/${archive}" "https://github.com/goreleaser/nfpm/releases/download/v${nfpm_version}/${archive}"
echo "${nfpm_checksum}  ${tmpdir}/${archive}" | sha256sum -c -
tar -C "${tmpdir}" -xzf "${tmpdir}/${archive}" nfpm

if [[ -n "${NFPM_INSTALL_DIR:-}" ]]; then
  mkdir -p "${install_dir}"
  install -m 0755 "${tmpdir}/nfpm" "${install_dir}/nfpm"
elif [[ -w "${install_dir}" ]]; then
  install -m 0755 "${tmpdir}/nfpm" "${install_dir}/nfpm"
else
  sudo install -m 0755 "${tmpdir}/nfpm" "${install_dir}/nfpm"
fi

"${install_dir}/nfpm" --version
