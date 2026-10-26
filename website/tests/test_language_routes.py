"""Exercise the actual static language router, including legacy URLs."""
import contextlib
import io
from pathlib import Path
import runpy
import tempfile
import unittest

ROUTER = runpy.run_path(str(Path(__file__).resolve().parents[1] / "localize-links.py"))


class LanguageRoutes(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for path in ["", "docs/spdl", "docs/store", "categories/docs/page/2"]:
            for prefix in ["", "zh/"]:
                route = "/" + prefix + path + ("/" if path else "")
                other = "en" if prefix else "zh-CN"
                target = "/" if prefix else "/zh/"
                self.write(route, f'<html><nav class="nav-links"><a href="{target}" hreflang="{other}">Language</a></nav>'
                           '<a href="/docs/store/?source=guide&amp;n=1#section">Document</a>'
                           '<a href="https://github.com/teramoby/speedle-plus">External</a>'
                           '<a href="/policy.proto">Download</a><a href="#local">Local</a>'
                           '<div id="local">Body and code stay unchanged.</div></html>')
        (self.root / "policy.proto").write_text("syntax = 'proto3';")

    def write(self, route, text):
        file = self.root / route.lstrip("/") / "index.html"
        file.parent.mkdir(parents=True, exist_ok=True)
        file.write_text(text)

    def process(self, check=False):
        with contextlib.redirect_stdout(io.StringIO()):
            ROUTER["process"](self.root, check)

    def test_same_page_switch_and_language_preserving_links(self):
        self.process()
        text = (self.root / "zh/docs/spdl/index.html").read_text()
        self.assertIn('href="/docs/spdl/" hreflang="en"', text)
        self.assertIn('href="/zh/docs/store/?source=guide&amp;n=1#section"', text)
        self.assertIn('href="/policy.proto"', text)
        self.assertIn('href="#local"', text)
        self.assertIn('href="https://github.com/teramoby/speedle-plus"', text)
        self.assertIn('Body and code stay unchanged.', text)

    def test_pagination_switch_does_not_return_to_the_first_page(self):
        self.process()
        text = (self.root / "categories/docs/page/2/index.html").read_text()
        self.assertIn('href="/zh/categories/docs/page/2/"', text)

    def test_legacy_dropdown_and_preview_keep_the_same_logical_page(self):
        route = "/" + "a" * 32 + "/zh/docs/spdl/"
        self.write(route, '<html><div class="langsel"><span><img src="/x.svg"/></span>'
                   '<a href=" /en">English</a><a href=" /zh">中文</a></div>'
                   '<a href="/devops/docs/store/">Store</a></html>')
        self.process()
        text = (self.root / route.lstrip("/") / "index.html").read_text()
        self.assertIn('href="/docs/spdl/" lang="en" hreflang="en"', text)
        self.assertIn('href="/zh/docs/store/"', text)

    def test_missing_translation_and_broken_links_fail_the_gate(self):
        (self.root / "zh/docs/store/index.html").unlink()
        with self.assertRaisesRegex(SystemExit, "missing|no matching"):
            self.process(check=True)

    def test_idempotence_and_check_mode(self):
        self.process()
        before = {p: p.read_bytes() for p in self.root.rglob("*.html")}
        self.process(check=True)
        self.assertEqual(before, {p: p.read_bytes() for p in self.root.rglob("*.html")})

    def test_unknown_internal_page_is_reported_without_guessing_a_destination(self):
        self.write("/docs/spdl/", '<html><nav class="nav-links"><a href="/zh/" hreflang="zh">中文</a></nav><a href="/missing/">Missing</a></html>')
        with self.assertRaisesRegex(SystemExit, "broken local link /missing/"):
            self.process(check=True)


if __name__ == "__main__":
    unittest.main()
