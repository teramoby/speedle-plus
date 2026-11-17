"""Offline tests for scripts/ci/test_report.py. No Ark, email or GitHub calls."""
import base64
import http.client
import io
import json
import sys
import tempfile
import unittest
import urllib.error
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

        not_run = [{"package": "github.com/x/cli", "test": f"T{i}", "reason": "r"} for i in range(12)]
        skips = {"elsewhere": 0, "not_applicable": 0, "not_run": not_run}
        _, text = test_report.summarize("nightly", packages, [], [], "", skips=skips)
        self.assertIn("- github.com/x/cli T0: r", text)
        self.assertIn("- github.com/x/cli T11: r", text)

    def test_main_classifies_across_sources(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "file-go.json").write_text(events(*self.skip("mongodb", "TestStore", "MongoDB not available")))
        (directory / "mongodb-go.json").write_text(events(("pass", "mongodb", "TestStore", None)))
        with mock.patch.object(test_report, "send_email", return_value=True) as send, mock.patch("builtins.print"):
            test_report.main(["--kind", "nightly", "--source", f"run:success:{directory}"], env={})
        self.assertIn("0 not run in any pass", send.call_args[0][1])


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
        with mock.patch("test_report.open_no_redirect", return_value=response) as urlopen:
            self.assertEqual(test_report.ark_analyze("summary", "k", "m"), "ok")
        request = urlopen.call_args[0][0]
        self.assertEqual(request.get_header("Authorization"), "Bearer k")
        self.assertEqual(json.loads(request.data)["model"], "m")

    def test_unreachable_ark_returns_none(self):
        with mock.patch("test_report.open_no_redirect", side_effect=OSError("down")):
            self.assertIsNone(test_report.ark_analyze("summary", "k", "m"))


class EmailTest(unittest.TestCase):
    env = {"CF_ACCOUNT_ID": "account-id", "CF_EMAIL_TOKEN": "api-token",
           "OPS_EMAIL_FROM": "sender@example.com", "OPS_EMAIL_TO": "recipient@example.com",
           "OPS_EMAIL_REPLY_TO": "reply@example.com"}

    def send(self, result=None, success=True, reply=None, error=None, env=None, attachment=None):
        if reply is None:
            reply = json.dumps({"success": success, "result":
                                {"queued": [self.env["OPS_EMAIL_TO"]]} if result is None else result}).encode()
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = reply
        with mock.patch("test_report.open_no_redirect", return_value=response, side_effect=error) as urlopen, \
                mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            ok = test_report.send_email("subject", "body", env or self.env, attachment=attachment)
        return ok, urlopen, stderr.getvalue()

    def test_posts_the_report_with_the_configured_settings(self):
        ok, urlopen, _ = self.send()
        self.assertTrue(ok)
        request = urlopen.call_args[0][0]
        self.assertEqual(request.full_url,
                         "https://api.cloudflare.com/client/v4/accounts/account-id/email/sending/send")
        self.assertEqual(request.get_method(), "POST")
        self.assertEqual(request.get_header("Authorization"), "Bearer api-token")
        self.assertEqual(json.loads(request.data), {"to": ["recipient@example.com"],
                                                    "from": "sender@example.com",
                                                    "subject": "subject", "text": "body",
                                                    "reply_to": "reply@example.com"})
        self.assertNotIn("attachments", json.loads(request.data))
        self.assertEqual(urlopen.call_args.kwargs["timeout"], test_report.EMAIL_TIMEOUT)

    def test_the_send_refuses_redirects(self):
        # urlopen forwards the Authorization header to a redirect target, so
        # the email opener must not follow one.
        self.assertIsNone(test_report._NoRedirect().redirect_request(
            mock.Mock(), None, 302, "Found", {}, "https://elsewhere.example/send"))
        with mock.patch("urllib.request.build_opener") as build:
            test_report.open_no_redirect("request", timeout=5)
        self.assertEqual(build.call_args[0], (test_report._NoRedirect,))
        build.return_value.open.assert_called_once_with("request", timeout=5)

    def test_a_redirect_is_a_failure(self):
        error = urllib.error.HTTPError("https://api.example/send", 302, "Found", None, io.BytesIO(b"{}"))
        ok, urlopen, message = self.send(error=error)
        self.assertFalse(ok)
        self.assertEqual(urlopen.call_count, 1)
        self.assertIn("HTTP 302", message)

    def test_an_attachment_is_sent_as_base64_text(self):
        report = "断言失败\n" * 2000
        attachment = test_report.report_attachment("speedle-nightly-report.txt", report)
        ok, urlopen, _ = self.send(attachment=attachment)
        self.assertTrue(ok)
        [sent] = json.loads(urlopen.call_args[0][0].data)["attachments"]
        self.assertEqual(sent["filename"], "speedle-nightly-report.txt")
        self.assertEqual(sent["type"], "text/plain")
        self.assertEqual(sent["disposition"], "attachment")
        self.assertEqual(base64.b64decode(sent["content"]).decode("utf-8"), report)

    def test_an_attachment_beyond_its_cap_is_cut_between_characters(self):
        limit = 200
        report = "断言失败\n" * 100
        with mock.patch.object(test_report, "ATTACHMENT_TEXT_LIMIT", limit):
            attachment = test_report.report_attachment("report.txt", report)
        text = base64.b64decode(attachment["content"]).decode("utf-8")
        self.assertLessEqual(len(text.encode("utf-8")), limit)
        self.assertTrue(text.startswith("断言失败"))
        self.assertIn("truncated at 200 bytes in the attachment", text)

    def test_the_attachment_cap_fits_the_message_limit(self):
        # Base64 of the capped text plus a body whose every character is
        # escaped as a surrogate pair (12 bytes) stays under 25 MiB.
        encoded = 4 * ((test_report.ATTACHMENT_TEXT_LIMIT + 2) // 3)
        escaped_body = 12 * test_report.EMAIL_TEXT_LIMIT
        self.assertLess(encoded + escaped_body, test_report.EMAIL_MAX_BYTES)

    def test_a_message_over_the_api_limit_is_not_sent(self):
        with mock.patch.object(test_report, "EMAIL_MAX_BYTES", 1024 * 1024), \
                mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            self.assertFalse(test_report.send_email("subject", "x" * (1024 * 1024 + 1), self.env))
        urlopen.assert_not_called()
        self.assertIn("::error::", stderr.getvalue())
        self.assertIn("1 MiB API limit", stderr.getvalue())

    def test_sends_normalized_addresses(self):
        env = dict(self.env, OPS_EMAIL_FROM="Sender <sender@example.com>",
                   OPS_EMAIL_TO="Ops <recipient@example.com>, second@example.com",
                   OPS_EMAIL_REPLY_TO="Reply <reply@example.com>")
        ok, urlopen, _ = self.send(result={"queued": ["recipient@example.com", "second@example.com"]}, env=env)
        self.assertTrue(ok)
        self.assertEqual(json.loads(urlopen.call_args[0][0].data),
                         {"to": ["recipient@example.com", "second@example.com"],
                          "from": "sender@example.com", "subject": "subject", "text": "body",
                          "reply_to": "reply@example.com"})

    def test_invalid_addresses_fail_without_a_request(self):
        for setting in ("OPS_EMAIL_FROM", "OPS_EMAIL_TO", "OPS_EMAIL_REPLY_TO"):
            with self.subTest(setting=setting):
                with mock.patch("test_report.open_no_redirect") as urlopen, \
                        mock.patch("sys.stderr", new=io.StringIO()) as stderr:
                    self.assertFalse(test_report.send_email("s", "b", dict(self.env, **{setting: "not an address"})))
                urlopen.assert_not_called()
                self.assertIn("not configured", stderr.getvalue())
                self.assertNotIn("::error::", stderr.getvalue())

    def test_a_configured_recipient_that_is_not_an_address_is_not_dropped(self):
        env = dict(self.env, OPS_EMAIL_TO="recipient@example.com, not an address")
        with mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            self.assertFalse(test_report.send_email("s", "b", env))
        urlopen.assert_not_called()
        self.assertNotIn("::error::", stderr.getvalue())

    def test_a_token_the_parser_swallows_is_not_dropped(self):
        # These values leave one good address and a token the parser turns
        # into an empty entry; sending only to the good one would silently
        # skip a configured operator, so the whole setting is rejected.
        for recipients in ("recipient@example.com, b@", "recipient@example.com; second@example.com",
                           "recipient@example.com, undisclosed-recipients:;"):
            with self.subTest(recipients=recipients):
                with mock.patch("test_report.open_no_redirect") as urlopen, \
                        mock.patch("sys.stderr", new=io.StringIO()) as stderr:
                    self.assertFalse(test_report.send_email(
                        "s", "b", dict(self.env, OPS_EMAIL_TO=recipients)))
                urlopen.assert_not_called()
                self.assertNotIn("::error::", stderr.getvalue())

    def test_rejects_more_recipients_than_the_api_accepts(self):
        env = dict(self.env, OPS_EMAIL_TO=", ".join(f"r{i}@example.com" for i in range(51)))
        with mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            self.assertFalse(test_report.send_email("s", "b", env))
        urlopen.assert_not_called()
        self.assertNotIn("::error::", stderr.getvalue())

    def test_a_missing_reply_to_sends_nothing(self):
        # The sender is outbound-only, so an email without Reply-To loses replies.
        env = {key: value for key, value in self.env.items() if key != "OPS_EMAIL_REPLY_TO"}
        with mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            self.assertFalse(test_report.send_email("s", "b", env))
        urlopen.assert_not_called()
        self.assertIn("not configured", stderr.getvalue())
        self.assertNotIn("::error::", stderr.getvalue())

    def test_the_account_id_is_percent_encoded(self):
        ok, urlopen, _ = self.send(env=dict(self.env, CF_ACCOUNT_ID="a/b?c"))
        self.assertTrue(ok)
        self.assertEqual(urlopen.call_args[0][0].full_url,
                         "https://api.cloudflare.com/client/v4/accounts/a%2Fb%3Fc/email/sending/send")

    def test_a_transient_failure_is_retried(self):
        transient = urllib.error.HTTPError("https://api.example/send", 503, "unavailable", None, io.BytesIO(b"{}"))
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = json.dumps(
            {"success": True, "result": {"queued": ["recipient@example.com"]}}).encode()
        with mock.patch.object(transient, "close") as close, \
                mock.patch("test_report.open_no_redirect", side_effect=[transient, response]) as urlopen, \
                mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                mock.patch("sys.stderr", new=io.StringIO()):
            self.assertTrue(test_report.send_email("s", "b", self.env))
        self.assertEqual(urlopen.call_count, 2)
        close.assert_called_once_with()

    def test_a_persistent_failure_stops_after_the_retries(self):
        error = urllib.error.HTTPError("https://api.example/send", 500, "server error", None, io.BytesIO(b"{}"))
        with mock.patch("test_report.open_no_redirect", side_effect=error) as urlopen, \
                mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            self.assertFalse(test_report.send_email("s", "b", self.env))
        self.assertEqual(urlopen.call_count, test_report.EMAIL_RETRIES + 1)
        self.assertIn("HTTP 500", stderr.getvalue())

    def test_a_rejected_request_is_not_retried(self):
        error = urllib.error.HTTPError("https://api.example/send", 403, "denied", None, io.BytesIO(b"{}"))
        with mock.patch("test_report.open_no_redirect", side_effect=error) as urlopen, \
                mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                mock.patch("sys.stderr", new=io.StringIO()):
            self.assertFalse(test_report.send_email("s", "b", self.env))
        self.assertEqual(urlopen.call_count, 1)

    def test_a_queued_or_delivered_recipient_is_success(self):
        self.assertTrue(self.send(result={"queued": ["recipient@example.com"]})[0])
        self.assertTrue(self.send(result={"delivered": ["recipient@example.com"]})[0])

    def test_result_addresses_accept_strings_and_address_objects(self):
        self.assertEqual(test_report.addresses(
            ["A@Example.com", {"address": "B@Example.com", "name": "Operators"}, "not an address"]),
            ["a@example.com", "b@example.com"])
        self.assertEqual(test_report.addresses(None), [])
        self.assertEqual(test_report.addresses("recipient@example.com"), [])

    def test_matches_recipients_case_insensitively_and_through_address_objects(self):
        self.assertTrue(self.send(result={"delivered": [
            {"address": "RECIPIENT@example.com", "name": "Operators"}]})[0])

    def test_case_is_preserved_on_the_wire_and_ignored_for_confirmation(self):
        env = dict(self.env, OPS_EMAIL_FROM="Sender@Example.com", OPS_EMAIL_TO="Ops@Example.com")
        ok, urlopen, _ = self.send(result={"queued": ["ops@example.com"]}, env=env)
        self.assertTrue(ok)
        payload = json.loads(urlopen.call_args[0][0].data)
        self.assertEqual(payload["to"], ["Ops@Example.com"])
        self.assertEqual(payload["from"], "Sender@Example.com")

    def test_a_large_report_is_truncated_for_the_email(self):
        directory = Path(tempfile.mkdtemp())
        stream = directory / "go.json"
        stream.write_text(events(*[
            item for i in range(1000)
            for item in (("output", "p", f"T{i}", "x" * 200 + "\n"), ("fail", "p", f"T{i}", None))]))
        summary = directory / "report.txt"
        with mock.patch.object(test_report, "send_email", return_value=True) as send, \
                mock.patch("builtins.print"):
            test_report.main(["--kind", "weekly", "--go-json", str(stream),
                              "--summary-file", str(summary)], env=dict(self.env, RUN_URL="http://run"))
        body = send.call_args[0][1]
        self.assertLessEqual(len(body), test_report.EMAIL_TEXT_LIMIT)
        self.assertIn("[report truncated", body)
        self.assertIn("the full report is attached", body)
        self.assertIn("the workflow run (http://run)", body)
        full = summary.read_text()
        self.assertGreater(len(full), test_report.EMAIL_TEXT_LIMIT)
        self.assertIn("x" * 200, full)
        # The attachment is what makes the email self-contained: it carries
        # the complete report, not the body's prefix.
        attachment = send.call_args.kwargs["attachment"]
        self.assertEqual(attachment["filename"], "speedle-weekly-report.txt")
        attached = base64.b64decode(attachment["content"]).decode("utf-8")
        self.assertEqual(attached, full)
        self.assertNotIn("[report truncated", attached)

    def test_the_summary_file_is_written_as_utf8(self):
        # The step-summary pipeline declares UTF-8; the report file must not
        # depend on the ambient locale to produce it.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(
            ("output", "p", "TestZ", "断言失败\n"), ("fail", "p", "TestZ", None)))
        summary = directory / "report.txt"
        with mock.patch.object(test_report, "send_email", return_value=True), \
                mock.patch("sys.stdout", new=io.StringIO()):
            test_report.main(["--kind", "weekly", "--go-json", str(directory / "go.json"),
                              "--summary-file", str(summary)], env={})
        self.assertIn("断言失败", summary.read_bytes().decode("utf-8"))

    def test_every_configured_recipient_must_be_confirmed(self):
        env = dict(self.env, OPS_EMAIL_TO="Recipient One <recipient@example.com>, second@example.com")
        ok, _, message = self.send(result={"queued": ["recipient@example.com"]}, env=env)
        self.assertFalse(ok)
        self.assertIn("every recipient", message)
        ok, _, _ = self.send(result={"queued": ["recipient@example.com"],
                                     "delivered": ["SECOND@example.com"]}, env=env)
        self.assertTrue(ok)

    def test_a_permanent_bounce_is_a_failure(self):
        ok, _, warning = self.send(result={"permanent_bounces": ["recipient@example.com"]})
        self.assertFalse(ok)
        self.assertIn("permanent_bounces", warning)
        self.assertNotIn("recipient@example.com", warning)

    def test_a_suppressed_recipient_is_a_failure(self):
        ok, _, warning = self.send(result={"queued": ["recipient@example.com"],
                                           "suppressed_recipients": ["recipient@example.com"]})
        self.assertFalse(ok)
        self.assertIn("suppressed_recipients", warning)

    def test_success_false_is_a_failure(self):
        ok, _, warning = self.send(result={"queued": ["recipient@example.com"]}, success=False)
        self.assertFalse(ok)
        self.assertIn("did not report success", warning)

    def test_a_non_2xx_status_is_a_failure(self):
        error = urllib.error.HTTPError("https://api.example/send", 500, "server error", None, io.BytesIO(b"{}"))
        ok, _, warning = self.send(error=error)
        self.assertFalse(ok)
        self.assertIn("HTTP 500", warning)
        self.assertNotIn("api.example", warning, "the URL carries the account id and is not logged")

    def test_an_unconfirmable_recipient_is_a_failure(self):
        self.assertFalse(self.send(result={"queued": [], "permanent_bounces": []})[0])
        self.assertFalse(self.send(result="not a result")[0])

    def test_an_unreadable_response_is_a_failure(self):
        self.assertFalse(self.send(reply=b"not json")[0])

    def test_an_unreachable_api_is_a_failure(self):
        self.assertFalse(self.send(error=OSError("down"))[0])

    def test_a_truncated_response_is_a_failure(self):
        ok, _, warning = self.send(error=http.client.IncompleteRead(b"{}"))
        self.assertFalse(ok)
        self.assertIn("IncompleteRead", warning)

    def test_missing_settings_send_nothing(self):
        with mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            self.assertFalse(test_report.send_email("s", "b", {}))
        urlopen.assert_not_called()
        self.assertIn("not configured", stderr.getvalue())
        self.assertNotIn("::error::", stderr.getvalue())

    def test_a_configuration_problem_is_noted_on_the_run_summary(self):
        summary = Path(tempfile.mkdtemp()) / "step-summary.md"
        with mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stderr", new=io.StringIO()):
            self.assertFalse(test_report.send_email("s", "b", {"GITHUB_STEP_SUMMARY": str(summary)}))
        urlopen.assert_not_called()
        self.assertIn("Report email not sent", summary.read_text())

    def test_missing_settings_still_track_the_failure(self):
        # No email is sent, but the run summary and the tracking issue still are.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        summary, github = directory / "report.txt", FakeGitHub()
        env = {"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r"}
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                     "--summary-file", str(summary)], env=env)
        self.assertEqual(code, 0)
        self.assertNotIn("::error::", stderr.getvalue())
        self.assertIn("TestZ", summary.read_text())
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestZ"])

    def test_the_annotation_names_the_setting_to_fix(self):
        # An operator must be able to tell which setting is wrong from the
        # failing run alone. Every setting is required, so each is named when
        # it is the one that is missing or malformed, and the others are not.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        cases = {
            "CF_ACCOUNT_ID": dict(self.env, CF_ACCOUNT_ID=""),
            "CF_EMAIL_TOKEN": dict(self.env, CF_EMAIL_TOKEN=""),
            "OPS_EMAIL_FROM": dict(self.env, OPS_EMAIL_FROM="sender@example.com, second@example.com"),
            "OPS_EMAIL_TO": dict(self.env, OPS_EMAIL_TO="not an address"),
            "OPS_EMAIL_REPLY_TO": dict(self.env, OPS_EMAIL_REPLY_TO=""),
        }
        for name, env in cases.items():
            with self.subTest(setting=name):
                with mock.patch("test_report.open_no_redirect") as urlopen, \
                        mock.patch("sys.stdout", new=io.StringIO()), \
                        mock.patch("sys.stderr", new=io.StringIO()) as stderr:
                    code = test_report.main(
                        ["--kind", "nightly", "--go-json", str(directory / "go.json")],
                        env=dict(env, REPORT_EMAIL_REQUIRED="true"))
                urlopen.assert_not_called()
                self.assertEqual(code, 1)
                annotations = stderr.getvalue()
                self.assertIn(f"::error::The report email is not configured correctly ({name} ", annotations)
                for other in set(cases) - {name}:
                    self.assertNotIn(other, annotations)

    def test_a_missing_email_setting_still_keeps_the_full_report(self):
        # The step summary copy is the run's only copy when no email can be
        # sent, so the summary file must hold the whole report, not the
        # truncated message body.
        directory = Path(tempfile.mkdtemp())
        marker = "end-of-the-report"
        (directory / "go.json").write_text(events(
            ("output", "p", "TestZ", "x" * test_report.EMAIL_TEXT_LIMIT + marker + "\n"),
            ("fail", "p", "TestZ", None)))
        summary = directory / "report.txt"
        with mock.patch("test_report.open_no_redirect") as urlopen, \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()):
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                     "--summary-file", str(summary)],
                                    env={"REPORT_EMAIL_REQUIRED": "true"})
        urlopen.assert_not_called()
        self.assertEqual(code, 1)
        kept = summary.read_text(encoding="utf-8")
        self.assertIn(marker, kept)
        self.assertGreater(len(kept), test_report.EMAIL_TEXT_LIMIT)

    def test_a_real_report_job_without_settings_fails_after_tracking(self):
        # A scheduled or post-merge report job must not go green when its only
        # notification channel is not configured; tests leave the marker unset.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        summary, github = directory / "report.txt", FakeGitHub()
        env = {"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r", "REPORT_EMAIL_REQUIRED": "true"}
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                     "--summary-file", str(summary)], env=env)
        self.assertEqual(code, 1)
        self.assertIn("::error::The report email is not configured", stderr.getvalue())
        self.assertIn("TestZ", summary.read_text())
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestZ"])

    def test_a_required_report_job_fails_for_every_failed_send(self):
        # Email is the only notification channel, so a real report job must
        # not go green on any failed send, after it still wrote the summary
        # and tracked the failure.
        cases = {
            "API error": {"error": urllib.error.HTTPError(
                "https://api.example/send", 500, "server error", None, io.BytesIO(b"{}"))},
            "success false": {"reply": {"success": False, "result": {"queued": []}}},
            "permanent bounce": {"reply": {"success": True, "result": {
                "permanent_bounces": ["recipient@example.com"]}}},
            "suppressed recipient": {"reply": {"success": True, "result": {
                "queued": ["recipient@example.com"],
                "suppressed_recipients": ["recipient@example.com"]}}},
        }
        for name, case in cases.items():
            with self.subTest(name):
                directory = Path(tempfile.mkdtemp())
                (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
                summary, github = directory / "report.txt", FakeGitHub()
                if "reply" in case:
                    response = mock.MagicMock()
                    response.__enter__.return_value.read.return_value = json.dumps(case["reply"]).encode()
                    urlopen = mock.patch("test_report.open_no_redirect", return_value=response)
                else:
                    urlopen = mock.patch("test_report.open_no_redirect", side_effect=case["error"])
                env = dict(self.env, GITHUB_TOKEN="t", GITHUB_REPOSITORY="o/r",
                           REPORT_EMAIL_REQUIRED="true")
                with urlopen, mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                        mock.patch.object(test_report, "GitHub", return_value=github), \
                        mock.patch("sys.stdout", new=io.StringIO()), \
                        mock.patch("sys.stderr", new=io.StringIO()) as stderr:
                    code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                             "--summary-file", str(summary)], env=env)
                self.assertEqual(code, 1)
                self.assertIn("::error::The report email could not be sent", stderr.getvalue())
                self.assertIn("TestZ", summary.read_text())
                self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestZ"])

    def test_any_enabled_required_marker_fails_a_broken_channel(self):
        # The marker is a human-set setting, so any non-empty value means
        # required and a misspelling must fail closed, not silently disable
        # the guarantee.
        for spelling in ("true", "True", "1", "yes", " TRUE ", "on", "enabled", "ture"):
            with self.subTest(spelling=spelling):
                directory = Path(tempfile.mkdtemp())
                (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
                env = {"REPORT_EMAIL_REQUIRED": spelling}
                with mock.patch("sys.stdout", new=io.StringIO()), \
                        mock.patch("sys.stderr", new=io.StringIO()) as stderr:
                    code = test_report.main(["--kind", "nightly", "--go-json",
                                             str(directory / "go.json")], env=env)
                self.assertEqual(code, 1)
                self.assertIn("::error::The report email is not configured", stderr.getvalue())

    def test_an_explicit_opt_out_keeps_a_failed_send_soft(self):
        for spelling in ("false", "FALSE", "0", "no", " "):
            with self.subTest(spelling=spelling):
                directory = Path(tempfile.mkdtemp())
                (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
                with mock.patch("sys.stdout", new=io.StringIO()), \
                        mock.patch("sys.stderr", new=io.StringIO()):
                    code = test_report.main(["--kind", "nightly", "--go-json",
                                             str(directory / "go.json")],
                                            env={"REPORT_EMAIL_REQUIRED": spelling})
                self.assertEqual(code, 0)

    def test_a_passing_report_job_fails_when_the_required_send_fails(self):
        # Nightly and weekly report passing runs too, so a broken channel
        # makes the run red even though every test passed.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("pass", "p", "TestA", None)))
        env = dict(self.env, REPORT_EMAIL_REQUIRED="true")
        error = urllib.error.HTTPError("https://api.example/send", 403, "denied", None, io.BytesIO(b"{}"))
        with mock.patch("test_report.open_no_redirect", side_effect=error), \
                mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json")], env=env)
        self.assertEqual(code, 1)
        self.assertIn("::error::The report email could not be sent", stderr.getvalue())

    def test_an_unexpected_send_exception_fails_a_required_report_job(self):
        # Any exception in the send path is a failed send, even when the
        # settings themselves look valid.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        summary, github = directory / "report.txt", FakeGitHub()
        step_summary = directory / "step-summary.md"
        env = dict(self.env, GITHUB_TOKEN="t", GITHUB_REPOSITORY="o/r",
                   REPORT_EMAIL_REQUIRED="true", GITHUB_STEP_SUMMARY=str(step_summary))
        with mock.patch.object(test_report, "send_email", side_effect=RuntimeError("boom")), \
                mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                     "--summary-file", str(summary)], env=env)
        self.assertEqual(code, 1)
        self.assertIn("::error::The report email could not be sent", stderr.getvalue())
        self.assertIn("**Report email not sent:** Email delivery failed (RuntimeError).",
                      step_summary.read_text())
        self.assertIn("TestZ", summary.read_text())
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestZ"])

    def test_a_failed_send_is_an_error_annotation(self):
        ok, _, message = self.send(error=OSError("down"))
        self.assertFalse(ok)
        self.assertIn("::error::", message)

    def test_a_failed_send_still_reports_the_run(self):
        # A send failure must not skip the summary or the failure tracking.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        summary, github = directory / "report.txt", FakeGitHub()
        error = urllib.error.HTTPError("https://api.example/send", 500, "server error", None, io.BytesIO(b"{}"))
        env = dict(self.env, GITHUB_TOKEN="t", GITHUB_REPOSITORY="o/r")
        with mock.patch("test_report.open_no_redirect", side_effect=error), \
                mock.patch.object(test_report, "EMAIL_RETRY_DELAY", 0), \
                mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                     "--summary-file", str(summary)], env=env)
        self.assertEqual(code, 0)
        self.assertIn("::error::", stderr.getvalue())
        self.assertIn("TestZ", summary.read_text())
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestZ"])

    def test_an_unexpected_send_exception_still_reports_the_run(self):
        # send_email must never be able to skip the summary or the tracking.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        summary, github = directory / "report.txt", FakeGitHub()
        env = dict(self.env, GITHUB_TOKEN="t", GITHUB_REPOSITORY="o/r")
        with mock.patch.object(test_report, "send_email", side_effect=RuntimeError("boom")), \
                mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch("sys.stdout", new=io.StringIO()), \
                mock.patch("sys.stderr", new=io.StringIO()) as stderr:
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json"),
                                     "--summary-file", str(summary)], env=env)
        self.assertEqual(code, 0)
        self.assertIn("::error::", stderr.getvalue())
        self.assertIn("TestZ", summary.read_text())
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestZ"])

    def test_logs_never_echo_the_settings(self):
        error = urllib.error.HTTPError("https://api.example/send", 403, "denied", None, io.BytesIO(b"{}"))
        _, _, warning = self.send(error=error)
        for value in self.env.values():
            self.assertNotIn(value, warning)

    def test_main_emails_the_report(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        with mock.patch.object(test_report, "send_email", return_value=True) as send, mock.patch("builtins.print"):
            test_report.main(["--kind", "weekly", "--go-json", str(directory / "go.json")],
                             env=dict(self.env, GITHUB_REPOSITORY="o/r", RUN_URL="http://run"))
        subject, body = send.call_args[0][:2]
        self.assertEqual(subject, "[Speedle weekly] FAILED")
        self.assertIn("p TestZ", body)
        self.assertIn("http://run", body)
        attachment = send.call_args.kwargs["attachment"]
        self.assertEqual(attachment["filename"], "speedle-weekly-report.txt")
        self.assertEqual(base64.b64decode(attachment["content"]).decode("utf-8"), body)

    def test_no_attachment_is_built_without_email_settings(self):
        # A local or test run without the settings must not encode the report.
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        with mock.patch.object(test_report, "report_attachment") as build, \
                mock.patch.object(test_report, "send_email", return_value=False), \
                mock.patch("builtins.print"):
            test_report.main(["--kind", "weekly", "--go-json", str(directory / "go.json")], env={})
        build.assert_not_called()

    def test_main_posts_the_configured_report(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "go.json").write_text(events(("fail", "p", "TestZ", None)))
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = json.dumps(
            {"success": True, "result": {"queued": [self.env["OPS_EMAIL_TO"]]}}).encode()
        with mock.patch("test_report.open_no_redirect", return_value=response) as urlopen, mock.patch("builtins.print"):
            code = test_report.main(["--kind", "nightly", "--go-json", str(directory / "go.json")],
                                    env=dict(self.env))
        self.assertEqual(code, 0)
        request = urlopen.call_args[0][0]
        self.assertEqual(request.full_url,
                         "https://api.cloudflare.com/client/v4/accounts/account-id/email/sending/send")
        payload = json.loads(request.data)
        self.assertEqual(payload["subject"], "[Speedle nightly] FAILED")
        self.assertIn("TestZ", payload["text"])
        [attachment] = payload["attachments"]
        self.assertEqual(attachment["filename"], "speedle-nightly-report.txt")
        self.assertEqual(base64.b64decode(attachment["content"]).decode("utf-8"), payload["text"])


class EmailTextTest(unittest.TestCase):
    """The body's claim about the attachment must match what it carries."""

    def body_and_attachment(self, report, run_url="http://run",
                            text_limit=200, attachment_limit=500):
        with mock.patch.object(test_report, "EMAIL_TEXT_LIMIT", text_limit), \
                mock.patch.object(test_report, "ATTACHMENT_TEXT_LIMIT", attachment_limit):
            body = test_report.email_text(report, run_url)
            attachment = test_report.report_attachment("report.txt", report)
        return body, base64.b64decode(attachment["content"]).decode("utf-8")

    def test_a_report_within_the_body_limit_is_sent_whole(self):
        body, attached = self.body_and_attachment("x" * 200)
        self.assertEqual(body, "x" * 200)
        self.assertNotIn("truncated", body)
        self.assertEqual(attached, "x" * 200)

    def test_a_report_within_the_attachment_limit_is_promised_whole(self):
        body, attached = self.body_and_attachment("x" * 500)
        self.assertLessEqual(len(body), 200)
        self.assertIn("the full report is attached", body)
        self.assertIn("the workflow run (http://run)", body)
        self.assertEqual(attached, "x" * 500)

    def test_a_report_beyond_the_attachment_limit_is_reported_cut_twice(self):
        body, attached = self.body_and_attachment("x" * 501)
        self.assertLessEqual(len(body), 200)
        self.assertIn("the attachment is truncated as well", body)
        self.assertNotIn("the full report is attached", body)
        self.assertIn("the workflow run (http://run)", body)
        self.assertLessEqual(len(attached.encode("utf-8")), 500)

    def test_the_attachment_cap_counts_bytes_not_characters(self):
        # Report text is Chinese, so the byte cap can be reached while the
        # character count is still under it.
        report = "断" * 250
        body, attached = self.body_and_attachment(report, attachment_limit=700)
        self.assertLessEqual(len(report), 700)
        self.assertGreater(len(report.encode("utf-8")), 700)
        self.assertIn("the attachment is truncated as well", body)
        self.assertLessEqual(len(attached.encode("utf-8")), 700)


class RequiredMarkerTest(unittest.TestCase):
    """The accepted opt-outs are documented in docs/RUNNERS.md."""

    def test_only_the_documented_values_turn_the_requirement_off(self):
        self.assertEqual(test_report.REPORT_EMAIL_OPT_OUTS, ("0", "false", "no"))
        for value in ("0", "false", "no", "False", "NO", " no ", "0\t"):
            with self.subTest(value=value):
                self.assertFalse(test_report.report_email_required({"REPORT_EMAIL_REQUIRED": value}))
        # Every other non-empty value, including a misspelling, means required.
        for value in ("true", "1", "yes", "on", "off", "disabled", "n", "ture", " false!"):
            with self.subTest(value=value):
                self.assertTrue(test_report.report_email_required({"REPORT_EMAIL_REQUIRED": value}))

    def test_an_unset_or_empty_marker_keeps_the_run_soft(self):
        self.assertFalse(test_report.report_email_required({}))
        self.assertFalse(test_report.report_email_required({"REPORT_EMAIL_REQUIRED": ""}))
        self.assertFalse(test_report.report_email_required({"REPORT_EMAIL_REQUIRED": "   "}))


class GitHubTest(unittest.TestCase):
    def test_call_uses_the_no_redirect_opener(self):
        # The repository token must not follow a redirect to another host.
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b"[]"
        with mock.patch("test_report.open_no_redirect", return_value=response) as opener:
            self.assertEqual(test_report.GitHub("token", "o/r").call("GET", "/issues"), [])
        self.assertEqual(opener.call_args.kwargs["timeout"], 60)


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
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch.object(test_report, "send_email", return_value=False), mock.patch("builtins.print"):
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
        with mock.patch.object(test_report, "send_email", return_value=False), mock.patch("builtins.print"):
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
        with mock.patch.object(test_report, "send_email", return_value=False), mock.patch("builtins.print"):
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


class RegressionTest(unittest.TestCase):
    """The post-merge regression: quiet on success, English, issues and email."""
    env = {"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r", "RUN_URL": "http://run",
           "COMMIT_URL": "http://commit/abc", "ARK_API_KEY": "k"}

    def run_main(self, text, issues=()):
        directory = Path(tempfile.mkdtemp())
        (directory / "go-file.json").write_text(text)
        summary = directory / "report.txt"
        github = FakeGitHub(issues)
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch.object(test_report, "ark_analyze", return_value="Likely cause: nil map.") as ark, \
                mock.patch.object(test_report, "send_email", return_value=True) as email, \
                mock.patch("builtins.print"):
            code = test_report.main(["--kind", "regression", "--source", f"run:success:{directory}",
                                     "--summary-file", str(summary)], env=self.env)
        self.assertEqual(code, 0)
        return github, ark, email, summary.read_text()

    def test_a_passing_run_sends_nothing(self):
        github, ark, email, summary = self.run_main(events(("pass", "p", "TestA", None)))
        ark.assert_not_called()
        email.assert_not_called()
        self.assertEqual(github.calls, [])
        self.assertIn("post-merge regression test report: PASSED", summary)
        self.assertIn("Commit: http://commit/abc", summary)

    def test_a_failure_opens_an_issue_with_output_and_english_analysis(self):
        github, ark, email, summary = self.run_main(events(
            ("output", "p", "TestB", "panic: assignment to entry in nil map\n"), ("fail", "p", "TestB", None)))
        self.assertEqual(ark.call_args.kwargs["language"], "English")
        self.assertIn("Commit: http://commit/abc", ark.call_args[0][0])
        subject, body = email.call_args[0][:2]
        self.assertEqual(subject, "[Speedle post-merge regression] FAILED")
        for part in ("Commit: http://commit/abc", "Run: http://run",
                     "assignment to entry in nil map", "Likely cause: nil map."):
            self.assertIn(part, body)
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
                mock.patch.object(test_report, "send_email", return_value=True), mock.patch("builtins.print"):
            test_report.main(["--kind", "regression", "--source", f"run:failure:{directory / 'missing'}"],
                             env={"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r"})
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: check results missing"])

    def test_a_failed_send_still_tracks_the_failure(self):
        directory = Path(tempfile.mkdtemp())
        (directory / "go-file.json").write_text(events(("fail", "p", "TestB", None)))
        github, summary = FakeGitHub(), directory / "report.txt"
        with mock.patch.object(test_report, "GitHub", return_value=github), \
                mock.patch.object(test_report, "ark_analyze", return_value=None), \
                mock.patch.object(test_report, "send_email", return_value=False), mock.patch("builtins.print"):
            code = test_report.main(["--kind", "regression", "--source", f"run:success:{directory}",
                                     "--summary-file", str(summary)], env=self.env)
        self.assertEqual(code, 0)
        self.assertEqual([c[2]["title"] for c in github.calls], ["Test failure: p TestB"])
        self.assertIn("TestB", summary.read_text())

    def test_scheduled_runs_keep_chinese_analysis(self):
        response = mock.MagicMock()
        response.__enter__.return_value.read.return_value = b"{}"
        with mock.patch("test_report.open_no_redirect", return_value=response) as urlopen:
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
        with mock.patch.object(test_report, "send_email", return_value=True), mock.patch("builtins.print"):
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
