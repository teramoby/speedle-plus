#!/usr/bin/env bash
# Install a pinned Helm 3 build. The checksum is for linux-amd64.
# The release workflow inlines the same version and checksum.
set -euo pipefail

helm_version=v3.22.1
helm_checksum=896cf63684a42a723d61c980369d6e588330927deb13676ca0d85006dd6da0ea

[[ "$(uname -m)" == "x86_64" ]] || { echo "install-helm.sh installs the linux-amd64 binary." >&2; exit 1; }

tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT
curl -fsSL -o "${tmpdir}/helm.tgz" "https://get.helm.sh/helm-${helm_version}-linux-amd64.tar.gz"
echo "${helm_checksum}  ${tmpdir}/helm.tgz" | sha256sum -c -
tar -C "${tmpdir}" -xzf "${tmpdir}/helm.tgz"
sudo install -m 755 "${tmpdir}/linux-amd64/helm" /usr/local/bin/helm
helm version --short
