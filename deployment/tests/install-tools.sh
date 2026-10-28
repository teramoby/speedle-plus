#!/usr/bin/env bash
set -euo pipefail
# Install pinned tools only into the caller's disposable test directory.
target=${1:?Usage: install-tools.sh TEST_TOOL_DIRECTORY}
mkdir -p "$target"
target=$(cd "$target" && pwd)
case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) exit 1 ;; esac
case "$(uname -m)" in x86_64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) exit 1 ;; esac
fetch() { curl -fsSL --connect-timeout 10 --max-time 180 --retry 2 "$1" -o "$2"; }
check() {
  python3 - "$1" "$2" <<'PY'
import hashlib,sys
from pathlib import Path
data,checksum=map(Path,sys.argv[1:])
expected=checksum.read_text().split()[0]
assert len(expected)==64 and hashlib.sha256(data.read_bytes()).hexdigest()==expected, 'Tool checksum mismatch'
PY
}
fetch "https://get.helm.sh/helm-v4.3.1-${os}-${arch}.tar.gz" "$target/helm.tar.gz"
fetch "https://get.helm.sh/helm-v4.3.1-${os}-${arch}.tar.gz.sha256sum" "$target/helm.sha256"
check "$target/helm.tar.gz" "$target/helm.sha256"
tar -xzf "$target/helm.tar.gz" -C "$target"
mv "$target/${os}-${arch}/helm" "$target/helm"
fetch "https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-${os}-${arch}" "$target/kind"
fetch "https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-${os}-${arch}.sha256sum" "$target/kind.sha256"
check "$target/kind" "$target/kind.sha256"
fetch "https://dl.k8s.io/release/v1.36.4/bin/${os}/${arch}/kubectl" "$target/kubectl"
fetch "https://dl.k8s.io/release/v1.36.4/bin/${os}/${arch}/kubectl.sha256" "$target/kubectl.sha256"
check "$target/kubectl" "$target/kubectl.sha256"
chmod +x "$target/helm" "$target/kind" "$target/kubectl"
"$target/helm" version --short
"$target/kind" version
"$target/kubectl" version --client
