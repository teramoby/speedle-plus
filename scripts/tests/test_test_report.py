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


if __name__ == "__main__":
    unittest.main()
