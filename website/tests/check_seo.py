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

# Public semantic terms are scoped to relevant pages; visible copy stays intact.
KEYWORD_GROUPS = {
    "core": (["authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization"], ["授权引擎", "细粒度访问控制", "策略引擎", "授权策略引擎", "外置授权", "授权外部化"]),
    "pap": (["PAP", "policy administration point"], ["策略管理点"]),
    "pdp": (["PDP", "policy decision point"], ["策略决策点"]),
    "pep": (["PEP", "policy enforcement point"], ["策略执行点"]),
    "pip": (["PIP", "policy information point"], ["策略信息点"]),
    "central": (["centralized authorization"], ["集中式授权"]),
    "management": (["authorization policy management"], ["授权策略管理"]),
    "decision": (["authorization decision service"], ["授权决策服务"]),
    "api": (["API authorization"], ["API 授权", "API 权限控制"]),
    "microservices": (["microservices authorization"], ["微服务授权", "微服务权限控制"]),
    "cloud": (["cloud-native authorization"], ["云原生授权"]),
    "zero-trust": (["zero trust"], ["零信任"]),
}
ROLE_GROUPS = ["pap", "pdp", "pep", "pip"]
SEMANTIC_PAGES = {
    "/docs/": list(KEYWORD_GROUPS),
    "/docs/access-control/": list(KEYWORD_GROUPS),
    "/docs/access-control/rbac/": ["core"],
    "/docs/access-control/abac/": ["core"],
    "/docs/access-control/pbac/": ["core", "central", "management", "decision"],
    "/docs/access-control/identity-iam/": ["core", "zero-trust"],
    "/docs/access-control/oes/": ["core", "central", "management"],
    "/docs/access-control/xacml/": ["core"] + ROLE_GROUPS,
    "/docs/architecture/": ["core"] + ROLE_GROUPS + ["central", "management", "decision", "api", "microservices", "cloud"],
    "/docs/usecases/": ["core", "central", "management", "api", "microservices", "cloud"],
    "/docs/decisions/": ["core", "pdp", "decision", "api"],
    "/docs/pms/": ["core", "pap", "central", "management"],
    "/docs/pms/policy-mgmt/": ["core", "pap", "central", "management"],
    "/docs/api/decision_api/": ["core", "pdp", "decision", "api"],
    "/docs/api/management_api/": ["core", "pap", "management", "api"],
}


def semantic_terms(groups, zh):
    return [term for group in groups for term in KEYWORD_GROUPS[group][0] + (KEYWORD_GROUPS[group][1] if zh else [])]


def check_keywords(schema, groups, zh, route):
    keywords = schema.get("keywords", [])
    assert isinstance(keywords, list) and all(isinstance(term, str) for term in keywords), route
    actual = {term.casefold() for term in keywords}
    assert len(actual) == len(keywords), (route, "duplicate semantic keywords")
    missing = [term for term in semantic_terms(groups, zh) if term.casefold() not in actual]
    assert not missing, (route, "missing semantic keywords", missing)


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
        if neutral == "/integrations/":
            assert "noindex" not in " ".join(page.metas.get("robots", [])).casefold(), route
            assert ORIGIN + metadata_route in sitemaps, route
        if neutral == "/":
            software = [entity for schema in page.schemas for entity in schema.get("@graph", [schema])
                        if entity.get("@type") == "SoftwareSourceCode"]
            assert len(software) == 1, route
            check_keywords(software[0], list(KEYWORD_GROUPS), zh, route)
        elif neutral in SEMANTIC_PAGES:
            check_keywords(page.schemas[0], SEMANTIC_PAGES[neutral], zh, route)
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
    # Explicit false only opens the selected sections; kind defaults still apply.
    for prefix in ("", "zh/"):
        for name in ("blog/index.html", "categories/cloudnative/index.html", "tags/policy/index.html"):
            relative = prefix + name
            blocked = Page((root / relative).read_text())
            assert "noindex" in " ".join(blocked.metas.get("robots", [])).casefold(), relative
    for prefix in ("", "/zh"):
        for topic in TOPICS:
            route = prefix + "/docs/access-control/" + topic
            assert (root / route.strip("/") / "index.html").is_file(), route
            assert ORIGIN + route in sitemaps, route
    llms = (root / "llms.txt").read_text()
    for topic in TOPICS:
        for prefix in ("", "/zh"):
            assert ORIGIN + prefix + "/docs/access-control/" + topic in llms
    for term in semantic_terms(list(KEYWORD_GROUPS), True):
        assert term.casefold() in llms.casefold(), ("llms.txt", term)
    assert "The integrating system supplies identity assurance, trusted device/session context and enforcement." in llms
    assert "The NIST policy administrator (PA) is distinct from the XACML policy administration point (PAP)." in llms
    print(f"PASS SEO: {pages} current pages, 14 bilingual topic routes, scoped English/Chinese semantic keywords, metadata, JSON-LD and sitemaps")


if __name__ == "__main__":
    check(sys.argv[1] if len(sys.argv) > 1 else "docs")
