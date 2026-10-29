#!/usr/bin/env python3
"""Enforce the merge-gating Go coverage threshold.

Reads `go test -coverprofile` files and the packages the gate ran. A package
without test files is absent from the profile, which would silently shrink
the denominator, so every package must have tests unless it is listed in
NO_STATEMENTS (declaration-only packages with nothing to cover). Generated
protobuf code (*.pb.go) is not counted.

Several profiles can be merged, for example the spctl binary's own profile
from the CLI tests: a block counts as covered when any profile covers it.
"""
import argparse
import json
import os
import subprocess
import sys
from collections import Counter
from pathlib import Path

MODULE = "github.com/teramoby/speedle-plus/"
NO_STATEMENTS = {"api/ext", "api/pms", "pkg/svcs"}


def load_profile(*paths):
    blocks = {}
    for path in paths:
        for line in Path(path).read_text().splitlines()[1:]:
            block, statements, count = line.rsplit(" ", 2)
            if block.split(":", 1)[0].endswith(".pb.go"):
                continue
            # The same block may come from several test binaries or profiles.
            hit = blocks.get(block, (0, False))[1] or int(count) > 0
            blocks[block] = (int(statements), hit)
    total, covered = Counter(), Counter()
    for block, (statements, hit) in blocks.items():
        package = block.split(":", 1)[0].rsplit("/", 1)[0].removeprefix(MODULE)
        total[package] += statements
        if hit:
            covered[package] += statements
    return total, covered


def untested(packages):
    if not packages:
        return []
    listed = subprocess.run(
        ["go", "list", "-f", "{{.ImportPath}} {{len .TestGoFiles}} {{len .XTestGoFiles}}", *packages],
        check=True, capture_output=True, text=True).stdout
    missing = []
    for line in listed.splitlines():
        path, tests, xtests = line.split()
        name = path.removeprefix(MODULE)
        if int(tests) + int(xtests) == 0 and name not in NO_STATEMENTS:
            missing.append(name)
    return missing


def report(total, covered, minimum, missing):
    statements, hit = sum(total.values()), sum(covered.values())
    percent = 100 * hit / statements if statements else 0.0
    rows = ["| Package | Covered | Statements | % |", "|---|---:|---:|---:|"]
    for package in sorted(total, key=lambda p: covered[p] / total[p] if total[p] else 1):
        share = 100 * covered[package] / total[package] if total[package] else 100.0
        rows.append(f"| `{package}` | {covered[package]} | {total[package]} | {share:.1f} |")
    lines = [f"## Go coverage: {percent:.1f}% (minimum {minimum:.1f}%)", ""]
    if missing:
        lines += ["Packages without tests: " + ", ".join(f"`{p}`" for p in missing), ""]
    return percent, "\n".join(lines + rows) + "\n"


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profile")
    parser.add_argument("--merge", action="append", default=[], help="another profile to merge")
    parser.add_argument("--min-file", required=True, help="file holding the minimum percentage")
    parser.add_argument("--json", help="also write the totals as JSON")
    parser.add_argument("packages", nargs="*", help="packages the gate tested")
    # Packages follow the options (see run-go-tests.sh); before Python 3.13,
    # parse_args rejects positionals that come after optional arguments.
    args = parser.parse_intermixed_args(argv)

    minimum = float(Path(args.min_file).read_text().strip())
    total, covered = load_profile(args.profile, *args.merge)
    missing = untested(args.packages)
    percent, text = report(total, covered, minimum, missing)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as summary:
            summary.write(text)
    if args.json:
        Path(args.json).write_text(json.dumps({"percent": round(percent, 2), "minimum": minimum,
                                               "untested": missing}))
    if missing:
        print(f"::error::Packages without tests: {', '.join(missing)}", file=sys.stderr)
        return 1
    if percent + 1e-9 < minimum:
        print(f"::error::Go coverage {percent:.1f}% is below the {minimum:.1f}% minimum.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
