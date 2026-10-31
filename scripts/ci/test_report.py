#!/usr/bin/env python3
"""Summarize scheduled and post-merge test results, analyze them with Ark,
send the report and track failing tests in GitHub issues.

Inputs are `go test -json` streams and plain check results. Every external
step is optional and fails soft: without an Ark key the raw summary is sent,
without a Feishu webhook or SMTP settings nothing is sent that way, and
without a GitHub token no issues are touched. The script exits non-zero only
on bad input.
"""
import argparse
import base64
import hashlib
import hmac
import json
import os
import smtplib
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from email.message import EmailMessage
from pathlib import Path

ARK_URL = "https://ark.cn-beijing.volces.com/api/plan/v3/responses"
DEFAULT_MODEL = "kimi-k3"
ISSUE_LABEL = "test-failure"
MAX_ISSUES = 20
OUTPUT_LINES = 40
PROMPT_LIMIT = 200_000

# How each kind of run reports. Scheduled runs report every result in
# Chinese, by Feishu and email. The post-merge regression only speaks up
# when it fails, in English, by Feishu and issues.
KINDS = {
    "nightly": {"title": "nightly", "language": "Chinese", "mail": True, "quiet_on_pass": False},
    "weekly": {"title": "weekly", "language": "Chinese", "mail": True, "quiet_on_pass": False},
    "regression": {"title": "post-merge regression", "language": "English", "mail": False, "quiet_on_pass": True},
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
            if action not in ("pass", "fail", "skip"):
                continue
            counts = packages.setdefault(pkg, {"pass": 0, "fail": 0, "skip": 0, "package_failed": False})
            if test is None:
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


def summarize(kind, packages, failures, checks, run_url, commit_url="", performance=""):
    total = {k: sum(c[k] for c in packages.values()) for k in ("pass", "fail", "skip")}
    failed_checks = [c for c in checks if not c["passed"]]
    ok = not failures and not failed_checks
    lines = [f"# Speedle+ {KINDS[kind]['title']} test report: {'PASSED' if ok else 'FAILED'}", ""]
    if commit_url:
        lines += [f"Commit: {commit_url}", ""]
    if run_url:
        lines += [f"Run: {run_url}", ""]
    lines += [f"Go tests: {total['pass']} passed, {total['fail']} failed, {total['skip']} skipped "
              f"in {len(packages)} packages.", ""]
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


def ark_analyze(summary, api_key, model, url=ARK_URL, timeout=300, language="Chinese"):
    """Return the model's analysis, or None when Ark is unavailable."""
    prompt = (
        "You are the test-results analyst for Speedle+, an open-source Go authorization "
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
        with urllib.request.urlopen(request, timeout=timeout) as response:
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


def send_mail(subject, body, env):
    host, to = env.get("SMTP_SERVER"), env.get("MAIL_TO")
    if not host or not to:
        print("::warning::SMTP_SERVER or MAIL_TO is not configured; the report is not mailed.", file=sys.stderr)
        return False
    message = EmailMessage()
    message["Subject"] = subject
    message["From"] = env.get("MAIL_FROM") or env.get("SMTP_USERNAME") or to
    message["To"] = to
    message.set_content(body)
    port = int(env.get("SMTP_PORT") or 465)
    user, password = env.get("SMTP_USERNAME"), env.get("SMTP_PASSWORD")
    if port == 465:
        server = smtplib.SMTP_SSL(host, port, timeout=60)
    else:
        server = smtplib.SMTP(host, port, timeout=60)
        server.starttls()
    with server:
        if user and password:
            server.login(user, password)
        server.send_message(message)
    return True


FEISHU_ANALYSIS_CHARS = 1500
FEISHU_LISTED_FAILURES = 10


def feishu_sign(secret, timestamp):
    """Signature of a Feishu custom bot with signature verification on."""
    key = f"{timestamp}\n{secret}".encode()
    return base64.b64encode(hmac.new(key, b"", hashlib.sha256).digest()).decode()


def feishu_card(kind, ok, packages, failed, analysis, run_url, issues_url):
    """A short card: verdict, counts, the first failures, the analysis and links."""
    total = {k: sum(c[k] for c in packages.values()) for k in ("pass", "fail", "skip")}
    lines = [f"**Go 测试**：{total['pass']} 通过，{total['fail']} 失败，{total['skip']} 跳过（{len(packages)} 个包）"]
    if failed:
        lines.append(f"**失败项**（{len(failed)}）：")
        lines += [f"- {name}" for name in failed[:FEISHU_LISTED_FAILURES]]
        if len(failed) > FEISHU_LISTED_FAILURES:
            lines.append(f"- ……另有 {len(failed) - FEISHU_LISTED_FAILURES} 项")
    if analysis:
        text = analysis if len(analysis) <= FEISHU_ANALYSIS_CHARS else analysis[:FEISHU_ANALYSIS_CHARS] + "……"
        lines += ["**Ark 分析**：", text]
    else:
        lines.append("（Ark 分析不可用）")
    links = [f"[运行详情]({run_url})"] if run_url else []
    if failed and issues_url:
        links.append(f"[失败跟踪 issue]({issues_url})")
    if links:
        lines.append(" · ".join(links))
    return {
        "header": {"template": "green" if ok else "red",
                   "title": {"tag": "plain_text",
                             "content": f"Speedle+ {KINDS[kind]['title']} 测试：{'通过' if ok else '失败'}"}},
        "elements": [{"tag": "markdown", "content": "\n".join(lines)}],
    }


def send_feishu(card, env, now=time.time):
    webhook = env.get("FEISHU_WEBHOOK")
    if not webhook:
        print("::warning::FEISHU_WEBHOOK is not configured; no Feishu message is sent.", file=sys.stderr)
        return False
    body = {"msg_type": "interactive", "card": card}
    if env.get("FEISHU_WEBHOOK_SECRET"):
        timestamp = str(int(now()))
        body.update(timestamp=timestamp, sign=feishu_sign(env["FEISHU_WEBHOOK_SECRET"], timestamp))
    request = urllib.request.Request(webhook, data=json.dumps(body).encode(), method="POST",
                                     headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            reply = json.loads(response.read() or b"{}")
    except (urllib.error.URLError, OSError, ValueError) as error:
        # Never echo the webhook URL: it is a credential.
        print(f"::warning::Feishu message failed ({type(error).__name__}).", file=sys.stderr)
        return False
    if reply.get("code", reply.get("StatusCode", 0)) != 0:
        print(f"::warning::Feishu rejected the message (code {reply.get('code', reply.get('StatusCode'))}).",
              file=sys.stderr)
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
        with urllib.request.urlopen(request, timeout=60) as response:
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
    # A job that did not succeed must show up as failed even when the results
    # it left behind, if any, hold no failure: it may have been cancelled or
    # timed out mid-suite, or a suite may have exited non-zero without a
    # failure event.
    for spec in args.source:
        job, _, rest = spec.partition(":")
        result, _, path = rest.partition(":")
        path = Path(path)
        if path.is_dir() or not path.suffix:
            streams, specs = expand_results_dir(path)
        else:
            streams, specs = [str(path)], []
        job_packages, job_failures, job_checks = collect(streams, specs)
        for package, counts in job_packages.items():
            merged = packages.setdefault(package, {"pass": 0, "fail": 0, "skip": 0, "package_failed": False})
            for key in ("pass", "fail", "skip"):
                merged[key] += counts[key]
        failures += job_failures
        checks += job_checks
        if result not in ("", "success") and not job_failures and all(c["passed"] for c in job_checks):
            checks.append({"name": f"{job} job", "passed": False,
                           "output": f"The {job} job ended with {result} and its results show no failure; "
                                     "they may be incomplete."})
    run_url, commit_url = env.get("RUN_URL", ""), env.get("COMMIT_URL", "")
    policy = KINDS[args.kind]
    directories = list(args.results_dir) + [spec.split(":", 2)[2] for spec in args.source if spec.count(":") >= 2]
    ok, summary = summarize(args.kind, packages, failures, checks, run_url, commit_url,
                            performance_section(directories))
    if ok and policy["quiet_on_pass"]:
        if args.summary_file:
            Path(args.summary_file).write_text(summary)
        print(summary)
        return 0

    analysis = None
    if env.get("ARK_API_KEY"):
        analysis = ark_analyze(summary, env["ARK_API_KEY"], env.get("ARK_MODEL") or DEFAULT_MODEL,
                               language=policy["language"])
    report = (f"{analysis}\n\n----\n\n" if analysis else
              "(Ark analysis unavailable; raw results follow.)\n\n") + summary
    if args.summary_file:
        Path(args.summary_file).write_text(report)

    if policy["mail"]:
        subject = f"[Speedle+ {policy['title']}] {'PASSED' if ok else 'FAILED'}"
        try:
            send_mail(subject, report, env)
        except (smtplib.SMTPException, OSError) as error:
            print(f"::warning::Mail delivery failed ({type(error).__name__}).", file=sys.stderr)

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
    failed = [issue_title(f).removeprefix("Test failure: ") for f in dedupe_failures(tracked)]
    repository = env.get("GITHUB_REPOSITORY")
    issues_url = (f"https://github.com/{repository}/issues?q=is%3Aopen+label%3A{ISSUE_LABEL}"
                  if repository else "")
    send_feishu(feishu_card(args.kind, ok, packages, failed, analysis, run_url, issues_url), env)
    print(summary)
    return 0


if __name__ == "__main__":
    sys.exit(main())
