"""Exercise the actual static language router, including legacy URLs."""
import contextlib
import io
from pathlib import Path
import re
import runpy
import shutil
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
                self.write(route, f'<html lang="{lang}"><head><link rel="canonical" href="https://engine.speedle.io{route}"></head>'
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
                       '<main id="top"><section id="about"></section></main>'
                       '<footer class="site-footer"><p>Shared footer</p></footer></body></html>')
        self.write(prefix + "/", '<html><head></head><body><header>Old</header><main id="top">Archived home.</main><footer class="row">Old footer</footer></body></html>')
        self.write(prefix + "/zh/", '<html><head></head><body><header id="about">Old</header><main id="top"></main></body></html>')
        self.process()
        text = (self.root / prefix.lstrip("/") / "zh/docs/spdl/index.html").read_text()
        self.assertIn(f'<a class="brand" href="{prefix}/zh/#top">', text)
        self.assertIn('<a href="/zh/#about">About</a>', text)
        self.assertIn(f'<a href="{prefix}/zh/docs/store/">文档</a>', text)
        self.assertIn(f'<a href="{prefix}/docs/spdl/" lang="en" hreflang="en">English</a>', text)
        self.assertIn('/js/navigation.js?v=header-20261106-v8', text)
        self.assertIn('Original archived content.', text)
        self.assertNotIn('langsel', text)
        english = (self.root / prefix.lstrip("/") / "index.html").read_text()
        self.assertIn('<a href="/#about">About</a>', english)
        self.assertIn('Archived home.', english)
        self.assertIn('<footer class="site-footer"><p>Shared footer</p></footer>', english)
        self.assertNotIn('Old footer', english)
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
        self.write(route, f'<html lang="en"><head><link rel="canonical" href="https://engine.speedle.io{route}"></head>'
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

    def test_cross_site_family_links_keep_the_page_language(self):
        menu = ('<div class="products-panel" id="products-panel">'
                '<a href="https://engine.speedle.io/">Engine</a>'
                '<a href="https://cloud.speedle.io/">Cloud</a>'
                '<a href="https://ai.speedle.io/">AI</a>'
                '<a href="https://cloud.speedle.io/pricing/">Cloud pricing</a></div>')
        footer = ('<div class="footer-family"><a href="https://engine.speedle.io/">E</a>'
                  '<a href="https://cloud.speedle.io/">C</a><a href="https://ai.speedle.io/">A</a></div>')
        content = '<a href="https://cloud.speedle.io/">Content link stays as written</a>'
        self.write('/zh/docs/spdl/', '<html><nav class="nav-links">'
                   '<a href="/docs/spdl/" hreflang="en">English</a></nav>' + menu + footer + content + '</html>')
        self.write('/docs/spdl/', '<html><nav class="nav-links">'
                   '<a href="/zh/docs/spdl/" hreflang="zh-CN">中文</a></nav>'
                   '<div class="products-panel" id="products-panel"><a href="https://engine.speedle.io/zh/">Engine</a>'
                   '<a href="https://cloud.speedle.io/zh-CN/">Cloud</a><a href="https://ai.speedle.io/zh/">AI</a></div></html>')
        self.process()
        chinese = (self.root / 'zh/docs/spdl/index.html').read_text()
        for url in ['https://engine.speedle.io/zh/', 'https://cloud.speedle.io/zh-CN/', 'https://ai.speedle.io/zh/']:
            self.assertIn(f'href="{url}"', chinese)
        self.assertIn('href="https://cloud.speedle.io/zh-CN/pricing/"', chinese)
        self.assertIn(content, chinese)
        english = (self.root / 'docs/spdl/index.html').read_text()
        for url in ['https://engine.speedle.io/', 'https://cloud.speedle.io/', 'https://ai.speedle.io/']:
            self.assertIn(f'href="{url}"', english)
        self.assertNotIn('/zh-CN/', english)
        self.assertNotIn('https://engine.speedle.io/zh/', english)
        self.process(check=True)

    def test_multilingual_counterparts_and_family_links(self):
        for lang in ['ja', 'ko', 'es']:
            route = f'/{lang}/docs/spdl/'
            self.assertEqual(ROUTER['language'](route), lang)
            self.assertEqual(ROUTER['counterpart'](route, 'en'), '/docs/spdl/')
            self.assertEqual(ROUTER['counterpart']('/docs/spdl/', lang), route)
            dest = ROUTER['family_destination'](ROUTER['urlsplit']('https://cloud.speedle.io/'), lang)
            self.assertEqual(dest, f'https://cloud.speedle.io/{lang}/')
            dest_ai = ROUTER['family_destination'](ROUTER['urlsplit']('https://ai.speedle.io/'), lang)
            self.assertEqual(dest_ai, f'https://ai.speedle.io/{lang}/')

    def test_five_language_pages_keep_links_and_menu_order(self):
        root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, root)
        labels = [("en", "English", ""), ("ja", "日本語", "ja/"), ("ko", "한국어", "ko/"),
                  ("es", "Español", "es/"), ("zh-CN", "中文", "zh/")]

        def write(route, text):
            file = root / route.lstrip("/") / "index.html"
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text(text)

        for code, label, prefix in labels:
            row = "".join(
                f'<a class="language-row" href="/{other_prefix}docs/spdl/" hreflang="{other_code}">{other_label}</a>'
                for other_code, other_label, other_prefix in labels
            )
            menu = ('<div class="products-panel" id="products-panel">'
                    '<a href="https://engine.speedle.io/">Engine</a>'
                    '<a href="https://cloud.speedle.io/">Cloud</a>'
                    '<a href="https://ai.speedle.io/">AI</a></div>')
            write(f"/{prefix}docs/spdl/", f'<html><nav class="nav-links">{row}</nav>{menu}'
                  '<a href="/docs/store/?source=guide&amp;n=1#section">Store</a></html>')
            write(f"/{prefix}docs/store/", f'<html><nav class="nav-links">{row}</nav><main>Store</main></html>')
        with contextlib.redirect_stdout(io.StringIO()):
            ROUTER["process"](root, False)
        for code, label, prefix in labels:
            text = (root / f"{prefix}docs/spdl/index.html").read_text()
            self.assertEqual(
                re.findall(r'class="language-row"[^>]*hreflang="([^"]+)"[^>]*>([^<]+)', text),
                [(other_code, other_label) for other_code, other_label, _ in labels],
            )
            self.assertIn(f'href="/{prefix}docs/store/?source=guide&amp;n=1#section"', text)
            lang = "zh" if code == "zh-CN" else code
            if lang == "en":
                self.assertIn('href="https://cloud.speedle.io/"', text)
                self.assertNotIn('https://cloud.speedle.io/ja/', text)
            elif lang == "zh":
                self.assertIn('href="https://cloud.speedle.io/zh-CN/"', text)
                self.assertIn('href="https://ai.speedle.io/zh/"', text)
            else:
                self.assertIn(f'href="https://cloud.speedle.io/{lang}/"', text)
                self.assertIn(f'href="https://ai.speedle.io/{lang}/"', text)
        with contextlib.redirect_stdout(io.StringIO()):
            ROUTER["process"](root, True)

    def test_archived_snapshots_do_not_require_later_languages(self):
        root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, root)
        labels = [("en", ""), ("ja", "ja/"), ("ko", "ko/"), ("es", "es/"), ("zh-CN", "zh/")]

        def write(route, text):
            file = root / route.lstrip("/") / "index.html"
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text(text)

        row = "".join(
            f'<a class="language-row" href="/{prefix}" hreflang="{code}">Lang</a>'
            for code, prefix in labels
        )
        header = (f'<header class="site-header"><nav class="nav-links">{row}'
                  '<a href="/docs/spdl/">SPDL</a></nav></header>')
        for code, prefix in labels:
            write(f"/{prefix}", f'<html><head></head><body>{header}<main id="top"></main></body></html>')
            write(f"/{prefix}docs/spdl/", f'<html>{header}<main>Current</main></html>')
        prefix = "/" + "a" * 32
        for lang in ("en", "zh"):
            for path in ("/docs/spdl/", "/blog/"):
                route = prefix + ("/zh" if lang == "zh" else "") + path
                write(route, f'<html lang="{lang}"><head><link rel="canonical" href="https://engine.speedle.io{route}"></head>'
                      '<header><div class="langsel"><a href="/en/">English</a><a href="/zh/">中文</a></div></header>'
                      '<main>Archived</main></html>')
        with contextlib.redirect_stdout(io.StringIO()):
            ROUTER["process"](root, False)
        archived = (root / prefix.lstrip("/") / "docs/spdl/index.html").read_text()
        self.assertIn(f'href="{prefix}/zh/docs/spdl/"', archived)
        self.assertIn('href="/ja/docs/spdl/"', archived)
        self.assertNotIn(f'{prefix}/ja/', archived)
        self.assertIn("Archived", archived)
        blog = (root / prefix.lstrip("/") / "blog/index.html").read_text()
        self.assertIn('href="/ja/"', blog)
        self.assertNotIn(f'{prefix}/ja/', blog)
        with contextlib.redirect_stdout(io.StringIO()):
            ROUTER["process"](root, True)

    def test_unknown_internal_page_is_reported_without_guessing_a_destination(self):
        self.write("/docs/spdl/", '<html><nav class="nav-links"><a href="/zh/" hreflang="zh">中文</a></nav><a href="/missing/">Missing</a></html>')
        with self.assertRaisesRegex(SystemExit, "broken local link /missing/"):
            self.process(check=True)


if __name__ == "__main__":
    unittest.main()
