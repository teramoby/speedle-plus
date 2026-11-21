"""Unit checks for the embedded guide checker."""
import contextlib
import io
import json
import subprocess
import sys
from pathlib import Path
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "website" / "tests"))
import check_embedded

SETUP = (
    "mkdir speedle-embedded-demo\n"
    "cd speedle-embedded-demo\n"
    "go mod init example.com/speedle-embedded-demo\n"
    "go get github.com/teramoby/speedle-plus/pkg/eval@v1.9.0"
)
SETUP_PREFIX = (
    "mkdir speedle-embedded-demo\n"
    "cd speedle-embedded-demo\n"
    "go mod init example.com/speedle-embedded-demo"
)
GET = "go get github.com/teramoby/speedle-plus/pkg/eval@v1.9.0"
BUILD = "go mod tidy\ngo build -o ./bin/embedded-demo ."
TIDY = "go mod tidy"
BUILD_COMMAND = "go build -o ./bin/embedded-demo ."
RUN = "./bin/embedded-demo"
GO_LIST = "go list -m -json github.com/teramoby/speedle-plus"
STEPS = {
    "version": "go version",
    "setup": SETUP,
    "policies": "[service.mysvc]\n[policy]\ngrant user alice get res1",
    "program": "package main",
    "build": BUILD,
    "run": RUN,
}
VERSION = "go version go1.26.10 darwin/arm64\n"
MODULE = {"Path": "github.com/teramoby/speedle-plus", "Version": "v1.9.0"}
REPLACED_MODULE = {
    "Path": "github.com/teramoby/speedle-plus",
    "Version": "v1.9.0",
    "Replace": {"Dir": str(ROOT)},
}
ALLOWED = "alice allowed=true\nbob allowed=false\n"


class FakeRun:
    """Stand in for the checker's `run`, failing selected call numbers.

    `failures` and `timeouts` map a command to the 1-based call numbers that
    fail with a command error or a timeout, as a transient network error or a
    hung command would.
    """

    def __init__(self, failures=None, timeouts=None, run_output=ALLOWED):
        self.failures = failures or {}
        self.timeouts = timeouts or {}
        self.run_output = run_output
        self.calls = []

    def occurrences(self, command):
        return sum(1 for call in self.calls if call == command)

    def __call__(self, command, cwd, env):
        self.calls.append(command)
        if isinstance(command, str):
            if self.occurrences(command) in self.timeouts.get(command, ()):
                raise subprocess.TimeoutExpired(cmd=command, timeout=600)
            if self.occurrences(command) in self.failures.get(command, ()):
                raise RuntimeError(f"Command failed: {command}")
        if command == STEPS["version"]:
            return VERSION
        if command == GO_LIST:
            return json.dumps(MODULE if self.occurrences(command) == 1 else REPLACED_MODULE) + "\n"
        if command == ["git", "rev-parse", "HEAD"]:
            return "deadbeef\n"
        if command == SETUP_PREFIX:
            app = Path(cwd) / "speedle-embedded-demo"
            app.mkdir()
            (app / "go.mod").write_text("module example.com/speedle-embedded-demo\n")
            return ""
        if command == GET:
            (Path(cwd) / "go.sum").write_text("")
            return ""
        if command == RUN:
            return self.run_output
        return ""


class EmbeddedRetryTest(unittest.TestCase):
    def verify(self, fake, candidate=False):
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()):
            check_embedded.verify("en", STEPS, None, candidate)
        return sleep

    def test_retries_a_transient_go_get_failure_and_then_passes(self):
        fake = FakeRun(failures={GET: (1,)})
        sleep = self.verify(fake)
        self.assertEqual(fake.occurrences(SETUP_PREFIX), 1)
        self.assertEqual(fake.occurrences(GET), 2)
        self.assertEqual(fake.occurrences(TIDY), 1)
        self.assertEqual(fake.occurrences(BUILD_COMMAND), 1)
        self.assertEqual(fake.occurrences(RUN), 1)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0)])

    def test_does_not_retry_a_compile_failure_in_the_documented_build(self):
        # A defect in the guide must fail on the first attempt instead of
        # looking like a flaky download.
        fake = FakeRun(failures={BUILD_COMMAND: (1,)})
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaisesRegex(RuntimeError, "go build"):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(GET), 1)
        self.assertEqual(fake.occurrences(TIDY), 1)
        self.assertEqual(fake.occurrences(BUILD_COMMAND), 1)
        self.assertEqual(fake.occurrences(RUN), 0)
        sleep.assert_not_called()

    def test_does_not_retry_a_setup_command_that_times_out(self):
        # A hung local command is not a download and must not multiply the
        # 600 s limit.
        fake = FakeRun(timeouts={SETUP_PREFIX: (1,)})
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaises(subprocess.TimeoutExpired):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(SETUP_PREFIX), 1)
        self.assertEqual(fake.occurrences(GET), 0)
        sleep.assert_not_called()

    def test_does_not_retry_a_go_get_that_times_out(self):
        fake = FakeRun(timeouts={GET: (1,)})
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaises(subprocess.TimeoutExpired):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(GET), 1)
        self.assertEqual(fake.occurrences(TIDY), 0)
        sleep.assert_not_called()

    def test_does_not_retry_a_go_mod_tidy_that_times_out(self):
        fake = FakeRun(timeouts={TIDY: (1,)})
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaises(subprocess.TimeoutExpired):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(TIDY), 1)
        self.assertEqual(fake.occurrences(BUILD_COMMAND), 0)
        sleep.assert_not_called()

    def test_retries_a_transient_go_mod_tidy_failure(self):
        fake = FakeRun(failures={TIDY: (1,)})
        sleep = self.verify(fake)
        self.assertEqual(fake.occurrences(TIDY), 2)
        self.assertEqual(fake.occurrences(BUILD_COMMAND), 1)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0)])

    def test_retries_the_candidate_build_download_after_a_transient_failure(self):
        fake = FakeRun(failures={TIDY: (2,)})
        sleep = self.verify(fake, candidate=True)
        self.assertEqual(fake.occurrences(TIDY), 3)
        self.assertEqual(fake.occurrences(BUILD_COMMAND), 2)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0)])

    def test_a_wrong_allow_deny_result_fails_at_once(self):
        fake = FakeRun(run_output="alice allowed=true\nbob allowed=true\n")
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaisesRegex(AssertionError, "bob allowed=true"):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(RUN), 1)
        sleep.assert_not_called()

    def test_gives_up_after_three_failed_go_get_attempts(self):
        fake = FakeRun(failures={GET: (1, 2, 3)})
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaisesRegex(RuntimeError, "Command failed"):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(GET), 3)
        self.assertEqual(fake.occurrences(TIDY), 0)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0), mock.call(10.0)])

    def test_reports_each_retry_with_a_neutral_command_failure_message(self):
        fake = FakeRun(failures={GET: (1,)})
        output = io.StringIO()
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep"), \
                contextlib.redirect_stdout(output):
            check_embedded.verify("en", STEPS, None)
        self.assertIn(
            f"Command failed (attempt 1/3); retrying in 5s: Command failed: {GET}",
            output.getvalue().splitlines(),
        )
        self.assertNotIn("Transient", output.getvalue())


class SplitNetworkCommandTest(unittest.TestCase):
    def test_splits_the_documented_setup_block_around_go_get(self):
        steps = check_embedded.blocks("en")
        before, command, after = check_embedded.split_network_command(steps["setup"], "go get")
        self.assertEqual(before, SETUP_PREFIX)
        self.assertEqual(command, GET)
        self.assertEqual(after, "")

    def test_splits_the_documented_build_block_around_go_mod_tidy(self):
        steps = check_embedded.blocks("en")
        before, command, after = check_embedded.split_network_command(steps["build"], "go mod tidy")
        self.assertEqual(before, "")
        self.assertEqual(command, TIDY)
        self.assertEqual(after, BUILD_COMMAND)

    def test_requires_exactly_one_matching_command(self):
        for block in ("mkdir -p bin\ngo build .", "go mod tidy\ngo mod tidy\ngo build ."):
            with self.subTest(block=block), self.assertRaises(AssertionError):
                check_embedded.split_network_command(block, "go mod tidy")


class CheckerImportTest(unittest.TestCase):
    def test_checkers_load_without_the_caller_adding_their_directory(self):
        # Isolated mode keeps the working directory and the site packages out
        # of sys.path: the checkers must import nothing from their own
        # directory, so the retry helpers stay private copies.
        script = (
            "import importlib.util, sys\n"
            "directory = sys.argv[1]\n"
            "for name in ('check_quick_start', 'check_embedded'):\n"
            "    path = directory + '/' + name + '.py'\n"
            "    spec = importlib.util.spec_from_file_location(name, path)\n"
            "    spec.loader.exec_module(importlib.util.module_from_spec(spec))\n"
        )
        subprocess.run(
            [sys.executable, "-I", "-c", script, str(ROOT / "website" / "tests")],
            check=True, capture_output=True, text=True, timeout=120,
        )


if __name__ == "__main__":
    unittest.main()
