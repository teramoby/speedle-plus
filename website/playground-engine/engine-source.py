#!/usr/bin/env python3
"""Snapshot/verify the immutable Engine sources used for the browser build.

The checked-in archive survives squash merges and branch deletion; rebuilding
requires neither an unmerged commit object nor fetching hidden PR refs.
"""
import gzip
import hashlib
import io
from pathlib import Path
import subprocess
import sys
import tarfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
ARCHIVE = HERE / "engine-source.tar.gz"
CHECKSUM = HERE / "engine-source.sha256"
PATHS = ["api", "pkg", "3rdparty", "go.mod", "go.sum", "LICENSE", "THIRD_PARTY_LICENSES.txt"]


def main():
    mode, revision = sys.argv[1:]
    if mode == "snapshot":
        revision = subprocess.check_output(["git", "rev-parse", revision + "^{commit}"], cwd=ROOT, text=True).strip()
        source = subprocess.check_output(["git", "archive", "--format=tar", revision, *PATHS], cwd=ROOT)
        output = io.BytesIO()
        with gzip.GzipFile(fileobj=output, mode="wb", filename="", mtime=0) as compressed:
            compressed.write(source)
        ARCHIVE.write_bytes(output.getvalue())
        CHECKSUM.write_text(hashlib.sha256(output.getvalue()).hexdigest() + "  " + ARCHIVE.name + "\n")
    elif mode != "verify":
        raise SystemExit("usage: engine-source.py snapshot|verify REVISION")
    data = ARCHIVE.read_bytes()
    expected = CHECKSUM.read_text().split()[0]
    actual = hashlib.sha256(data).hexdigest()
    if actual != expected:
        raise SystemExit("Engine source archive checksum mismatch")
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        if archive.pax_headers.get("comment") != revision:
            raise SystemExit("Engine source archive revision mismatch")
        for member in archive.getmembers():
            path = Path(member.name)
            if path.is_absolute() or ".." in path.parts or member.issym() or member.islnk():
                raise SystemExit("Engine source archive contains an unsafe path")
    print(f"Verified Engine source {revision}: SHA-256 {actual}")


if __name__ == "__main__":
    main()
