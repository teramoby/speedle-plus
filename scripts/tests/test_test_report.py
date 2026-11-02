"""Offline tests for scripts/ci/test_report.py. No Ark, SMTP or GitHub calls."""
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "ci"))
import test_report  # noqa: E402


def events(*items):
    return "\n".join(json.dumps(dict(zip(("Action", "Package", "Test", "Output"), item))) for item in items) + "\n"


class FakeGitHub:
    def __init__(self, issues=()):
        self.issues, self.calls = list(issues), []

    def open_issues(self):
        return self.issues

    def call(self, method, path, data=None):
        self.calls.append((method, path, data))


class ParseTest(unittest.TestCase):
    def write(self, text):
        directory = tempfile.mkdtemp()
        path = Path(directory) / "go.json"
        path.write_text(text)
        return path

    def test_counts_and_leaf_failures_with_output(self):
        path = self.write(events(
            ("run", "p", "TestA", None), ("output", "p", "TestA", "ok\n"), ("pass", "p", "TestA", None),
            ("output", "p", "TestB/sub", "boom: want 1 got 2\n"), ("fail", "p", "TestB/sub", None),
            ("fail", "p", "TestB", None), ("skip", "p", "TestC", None), ("fail", "p", None, None),
            "not json"))
        packages, failures = test_report.parse_go_json([path])
        self.assertEqual(packages["p"]["pass"], 1)
        self.assertEqual(packages["p"]["skip"], 1)
        self.assertEqual([f["test"] for f in failures], ["TestB/sub"])
        self.assertIn("want 1 got 2", failures[0]["output"])

    def test_package_failure_without_test_is_reported(self):
        path = self.write(events(("output", "q", None, "panic: proto already registered\n"), ("fail", "q", None, None)))
        _, failures = test_report.parse_go_json([path])
        self.assertEqual(failures, [{"package": "q", "test": "(package)", "output": "panic: proto already registered"}])

    def test_output_is_trimmed_to_the_last_lines(self):
        self.assertEqual(test_report.tail("\n".join(map(str, range(100))), 3), "97\n98\n99")


class SkipTest(unittest.TestCase):
    def streams(self, **texts):
        directory = Path(tempfile.mkdtemp())
        for name, text in texts.items():
            (directory / f"{name}.json").write_text(text)
        return sorted(str(p) for p in directory.glob("*.json"))

    def skip(self, package, test, reason):
        return [("output", package, test, f"    x_test.go:9: {reason}\n"),
                ("output", package, test, f"--- SKIP: {test} (0.00s)\n"), ("skip", package, test, None)]

    def test_sorts_skips_by_whether_they_ran_elsewhere(self):
        paths = self.streams(
            file=events(*self.skip("mongodb", "TestStore", "MongoDB not available"),
                        *self.skip("mysql", "TestConformance/Discover", "the store does not manage discover requests"),
                        *self.skip("cli", "TestSpctl", "needs the fixture spctl built by TestMain")),
            mongodb=events(("pass", "mongodb", "TestStore", None),
                           *self.skip("cli", "TestSpctl", "needs the fixture spctl built by TestMain")))
        skips = test_report.classify_skips(paths)
        self.assertEqual(skips["elsewhere"], 1)
        self.assertEqual(skips["not_applicable"], 1)
        # A test skipped in every pass is listed once, with its reason.
        self.assertEqual(skips["not_run"], [{"package": "cli", "test": "TestSpctl",
                                             "reason": "x_test.go:9: needs the fixture spctl built by TestMain"}])

    def test_a_store_test_skipped_in_its_own_pass_did_not_run(self):
        # The MongoDB pass lost its database: every pass skipped the test.
        paths = self.streams(file=events(*self.skip("mongodb", "TestStore", "MongoDB not available")),
                             mongodb=events(*self.skip("mongodb", "TestStore", "MongoDB not available")))
        skips = test_report.classify_skips(paths)
        self.assertEqual([s["test"] for s in skips["not_run"]], ["TestStore"])
        self.assertEqual(skips["elsewhere"], 0)

    def test_reports_show_the_classification(self):
        skips = {"elsewhere": 63, "not_applicable": 2, "not_run": []}
        packages = {"p": {"pass": 3, "fail": 0, "skip": 65, "package_failed": False}}
        _, text = test_report.summarize("nightly", packages, [], [], "", skips=skips)
        self.assertIn("Skipped: 0 not run in any pass; 63 skipped in one store's pass but run in another's; "
                      "2 not applicable", text)
        card = test_report.feishu_card("nightly", True, packages, [], None, "", "", skips)["elements"][0]["content"]
        self.assertIn("3 通过，0 失败，0 未运行", card)
        self.assertIn("跳过 65 项：63 项已在对应存储的轮次运行，2 项不适用于该存储", card)
        self.assertNotIn("⚠️", card)

        not_run = [{"package": "github.com/x/cli", "test": f"T{i}", "reason": "r"} for i in range(12)]
        skips = {"elsewhere": 0, "not_applicable": 0, "not_run": not_run}
        _, text = test_report.summarize("nightly", packages, [], [], "", skips=skips)
        self.assertIn("- github.com/x/cli T0: r", text)
        card = test_report.feishu_card("nightly", True, packages, [], None, "", "", skips)["elements"][0]["content"]
        self.assertIn("⚠️ **未运行的测试**（12）", card)
        self.assertIn("- cli T0：r", card)
        self.assertIn("另有 2 项", card)
        self.assertNotIn("T10", card)

    def test_main_classifies_across_sources(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "file-go.json").write_text(events(*self.skip("mongodb", "TestStore", "MongoDB not available")))
        (directory / "mongodb-go.json").write_text(events(("pass", "mongodb", "TestStore", None)))
        with mock.patch.object(test_report, "send_feishu", return_value=True) as send, mock.patch("builtins.print"):
            test_report.main(["--kind", "nightly", "--source", f"run:success:{directory}"], env={})
        self.assertIn("0 未运行", send.call_args[0][0]["elements"][0]["content"])


class SummaryTest(unittest.TestCase):
    def test_summary_marks_failed_checks(self):
        ok, text = test_report.summarize("nightly", {"p": {"pass": 2, "fail": 0, "skip": 0, "package_failed": False}},
                                         [], [{"name": "quick-start", "passed": False, "output": "x"}], "http://run")
        self.assertFalse(ok)
        self.assertIn("FAILED", text)
        self.assertIn("quick-start: FAILED", text)
        self.assertIn("http://run", text)

    def test_check_spec(self):
        log = Path(tempfile.mkdtemp()) / "log"
        log.write_text("line\n")
        self.assertEqual(test_report.parse_check(f"vuln=0:{log}"), {"name": "vuln", "passed": True, "output": "line"})
        self.assertFalse(test_report.parse_check("vuln=3:/missing")["passed"])
        with self.assertRaises(ValueError):
            test_report.parse_check("vuln=x")


class ArkTest(unittest.TestCase):
    def test_extracts_message_text_only(self):
        payload = {"status": "completed", "output": [
            {"type": "reasoning", "summary": []},
            {"type": "message", "content": [{"type": "output_text", "text": "全部通过"}]}]}
        self.assertEqual(test_report.extract_text(payload), "全部通过")
        self.assertIsNone(test_report.extract_text({"status": "incomplete", "output": []}))
        self.assertIsNone(test_report.extract_text([]))

    def test_request_uses_bearer_key_and_model(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = json.dumps(
            {"output": [{"type": "message", "content": [{"type": "output_text", "text": "ok"}]}]}).encode()
        with mock.patch("urllib.request.urlopen", return_value=response) as urlopen:
            self.assertEqual(test_report.ark_analyze("summary", "k", "m"), "ok")
        request = urlopen.call_args[0][0]
        self.assertEqual(request.get_header("Authorization"), "Bearer k")
        self.assertEqual(json.loads(request.data)["model"], "m")

    def test_unreachable_ark_returns_none(self):
        with mock.patch("urllib.request.urlopen", side_effect=OSError("down")):
            self.assertIsNone(test_report.ark_analyze("summary", "k", "m"))


class MailTest(unittest.TestCase):
    def test_skips_without_configuration(self):
        self.assertFalse(test_report.send_mail("s", "b", {}))

    def test_sends_over_ssl_with_login(self):
        with mock.patch("smtplib.SMTP_SSL") as smtp:
            self.assertTrue(test_report.send_mail("s", "body", {
                "SMTP_SERVER": "smtp.example", "SMTP_USERNAME": "u", "SMTP_PASSWORD": "p", "MAIL_TO": "caiwl@acm.org"}))
        server = smtp.return_value
        server.login.assert_called_once_with("u", "p")
        message = server.send_message.call_args[0][0]
        self.assertEqual(message["To"], "caiwl@acm.org")
        self.assertEqual(message["Subject"], "s")

    def test_starttls_on_other_ports(self):
        with mock.patch("smtplib.SMTP") as smtp:
            test_report.send_mail("s", "b", {"SMTP_SERVER": "h", "SMTP_PORT": "587", "MAIL_TO": "t"})
        smtp.return_value.starttls.assert_called_once()


class IssueTest(unittest.TestCase):
    failure = {"package": "p", "test": "TestB", "output": "boom"}

    def test_opens_a_labelled_issue_for_a_new_failure(self):
        github = FakeGitHub()
        self.assertEqual(test_report.track_failures(github, "nightly", [self.failure], "u", "分析"), (1, 0))
        method, path, data = github.calls[0]
        self.assertEqual((method, path), ("POST", "/issues"))
        self.assertEqual(data["title"], "Test failure: p TestB")
        self.assertEqual(data["labels"], ["test-failure"])
        self.assertIn("分析", data["body"])

    def test_comments_instead_of_duplicating(self):
        github = FakeGitHub([{"title": "Test failure: p TestB", "number": 7}])
        self.assertEqual(test_report.track_failures(github, "nightly", [self.failure], "u", None), (0, 1))
        self.assertEqual(github.calls[0][1], "/issues/7/comments")

    def test_caps_new_issues_and_groups_the_rest(self):
        github = FakeGitHub()
        failures = [{"package": "p", "test": f"T{i}", "output": ""} for i in range(25)]
        self.assertEqual(test_report.track_failures(github, "weekly", failures, "u", None), (21, 0))
        self.assertIn("5 more failing tests", github.calls[-1][2]["title"])


class MainTest(unittest.TestCase):
    def test_end_to_end_without_external_services(self):
        directory = Path(tempfile.mkdtemp())
        results = directory / "go.json"
        results.write_text(events(("fail", "p", "TestX", None)))
        summary = directory / "report.txt"
        github = FakeGitHub()
        with mock.patch.object(test_report, "GitHub", return_value=github), mock.patch("builtins.print"):
            code = test_report.main(["--kind", "nightly", "--go-json", str(results), "--go-json", str(directory / "lost.json"),
                                     "--check", "vuln=1:/missing", "--summary-file", str(summary)],
                                    env={"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r", "RUN_URL": "http://run"})
        self.assertEqual(code, 0)
        self.assertIn("Ark analysis unavailable", summary.read_text())
        titles = [c[2]["title"] for c in github.calls]
        self.assertIn("Test failure: p TestX", titles)
        self.assertIn("Test failure: check vuln", titles)
        self.assertIn("results lost.json", summary.read_text())


class ResultsDirTest(unittest.TestCase):
    def test_reads_streams_and_checks_relative_to_the_directory(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "go-file.json").write_text(events(("fail", "p", "TestY", None)))
        (directory / "quick.log").write_text("step failed\n")
        (directory / "checks.txt").write_text("quick=1:quick.log\ngovulncheck=0:govulncheck.log\n")
        summary = directory / "out.txt"
        with mock.patch("builtins.print"):
            test_report.main(["--kind", "weekly", "--results-dir", str(directory),
                              "--results-dir", str(directory / "absent"), "--summary-file", str(summary)], env={})
        text = summary.read_text()
        self.assertIn("p TestY", text)
        self.assertIn("quick: FAILED", text)
        self.assertIn("step failed", text)
        self.assertIn("govulncheck: passed", text)
        self.assertIn("results absent: FAILED", text)


class SourceTest(unittest.TestCase):
    def report(self, *sources):
        summary = Path(tempfile.mkdtemp()) / "out.txt"
        with mock.patch("builtins.print"):
            test_report.main(["--kind", "nightly", *[a for s in sources for a in ("--source", s)],
                              "--summary-file", str(summary)], env={})
        return summary.read_text()

    def results(self, *checks, stream=None):
        directory = Path(tempfile.mkdtemp())
        (directory / "checks.txt").write_text("".join(f"{c}\n" for c in checks))
        if stream is not None:
            (directory / "go-file.json").write_text(stream)
        return directory

    def test_a_cancelled_job_with_only_passing_partial_results_fails(self):
        # The worker wrote one passing check, then was cancelled mid-suite.
        directory = self.results("quick=0:quick.log", stream=events(("pass", "p", "TestA", None)))
        text = self.report(f"run:cancelled:{directory}")
        self.assertIn("FAILED", text.splitlines()[2])
        self.assertIn("run job: FAILED", text)
        self.assertIn("ended with cancelled", text)

    def test_a_failed_job_with_failing_results_adds_no_extra_failure(self):
        directory = self.results("quick=1:quick.log", stream=events(("fail", "p", "TestB", None)))
        text = self.report(f"run:failure:{directory}")
        self.assertIn("quick: FAILED", text)
        self.assertIn("p TestB", text)
        self.assertNotIn("run job", text)

    def test_a_successful_job_and_a_failed_file_source(self):
        stream = Path(tempfile.mkdtemp()) / "go-mongodb.json"
        stream.write_text(events(("pass", "m", "TestM", None)))
        text = self.report(f"run:success:{self.results('quick=0:quick.log')}", f"mongodb:timed_out:{stream}")
        self.assertIn("mongodb job: FAILED", text)
        self.assertNotIn("run job", text)
        self.assertIn("1 passed", text)

    def test_missing_results_of_a_failed_job_are_reported_once(self):
        text = self.report("run:failure:/nonexistent/test-results", "mongodb:failure:/nonexistent/go-mongodb.json")
        self.assertIn("results test-results: FAILED", text)
        self.assertIn("results go-mongodb.json: FAILED", text)
        self.assertNotIn(" job: FAILED", text)


class DedupeTest(unittest.TestCase):
    def test_a_failure_in_two_streams_opens_one_issue(self):
        github = FakeGitHub()
        failures = [{"package": "p", "test": "TestX", "output": "file store"},
                    {"package": "p", "test": "TestX", "output": "etcd store"},
                    {"package": "p", "test": "TestX", "output": "file store"},
                    {"package": "p", "test": "TestY", "output": ""}]
        created, commented = test_report.track_failures(github, "nightly", failures, "http://run", None)
        issues = [c[2] for c in github.calls if c[1] == "/issues"]
        self.assertEqual((created, commented), (2, 0))
        self.assertEqual(sorted(i["title"] for i in issues), ["Test failure: p TestX", "Test failure: p TestY"])
        body = next(i["body"] for i in issues if i["title"].endswith("TestX"))
        self.assertIn("file store", body)
        self.assertIn("etcd store", body)
        self.assertEqual(body.count("file store"), 1)

    def test_feishu_lists_each_failure_once(self):
        directory = Path(tempfile.mkdtemp())
        for name in ("go-file.json", "go-etcd.json"):
            (directory / name).write_text(events(("fail", "p", "TestZ", None)))
        with mock.patch.object(test_report, "send_feishu", return_value=True) as send, mock.patch("builtins.print"):
            test_report.main(["--kind", "nightly", "--results-dir", str(directory)], env={})
        self.assertEqual(send.call_args[0][0]["elements"][0]["content"].count("- p TestZ"), 1)


class FeishuTest(unittest.TestCase):
    def card(self, ok=False, failed=("p TestX",), analysis="结论：失败"):
        packages = {"p": {"pass": 3, "fail": 1, "skip": 2, "package_failed": False}}
        return test_report.feishu_card("nightly", ok, packages, list(failed), analysis,
                                       "http://run", "http://issues")

    def test_signature_matches_feishu_algorithm(self):
        import base64, hashlib, hmac
        expected = base64.b64encode(hmac.new(b"1700000000\nsecret", b"", hashlib.sha256).digest()).decode()
        self.assertEqual(test_report.feishu_sign("secret", "1700000000"), expected)

    def test_card_content(self):
        card = self.card()
        self.assertEqual(card["header"]["template"], "red")
        self.assertIn("失败", card["header"]["title"]["content"])
        text = card["elements"][0]["content"]
        for part in ("3 通过，1 失败，2 跳过", "- p TestX", "结论：失败", "[运行详情](http://run)", "[失败跟踪 issue](http://issues)"):
            self.assertIn(part, text)
        passed = self.card(ok=True, failed=(), analysis=None)
        self.assertEqual(passed["header"]["template"], "green")
        self.assertIn("Ark 分析不可用", passed["elements"][0]["content"])
        self.assertNotIn("issue", passed["elements"][0]["content"])

    def test_card_bounds_long_lists_and_analysis(self):
        card = self.card(failed=[f"p Test{i}" for i in range(15)], analysis="x" * 5000)
        text = card["elements"][0]["content"]
        self.assertIn("另有 5 项", text)
        self.assertNotIn("p Test10", text)
        self.assertLess(len(text), 2500)

    def send(self, env, reply=b'{"code":0,"msg":"success"}', error=None):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = reply
        with mock.patch("urllib.request.urlopen", return_value=response, side_effect=error) as urlopen, \
                mock.patch("builtins.print"):
            result = test_report.send_feishu(self.card(), env, now=lambda: 1700000000.5)
        return result, urlopen

    def test_signed_message(self):
        ok, urlopen = self.send({"FEISHU_WEBHOOK": "https://open.feishu.cn/hook/x", "FEISHU_WEBHOOK_SECRET": "secret"})
        self.assertTrue(ok)
        request = urlopen.call_args[0][0]
        body = json.loads(request.data)
        self.assertEqual(request.full_url, "https://open.feishu.cn/hook/x")
        self.assertEqual(body["msg_type"], "interactive")
        self.assertEqual(body["timestamp"], "1700000000")
        self.assertEqual(body["sign"], test_report.feishu_sign("secret", "1700000000"))

    def test_unsigned_message_and_failures(self):
        ok, urlopen = self.send({"FEISHU_WEBHOOK": "https://hook"})
        self.assertTrue(ok)
        self.assertNotIn("sign", json.loads(urlopen.call_args[0][0].data))
        self.assertFalse(self.send({"FEISHU_WEBHOOK": "https://hook"}, reply=b'{"code":19021,"msg":"sign match fail"}')[0])
        self.assertFalse(self.send({"FEISHU_WEBHOOK": "https://hook"}, error=OSError("down"))[0])
        ok, urlopen = self.send({})
        self.assertFalse(ok)
        urlopen.assert_not_called()

    def test_main_sends_the_card(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        with mock.patch.object(test_report, "send_feishu", return_value=True) as send, mock.patch("builtins.print"):
            test_report.main(["--kind", "weekly", "--go-json", str(directory / "go.json")],
                             env={"GITHUB_REPOSITORY": "o/r", "RUN_URL": "http://run"})
        card = send.call_args[0][0]
        self.assertEqual(card["header"]["template"], "red")
        self.assertIn("p TestZ", card["elements"][0]["content"])
        self.assertIn("https://github.com/o/r/issues?q=is%3Aopen+label%3Atest-failure", card["elements"][0]["content"])


class RegressionTest(unittest.TestCase):
    """The post-merge regression: quiet on success, English, issues and Feishu, no mail."""
    env = {"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r", "RUN_URL": "http://run",
           "COMMIT_URL": "http://commit/abc", "ARK_API_KEY": "k"}

    def run_main(self, text, issues=()):
        directory = Path(tempfile.mkdtemp())
        (directory / "go-file.json").write_text(text)
        summary = directory / "report.txt"
        github = FakeGitHub(issues)
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch.object(test_report, "ark_analyze", return_value="Likely cause: nil map.") as ark, \
                mock.patch.object(test_report, "send_feishu", return_value=True) as feishu, \
                mock.patch.object(test_report, "send_mail") as mail, mock.patch("builtins.print"):
            code = test_report.main(["--kind", "regression", "--source", f"run:success:{directory}",
                                     "--summary-file", str(summary)], env=self.env)
        self.assertEqual(code, 0)
        return github, ark, feishu, mail, summary.read_text()

    def test_a_passing_run_sends_nothing(self):
        github, ark, feishu, mail, summary = self.run_main(events(("pass", "p", "TestA", None)))
        ark.assert_not_called()
        feishu.assert_not_called()
        mail.assert_not_called()
        self.assertEqual(github.calls, [])
        self.assertIn("post-merge regression test report: PASSED", summary)
        self.assertIn("Commit: http://commit/abc", summary)

    def test_a_failure_opens_an_issue_with_output_and_english_analysis(self):
        github, ark, feishu, mail, summary = self.run_main(events(
            ("output", "p", "TestB", "panic: assignment to entry in nil map\n"), ("fail", "p", "TestB", None)))
        self.assertEqual(ark.call_args.kwargs["language"], "English")
        self.assertIn("Commit: http://commit/abc", ark.call_args[0][0])
        mail.assert_not_called()
        self.assertIn("post-merge regression", feishu.call_args[0][0]["header"]["title"]["content"])
        (method, path, data), = github.calls
        self.assertEqual((method, path, data["title"]), ("POST", "/issues", "Test failure: p TestB"))
        for part in ("post-merge regression run failed `TestB`", "Commit: http://commit/abc",
                     "Run: http://run", "assignment to entry in nil map", "Likely cause: nil map."):
            self.assertIn(part, data["body"])
        self.assertTrue(summary.startswith("Likely cause: nil map."))

    def test_a_known_failure_gets_a_comment_with_the_analysis(self):
        github, *_ = self.run_main(events(("fail", "p", "TestB", None)),
                                   issues=[{"title": "Test failure: p TestB", "number": 9}])
        (method, path, data), = github.calls
        self.assertEqual(path, "/issues/9/comments")
        self.assertIn("Likely cause: nil map.", data["body"])
        self.assertIn("Commit: http://commit/abc", data["body"])

    def test_a_failed_job_without_results_is_reported(self):
        directory = Path(tempfile.mkdtemp())
        github = FakeGitHub()
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch.object(test_report, "send_feishu", return_value=True), mock.patch("builtins.print"):
            test_report.main(["--kind", "regression", "--source", f"run:failure:{directory / 'missing'}"],
                             env={"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r"})
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: check results missing"])

    def test_scheduled_runs_keep_chinese_analysis(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b"{}"
        with mock.patch("urllib.request.urlopen", return_value=response) as urlopen:
            test_report.ark_analyze("s", "k", "m")
            test_report.ark_analyze("s", "k", "m", language="English")
        prompts = [json.loads(c[0][0].data)["input"] for c in urlopen.call_args_list]
        self.assertIn("Write in Chinese", prompts[0])
        self.assertIn("Write in English", prompts[1])


class PerformanceTest(unittest.TestCase):
    def test_the_benchmark_table_is_in_the_report_and_its_json_is_not_a_test_stream(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "go-file.json").write_text(events(("pass", "p", "TestA", None)))
        (directory / "performance.json").write_text('{\n  "regressed": []\n}\n')
        (directory / "performance.md").write_text("| BenchmarkOne | 100 | 101 | +1.0% |\n\nNo regression.\n")
        (directory / "checks.txt").write_text("performance=0:performance.log\n")
        streams, _ = test_report.expand_results_dir(directory)
        self.assertEqual([Path(s).name for s in streams], ["go-file.json"])
        summary = directory / "report.txt"
        with mock.patch.object(test_report, "send_feishu", return_value=True), mock.patch("builtins.print"):
            test_report.main(["--kind", "weekly", "--source", f"run:success:{directory}",
                              "--summary-file", str(summary)], env={})
        text = summary.read_text()
        self.assertIn("weekly test report: PASSED", text)
        self.assertIn("## Performance", text)
        self.assertIn("| BenchmarkOne | 100 | 101 | +1.0% |", text)

    def test_no_section_without_a_table(self):
        self.assertEqual(test_report.performance_section([tempfile.mkdtemp(), "/missing"]), "")


if __name__ == "__main__":
    unittest.main()
