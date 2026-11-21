#!/usr/bin/env python3
"""Build and run the bilingual embedded guide as an external Go module.

Execute the documented shell commands and compile the actual Go/SPDL blocks
against the public release and, in the merge gate, the candidate library too.
Only the module downloads (`go get` and `go mod tidy`) retry, up to three
times with exponential backoff, because a transient network failure must not
fail the walkthrough. Every other documented command, `go build` included,
runs once: a real defect in the guide must fail on the first attempt instead
of looking like a flaky download. A command that hits its timeout fails at
once instead of multiplying the 600 s limit. The documented commands stay byte
for byte unchanged and the module-version and allow/deny checks are never
retried.
"""
import argparse
import json
import os
import re
import subprocess
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PATTERN = r"<!-- embedded: ([a-z-]+) -->\s*```(bash|go|ini)\n(.*?)\n```"


def blocks(locale):
    name = "embedded" + (".zh" if locale == "zh" else "") + ".md"
    source = ROOT / "docs/hugo/content/docs/deployment" / name
    text = source.read_text()
    pairs = re.findall(PATTERN, text, re.S)
    assert [name for name, _, _ in pairs] == ["version", "setup", "policies", "program", "build", "run"], source
    assert len(pairs) == len(re.findall(r"^```(?:bash|go|ini)$", text, re.M)), (source, "untested block")
    return {name: code for name, _, code in pairs}


def run(command, cwd, env):
    args = ["bash", "-euc", command] if isinstance(command, str) else command
    result = subprocess.run(args, cwd=cwd, env=env,
                            text=True, capture_output=True, timeout=600)
    if result.returncode:
        raise RuntimeError(f"{command}\n{result.stdout}\n{result.stderr}")
    return result.stdout


def split_network_command(block, command):
    """Split a documented block around its single network-touching command.

    Returns the lines before it, the command itself and the lines after it.
    The caller retries only the network command and runs the others once, so a
    genuine defect in the guide fails on the first attempt while a transient
    download failure is retried. The block must contain the command exactly
    once: a silent mismatch would retry the wrong step or skip a documented
    command.
    """
    pattern = re.compile(rf"(?:^|\s){re.escape(command)}(?:\s|$)")
    lines = block.splitlines()
    matches = [index for index, line in enumerate(lines) if pattern.search(line)]
    assert len(matches) == 1, (command, block)
    index = matches[0]
    return "\n".join(lines[:index]), lines[index], "\n".join(lines[index + 1:])


def run_with_retry(action, attempts=3, delay=5.0):
    """Retry a network command up to `attempts` times with exponential backoff.

    Only command failures are retried: they cover the transient TLS and
    connection resets of the module downloads. A timeout is not retried,
    because a hung command would multiply the 600 s limit across attempts and
    hide the underlying error. Assertions about the result are never retried,
    and neither is any documented command other than the download passed in.

    This is a private copy rather than a shared module: the walkthrough checks
    run under python3 -I, which keeps the script directory off sys.path, so a
    bare sibling import is unavailable; the workflow Python isolation guard
    (scripts/tests/test_workflow_python_isolation.cjs) enforces that.
    """
    for attempt in range(1, attempts + 1):
        try:
            return action()
        except RuntimeError as error:
            if attempt == attempts:
                raise
            detail = str(error).splitlines()[0]
            print(f"Command failed (attempt {attempt}/{attempts}); retrying in {delay:.0f}s: {detail}", flush=True)
            time.sleep(delay)
            delay *= 2


def verify(locale, steps, toolchain, candidate=False):
    env = dict(os.environ, GOWORK="off")
    # This throwaway application has no Git history. Persistent runner Git
    # metadata must not affect an external-module compilation (or runtime).
    env["GOFLAGS"] = (env.get("GOFLAGS", "") + " -buildvcs=false").strip()
    if toolchain:
        # The published release this guide builds may declare a newer toolchain
        # than the one under test; +auto lets that module select it while the
        # documented commands otherwise run on the named toolchain.
        env["GOTOOLCHAIN"] = toolchain + "+auto"
    setup_before, get, setup_after = split_network_command(steps["setup"], "go get")
    build_before, tidy, build_after = split_network_command(steps["build"], "go mod tidy")
    with tempfile.TemporaryDirectory(prefix="speedle-embedded-") as temporary:
        work = Path(temporary)
        version = run(steps["version"], work, env).strip()
        if toolchain:
            assert f"go version {toolchain} " in version, version
        print(f"{locale}: {version}; runner={env.get('RUNNER_NAME', 'local')}", flush=True)
        app = work / "speedle-embedded-demo"
        # The `cd` in the documented setup block only affects its own shell, so
        # the download retries from the module directory the block creates.
        run(setup_before, work, env)
        run_with_retry(lambda: run(get, app, env))
        if setup_after:
            run(setup_after, app, env)
        (app / "policies.spdl").write_text(steps["policies"] + "\n")
        (app / "main.go").write_text(steps["program"] + "\n")

        def build():
            # Only `go mod tidy` downloads here. The documented `go build` runs
            # once, so a compile error fails on the first attempt instead of
            # being retried as a flaky download.
            if build_before:
                run(build_before, app, env)
            run_with_retry(lambda: run(tidy, app, env))
            if build_after:
                run(build_after, app, env)

        build()
        module = json.loads(run("go list -m -json github.com/teramoby/speedle-plus", app, env))
        assert module["Version"] == "v1.9.0" and "Replace" not in module, module
        output = run(steps["run"], app, env).strip()
        assert output.splitlines() == ["alice allowed=true", "bob allowed=false"], output
        assert (app / "go.mod").is_file() and (app / "go.sum").is_file()
        print(f"{locale}: PASS module setup, published {module['Version']}, build, allow and deny", flush=True)
        if candidate:
            revision = run(["git", "rev-parse", "HEAD"], ROOT, env).strip()
            run(["go", "mod", "edit", "-replace", f"github.com/teramoby/speedle-plus={ROOT}"], app, env)
            build()
            selected = json.loads(run("go list -m -json github.com/teramoby/speedle-plus", app, env))
            assert Path(selected["Replace"]["Dir"]).resolve() == ROOT.resolve(), selected
            output = run(steps["run"], app, env).strip()
            assert output.splitlines() == ["alice allowed=true", "bob allowed=false"], output
            print(f"{locale}: PASS same documented program against candidate {revision}, allow and deny", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--toolchain", help="Exact Go toolchain, for example go1.27.3")
    parser.add_argument("--candidate", action="store_true", help="Also verify the same example against this checkout")
    args = parser.parse_args()
    english, chinese = blocks("en"), blocks("zh")
    assert english == chinese, "English and Chinese executable examples differ"
    for locale, steps in (("en", english), ("zh", chinese)):
        verify(locale, steps, args.toolchain, args.candidate)
    print("PASS English and Chinese embedded guides against the public Go module", flush=True)


if __name__ == "__main__":
    main()
