#!/usr/bin/env python3
"""Compare the evaluation benchmarks of the checked-out code with a fixed
baseline commit, on the same runner and in the same run.

The benchmarks in benchmarks/ run alternately for the baseline and the
candidate, ROUNDS times each, so both sides see the same machine and load.
Each benchmark's median ns/op is compared; one that is more than THRESHOLD
slower than the baseline is a regression. The comparison is written as a
Markdown table (performance.md) and JSON (performance.json) to the results
directory, and the script exits 1 on a regression or an incomplete run.

The baseline is the code as master stood in May 2026; see BASELINE.
"""
import argparse
import json
import os
import platform
import re
import shutil
import statistics
import subprocess
import sys
import tempfile
from pathlib import Path

# master during May 2026 (unchanged from 2026-01-16 until 2026-06-24).
BASELINE = "8fb1481d715e476242a991d391778b4334763afd"
ROUNDS = 5
THRESHOLD = 0.20
PACKAGE = "./benchmarks/"
LINE = re.compile(r"^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op(?:\s+([\d.]+) B/op)?(?:\s+([\d.]+) allocs/op)?")


def parse(output):
    """Return {benchmark: {"ns": .., "bytes": .., "allocs": ..}} for one run."""
    results = {}
    for line in output.splitlines():
        match = LINE.match(line.strip())
        if match:
            name, ns, size, allocs = match.groups()
            results[name] = {"ns": float(ns), "bytes": float(size or 0), "allocs": float(allocs or 0)}
    return results


def medians(runs):
    """Median of each metric over the runs in which a benchmark appeared."""
    names = sorted({name for run in runs for name in run})
    return {name: {key: statistics.median(run[name][key] for run in runs if name in run)
                   for key in ("ns", "bytes", "allocs")} for name in names}


def compare(baseline, candidate, threshold=THRESHOLD):
    """Rows for every benchmark and the names that regressed or are missing."""
    rows, regressed, missing = [], [], []
    for name in sorted(set(baseline) | set(candidate)):
        if name not in baseline or name not in candidate:
            missing.append(name)
            continue
        base, cand = baseline[name], candidate[name]
        change = cand["ns"] / base["ns"] - 1 if base["ns"] else 0.0
        slower = change > threshold
        if slower:
            regressed.append(name)
        rows.append({"name": name, "baseline": base, "candidate": cand, "change": change, "regressed": slower})
    return rows, regressed, missing


def runner_description(env=os.environ):
    """The machine the benchmarks ran on, as GitHub Actions names it."""
    name = env.get("RUNNER_NAME") or platform.node() or "unknown host"
    system = "/".join(v for v in (env.get("RUNNER_OS"), env.get("RUNNER_ARCH")) if v) or \
        f"{platform.system()}/{platform.machine()}"
    return f"{name} ({system})"


def render(rows, regressed, missing, baseline, candidate, rounds, threshold=THRESHOLD, runner=""):
    lines = [f"Baseline `{baseline[:12]}` (master in May 2026) against `{candidate[:12]}`, "
             f"median of {rounds} alternating rounds on the same runner. "
             f"A benchmark more than {threshold:.0%} slower than the baseline is a regression.", ""]
    if runner:
        lines += [f"Runner: {runner}", ""]
    lines += ["| Benchmark | Baseline ns/op | Current ns/op | Change | B/op | allocs/op |",
              "| --- | ---: | ---: | ---: | ---: | ---: |"]
    for row in rows:
        mark = " ⚠️" if row["regressed"] else ""
        b, c = row["baseline"], row["candidate"]
        lines.append(f"| {row['name']} | {b['ns']:.0f} | {c['ns']:.0f} | {row['change']:+.1%}{mark} | "
                     f"{b['bytes']:.0f} → {c['bytes']:.0f} | {b['allocs']:.0f} → {c['allocs']:.0f} |")
    lines.append("")
    if regressed:
        lines.append(f"Regressed: {', '.join(regressed)}.")
    if missing:
        lines.append(f"Present on only one side: {', '.join(missing)}.")
    if not regressed and not missing:
        lines.append("No regression.")
    return "\n".join(lines) + "\n"


def run_benchmarks(directory):
    """Run the benchmarks once in directory and return go test's output."""
    command = ["go", "test", "-run", "^$", "-bench", ".", "-benchmem", "-count", "1", "-timeout", "1800s", PACKAGE]
    result = subprocess.run(command, cwd=directory, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError(f"go test failed in {directory}:\n{result.stdout}\n{result.stderr}")
    return result.stdout


def baseline_checkout(repository, commit, parent):
    """A detached worktree of commit, fetched first when the clone is shallow."""
    def git(*args, check=True):
        return subprocess.run(["git", "-C", str(repository), *args], capture_output=True, text=True, check=check)
    if git("cat-file", "-e", f"{commit}^{{commit}}", check=False).returncode:
        git("fetch", "--no-tags", "--depth", "1", "origin", commit)
    path = Path(parent) / "baseline"
    git("worktree", "add", "--detach", str(path), commit)
    return path


def main(argv=None, runner=run_benchmarks):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--results", required=True, help="directory for performance.md and performance.json")
    parser.add_argument("--baseline", default=BASELINE)
    parser.add_argument("--rounds", type=int, default=ROUNDS)
    parser.add_argument("--threshold", type=float, default=THRESHOLD)
    parser.add_argument("--repository", default=".", help="the candidate checkout")
    args = parser.parse_args(argv)
    repository = Path(args.repository).resolve()
    results = Path(args.results)
    results.mkdir(parents=True, exist_ok=True)
    candidate = subprocess.run(["git", "-C", str(repository), "rev-parse", "HEAD"],
                               capture_output=True, text=True, check=True).stdout.strip()

    parent = tempfile.mkdtemp(prefix="speedle-benchmark-")
    try:
        base_dir = baseline_checkout(repository, args.baseline, parent)
        base_runs, cand_runs = [], []
        for round_number in range(1, args.rounds + 1):
            # Alternate, so slow drift on the machine hits both sides alike.
            base_runs.append(parse(runner(base_dir)))
            cand_runs.append(parse(runner(repository)))
            print(f"round {round_number}/{args.rounds} done", flush=True)
    finally:
        subprocess.run(["git", "-C", str(repository), "worktree", "remove", "--force", str(Path(parent) / "baseline")],
                       capture_output=True)
        shutil.rmtree(parent, ignore_errors=True)

    base, cand = medians(base_runs), medians(cand_runs)
    rows, regressed, missing = compare(base, cand, args.threshold)
    runner_name = runner_description()
    report = render(rows, regressed, missing, args.baseline, candidate, args.rounds, args.threshold, runner_name)
    (results / "performance.md").write_text(report)
    (results / "performance.json").write_text(json.dumps({
        "baseline": args.baseline, "candidate": candidate, "runner": runner_name,
        "rounds": args.rounds, "threshold": args.threshold,
        "benchmarks": rows, "regressed": regressed, "missing": missing}, indent=2) + "\n")
    print(report)
    return 1 if regressed or missing or not rows else 0


if __name__ == "__main__":
    sys.exit(main())
