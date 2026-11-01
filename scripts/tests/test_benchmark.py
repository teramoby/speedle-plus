"""Offline tests for scripts/ci/benchmark.py. No real benchmarks run."""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "ci"))
import benchmark  # noqa: E402

OUTPUT = """goos: darwin
pkg: github.com/teramoby/speedle-plus/benchmarks
BenchmarkOne-8          	  237543	      4391 ns/op	    2864 B/op	      47 allocs/op
BenchmarkLargeExp-8     	   85657	     66791.5 ns/op	   10288 B/op	      93 allocs/op
BenchmarkNoMem          	     100	        12 ns/op
PASS
"""


def run_of(**ns):
    return {name: {"ns": value, "bytes": 0.0, "allocs": 0.0} for name, value in ns.items()}


class ParseTest(unittest.TestCase):
    def test_reads_time_memory_and_allocations(self):
        self.assertEqual(benchmark.parse(OUTPUT), {
            "BenchmarkOne": {"ns": 4391.0, "bytes": 2864.0, "allocs": 47.0},
            "BenchmarkLargeExp": {"ns": 66791.5, "bytes": 10288.0, "allocs": 93.0},
            "BenchmarkNoMem": {"ns": 12.0, "bytes": 0.0, "allocs": 0.0},
        })

    def test_ignores_other_lines(self):
        self.assertEqual(benchmark.parse("PASS\nok  pkg 1s\n--- FAIL: BenchmarkX\n"), {})

    def test_medians_tolerate_outliers_and_gaps(self):
        runs = [run_of(A=10, B=5), run_of(A=1000), run_of(A=12, B=7)]
        self.assertEqual(benchmark.medians(runs), {"A": {"ns": 12, "bytes": 0, "allocs": 0},
                                                   "B": {"ns": 6, "bytes": 0, "allocs": 0}})


class CompareTest(unittest.TestCase):
    def test_flags_only_slowdowns_beyond_the_threshold(self):
        base = benchmark.medians([run_of(Same=100, Faster=100, Slightly=100, Slower=100)])
        cand = benchmark.medians([run_of(Same=100, Faster=50, Slightly=119, Slower=121)])
        rows, regressed, missing = benchmark.compare(base, cand, 0.20)
        self.assertEqual(regressed, ["Slower"])
        self.assertEqual(missing, [])
        self.assertEqual({r["name"]: round(r["change"], 2) for r in rows},
                         {"Faster": -0.5, "Same": 0.0, "Slightly": 0.19, "Slower": 0.21})

    def test_reports_benchmarks_on_one_side_only(self):
        rows, regressed, missing = benchmark.compare(benchmark.medians([run_of(Old=1, Both=1)]),
                                                     benchmark.medians([run_of(New=1, Both=1)]))
        self.assertEqual(missing, ["New", "Old"])
        self.assertEqual([r["name"] for r in rows], ["Both"])

    def test_table(self):
        base = benchmark.medians([{"A": {"ns": 100, "bytes": 10, "allocs": 2}}])
        cand = benchmark.medians([{"A": {"ns": 150, "bytes": 12, "allocs": 3}}])
        text = benchmark.render(*benchmark.compare(base, cand), "a" * 40, "b" * 40, 5)
        self.assertIn("| A | 100 | 150 | +50.0% ⚠️ | 10 → 12 | 2 → 3 |", text)
        self.assertIn("Regressed: A.", text)
        self.assertIn("median of 5 alternating rounds", text)
        clean = benchmark.render(*benchmark.compare(base, base), "a" * 40, "b" * 40, 5)
        self.assertIn("No regression.", clean)
        self.assertNotIn("Runner:", clean)
        named = benchmark.render(*benchmark.compare(base, base), "a" * 40, "b" * 40, 5, runner="fiona-speedle (macOS/ARM64)")
        self.assertIn("Runner: fiona-speedle (macOS/ARM64)", named)

    def test_runner_description(self):
        self.assertEqual(benchmark.runner_description(
            {"RUNNER_NAME": "charming-speedle-2", "RUNNER_OS": "Linux", "RUNNER_ARCH": "X64"}),
            "charming-speedle-2 (Linux/X64)")
        # Outside Actions the host name and platform are used.
        self.assertRegex(benchmark.runner_description({}), r"^.+ \(.+/.+\)$")


class MainTest(unittest.TestCase):
    """Runs main against a throwaway repository with a fake benchmark runner."""

    def setUp(self):
        self.repo = Path(tempfile.mkdtemp())
        git = lambda *a: subprocess.run(["git", "-C", str(self.repo), *a], check=True, capture_output=True, text=True)
        git("init", "-q")
        git("config", "user.email", "t@example.invalid")
        git("config", "user.name", "t")
        (self.repo / "version").write_text("old\n")
        git("add", ".")
        git("commit", "-qm", "baseline")
        self.baseline = git("rev-parse", "HEAD").stdout.strip()
        (self.repo / "version").write_text("new\n")
        git("commit", "-qam", "candidate")
        self.results = Path(tempfile.mkdtemp()) / "results"

    def run_main(self, timings):
        calls = []

        def runner(directory):
            side = (Path(directory) / "version").read_text().strip()
            calls.append(side)
            return "\n".join(f"Benchmark{name}-8  100  {ns} ns/op  10 B/op  1 allocs/op"
                             for name, ns in timings[side].items())
        code = benchmark.main(["--results", str(self.results), "--repository", str(self.repo),
                               "--baseline", self.baseline, "--rounds", "3"], runner=runner)
        return code, calls

    def test_alternates_rounds_and_passes_without_regression(self):
        code, calls = self.run_main({"old": {"One": 100}, "new": {"One": 110}})
        self.assertEqual(code, 0)
        self.assertEqual(calls, ["old", "new"] * 3)
        data = json.loads((self.results / "performance.json").read_text())
        self.assertEqual(data["baseline"], self.baseline)
        self.assertTrue(data["runner"])
        self.assertIn("Runner: ", (self.results / "performance.md").read_text())
        self.assertEqual(data["regressed"], [])
        self.assertIn("No regression.", (self.results / "performance.md").read_text())
        # The baseline worktree is cleaned up.
        listed = subprocess.run(["git", "-C", str(self.repo), "worktree", "list"], capture_output=True, text=True).stdout
        self.assertEqual(len(listed.strip().splitlines()), 1)

    def test_a_regression_fails(self):
        code, _ = self.run_main({"old": {"One": 100}, "new": {"One": 150}})
        self.assertEqual(code, 1)
        self.assertIn("Regressed: BenchmarkOne.", (self.results / "performance.md").read_text())

    def test_a_missing_benchmark_fails(self):
        code, _ = self.run_main({"old": {"One": 100, "Two": 1}, "new": {"One": 100}})
        self.assertEqual(code, 1)

    def test_a_failing_benchmark_run_raises(self):
        def runner(directory):
            raise RuntimeError("go test failed")
        with self.assertRaises(RuntimeError):
            benchmark.main(["--results", str(self.results), "--repository", str(self.repo),
                            "--baseline", self.baseline, "--rounds", "1"], runner=runner)

    def test_the_default_baseline_is_master_in_may_2026(self):
        self.assertEqual(benchmark.BASELINE, "8fb1481d715e476242a991d391778b4334763afd")


if __name__ == "__main__":
    unittest.main()
