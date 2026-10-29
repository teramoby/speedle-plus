#!/usr/bin/env python3
"""Run the existing Go/API suites against an owned MySQL Docker fixture.

This is executed by the GitHub Actions runner, not a resident host service.
The database, native PMS/ADS, CLI config and every listen port belong to this run.
"""
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
IMAGE = "mysql:8.4@sha256:a0225d1ea6631c8bc2385dc4c2faba17bbf1834ecf369e1659a9a382bd8d5dd7"
CLIENT = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, text=True, **kwargs)


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def stop(process):
    if process.poll() is None:
        process.send_signal(signal.SIGINT)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)


def main():
    results = Path(os.environ.get("RESULTS_DIR", "test-results")).resolve()
    results.mkdir(parents=True, exist_ok=True)
    container = "speedle-mysql-nightly-" + secrets.token_hex(6)
    password = secrets.token_hex(24)
    failed = False
    processes, logs = [], []
    suites = []
    environment = dict(os.environ)
    # Do not inherit a developer's CLI config, credentials or alternate backend.
    for key in list(environment):
        if key.startswith("SPDL_"):
            environment.pop(key)
    environment.update({key: "" for key in (
        "SP_APP_NAME", "PMS_ADMIN_TOKEN", "ADS_ADMIN_TOKEN", "CA_LOCATION",
        "CERT_LOCATION", "KEY_LOCATION", "SPEEDLE_CLI_COVERPROFILE",
    )})
    environment.update(STORE_TYPE="mysql", SPEEDLE_REQUIRE_MYSQL="1")

    def suite(name, args, env=None):
        nonlocal failed
        with (results / (name + ".json")).open("w") as out, (results / (name + ".stderr")).open("w") as err:
            code = subprocess.run(args, cwd=ROOT, env=env or environment, stdout=out, stderr=err).returncode
        failed |= code != 0
        suites.append((name, code))
        print(f"{name}: exit {code}", flush=True)
        return code

    def interrupted(signum, frame):
        raise SystemExit("runner interrupted MySQL fixture")

    signal.signal(signal.SIGTERM, interrupted)
    try:
        with tempfile.TemporaryDirectory(prefix="speedle-mysql-nightly-") as temporary:
            work = Path(temporary)
            with (results / "mysql-docker.log").open("w") as log:
                run(["docker", "run", "--rm", "--detach", "--name", container,
                     "--label", "com.speedle.fixture=mysql-nightly",
                     "--label", "com.speedle.mysql.run=" + environment.get("GITHUB_RUN_ID", container) + "-" + environment.get("GITHUB_RUN_ATTEMPT", "1"),
                     "--env", "MYSQL_ROOT_PASSWORD=" + password,
                     "--env", "MYSQL_ROOT_HOST=%", "--env", "MYSQL_DATABASE=speedle_test",
                     "--publish", "127.0.0.1::3306", IMAGE], stdout=log, stderr=log)
                port = run(["docker", "port", container, "3306/tcp"], capture_output=True).stdout.strip().rsplit(":", 1)[1]
                deadline = time.monotonic() + 120
                while subprocess.run(["docker", "exec", container, "mysqladmin", "ping", "--host=127.0.0.1",
                                      "--user=root", "--password=" + password, "--silent"],
                                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
                    if time.monotonic() > deadline:
                        raise RuntimeError("owned MySQL container did not become ready")
                    time.sleep(1)
            environment["SPEEDLE_MYSQL_TEST_DSN"] = f"root:{password}@tcp(127.0.0.1:{port})/speedle_test"
            # Native binaries and go test itself both run under the race detector.
            binaries = work / "bin"
            binaries.mkdir()
            for name in ("speedle-pms", "speedle-ads", "spctl"):
                run(["go", "build", "-race", "-o", str(binaries / name), "./cmd/" + name], env=environment)
            listed = run(["go", "list", "./pkg/...", "./cmd/...", "./api/..."], env=environment, capture_output=True).stdout.splitlines()
            packages = [p for p in listed if not any(x in p for x in ("/samples", "/pkg/store/mongodb", "/pkg/logging/demo")) and not p.endswith("/pb")]
            if not packages:
                raise RuntimeError("no Go packages discovered")
            suite("mysql-go", ["go", "test", "-race", "-json", "-p", "1", "-count=1", "-timeout", "600s", *packages])

            # Existing runtime_test and runtime_cache_test files are excluded
            # from ordinary go test. Execute every one against actual PMS/ADS.
            pms_port, ads_port = free_port(), free_port()
            while ads_port == pms_port:
                ads_port = free_port()
            config = {
                "storeConfig": {"storeType": "mysql", "storeProps": {
                    "MySQLDSN": environment["SPEEDLE_MYSQL_TEST_DSN"],
                    "MySQLInitSchema": True, "MySQLPollInterval": "20ms", "MySQLQueryTimeout": "30s",
                }}, "enableWatch": True,
            }
            config_file = work / "mysql.json"
            config_file.write_text(json.dumps(config))
            config_file.chmod(0o600)
            for kind, listen_port in (("pms", pms_port), ("ads", ads_port)):
                log = (results / ("mysql-" + kind + ".log")).open("w")
                logs.append(log)
                process = subprocess.Popen([str(binaries / ("speedle-" + kind)), "--config-file", str(config_file),
                                            "--insecure=true", "--endpoint", f"127.0.0.1:{listen_port}"],
                                           cwd=work, env=environment, stdout=log, stderr=log)
                processes.append(process)
                deadline = time.monotonic() + 30
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(f"MySQL {kind.upper()} exited before readiness")
                    try:
                        with socket.create_connection(("127.0.0.1", listen_port), timeout=1):
                            break
                    except OSError:
                        if time.monotonic() > deadline:
                            raise RuntimeError(f"MySQL {kind.upper()} did not listen")
                        time.sleep(0.1)
            runtime_env = dict(environment, SP_APP_NAME=str(binaries / "spctl"),
                               PMS_ENDPOINT=f"http://127.0.0.1:{pms_port}",
                               ADS_ENDPOINT=f"http://127.0.0.1:{ads_port}", SPEEDLE_MYSQL_EXTERNAL="1")
            # The current CLI's endpoint flags are passed by the test client.
            with CLIENT.open(urllib.request.Request(runtime_env["PMS_ENDPOINT"] + "/policy-mgmt/v1/service", method="DELETE"), timeout=10):
                pass
            prepared = suite("mysql-ads-prepare", ["go", "test", "-race", "-json", "-count=1", "-timeout", "600s",
                                                   "-tags", "runtime_test_prepare", "./pkg/svcs/adsrest", "-run", "^TestPrepareCheckEnvironment$"], runtime_env)
            if prepared == 0:
                time.sleep(0.2)
                suite("mysql-ads-runtime", ["go", "test", "-race", "-json", "-count=1", "-timeout", "1200s",
                                             "-tags", "runtime_test,runtime_cache_test", "./pkg/svcs/adsrest"], runtime_env)
            else:
                (results / "mysql-ads-runtime.stderr").write_text("Runtime suite blocked by failed preparation.\n")
                failed = True
            if any(p.poll() is not None for p in processes):
                raise RuntimeError("native PMS/ADS exited during runtime tests")
    except BaseException as error:
        # Do not print a command line or exception containing the generated DSN.
        print("MySQL full regression failed: " + str(error).replace(password, "[redacted]"), file=sys.stderr, flush=True)
        failed = True
        (results / "mysql-fixture.stderr").write_text(str(error).replace(password, "[redacted]") + "\n")
    finally:
        for process in reversed(processes):
            stop(process)
            if process.returncode != 0:
                failed = True
        for log in logs:
            log.close()
        subprocess.run(["docker", "rm", "--force", container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        # Sanitize artifacts before upload, including output from unexpected failures.
        for path in results.glob("mysql-*"):
            if path.is_file():
                data = path.read_text(errors="replace")
                if "WARNING: DATA RACE" in data:
                    failed = True
                path.write_text(data.replace(password, "[redacted]"))
        counts = {"pass": 0, "fail": 0, "skip": 0}
        for path in results.glob("mysql-*.json"):
            for line in path.read_text().splitlines():
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                if event.get("Test") and event.get("Action") in counts:
                    counts[event["Action"]] += 1
        summary = f"MySQL full regression: {counts}; suites={suites}; failed={failed}\n"
        (results / "mysql-summary.txt").write_text(summary)
        print(summary, end="", flush=True)
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(main())
