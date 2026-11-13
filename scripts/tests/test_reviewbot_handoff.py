# Adapted for Speedle. The guard must not log review text or secret values.
"""Exercise the network-free handoff guard with synthetic credentials only."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


HELPER = Path(__file__).resolve().parents[2] / ".github/codex/reviewbot_handoff.py"
REASON_INVALID = "ReviewBot reason_code=invalid_handoff\n"
REASON_MATCH = "ReviewBot reason_code=secret_match\n"


class HandoffTests(unittest.TestCase):
    def seal(self, result=None, *, env=None, raw=None):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        source = Path(directory.name) / "result.json"
        destination = Path(directory.name) / "report.json"
        source.write_text(raw if raw is not None else json.dumps(
            {"message": "A review with Unicode 中文 and `code`.", "model": "fixture-model"}
            if result is None else result))
        destination.write_text("stale report must disappear on failure")
        environment = {
            **os.environ,
            "REVIEWBOT_SECRETS": json.dumps({
                "github_token": "synthetic-context-token",
                "ARK_API_KEY": "synthetic-ark-credential",
                "DEEPSEEK_API_KEY": "synthetic-deepseek-credential",
            }),
            "GH_TOKEN": "synthetic-job-token",
            "REVIEWBOT_RETRIED": "false",
            "GITHUB_RUN_ID": "123",
            "GITHUB_RUN_ATTEMPT": "2",
            "PULL_NUMBER": "7",
            "HEAD_SHA": "a" * 40,
            "COMMAND": "review",
            **(env or {}),
        }
        process = subprocess.run(
            [sys.executable, str(HELPER), str(source), str(destination)],
            env=environment, text=True, capture_output=True)
        payload = destination.read_text() if destination.exists() else ""
        return process, payload, destination.exists()

    def assert_invalid(self, process, payload, exists):
        self.assertNotEqual(process.returncode, 0)
        self.assertEqual(payload, "")
        self.assertFalse(exists)
        self.assertEqual(process.stdout, "")
        self.assertEqual(process.stderr, REASON_INVALID)

    def test_accepts_injected_github_token_and_either_provider_key(self):
        process, payload, exists = self.seal()
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertTrue(exists)
        report = json.loads(payload)
        self.assertEqual(report["model"], "fixture-model")
        self.assertEqual(process.stdout + process.stderr, "")
        for secrets in (
            {"github_token": "synthetic-context-token", "DEEPSEEK_API_KEY": "synthetic-deepseek-credential"},
            {"ARK_API_KEY": "synthetic-ark-credential"},
        ):
            with self.subTest(secrets=sorted(secrets)):
                process, payload, exists = self.seal(env={"REVIEWBOT_SECRETS": json.dumps(secrets)})
                self.assertEqual(process.returncode, 0, process.stderr)
                self.assertTrue(exists)
                self.assertEqual(process.stdout + process.stderr, "")

    def test_unrecognized_secret_names_and_non_string_values_fail_closed(self):
        for name in (
            "ark_api_key", "Github_token", "github_Token", "github_token_extra",
            "github.token", "github-token", "github_token\n", "_SECRET", "1SECRET", "SÉCRET",
        ):
            with self.subTest(name=name):
                process, payload, exists = self.seal(env={"REVIEWBOT_SECRETS": json.dumps({
                    "github_token": "synthetic-context-token",
                    "ARK_API_KEY": "synthetic-ark-credential",
                    name: "synthetic-invalid-name-credential",
                })})
                self.assert_invalid(process, payload, exists)
                self.assertNotIn(name, process.stdout + process.stderr)
                self.assertNotIn("synthetic-invalid-name-credential", process.stdout + process.stderr)
        for name in ("github_token", "NEW_INHERITED_SECRET"):
            for value in (None, True, 123, [], {}):
                with self.subTest(name=name, value=value):
                    process, payload, exists = self.seal(env={"REVIEWBOT_SECRETS": json.dumps({
                        "ARK_API_KEY": "synthetic-ark-credential", name: value,
                    })})
                    self.assert_invalid(process, payload, exists)

    def test_commands_and_retry_preserve_binding_and_markdown(self):
        for mode in ("review", "recheck", "explain"):
            for retry in ("true", "false"):
                with self.subTest(mode=mode, retry=retry):
                    process, payload, exists = self.seal(env={"COMMAND": mode, "REVIEWBOT_RETRIED": retry})
                    self.assertEqual(process.returncode, 0, process.stderr)
                    self.assertTrue(exists)
                    report = json.loads(payload)
                    self.assertEqual(set(report), {
                        "version", "message", "model", "run_id", "run_attempt",
                        "pull_number", "head_sha", "command",
                    })
                    self.assertEqual(report["version"], 1)
                    self.assertEqual(report["command"], mode)
                    self.assertEqual(report["head_sha"], "a" * 40)
                    self.assertEqual(report["run_id"], "123")
                    self.assertEqual(report["run_attempt"], "2")
                    self.assertEqual(report["pull_number"], "7")
                    self.assertIn("中文 and `code`", report["message"])
                    self.assertEqual("unified=3" in report["message"], retry == "true")
                    self.assertEqual(process.stdout + process.stderr, "")

    def test_secret_match_leaves_no_artifact_and_names_nothing(self):
        samples = (
            ("ARK_API_KEY", "synthetic-ark-credential"),
            ("DEEPSEEK_API_KEY", "synthetic-deepseek-credential"),
            ("github_token", "synthetic-context-token"),
            ("GITHUB_TOKEN", "synthetic-job-token"),
        )
        for name, value in samples:
            for field in ("message", "model"):
                with self.subTest(name=name, field=field):
                    result = {"message": "safe review text", "model": "fixture-model", field: value}
                    process, payload, exists = self.seal(result)
                    self.assertNotEqual(process.returncode, 0)
                    self.assertEqual(payload, "")
                    self.assertFalse(exists)
                    self.assertEqual(process.stdout, "")
                    self.assertEqual(process.stderr, REASON_MATCH)
                    self.assertNotIn(name, process.stderr)
                    self.assertNotIn(value, process.stdout + process.stderr)
        quoted = 'a"quoted\ncredential'
        process, payload, exists = self.seal(
            {"message": "review with a\"quoted\ncredential", "model": "fixture-model"},
            env={"REVIEWBOT_SECRETS": json.dumps({
                "ARK_API_KEY": "synthetic-ark-credential",
                "DEEPSEEK_API_KEY": quoted,
            })})
        self.assertNotEqual(process.returncode, 0)
        self.assertEqual(process.stderr, REASON_MATCH)
        self.assertEqual(payload, "")
        self.assertFalse(exists)
        self.assertNotIn(quoted, process.stdout + process.stderr)
        self.assertNotIn("DEEPSEEK_API_KEY", process.stderr)

    def test_invalid_shape_size_metadata_and_missing_credentials_fail_without_values(self):
        for result in (
            [], {}, {"message": " ", "model": "fixture-model"},
            {"message": "safe", "model": "bad\nmodel"},
            {"message": "safe", "model": "fixture-model", "extra": True},
            {"message": "x" * 240001, "model": "fixture-model"},
            {"message": "中" * 80001, "model": "fixture-model"},
        ):
            with self.subTest(result=type(result).__name__):
                self.assert_invalid(*self.seal(result))
        for variable, value in (
            ("COMMAND", "invalid"), ("HEAD_SHA", "bad"),
            ("GITHUB_RUN_ID", "0"), ("GITHUB_RUN_ATTEMPT", ""),
            ("PULL_NUMBER", "-1"), ("REVIEWBOT_RETRIED", ""),
            ("REVIEWBOT_SECRETS", "[]"), ("REVIEWBOT_SECRETS", "{}"),
            ("REVIEWBOT_SECRETS", "not json"), ("GH_TOKEN", ""),
            ("REVIEWBOT_SECRETS", json.dumps({"github_token": "synthetic-context-token"})),
        ):
            with self.subTest(variable=variable, value=value):
                process, payload, exists = self.seal(env={variable: value})
                self.assert_invalid(process, payload, exists)
                self.assertNotIn("synthetic-context-token", process.stdout + process.stderr)
                self.assertNotIn("synthetic-ark-credential", process.stdout + process.stderr)
        process, payload, exists = self.seal(raw="not valid JSON PRIVATE_DATA")
        self.assert_invalid(process, payload, exists)
        self.assertNotIn("PRIVATE_DATA", process.stdout + process.stderr)

    def test_message_boundary_and_new_secret_are_guarded(self):
        process, payload, exists = self.seal({"message": "x" * 240000, "model": "fixture-model"})
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertTrue(exists)
        self.assertEqual(len(json.loads(payload)["message"]), 240000)
        process, payload, exists = self.seal(
            {"message": "synthetic-new-credential", "model": "fixture-model"},
            env={"REVIEWBOT_SECRETS": json.dumps({
                "ARK_API_KEY": "synthetic-ark-credential",
                "NEW_INHERITED_SECRET": "synthetic-new-credential",
            })})
        self.assertNotEqual(process.returncode, 0)
        self.assertEqual(payload, "")
        self.assertFalse(exists)
        self.assertEqual(process.stderr, REASON_MATCH)
        self.assertNotIn("NEW_INHERITED_SECRET", process.stderr)
        self.assertNotIn("synthetic-new-credential", process.stdout + process.stderr)

    def test_wrong_arguments_fail_without_reading_a_secret(self):
        process = subprocess.run(
            [sys.executable, str(HELPER)], text=True, capture_output=True)
        self.assertNotEqual(process.returncode, 0)
        self.assertEqual(process.stderr, REASON_INVALID)
        self.assertEqual(process.stdout, "")


if __name__ == "__main__":
    unittest.main()
