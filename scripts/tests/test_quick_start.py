"""Unit checks for the Quick Start checker."""
import importlib.util
import os
import re
import subprocess
import tempfile
from pathlib import Path
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "check_quick_start", ROOT / "website" / "tests" / "check_quick_start.py"
)
QUICK_START = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(QUICK_START)


class LatestInstallVersionTest(unittest.TestCase):
    def test_queries_public_tags_and_accepts_the_highest_version(self):
        env = {"GOTOOLCHAIN": "local"}
        listing = (
            "a\trefs/tags/v1.5.0\n"
            "b\trefs/tags/v1.6.0\n"
            "c\trefs/tags/v1.6.0^{}\n"
            "d\trefs/tags/v1.6\n"
        )
        with mock.patch.object(QUICK_START, "run", return_value=listing) as run:
            QUICK_START.check_latest_install_version(
                "github.com/teramoby/speedle-plus", "v1.6.0", env
            )
        run.assert_called_once_with(
            ["git", "ls-remote", "--tags", "https://github.com/teramoby/speedle-plus.git", "refs/tags/v*"],
            ROOT,
            env,
            timeout=120,
        )

    def test_retries_the_public_listing_after_a_transient_failure(self):
        listing = "a\trefs/tags/v1.6.0\n"
        failure = RuntimeError("Command failed (128): git ls-remote")
        with mock.patch.object(
                QUICK_START, "run", side_effect=[failure, failure, listing]) as run, \
                mock.patch.object(QUICK_START.time, "sleep") as sleep:
            QUICK_START.check_latest_install_version(
                "github.com/teramoby/speedle-plus", "v1.6.0", {}
            )
        self.assertEqual(run.call_count, 3)
        self.assertEqual(sleep.call_args_list, [mock.call(5.0), mock.call(10.0)])

    def test_rejects_a_stale_pin_and_names_the_replacement(self):
        listing = "a\trefs/tags/v1.6.0\nb\trefs/tags/v1.7.0\n"
        with mock.patch.object(QUICK_START, "run", return_value=listing):
            with self.assertRaisesRegex(
                AssertionError,
                r"stale github\.com/teramoby/speedle-plus@v1\.6\.0.*v1\.7\.0",
            ):
                QUICK_START.check_latest_install_version(
                    "github.com/teramoby/speedle-plus", "v1.6.0", {}
                )

    def test_detects_a_new_major_and_compares_versions_numerically(self):
        for latest in ("v1.10.0", "v2.0.0"):
            listing = f"a\trefs/tags/v1.9.0\nb\trefs/tags/{latest}\n"
            with self.subTest(latest=latest), mock.patch.object(QUICK_START, "run", return_value=listing):
                with self.assertRaisesRegex(AssertionError, re.escape(latest)):
                    QUICK_START.check_latest_install_version(
                        "github.com/teramoby/speedle-plus", "v1.9.0", {}
                    )

    def test_rejects_a_response_without_a_full_release_version(self):
        with mock.patch.object(QUICK_START, "run", return_value="a\trefs/tags/v1.6\n"):
            with self.assertRaisesRegex(AssertionError, "No full semantic version tags"):
                QUICK_START.check_latest_install_version(
                    "github.com/teramoby/speedle-plus", "v1.6.0", {}
                )


class NetworkRetryTest(unittest.TestCase):
    def fake_command(self, work, succeed_on):
        """Record each attempt in a file and fail until the requested one."""
        attempts = work / "attempts"
        command = f'printf x >> "{attempts}"; test "$(wc -c < "{attempts}")" -ge {succeed_on}'
        return command, attempts

    def test_retries_a_command_that_fails_twice_and_then_succeeds(self):
        with tempfile.TemporaryDirectory() as temporary:
            work = Path(temporary)
            command, attempts = self.fake_command(work, succeed_on=3)
            result = QUICK_START.run_with_retry(
                lambda: QUICK_START.run(command, work, dict(os.environ), timeout=10),
                delay=0,
            )
            self.assertEqual(result, "")
            self.assertEqual(attempts.read_text(), "xxx")

    def test_gives_up_after_three_attempts_and_reraises_the_failure(self):
        with tempfile.TemporaryDirectory() as temporary:
            work = Path(temporary)
            command, attempts = self.fake_command(work, succeed_on=4)
            with self.assertRaisesRegex(RuntimeError, r"Command failed \(1\)"):
                QUICK_START.run_with_retry(
                    lambda: QUICK_START.run(command, work, dict(os.environ), timeout=10),
                    delay=0,
                )
            self.assertEqual(attempts.read_text(), "xxx")

    def test_does_not_retry_a_timeout(self):
        # A hung command must not multiply the per-command timeout.
        action = mock.Mock(side_effect=subprocess.TimeoutExpired(cmd="git clone", timeout=180))
        with mock.patch.object(QUICK_START.time, "sleep") as sleep, \
                self.assertRaises(subprocess.TimeoutExpired):
            QUICK_START.run_with_retry(action)
        action.assert_called_once_with()
        sleep.assert_not_called()

    def test_runs_the_reset_before_the_retried_attempt(self):
        action = mock.Mock(side_effect=[RuntimeError("Command failed (128): git clone"), "ok"])
        reset = mock.Mock()
        with mock.patch.object(QUICK_START.time, "sleep"):
            self.assertEqual(QUICK_START.run_with_retry(action, reset=reset), "ok")
        reset.assert_called_once_with()

    def test_reports_a_cleanup_failure_chained_to_the_command_failure(self):
        action = mock.Mock(side_effect=RuntimeError("Command failed (128): git clone"))
        reset = mock.Mock(side_effect=OSError("permission denied"))
        with mock.patch.object(QUICK_START.time, "sleep") as sleep, \
                self.assertRaises(OSError) as raised:
            QUICK_START.run_with_retry(action, reset=reset)
        self.assertIsInstance(raised.exception.__cause__, RuntimeError)
        action.assert_called_once_with()
        sleep.assert_not_called()


if __name__ == "__main__":
    unittest.main()
