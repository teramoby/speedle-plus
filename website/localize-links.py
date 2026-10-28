#!/usr/bin/env python3
"""Keep Hugo's static language routes and navigation consistent, without JS."""
import argparse
import html
from html.parser import HTMLParser
from pathlib import Path
import posixpath
import re
from urllib.parse import unquote, urljoin, urlsplit, urlunsplit


ORIGIN = "https://speedle.io"
NAVIGATION_VERSION = "language-direct-20261026"
PREVIEW = re.compile(r"^/[0-9a-f]{32}(?=/)")
HEADER_VERSION = "header-20261027"
HEADER_ASSETS = (f'<link href="/css/navigation.css?v={HEADER_VERSION}" rel="stylesheet">\n'
                 f'<script src="/js/navigation.js?v={HEADER_VERSION}" defer></script>\n')


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
    return "zh" if logical(path).startswith("/zh/") else "en"


def snapshot(path):
    match = PREVIEW.match(unquote(path))
    return match[0] if match else ""


def canonical_path(path):
    return snapshot(path) + logical(path)


def counterpart(path, lang):
    prefix = snapshot(path)
    path = logical(path)
    neutral = path[3:] if path.startswith("/zh/") else path
    return prefix + ("/zh" + neutral if lang == "zh" else neutral)


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
    and otherwise lead to the current site.
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
    # Earlier snapshots carry their own copies of the navigation stylesheet and menu script.
    text = re.sub(r'[ \t]*<link rel="stylesheet" href="' + re.escape(prefix) + r'/css/navigation\.css">\n?', "", text)
    text = re.sub(r"[ \t]*<script>\s*const button = document\.querySelector\('\.menu'\);[\s\S]*?</script>\n?", "", text)
    if "/js/navigation.js" not in text:
        end = text.lower().find("</head>")
        if end >= 0:
            text = text[:end] + HEADER_ASSETS + text[end:]
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
            if target.netloc != "speedle.io":
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
        self.feed(text)
        if PREVIEW.match(route) and 'class="nav-links"' in text and '/css/navigation.css' not in text:
            end = text.lower().find('</head>')
            if end >= 0:
                self.edits.append((end, end, f'<link rel="stylesheet" href="{self.prefix}/css/navigation.css">\n'))

    def handle_starttag(self, tag, attributes):
        attrs = dict(attributes)
        raw = self.get_starttag_text()
        changes = {}
        in_languages = any("langsel" in scope for _, scope in self.stack)
        if tag == "a" and attrs.get("href"):
            href = attrs["href"].strip()
            url = urlsplit(urljoin(ORIGIN + self.base_route, href))
            if url.netloc == "speedle.io" and url.scheme in {"http", "https"}:
                switch = attrs.get("hreflang", "").lower().split("-")[0]
                if in_languages and not switch:
                    switch = "zh" if logical(url.path).startswith("/zh/") else "en"
                if switch in {"en", "zh"}:
                    target = counterpart(self.route, switch)
                    if target not in self.site.canonical:
                        self.errors.append(f"{self.route}: missing translation {target}")
                    else:
                        changes.update(href=target, lang="zh-CN" if switch == "zh" else "en",
                                       hreflang="zh-CN" if switch == "zh" else "en")
                        if "aria-label" in attrs:
                            changes["aria-label"] = "切换到中文" if switch == "zh" else "Switch to English"
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
                    elif href.startswith(ORIGIN + "/") and (self.site.root / unquote(url.path.lstrip("/"))).is_file():
                        changes["href"] = urlunsplit(("", "", url.path, url.query, url.fragment))
                    elif not (self.site.root / unquote(url.path.lstrip("/"))).is_file():
                        self.errors.append(f"{self.route}: broken local link {href}")
        # Normalize Hugo's same-origin resource URLs without changing metadata.
        resource = "src" if tag in {"img", "script", "source", "iframe"} else "href" if tag == "link" and attrs.get("rel") == "stylesheet" else None
        if resource and attrs.get(resource, "").startswith(ORIGIN + "/"):
            changes[resource] = attrs[resource][len(ORIGIN):]
        if tag == "script" and attrs.get("src"):
            script = urlsplit(urljoin(ORIGIN + self.base_route, attrs["src"]))
            if script.netloc == "speedle.io" and script.path.endswith("/js/master.js"):
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
            opposite = "en" if language(route) == "zh" else "zh"
            if counterpart(route, opposite) not in site.canonical:
                errors.append(f"{route}: no matching {opposite} page")
            if opposite not in links.switches:
                errors.append(f"{route}: no same-page {opposite} language switch")
        output = links.output()
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
