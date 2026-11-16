#!/usr/bin/env python3
"""Build and run the bilingual embedded guide as an external Go module.

Execute the documented shell commands and compile the actual Go/SPDL blocks
against the public release and, in the merge gate, the candidate library too.
The module download steps (`go get` and `go mod tidy`/`go build`) retry up to
three times with exponential backoff, because a transient network failure must
not fail the walkthrough; a step that hits its timeout fails at once instead of
multiplying the 600 s limit. The documented commands stay byte for byte
unchanged and the module-version and allow/deny checks are never retried.
"""
import argparse
import json
import os
import re
import shutil
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


def run_with_retry(action, reset=None, attempts=3, delay=5.0):
    """Retry a network action up to `attempts` times with exponential backoff.

    Only command failures are retried: they cover the transient TLS and
    connection resets of the module download and build steps. A timeout is not
    retried, because a hung command would multiply the 600 s limit across
    attempts and hide the underlying error. Assertions about the result are
    never retried.

    `reset` runs after a failed attempt, before the next one. If it fails, the
    cleanup error is raised chained to the command failure that triggered the
    retry, so it cannot surface later as a misleading "File exists".

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
            if reset is not None:
                try:
                    reset()
                except OSError as cleanup:
                    raise cleanup from error
            detail = str(error).splitlines()[0]
            print(f"Transient failure (attempt {attempt}/{attempts}); retrying in {delay:.0f}s: {detail}", flush=True)
            time.sleep(delay)
            delay *= 2


def verify(locale, steps, toolchain, candidate=False):
    env = dict(os.environ, GOWORK="off")
    # This throwaway application has no Git history. Persistent runner Git
    # metadata must not affect an external-module compilation (or runtime).
    env["GOFLAGS"] = (env.get("GOFLAGS", "") + " -buildvcs=false").strip()
    if toolchain:
        env["GOTOOLCHAIN"] = toolchain
    with tempfile.TemporaryDirectory(prefix="speedle-embedded-") as temporary:
        work = Path(temporary)
        version = run(steps["version"], work, env).strip()
        if toolchain:
            assert f"go version {toolchain} " in version, version
        print(f"{locale}: {version}; runner={env.get('RUNNER_NAME', 'local')}", flush=True)
        app = work / "speedle-embedded-demo"

        def reset_app():
            # A failed attempt can leave the module directory behind, which
            # would make every retry fail with "File exists"; start clean.
            # Only absence is expected: any other cleanup error is raised
            # (chained to the failed attempt) rather than hidden.
            try:
                shutil.rmtree(app)
            except FileNotFoundError:
                pass

        run_with_retry(lambda: run(steps["setup"], work, env), reset=reset_app)
        (app / "policies.spdl").write_text(steps["policies"] + "\n")
        (app / "main.go").write_text(steps["program"] + "\n")
        run_with_retry(lambda: run(steps["build"], app, env))
        module = json.loads(run("go list -m -json github.com/teramoby/speedle-plus", app, env))
        assert module["Version"] == "v1.6.0" and "Replace" not in module, module
        output = run(steps["run"], app, env).strip()
        assert output.splitlines() == ["alice allowed=true", "bob allowed=false"], output
        assert (app / "go.mod").is_file() and (app / "go.sum").is_file()
        print(f"{locale}: PASS module setup, published {module['Version']}, build, allow and deny", flush=True)
        if candidate:
            revision = run(["git", "rev-parse", "HEAD"], ROOT, env).strip()
            run(["go", "mod", "edit", "-replace", f"github.com/teramoby/speedle-plus={ROOT}"], app, env)
            run_with_retry(lambda: run(steps["build"], app, env))
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
