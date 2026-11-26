#!/usr/bin/env python3
"""Summarize scheduled and post-merge test results, analyze them with Ark,
email the report with its full text attached, and track failing tests in
GitHub issues.

Inputs are `go test -json` streams and plain check results. Ark analysis and
GitHub tracking are optional and fail soft: without an Ark key the raw
summary is sent, and without a GitHub token no issues are touched. The email
is the notification channel: a missing or malformed configuration and a
failed send are annotated (a configuration annotation names the setting) and
fail the step only when it runs as a real report job, marked by
REPORT_EMAIL_REQUIRED; tests and local runs stay green. The script exits
non-zero on bad input and on such a failure.
"""
import argparse
import base64
import email.errors
import email.utils
import http.client
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from email.headerregistry import Address
from pathlib import Path

ARK_URL = "https://ark.cn-beijing.volces.com/api/plan/v3/responses"
# The Cloudflare Email Service send API, its request fields, the
# queued/delivered/permanent_bounces/suppressed_recipients result fields and
# the limits below:
# https://developers.cloudflare.com/api/resources/email_sending/methods/send/
EMAIL_API = "https://api.cloudflare.com/client/v4/accounts/{account}/email/sending/send"
DEFAULT_MODEL = "kimi-k3"
ISSUE_LABEL = "test-failure"
MAX_ISSUES = 20
OUTPUT_LINES = 40
PROMPT_LIMIT = 200_000
# A transient email-API failure must not turn a passing run red. Only these
# statuses and a network error are retried; a rejected request is not. One
# retry bounds a duplicate report whose first response was lost, while still
# covering a single blip.
EMAIL_RETRIES = 1
EMAIL_RETRY_DELAY = 2
RETRYABLE_STATUS = (429, 500, 502, 503, 504)

# How each kind of run reports. Scheduled runs report every result in
# Chinese, by email. The post-merge regression only speaks up when it fails,
# in English, by email and issues.
KINDS = {
    "nightly": {"title": "nightly", "language": "Chinese", "quiet_on_pass": False},
    "weekly": {"title": "weekly", "language": "Chinese", "quiet_on_pass": False},
    "regression": {"title": "post-merge regression", "language": "English", "quiet_on_pass": True},
}


def parse_go_json(paths):
    """Return per-package counts and the failed tests with their output."""
    packages, outputs, failures = {}, {}, []
    for path in paths:
        for line in Path(path).read_text(errors="replace").splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if not isinstance(event, dict):
                continue
            pkg, test, action = event.get("Package", ""), event.get("Test"), event.get("Action")
            key = (pkg, test)
            if action == "output":
                outputs.setdefault(key, []).append(event.get("Output", ""))
                continue
            if not pkg or action not in ("pass", "fail", "skip"):
                continue
            counts = packages.setdefault(pkg, {"pass": 0, "fail": 0, "skip": 0,
                                               "package_failed": False, "completed": False})
            if test is None:
                counts["completed"] = True
                if action == "fail":
                    counts["package_failed"] = True
                continue
            counts[action] += 1
            # A failing subtest also fails its parent; report only the leaf.
            if action == "fail" and not any(f["package"] == pkg and f["test"].startswith(test + "/") for f in failures):
                failures = [f for f in failures if not (f["package"] == pkg and test.startswith(f["test"] + "/"))]
                failures.append({"package": pkg, "test": test,
                                 "output": tail("".join(outputs.get(key, [])))})
    # A package that failed without a failing test (build error, panic in init
    # or TestMain) is reported as one failure for the whole package.
    for pkg, counts in packages.items():
        if counts["package_failed"] and not any(f["package"] == pkg for f in failures):
            failures.append({"package": pkg, "test": "(package)",
                             "output": tail("".join(outputs.get((pkg, None), [])))})
    return packages, failures


# Skip messages of tests that can never run against a given store, so are not
# a gap in the run (see testutil/storetest/conformance.go).
NOT_APPLICABLE = ("the store does not manage",)
LISTED_SKIPS = 10


def skip_reason(lines):
    """The t.Skip message among a test's output lines."""
    for line in reversed(lines):
        text = line.strip()
        if text and not text.startswith(("---", "===")):
            return text
    return ""


def classify_skips(paths):
    """Sort the skipped tests of a run into three kinds.

    A run executes the suite once per store, so a test that needs one store is
    skipped in the passes for the others although it ran in its own. Such a
    test counts as run elsewhere. A skip whose message says the test does not
    apply to a store is not applicable. Every other skipped test did not run
    in this run at all; it is listed once with its reason."""
    passed, skipped = set(), []
    for path in paths:
        outputs = {}
        for line in Path(path).read_text(errors="replace").splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if not isinstance(event, dict) or not event.get("Test"):
                continue
            key = (event.get("Package", ""), event["Test"])
            action = event.get("Action")
            if action == "output":
                outputs.setdefault(key, []).append(event.get("Output", ""))
            elif action == "pass":
                passed.add(key)
            elif action == "skip":
                skipped.append((key, skip_reason(outputs.get(key, []))))
    result = {"elsewhere": 0, "not_applicable": 0, "not_run": []}
    seen = set()
    for key, reason in skipped:
        if key in passed:
            result["elsewhere"] += 1
        elif any(marker in reason for marker in NOT_APPLICABLE):
            result["not_applicable"] += 1
        elif key not in seen:
            seen.add(key)
            result["not_run"].append({"package": key[0], "test": key[1], "reason": reason})
    return result


def tail(text, lines=OUTPUT_LINES):
    return "\n".join(text.rstrip().splitlines()[-lines:])


def parse_check(spec):
    """name=status:logfile, status being an exit code."""
    name, _, rest = spec.partition("=")
    status, _, log = rest.partition(":")
    if not name or not status.lstrip("-").isdigit():
        raise ValueError(f"invalid --check {spec!r}")
    text = Path(log).read_text(errors="replace") if log and Path(log).is_file() else ""
    return {"name": name, "passed": int(status) == 0, "output": tail(text)}


def missing_results(job, result, name):
    """The output of the failed check for results that never reached the report.

    The job's result is the only evidence of why nothing can be read, so the
    check names it. An empty output shows a failure without a cause, and the
    analysis then guesses that tests were not found.
    """
    if result == "success":
        return (f"The {job} job succeeded, but {name} does not exist: "
                "its results were not uploaded or could not be downloaded.")
    if result:
        return (f"The {job} job ended with {result} before any result reached this report: {name} does not exist. "
                "Its tests did not run, or their results were not uploaded; "
                "the job on the run page shows whether it ran at all.")
    return f"No results: {name} does not exist."


def has_readable_results(packages):
    """True when at least one terminal package or test result was read."""
    return any(c["completed"] or c["pass"] or c["fail"] or c["skip"]
               for c in packages.values())


def empty_results(job, result, name):
    """The output of the failed check when a source exists but holds no readable
    test results."""
    if result == "success":
        return (f"The {job} job succeeded, but {name} contains no readable test results: "
                "its results were not written or are incomplete.")
    return f"No results: {name} contains no readable test results."


def expand_results_dir(directory):
    """Return the go test -json streams and check specs a results directory holds."""
    if not directory.is_dir():
        return [], [f"results {directory.name}=1:"]
    # performance.json is scripts/ci/benchmark.py's result, not a test stream.
    streams = sorted(str(p) for p in directory.glob("*.json") if p.name != "performance.json")
    specs = []
    checks_file = directory / "checks.txt"
    if checks_file.is_file():
        for line in checks_file.read_text().splitlines():
            name, _, rest = line.partition("=")
            status, _, log = rest.partition(":")
            specs.append(f"{name}={status}:{directory / log}" if log else line)
    return streams, specs


def collect(go_json, check_specs):
    """Parse streams and checks; a missing stream is a failed check."""
    packages, failures = parse_go_json(p for p in go_json if Path(p).is_file())
    checks = [parse_check(spec) for spec in check_specs]
    checks += [{"name": f"results {Path(p).name}", "passed": False, "output": "result file is missing"}
               for p in go_json if not Path(p).is_file()]
    return packages, failures, checks


def performance_section(directories):
    """The benchmark comparison tables (performance.md) found in results directories."""
    tables = [p.read_text().strip() for d in directories for p in [Path(d) / "performance.md"] if p.is_file()]
    return "\n\n".join(tables)


def summarize(kind, packages, failures, checks, run_url, commit_url="", performance="", skips=None):
    total = {k: sum(c[k] for c in packages.values()) for k in ("pass", "fail", "skip")}
    failed_checks = [c for c in checks if not c["passed"]]
    ok = not failures and not failed_checks
    lines = [f"# Speedle {KINDS[kind]['title']} test report: {'PASSED' if ok else 'FAILED'}", ""]
    if commit_url:
        lines += [f"Commit: {commit_url}", ""]
    if run_url:
        lines += [f"Run: {run_url}", ""]
    lines += [f"Go tests: {total['pass']} passed, {total['fail']} failed, {total['skip']} skipped "
              f"in {len(packages)} packages.", ""]
    if not packages:
        # Zero everywhere is no suite at all: say so before it reads as one
        # that found nothing to test.
        lines += ["No test result was read, so nothing here says whether the tests pass.", ""]
    if skips is not None:
        lines += [f"Skipped: {len(skips['not_run'])} not run in any pass; {skips['elsewhere']} skipped in one "
                  f"store's pass but run in another's; {skips['not_applicable']} not applicable to their store.", ""]
        if skips["not_run"]:
            lines.append("Tests that did not run:")
            lines += [f"- {s['package']} {s['test']}: {s['reason'] or '(no reason given)'}" for s in skips["not_run"]]
            lines.append("")
    if checks:
        lines.append("Checks:")
        lines += [f"- {c['name']}: {'passed' if c['passed'] else 'FAILED'}" for c in checks]
        lines.append("")
    if failures:
        lines.append("Failed tests:")
        for f in failures:
            lines += [f"## {f['package']} {f['test']}", "```", f["output"] or "(no output)", "```"]
    for c in failed_checks:
        lines += [f"## Check {c['name']}", "```", c["output"] or "(no output)", "```"]
    if performance:
        lines += ["## Performance", "", performance]
    return ok, "\n".join(lines) + "\n"


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Refuse redirects: urlopen copies the Authorization header to the
    target, so a 3xx would hand a bearer token to another host."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def open_no_redirect(request, timeout):
    """Open a request without following redirects.

    Returning None from the redirect handler leaves urllib raising the 3xx as
    an HTTPError. Every call that carries a bearer token uses this opener.
    """
    return urllib.request.build_opener(_NoRedirect).open(request, timeout=timeout)


def ark_analyze(summary, api_key, model, url=ARK_URL, timeout=300, language="Chinese"):
    """Return the model's analysis, or None when Ark is unavailable."""
    prompt = (
        "You are the test-results analyst for Speedle, an open-source Go authorization "
        "engine (PMS policy management, ADS decisions, SPDL policies, file/etcd/MongoDB stores). "
        f"Analyze the test report below for the maintainers. Write in {language}, plain "
        "text. Start with a one-line overall verdict. For each failure, give the likely cause "
        "from the output, whether it looks like a product bug, a test bug, a flaky test or an "
        "environment problem, and the next step. Do not invent failures that are not in the "
        "report. If everything passed, say so briefly.\n\n" + summary[:PROMPT_LIMIT])
    body = json.dumps({"model": model, "input": prompt, "max_output_tokens": 4096,
                       "reasoning": {"effort": "medium"}}).encode()
    request = urllib.request.Request(url, data=body, method="POST", headers={
        "Authorization": f"Bearer {api_key}", "Content-Type": "application/json"})
    try:
        with open_no_redirect(request, timeout=timeout) as response:
            payload = json.loads(response.read())
    except (urllib.error.URLError, OSError, ValueError) as error:
        # Never echo the response or the key; the status class is enough.
        print(f"::warning::Ark analysis unavailable ({type(error).__name__}).", file=sys.stderr)
        return None
    return extract_text(payload)


def extract_text(payload):
    if not isinstance(payload, dict) or payload.get("status") not in (None, "completed"):
        return None
    parts = []
    for item in payload.get("output") or []:
        if isinstance(item, dict) and item.get("type") == "message":
            for part in item.get("content") or []:
                if isinstance(part, dict) and part.get("type") == "output_text" and isinstance(part.get("text"), str):
                    parts.append(part["text"])
    text = "\n".join(parts).strip()
    return text or None


EMAIL_TIMEOUT = 30
# The Email Service API accepts at most 50 addresses across to, cc and bcc.
MAX_EMAIL_RECIPIENTS = 50
# A mass-failure report can grow to megabytes. The message body carries a
# bounded prefix, and the full report travels in the attachment, so the email
# does not depend on the run summary or the issues for the whole text.
EMAIL_TEXT_LIMIT = 100_000
# The Email Service API accepts at most 25 MiB for the whole message,
# attachments included. Base64 expands the attachment by a third, so 17 MiB of
# report text becomes about 22.7 MiB, and even a body of EMAIL_TEXT_LIMIT
# characters, every one escaped in JSON, fits beside it. No real report comes
# near the cap; it keeps a pathological one from failing the send.
EMAIL_MAX_BYTES = 25 * 1024 * 1024
ATTACHMENT_TEXT_LIMIT = 17 * 1024 * 1024
REPORT_FILENAME = "speedle-{kind}-report.txt"
# The report jobs set REPORT_EMAIL_REQUIRED. These non-empty values turn the
# requirement off, compared case-insensitively after trimming whitespace; every
# other non-empty value turns it on, so a misspelling fails closed. An empty or
# unset marker is off too, keeping local runs and tests green.
REPORT_EMAIL_OPT_OUTS = ("0", "false", "no")


def addresses(value):
    """The bare, lowercased addresses of one key of the send result.

    A list entry is an address string or a REST address object
    ({"address", "name"}). Comparing bare addresses keeps a display name or a
    case difference from turning a delivered report into a failure.
    """
    if not isinstance(value, list):
        return []
    entries = [item.get("address") if isinstance(item, dict) else item for item in value]
    return [address.lower() for _, address in email.utils.getaddresses(
        [entry for entry in entries if isinstance(entry, str)]) if "@" in address]


def configured_addresses(value):
    """The bare addresses of one configured setting, in their configured case,
    or None when the value is not a list of addresses.

    A setting is a string that may list several recipients or use a display
    name ("Ops <ops@example.com>, second@example.com"). A token the parser
    cannot turn into an address is a configuration error, not a recipient to
    drop silently: a swallowed token appears as an entry with an empty
    address, and one malformed entry ("ops@", "@example.com") also rejects
    the whole setting. The case is kept because the local part may be
    case-sensitive on the server.
    """
    if not isinstance(value, str):
        return None
    entries = email.utils.getaddresses([value])
    if not entries or not all(address and is_mail_address(address) for _, address in entries):
        return None
    return [address for _, address in entries]


def is_mail_address(address):
    """Whether one bare address has a valid local part and a dotted domain
    name, so that a send to it can be delivered."""
    try:
        parsed = Address(addr_spec=address)
    except (ValueError, IndexError, email.errors.HeaderParseError):
        # Malformed input can also trigger parser bugs (for example,
        # IndexError on an empty local part in Python 3.12).
        return False
    labels = parsed.domain.split(".")
    return bool(parsed.username) and len(labels) > 1 and all(labels)


def note_in_summary(env, message):
    """Add a plain notice to the run summary, where an operator sees it,
    without a workflow-command annotation. Annotations are reserved for a
    failed send."""
    path = env.get("GITHUB_STEP_SUMMARY")
    if not path:
        return
    try:
        with open(path, "a", encoding="utf-8") as summary:
            summary.write(f"\n**Report email not sent:** {message}\n")
    except OSError:
        pass


def email_text(report, run_url=""):
    """The report as far as the message body carries it, with a pointer to the
    attachment when the full report travels there, or to the run page when the
    attachment is cut as well.

    The attachment is cut at ATTACHMENT_TEXT_LIMIT bytes, so the body must not
    call it complete beyond that. A report can exceed the byte cap while its
    character count is still under it, so the check is on the encoded bytes.
    """
    if len(report) <= EMAIL_TEXT_LIMIT:
        return report
    where = f"the workflow run ({run_url})" if run_url else "the workflow run page"
    if len(report.encode("utf-8")) <= ATTACHMENT_TEXT_LIMIT:
        footer = (f"\n\n[report truncated at {EMAIL_TEXT_LIMIT} characters; the full report is attached; "
                  f"see also {where} and the tracked issues]\n")
    else:
        footer = (f"\n\n[report truncated at {EMAIL_TEXT_LIMIT} characters; the attachment is truncated "
                  f"as well; see {where} for the rest]\n")
    return report[:EMAIL_TEXT_LIMIT - len(footer)] + footer


def report_attachment(filename, report):
    """The full report as a base64 text attachment of the email API.

    The API limits the whole message to 25 MiB, so a report beyond
    ATTACHMENT_TEXT_LIMIT bytes is cut between characters and ends with a note
    that says so. Only the first ATTACHMENT_TEXT_LIMIT characters are encoded:
    a character is at least one byte, so that prefix already covers the cap
    and a huge report is never encoded whole.
    """
    limit = ATTACHMENT_TEXT_LIMIT
    note = (f"\n\n[report truncated at {limit} bytes in the attachment; "
            f"see the workflow run]\n")
    data = (report[:limit] if len(report) > limit else report).encode("utf-8")
    if len(report) > limit or len(data) > limit:
        data = data[:limit - len(note.encode("utf-8"))]
        text = data.decode("utf-8", "ignore") + note
    else:
        text = report
    return {"content": base64.b64encode(text.encode("utf-8")).decode("ascii"),
            "filename": filename, "type": "text/plain", "disposition": "attachment"}


def email_config_problems(env):
    """The email settings that are missing or malformed, with the reason.

    Every setting is required. The sender is an outbound-only address that
    cannot receive mail, so a report sent without a Reply-To loses any reply,
    and the API takes exactly one sender. A recipient entry that is not an
    address rejects the whole list rather than dropping an operator silently.
    Only setting names and generic reasons are returned, never values, because
    the values hold addresses.
    """
    problems = []
    if not env.get("CF_ACCOUNT_ID"):
        problems.append("CF_ACCOUNT_ID is not set")
    if not env.get("CF_EMAIL_TOKEN"):
        problems.append("CF_EMAIL_TOKEN is not set")
    senders = configured_addresses(env.get("OPS_EMAIL_FROM"))
    if not senders:
        problems.append("OPS_EMAIL_FROM is not set or is not an address")
    elif len(senders) != 1:
        problems.append("OPS_EMAIL_FROM must hold exactly one address")
    recipients = configured_addresses(env.get("OPS_EMAIL_TO"))
    if not recipients:
        problems.append("OPS_EMAIL_TO is not set or is not an address list")
    elif len(recipients) > MAX_EMAIL_RECIPIENTS:
        problems.append(f"OPS_EMAIL_TO holds more than {MAX_EMAIL_RECIPIENTS} addresses")
    reply_to = configured_addresses(env.get("OPS_EMAIL_REPLY_TO"))
    if not reply_to:
        problems.append("OPS_EMAIL_REPLY_TO is not set or is not an address")
    elif len(reply_to) != 1:
        problems.append("OPS_EMAIL_REPLY_TO must hold exactly one address")
    return problems


def email_configured(env):
    """True when the Cloudflare Email settings hold one sender, one valid
    Reply-To and a usable recipient list."""
    return not email_config_problems(env)


def report_email_required(env):
    """True when the run is a real report job, so a failed send fails it.

    Any non-empty marker means required so a misspelling fails closed; only
    REPORT_EMAIL_OPT_OUTS and an unset or empty marker stay soft.
    """
    marker = env.get("REPORT_EMAIL_REQUIRED", "").strip().lower()
    return bool(marker) and marker not in REPORT_EMAIL_OPT_OUTS


def send_email(subject, body, env, attachment=None, timeout=EMAIL_TIMEOUT):
    """Send the report through the Cloudflare Email Service REST API.

    The API takes to as an array of plain address strings, and from and
    reply_to as plain address strings, so the configured values are parsed to
    their bare addresses once and the same set is sent and verified. The case
    is kept on the wire, where the local part may be case-sensitive, and
    ignored only when matching the API's reply. An attachment, built by
    report_attachment, carries the full report as base64 text. A send
    succeeds when the response lists every recipient under queued or
    delivered. A non-2xx status, success: false, a permanent bounce or a
    suppressed recipient is a failed send, and a redirect is refused rather
    than followed so the token is not forwarded. OPS_EMAIL_REPLY_TO is
    required: the sender is an outbound address that cannot receive mail,
    so without it a reply is lost. A message the API's size limit would
    reject is a failed send and sends nothing. Nothing from the settings or
    the response is logged, because both may hold addresses.

    A configuration problem is a plain notice that names the setting and
    sends nothing. A failed send is an error annotation on the run, and it
    fails the step only when the run is a real report job
    (REPORT_EMAIL_REQUIRED), after the summary and the failure tracking have
    run. A 429, a 5xx status or a network
    error is retried once with a short backoff, so a single transient blip
    does not turn a passing run red; the retry may duplicate a report whose
    first response was lost. The account id is percent-encoded into the
    path.
    """
    problems = email_config_problems(env)
    if problems:
        problem = "; ".join(problems)
        print(f"Cloudflare Email is not configured correctly ({problem}); "
              "the report is not emailed.", file=sys.stderr)
        note_in_summary(env, f"Cloudflare Email is not configured correctly ({problem}).")
        return False
    account, token = env["CF_ACCOUNT_ID"], env["CF_EMAIL_TOKEN"]
    senders = configured_addresses(env["OPS_EMAIL_FROM"])
    recipients = configured_addresses(env["OPS_EMAIL_TO"])
    reply_to = configured_addresses(env["OPS_EMAIL_REPLY_TO"])
    payload = {"to": recipients, "from": senders[0], "subject": subject, "text": body,
               "reply_to": reply_to[0]}
    if attachment is not None:
        payload["attachments"] = [attachment]
    message = json.dumps(payload).encode()
    if len(message) > EMAIL_MAX_BYTES:
        # The attachment cap keeps a real report well under this; the guard
        # only rejects a message the API would refuse.
        print(f"::error::Email delivery failed (the message exceeds the "
              f"{EMAIL_MAX_BYTES // (1024 * 1024)} MiB API limit).", file=sys.stderr)
        return False
    request = urllib.request.Request(
        EMAIL_API.format(account=urllib.parse.quote(account, safe="")), method="POST",
        data=message,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"})
    reply = None
    for attempt in range(EMAIL_RETRIES + 1):
        retry = attempt < EMAIL_RETRIES
        try:
            with open_no_redirect(request, timeout=timeout) as response:
                reply = json.loads(response.read() or b"{}")
            break
        except urllib.error.HTTPError as error:
            # The response body and the URL are not logged: both may hold addresses.
            error.close()
            if not retry or error.code not in RETRYABLE_STATUS:
                print(f"::error::Email delivery failed (HTTP {error.code}).", file=sys.stderr)
                return False
        except (urllib.error.URLError, OSError, http.client.HTTPException) as error:
            if not retry:
                print(f"::error::Email delivery failed ({type(error).__name__}).", file=sys.stderr)
                return False
        except ValueError as error:
            # An unreadable response body will not fix itself on a retry.
            print(f"::error::Email delivery failed ({type(error).__name__}).", file=sys.stderr)
            return False
        if retry:
            time.sleep(EMAIL_RETRY_DELAY)
    if not isinstance(reply, dict) or reply.get("success") is not True:
        print("::error::Email delivery failed (the API did not report success).", file=sys.stderr)
        return False
    result = reply.get("result")
    if not isinstance(result, dict):
        print("::error::Email delivery failed (the API returned no result).", file=sys.stderr)
        return False
    wanted = {address.lower() for address in recipients}
    for key in ("permanent_bounces", "suppressed_recipients"):
        if wanted & set(addresses(result.get(key))):
            print(f"::error::Email delivery failed (a recipient is listed in {key}).", file=sys.stderr)
            return False
    confirmed = set(addresses(result.get("queued"))) | set(addresses(result.get("delivered")))
    if not wanted <= confirmed:
        print("::error::Email delivery failed (the API did not confirm every recipient).", file=sys.stderr)
        return False
    return True


class GitHub:
    def __init__(self, token, repository, api="https://api.github.com"):
        self.token, self.repository, self.api = token, repository, api

    def call(self, method, path, data=None):
        request = urllib.request.Request(
            f"{self.api}/repos/{self.repository}{path}", method=method,
            data=json.dumps(data).encode() if data is not None else None,
            headers={"Authorization": f"Bearer {self.token}", "Accept": "application/vnd.github+json",
                     "X-GitHub-Api-Version": "2022-11-28", "Content-Type": "application/json"})
        with open_no_redirect(request, timeout=60) as response:
            return json.loads(response.read() or b"null")

    def open_issues(self):
        issues, page = [], 1
        while True:
            query = urllib.parse.urlencode({"state": "open", "labels": ISSUE_LABEL, "per_page": 100, "page": page})
            batch = self.call("GET", f"/issues?{query}")
            issues += [i for i in batch if "pull_request" not in i]
            if len(batch) < 100:
                return issues
            page += 1


def issue_title(failure):
    return f"Test failure: {failure['package']} {failure['test']}"


def dedupe_failures(failures):
    """Merge failures with the same issue title, such as one test failing with
    both stores or both toolchains, keeping each output."""
    merged = {}
    for failure in failures:
        title = issue_title(failure)
        if title in merged:
            if failure["output"] and failure["output"] not in merged[title]["output"]:
                merged[title]["output"] = (merged[title]["output"] + "\n----\n" + failure["output"]).strip()
        else:
            merged[title] = dict(failure)
    return list(merged.values())


def track_failures(github, kind, failures, run_url, analysis, commit_url=""):
    """Comment on an open issue for a known failure, otherwise open one.
    The post-merge regression attaches the analysis to comments as well, since
    each comment reports a new commit."""
    title_of_kind = KINDS[kind]["title"]
    existing = {i["title"]: i["number"] for i in github.open_issues()}
    created = commented = 0
    pending = []
    links = (f"Commit: {commit_url}\n\n" if commit_url else "") + f"Run: {run_url}\n\n"
    details = (f"\n<details><summary>Ark analysis of the whole run</summary>\n\n{analysis}\n\n</details>\n"
               if analysis else "")
    for failure in dedupe_failures(failures):
        title = issue_title(failure)
        body = (f"The {title_of_kind} run failed `{failure['test']}` in `{failure['package']}`.\n\n"
                f"{links}```\n{failure['output'] or '(no output)'}\n```\n")
        if title in existing:
            if kind == "regression":
                body += details
            github.call("POST", f"/issues/{existing[title]}/comments", {"body": body})
            commented += 1
        else:
            pending.append((title, body))
    for title, body in pending[:MAX_ISSUES]:
        github.call("POST", "/issues", {"title": title, "body": body + details, "labels": [ISSUE_LABEL]})
        created += 1
    if len(pending) > MAX_ISSUES:
        rest = "\n".join(f"- {t}" for t, _ in pending[MAX_ISSUES:])
        github.call("POST", "/issues", {
            "title": f"{title_of_kind.capitalize()} run: {len(pending) - MAX_ISSUES} more failing tests",
            "body": f"{links}Not tracked individually:\n{rest}\n", "labels": [ISSUE_LABEL]})
        created += 1
    return created, commented


def main(argv=None, env=os.environ):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kind", required=True, choices=list(KINDS))
    parser.add_argument("--go-json", action="append", default=[], help="go test -json output")
    parser.add_argument("--check", action="append", default=[], help="name=exit_status:log_file")
    parser.add_argument("--results-dir", action="append", default=[],
                        help="directory written by run-go-tests.sh: *.json streams and checks.txt")
    parser.add_argument("--source", action="append", default=[],
                        help="JOB:RESULT:PATH - results of a workflow job (a results directory or a "
                             "go test -json file) and the job's result, such as success or cancelled")
    parser.add_argument("--summary-file", help="also write the summary here")
    args = parser.parse_args(argv)

    go_json, check_specs = list(args.go_json), list(args.check)
    for directory in args.results_dir:
        streams, specs = expand_results_dir(Path(directory))
        go_json += streams
        check_specs += specs
    packages, failures, checks = collect(go_json, check_specs)
    all_streams = [p for p in go_json if Path(p).is_file()]
    # A job that did not succeed must show up as failed even when the results
    # it left behind, if any, hold no failure: it may have been cancelled or
    # timed out mid-suite, or a suite may have exited non-zero without a
    # failure event.
    for spec in args.source:
        job, _, rest = spec.partition(":")
        result, _, path = rest.partition(":")
        path = Path(path)
        name = path.name or str(path)
        if path.is_dir() or (not path.exists() and not path.suffix):
            streams, specs = expand_results_dir(path)
        else:
            streams, specs = [str(path)], []
        job_packages, job_failures, job_checks = collect(streams, specs)
        all_streams += [p for p in streams if Path(p).is_file()]
        for package, counts in job_packages.items():
            merged = packages.setdefault(package, {"pass": 0, "fail": 0, "skip": 0, "package_failed": False})
            for key in ("pass", "fail", "skip"):
                merged[key] += counts[key]
        failures += job_failures
        checks += job_checks
        if not path.exists():
            # Nothing of this job can be read, and the job's result is the
            # only evidence of why. Name it on collect()'s failed check, which
            # has no log to show; the check stays, so the run is still failed
            # and tracked.
            for check in job_checks:
                if check["name"] == f"results {name}" and not check["passed"]:
                    check["output"] = missing_results(job, result, name)
        elif result in ("", "success") and not has_readable_results(job_packages) and (streams or not job_checks):
            # An existing empty directory, or a stream file with no readable Go
            # test events, would otherwise leave a successful job with no failed
            # check and turn an incomplete artifact into PASSED. Keep any
            # explicit checks alongside this one; a check-only directory with no
            # stream files is still governed by its checks.txt entries.
            empty_check = {"name": f"results {name}", "passed": False,
                           "output": empty_results(job, result, name)}
            job_checks.append(empty_check)
            checks.append(empty_check)
        if result not in ("", "success") and not job_failures and all(c["passed"] for c in job_checks):
            checks.append({"name": f"{job} job", "passed": False,
                           "output": f"The {job} job ended with {result} and its results show no failure; "
                                     "they may be incomplete."})
    run_url, commit_url = env.get("RUN_URL", ""), env.get("COMMIT_URL", "")
    policy = KINDS[args.kind]
    directories = list(args.results_dir) + [spec.split(":", 2)[2] for spec in args.source if spec.count(":") >= 2]
    skips = classify_skips(all_streams)
    ok, summary = summarize(args.kind, packages, failures, checks, run_url, commit_url,
                            performance_section(directories), skips)
    if ok and policy["quiet_on_pass"]:
        if args.summary_file:
            Path(args.summary_file).write_text(summary, encoding="utf-8")
        print(summary)
        # A quiet-on-pass report job sends nothing, but it is still a real
        # report job: a missing or malformed setting must fail it now rather
        # than stay hidden until a failing run needs the notification.
        problems = email_config_problems(env) if report_email_required(env) else []
        if problems:
            problem = "; ".join(problems)
            note_in_summary(env, f"Cloudflare Email is not configured correctly ({problem}).")
            print(f"::error::The report email is not configured correctly ({problem}); "
                  "failing the report job.", file=sys.stderr)
            return 1
        return 0

    analysis = None
    if env.get("ARK_API_KEY"):
        analysis = ark_analyze(summary, env["ARK_API_KEY"], env.get("ARK_MODEL") or DEFAULT_MODEL,
                               language=policy["language"])
    report = (f"{analysis}\n\n----\n\n" if analysis else
              "(Ark analysis unavailable; raw results follow.)\n\n") + summary
    if args.summary_file:
        Path(args.summary_file).write_text(report, encoding="utf-8")

    subject = f"[Speedle {policy['title']}] {'PASSED' if ok else 'FAILED'}"
    email_failed = False
    try:
        # Build the attachment only when a send can happen: a local or test run
        # without the settings must not encode the report.
        attachment = (report_attachment(REPORT_FILENAME.format(kind=args.kind), report)
                      if email_configured(env) else None)
        email_failed = not send_email(subject, email_text(report, run_url), env, attachment=attachment)
    except Exception as error:
        # A notification bug must not skip the summary and failure tracking.
        email_failed = True
        note_in_summary(env, f"Email delivery failed ({type(error).__name__}).")
        print(f"::error::Email delivery failed ({type(error).__name__}).", file=sys.stderr)

    tracked = failures + [{"package": "check", "test": c["name"], "output": c["output"]}
                          for c in checks if not c["passed"]]
    if tracked and env.get("GITHUB_TOKEN") and env.get("GITHUB_REPOSITORY"):
        try:
            created, commented = track_failures(
                GitHub(env["GITHUB_TOKEN"], env["GITHUB_REPOSITORY"]), args.kind, tracked, run_url, analysis,
                commit_url)
            print(f"Tracked failures: {created} issues opened, {commented} comments added.")
        except (urllib.error.URLError, OSError, ValueError) as error:
            print(f"::warning::Failure issues not updated ({type(error).__name__}).", file=sys.stderr)
    print(summary)
    # A real report job must not go green when its only notification channel
    # did not deliver, whether it is misconfigured or the send failed.
    if email_failed and report_email_required(env):
        problems = email_config_problems(env)
        reason = (f"is not configured correctly ({'; '.join(problems)})" if problems
                  else "could not be sent")
        print(f"::error::The report email {reason}; failing the report job.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
