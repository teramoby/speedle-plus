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

    def test_homepage_section_links_keep_the_language_and_fragment(self):
        self.write('/zh/docs/spdl/', '<html><nav class="nav-links"><a href="/#features">Features</a>'
                   '<a href="/" hreflang="en">English</a></nav><main>Unchanged</main></html>')
        self.process()
        text = (self.root / 'zh/docs/spdl/index.html').read_text()
        self.assertIn('href="/zh/#features"', text)
        self.assertIn('href="/docs/spdl/" hreflang="en"', text)

    def test_published_headers_have_one_direct_opposite_language_link(self):
        prefix = self.archive()
        routes = ['/docs/spdl/', '/zh/docs/spdl/', prefix + '/docs/spdl/', prefix + '/zh/docs/spdl/']
        old_header = ('<header><nav id="top-nav-lang" class="hide-sm"><ul><li><span>Current language</span></li></ul></nav>'
                      '<div class="langsel hide-sm" id="langselFloatDiv"><ul><li><a href="/zh/">中文</a></li></ul></div>'
                      '<nav class="langsel show-sm"><a href="/">English</a><a href="/zh/">中文</a></nav></header>')
        for route in routes:
            self.write(route, '<html>' + old_header + '<main>Unchanged content.</main></html>')
        self.process()
        for route in routes:
            with self.subTest(route=route):
                text = (self.root / route.lstrip('/') / 'index.html').read_text()
                opposite = 'en' if ROUTER['language'](route) == 'zh' else 'zh'
                target = ROUTER['counterpart'](route, opposite)
                label = 'English' if opposite == 'en' else '中文'
                code = 'en' if opposite == 'en' else 'zh-CN'
                self.assertIn(f'<a href="{target}" lang="{code}" hreflang="{code}">{label}</a>', text)
                self.assertEqual(text.count('<a '), 1)
                self.assertNotIn('langsel', text)
                self.assertNotIn('hide-sm', text)
                self.assertIn('<main>Unchanged content.</main>', text)
        self.process(check=True)

    def archive(self):
        prefix = "/" + "a" * 32
        for path in ["/docs/", "/docs/spdl/", "/docs/store/", "/blog/"]:
            for lang in ["en", "zh"]:
                route = prefix + ("/zh" if lang == "zh" else "") + path
                self.write(route, f'<html lang="{lang}"><head><link rel="canonical" href="https://speedle.io{route}"></head>'
                           '<header><div class="langsel"><a href="/en/">English</a><a href="/zh/">中文</a></div></header>'
                           '<main>Original archived content.</main></html>')
        return prefix

    def test_legacy_dropdown_and_archive_keep_the_version_and_logical_page(self):
        prefix = self.archive()
        route = prefix + "/zh/docs/spdl/"
        self.write(route, '<html><div class="langsel"><span><img src="/x.svg"/></span>'
                   '<a href=" /en">English</a><a href=" /zh">中文</a></div>'
                   '<a href="/devops/docs/store/">Store</a></html>')
        self.process()
        text = (self.root / route.lstrip("/") / "index.html").read_text()
        self.assertIn(f'href="{prefix}/docs/spdl/" lang="en" hreflang="en"', text)
        self.assertIn(f'href="{prefix}/zh/docs/store/"', text)

    def test_archives_use_the_current_homepage_header(self):
        prefix = self.archive()
        for home, label in [("/", "Docs"), ("/zh/", "文档")]:
            self.write(home, '<html><head></head><body><header class="site-header"><a class="brand" href="#top">Speedle</a>'
                       f'<nav class="nav-links"><a href="#about">About</a><a href="{home}docs/store/">{label}</a>'
                       f'<a href="{home}docs/spdl/">SPDL</a><a href="{home}">Home</a></nav></header>'
                       '<main id="top"><section id="about"></section></main></body></html>')
        self.write(prefix + "/", '<html><head></head><body><header>Old</header><main id="top">Archived home.</main></body></html>')
        self.write(prefix + "/zh/", '<html><head></head><body><header id="about">Old</header><main id="top"></main></body></html>')
        self.process()
        text = (self.root / prefix.lstrip("/") / "zh/docs/spdl/index.html").read_text()
        self.assertIn(f'<a class="brand" href="{prefix}/zh/#top">', text)
        self.assertIn('<a href="/zh/#about">About</a>', text)
        self.assertIn(f'<a href="{prefix}/zh/docs/store/">文档</a>', text)
        self.assertIn(f'<a href="{prefix}/docs/spdl/" lang="en" hreflang="en">English</a>', text)
        self.assertIn('/js/navigation.js?v=header-20261027', text)
        self.assertIn('Original archived content.', text)
        self.assertNotIn('langsel', text)
        english = (self.root / prefix.lstrip("/") / "index.html").read_text()
        self.assertIn('<a href="/#about">About</a>', english)
        self.assertIn('Archived home.', english)
        self.process(check=True)

    def test_archived_pagination_redirect_retains_the_snapshot(self):
        prefix = self.archive()
        self.write(prefix + "/blog/page/1/", '<html><head><meta http-equiv="refresh" content="0; url=/blog/">'
                   f'<link rel="canonical" href="{prefix}/blog/"></head></html>')
        self.process()
        text = (self.root / prefix.lstrip("/") / "blog/page/1/index.html").read_text()
        self.assertIn(f'content="0; url={prefix}/blog/"', text)
        self.assertIn(f'href="{prefix}/blog/"', text)
        self.process(check=True)

    def test_missing_archive_translation_keeps_original_content_with_a_notice(self):
        prefix = self.archive()
        route = prefix + "/blog/legacy/"
        body = '<main><h1>Historical policy</h1><pre>old_version()</pre></main>'
        self.write(route, f'<html lang="en"><head><link rel="canonical" href="https://speedle.io{route}"></head>'
                   '<header><div class="langsel"><a href="/zh/">中文</a></div></header>' + body + '</html>')
        self.process()
        translated = (self.root / prefix.lstrip("/") / "zh/blog/legacy/index.html").read_text()
        self.assertIn('lang="zh-CN"', translated)
        self.assertIn('data-language-fallback="en" lang="en"', translated)
        self.assertIn('此历史版本的页面尚无中文译文', translated)
        self.assertIn('<h1>Historical policy</h1><pre>old_version()</pre>', translated)
        self.assertIn(f'href="{prefix}/blog/legacy/"', translated)
        self.assertEqual(1, translated.count('data-language-fallback='))
        self.process(check=True)

    def test_check_mode_reports_missing_archive_translation_without_creating_it(self):
        prefix = self.archive()
        target = self.root / prefix.lstrip("/") / "zh/blog/index.html"
        target.unlink()
        with self.assertRaisesRegex(SystemExit, "missing|no matching"):
            self.process(check=True)
        self.assertFalse(target.exists())

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
