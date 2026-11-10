#!/usr/bin/env python3
"""Keep Hugo's static language routes and navigation consistent, without JS."""
import argparse
import html
from html.parser import HTMLParser
from pathlib import Path
import posixpath
import re
from urllib.parse import unquote, urljoin, urlsplit, urlunsplit


ORIGIN = "https://engine.speedle.io"
# speedle.io redirects to engine.speedle.io, so links to either host are same-site.
HOSTS = {"engine.speedle.io", "speedle.io"}
# The Products menu and the footer family link the three sites; each keeps the reader's language.
FAMILY_HOSTS = {"engine.speedle.io", "cloud.speedle.io", "ai.speedle.io", "speedle.io"}
FAMILY_ROOT = {
    "zh": {"engine.speedle.io": "/zh/", "cloud.speedle.io": "/zh-CN/", "ai.speedle.io": "/zh/"},
    "ja": {"engine.speedle.io": "/ja/", "cloud.speedle.io": "/ja/", "ai.speedle.io": "/ja/"},
    "ko": {"engine.speedle.io": "/ko/", "cloud.speedle.io": "/ko/", "ai.speedle.io": "/ko/"},
    "es": {"engine.speedle.io": "/es/", "cloud.speedle.io": "/es/", "ai.speedle.io": "/es/"},
}
LANGUAGE_PREFIXES = ("/zh-CN", "/zh", "/ja", "/ko", "/es", "/en")
LANGUAGES = ("en", "zh", "ja", "ko", "es")
OWN_ORIGIN = re.compile(r"^https?://(?:engine\.)?speedle\.io(?=/)", re.I)
NAVIGATION_VERSION = "language-direct-20261026"
PREVIEW = re.compile(r"^/[0-9a-f]{32}(?=/)")
HEADER_VERSION = "header-20261106-v8"
# Collapse the mobile menu before first paint so short pages do not shift (#247);
# without scripts the links stay in normal flow and remain usable.
PREPAINT_SCRIPT = ("<script>document.documentElement.classList.add('navigation-js');"
                   "addEventListener('DOMContentLoaded',function(){"
                   "if(!document.querySelector('header.site-header.navigation-ready'))"
                   "document.documentElement.classList.remove('navigation-js')});</script>\n")
HEADER_ASSETS = (PREPAINT_SCRIPT +
                 f'<link href="/css/navigation.css?v={HEADER_VERSION}" rel="stylesheet">\n'
                 f'<script src="/js/navigation.js?v={HEADER_VERSION}" defer></script>\n')
WEB_ANALYTICS = (Path(__file__).resolve().parent / "cloudflare-web-analytics.html").read_text()
BEACON = re.compile(r"<!-- Cloudflare Web Analytics -->[\s\S]*?<!-- End Cloudflare Web Analytics -->")


def web_analytics(text):
    """Install the manual Cloudflare beacon once on each rendered page, replacing an older snippet."""
    if BEACON.search(text):
        return BEACON.sub(lambda _: WEB_ANALYTICS.strip(), text, count=1)
    if "data-cf-beacon=" in text:
        return text
    return re.sub(r"(?im)^([ \t]*)(</body>)",
                  lambda match: match[1] + WEB_ANALYTICS + match[0], text, count=1)


def logical(path):
    path = PREVIEW.sub("", unquote(path))
    if path.endswith("/index.html"):
        path = path[:-10]
    if path == "/en/" or path == "/en":
        return "/"
    if not posixpath.splitext(path)[1] and not path.endswith("/"):
        path += "/"
    return path or "/"


def language(path):
    p = logical(path)
    for lang in ("zh", "ja", "ko", "es"):
        if p == f"/{lang}" or p.startswith(f"/{lang}/"):
            return lang
    return "en"


def snapshot(path):
    match = PREVIEW.match(unquote(path))
    return match[0] if match else ""


def canonical_path(path):
    return snapshot(path) + logical(path)


def counterpart(path, lang):
    prefix = snapshot(path)
    path = logical(path)
    current = language(path)
    if current != "en":
        neutral = path[len(current) + 1 :]
    else:
        neutral = path
    if not neutral.startswith("/"):
        neutral = "/" + neutral
    if lang == "en":
        return prefix + neutral
    return prefix + f"/{lang}" + (neutral if neutral != "/" else "/")


def family_destination(url, lang):
    """Keep Products-menu and footer-family links in the reader's language."""
    path = url.path or "/"
    for prefix in LANGUAGE_PREFIXES:
        if path == prefix or path.startswith(prefix + "/"):
            path = path[len(prefix):] or "/"
            break
    if lang in FAMILY_ROOT and url.netloc in FAMILY_ROOT[lang]:
        root = FAMILY_ROOT[lang][url.netloc]
        path = root if path == "/" else root[:-1] + path
    return urlunsplit((url.scheme, url.netloc, path, url.query, url.fragment))


def direct_language_navigation(text, route):
    """Upgrade old published headers that are outside the current Hugo build."""
    if 'id="langselFloatDiv"' not in text:
        return text
    lang = language(route)
    opposite = "en" if lang == "zh" else "zh"
    code = "en" if opposite == "en" else "zh-CN"
    label = "English" if opposite == "en" else "中文"
    name = "语言" if lang == "zh" else "Language"
    target = counterpart(route, opposite)
    navigation = (f'<nav id="top-nav-lang" aria-label="{name}"><ul><li>'
                  f'<a href="{target}" lang="{code}" hreflang="{code}">{label}</a>'
                  '</li></ul></nav>')

    def upgrade(match):
        header = re.sub(r'<nav\b[^>]*id="top-nav-lang"[^>]*>[\s\S]*?</nav>',
                        lambda _: navigation, match[0], count=1)
        header = re.sub(r'[ \t]*<div\b[^>]*id="langselFloatDiv"[^>]*>[\s\S]*?</div>[ \t]*\n?', '', header, count=1)
        return re.sub(r'[ \t]*<nav\b[^>]*class="langsel show-sm"[^>]*>[\s\S]*?</nav>[ \t]*\n?', '', header, count=1)

    return re.sub(r'<header\b[^>]*>[\s\S]*?</header>', upgrade, text, count=1)


def archive_header(site, text, route):
    """Give archived snapshots the current homepage header.

    Header links keep the snapshot's version where it has the page (and anchor),
    and otherwise lead to the current site. Language switches are resolved
    afterwards: a language this snapshot did not publish is removed.
    """
    prefix = snapshot(route)
    lang = language(route)
    home = "/zh/" if lang == "zh" else "/"
    source = site.root / home.lstrip("/") / "index.html"
    if not prefix or not source.is_file():
        return text
    header = re.search(r'<header class="site-header">[\s\S]*?</header>', source.read_text())
    if not header or not re.search(r"<header\b", text):
        return text

    def destination(match):
        href = match[2]
        if href.startswith("#"):
            href = home + href
        url = urlsplit(href)
        if url.netloc or not url.path.startswith("/"):
            return match[0]
        versioned = prefix + logical(url.path)
        page = site.pages.get(versioned)
        if versioned in site.canonical and page and (
                not url.fragment or f'id="{url.fragment}"' in re.sub(r"<header\b[\s\S]*?</header>", "", page[1])):
            href = urlunsplit(("", "", versioned, url.query, url.fragment))
        return match[1] + href + '"'

    unified = re.sub(r'(<a\b(?![^>]*hreflang)[^>]*\shref=")([^"]*)"', destination, header[0])
    text = re.sub(r"<header\b[^>]*>[\s\S]*?</header>", lambda _: unified, text, count=1)
    # The shared three-site footer is reused as is; its lock-up home link is a local link
    # that the normal link pass keeps inside the snapshot.
    footer = re.search(r'<footer class="site-footer">[\s\S]*?</footer>', source.read_text())
    if footer:
        text = re.sub(r"<footer\b[^>]*>[\s\S]*?</footer>", lambda _: footer[0], text, count=1)
    # Earlier snapshots carry their own copies of the navigation stylesheet and menu script.
    text = re.sub(r'[ \t]*<link rel="stylesheet" href="' + re.escape(prefix) + r'/css/navigation\.css">\n?', "", text)
    text = re.sub(r"[ \t]*<script>\s*const button = document\.querySelector\('\.menu'\);[\s\S]*?</script>\n?", "", text)
    if "/js/navigation.js" not in text:
        end = text.lower().find("</head>")
        if end >= 0:
            text = text[:end] + HEADER_ASSETS + text[end:]
    elif "navigation-js" not in text:
        end = text.lower().find("</head>")
        if end >= 0:
            text = text[:end] + PREPAINT_SCRIPT + text[end:]
    return text


class Metadata(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.redirect = None
        self.feed(text)

    def handle_starttag(self, tag, attributes):
        attrs = dict(attributes)
        if tag == "meta" and attrs.get("http-equiv", "").lower() == "refresh":
            match = re.search(r"url\s*=\s*(.*)", attrs.get("content", ""), re.I)
            if match:
                self.redirect = match[1].strip(" '\"")


class Site:
    def __init__(self, root):
        self.root = Path(root)
        self.pages = {}
        for file in self.root.rglob("*.html"):
            relative = file.relative_to(self.root)
            if relative.parts[0] == "hugo":
                continue
            route = "/" + relative.as_posix()
            if route.endswith("index.html"):
                route = route[:-10]
            text = file.read_text()
            self.pages[route] = (file, text, Metadata(text).redirect)
        self.canonical = {canonical_path(route) for route, (_, _, redirect) in self.pages.items()
                          if not redirect}

    def resolve(self, path, context=""):
        prefix = snapshot(path) or context
        path = logical(path)
        # These old relative destinations predate the /docs/ URL structure.
        path = re.sub(r"^/(zh/)?(?:devops|developer|support)/docs/", lambda m: "/" + (m[1] or "") + "docs/", path)
        path = re.sub(r"^/(zh/)?support/(quick-start|contribute)/", lambda m: "/" + (m[1] or "") + m[2] + "/", path)
        path = re.sub(r"^/(zh/)?integrations/quick-start/", lambda m: "/" + (m[1] or "") + "quick-start/", path)
        path = re.sub(r"^/(zh/)?docs/pms/docs/", lambda m: "/" + (m[1] or "") + "docs/", path)
        path = path.replace("/docs/api/discovery/", "/docs/pms/discover/")
        path = prefix + path
        for _ in range(8):
            page = self.pages.get(path)
            if path in self.canonical:
                return path
            if not page or not page[2]:
                return None
            target = urlsplit(urljoin(ORIGIN + path, page[2]))
            if target.netloc not in HOSTS:
                return None
            path = (snapshot(target.path) or prefix) + logical(target.path)
        return None


def complete_archives(root):
    """Add honest untranslated shells; never substitute current-version content."""
    site = Site(root)
    for route, (_, original, redirect) in list(site.pages.items()):
        prefix = snapshot(route)
        if not prefix or redirect or 'data-language-fallback=' in original:
            continue
        source_lang = language(route)
        target_lang = "en" if source_lang == "zh" else "zh"
        target = counterpart(route, target_lang)
        if target in site.canonical:
            continue
        frame = prefix + ("/zh/docs/" if target_lang == "zh" else "/docs/")
        header = re.search(r"<header\b[^>]*>[\s\S]*?</header>", site.pages[frame][1], re.I)
        if not header:
            raise SystemExit(f"Missing archived navigation frame: {frame}")
        result = re.sub(r"<header\b[^>]*>[\s\S]*?</header>", lambda _: header[0], original, count=1, flags=re.I)
        result = re.sub(r'(<html\b[^>]*\blang=)["\'][^"\']+["\']',
                        lambda m: m[1] + '"' + ("zh-CN" if target_lang == "zh" else "en") + '"', result, count=1, flags=re.I)
        notice = ("此历史版本的页面尚无中文译文。以下保留该版本的英文原文。" if target_lang == "zh" else
                  "This archived page has no English translation. The original Chinese text from this version is shown below.")
        source_code = "zh-CN" if source_lang == "zh" else "en"
        target_code = "zh-CN" if target_lang == "zh" else "en"
        result = re.sub(r"<main\b[^>]*>", lambda m: m[0][:-1] + f' data-language-fallback="{source_lang}" lang="{source_code}"><p role="note" lang="{target_code}">{notice}</p>', result, count=1, flags=re.I)
        result = re.sub(r'(<link\b[^>]*rel=["\']canonical["\'][^>]*href=)["\'][^"\']+["\']',
                        lambda m: m[1] + '"' + ORIGIN + target + '"', result, count=1, flags=re.I)
        file = site.root / target.lstrip("/") / "index.html"
        file.parent.mkdir(parents=True, exist_ok=True)
        file.write_text(result)


class Links(HTMLParser):
    def __init__(self, site, route, text):
        super().__init__(convert_charrefs=False)
        self.site, self.route, self.text = site, canonical_path(route), text
        self.prefix = snapshot(route)
        self.base_route = route
        self.lang = language(route)
        self.offsets = [0]
        for match in re.finditer("\n", text):
            self.offsets.append(match.end())
        self.stack, self.edits, self.errors = [], [], []
        self.switches = set()
        self.drop_anchor = None
        self.feed(text)
        if PREVIEW.match(route) and 'class="nav-links"' in text and '/css/navigation.css' not in text:
            end = text.lower().find('</head>')
            if end >= 0:
                self.edits.append((end, end, f'<link rel="stylesheet" href="{self.prefix}/css/navigation.css">\n'))

    def handle_starttag(self, tag, attributes):
        attrs = dict(attributes)
        raw = self.get_starttag_text()
        changes = {}
        in_languages = any("languages" in scope or "langsel" in scope for _, scope in self.stack)
        # The Products menu and footer family link the three sites by absolute URL:
        # localize them to the reader's language instead of resolving them as local links.
        cross_site = any(name in scope for _, scope in self.stack for name in ("products-panel", "footer-family"))
        if tag == "a" and attrs.get("href"):
            href = attrs["href"].strip()
            url = urlsplit(urljoin(ORIGIN + self.base_route, href))
            if cross_site and urlsplit(href).netloc:
                if url.netloc in FAMILY_HOSTS and url.scheme in {"http", "https"}:
                    changes["href"] = family_destination(url, self.lang)
            elif url.netloc in HOSTS and url.scheme in {"http", "https"}:
                switch = attrs.get("hreflang", "").lower().split("-")[0]
                if in_languages and not switch:
                    switch = language(logical(url.path))
                if switch in LANGUAGES:
                    target = counterpart(self.route, switch)
                    # An archived snapshot only offers the languages published with
                    # that version. Omit a later language instead of sending the
                    # reader to the current site without saying the version changed.
                    if target not in self.site.canonical and self.prefix:
                        if self.drop_anchor is None:
                            line, column = self.getpos()
                            self.drop_anchor = self.offsets[line - 1] + column
                    elif target not in self.site.canonical:
                        self.errors.append(f"{self.route}: missing translation {target}")
                    else:
                        code = "zh-CN" if switch == "zh" else switch
                        changes.update(href=target, lang=code, hreflang=code)
                        if "aria-label" in attrs:
                            labels = {
                                "zh": "切换到中文",
                                "en": "Switch to English",
                                "ja": "日本語に切り替える",
                                "ko": "한국어로 변경",
                                "es": "Cambiar a español",
                            }
                            changes["aria-label"] = labels.get(switch, "")
                        self.switches.add(switch)
                elif self.prefix and any("site-header" in scope for _, scope in self.stack):
                    if canonical_path(url.path) not in self.site.canonical:
                        self.errors.append(f"{self.route}: broken header link {href}")
                elif not href.startswith("#"):
                    destination = self.site.resolve(url.path, self.prefix)
                    if destination:
                        target = counterpart(destination, self.lang)
                        if target not in self.site.canonical:
                            self.errors.append(f"{self.route}: missing localized destination {target}")
                        else:
                            changes["href"] = urlunsplit(("", "", target, url.query, url.fragment))
                    elif OWN_ORIGIN.match(href) and (self.site.root / unquote(url.path.lstrip("/"))).is_file():
                        changes["href"] = urlunsplit(("", "", url.path, url.query, url.fragment))
                    elif not (self.site.root / unquote(url.path.lstrip("/"))).is_file():
                        self.errors.append(f"{self.route}: broken local link {href}")
        # Normalize Hugo's same-origin resource URLs without changing metadata.
        resource = "src" if tag in {"img", "script", "source", "iframe"} else "href" if tag == "link" and attrs.get("rel") in {"stylesheet", "icon", "apple-touch-icon"} else None
        if resource and OWN_ORIGIN.match(attrs.get(resource, "")):
            changes[resource] = OWN_ORIGIN.sub("", attrs[resource], count=1)
        if tag in {"img", "source"} and attrs.get("srcset"):
            normalized = OWN_ORIGIN.sub("", attrs["srcset"])
            if normalized != attrs["srcset"]:
                changes["srcset"] = normalized
        if tag == "script" and attrs.get("src"):
            script = urlsplit(urljoin(ORIGIN + self.base_route, attrs["src"]))
            if script.netloc in HOSTS and script.path.endswith("/js/master.js"):
                changes["src"] = script.path + "?v=" + NAVIGATION_VERSION
        if tag == "meta" and attrs.get("http-equiv", "").lower() == "refresh":
            destination = self.site.resolve(urlsplit(urljoin(ORIGIN + self.base_route, Metadata(raw).redirect or "")).path, self.prefix)
            if destination:
                changes["content"] = "0; url=" + counterpart(destination, self.lang)
        updated = raw
        for name, value in changes.items():
            if attrs.get(name) == value:
                continue
            pattern = re.compile(r"(?P<prefix>\s" + re.escape(name) + r"\s*=\s*)(?P<quote>['\"])(.*?)\2", re.I | re.S)
            if pattern.search(updated):
                updated = pattern.sub(lambda m: m["prefix"] + m["quote"] + html.escape(value, quote=True) + m["quote"], updated, count=1)
            else:
                end = "/>" if updated.endswith("/>") else ">"
                updated = updated[:-len(end)] + f' {name}="{html.escape(value, quote=True)}"' + end
        if updated != raw:
            line, column = self.getpos()
            start = self.offsets[line - 1] + column
            self.edits.append((start, start + len(raw), updated))
        if tag not in {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr", "use"}:
            self.stack.append((tag, attrs.get("id", "") + " " + attrs.get("class", "")))

    def handle_endtag(self, tag):
        if tag == "a" and self.drop_anchor is not None:
            line, column = self.getpos()
            end_start = self.offsets[line - 1] + column
            close = self.text.find(">", end_start)
            if close < 0 or not self.text[end_start:close + 1].lower().startswith("</a"):
                self.errors.append(f"{self.route}: could not remove unpublished language switch")
            elif self.drop_anchor < close:
                self.edits.append((self.drop_anchor, close + 1, ""))
            self.drop_anchor = None
        opposite = "en" if self.lang == "zh" else "zh"
        if tag == "nav" and self.stack and "nav-links" in self.stack[-1][1] and opposite not in self.switches:
            target = counterpart(self.route, opposite)
            label = "中文" if opposite == "zh" else "English"
            code = "zh-CN" if opposite == "zh" else "en"
            line, column = self.getpos()
            start = self.offsets[line - 1] + column
            self.edits.append((start, start, f'<a href="{target}" lang="{code}" hreflang="{code}">{label}</a>'))
            self.switches.add(opposite)
        for index in range(len(self.stack) - 1, -1, -1):
            if self.stack[index][0] == tag:
                del self.stack[index:]
                break

    def output(self):
        result = self.text
        for start, end, replacement in sorted(self.edits, reverse=True):
            result = result[:start] + replacement + result[end:]
        return result


def process(root, check=False):
    if not check:
        complete_archives(root)
    site = Site(root)
    errors, changed = [], 0
    for route, (file, text, redirect) in site.pages.items():
        links = Links(site, route, archive_header(site, direct_language_navigation(text, route), route))
        errors.extend(links.errors)
        if not redirect:
            # Each published snapshot keeps the languages it actually contains.
            # Current pages require every language present on the current site.
            prefix = snapshot(route)
            active_languages = {language(item) for item in site.canonical if snapshot(item) == prefix}
            for other in LANGUAGES:
                if other in active_languages and other != language(route):
                    target = counterpart(route, other)
                    if target not in site.canonical:
                        errors.append(f"{route}: no matching {other} page")
                    if other not in links.switches:
                        errors.append(f"{route}: no same-page {other} language switch")
        output = links.output()
        if not redirect:
            output = web_analytics(output)
        if output != text:
            changed += 1
            if not check:
                file.write_text(output)
    if errors:
        raise SystemExit("\n".join(sorted(set(errors))))
    if check and changed:
        raise SystemExit(f"{changed} pages still contain language-routing changes")
    print(f"PASS language routes: {len(site.pages)} HTML pages, {len(site.canonical)} canonical pages, {changed} updated")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    process(args.directory, args.check)
