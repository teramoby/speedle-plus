#!/usr/bin/env python3
"""Audit a rendered Speedle site: internal files, in-page anchors and external URLs.

Run against the published tree, skipping the Hugo source templates:

    python3 website/tests/check_links.py docs --exclude hugo/

Loopback examples such as http://127.0.0.1:6734/... and historical non-ASCII
links in archived snapshots are counted as skipped, not requested.
"""
import argparse
import concurrent.futures
import html.parser
import os
import posixpath
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

SITE_HOSTS = {"engine.speedle.io", "speedle.io"}
USER_AGENT = "Mozilla/5.0 (compatible; SpeedleLinkCheck/1.0)"


class Parser(html.parser.HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.refs = []          # (tag, attr, url)
        self.ids = set()

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if attrs.get("id"):
            self.ids.add(attrs["id"])
        if tag == "a" and attrs.get("name"):
            self.ids.add(attrs["name"])
        for attr in ("href", "src"):
            value = attrs.get(attr)
            if value:
                self.refs.append((tag, attr, value.strip()))


def url_for_file(root, file):
    rel = os.path.relpath(file, root).replace(os.sep, "/")
    if rel.endswith("index.html"):
        rel = rel[: -len("index.html")]
    return "/" + rel


def target_file(root, url_path):
    """Resolve a site path to an existing file, or return None."""
    path = urllib.parse.unquote(url_path)
    if not path.startswith("/"):
        path = "/" + path
    norm = posixpath.normpath(path)
    if norm.startswith("/.."):
        return None
    candidates = []
    if norm.endswith("/"):
        candidates.append(norm + "index.html")
    else:
        candidates.append(norm)
        candidates.append(norm + "/index.html")
        if not posixpath.splitext(norm)[1]:
            candidates.append(norm + ".html")
    for candidate in candidates:
        full = os.path.join(root, candidate.lstrip("/"))
        if os.path.isfile(full):
            return full
    return None


def parse_html(file):
    parser = Parser()
    with open(file, encoding="utf-8", errors="replace") as handle:
        parser.feed(handle.read())
    return parser


def check_external(url):
    # Some sites answer HEAD with 404 but GET with 200, so always use GET.
    # Rate-limited registries get three attempts before they count as failures.
    for attempt in range(3):
        request = urllib.request.Request(
            url,
            headers={
                "User-Agent": USER_AGENT,
                "Accept": "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8",
                "Range": "bytes=0-2047",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=25) as response:
                response.read(2048)
                return url, response.status, None
        except urllib.error.HTTPError as error:
            if error.code in (429, 503) and attempt < 2:
                time.sleep(5 * (attempt + 1))
                continue
            return url, error.code, str(error)
        except Exception as error:  # noqa: BLE001 - report the actual failure
            return url, None, str(error)
    return url, None, "no response"


def main():
    args = argparse.ArgumentParser()
    args.add_argument("root")
    args.add_argument("--no-external", dest="external", action="store_false")
    args.add_argument("--jobs", type=int, default=16)
    args.add_argument("--exclude", action="append", default=[])
    options = args.parse_args()
    root = os.path.abspath(options.root)

    pages = []
    for directory, _, files in os.walk(root):
        for name in files:
            if not name.endswith(".html"):
                continue
            file = os.path.join(directory, name)
            rel = os.path.relpath(file, root).replace(os.sep, "/")
            if any(rel.startswith(prefix) for prefix in options.exclude):
                continue
            pages.append(file)
    pages.sort()
    page_data = {file: parse_html(file) for file in pages}

    internal = 0
    anchors = 0
    failures = []
    external_urls = set()
    skipped = 0

    for file in pages:
        here = url_for_file(root, file)
        for tag, attr, raw in page_data[file].refs:
            if raw.startswith(("mailto:", "tel:", "javascript:", "data:")):
                continue
            split = urllib.parse.urlsplit(raw)
            if split.scheme in ("http", "https"):
                if split.netloc in SITE_HOSTS:
                    path = split.path or "/"
                    fragment = split.fragment
                elif split.hostname in ("localhost", "127.0.0.1", "::1") or not raw.isascii():
                    # Loopback examples and historical non-ASCII URLs are not web links.
                    skipped += 1
                    continue
                else:
                    external_urls.add(urllib.parse.urlunsplit((split.scheme, split.netloc, split.path, split.query, "")))
                    continue
            elif split.scheme or raw.startswith("//"):
                continue  # other schemes
            else:
                base = posixpath.dirname(here) + "/" if not here.endswith("/") else here
                path = urllib.parse.urljoin(base, split.path or "")
                fragment = split.fragment
            target = target_file(root, path)
            internal += 1
            if target is None:
                failures.append(("internal", file, raw, "target not found"))
                continue
            if fragment:
                target_parser = page_data.get(target)
                if target_parser is None:
                    continue  # static non-HTML target: no anchors to verify
                anchor = urllib.parse.unquote(fragment)
                anchors += 1
                if anchor not in target_parser.ids:
                    failures.append(("anchor", file, raw, f"#{anchor} not in {os.path.relpath(target, root)}"))

    if options.external and external_urls:
        with concurrent.futures.ThreadPoolExecutor(max_workers=options.jobs) as pool:
            results = list(pool.map(check_external, sorted(external_urls)))
        for url, status, error in results:
            if error is not None or status is None or status >= 400:
                failures.append(("external", "", url, f"{status} {error or ''}".strip()))

    print(f"pages: {len(pages)}")
    print(f"internal links resolved: {internal}")
    print(f"in-page anchors checked: {anchors}")
    print(f"unique external URLs checked: {len(external_urls)}")
    print(f"skipped loopback or non-ASCII example URLs: {skipped}")
    if failures:
        print(f"FAIL: {len(failures)} broken link(s)")
        for kind, file, raw, detail in failures:
            where = os.path.relpath(file, root) if file else "-"
            print(f"  [{kind}] {where}: {raw} ({detail})")
        return 1
    print("PASS: no broken internal links, anchors or external URLs")
    return 0


if __name__ == "__main__":
    sys.exit(main())
