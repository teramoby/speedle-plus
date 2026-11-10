#!/usr/bin/env python3
"""Point the Quick Start install pin at one release.

The English and Chinese guides install
``github.com/teramoby/speedle-plus/cmd/...@vX.Y.Z``. Generated page dates come
from front matter, so this update does not rewrite them.

Usage:
    python3 bump-quick-start-pin.py vX.Y.Z [repo]
"""

import re
import sys
from pathlib import Path

INSTALL = re.compile(
    r"(github\.com/teramoby/speedle-plus/cmd/\.\.\.@)(v[0-9]+\.[0-9]+\.[0-9]+)"
)
VERSION = re.compile(r"^v[0-9]+\.[0-9]+\.[0-9]+$")

GUIDES = (
    "docs/hugo/content/quick-start.md",
    "docs/hugo/content/quick-start.zh.md",
)
GENERATED = (
    "docs/quick-start/index.html",
    "docs/zh/quick-start/index.html",
    "docs/zh/index.xml",
)


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


def main(argv):
    args = argv[1:]
    if not args:
        print("usage: bump-quick-start-pin.py vX.Y.Z [repo]", file=sys.stderr)
        return 2
    if not VERSION.fullmatch(args[0]) or len(args) > 2:
        print("usage: bump-quick-start-pin.py vX.Y.Z [repo]", file=sys.stderr)
        return 2
    root = Path(args[1]) if len(args) == 2 else Path.cwd()
    if bump_tree(root, args[0]):
        print(f"Quick Start install pin set to {args[0]}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
