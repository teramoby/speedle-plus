#!/usr/bin/env python3
"""Keep Hugo's static language routes and navigation consistent, without JS."""
import argparse
import html
from html.parser import HTMLParser
import json
from pathlib import Path
import posixpath
import re
from urllib.parse import unquote, urljoin, urlsplit, urlunsplit


ORIGIN = "https://speedle.io"
NAVIGATION_VERSION = "language-20261026"
PREVIEW = re.compile(r"^/[0-9a-f]{32}(?=/)")


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


def counterpart(path, lang):
    path = logical(path)
    neutral = path[3:] if path.startswith("/zh/") else path
    return "/zh" + neutral if lang == "zh" else neutral


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
            if relative.parts[0] in {"hugo", "beta"}:
                continue
            route = "/" + relative.as_posix()
            if route.endswith("index.html"):
                route = route[:-10]
            text = file.read_text()
            self.pages[route] = (file, text, Metadata(text).redirect)
        self.canonical = {logical(route) for route, (_, _, redirect) in self.pages.items()
                          if not redirect and not PREVIEW.match(route)}

    def resolve(self, path):
        path = logical(path)
        # These old relative destinations predate the /docs/ URL structure.
        path = re.sub(r"^/(zh/)?(?:devops|developer|support)/docs/", lambda m: "/" + (m[1] or "") + "docs/", path)
        path = re.sub(r"^/(zh/)?support/(quick-start|contribute)/", lambda m: "/" + (m[1] or "") + m[2] + "/", path)
        path = re.sub(r"^/(zh/)?docs/pms/docs/", lambda m: "/" + (m[1] or "") + "docs/", path)
        path = path.replace("/docs/api/discovery/", "/docs/pms/discover/")
        for _ in range(8):
            page = self.pages.get(path)
            if path in self.canonical:
                return path
            if not page or not page[2]:
                return None
            target = urlsplit(urljoin(ORIGIN + path, page[2]))
            if target.netloc != "speedle.io":
                return None
            path = logical(target.path)
        return None


class Links(HTMLParser):
    def __init__(self, site, route, text):
        super().__init__(convert_charrefs=False)
        self.site, self.route, self.text = site, logical(route), text
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
                self.edits.append((end, end, '<link rel="stylesheet" href="/css/navigation.css">\n'))

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
                elif not href.startswith("#"):
                    destination = self.site.resolve(url.path)
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
            destination = self.site.resolve(urlsplit(urljoin(ORIGIN + self.route, Metadata(raw).redirect or "")).path)
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
    site = Site(root)
    errors, changed = [], 0
    for route, (file, text, redirect) in site.pages.items():
        links = Links(site, route, text)
        errors.extend(links.errors)
        if not redirect:
            opposite = "en" if language(route) == "zh" else "zh"
            if counterpart(route, opposite) not in site.canonical:
                errors.append(f"{route}: no matching {opposite} page")
            if opposite not in links.switches:
                errors.append(f"{route}: no same-page {opposite} language switch")
        if links.edits:
            changed += 1
            if not check:
                file.write_text(links.output())
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
