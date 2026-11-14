#!/usr/bin/env python3
"""Exercise source provenance without borrowing the checkout's Git objects."""
import hashlib
import importlib.util
import io
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

HERE = Path(__file__).resolve().parents[1]
REVISION = next(line.split("=", 1)[1] for line in (HERE / "build.sh").read_text().splitlines() if line.startswith("REVISION="))


class SourceProvenance(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="speedle-proof-test-")
        self.addCleanup(self.directory.cleanup)
        self.work = Path(self.directory.name)
        spec = importlib.util.spec_from_file_location("engine_source", HERE / "engine-source.py")
        self.source = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.source)
        self.source.PACK = self.work / "engine-source.pack"
        self.source.CHECKSUM = self.work / "engine-source.sha256"
        shutil.copyfile(HERE / "engine-source.pack", self.source.PACK)
        shutil.copyfile(HERE / "engine-source.sha256", self.source.CHECKSUM)

    def test_standalone_pack_exports_source_without_original_history(self):
        exported = self.source.verified_archive(REVISION)
        destination = self.work / "source"
        self.source.extract(exported, destination)
        self.assertIn(b'"log/slog"', (destination / "pkg/logging/initlog.go").read_bytes())
        self.assertTrue((destination / "api/ads/interface.go").exists() or any((destination / "api/ads").glob("*.go")))
        self.assertFalse((destination / ".git").exists())
        with tarfile.open(fileobj=io.BytesIO(exported)) as archive:
            self.assertTrue(all(member.isfile() for member in archive.getmembers()))

    def test_wrong_commit_is_rejected_even_with_matching_pack_checksum(self):
        with self.assertRaises(subprocess.CalledProcessError):
            self.source.verified_archive("f" * 40)

    def test_changed_source_with_recomputed_checksum_cannot_claim_pinned_commit(self):
        # Retain the genuine commit and tree objects but replace one source blob
        # with modified evaluator logic and recompute the outer SHA-256. Git's
        # tree still names the original blob, so verification must fail.
        repo = self.work / "objects"
        git = self.source.git
        prefix = ("--git-dir", str(repo))
        git("init", "--bare", "--initial-branch=engine-snapshot", str(repo))
        git(*prefix, "unpack-objects", "-r", data=self.source.PACK.read_bytes())
        original = git(*prefix, "rev-parse", REVISION + ":pkg/eval/evaluator.go").strip()
        content = git(*prefix, "cat-file", "blob", original.decode())
        git(*prefix, "hash-object", "-w", "--stdin", data=content + b"\n// substituted source\n")
        objects = git(*prefix, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)").splitlines()
        forged = git(*prefix, "pack-objects", "--stdout", data=b"\n".join(oid for oid in objects if oid != original) + b"\n")
        self.source.PACK.write_bytes(forged)
        self.source.CHECKSUM.write_text(hashlib.sha256(forged).hexdigest() + "  engine-source.pack\n")
        with self.assertRaisesRegex(ValueError, "blob is missing"):
            self.source.verified_archive(REVISION)

    def test_corrupt_pack_checksum_is_rejected(self):
        self.source.PACK.write_bytes(self.source.PACK.read_bytes() + b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            self.source.verified_archive(REVISION)


if __name__ == "__main__":
    unittest.main()
