# Adapted from Pavu a7b458e; modified for Speedle. Apache-2.0; see .github/codex/PAVU-LICENSE.txt.
"""Run both real workflow shell blocks with synthetic Git and stubbed HTTP only."""

import base64
import json
import os
from pathlib import Path
import re
import runpy
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
DEEPSEEK_NORMAL = json.loads((FIXTURES / "deepseek-normal.json").read_text())
DEEPSEEK_LENGTH = json.loads((FIXTURES / "deepseek-length.json").read_text())
DEEPSEEK_HTTP_ERROR = (FIXTURES / "deepseek-http-error.json").read_text()
DEEPSEEK_MALFORMED = (FIXTURES / "deepseek-malformed.json").read_text()
format_markdown = runpy.run_path(str(ROOT / HELPER))["format_markdown"]


class ReviewBotResponseTests(unittest.TestCase):
    def test_report_markdown_preserves_inner_code_and_machine_markers(self):
        report = ("## ReviewBot review\n\n**Verdict:** Comment only\n\n"
                  "### Findings\n\n#### [P2] Example finding\n`pkg/eval.go:12`\n\n"
                  "```go\nreturn false\n```\n\n"
                  "<!-- reviewbot-verdict:actionable-findings -->\n— **ReviewBot**")
        legacy = report.replace("## ReviewBot review", "## Ark ReviewBot review").replace(
            "— **ReviewBot**", "— **Ark ReviewBot**")
        for label in ("", "text", "markdown", "md"):
            with self.subTest(label=label):
                self.assertEqual(format_markdown(f"````{label}\n{legacy}\n````"), report)
        self.assertEqual(format_markdown(report), report)
        self.assertEqual(format_markdown(legacy), report)
        for code in ("```python\nprint('example')\n```", "```text\nExample only\n```",
                     "```text\n## ReviewBot review\nUnclosed report"):
            self.assertEqual(format_markdown(code), code)

    def test_both_workflows_emit_renderable_markdown_for_all_response_modes(self):
        for workflow, step, _ in WORKFLOWS:
            modes = ("review", "recheck", "explain") if "command" in workflow else ("review",)
            for mode in modes:
                title = {"review": "review", "recheck": "recheck", "explain": "explanation"}[mode]
                expected = f"## ReviewBot {title}\n\nUse `pkg/eval.go:12`.\n\n— **ReviewBot**"
                legacy = expected.replace("## ReviewBot", "## Ark ReviewBot").replace(
                    "— **ReviewBot**", "— **Ark ReviewBot**")
                response = {**NORMAL, "output": [{"type": "message", "content": [
                    {"type": "output_text", "text": f"```text\n{legacy}\n```"}]}]}
                with self.subTest(workflow=workflow, mode=mode):
                    result, requests, output = self.run_workflow(workflow, step, [response], mode=mode)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(len(requests), 1)
                    match = re.search(r"final_message<<([^\n]+)\n([\s\S]*?)\n\1\n", output)
                    self.assertIsNotNone(match)
                    self.assertEqual(match[2], expected)

    def run_workflow(self, workflow, step, responses, *, http_code="200", mode="review",
                     long_input=False, provider=None, deepseek_model="fixture-deepseek-model",
                     curl_exit=0):
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
urls = [a for a in args if a.startswith("https://")]
(root / f"url-{index}.txt").write_text(urls[0] if urls else "")
headers = [args[i + 1] for i, a in enumerate(args[:-1]) if a == "-H"]
auth_kind = "other"
if "Authorization: Bearer fixture-not-a-secret" in headers:
    auth_kind = "ark"
elif "Authorization: Bearer fixture-deepseek-not-a-secret" in headers:
    auth_kind = "deepseek"
(root / f"auth-{index}.txt").write_text(auth_kind + "|" + str("Content-Type: application/json" in headers))
curl_exit = int(os.environ.get("FIXTURE_CURL_EXIT", "0"))
if curl_exit:
    sys.stderr.write(f"curl: ({curl_exit}) synthetic transport error\\n")
    sys.exit(curl_exit)
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
            "DEEPSEEK_API_KEY": "fixture-deepseek-not-a-secret",
            "COMMAND": mode, "REQUEST_B64": encode("UNTRUSTED_REQUEST"),
            "CONTEXT_B64": encode('{"url":"fixture.invalid"}'),
            "HISTORY_B64": encode("UNTRUSTED_HISTORY") if mode == "recheck" else "",
            "FIXTURE_RESPONSES": str(fixtures), "FIXTURE_HTTP_CODE": http_code,
            "FIXTURE_CURL_EXIT": str(curl_exit),
        }
        env.pop("REVIEWBOT_PROVIDER", None)
        env.pop("DEEPSEEK_MODEL", None)
        if provider is not None:
            env["REVIEWBOT_PROVIDER"] = provider
        if deepseek_model is not None:
            env["DEEPSEEK_MODEL"] = deepseek_model
        result = subprocess.run(["bash", "-c", workflow_script(workflow, step)],
                                cwd=repo, env=env, capture_output=True, text=True)
        requests = [json.loads(path.read_text()) for path in sorted(runtime.glob("request-*.json"))]
        urls = [path.read_text() for path in sorted(runtime.glob("url-*.txt"))]
        auths = [path.read_text() for path in sorted(runtime.glob("auth-*.txt"))]
        published = output.read_text() if output.exists() else ""
        logs = result.stdout + result.stderr
        timeouts = [int(path.read_text()) for path in sorted(runtime.glob("timeout-*.txt"))]
        for timeout in timeouts:
            self.assertGreater(timeout, 0)
            self.assertLessEqual(timeout, 600)
        if len(timeouts) == 2:
            self.assertLessEqual(timeouts[1], timeouts[0])
        if provider == "deepseek":
            for url in urls:
                self.assertEqual(url, "https://api.deepseek.com/chat/completions")
            for auth in auths:
                self.assertEqual(auth, "deepseek|True")
        elif provider in (None, "", "ark"):
            for url in urls:
                self.assertEqual(url, "https://ark.cn-beijing.volces.com/api/plan/v3/responses")
            for auth in auths:
                self.assertEqual(auth, "ark|True")
        for private in ("PRIVATE_REASONING", "PRIVATE_PROMPT", "PRIVATE_ERROR",
                        "PRIVATE_INVALID_JSON", "UNTRUSTED_HELPER_EXECUTED",
                        "fixture-not-a-secret", "fixture-deepseek-not-a-secret"):
            self.assertNotIn(private, logs)
        self.assertNotIn("PRIVATE_REASONING", published)
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

    def test_deepseek_normal_review_ignores_reasoning_and_names_model(self):
        for workflow, step, _ in WORKFLOWS:
            with self.subTest(workflow=workflow, default_model=True):
                result, requests, output = self.run_workflow(
                    workflow, step, [DEEPSEEK_NORMAL], provider="deepseek", deepseek_model=None)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(requests), 1)
                req = requests[0]
                self.assertEqual(req["model"], "deepseek-flash")
                self.assertEqual(req["max_tokens"], 65536)
                self.assertLessEqual(req["max_tokens"], 393216)
                self.assertIs(req["stream"], False)
                self.assertEqual([m["role"] for m in req["messages"]], ["system", "user"])
                self.assertIn("TRUSTED_POLICY", req["messages"][0]["content"])
                self.assertIn("TRUSTED_REPOSITORY_POLICY", req["messages"][0]["content"])
                self.assertNotIn("+unchanged FEATURE_CHANGE", req["messages"][0]["content"])
                self.assertIn("+unchanged FEATURE_CHANGE", req["messages"][1]["content"])
                self.assertIn("Fixture review: no actionable findings.", output)
                self.assertIn("model_id_b64=" + base64.b64encode(b"deepseek-flash").decode(), output)

            modes = ("review", "recheck", "explain") if "command" in workflow else ("review",)
            for mode in modes:
                title = {"review": "review", "recheck": "recheck", "explain": "explanation"}[mode]
                verdict = "\n\n<!-- reviewbot-verdict:no-actionable-findings -->" if mode == "review" else ""
                expected = f"## ReviewBot {title}\n\nUse `pkg/eval.go:12`.{verdict}\n— **ReviewBot**"
                fenced = expected.replace("## ReviewBot", "## DeepSeek ReviewBot").replace(
                    "— **ReviewBot**", "— **DeepSeek ReviewBot**")
                response = {
                    **DEEPSEEK_NORMAL,
                    "model": None,
                    "choices": [{
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "reasoning_content": "PRIVATE_REASONING",
                            "content": f"```markdown\n{fenced}\n```",
                        },
                        "finish_reason": "stop",
                    }],
                }
                with self.subTest(workflow=workflow, mode=mode):
                    result, requests, output = self.run_workflow(
                        workflow, step, [response], mode=mode, provider="deepseek",
                        deepseek_model="deepseek-v4-pro")
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(requests[0]["model"], "deepseek-v4-pro")
                    match = re.search(r"final_message<<([^\n]+)\n([\s\S]*?)\n\1\n", output)
                    self.assertIsNotNone(match)
                    self.assertEqual(match[2], expected)
                    self.assertIn("model_id_b64=" + base64.b64encode(b"deepseek-v4-pro").decode(), output)
                    if workflow == "reviewbot.yml":
                        self.assertIn("safe_to_merge=true", output)

    def test_deepseek_length_retries_once_with_less_context_and_bounds_retries(self):
        for workflow, step, _ in WORKFLOWS:
            modes = ("review", "recheck", "explain") if "command" in workflow else ("review",)
            for mode in modes:
                with self.subTest(workflow=workflow, mode=mode):
                    result, requests, output = self.run_workflow(
                        workflow, step, [DEEPSEEK_LENGTH, DEEPSEEK_NORMAL],
                        mode=mode, provider="deepseek")
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(len(requests), 2)
                    self.assertIn('"reason_code":"output_token_limit"', result.stderr)
                    self.assertIn('"finish_reason":"length"', result.stderr)
                    self.assertIn('"reasoning_tokens":65536', result.stderr)
                    self.assertEqual(requests[0]["messages"][0], requests[1]["messages"][0])
                    self.assertLess(len(requests[1]["messages"][1]["content"]),
                                    len(requests[0]["messages"][1]["content"]))
                    for req in requests:
                        self.assertIn("TRUSTED_POLICY", req["messages"][0]["content"])
                        self.assertIn("TRUSTED_REPOSITORY_POLICY", req["messages"][0]["content"])
                        self.assertIn("+unchanged FEATURE_CHANGE", req["messages"][1]["content"])
                        self.assertEqual(req["model"], "fixture-deepseek-model")
                        if "command" in workflow:
                            self.assertIn("UNTRUSTED_REQUEST", req["messages"][1]["content"])
                            if mode == "recheck":
                                self.assertIn("UNTRUSTED_HISTORY", req["messages"][1]["content"])
                    self.assertIn("unified=3", output)
                    if workflow == "reviewbot.yml":
                        self.assertIn("safe_to_merge=false", output)

        partial = {
            **DEEPSEEK_LENGTH,
            "choices": [{
                "index": 0,
                "message": {
                    "role": "assistant",
                    "reasoning_content": "PRIVATE_REASONING",
                    "content": "Partial truncated review that must not publish.",
                },
                "finish_reason": "length",
            }],
        }
        for workflow, step, _ in WORKFLOWS:
            with self.subTest(workflow=workflow, repeated=True):
                result, requests, output = self.run_workflow(
                    workflow, step, [partial, partial], provider="deepseek")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(len(requests), 2)
                self.assertEqual(output, "")
                self.assertIn("retry_exhausted", result.stderr)

    def test_deepseek_http_and_transport_errors_fail_without_retry_or_leak(self):
        for workflow, step, _ in WORKFLOWS:
            for code in ("401", "429", "503"):
                with self.subTest(workflow=workflow, http_code=code):
                    result, requests, output = self.run_workflow(
                        workflow, step, [DEEPSEEK_HTTP_ERROR], http_code=code, provider="deepseek")
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(len(requests), 1)
                    self.assertEqual(output, "")
                    self.assertIn(f"ReviewBot reason_code=http_error HTTP {code}", result.stderr)
            for provider in ("ark", "deepseek"):
                for curl_exit in (7, 28):
                    with self.subTest(workflow=workflow, provider=provider, curl_exit=curl_exit):
                        result, requests, output = self.run_workflow(
                            workflow, step, [DEEPSEEK_NORMAL], provider=provider, curl_exit=curl_exit)
                        self.assertEqual(result.returncode, curl_exit)
                        self.assertEqual(len(requests), 1)
                        self.assertEqual(output, "")

    def test_deepseek_malformed_unusable_truncation_and_invalid_provider(self):
        unsafe = "PRIVATE_PROMPT\n::error::PRIVATE_ERROR"
        unusable = (
            (DEEPSEEK_MALFORMED, "malformed_json"),
            ({**DEEPSEEK_NORMAL, "choices": [{
                "index": 0, "finish_reason": "stop",
                "message": {"role": "assistant", "reasoning_content": "PRIVATE_REASONING", "content": " \n\t"},
            }]}, "empty_output"),
            ({**DEEPSEEK_NORMAL, "choices": [{
                "index": 0, "finish_reason": "stop",
                "message": {"role": "assistant", "reasoning_content": "PRIVATE_REASONING", "content": None},
            }]}, "empty_output"),
            ({**DEEPSEEK_NORMAL, "choices": [{
                "index": 0, "finish_reason": "content_filter",
                "message": {"role": "assistant", "content": unsafe},
            }]}, "incomplete_response"),
            ({**DEEPSEEK_NORMAL, "choices": [{
                "index": 0, "finish_reason": "insufficient_system_resource",
                "message": {"role": "assistant", "content": unsafe},
            }]}, "incomplete_response"),
            ({**DEEPSEEK_NORMAL, "choices": [{
                "index": 0, "finish_reason": unsafe,
                "message": {"role": "assistant", "content": unsafe},
            }], "usage": {"prompt_tokens": unsafe, "completion_tokens": True, "total_tokens": -1}},
             "unsuccessful_response"),
            ({"error": {"message": "PRIVATE_ERROR"}}, "unsuccessful_response"),
            ({"choices": []}, "invalid_response"),
        )
        for workflow, step, _ in WORKFLOWS:
            for response, reason in unusable:
                with self.subTest(workflow=workflow, reason=reason):
                    result, requests, output = self.run_workflow(
                        workflow, step, [response], provider="deepseek")
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(len(requests), 1)
                    self.assertEqual(output, "")
                    self.assertIn(f'"reason_code":"{reason}"', result.stderr)

            with self.subTest(workflow=workflow, long_input=True):
                result, requests, output = self.run_workflow(
                    workflow, step, [DEEPSEEK_LENGTH, DEEPSEEK_NORMAL],
                    long_input=True, provider="deepseek")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(len(requests), 2)
                for req in requests:
                    total_bytes = sum(len(m["content"].encode()) for m in req["messages"])
                    self.assertLess(total_bytes, 750100)
                    self.assertIn("TRUSTED_POLICY", req["messages"][0]["content"])
                    self.assertIn("Review input truncated at 750000 bytes", req["messages"][1]["content"])
                if workflow == "reviewbot.yml":
                    self.assertIn("safe_to_merge=false", output)

            with self.subTest(workflow=workflow, invalid_provider=True):
                result, requests, output = self.run_workflow(
                    workflow, step, [DEEPSEEK_NORMAL], provider="unsupported")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(len(requests), 0)
                self.assertEqual(output, "")
                self.assertIn("reason_code=invalid_provider", result.stderr)


if __name__ == "__main__":
    unittest.main()
