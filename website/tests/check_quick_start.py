#!/usr/bin/env python3
"""Execute the actual English/Chinese quick-start shell blocks against native services.

A public shallow clone verifies the download step; CI then checks out the captured
candidate revision before compiling. Linux runs the native services and clients
in unprivileged user/network/PID namespaces, using the exact documented ports
without conflicting with existing services. On macOS, alternate REST ports and a
shared lock isolate fixture services; occupied gRPC ports are rejected.
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
REQUIRED = {"version", "build", "pms", "service", "policies", "list", "ads", "allow-direct", "deny"}


def blocks(path):
    text = path.read_text()
    pairs = re.findall(PATTERN, text, re.S)
    names = [name for name, _ in pairs]
    assert len(names) == len(set(names)), (path, "duplicate step")
    assert len(pairs) == len(re.findall(r"^```bash$", text, re.M)), (path, "untested shell block")
    assert REQUIRED <= set(names), (path, "missing steps", REQUIRED - set(names))
    assert set(names) <= REQUIRED | {"roles", "allow-role"}, (path, "unknown step")
    assert names.index("pms") < names.index("service") < names.index("ads") < names.index("allow-direct"), path
    for _, command in pairs:
        assert not re.search(r"^\$ |…|\.\.\.", command, re.M), (path, "non-executable example")
    return pairs


def run(command, cwd, env, timeout=60):
    args = ["bash", "-euc", command] if isinstance(command, str) else command
    result = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}): {command}\n{result.stdout}\n{result.stderr}")
    return result.stdout


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
            for port in (50001, 50002):
                with socket.socket() as probe:
                    probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                    probe.bind(("0.0.0.0", port))
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
        run("\n".join(build[:2]), work, env, timeout=180)
        checkout = work / "speedle-plus"
        revision = run(["git", "rev-parse", "HEAD"], ROOT, env).strip()
        # Keep the real clone step, then test this captured candidate rather than
        # a potentially older/newer master. No credentials or shared config changes.
        run(["git", "-c", "protocol.file.allow=always", "fetch", "--depth=1", "--no-tags", str(ROOT), revision], checkout, env)
        run(["git", "checkout", "--detach", revision], checkout, env)
        output = run("\n".join(build[2:]), checkout, env, timeout=600)
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
                if name in {"version", "build"}:
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
                    output = run(command, checkout, env)
                    if name == "list":
                        services = json.loads(output)
                        service = next(item for item in services if item["name"] == expected_service)
                        assert len(service.get("policies", [])) == (1 if locale == "zh" else 2), service
                        assert len(service.get("rolePolicies", [])) == (0 if locale == "zh" else 1), service
                    elif name.startswith("allow-") or name == "deny":
                        result = json.loads(output)
                        assert result.get("allowed") is (name != "deny"), (name, result)
                        decisions[name] = result["allowed"]
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
    results = [verify(locale, args.toolchain, env) for locale in ("en", "zh")]
    if args.report:
        args.report.write_text(json.dumps(results, indent=2) + "\n")
    print("PASS English and Chinese quick-start commands against native PMS/ADS", flush=True)


if __name__ == "__main__":
    main()
