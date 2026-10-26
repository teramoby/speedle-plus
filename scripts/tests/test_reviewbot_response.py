# Adapted from Pavu a7b458e; modified for Speedle+. Apache-2.0; see .github/codex/PAVU-LICENSE.txt.
"""Run both real workflow shell blocks with synthetic Git and stubbed HTTP only."""

import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True

from test_reviewbot_diff import ROOT, WORKFLOWS, workflow_script


FIXTURES = ROOT / "scripts/tests/fixtures/reviewbot"
HELPER = ".github/codex/reviewbot_response.py"
NORMAL = json.loads((FIXTURES / "normal.json").read_text())
LENGTH = json.loads((FIXTURES / "incomplete-length.json").read_text())


class ReviewBotResponseTests(unittest.TestCase):
    def run_workflow(self, workflow, step, responses, *, http_code="200", mode="review",
                     long_input=False):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        repo = root / "repo"
        repo.mkdir()

        def git(*args):
            return subprocess.run(["git", *args], cwd=repo, check=True,
                                  capture_output=True, text=True).stdout.strip()

        def write(path, text):
            target = repo / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text)

        git("init", "-qb", "main")
        git("config", "user.name", "ReviewBot fixture")
        git("config", "user.email", "fixture@example.invalid")
        git("config", "commit.gpgsign", "false")
        write(".github/codex/prompts/reviewbot.md", "TRUSTED_POLICY\n")
        write(".github/codex/prompts/reviewbot-command.md", "TRUSTED_COMMAND_POLICY\n")
        write("AGENTS.md", "TRUSTED_REPOSITORY_POLICY\n")
        if (ROOT / HELPER).exists():
            write(HELPER, (ROOT / HELPER).read_text())
        original = "".join(f"unchanged context {line:03d}\n" for line in range(220))
        write("feature.txt", original)
        git("add", ".")
        git("commit", "-qm", "Trusted base")
        base = git("rev-parse", "HEAD")
        write("feature.txt", original.replace("context 110", "FEATURE_CHANGE"))
        # A head version must never execute in the secret-bearing analysis job.
        write(HELPER, 'raise SystemExit("UNTRUSTED_HELPER_EXECUTED")\n')
        if long_input:
            write("large.txt", "added line " + "X" * 760000 + "\n")
        git("add", ".")
        git("commit", "-qm", "Untrusted feature")
        head = git("rev-parse", "HEAD")
        git("checkout", "-q", "--detach", head if workflow == "reviewbot.yml" else base)

        runtime = root / "runtime"
        runtime.mkdir()
        fixtures = root / "responses.json"
        fixtures.write_text(json.dumps(responses))
        stub = root / "bin"
        stub.mkdir()
        curl = stub / "curl"
        curl.write_text(f"#!{sys.executable}\n" + '''
import json, os, pathlib, sys
args = sys.argv[1:]
root = pathlib.Path(os.environ["RUNNER_TEMP"])
calls = sorted(root.glob("request-*.json"))
index = len(calls)
body = pathlib.Path(args[args.index("--data-binary") + 1][1:])
(root / f"request-{index}.json").write_bytes(body.read_bytes())
(root / f"timeout-{index}.txt").write_text(args[args.index("--max-time") + 1])
responses = json.loads(pathlib.Path(os.environ["FIXTURE_RESPONSES"]).read_text())
if index >= len(responses):
    sys.exit("TOO_MANY_REQUESTS")
response = responses[index]
pathlib.Path(args[args.index("--output") + 1]).write_text(
    response if isinstance(response, str) else json.dumps(response))
sys.stdout.write(os.environ["FIXTURE_HTTP_CODE"])
''')
        curl.chmod(0o755)
        output = root / "github-output"
        encode = lambda text: base64.b64encode(text.encode()).decode()
        env = {
            **os.environ, "PATH": f"{stub}:{os.environ['PATH']}",
            "RUNNER_TEMP": str(runtime), "GITHUB_OUTPUT": str(output),
            "BASE_SHA": base, "HEAD_SHA": head, "TRUSTED_SHA": base,
            "ARK_API_KEY": "fixture-not-a-secret", "ARK_MODEL": "fixture-request-model",
            "COMMAND": mode, "REQUEST_B64": encode("UNTRUSTED_REQUEST"),
            "CONTEXT_B64": encode('{"url":"fixture.invalid"}'),
            "HISTORY_B64": encode("UNTRUSTED_HISTORY") if mode == "recheck" else "",
            "FIXTURE_RESPONSES": str(fixtures), "FIXTURE_HTTP_CODE": http_code,
        }
        result = subprocess.run(["bash", "-c", workflow_script(workflow, step)],
                                cwd=repo, env=env, capture_output=True, text=True)
        requests = [json.loads(path.read_text()) for path in sorted(runtime.glob("request-*.json"))]
        published = output.read_text() if output.exists() else ""
        logs = result.stdout + result.stderr
        timeouts = [int(path.read_text()) for path in sorted(runtime.glob("timeout-*.txt"))]
        for timeout in timeouts:
            self.assertGreater(timeout, 0)
            self.assertLessEqual(timeout, 600)
        if len(timeouts) == 2:
            self.assertLessEqual(timeouts[1], timeouts[0])
        for private in ("PRIVATE_REASONING", "PRIVATE_PROMPT", "PRIVATE_ERROR",
                        "PRIVATE_INVALID_JSON", "UNTRUSTED_HELPER_EXECUTED",
                        "fixture-not-a-secret"):
            self.assertNotIn(private, logs)
        return result, requests, published

    def test_normal(self):
        for workflow, step, _ in WORKFLOWS:
            with self.subTest(workflow=workflow):
                result, requests, output = self.run_workflow(workflow, step, [NORMAL])
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(requests), 1)
                self.assertIn("Fixture review: no actionable findings.", output)
                self.assertIn("model_id_b64=" + base64.b64encode(b"fixture-model").decode(), output)

    def test_empty_and_malformed_fail_with_safe_reason_without_retry(self):
        for fixture, reason in (("empty.json", "empty_output"),
                                ("malformed.json", "malformed_json")):
            for workflow, step, _ in WORKFLOWS:
                with self.subTest(workflow=workflow, fixture=fixture):
                    result, requests, output = self.run_workflow(
                        workflow, step, [(FIXTURES / fixture).read_text()])
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(f'"reason_code":"{reason}"', result.stderr)
                    self.assertEqual(len(requests), 1)
                    self.assertEqual(output, "")

    def test_length_retries_once_with_less_context_and_preserves_policy(self):
        for workflow, step, _ in WORKFLOWS:
            modes = ("review", "recheck", "explain") if "command" in workflow else ("review",)
            for mode in modes:
                with self.subTest(workflow=workflow, mode=mode):
                    result, requests, output = self.run_workflow(workflow, step, [LENGTH, NORMAL], mode=mode)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(len(requests), 2)
                    self.assertIn('"reason_code":"output_token_limit"', result.stderr)
                    self.assertIn('"reasoning_tokens":8192', result.stderr)
                    self.assertLess(len(requests[1]["input"]), len(requests[0]["input"]))
                    for request in requests:
                        self.assertIn("TRUSTED_POLICY", request["input"])
                        self.assertIn("TRUSTED_REPOSITORY_POLICY", request["input"])
                        self.assertIn("+unchanged FEATURE_CHANGE", request["input"])
                        self.assertEqual(request["model"], "fixture-request-model")
                        self.assertEqual(request["max_output_tokens"], 8192)
                        self.assertEqual(request["reasoning"], {"effort": "medium"})
                        if "command" in workflow:
                            self.assertIn("UNTRUSTED_REQUEST", request["input"])
                            if mode == "recheck":
                                self.assertIn("UNTRUSTED_HISTORY", request["input"])
                    self.assertIn("unified=3", output)

    def test_repeated_length_failure_is_bounded_and_does_not_publish_partial_text(self):
        partial = {**LENGTH, "output": NORMAL["output"]}
        for workflow, step, _ in WORKFLOWS:
            with self.subTest(workflow=workflow):
                result, requests, output = self.run_workflow(workflow, step, [partial, partial])
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(len(requests), 2)
                self.assertEqual(output, "")
                self.assertIn("retry_exhausted", result.stderr)

    def test_non_length_incomplete_and_http_failures_do_not_retry(self):
        filtered = {**LENGTH, "incomplete_details": {"reason": "content_filter"}}
        for workflow, step, _ in WORKFLOWS:
            for response, code, reason in ((filtered, "200", "incomplete_response"),
                                           ({"error": {"message": "PRIVATE_ERROR"}}, "200", "unsuccessful_response"),
                                           ({"error": {"message": "PRIVATE_ERROR"}}, "429", "http_error")):
                with self.subTest(workflow=workflow, code=code):
                    result, requests, output = self.run_workflow(workflow, step, [response], http_code=code)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(len(requests), 1)
                    self.assertEqual(output, "")
                    self.assertIn(reason, result.stderr)

    def test_input_cap_applies_to_both_attempts(self):
        for workflow, step, _ in WORKFLOWS:
            with self.subTest(workflow=workflow):
                result, requests, _ = self.run_workflow(workflow, step, [LENGTH, NORMAL], long_input=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(requests), 2)
                for request in requests:
                    self.assertLess(len(request["input"].encode()), 750100)
                    self.assertIn("Review input truncated at 750000 bytes", request["input"])

    def test_untrusted_diagnostic_fields_and_invalid_shapes_cannot_leak_text(self):
        unsafe = "PRIVATE_PROMPT\n::error::PRIVATE_ERROR"
        responses = (
            {"status": unsafe, "incomplete_details": {"reason": unsafe},
             "output": [{"type": unsafe}],
             "usage": {"input_tokens": unsafe, "output_tokens": True, "total_tokens": -1}},
            {"status": "completed", "output": {"content": unsafe}},
            [unsafe],
            {"output": [{"type": "message", "content": [{"type": "output_text", "text": " \n\t"}]}]},
        )
        for workflow, step, _ in WORKFLOWS:
            for response in responses:
                with self.subTest(workflow=workflow, response=response):
                    result, requests, output = self.run_workflow(workflow, step, [response])
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(len(requests), 1)
                    self.assertEqual(output, "")
                    self.assertIn('"reason_code":', result.stderr)

    def test_model_fallback_and_failed_message_status(self):
        fallback = {**NORMAL, "model": None}
        failed = {**NORMAL, "output": [{"type": "message", "status": "incomplete",
                                        "content": [{"type": "output_text", "text": "PRIVATE_PROMPT"}]}]}
        for workflow, step, _ in WORKFLOWS:
            with self.subTest(workflow=workflow):
                result, _, output = self.run_workflow(workflow, step, [fallback])
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("model_id_b64=" + base64.b64encode(b"fixture-request-model").decode(), output)
                result, requests, output = self.run_workflow(workflow, step, [failed])
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(len(requests), 1)
                self.assertEqual(output, "")
                self.assertIn('"reason_code":"incomplete_message"', result.stderr)


if __name__ == "__main__":
    unittest.main()
