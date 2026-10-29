"""Offline tests for scripts/ci/coverage.py."""
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "ci"))
import coverage  # noqa: E402

M = "github.com/teramoby/speedle-plus/"


def profile(*blocks):
    path = Path(tempfile.mkdtemp()) / "c.out"
    path.write_text("mode: set\n" + "".join(f"{M}{b}\n" for b in blocks))
    return path


class CoverageTest(unittest.TestCase):
    def setUp(self):
        self.min = Path(tempfile.mkdtemp()) / "min"
        self.min.write_text("90\n")

    def run_gate(self, path, listed=""):
        result = mock.Mock(stdout=listed)
        with mock.patch("subprocess.run", return_value=result), mock.patch("builtins.print"):
            return coverage.main([str(path), "--min-file", str(self.min), *(["p"] if listed else [])])

    def test_counts_statements_and_ignores_repeated_blocks(self):
        total, covered = coverage.load_profile(profile(
            "pkg/a/x.go:1.1,2.2 9 1", "pkg/a/x.go:3.1,4.2 1 0", "pkg/a/x.go:1.1,2.2 9 0"))
        self.assertEqual((total["pkg/a"], covered["pkg/a"]), (10, 9))

    def test_a_block_covered_by_any_profile_counts(self):
        total, covered = coverage.load_profile(
            profile("pkg/a/x.go:1.1,2.2 9 0", "pkg/a/x.go:3.1,4.2 1 0"), profile("pkg/a/x.go:1.1,2.2 9 1"))
        self.assertEqual((total["pkg/a"], covered["pkg/a"]), (10, 9))

    def test_generated_protobuf_code_is_not_counted(self):
        total, _ = coverage.load_profile(profile("pkg/a/x.go:1.1,2.2 2 1", "pkg/a/pb/s.pb.go:1.1,2.2 50 0"))
        self.assertEqual(dict(total), {"pkg/a": 2})

    def test_merged_profiles_can_pass_the_gate(self):
        below = profile("pkg/a/x.go:1.1,2.2 5 1", "pkg/a/x.go:3.1,4.2 5 0")
        cli = profile("pkg/a/x.go:3.1,4.2 5 1")
        with mock.patch("builtins.print"):
            self.assertEqual(coverage.main([str(below), "--min-file", str(self.min)]), 1)
            self.assertEqual(coverage.main([str(below), "--merge", str(cli), "--min-file", str(self.min)]), 0)

    def test_accepts_packages_after_the_options_as_run_go_tests_passes_them(self):
        # run-go-tests.sh gate: profile --merge cli --min-file min packages...
        result = mock.Mock(stdout=f"{M}pkg/a 1 0\n")
        argv = [str(profile("pkg/a/x.go:1.1,2.2 9 1")), "--merge", str(profile("pkg/a/x.go:1.1,2.2 9 1")),
                "--min-file", str(self.min), f"{M}pkg/a", f"{M}pkg/b"]
        with mock.patch("subprocess.run", return_value=result), mock.patch("builtins.print"):
            self.assertEqual(coverage.main(argv), 0)

    def test_passes_at_the_minimum(self):
        self.assertEqual(self.run_gate(profile("pkg/a/x.go:1.1,2.2 9 1", "pkg/a/x.go:3.1,4.2 1 0")), 0)

    def test_fails_below_the_minimum(self):
        self.assertEqual(self.run_gate(profile("pkg/a/x.go:1.1,2.2 8 1", "pkg/a/x.go:3.1,4.2 2 0")), 1)

    def test_fails_when_a_package_has_no_tests(self):
        listed = f"{M}pkg/a 1 0\n{M}pkg/new 0 0\n{M}api/pms 0 0\n"
        self.assertEqual(self.run_gate(profile("pkg/a/x.go:1.1,2.2 9 1"), listed), 1)

    def test_declaration_only_packages_are_allowed_without_tests(self):
        self.assertEqual(self.run_gate(profile("pkg/a/x.go:1.1,2.2 9 1"), f"{M}api/pms 0 0\n"), 0)

    def test_json_output(self):
        out = Path(tempfile.mkdtemp()) / "c.json"
        with mock.patch("builtins.print"):
            coverage.main([str(profile("pkg/a/x.go:1.1,2.2 1 1")), "--min-file", str(self.min), "--json", str(out)])
        self.assertEqual(json.loads(out.read_text())["percent"], 100.0)


if __name__ == "__main__":
    unittest.main()
