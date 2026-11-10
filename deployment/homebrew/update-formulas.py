#!/usr/bin/env python3
"""Rewrite Homebrew formula URLs and sha256 values from a release checksum file.

The formulas in this directory are the source of truth for teramoby/homebrew-tap.
`checksums.txt` is the file attached to a speedle-plus GitHub release. Each line
is a SHA-256 digest, whitespace, and an asset name such as spctl_darwin_arm64.
"""

import hashlib
import re
import sys
from pathlib import Path

VERSION_RE = re.compile(r"v\d+\.\d+\.\d+")
DIGEST_RE = re.compile(r"[0-9a-f]{64}")
URL_RE = re.compile(
    r'^(?P<indent>\s*)url "https://github.com/teramoby/speedle-plus/releases/download/'
    r'v\d+\.\d+\.\d+/(?P<asset>[A-Za-z0-9._-]+)"[ \t]*$'
)
SHA_RE = re.compile(r'^(?P<indent>\s*)sha256 "[0-9a-f]{64}"[ \t]*$')


class FormulaUpdateError(Exception):
    """The formulas could not be updated from the checksum file."""


def load_checksums(path):
    sums = {}
    text = Path(path).read_text()
    for line_number, line in enumerate(text.splitlines(), 1):
        if not line.strip():
            continue
        parts = line.split()
        if len(parts) != 2 or not DIGEST_RE.fullmatch(parts[0]):
            raise FormulaUpdateError(f"{path}:{line_number}: expected '<sha256> <asset>'")
        digest, asset = parts
        if asset in sums and sums[asset] != digest:
            raise FormulaUpdateError(f"{path}:{line_number}: {asset} has two checksums")
        sums[asset] = digest
    if not sums:
        raise FormulaUpdateError(f"{path}: no checksums")
    return sums


def update_formulas(formula_dir, version, checksums, checksums_digest=None):
    """Rewrite every Formula/*.rb under formula_dir. Return the paths written.

    checksums_digest is the SHA-256 of the checksums file itself. The speedle
    formula downloads that file so Homebrew has a versioned URL, and this
    digest is not a line inside the file.
    """
    if not VERSION_RE.fullmatch(version):
        raise FormulaUpdateError("version must look like v1.2.3")
    checksums = dict(checksums)
    if checksums_digest is not None:
        checksums["checksums.txt"] = checksums_digest
    formula_dir = Path(formula_dir)
    formulas = sorted(formula_dir.glob("*.rb"))
    if not formulas:
        raise FormulaUpdateError(f"{formula_dir}: no formulas")

    pending = []
    for formula in formulas:
        original = formula.read_text()
        newline = "\r\n" if "\r\n" in original else "\n"
        lines = original.splitlines()
        output = []
        seen = []
        index = 0
        while index < len(lines):
            match = URL_RE.match(lines[index])
            if match is None:
                output.append(lines[index])
                index += 1
                continue
            if index + 1 >= len(lines) or SHA_RE.match(lines[index + 1]) is None:
                raise FormulaUpdateError(f"{formula.name}: sha256 line must follow {match.group('asset')}")
            asset = match.group("asset")
            if asset not in checksums:
                raise FormulaUpdateError(f"{formula.name}: no checksum for {asset}")
            seen.append(asset)
            output.append(
                f'{match.group("indent")}url "https://github.com/teramoby/speedle-plus/releases/download/{version}/{asset}"'
            )
            sha = SHA_RE.match(lines[index + 1])
            output.append(f'{sha.group("indent")}sha256 "{checksums[asset]}"')
            index += 2
        if not seen:
            # A formula with no url lines is a dependency bundle, not a binary.
            # A url that is not a release asset is a mistake.
            if any(line.lstrip().startswith("url ") for line in lines):
                raise FormulaUpdateError(f"{formula.name}: no release asset URLs")
            continue
        text = newline.join(output)
        if original.endswith(("\n", "\r\n")):
            text += newline
        pending.append((formula, text))
    for formula, text in pending:
        formula.write_text(text)
    return [formula for formula, _text in pending]


def main(argv):
    if len(argv) not in (3, 4):
        print("usage: update-formulas.py vX.Y.Z checksums.txt [formula-dir]", file=sys.stderr)
        return 2
    version, checksums_path = argv[1], argv[2]
    formula_dir = Path(argv[3]) if len(argv) == 4 else Path(__file__).resolve().parent / "Formula"
    try:
        checksums_file = Path(checksums_path)
        written = update_formulas(
            formula_dir,
            version,
            load_checksums(checksums_file),
            hashlib.sha256(checksums_file.read_bytes()).hexdigest(),
        )
    except FormulaUpdateError as error:
        print(str(error), file=sys.stderr)
        return 1
    for path in written:
        print(f"updated {path.name} to {version}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
