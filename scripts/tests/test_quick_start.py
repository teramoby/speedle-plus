"""Unit checks for the Quick Start checker."""
import contextlib
import importlib.util
import io
import json
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

    def test_reports_each_retry_with_a_neutral_command_failure_message(self):
        action = mock.Mock(side_effect=[
            RuntimeError("Command failed (128): git clone\nfatal: early EOF"), "ok"])
        output = io.StringIO()
        with mock.patch.object(QUICK_START.time, "sleep"), contextlib.redirect_stdout(output):
            QUICK_START.run_with_retry(action)
        self.assertEqual(
            output.getvalue().splitlines(),
            ["Command failed (attempt 1/3); retrying in 5s: Command failed (128): git clone"],
        )


class SplitNetworkCommandTest(unittest.TestCase):
    def quick_start(self, locale=""):
        return dict(QUICK_START.blocks(ROOT / "docs/hugo/content" / f"quick-start{locale}.md"))

    def test_splits_the_documented_install_block_around_go_install(self):
        before, command, after = QUICK_START.split_network_command(
            self.quick_start()["install"], "go install")
        self.assertEqual(before, "")
        self.assertTrue(command.startswith('GOBIN="$PWD/bin" go install '), command)
        self.assertEqual(after, "ls bin")

    def test_splits_the_documented_build_block_around_go_build(self):
        build = self.quick_start()["build"].splitlines()[2:]
        before, command, after = QUICK_START.split_network_command("\n".join(build), "go build")
        self.assertEqual(before, "mkdir -p bin")
        self.assertTrue(command.startswith("go build -o ./bin/ "), command)
        self.assertEqual(after, "ls bin")

    def test_chinese_blocks_split_the_same_way(self):
        for name, network in (("install", "go install"), ("build", "go build")):
            with self.subTest(name=name):
                english = QUICK_START.split_network_command(self.quick_start()[name], network)
                chinese = QUICK_START.split_network_command(self.quick_start(".zh")[name], network)
                self.assertEqual(english, chinese)

    def test_requires_exactly_one_matching_command(self):
        for block in ("mkdir -p bin", "go build .\nls bin\ngo build -o x ."):
            with self.subTest(block=block), self.assertRaises(AssertionError):
                QUICK_START.split_network_command(block, "go build")


class DecisionWaitTest(unittest.TestCase):
    """ADS reloads the policy file asynchronously after a documented create."""

    def wait(self, responses, clock=None, name="allow-direct"):
        with contextlib.ExitStack() as stack:
            run = stack.enter_context(mock.patch.object(
                QUICK_START, "run", side_effect=[json.dumps(response) for response in responses]))
            sleep = stack.enter_context(mock.patch.object(QUICK_START.time, "sleep"))
            monotonic = None
            if clock is not None:
                monotonic = stack.enter_context(
                    mock.patch.object(QUICK_START.time, "monotonic", side_effect=clock))
            result = QUICK_START.wait_for_decision(
                name, "curl", ROOT, {"GOFLAGS": ""})
        return result, run, sleep, monotonic

    def test_returns_the_first_matching_decision_without_sleeping(self):
        result, run, sleep, _ = self.wait([{"allowed": True, "reason": 0}])
        self.assertEqual(result, {"allowed": True, "reason": 0})
        run.assert_called_once_with("curl", ROOT, {"GOFLAGS": ""})
        sleep.assert_not_called()

    def test_retries_the_unchanged_documented_command_until_ads_reloads(self):
        result, run, sleep, _ = self.wait(
            [{"allowed": False, "reason": 3}, {"allowed": False, "reason": 3},
             {"allowed": True, "reason": 0}],
            clock=[0.0, 0.0, 5.0],
        )
        self.assertEqual(result, {"allowed": True, "reason": 0})
        self.assertEqual(run.call_count, 3)
        self.assertEqual(sleep.call_args_list,
                         [mock.call(QUICK_START.DECISION_INTERVAL)] * 2)

    def test_returns_the_last_decision_after_the_deadline_passes(self):
        stale = {"allowed": False, "reason": 3}
        result, run, sleep, _ = self.wait([stale], clock=[0.0, 10.0])
        self.assertEqual(result, stale)
        self.assertEqual(run.call_count, 1)
        sleep.assert_not_called()

    def test_a_deny_step_retries_an_unexpected_reason(self):
        result, run, sleep, _ = self.wait(
            [{"allowed": False, "reason": 1}, {"allowed": False, "reason": 3}],
            clock=[0.0, 5.0], name="deny",
        )
        self.assertEqual(result, {"allowed": False, "reason": 3})
        self.assertEqual(run.call_count, 2)
        self.assertEqual(sleep.call_args_list, [mock.call(QUICK_START.DECISION_INTERVAL)])

    def test_a_permanent_mismatch_is_returned_for_the_callers_assertion(self):
        # wait_for_decision never asserts: exercise() keeps the original final
        # assertions and their messages, including the last observed response.
        stale = {"allowed": False, "reason": 3}
        with contextlib.ExitStack() as stack:
            stack.enter_context(mock.patch.object(
                QUICK_START, "run", return_value=json.dumps(stale)))
            stack.enter_context(mock.patch.object(QUICK_START.time, "sleep"))
            stack.enter_context(mock.patch.object(
                QUICK_START.time, "monotonic", side_effect=[0.0, 11.0]))
            result = QUICK_START.wait_for_decision("allow-direct", "curl", ROOT, {})
        self.assertEqual(result, stale)


class DecisionStepsTest(unittest.TestCase):
    def test_covers_every_documented_decision_step_in_order(self):
        expected = {
            "": ["allow-direct", "allow-role", "deny", "abac-allow", "abac-deny"],
            ".zh": ["allow-direct", "deny", "abac-allow", "abac-deny"],
        }
        for locale, names in expected.items():
            with self.subTest(locale=locale or "en"):
                steps = QUICK_START.blocks(
                    ROOT / "docs/hugo/content" / f"quick-start{locale}.md")
                documented = [name for name, _ in steps if name in QUICK_START.DECISIONS]
                self.assertEqual(documented, names)


class ExerciseDecisionRetryTest(unittest.TestCase):
    def test_retries_only_the_decision_steps_until_ads_reloads(self):
        steps = QUICK_START.blocks(ROOT / "docs/hugo/content/quick-start.md")
        documented = dict(steps)
        pending = {name: 1 for name in QUICK_START.DECISIONS}
        calls = []

        def run(command, cwd, env, timeout=60):
            calls.append(command)
            for name in QUICK_START.DECISIONS:
                if command == documented[name]:
                    allowed = QUICK_START.DECISIONS[name]
                    current = not allowed if pending[name] else allowed
                    pending[name] = 0
                    return json.dumps({"allowed": current, "reason": 0 if current else 3})
            if command == documented["list"]:
                return json.dumps([{
                    "name": "mysvc",
                    "policies": [{}, {}],
                    "rolePolicies": [{}],
                }])
            return ""

        class Process:
            pid = os.getpid()

            def __init__(self, *args, **kwargs):
                pass

            def poll(self):
                return None

        with tempfile.TemporaryDirectory() as temporary:
            checkout = Path(temporary) / "speedle-plus"
            checkout.mkdir()
            (checkout / "quick-start-policies.json").write_text(
                json.dumps({"services": [{"name": "mysvc"}]}))
            with mock.patch.object(QUICK_START, "ports", return_value=(6733, 6734)), \
                    mock.patch.object(QUICK_START, "server_lock",
                                      lambda: contextlib.nullcontext()), \
                    mock.patch.object(QUICK_START.subprocess, "Popen", Process), \
                    mock.patch.object(QUICK_START, "wait_ready"), \
                    mock.patch.object(QUICK_START, "stop"), \
                    mock.patch.object(QUICK_START.time, "sleep"), \
                    mock.patch.object(QUICK_START, "run", side_effect=run), \
                    contextlib.redirect_stdout(io.StringIO()):
                result = QUICK_START.exercise("en", steps, checkout, dict(os.environ))
        for name in QUICK_START.DECISIONS:
            with self.subTest(step=name):
                self.assertEqual(calls.count(documented[name]), 2)
        self.assertEqual(calls.count(documented["list"]), 1)
        for name in ("service", "policies", "roles", "abac-policy"):
            with self.subTest(step=name):
                self.assertEqual(calls.count(documented[name]), 1)
        self.assertEqual(result["decisions"], QUICK_START.DECISIONS)


class DocumentedBuildTest(unittest.TestCase):
    def test_only_the_clone_and_module_download_are_retried(self):
        steps = QUICK_START.blocks(ROOT / "docs/hugo/content/quick-start.md")
        version = dict(steps)["version"]
        calls, retried = [], []

        def run(command, cwd, env, timeout=60):
            calls.append(command)
            if isinstance(command, list):
                return "deadbeef\n" if command == ["git", "rev-parse", "HEAD"] else ""
            if command == version:
                return "go version go1.26.10 darwin/arm64\n"
            if command.startswith("git clone "):
                for binary in ("spctl", "speedle-pms", "speedle-ads"):
                    path = Path(cwd) / "speedle-plus" / "bin" / binary
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.touch()
                    path.chmod(0o755)
                return ""
            if command == "ls bin":
                return "spctl\nspeedle-ads\nspeedle-pms\n"
            if command in {"cd speedle-plus", "mkdir -p bin"} or command.startswith("go build "):
                return ""
            raise AssertionError(f"unexpected command: {command}")

        def retry(action, reset=None, attempts=3, delay=5.0):
            start = len(calls)
            result = action()
            retried.extend(calls[start:])
            return result

        # Force the portable branch: CI runs Linux, where verify() re-executes
        # itself in unprivileged namespaces instead of calling exercise().
        with mock.patch.object(QUICK_START, "run", run), \
                mock.patch.object(QUICK_START, "run_with_retry", retry), \
                mock.patch.object(QUICK_START.sys, "platform", "darwin"), \
                mock.patch.object(QUICK_START, "exercise", return_value={"steps": [], "decisions": {}}):
            QUICK_START.verify("en", None, dict(os.environ))
        self.assertEqual(retried, [
            "git clone --depth 1 https://github.com/teramoby/speedle-plus.git",
            ["go", "mod", "download"],
        ])
        self.assertIn(
            "go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads", calls)
        self.assertIn("cd speedle-plus", calls)
        self.assertIn("ls bin", calls)


if __name__ == "__main__":
    unittest.main()
