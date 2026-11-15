#!/usr/bin/env python3
"""Keep a self-contained, commit-authenticated Engine source snapshot.

The pack carries the pinned commit, tree objects and selected source blobs.
Git verifies object IDs and walks the commit's real tree before exporting any
source. No original PR history, branch, hidden ref or network fetch is needed.
"""
import argparse
import hashlib
import io
import os
import re
from pathlib import Path
import subprocess
import tarfile
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
PACK = HERE / "engine-source.pack"
CHECKSUM = HERE / "engine-source.sha256"
PATHS = ["api", "pkg", "3rdparty", "go.mod", "go.sum", "LICENSE", "THIRD_PARTY_LICENSES.txt"]
# A partial proof must never borrow objects from the caller's Git repository.
GIT_ENV = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
GIT_ENV.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)


def git(*args, cwd=ROOT, data=None):
    return subprocess.run(["git", *args], cwd=cwd, input=data, env=GIT_ENV,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout


def snapshot(revision):
    revision = git("rev-parse", "--verify", "--end-of-options", revision + "^{commit}").decode().strip()
    objects = {revision, git("rev-parse", revision + "^{tree}").decode().strip()}
    # archive walks trees to find .gitattributes even outside the selected paths.
    # Include those trees/attribute blobs, but not unrelated blobs or PR parents.
    for entry in git("ls-tree", "-r", "-t", revision).decode().splitlines():
        metadata, path = entry.split("\t", 1)
        _, kind, oid = metadata.split()
        if kind == "tree" or path.endswith(".gitattributes"):
            objects.add(oid)
    for entry in git("ls-tree", "-r", revision, "--", *PATHS).decode().splitlines():
        objects.add(entry.split()[2])
    data = git("pack-objects", "--stdout", data=("\n".join(sorted(objects)) + "\n").encode())
    PACK.write_bytes(data)
    CHECKSUM.write_text(hashlib.sha256(data).hexdigest() + "  " + PACK.name + "\n")
    return revision


def verified_archive(revision):
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("Engine source requires an exact Git commit hash")
    data = PACK.read_bytes()
    actual = hashlib.sha256(data).hexdigest()
    if actual != CHECKSUM.read_text().split()[0]:
        raise ValueError("Engine source pack checksum mismatch")
    with tempfile.TemporaryDirectory(prefix="speedle-source-proof-") as directory:
        repository = Path(directory)
        prefix = ("--git-dir", str(repository))
        git("init", "--bare", "--object-format=sha1", "--initial-branch=engine-snapshot", str(repository))
        git(*prefix, "unpack-objects", "-r", data=data)
        # Resolve the exact commit and its real tree, never a tar comment.
        if git(*prefix, "cat-file", "-t", revision).strip() != b"commit":
            raise ValueError("Engine source revision is not a commit")
        entries = []
        for entry in git(*prefix, "ls-tree", "-r", "-z", revision, "--", *PATHS).split(b"\0"):
            if not entry:
                continue
            metadata, path = entry.split(b"\t", 1)
            mode, kind, oid = metadata.split()
            if kind != b"blob" or mode not in (b"100644", b"100755"):
                raise ValueError("Engine source contains an unsupported object")
            entries.append((mode, oid, path.decode()))
        blobs = git(*prefix, "cat-file", "--batch", data=b"\n".join(e[1] for e in entries) + b"\n")
        offset = 0
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w:") as archive:
            for mode, oid, path in entries:
                end = blobs.index(b"\n", offset)
                header = blobs[offset:end].split()
                if len(header) != 3 or header[0] != oid or header[1] != b"blob":
                    raise ValueError("Engine source blob is missing or invalid")
                size = int(header[2])
                content = blobs[end + 1:end + 1 + size]
                if len(content) != size or blobs[end + 1 + size:end + 2 + size] != b"\n":
                    raise ValueError("Engine source blob is truncated")
                offset = end + 2 + size
                member = tarfile.TarInfo(path)
                member.size, member.mode = size, int(mode, 8) & 0o777
                archive.addfile(member, io.BytesIO(content))
    print(f"Verified Engine commit/tree/blob provenance {revision}: pack SHA-256 {actual}")
    return output.getvalue()


def extract(source, destination):
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    if any(destination.iterdir()):
        raise ValueError("Engine source destination must be empty")
    with tarfile.open(fileobj=io.BytesIO(source), mode="r:") as archive:
        for member in archive.getmembers():
            path = Path(member.name)
            if path.is_absolute() or ".." in path.parts or not (member.isdir() or member.isfile()):
                raise ValueError("Engine source contains an unsafe path")
            target = destination / path
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(archive.extractfile(member).read())
                target.chmod(member.mode & 0o777)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("snapshot", "verify"))
    parser.add_argument("revision")
    parser.add_argument("destination", nargs="?")
    args = parser.parse_args()
    revision = snapshot(args.revision) if args.mode == "snapshot" else args.revision
    source = verified_archive(revision)
    if args.destination:
        extract(source, args.destination)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"Engine source provenance verification failed: {error}")
