#!/usr/bin/env python3
"""Execute the actual English/Chinese quick-start shell blocks against native services.

The documented `go install` route runs once into a temporary GOBIN, its module
path and version pattern are checked against the checkout, and its pinned version
must match the highest full semantic tag on the public remote. The installed
binaries must accept the flags used by the guide, and one documented walkthrough
runs with those installed binaries. Network steps (the public clone, the
module download and the install) retry up to three times with exponential
backoff, because a transient TLS or connection reset must not fail the
walkthrough; a step that hits its timeout fails at once instead of multiplying
the limit. The documented build runs once after a checker-added `go mod
download`, so a real defect in the guide fails on the first attempt instead of
looking like a flaky download. The documented commands themselves stay byte
for byte unchanged. A public shallow clone verifies the download step; CI then
checks out the captured candidate revision before compiling. Linux
runs the native services and clients in unprivileged user/network/PID
namespaces, using the exact documented ports without conflicting with existing
services. On macOS, alternate REST ports and a shared lock isolate fixture
services; occupied gRPC ports are rejected. Decision steps retry briefly,
because ADS reloads a changed policy file asynchronously.
"""
import argparse
import contextlib
import fcntl
import json
import os
import re
import signal
import shutil
import sys
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PATTERN = r"<!-- quick-start: ([a-z-]+) -->\s*```bash\n(.*?)\n```"
REQUIRED = {"version", "install", "build", "pms", "service", "policies", "list", "ads", "allow-direct", "deny"}
# The documented install route: module path and full semantic version tag.
INSTALL = re.compile(r'GOBIN="\$PWD/bin" go install (github\.com/teramoby/speedle-plus)/cmd/\.\.\.@(v[0-9]+\.[0-9]+\.[0-9]+)\nls bin\n?')
# Flags the guide passes to the installed release binaries.
INSTALLED_FLAGS = {
    "spctl": ("--pms-endpoint",),
    "speedle-pms": ("--store-type", "--filestore-loc", "--endpoint", "--insecure", "--grpc-endpoint"),
    "speedle-ads": ("--store-type", "--filestore-loc", "--endpoint", "--insecure", "--grpc-endpoint"),
}
# Documented decision steps and their expected `allowed` value. ADS watches the
# shared policy file and applies changes asynchronously, so a request issued
# right after `spctl create policy` can still use the previous policy set; the
# step is retried for DECISION_DEADLINE seconds, polling every DECISION_INTERVAL.
DECISIONS = {
    "allow-direct": True,
    "allow-role": True,
    "abac-allow": True,
    "deny": False,
    "abac-deny": False,
}
DECISION_DEADLINE = 10.0
DECISION_INTERVAL = 0.2


def blocks(path):
    text = path.read_text()
    pairs = re.findall(PATTERN, text, re.S)
    names = [name for name, _ in pairs]
    assert len(names) == len(set(names)), (path, "duplicate step")
    assert len(pairs) == len(re.findall(r"^```bash$", text, re.M)), (path, "untested shell block")
    assert REQUIRED <= set(names), (path, "missing steps", REQUIRED - set(names))
    assert set(names) <= REQUIRED | {"roles", "allow-role", "abac-policy", "abac-allow", "abac-deny"}, (path, "unknown step")
    assert names.index("install") < names.index("build"), path
    assert names.index("pms") < names.index("service") < names.index("ads") < names.index("allow-direct"), path
    assert names.index("deny") < names.index("abac-policy") < names.index("abac-allow") < names.index("abac-deny"), path
    for _, command in pairs:
        assert not re.search(r"^\$ |…|(?:^|\s)\.\.\.(?:\s|$)", command, re.M), (path, "non-executable example")
    return pairs


def run(command, cwd, env, timeout=60):
    args = ["bash", "-euc", command] if isinstance(command, str) else command
    result = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}): {command}\n{result.stdout}\n{result.stderr}")
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


def run_with_retry(action, reset=None, attempts=3, delay=5.0):
    """Retry a network command up to `attempts` times with exponential backoff.

    Only command failures are retried: they cover the transient TLS and
    connection resets of the clone, module download and install steps. A
    timeout is not retried, because a hung command would multiply the
    per-command limit across attempts and hide the underlying error.
    Assertions about the result are never retried, and neither is any
    documented command other than the network step passed in.

    `reset` runs after a failed attempt, before the next one. If it fails, the
    cleanup error is raised chained to the command failure that triggered the
    retry, so it cannot surface later as a misleading "already exists".

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
            print(f"Command failed (attempt {attempt}/{attempts}); retrying in {delay:.0f}s: {detail}", flush=True)
            time.sleep(delay)
            delay *= 2


def help_text(path, cwd, env):
    # The flag package writes usage to stderr, so combine both streams.
    result = subprocess.run([str(path), "--help"], cwd=cwd, env=env, text=True, capture_output=True, timeout=60)
    if result.returncode:
        raise RuntimeError(f"{path} --help failed ({result.returncode}):\n{result.stdout}\n{result.stderr}")
    return result.stdout + result.stderr


@contextlib.contextmanager
def server_lock():
    # Both runner instances use this host-wide path, outside their separate workspaces.
    with open("/tmp/speedle-plus-quick-start.lock", "a") as lock:
        deadline = time.monotonic() + 120
        while True:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                if time.monotonic() > deadline:
                    raise RuntimeError("Another quick-start check still holds the service-port lock")
                time.sleep(0.2)
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def ports():
    with socket.socket() as first, socket.socket() as second:
        first.bind(("127.0.0.1", 0))
        second.bind(("127.0.0.1", 0))
        return first.getsockname()[1], second.getsockname()[1]


def wait_ready(process, url):
    # Bypass HTTP proxies for owned loopback fixtures only.
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"Service exited before readiness: {process.returncode}")
        try:
            with client.open(url, timeout=1) as response:
                assert response.status == 200
                return
        except urllib.error.HTTPError as error:
            if error.code in (404, 405):
                return  # ADS has no GET handler at its POST-only decision URL.
            raise
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.1)
    raise RuntimeError(f"Service did not become ready: {url}")


def stop(process):
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGINT)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)
            raise RuntimeError("Quick-start service did not stop on Ctrl+C")
    assert process.returncode == 0, ("service exit", process.returncode)


def check_latest_install_version(module, version, env):
    """Fail when the version pinned in both guides is not the highest release tag."""
    repository = "https://" + "/".join(module.split("/")[:3]) + ".git"
    def list_tags():
        return run(["git", "ls-remote", "--tags", repository, "refs/tags/v*"], ROOT, env, timeout=120)
    listing = run_with_retry(list_tags)
    tags = {
        match.group(1)
        for line in listing.splitlines()
        if (match := re.search(r"refs/tags/(v[0-9]+\.[0-9]+\.[0-9]+)(?:\^\{\})?$", line))
    }
    assert tags, f"No full semantic version tags found for {module}"
    latest_version = max(tags, key=lambda tag: tuple(int(part) for part in tag[1:].split(".")))
    assert version == latest_version, (
        f"Quick Start install route pins stale {module}@{version}; "
        f"the highest release tag is {latest_version}. Update both language guides."
    )
    print(f"PASS Quick Start install pin {module}@{version} is the latest release", flush=True)


def verify_install_route(env):
    """Run the documented go install route once, not once per locale."""
    english_steps = blocks(ROOT / "docs/hugo/content/quick-start.md")
    chinese_steps = blocks(ROOT / "docs/hugo/content/quick-start.zh.md")
    english = dict(english_steps)["install"]
    assert english == dict(chinese_steps)["install"], "install command drift between English and Chinese"
    match = INSTALL.fullmatch(english)
    assert match, english
    module, version = match.groups()
    # Offline: the checkout must be the module the guide installs.
    assert run(["go", "list", "-m"], ROOT, env).strip() == module, module
    check_latest_install_version(module, version, env)
    with tempfile.TemporaryDirectory(prefix="speedle-go-install-") as temporary:
        work = Path(temporary)
        # GOBIN is this temporary bin directory, never the shared GOPATH/bin.
        # Only the documented install download retries; the `ls bin` check
        # that follows runs once.
        before, install, after = split_network_command(english, "go install")
        if before:
            run(before, work, env, timeout=600)
        run_with_retry(lambda: run(install, work, env, timeout=600))
        if after:
            run(after, work, env, timeout=600)
        for binary, flags in INSTALLED_FLAGS.items():
            path = work / "bin" / binary
            assert os.access(path, os.X_OK), binary
            usage = help_text(path, work, env)
            for flag in flags:
                # Accept one or two leading dashes: pflag prints --flag, while
                # the standard flag package prints -flag.
                assert re.search(rf"(?<![\w-])-{{1,2}}{re.escape(flag.lstrip('-'))}(?![\w-])", usage), (binary, flag, usage)
        print(f"PASS documented go install route ({module}@{version}); installed binaries accept the documented flags", flush=True)
        # One documented walkthrough with the installed release binaries: --help
        # alone does not prove the guide works with either route.
        route_a = work / "route-a"
        (route_a / "bin").mkdir(parents=True)
        for binary in INSTALLED_FLAGS:
            shutil.copy2(work / "bin" / binary, route_a / "bin" / binary)
        exercise("en", english_steps, route_a, env)
        print(f"PASS documented walkthrough with installed {module}@{version} binaries", flush=True)


def verify(locale, toolchain, env):
    source = ROOT / "docs/hugo/content" / ("quick-start.zh.md" if locale == "zh" else "quick-start.md")
    steps = blocks(source)
    with tempfile.TemporaryDirectory(prefix="speedle-quick-start-") as temporary:
        work = Path(temporary)
        version = run(dict(steps)["version"], work, env).strip()
        if toolchain:
            assert f"go version {toolchain} " in version, version
        print(f"{locale}: {version}; runner={os.environ.get('RUNNER_NAME', 'local')}", flush=True)
        build = dict(steps)["build"].splitlines()
        assert build[:2] == ["git clone --depth 1 https://github.com/teramoby/speedle-plus.git", "cd speedle-plus"], source
        checkout = work / "speedle-plus"

        def reset_checkout():
            # A failed attempt can leave a partial checkout behind, which would
            # make every retry fail with "already exists"; start clean. Only
            # absence is expected: any other cleanup error is raised (chained
            # to the failed attempt) rather than hidden.
            try:
                shutil.rmtree(checkout)
            except FileNotFoundError:
                pass

        # Only the documented clone downloads and retries; the `cd` that
        # follows is a local step and runs once.
        run_with_retry(lambda: run(build[0], work, env, timeout=180), reset=reset_checkout)
        run(build[1], work, env)
        revision = run(["git", "rev-parse", "HEAD"], ROOT, env).strip()
        # Keep the real clone step, then test this captured candidate rather than
        # a potentially older/newer master. No credentials or shared config changes.
        run(["git", "-c", "protocol.file.allow=always", "fetch", "--depth=1", "--no-tags", str(ROOT), revision], checkout, env)
        run(["git", "checkout", "--detach", revision], checkout, env)
        before, build_command, after = split_network_command("\n".join(build[2:]), "go build")
        if before:
            run(before, checkout, env)
        # Checker-added download step: the documented build needs the network
        # on a cold module cache, so the download retries and the documented
        # `go build` below runs once, failing on the first compile error.
        run_with_retry(lambda: run(["go", "mod", "download"], checkout, env, timeout=600))
        run(build_command, checkout, env, timeout=600)
        output = run(after, checkout, env) if after else ""
        for binary in ("spctl", "speedle-pms", "speedle-ads"):
            assert binary in output and os.access(checkout / "bin" / binary, os.X_OK), binary
        print(f"{locale}: PASS public clone and native build at {revision}", flush=True)
        if sys.platform.startswith("linux"):
            if not shutil.which("unshare") or not shutil.which("ip"):
                raise RuntimeError("Linux quick-start verification requires unshare and iproute2")
            report = work / "native-results.json"
            command = ["unshare", "--user", "--map-root-user", "--net", "--pid", "--fork",
                       "--mount-proc", "--kill-child=SIGKILL", "--", sys.executable,
                       str(Path(__file__).resolve()), "--exercise", str(checkout),
                       "--locale", locale, "--report", str(report)]
            print(run(command, checkout, env, timeout=180), end="", flush=True)
            result = json.loads(report.read_text())
        else:
            result = exercise(locale, steps, checkout, env)
        return {"locale": locale, "version": version, "runner": os.environ.get("RUNNER_NAME", "local"),
                "revision": revision, **result}


def wait_for_decision(name, command, checkout, env):
    """Run a documented decision command until ADS reports the expected decision.

    ADS watches the shared policy file and reloads it asynchronously, so the
    request that follows `spctl create policy` can still use the previous policy
    set. Retry the unchanged documented command until the decision matches or the
    deadline passes. The caller keeps the final assertion and its message, so a
    persistent mismatch still fails with the last observed response.
    """
    allowed = DECISIONS[name]
    deadline = time.monotonic() + DECISION_DEADLINE
    while True:
        result = json.loads(run(command, checkout, env))
        if result.get("allowed") is allowed and result.get("reason") == (0 if allowed else 3):
            return result
        if time.monotonic() >= deadline:
            return result
        time.sleep(DECISION_INTERVAL)


def exercise(locale, steps, checkout, env, isolated=False):
    work = checkout.parent
    pms_port, ads_port = (6733, 6734) if isolated else ports()
    def local(command):
        return command.replace("127.0.0.1:6733", f"127.0.0.1:{pms_port}").replace("127.0.0.1:6734", f"127.0.0.1:{ads_port}")
    expected_service = "library" if locale == "zh" else "mysvc"
    processes, logs, decisions = [], [], {}
    with (contextlib.nullcontext() if isolated else server_lock()), contextlib.ExitStack() as resources:
        try:
            for name, command in steps:
                if name in {"version", "install", "build"}:
                    continue
                command = local(command)
                if name in {"pms", "ads"}:
                    log = work / f"{name}.log"
                    logs.append(log)
                    stream = resources.enter_context(log.open("w"))
                    process = subprocess.Popen(["bash", "-euc", "exec " + command], cwd=checkout, env=env,
                                               stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
                    processes.append(process)
                    url = f"http://127.0.0.1:{pms_port}/policy-mgmt/v1/service" if name == "pms" else f"http://127.0.0.1:{ads_port}/authz-check/v1/is-allowed"
                    wait_ready(process, url)
                else:
                    if name == "list":
                        services = json.loads(run(command, checkout, env))
                        service = next(item for item in services if item["name"] == expected_service)
                        assert len(service.get("policies", [])) == (1 if locale == "zh" else 2), service
                        assert len(service.get("rolePolicies", [])) == (0 if locale == "zh" else 1), service
                    elif name in DECISIONS:
                        result = wait_for_decision(name, command, checkout, env)
                        allowed = DECISIONS[name]
                        assert result.get("allowed") is allowed, (name, result)
                        assert result.get("reason") == (0 if allowed else 3), (name, result)
                        decisions[name] = result["allowed"]
                    else:
                        run(command, checkout, env)
                assert all(process.poll() is None for process in processes), "service stopped during tutorial"
                print(f"{locale}: PASS {name}", flush=True)
            saved = json.loads((checkout / "quick-start-policies.json").read_text())
            assert any(service["name"] == expected_service for service in saved["services"]), saved
        except BaseException:
            for log in logs:
                print(f"--- {log.name} ---\n{log.read_text()[-5000:]}", flush=True)
            raise
        finally:
            failures = []
            for process in reversed(processes):
                try:
                    stop(process)
                except Exception as error:
                    failures.append(str(error))
            if failures:
                raise RuntimeError("Service shutdown failed: " + "; ".join(failures))
    print(f"{locale}: PASS persisted policy file and Ctrl+C shutdown", flush=True)
    return {"steps": [name for name, _ in steps], "decisions": decisions}



def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--toolchain", help="Exact Go toolchain, for example go1.27.3")
    parser.add_argument("--report", type=Path)
    parser.add_argument("--exercise", type=Path, help=argparse.SUPPRESS)
    parser.add_argument("--locale", choices=("en", "zh"), help=argparse.SUPPRESS)
    args = parser.parse_args()
    env = {key: value for key, value in os.environ.items() if not key.startswith("SPDL_")}
    if args.toolchain:
        env["GOTOOLCHAIN"] = args.toolchain
    env["NO_PROXY"] = env["no_proxy"] = "127.0.0.1,localhost" + ("," + env["NO_PROXY"] if env.get("NO_PROXY") else "")
    if args.exercise:
        assert sys.platform.startswith("linux") and args.locale and args.report
        # This runs only inside the unprivileged child user/network/PID namespaces.
        # Bringing up its loopback does not modify the host network. PID-namespace
        # teardown also reaps the entire fixture if the CI job is interrupted.
        run(["ip", "link", "set", "lo", "up"], args.exercise, env)
        source = ROOT / "docs/hugo/content" / ("quick-start.zh.md" if args.locale == "zh" else "quick-start.md")
        result = exercise(args.locale, blocks(source), args.exercise, env, isolated=True)
        args.report.write_text(json.dumps(result) + "\n")
        print(f"{args.locale}: PASS exact documented REST/gRPC ports in isolated Linux namespaces", flush=True)
        return
    verify_install_route(env)
    results = [verify(locale, args.toolchain, env) for locale in ("en", "zh")]
    if args.report:
        args.report.write_text(json.dumps(results, indent=2) + "\n")
    print("PASS English and Chinese quick-start commands against native PMS/ADS", flush=True)


if __name__ == "__main__":
    main()
