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
    "go get github.com/teramoby/speedle-plus/pkg/eval@v1.6.0"
)
BUILD = "go mod tidy\ngo build -o ./bin/embedded-demo ."
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
MODULE = {"Path": "github.com/teramoby/speedle-plus", "Version": "v1.6.0"}
REPLACED_MODULE = {
    "Path": "github.com/teramoby/speedle-plus",
    "Version": "v1.6.0",
    "Replace": {"Dir": str(ROOT)},
}
ALLOWED = "alice allowed=true\nbob allowed=false\n"


class FakeRun:
    """Stand in for the checker's `run`, failing selected call numbers.

    `failures` and `timeouts` map a command to the 1-based call numbers that
    fail with a command error or a timeout, as a transient network error would.
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
        if command == STEPS["setup"]:
            app = Path(cwd) / "speedle-embedded-demo"
            app.mkdir()
            (app / "go.mod").write_text("module example.com/speedle-embedded-demo\n")
            (app / "go.sum").write_text("")
            return ""
        if command == STEPS["run"]:
            return self.run_output
        return ""


class EmbeddedRetryTest(unittest.TestCase):
    def verify(self, fake, candidate=False):
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()):
            check_embedded.verify("en", STEPS, None, candidate)
        return sleep

    def test_retries_a_transient_setup_failure_and_then_passes(self):
        fake = FakeRun(failures={SETUP: (1,)})
        sleep = self.verify(fake)
        self.assertEqual(fake.occurrences(SETUP), 2)
        self.assertEqual(fake.occurrences(BUILD), 1)
        self.assertEqual(fake.occurrences(RUN), 1)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0)])

    def test_retries_a_setup_attempt_that_times_out(self):
        fake = FakeRun(timeouts={SETUP: (1,)})
        sleep = self.verify(fake)
        self.assertEqual(fake.occurrences(SETUP), 2)
        self.assertEqual(fake.occurrences(RUN), 1)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0)])

    def test_clears_the_partial_module_before_retrying_setup(self):
        fake = FakeRun(failures={SETUP: (1,)})
        partial_present = []

        def run(command, cwd, env):
            if command == SETUP:
                app = Path(cwd) / "speedle-embedded-demo"
                if fake.occurrences(SETUP) == 0:
                    app.mkdir()
                    (app / "go.mod").write_text("module example.com/speedle-embedded-demo\n")
                else:
                    partial_present.append(app.exists())
            return fake(command, cwd, env)

        with mock.patch.object(check_embedded, "run", run), \
                mock.patch.object(check_embedded.time, "sleep"), \
                contextlib.redirect_stdout(io.StringIO()):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(partial_present, [False])

    def test_retries_a_transient_download_during_the_build(self):
        fake = FakeRun(failures={BUILD: (1,)})
        sleep = self.verify(fake)
        self.assertEqual(fake.occurrences(BUILD), 2)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0)])

    def test_retries_the_candidate_build_after_a_transient_failure(self):
        fake = FakeRun(failures={BUILD: (2,)})
        sleep = self.verify(fake, candidate=True)
        self.assertEqual(fake.occurrences(BUILD), 3)
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

    def test_gives_up_after_three_failed_setup_attempts(self):
        fake = FakeRun(failures={SETUP: (1, 2, 3)})
        with mock.patch.object(check_embedded, "run", fake), \
                mock.patch.object(check_embedded.time, "sleep") as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaisesRegex(RuntimeError, "Command failed"):
            check_embedded.verify("en", STEPS, None)
        self.assertEqual(fake.occurrences(SETUP), 3)
        self.assertEqual(fake.occurrences(BUILD), 0)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0), mock.call(10.0)])


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
