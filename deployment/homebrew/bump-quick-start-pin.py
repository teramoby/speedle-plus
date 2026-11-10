#!/usr/bin/env python3
"""Point the Quick Start install pin at one release.

The English and Chinese guides install
``github.com/teramoby/speedle-plus/cmd/...@vX.Y.Z``. When the pin changes,
``--stamp`` writes one release time into front matter. The site build reads
that value and does not read Git.

Usage:
    python3 bump-quick-start-pin.py vX.Y.Z [repo]
    python3 bump-quick-start-pin.py --stamp 2026-11-10T16:35:23+08:00 [repo]
"""

import re
import sys
from datetime import datetime
from pathlib import Path

INSTALL = re.compile(
    r"(github\.com/teramoby/speedle-plus/cmd/\.\.\.@)(v[0-9]+\.[0-9]+\.[0-9]+)"
)
VERSION = re.compile(r"^v[0-9]+\.[0-9]+\.[0-9]+$")
# The same form Hugo writes for Lastmod. A trailing Z is normalized to +00:00
# because the page template's numeric offset prints UTC that way.
AUTHORED = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2})$")

GUIDES = (
    "docs/hugo/content/quick-start.md",
    "docs/hugo/content/quick-start.zh.md",
)
GENERATED = (
    "docs/quick-start/index.html",
    "docs/zh/quick-start/index.html",
    "docs/zh/index.xml",
)
SITEMAPS = (
    ("docs/en/sitemap.xml", "https://engine.speedle.io/quick-start/"),
    ("docs/zh/sitemap.xml", "https://engine.speedle.io/zh/quick-start/"),
)
SITEMAP_INDEX = "docs/sitemap.xml"


def bump_text(text, version):
    """Replace this file's install pin and the sentence that names it.

    Returns the new text and whether it changed. A file without the install
    command is left untouched.
    """
    match = INSTALL.search(text)
    if match is None:
        return text, False
    old = match.group(2)
    if old == version:
        return text, False
    text = text.replace(match.group(0), match.group(1) + version, 1)
    markdown = f"`{old}`"
    html = f"<code>{old}</code>"
    if markdown in text:
        text = text.replace(markdown, f"`{version}`", 1)
    elif html in text:
        text = text.replace(html, f"<code>{version}</code>", 1)
    return text, True


def bump_tree(root, version):
    """Update both guides and the generated pages that quote the install command."""
    root = Path(root)
    guides = [root / rel for rel in GUIDES]
    if not any(path.is_file() for path in guides):
        return False
    missing = [path for path in guides if not path.is_file()]
    if missing:
        raise SystemExit("both Quick Start guides are required: " + ", ".join(str(path) for path in missing))
    changed = False
    for path in guides:
        text, did = bump_text(path.read_text(), version)
        if did:
            path.write_text(text)
            changed = True
    if not changed:
        return False
    for rel in GENERATED:
        path = root / rel
        if not path.is_file():
            raise SystemExit(f"missing generated Quick Start page {path}")
        text, did = bump_text(path.read_text(), version)
        if not did:
            raise SystemExit(f"{path} does not contain the previous install pin")
        path.write_text(text)
    return True


def hugo_stamp(authored):
    """Return the offset form Hugo prints for this timestamp."""
    if authored.endswith("Z"):
        return authored[:-1] + "+00:00"
    return authored


def stamp_front_matter(text, authored):
    """Set the guide's lastmod. The site build uses this instead of Git."""
    updated, count = re.subn(
        r"(?m)^(lastmod\s*=\s*)\S+",
        rf"\g<1>{authored}",
        text,
        count=1,
    )
    if count != 1:
        raise SystemExit("Quick Start guide is missing lastmod")
    return updated


def stamp_html(text, authored):
    """Set the article modification time Hugo writes into the page."""
    escaped = authored.replace("+", "&#43;")
    updated, count = re.subn(
        r'(<meta property="article:modified_time" content=")[^"]+(">)',
        rf"\g<1>{escaped}\g<2>",
        text,
        count=1,
    )
    if count != 1:
        raise SystemExit("Quick Start HTML is missing article:modified_time")
    return updated


def stamp_sitemap(text, loc, authored):
    """Set the lastmod of one sitemap URL."""
    pattern = re.compile(
        rf"(<loc>{re.escape(loc)}</loc>\s*<lastmod>)[^<]+(</lastmod>)"
    )
    updated, count = pattern.subn(rf"\g<1>{authored}\2", text, count=1)
    if count != 1:
        raise SystemExit(f"sitemap is missing {loc}")
    return updated


def instant(value):
    """Parse a Hugo or Git timestamp."""
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)


def stamp_index(text, authored):
    """Raise a sitemap index lastmod that is older than the release time."""
    release = instant(authored)

    def replace(match):
        if instant(match.group(1)) >= release:
            return match.group(0)
        return f"<lastmod>{authored}</lastmod>"

    return re.sub(r"<lastmod>([^<]+)</lastmod>", replace, text)


def stamp_tree(root, authored):
    """Write one release time into the Quick Start sources and generated pages."""
    root = Path(root)
    authored = hugo_stamp(authored)
    if not (root / GUIDES[0]).is_file():
        return False
    for rel in GUIDES:
        path = root / rel
        if not path.is_file():
            raise SystemExit(f"missing Quick Start guide {path}")
        path.write_text(stamp_front_matter(path.read_text(), authored))
    for rel in GENERATED[:2]:
        path = root / rel
        if not path.is_file():
            raise SystemExit(f"missing generated Quick Start page {path}")
        path.write_text(stamp_html(path.read_text(), authored))
    for rel, loc in SITEMAPS:
        path = root / rel
        if not path.is_file():
            raise SystemExit(f"missing sitemap {path}")
        path.write_text(stamp_sitemap(path.read_text(), loc, authored))
    index = root / SITEMAP_INDEX
    if index.is_file():
        index.write_text(stamp_index(index.read_text(), authored))
    return True


def main(argv):
    args = argv[1:]
    if not args:
        print("usage: bump-quick-start-pin.py vX.Y.Z [repo]", file=sys.stderr)
        return 2
    if args[0] == "--stamp":
        if len(args) not in (2, 3) or not AUTHORED.fullmatch(args[1]):
            print(
                "usage: bump-quick-start-pin.py --stamp YYYY-MM-DDTHH:MM:SS+HH:MM [repo]",
                file=sys.stderr,
            )
            return 2
        root = Path(args[2]) if len(args) == 3 else Path.cwd()
        if stamp_tree(root, args[1]):
            print(f"Quick Start modification time set to {hugo_stamp(args[1])}")
        return 0
    if not VERSION.fullmatch(args[0]) or len(args) > 2:
        print("usage: bump-quick-start-pin.py vX.Y.Z [repo]", file=sys.stderr)
        return 2
    root = Path(args[1]) if len(args) == 2 else Path.cwd()
    if bump_tree(root, args[0]):
        print(f"Quick Start install pin set to {args[0]}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
