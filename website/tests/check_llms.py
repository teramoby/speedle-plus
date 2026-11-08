#!/usr/bin/env python3
"""Check the generated llms.txt, llms-full.txt and robots.txt.

Offline checks run on every site build. With --external, every external link is
requested without following redirects and must answer 200 directly.

    python3 website/tests/check_llms.py docs
    python3 website/tests/check_llms.py docs --external
    python3 website/tests/check_llms.py docs --external-full

--external checks the curated links in llms.txt; --external-full also checks
every link inside llms-full.txt, which includes code-reference links that some
hosts rate-limit for automated clients.
"""
import argparse
import concurrent.futures
import html
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[2]
ORIGIN = "https://engine.speedle.io"
HOSTS = {"engine.speedle.io", "speedle.io"}
AI_CRAWLERS = (
    "GPTBot",
    "OAI-SearchBot",
    "ChatGPT-User",
    "ClaudeBot",
    "Claude-User",
    "Claude-SearchBot",
    "PerplexityBot",
    "Google-Extended",
    "Applebot-Extended",
    "CCBot",
)
MARKDOWN_LINK = re.compile(r"!?\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
HTML_LINK = re.compile(r"\b(?:href|src|srcset)=\"([^\"]*)\"")
BULLET_LINK = re.compile(r"^- \[[^\]]+\]\(https://[^)]+\): .+$")


def fail(message):
    print(f"FAIL llms: {message}", file=sys.stderr)
    return 1


def page_copy_definition():
    source = ROOT / "docs/hugo/content/about.md"
    text = source.read_text(encoding="utf-8")
    body = text.split("\n+++\n", 1)[1].strip()
    paragraph = re.split(r"\n\s*\n", body, maxsplit=1)[0]
    return re.sub(r"\s+", " ", paragraph)


def target_file(root, path):
    if not path.startswith("/"):
        return None
    path = path[1:]
    candidates = [root / path]
    if path.endswith("/") or not path:
        candidates = [root / path / "index.html"]
    else:
        candidates.append(root / path / "index.html")
    for candidate in candidates:
        if candidate.is_file():
            return candidate
    return None


def collect_links(text):
    links = set(MARKDOWN_LINK.findall(text))
    for value in HTML_LINK.findall(text):
        links.add(html.unescape(value))
    return links


def check_links(name, text, root, external):
    failures = []
    internal = 0
    remote = set()
    for raw in sorted(collect_links(text)):
        if raw.startswith(("mailto:", "tel:", "data:", "javascript:")):
            continue
        if raw.startswith("#"):
            failures.append(f"{name}: relative fragment {raw}")
            continue
        if not raw.startswith("https://"):
            failures.append(f"{name}: link is not absolute https: {raw}")
            continue
        split = urlsplit(raw)
        if split.netloc in HOSTS:
            internal += 1
            if split.netloc != "engine.speedle.io":
                failures.append(f"{name}: canonical host required, got {raw}")
                continue
            target = target_file(root, split.path)
            if target is None:
                failures.append(f"{name}: internal target missing for {raw}")
                continue
            if target.suffix == ".html" and 'http-equiv="refresh"' in target.read_text(encoding="utf-8", errors="replace"):
                failures.append(f"{name}: {raw} redirects instead of returning 200 directly")
        else:
            remote.add((split.scheme, split.netloc, split.path, split.query))
    if external and remote:
        failures.extend(check_remote(remote))
    return failures, internal, len(remote)


def check_remote(urls):
    def fetch(parts):
        scheme, netloc, path, query = parts
        url = f"{scheme}://{netloc}{path}"
        if query:
            url += "?" + query
        for attempt in range(4):
            request = urllib.request.Request(
                url,
                headers={
                    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
                    "Accept": "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8",
                },
            )
            try:
                with urllib.request.urlopen(request, timeout=30) as response:
                    if response.status != 200:
                        return f"external link {url} returned {response.status}"
                    response.read(1)
                    return None
            except urllib.error.HTTPError as error:
                if (error.code == 429 or error.code >= 500) and attempt < 3:
                    time.sleep(4 * (attempt + 1))
                    continue
                return f"external link {url} returned {error.code}"
            except Exception as error:  # noqa: BLE001 - report the real failure
                return f"external link {url} failed: {error}"
        return f"external link {url} failed after retries"

    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        return [failure for failure in pool.map(fetch, sorted(urls)) if failure]


def check_llms(root, external):
    path = root / "llms.txt"
    if not path.is_file():
        return [f"{path} is missing"], 0, 0
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    failures = []
    if not lines or lines[0] != "# Speedle":
        failures.append("llms.txt must start with '# Speedle'")
    summary = next((line.strip() for line in lines[1:] if line.strip()), "")
    if not summary.startswith("> "):
        failures.append("llms.txt must start with an H1 followed by a blockquote summary")
    else:
        definition = summary[2:].strip()
        expected = page_copy_definition()
        if definition != expected:
            failures.append("llms.txt definition is not the first paragraph of docs/hugo/content/about.md")
    h2 = [line for line in lines if line.startswith("## ")]
    if len(h2) < 6:
        failures.append("llms.txt must have H2 sections of annotated links")
    for line in lines:
        if line.startswith("- ["):
            if not BULLET_LINK.match(line):
                failures.append(f"llms.txt bullet is not an annotated absolute link: {line}")
    for fact in ("0.2.0", "@speedle/sdk", "@speedle/mcp", "opens soon"):
        if fact not in text:
            failures.append(f"llms.txt is missing the current statement containing {fact!r}")
    for cross in ("https://ai.speedle.io/llms.txt", "https://cloud.speedle.io/llms.txt"):
        if cross not in text:
            failures.append(f"llms.txt must link {cross}")
    link_failures, internal, remote = check_links("llms.txt", text, root, external)
    return failures + link_failures + [
        f"llms.txt has too few absolute engine links ({internal})" if internal < 20 else ""
    ], internal, remote


def check_llms_full(root, external):
    path = root / "llms-full.txt"
    if not path.is_file():
        return [f"{path} is missing"], 0, 0
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    failures = []
    if not lines or not lines[0].startswith("# Speedle"):
        failures.append("llms-full.txt must start with a Speedle H1")
    summary = next((line.strip() for line in lines[1:] if line.strip()), "")
    if not summary.startswith("> "):
        failures.append("llms-full.txt must start with a blockquote summary")
    elif summary[2:].strip() != page_copy_definition():
        failures.append("llms-full.txt definition is not the first paragraph of about.md")
    required = [
        ("policy grammar", "POLICY = EFFECT SUBJECT ACTION RESOURCE"),
        ("role-policy grammar", "ROLE_POLICY = EFFECT SUBJECT ROLE"),
        ("role inheritance example", "grant role manager employee"),
        ("deny over grant example", "deny user carol delete journal"),
        ("condition example", "if department == 'finance'"),
        ("built-in attributes", "request_user"),
        ("built-in functions", "IsSubSet"),
        ("custom functions", "Custom Functions"),
        ("missing attribute rule", "does not match"),
        ("override rule", "take precedence over built-in attributes"),
    ]
    for label, needle in required:
        if needle not in text:
            failures.append(f"llms-full.txt is missing the {label} ({needle!r})")
    link_failures, internal, remote = check_links("llms-full.txt", text, root, external)
    return failures + link_failures + [
        f"llms-full.txt has too few absolute engine links ({internal})" if internal < 40 else ""
    ], internal, remote


def check_robots(root):
    path = root / "robots.txt"
    if not path.is_file():
        return [f"{path} is missing"]
    lines = [line.strip() for line in path.read_text(encoding="utf-8").splitlines()]
    failures = []
    if "User-agent: *" not in lines:
        failures.append("robots.txt must keep the wildcard user-agent group")
    groups = {}
    current = None
    for line in lines:
        if not line or line.startswith("#"):
            continue
        if line.lower().startswith("user-agent:"):
            agent = line.split(":", 1)[1].strip()
            if current != agent:
                groups.setdefault(agent, [])
            current = agent
        elif current:
            groups[current].append(line.lower())
    if "/" not in groups.get("*", []) and "allow: /" not in groups.get("*", []):
        failures.append("robots.txt must allow the wildcard user-agent")
    for agent in AI_CRAWLERS:
        rules = groups.get(agent)
        if rules is None:
            failures.append(f"robots.txt must name {agent}")
        elif "allow: /" not in rules:
            failures.append(f"robots.txt must allow {agent} to read the site")
    if "Sitemap: https://engine.speedle.io/sitemap.xml" not in lines:
        failures.append("robots.txt must keep the sitemap declaration")
    return failures


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("root", help="published site directory, for example docs")
    parser.add_argument("--external", action="store_true", help="request llms.txt external links without redirects")
    parser.add_argument("--external-full", action="store_true", help="also request llms-full.txt external links")
    options = parser.parse_args()
    root = Path(options.root)

    results = []
    for name, check, check_external in (
        ("llms.txt", check_llms, options.external),
        ("llms-full.txt", check_llms_full, options.external_full),
    ):
        failures, internal, remote = check(root, check_external)
        results.extend(failures)
        print(f"{name}: {internal} engine links, {remote} external links")
    results.extend(check_robots(root))

    results = [item for item in results if item]
    if results:
        for message in results:
            print(f"FAIL llms: {message}", file=sys.stderr)
        return 1
    print("PASS llms.txt, llms-full.txt and robots.txt")
    return 0


if __name__ == "__main__":
    sys.exit(main())
