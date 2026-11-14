#!/usr/bin/env python3
"""Point the Quick Start install pin at one release.

All Quick Start locales install
``github.com/teramoby/speedle-plus/cmd/...@vX.Y.Z``. When the pin changes,
``--stamp`` writes one release time into front matter. The site build reads
that value and does not read Git.

Usage:
    python3 bump-quick-start-pin.py vX.Y.Z [repo]
    python3 bump-quick-start-pin.py --stamp 2026-11-10T16:35:23+08:00 [repo]
    python3 bump-quick-start-pin.py --paths [repo]
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

def guide_paths(root):
    """Discover the default guide and every locale, including future additions."""
    return sorted((Path(root) / "docs/hugo/content").glob("quick-start*.md"))


def outputs(root, guide):
    """Return the page, optional feed and sitemap for a discovered source."""
    locale = guide.name[len("quick-start"):-len(".md")].lstrip(".")
    directory = Path(root) / "docs" / locale
    return (
        directory / "quick-start/index.html",
        directory / "index.xml",
        Path(root) / "docs" / (locale or "en") / "sitemap.xml",
        f"https://engine.speedle.io/{locale + '/' if locale else ''}quick-start/",
    )


def tracked_paths(root):
    """Files the publisher must stage after pinning or stamping."""
    root = Path(root)
    paths = set(guide_paths(root))
    for guide in guide_paths(root):
        paths.update(outputs(root, guide)[:3])
    paths.update(root / "docs" / name for name in ("sitemap.xml", "llms.txt", "llms-full.txt"))
    return sorted(path.relative_to(root) for path in paths if path.is_file())


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
    """Update every guide and its generated page/feed without changing archives."""
    root = Path(root)
    changed = False
    for guide in guide_paths(root):
        text, did = bump_text(guide.read_text(), version)
        if not did:
            continue
        page, feed, _, _ = outputs(root, guide)
        if not page.is_file():
            raise SystemExit(f"missing generated Quick Start page {page}")
        rendered, rendered_did = bump_text(page.read_text(), version)
        if not rendered_did:
            raise SystemExit(f"{page} does not contain the previous install pin")
        guide.write_text(text)
        page.write_text(rendered)
        if feed.is_file():
            # RSS summaries have plain text rather than Markdown/HTML code spans.
            # Restrict replacements to the item containing the install command.
            def bump_item(match):
                item = match.group(0)
                pin = INSTALL.search(item)
                return item.replace(pin.group(2), version) if pin else item

            feed.write_text(re.sub(r"<item>[\s\S]*?</item>", bump_item, feed.read_text()))
        changed = True
    return changed


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
    guides = guide_paths(root)
    if not guides:
        return False
    for guide in guides:
        page, _, sitemap, loc = outputs(root, guide)
        if not page.is_file():
            raise SystemExit(f"missing generated Quick Start page {page}")
        if not sitemap.is_file():
            raise SystemExit(f"missing sitemap {sitemap}")
        guide.write_text(stamp_front_matter(guide.read_text(), authored))
        page.write_text(stamp_html(page.read_text(), authored))
        sitemap.write_text(stamp_sitemap(sitemap.read_text(), loc, authored))
    index = root / "docs/sitemap.xml"
    if index.is_file():
        index.write_text(stamp_index(index.read_text(), authored))
    return True


def main(argv):
    args = argv[1:]
    if not args:
        print("usage: bump-quick-start-pin.py vX.Y.Z [repo]", file=sys.stderr)
        return 2
    if args[0] == "--paths":
        if len(args) > 2:
            print("usage: bump-quick-start-pin.py --paths [repo]", file=sys.stderr)
            return 2
        root = Path(args[1]) if len(args) == 2 else Path.cwd()
        for path in tracked_paths(root):
            print(path)
        return 0
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
