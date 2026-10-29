#!/usr/bin/env python3
"""Build and run the bilingual embedded guide as an external Go module.

Execute the documented shell commands and compile the actual Go/SPDL blocks
against the public release, without a local module replacement or any servers.
"""
import argparse
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PATTERN = r"<!-- embedded: ([a-z-]+) -->\s*```(bash|go|ini)\n(.*?)\n```"


def blocks(locale):
    name = "deployment-embeded" + (".zh" if locale == "zh" else "") + ".md"
    source = ROOT / "docs/hugo/content/docs" / name
    text = source.read_text()
    pairs = re.findall(PATTERN, text, re.S)
    assert [name for name, _, _ in pairs] == ["version", "setup", "policies", "program", "build", "run"], source
    assert len(pairs) == len(re.findall(r"^```(?:bash|go|ini)$", text, re.M)), (source, "untested block")
    return {name: code for name, _, code in pairs}


def run(command, cwd, env):
    result = subprocess.run(["bash", "-euc", command], cwd=cwd, env=env,
                            text=True, capture_output=True, timeout=600)
    if result.returncode:
        raise RuntimeError(f"{command}\n{result.stdout}\n{result.stderr}")
    return result.stdout


def verify(locale, steps, toolchain):
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
        run(steps["setup"], work, env)
        app = work / "speedle-embedded-demo"
        (app / "policies.spdl").write_text(steps["policies"] + "\n")
        (app / "main.go").write_text(steps["program"] + "\n")
        run(steps["build"], app, env)
        module = json.loads(run("go list -m -json github.com/teramoby/speedle-plus", app, env))
        assert module["Version"] == "v1.6.0" and "Replace" not in module, module
        output = run(steps["run"], app, env).strip()
        assert output.splitlines() == ["alice allowed=true", "bob allowed=false"], output
        assert (app / "go.mod").is_file() and (app / "go.sum").is_file()
        print(f"{locale}: PASS module setup, published {module['Version']}, build, allow and deny", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--toolchain", help="Exact Go toolchain, for example go1.27.3")
    args = parser.parse_args()
    english, chinese = blocks("en"), blocks("zh")
    assert english == chinese, "English and Chinese executable examples differ"
    for locale, steps in (("en", english), ("zh", chinese)):
        verify(locale, steps, args.toolchain)
    print("PASS English and Chinese embedded guides against the public Go module", flush=True)


if __name__ == "__main__":
    main()
