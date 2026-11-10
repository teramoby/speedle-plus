#!/usr/bin/env python3
"""Generate llms.txt and llms-full.txt from the Hugo content.

The files follow the llmstxt.org convention: one H1, a blockquote summary, then
H2 sections of annotated links. llms-full.txt republishes the English
documentation as Markdown. Both files are generated from docs/hugo/content, so
page titles, descriptions and routes cannot drift from the published site.

Usage:
    python3 website/generate-llms.py --output DIR   # write into DIR
    python3 website/generate-llms.py --check DIR    # compare DIR with a fresh build
"""
import argparse
import difflib
import json
import re
import sys
from pathlib import Path
from urllib.parse import urljoin, urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parent.parent
CONTENT = ROOT / "docs/hugo/content"
ORIGIN = "https://engine.speedle.io"

# Pages included in llms-full.txt, in reading order. Every published English
# page must be listed here or in EXCLUDED_FROM_FULL, so adding a page forces a
# deliberate decision about the machine-readable documentation.
FULL_PAGES = [
    "about",
    "quick-start",
    "docs/overview",
    "docs/architecture",
    "docs/spdl",
    "docs/decisions",
    "docs/pms/_index",
    "docs/pms/policy-mgmt",
    "docs/pms/global-policy",
    "docs/pms/discover",
    "docs/pms/diagnosis",
    "docs/custom-function",
    "docs/assertor",
    "docs/idd",
    "docs/api/_index",
    "docs/api/decision_api",
    "docs/api/management_api",
    "docs/api/asserter_api",
    "docs/sdks/_index",
    "docs/sdks/java",
    "docs/sdks/python",
    "docs/sdks/go",
    "docs/sdks/dotnet",
    "docs/sdks/rust",
    "docs/sdks/node",
    "docs/cloud/_index",
    "docs/cloud/getting-started",
    "docs/cloud/tokens",
    "docs/cloud/authorization-api",
    "docs/cloud/limits-and-errors",
    "docs/deployment",
    "docs/deployment-embeded",
    "docs/store/_index",
    "docs/store/mysql",
    "docs/store/etcd",
    "docs/store/mongodb",
    "docs/security",
    "docs/usecases",
    "docs/logging",
    "docs/versions",
    "integrations/_index",
]

# Published pages that are deliberately not part of llms-full.txt.
EXCLUDED_FROM_FULL = {
    "_index",  # Hugo landing metadata; the home page is a static HTML file.
    "docs/_index",  # Section metadata; the navigation is part of every page.
    "playground",  # Rendered by its own layout; no Markdown body.
    "support",
    "contribute",
}

# Annotations for the links in llms.txt. Keys are content pages, resolved to
# their live route and checked by the generator.
ANNOTATIONS = {
    "docs/_index": "Install, configure and operate Speedle: concepts, policy management, deployment, APIs and SDKs.",
    "quick-start": "Install Speedle with go install or from source and make the first authorization decision with SPDL, spctl and the REST API.",
    "about": "What Speedle is, the project's origin, and the Universal Permissive License v1.0.",
    "docs/overview": "How Speedle maps to authorization terms: PAP, PDP, PEP, PIP, RBAC, ABAC, PBAC, XACML, identity, IAM and zero trust.",
    "docs/architecture": "The PMS, the policy store, the ADS, SDKs and the request flow between them.",
    "integrations/_index": "REST and gRPC APIs, embedded Go, six client SDKs, and file, etcd, MySQL and MongoDB policy stores.",
    "playground": "Edit SPDL policies and test real authorization requests in the browser; the engine runs locally in WebAssembly.",
    "docs/spdl": "The complete SPDL reference: policy and role-policy grammar, grant and deny, principals and AND groups, resources, conditions, attributes, built-in and custom functions.",
    "docs/pms/policy-mgmt": "Services, authorization policies and role policies, including resource-scoped role assignment.",
    "docs/decisions": "Authorization requests, allowed/denied responses, reason codes, granted roles and granted permissions.",
    "docs/custom-function": "Extend condition expressions with application-specific functions served over REST.",
    "docs/api/decision_api": "The ADS decision API contract: IsAllowed, granted roles and permissions, discovery and diagnosis.",
    "docs/api/management_api": "The PMS management API contract for services, policies, role policies and functions.",
    "docs/sdks/_index": "Client libraries for the decision API in six languages.",
    "docs/sdks/java": "Maven Central `io.speedle:speedle-sdk` 0.2.0, published.",
    "docs/sdks/python": "PyPI `speedle` 0.2.0, published.",
    "docs/sdks/go": "Go module `github.com/teramoby/speedle-sdk/go` v0.2.0, published.",
    "docs/sdks/dotnet": "NuGet `Speedle.Sdk` 0.2.0, published.",
    "docs/sdks/rust": "crates.io `speedle` 0.2.0, published.",
    "docs/sdks/node": "npm `@speedle/sdk`, published on npm. Install with `npm install @speedle/sdk`.",
    "docs/cloud/_index": "Hosted authorization for developers: sign-in and account management, SPDL policies, application tokens, the authorization API preview, limits, errors and downloads. Sign-in on cloud.speedle.io opens soon.",
    "docs/cloud/getting-started": "The console workflow: sign-in and account settings, a service, SPDL policies, session management requests, Playground tests and policy export.",
    "docs/cloud/tokens": "Issue, protect, list, rotate and revoke service-scoped Bearer tokens, with one-time secret disclosure and recent authentication.",
    "docs/cloud/authorization-api": "The published ADS IsAllowed request and response preview, with caller identity, a Node.js 22 example, policy snapshots and transport constraints.",
    "docs/cloud/limits-and-errors": "Account quotas, request limits, flat JSON errors, rate and quota headers, daily accounting and retry guidance.",
    "docs/versions": "Release history and semantic versioning.",
}

# Non-page links used by llms.txt. Every URL is checked when --check-links runs.
FIXED_LINKS = {
    "llms-full": (f"{ORIGIN}/llms-full.txt", "The full English documentation in one Markdown file, including the complete SPDL reference."),
    "repository": ("https://github.com/teramoby/speedle-plus/", "Canonical source code (Go), issues and releases."),
    "sdk_repository": ("https://github.com/teramoby/speedle-sdk/", "Source and reference for the six client SDKs."),
    "license": ("https://oss.oracle.com/licenses/upl/", "Universal Permissive License v1.0."),
    "ai": ("https://ai.speedle.io/", "Authorization for and with AI agents: use AI to manage access, and control what AI can access."),
    "ai_patterns": ("https://ai.speedle.io/patterns/", "SPDL patterns for data and training, model release, permission-aware RAG, delegated agents, MCP tool calls, agent-to-agent calls and zero standing privileges."),
    "mcp": ("https://ai.speedle.io/#mcp", "Speedle MCP launch preview: `@speedle/mcp` is published on npm as one package with check and manage modes. Install with `npm install @speedle/mcp`, or run the `speedle-mcp` binary with `npx @speedle/mcp check`."),
    "modeler": ("https://ai.speedle.io/#policy-modeler", "The open-source policy-modeler Agent Skill turns employee handbooks and regulations into RBAC + ABAC SPDL models with tests."),
    "mcp_repository": ("https://github.com/teramoby/speedle-mcp/", "Speedle MCP server source. `@speedle/mcp` is published on npm with check and manage modes. Run the `speedle-mcp` binary with `npx @speedle/mcp check`."),
    "modeler_skill": ("https://raw.githubusercontent.com/teramoby/ai.speedle.io/main/skills/speedle-policy-modeler/SKILL.md", "The entry point for the policy-modeler Agent Skill; its repository folder holds references, scripts, templates and English/Chinese example runs."),
    "cloud": ("https://cloud.speedle.io/", "Speedle Cloud, a hosted authorization service. Free public beta; sign-in opens soon."),
    "ai_llms": ("https://ai.speedle.io/llms.txt", "Speedle AI's own llms.txt index."),
    "cloud_llms": ("https://cloud.speedle.io/llms.txt", "Speedle Cloud's own llms.txt index."),
}

ZH_LINKS = [
    ("简体中文", "https://engine.speedle.io/zh/", "Speedle 中文首页。"),
    ("中文文档", "https://engine.speedle.io/zh/docs/", "概念、策略管理、部署、API 与 SDK 的中文文档。"),
    ("SPDL 中文参考", "https://engine.speedle.io/zh/docs/spdl/", "SPDL 策略与角色策略语法、条件、属性与函数。"),
    ("中文快速开始", "https://engine.speedle.io/zh/quick-start/", "安装 Speedle 并做出第一个授权决策。"),
    ("Speedle Cloud 中文文档", "https://engine.speedle.io/zh/docs/cloud/", "Speedle Cloud 托管授权文档：账号与策略管理、应用令牌、授权 API、限制与错误。"),
    ("SDK 中文文档", "https://engine.speedle.io/zh/docs/sdks/", "六种语言 SDK 的中文文档。"),
]


class ContentError(Exception):
    pass


def parse_front_matter(text, source):
    if not text.startswith("+++\n"):
        raise ContentError(f"{source}: expected a +++ front matter block")
    end = text.find("\n+++\n", 4)
    if end < 0:
        raise ContentError(f"{source}: unterminated front matter")
    meta_text = text[4:end]
    body = text[end + 5:]
    meta = {}
    for line in meta_text.splitlines():
        match = re.match(r"^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$", line)
        if not match:
            continue
        key, raw = match.group(1), match.group(2).strip()
        if key in {"title", "linktitle", "description", "bref", "layout", "customcssfile"}:
            try:
                meta[key] = json.loads(raw) if raw.startswith('"') else raw
            except json.JSONDecodeError:
                meta[key] = raw.strip('"')
        elif key == "draft":
            meta[key] = raw == "true"
        elif key == "aliases":
            meta[key] = [json.loads(value) for value in re.findall(r'"(?:[^"\\]|\\.)*"', raw)]
    return meta, body


def page_route(relative, language):
    relative = Path(relative)
    name = relative.name
    if name.endswith(".zh.md"):
        stem = name[: -len(".zh.md")]
    else:
        stem = name[: -len(".md")]
    parent = relative.parent.as_posix()
    parts = []
    if language == "zh":
        parts.append("zh")
    if parent not in ("", "."):
        parts.extend(part for part in parent.split("/") if part)
    if stem != "_index":
        parts.append(stem)
    route = "/" + "/".join(parts)
    if not route.endswith("/"):
        route += "/"
    return re.sub(r"/+$", "/", route)


def discover():
    pages = {"en": {}, "zh": {}}
    for source in sorted(CONTENT.rglob("*.md")):
        relative = source.relative_to(CONTENT)
        name = source.name
        language = "zh" if name.endswith(".zh.md") else "en"
        meta, body = parse_front_matter(source.read_text(encoding="utf-8"), source)
        stem = name[: -len(".zh.md")] if language == "zh" else name[: -len(".md")]
        key = (relative.parent / stem).as_posix()
        pages[language][key] = {
            "key": key,
            "language": language,
            "route": page_route(relative, language),
            "title": meta.get("title", key),
            "linktitle": meta.get("linktitle", ""),
            "description": meta.get("description", "").strip(),
            "bref": meta.get("bref", "").strip(),
            "draft": meta.get("draft", False),
            "aliases": meta.get("aliases", []),
            "body": body,
            "source": source,
        }
    return pages


def canonical_definition(pages):
    about = pages["en"].get("about")
    if not about:
        raise ContentError("about.md is missing; it holds the canonical definition")
    body = about["body"].strip()
    paragraph = re.split(r"\n\s*\n", body, maxsplit=1)[0].strip()
    if not paragraph or paragraph.startswith(("#", "-", "<")):
        raise ContentError("about.md must start with the canonical definition paragraph")
    return re.sub(r"\s+", " ", paragraph)


def page_link(page):
    return page["route"].join((ORIGIN, "")) if page["route"] != "/" else f"{ORIGIN}/"


def page_label(page):
    return page["linktitle"] or page["title"]


def annotation(key, pages):
    if key in ANNOTATIONS:
        return ANNOTATIONS[key]
    page = pages["en"][key]
    text = page["bref"] or page["description"] or page["title"]
    text = re.sub(r"\s+", " ", text).strip()
    if text.endswith("."):
        return text
    return text + "."


def require_page(pages, key):
    page = pages["en"].get(key)
    if page is None:
        raise ContentError(f"llms.txt refers to missing page {key!r}")
    if page["draft"]:
        raise ContentError(f"llms.txt refers to draft page {key!r}")
    return page


def render_llms(pages, definition):
    def item(key):
        page = require_page(pages, key)
        return f"- [{page_label(page)}]({page_link(page)}): {annotation(key, pages)}"

    def fixed(key, label):
        url, description = FIXED_LINKS[key]
        return f"- [{label}]({url}): {description}"

    def section(title, items):
        return f"## {title}\n\n" + "\n".join(items)

    blocks = [
        "# Speedle",
        f"> {definition}",
        (
            "Speedle evaluates RBAC and ABAC policies for users, services and processes acting on behalf of users, "
            "so authorization logic stays out of application code. SPDL (Speedle Policy Definition Language) is "
            "the language for its policies and role policies. The full English documentation is in "
            f"[llms-full.txt]({ORIGIN}/llms-full.txt); the canonical source is "
            "[teramoby/speedle-plus](https://github.com/teramoby/speedle-plus/) "
            "under the Universal Permissive License v1.0."
        ),
        section("Start here", [
            item("docs/_index"),
            item("quick-start"),
            item("about"),
            item("docs/overview"),
            item("docs/architecture"),
            item("integrations/_index"),
            item("playground"),
            fixed("llms-full", "llms-full.txt"),
        ]),
        section("SPDL and policy authoring", [
            item("docs/spdl"),
            item("docs/pms/policy-mgmt"),
            item("docs/decisions"),
            item("docs/custom-function"),
            item("docs/api/decision_api"),
            item("docs/api/management_api"),
        ]),
        section("SDKs", [
            "Java, Python, Go, .NET and Rust SDKs are version 0.2.0 and published. `@speedle/sdk` is published on npm (`npm install @speedle/sdk`).",
            "",
            item("docs/sdks/_index"),
            item("docs/sdks/java"),
            item("docs/sdks/python"),
            item("docs/sdks/go"),
            item("docs/sdks/dotnet"),
            item("docs/sdks/rust"),
            item("docs/sdks/node"),
        ]),
        section("Speedle Cloud", [
            item("docs/cloud/_index"),
            item("docs/cloud/getting-started"),
            item("docs/cloud/tokens"),
            item("docs/cloud/authorization-api"),
            item("docs/cloud/limits-and-errors"),
            fixed("cloud", "cloud.speedle.io"),
        ]),
        section("Speedle AI", [
            fixed("ai", "Speedle AI"),
            fixed("ai_patterns", "AI authorization patterns"),
            fixed("mcp", "Speedle MCP"),
            fixed("modeler", "Policy modeler skill"),
            fixed("mcp_repository", "Speedle MCP repository"),
            fixed("modeler_skill", "Policy modeler SKILL.md"),
        ]),
        section("Source, releases and license", [
            fixed("repository", "Source code"),
            fixed("sdk_repository", "SDK source"),
            item("docs/versions"),
            fixed("license", "License"),
        ]),
        section("Chinese", [
            f"- [{label}]({url}): {description}" for label, url, description in ZH_LINKS
        ]),
        section("Other Speedle sites", [
            fixed("ai_llms", "Speedle AI llms.txt"),
            fixed("cloud_llms", "Speedle Cloud llms.txt"),
        ]),
    ]
    return "\n\n".join(blocks) + "\n"


def replace_shortcodes(text):
    def swagger(match):
        url = match.group(1)
        absolute = urljoin(ORIGIN + "/", url)
        return f"The full OpenAPI definition is published at [{url}]({absolute})."
    return re.sub(r"\{\{<\s*swagger-spec\s+url=\"([^\"]+)\"\s*>\}\}", swagger, text)


def resolve_target(target, base_url, aliases):
    if target.startswith(("mailto:", "tel:", "data:", "javascript:")):
        return target
    if target.startswith("#"):
        return base_url + target
    split = urlsplit(target)
    if split.scheme or target.startswith("//"):
        return target
    absolute = urljoin(base_url, target)
    split = urlsplit(absolute)
    if split.scheme in ("http", "https") and split.netloc == "engine.speedle.io":
        path = aliases.get(split.path, split.path)
        absolute = urlunsplit((split.scheme, split.netloc, path, split.query, split.fragment))
    return absolute


MARKDOWN_LINK = re.compile(r"(!?\[[^\]]*\])\(([^)\s]+)(\s+\"[^\"]*\")?\)")
HTML_ATTR = re.compile(r"\b(href|src|srcset)=\"([^\"]*)\"")
HEADING = re.compile(r"^(#{1,6})(\s)")
FENCE = re.compile(r"^\s*(```|~~~)")


def transform_body(body, page_url, aliases):
    body = re.sub(r"<!--.*?-->", "", body, flags=re.S)
    body = replace_shortcodes(body)
    out = []
    in_fence = False
    for line in body.splitlines():
        if FENCE.match(line):
            in_fence = not in_fence
            out.append(line)
            continue
        if in_fence:
            out.append(line)
            continue
        heading = HEADING.match(line)
        if heading and len(heading.group(1)) < 6:
            line = "#" + line
        line = MARKDOWN_LINK.sub(
            lambda match: f"{match.group(1)}({resolve_target(match.group(2), page_url, aliases)}{match.group(3) or ''})",
            line,
        )
        line = HTML_ATTR.sub(
            lambda match: f'{match.group(1)}="{resolve_target(match.group(2), page_url, aliases)}"',
            line,
        )
        out.append(line)
    return "\n".join(out).strip()


def render_full(pages, definition):
    aliases = {}
    for language in ("en", "zh"):
        for page in pages[language].values():
            for alias in page["aliases"]:
                aliases[alias] = page["route"]

    missing = [key for key in FULL_PAGES if key not in pages["en"]]
    if missing:
        raise ContentError(f"llms-full.txt lists missing pages: {', '.join(missing)}")
    drafts = [key for key in FULL_PAGES if pages["en"][key]["draft"]]
    if drafts:
        raise ContentError(f"llms-full.txt lists draft pages: {', '.join(drafts)}")
    discovered = {
        key for key, page in pages["en"].items()
        if not page["draft"] and key not in EXCLUDED_FROM_FULL
    }
    unlisted = sorted(discovered - set(FULL_PAGES))
    if unlisted:
        raise ContentError(
            "published pages are neither in FULL_PAGES nor EXCLUDED_FROM_FULL: " + ", ".join(unlisted)
        )

    contents = []
    sections = []
    for key in FULL_PAGES:
        page = pages["en"][key]
        url = page_link(page)
        contents.append(f"- [{page_label(page)}]({url})")
        body = transform_body(page["body"], url, aliases)
        sections.append(f"## {page_label(page)}\n\nSource: {url}\n\n{body}")

    intro = (
        f"This file contains the complete English documentation published at {ORIGIN}/, "
        f"including the full reference for SPDL (Speedle Policy Definition Language). Index: {ORIGIN}/llms.txt. "
        f"Canonical source: https://github.com/teramoby/speedle-plus (Universal Permissive License v1.0)."
    )
    return "\n\n".join([
        "# Speedle — full documentation",
        f"> {definition}",
        intro,
        "## Contents",
        "\n".join(contents),
        *sections,
    ]) + "\n"


def build():
    pages = discover()
    definition = canonical_definition(pages)
    return {
        "llms.txt": render_llms(pages, definition),
        "llms-full.txt": render_full(pages, definition),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--output", metavar="DIR", help="write llms.txt and llms-full.txt into DIR")
    group.add_argument("--check", metavar="DIR", help="fail if DIR differs from a fresh build")
    options = parser.parse_args()

    try:
        outputs = build()
    except ContentError as error:
        print(f"generate-llms: {error}", file=sys.stderr)
        return 1

    if options.output:
        target = Path(options.output)
        target.mkdir(parents=True, exist_ok=True)
        for name, text in outputs.items():
            (target / name).write_text(text, encoding="utf-8")
        return 0

    target = Path(options.check)
    failed = False
    for name, expected in outputs.items():
        path = target / name
        if not path.exists():
            failed = True
            print(f"generate-llms: {path} is missing; rerun the generator", file=sys.stderr)
            continue
        actual = path.read_text(encoding="utf-8")
        if actual == expected:
            continue
        failed = True
        print(f"generate-llms: {path} is stale; rerun the generator", file=sys.stderr)
        diff = list(difflib.unified_diff(actual.splitlines(), expected.splitlines(),
                                         fromfile=str(path), tofile=f"generated {name}", lineterm=""))
        print("\n".join(diff[:80]), file=sys.stderr)
    if failed:
        return 1
    print("PASS llms.txt and llms-full.txt match the Hugo content")
    return 0


if __name__ == "__main__":
    sys.exit(main())
