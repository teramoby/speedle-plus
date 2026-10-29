#!/usr/bin/env python3
"""Run the existing Go/API suites against an owned MySQL Docker fixture.

This is executed by the GitHub Actions runner, not a resident host service.
The database, native PMS/ADS, CLI config and every listen port belong to this run.
"""
import json
import grp
import os
import pwd
from pathlib import Path
import secrets
import shlex
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
from store_regression import run_regression

ROOT = Path(__file__).resolve().parents[2]
IMAGE = "mysql:8.4@sha256:a0225d1ea6631c8bc2385dc4c2faba17bbf1834ecf369e1659a9a382bd8d5dd7"
CLIENT = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def ensure_docker_access():
    """Refresh an existing group membership in a long-running runner process.

    Installing Docker/adduser can happen after the runner service starts. sg
    starts a child with the account's already-authorized docker group; it does
    not edit /etc/group, change socket permissions, or restart a host service.
    """
    probe = subprocess.run(["docker", "info"], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    if probe.returncode == 0:
        return
    user = pwd.getpwuid(os.getuid())
    current = os.getgroups()
    print(f"Docker preflight: runner={os.environ.get('RUNNER_NAME', 'local')}, user={user.pw_name}, groups={current}", flush=True)
    try:
        docker_group = grp.getgrnam("docker")
    except KeyError:
        raise RuntimeError("Docker daemon unavailable and the host has no docker group")
    eligible = os.getgrouplist(user.pw_name, user.pw_gid)
    if (docker_group.gr_gid in eligible and docker_group.gr_gid not in current
            and os.environ.get("SPEEDLE_DOCKER_GROUP_REFRESHED") != "1"):
        print("Refreshing the runner account's existing docker group for this job.", flush=True)
        environment = dict(os.environ, SPEEDLE_DOCKER_GROUP_REFRESHED="1")
        command = "exec " + shlex.join([sys.executable, str(Path(__file__).resolve()), *sys.argv[1:]])
        os.execvpe("sg", ["sg", "docker", "-c", command], environment)
    raise RuntimeError("runner cannot access Docker; " + probe.stderr.strip())


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
    ensure_docker_access()
    if "--cleanup" in sys.argv:
        label = os.environ["GITHUB_RUN_ID"] + "-" + os.environ.get("GITHUB_RUN_ATTEMPT", "1")
        owned = run(["docker", "ps", "--all", "--quiet", "--filter", "label=com.speedle.mysql.run=" + label], capture_output=True).stdout.split()
        if owned:
            run(["docker", "rm", "--force", *owned])
        return 0
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
            print(f"MySQL ready: container={container}, loopback_port={port}", flush=True)
            environment["SPEEDLE_MYSQL_TEST_DSN"] = f"root:{password}@tcp(127.0.0.1:{port})/speedle_test"
            config = {"storeConfig": {"storeType": "mysql", "storeProps": {
                "MySQLDSN": environment["SPEEDLE_MYSQL_TEST_DSN"],
                "MySQLInitSchema": True, "MySQLPollInterval": "20ms", "MySQLQueryTimeout": "30s",
            }}, "enableWatch": True}
            failed, suites = run_regression(environment, config, results)
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
