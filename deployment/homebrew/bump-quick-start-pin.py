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

import html
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
    directory = Path(root) / "docs/hugo/content"
    guides = sorted(directory.glob("quick-start*.md"))
    config = Path(root) / "docs/hugo/config.toml"
    if (guides or config.is_file()) and not (directory / "quick-start.md").is_file():
        raise SystemExit("default Quick Start guide is required: quick-start.md")
    if config.is_file():
        settings = config.read_text()
        default = re.search(r'^defaultContentLanguage\s*=\s*"([^"\n]+)"', settings, re.M)
        default = default[1] if default else "en"
        for locale in re.findall(r"^\[languages\.([A-Za-z0-9_-]+)\]", settings, re.M):
            name = "quick-start.md" if locale == default else f"quick-start.{locale}.md"
            if not (directory / name).is_file():
                raise SystemExit(f"configured Quick Start locale is missing: {name}")
    return guides


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


TAG = re.compile(r"v[0-9]+\.[0-9]+\.[0-9]+")
CODE_TAG = re.compile(r"`(v[0-9]+\.[0-9]+\.[0-9]+)`|<code>(v[0-9]+\.[0-9]+\.[0-9]+)</code>")


def release_note(text):
    """Locate only the paragraph immediately after the first install block."""
    command = INSTALL.search(text)
    if command is None:
        return None
    tail = text[command.end():]
    offset = command.end()
    if text.rfind("<pre", 0, command.start()) > text.rfind("</pre>", 0, command.start()):
        block = re.search(r"</pre>\s*(?:</div>\s*)?", tail)
        if block is None:
            return None
        offset += block.end()
        note = re.match(r"<p>(.*?)</p>", tail[block.end():], re.S)
    else:
        note = re.search(r"\n```[ \t]*\n\s*\n([^\n]+(?:\n(?!\s*\n)[^\n]+)*)", tail)
    if note is None:
        return None
    return offset + note.start(1), offset + note.end(1)


def bump_text(text, version, plain=False, note=None):
    """Repair the first install command and its own release-note paragraph.

    For RSS, use the source paragraph's text before its release tag as context.
    A removed/reordered note cannot redirect a replacement into another example.
    """
    command = INSTALL.search(text)
    if command is None:
        return text, False
    updated = INSTALL.sub(lambda pin: pin.group(1) + version, text, count=1)
    if plain:
        if note is not None:
            # Hugo's RSS summary strips inline code and XML-escapes text.
            reference = html.escape(note.replace("`", ""), quote=False)
            tag = TAG.search(reference)
            if tag:
                def context(value):
                    return "".join(r"\s+" if part.isspace() else re.escape(part)
                                   for part in re.split(r"(\s+)", value))

                prefix, suffix = reference[:tag.start()], reference[tag.end():]
                if not (prefix.strip() or suffix.strip()):
                    raise SystemExit("RSS release note needs surrounding text; regenerate the site")
                pattern = re.compile(context(prefix) + r"(?P<tag>v[0-9]+\.[0-9]+\.[0-9]+)"
                                     + context(suffix))
                # Match the complete source note, not another version in an item.
                pin = INSTALL.search(updated)
                tail = updated[pin.end():]
                match = pattern.search(tail)
                if match:
                    tail = tail[:match.start("tag")] + version + tail[match.end("tag"):]
                elif TAG.search(tail):
                    raise SystemExit("RSS release note differs from source; regenerate the site")
                updated = updated[:pin.end()] + tail
    else:
        span = release_note(updated)
        if span:
            paragraph = updated[span[0]:span[1]]
            paragraph = CODE_TAG.sub(lambda tag: f"`{version}`" if tag.group().startswith("`")
                                     else f"<code>{version}</code>", paragraph, count=1)
            updated = updated[:span[0]] + paragraph + updated[span[1]:]
    return updated, updated != text


def bump_tree(root, version):
    """Repair every source, generated page and feed, including partial retries."""
    root = Path(root)
    changed = False
    for guide in guide_paths(root):
        page, feed, _, _ = outputs(root, guide)
        if not page.is_file():
            raise SystemExit(f"missing generated Quick Start page {page}")
        for path in (guide, page):
            original = path.read_text()
            if INSTALL.search(original) is None:
                raise SystemExit(f"{path} does not contain the install pin")
            text, did = bump_text(original, version)
            if did:
                path.write_text(text)
                changed = True
        if feed.is_file():
            # Restrict replacements to the item containing the install command.
            original = feed.read_text()
            source = guide.read_text()
            span = release_note(source)
            note = source[span[0]:span[1]] if span else None
            text = re.sub(r"<item>[\s\S]*?</item>",
                          lambda item: bump_text(item.group(), version, plain=True, note=note)[0], original)
            if text != original:
                feed.write_text(text)
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
