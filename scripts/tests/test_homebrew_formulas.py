"""Offline tests for the Homebrew formula updater."""

import importlib.util
import re
import shutil
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "update_formulas", ROOT / "deployment" / "homebrew" / "update-formulas.py")
update_formulas = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(update_formulas)


FORMULAS = ROOT / "deployment" / "homebrew" / "Formula"


def checksums_for(formula_dir):
    sums = {}
    for formula in formula_dir.glob("*.rb"):
        lines = formula.read_text().splitlines()
        for index, line in enumerate(lines):
            match = update_formulas.URL_RE.match(line)
            if match is None:
                continue
            sha = update_formulas.SHA_RE.match(lines[index + 1])
            digest = sha.group(0).split('"')[1]
            sums[match.group("asset")] = digest
    return sums


class HomebrewFormulaTest(unittest.TestCase):
    def test_committed_formulas_round_trip(self):
        with tempfile.TemporaryDirectory() as temporary:
            copy = Path(temporary) / "Formula"
            shutil.copytree(FORMULAS, copy)
            versions = {
                version
                for formula in copy.glob("*.rb")
                for version in re.findall(
                    r"/releases/download/(v\d+\.\d+\.\d+)/", formula.read_text())
            }
            self.assertEqual(len(versions), 1, f"Formula release versions differ: {versions}")
            version = versions.pop()
            sums = checksums_for(copy)
            self.assertEqual(len(sums), 13)
            checksums_digest = sums.pop("checksums.txt")
            self.assertEqual(len(sums), 12)
            checksums = Path(temporary) / "checksums.txt"
            checksums.write_text("".join(f"{digest}  {asset}\n" for asset, digest in sorted(sums.items())))
            before = {path.name: path.read_text() for path in copy.glob("*.rb")}
            update_formulas.update_formulas(
                copy, version, update_formulas.load_checksums(checksums), checksums_digest)
            after = {path.name: path.read_text() for path in copy.glob("*.rb")}
            self.assertEqual(before, after)

    def test_updater_replaces_version_and_checksum(self):
        with tempfile.TemporaryDirectory() as temporary:
            formula_dir = Path(temporary)
            formula = formula_dir / "spctl.rb"
            formula.write_text(
                "\n".join([
                    'url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/spctl_darwin_arm64"',
                    'sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"',
                    "",
                ])
            )
            checksums = formula_dir / "checksums.txt"
            digest = "b" * 64
            checksums.write_text(f"{digest}  spctl_darwin_arm64\n")
            update_formulas.update_formulas(formula_dir, "v1.8.0", update_formulas.load_checksums(checksums))
            text = formula.read_text()
            self.assertIn("/download/v1.8.0/spctl_darwin_arm64", text)
            self.assertIn(f'sha256 "{digest}"', text)
            self.assertNotIn("v1.7.0", text)
            self.assertNotIn("aaa", text)

    def test_missing_checksum_writes_nothing(self):
        with tempfile.TemporaryDirectory() as temporary:
            formula_dir = Path(temporary)
            formula = formula_dir / "spctl.rb"
            original = 'url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/spctl_darwin_arm64"\nsha256 "' + ("c" * 64) + '"\n'
            formula.write_text(original)
            with self.assertRaises(update_formulas.FormulaUpdateError):
                update_formulas.update_formulas(formula_dir, "v1.8.0", {})
            self.assertEqual(formula.read_text(), original)

    def test_formulas_match_the_homebrew_demo(self):
        spctl = (FORMULAS / "spctl.rb").read_text()
        self.assertIn('bin.install binaries.fetch(0) => "speedle-spctl"', spctl)
        self.assertIn('bin.install_symlink bin/"speedle-spctl" => "spctl" if OS.linux?', spctl)
        self.assertNotIn("=> name", spctl)

        for name, port in (("speedle-pms.rb", "6733"), ("speedle-ads.rb", "6734")):
            text = (FORMULAS / name).read_text()
            self.assertIn("service do", text)
            self.assertIn(f"127.0.0.1:{port}", text)
            self.assertIn('var/"speedle/policies.json"', text)
            self.assertIn('var/"log/', text)
            self.assertIn('"--insecure=true"', text)
            self.assertIn('"--store-type"', text)
            self.assertIn('"file"', text)
            self.assertIn('(var/"speedle").mkpath', text)
            self.assertIn('(var/"log").mkpath', text)
            self.assertIn('assert_path_exists var/"speedle"', text)

        speedle = (FORMULAS / "speedle.rb").read_text()
        for dependency in ('"spctl"', '"speedle-ads"', '"speedle-pms"'):
            self.assertIn(f"depends_on {dependency}", speedle)
        self.assertRegex(
            speedle,
            r'url "https://github\.com/teramoby/speedle-plus/releases/download/'
            r'v\d+\.\d+\.\d+/checksums\.txt"',
        )


if __name__ == "__main__":
    unittest.main()
