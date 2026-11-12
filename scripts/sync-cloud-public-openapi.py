#!/usr/bin/env python3
"""Fetch the public Cloud OpenAPI document into the docs site.

The Engine documentation renders docs/hugo/static/openapi/cloud-public.yaml.
Console routes under /api/ must not appear in that file.

    python3 scripts/sync-cloud-public-openapi.py
    python3 scripts/sync-cloud-public-openapi.py --check
"""

import argparse
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEST = ROOT / "docs/hugo/static/openapi/cloud-public.yaml"
PUBLIC_URL = "https://cloud.speedle.io/openapi.public.yaml"
USER_AGENT = "speedle-docs-openapi-sync"


class SpecError(Exception):
    """The public OpenAPI document cannot be vendored."""


def path_keys(text):
    """Return path keys of a top-level OpenAPI paths mapping.

    This is enough to name a rejected path. It is not a YAML parser.
    """
    keys = []
    in_paths = False
    for line in text.splitlines():
        if not in_paths:
            if re.match(r"^paths:\s*(#.*)?$", line):
                in_paths = True
            continue
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if line[0] not in (" ", "\t"):
            break
        match = re.match(r"^  (/[^:\s]*):\s*(?:#.*)?$", line)
        if match:
            keys.append(match.group(1))
    return keys


def prepare_public_spec(text):
    """Drop the JSON Schema dialect Swagger UI 5 refuses to render.

    Swagger UI 5.34 shows a warning when jsonSchemaDialect is not the
    OpenAPI 3.1 default, and asks for the field to be omitted. Paths and
    component schemas stay as published.
    """
    return re.sub(r"(?m)^jsonSchemaDialect:\s*\S+[^\n]*\n", "", text)


def validate_public_spec(text):
    """Reject an empty document, a non-OpenAPI file, or any /api/ path."""
    if not isinstance(text, str) or not text.strip():
        raise SpecError("public OpenAPI document is empty")
    if "\x00" in text:
        raise SpecError("public OpenAPI document is not UTF-8 text")
    if re.search(r"(?m)^jsonSchemaDialect:\s*\S+", text):
        raise SpecError("public OpenAPI document sets jsonSchemaDialect, which Swagger UI 5 cannot render")
    if not re.search(r"(?m)^(openapi:|swagger:)\s*\S+", text):
        raise SpecError("public OpenAPI document has no openapi or swagger version")
    keys = path_keys(text)
    if not keys:
        raise SpecError("public OpenAPI document has no paths")
    banned = [key for key in keys if "/api/" in key or key == "/api" or key.startswith("/api")]
    if "/api/" in text or banned:
        listed = ", ".join(banned) if banned else "text contains /api/"
        raise SpecError(f"public OpenAPI document contains /api/ paths: {listed}")
    return keys


def fetch(url, timeout=30):
    request = urllib.request.Request(
        url,
        headers={"User-Agent": USER_AGENT, "Accept": "application/yaml, text/yaml, */*"},
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            status = getattr(response, "status", None) or response.getcode()
            body = response.read()
    except urllib.error.HTTPError as exc:
        raise SpecError(f"fetch {url} failed: HTTP {exc.code}") from exc
    except urllib.error.URLError as exc:
        raise SpecError(f"fetch {url} failed: {exc.reason}") from exc
    if status != 200:
        raise SpecError(f"fetch {url} failed: HTTP {status}")
    try:
        text = body.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise SpecError("public OpenAPI document is not UTF-8") from exc
    return text


def write_spec(text, dest):
    dest = Path(dest)
    dest.parent.mkdir(parents=True, exist_ok=True)
    temporary = dest.with_name(dest.name + ".tmp")
    if not text.endswith("\n"):
        text += "\n"
    temporary.write_text(text, encoding="utf-8")
    temporary.replace(dest)


def build_parser():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="validate the vendored file and do not fetch")
    parser.add_argument("--url", default=PUBLIC_URL, help=f"document URL (default: {PUBLIC_URL})")
    parser.add_argument("--dest", type=Path, default=DEST, help="vendored file path")
    parser.add_argument("--timeout", type=float, default=30)
    return parser


def main(argv=None):
    options = build_parser().parse_args(argv)
    try:
        if options.check:
            if not options.dest.is_file():
                raise SpecError(f"vendored public OpenAPI document is missing: {options.dest}")
            text = options.dest.read_text(encoding="utf-8")
            keys = validate_public_spec(text)
            print(f"ok {options.dest} ({len(keys)} paths, no /api/ paths)")
            return 0
        text = prepare_public_spec(fetch(options.url, timeout=options.timeout))
        keys = validate_public_spec(text)
        write_spec(text, options.dest)
        print(f"wrote {options.dest} ({len(keys)} paths) from {options.url}")
        return 0
    except SpecError as exc:
        print(f"sync-cloud-public-openapi: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
