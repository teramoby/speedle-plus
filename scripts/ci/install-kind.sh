#!/usr/bin/env bash
# Install pinned kind and kubectl builds. Checksums are for linux-amd64.
# kind v0.33.0 defaults to Kubernetes 1.37.0, and kubectl matches that release.
set -euo pipefail

kind_version=v0.33.0
kind_checksum=aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d
kubectl_version=v1.37.0
kubectl_checksum=6129359f4e1f3848a5572ccb0b26cf28b8ca08cef38c95a765b2f64a2c961a2f

[[ "$(uname -m)" == "x86_64" ]] || { echo "install-kind.sh installs the linux-amd64 binaries." >&2; exit 1; }

tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT

curl -fsSL -o "${tmpdir}/kind" "https://github.com/kubernetes-sigs/kind/releases/download/${kind_version}/kind-linux-amd64"
echo "${kind_checksum}  ${tmpdir}/kind" | sha256sum -c -
sudo install -m 755 "${tmpdir}/kind" /usr/local/bin/kind

curl -fsSL -o "${tmpdir}/kubectl" "https://dl.k8s.io/release/${kubectl_version}/bin/linux/amd64/kubectl"
echo "${kubectl_checksum}  ${tmpdir}/kubectl" | sha256sum -c -
sudo install -m 755 "${tmpdir}/kubectl" /usr/local/bin/kubectl

kind version
kubectl version --client
