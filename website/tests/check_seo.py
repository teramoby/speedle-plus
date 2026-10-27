#!/usr/bin/env python3
"""Check rendered bilingual SEO contracts, not search ranking or indexing."""
import json
import re
import sys
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from pathlib import Path

ORIGIN = "https://speedle.io"
TOPICS = {
    "": ["authorization", "access control", "授权", "访问控制"],
    "rbac/": ["RBAC", "role-based access control", "基于角色的访问控制"],
    "abac/": ["ABAC", "attribute-based access control", "基于属性的访问控制"],
    "pbac/": ["PBAC", "policy-based access control", "基于策略的访问控制"],
    "identity-iam/": ["identity", "IAM", "identity and access management", "身份与访问管理"],
    "oes/": ["OES", "Oracle Entitlements Server", "权限管理服务器"],
    "xacml/": ["XACML", "eXtensible Access Control Markup Language", "可扩展访问控制标记语言"],
}


class Page(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.metas, self.canonicals, self.alternates, self.schemas = {}, [], {}, []
        self.body, self.script, self.buffer, self.text = False, False, "", []
        self.feed(text)

    def handle_starttag(self, tag, pairs):
        attrs = dict(pairs)
        if tag == "body":
            self.body = True
        if tag == "meta":
            key = attrs.get("name", attrs.get("property"))
            self.metas.setdefault(key, []).append(attrs.get("content", ""))
        if tag == "link" and attrs.get("rel") == "canonical":
            self.canonicals.append(attrs.get("href"))
        if tag == "link" and attrs.get("rel") == "alternate" and "hreflang" in attrs:
            lang = attrs["hreflang"]
            assert lang not in self.alternates, f"Duplicate alternate: {lang}"
            self.alternates[lang] = attrs.get("href")
        if tag == "script":
            self.script = True
            self.buffer = "" if attrs.get("type") == "application/ld+json" else None

    def handle_data(self, data):
        if self.script and self.buffer is not None:
            self.buffer += data
        elif self.body and not self.script:
            self.text.append(data)

    def handle_endtag(self, tag):
        if tag == "script":
            if self.script and self.buffer is not None:
                self.schemas.append(json.loads(self.buffer))
            self.script = False
        if tag == "body":
            self.body = False


def check(root):
    root = Path(root)
    sitemaps = []
    for lang in ("en", "zh"):
        tree = ET.parse(root / lang / "sitemap.xml")
        sitemaps.extend(e.text for e in tree.iter() if e.tag.endswith("}loc"))
    pages = 0
    for file in root.rglob("*.html"):
        relative = file.relative_to(root)
        if relative.parts[0] in {"hugo", "beta"} or re.fullmatch(r"[0-9a-f]{32}", relative.parts[0]):
            continue
        text = file.read_text()
        if re.search(r'http-equiv=["\']refresh', text, re.I):
            continue
        route = "/" + relative.as_posix().removesuffix("index.html")
        zh = route.startswith("/zh/")
        # Existing noindex taxonomy pagination canonicalizes to its listing.
        # Preserve that policy while checking its language alternatives.
        metadata_route = re.sub(r"/page/[0-9]+/$", "/", route)
        neutral = metadata_route[3:] if zh else metadata_route
        page = Page(text)
        expected = {"en": ORIGIN + neutral, "zh-CN": ORIGIN + "/zh" + neutral, "x-default": ORIGIN + neutral}
        # Static homepages already supply the same contracts; Playground has no
        # x-default and uses its existing browser-specific metadata unchanged.
        assert page.canonicals == [ORIGIN + metadata_route], (route, page.canonicals)
        for lang in ("en", "zh-CN"):
            assert page.alternates.get(lang) == expected[lang], (route, lang, page.alternates)
        assert len(page.metas.get("description", [])) == 1, route
        assert page.metas["description"][0].strip(), route
        if "/playground/" not in route:
            assert page.alternates.get("x-default") == expected["x-default"], route
            assert page.schemas, route
        if "/docs/access-control/" in route:
            topic = route.split("/docs/access-control/")[1]
            visible = " ".join(page.text).lower()
            for term in TOPICS[topic]:
                if zh or term.isascii():
                    assert term.lower() in visible, (route, term)
            assert "speedle" in visible and ORIGIN + route in sitemaps, route
            assert "noindex" not in " ".join(page.metas.get("robots", [])), route
            assert page.schemas[0]["inLanguage"] == ("zh-CN" if zh else "en"), route
        pages += 1
    for prefix in ("", "/zh"):
        for topic in TOPICS:
            route = prefix + "/docs/access-control/" + topic
            assert (root / route.strip("/") / "index.html").is_file(), route
            assert ORIGIN + route in sitemaps, route
    llms = (root / "llms.txt").read_text()
    for topic in TOPICS:
        for prefix in ("", "/zh"):
            assert ORIGIN + prefix + "/docs/access-control/" + topic in llms
    print(f"PASS SEO: {pages} current pages, 14 bilingual topic routes, visible keyword coverage, metadata, JSON-LD and sitemaps")


if __name__ == "__main__":
    check(sys.argv[1] if len(sys.argv) > 1 else "docs")
