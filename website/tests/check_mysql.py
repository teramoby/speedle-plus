#!/usr/bin/env python3
"""Execute the paired MySQL guides with owned Docker volumes and native services."""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]
PAGE = ROOT / "docs/hugo/content/docs/store/mysql.md"
IMAGE = "mysql:8.4@sha256:a0225d1ea6631c8bc2385dc4c2faba17bbf1834ecf369e1659a9a382bd8d5dd7"
NAMES = ["build", "database", "ready", "accounts", "configuration", "pms", "tables",
         "ads", "policy", "allow", "deny", "update", "schema", "revoke-ddl"]


def steps(locale):
    page = PAGE if locale == "en" else PAGE.with_name("mysql.zh.md")
    text = page.read_text()
    found = re.findall(r"<!-- mysql-step: ([a-z-]+) -->\s*```bash\n(.*?)\n```", text, re.S)
    assert [name for name, _ in found] == NAMES, (page, "missing or reordered steps")
    assert len(found) == len(re.findall(r"^```bash$", text, re.M)), (page, "unmarked command")
    for name, code in found:
        result = subprocess.run(["bash", "-n"], input=code, text=True, capture_output=True)
        assert result.returncode == 0, (page, name, result.stderr)
    return dict(found)


def static_checks():
    commands = steps("en")
    assert commands == steps("zh"), "MySQL guide command drift between languages"
    quick_start = (ROOT / "docs/hugo/content/quick-start.md").read_text()
    build = re.search(r"<!-- quick-start: build -->\s*```bash\n(.*?)\n```", quick_start, re.S)[1]
    assert build.startswith(commands["build"]), "MySQL build differs from exercised Quick Start"
    assert "--publish 127.0.0.1:3306:3306" in commands["database"]
    assert commands["database"].endswith("mysql:8.4")
    print("PASS bilingual MySQL commands, Bash syntax and source-build instructions", flush=True)


def port():
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
    if process.returncode != 0:
        raise RuntimeError("native guide service exited unsuccessfully")


def verify(locale, binaries, parent, environment):
    commands = steps(locale)
    fixture = "speedle-mysql-guide-" + secrets.token_hex(6)
    volume = fixture + "-data"
    label = environment.get("GITHUB_RUN_ID", fixture) + "-" + environment.get("GITHUB_RUN_ATTEMPT", "1")
    credentials = {sample: secrets.token_hex(24) for sample in
                   ("SpeedleDemoRoot84", "SpeedleDemoPms84", "SpeedleDemoAds84")}
    ports = {6733: port(), 6734: port()}
    while ports[6734] == ports[6733]:
        ports[6734] = port()
    processes, streams, logs = {}, [], []

    with tempfile.TemporaryDirectory(prefix="guide-" + locale + "-", dir=parent) as temporary:
        work = Path(temporary)
        (work / "bin").symlink_to(binaries, target_is_directory=True)

        def run(command, timeout=60, check=True):
            result = subprocess.run(["bash", "-euc", command], cwd=work, env=environment,
                                    capture_output=True, text=True, timeout=timeout)
            if check and result.returncode:
                raise RuntimeError("guide command failed: " + result.stdout + result.stderr)
            return result

        def local(command):
            command = command.replace("speedle-mysql", fixture)
            for sample, password in credentials.items():
                command = command.replace(sample, password)
            for original, replacement in ports.items():
                command = command.replace("127.0.0.1:" + str(original), "127.0.0.1:" + str(replacement))
            return command

        def start(kind):
            path = work / (kind + ".log")
            log = path.open("a")
            streams.append(log)
            if path not in logs:
                logs.append(path)
            process = subprocess.Popen(["bash", "-euc", "exec " + local(commands[kind])], cwd=work,
                                       env=environment, stdout=log, stderr=log)
            processes[kind] = process
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise RuntimeError(kind + " exited before readiness")
                try:
                    with socket.create_connection(("127.0.0.1", ports[6733 if kind == "pms" else 6734]), timeout=.5):
                        return
                except OSError:
                    time.sleep(.1)
            raise RuntimeError(kind + " readiness timed out")

        def decision(command, allowed, reason):
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                result = run(local(command), check=False)
                if result.returncode == 0:
                    value = json.loads(result.stdout)
                    if value == {"allowed": allowed, "reason": reason}:
                        return
                time.sleep(.1)
            raise RuntimeError("guide authorization decision did not converge")

        try:
            subprocess.run(["docker", "volume", "create", "--label", "com.speedle.mysql.run=" + label, volume],
                           check=True, capture_output=True, text=True)
            database = local(commands["database"]).replace("127.0.0.1:3306:3306", "127.0.0.1::3306")
            database = database.replace("--detach", "--detach --label com.speedle.mysql.run=" + label)
            run(database.replace("mysql:8.4", IMAGE), timeout=180)
            mapped = subprocess.run(["docker", "port", fixture, "3306/tcp"], check=True,
                                    capture_output=True, text=True).stdout.strip().rsplit(":", 1)[1]
            ports[3306] = int(mapped)
            run(local(commands["ready"]), timeout=120)
            run(local(commands["accounts"]))
            run(local(commands["configuration"]))
            start("pms")
            tables = set(run(local(commands["tables"])).stdout.splitlines()[1:])
            assert tables == {"speedle_services", "speedle_policies", "speedle_role_policies",
                              "speedle_functions", "speedle_store_meta"}, tables
            start("ads")
            service = json.loads(run(local(commands["policy"])).stdout)
            assert service["name"] == "mysql-demo" and service["policies"][0]["name"] == "alice-read", service
            policy_id = service["policies"][0]["id"]
            assert isinstance(policy_id, str) and policy_id, service
            decision(commands["allow"], True, 0)
            decision(commands["deny"], False, 3)
            run(local(commands["update"]))
            decision(commands["allow"], False, 1)
            for process in processes.values():
                stop(process)
            start("pms")
            start("ads")
            decision(commands["allow"], False, 1)
            # Exercise the alternative provisioning instructions with accounts
            # that no longer have CREATE or REFERENCES privileges.
            run(local(commands["schema"]).replace("pkg/store/mysql/schema.sql", str(ROOT / "pkg/store/mysql/schema.sql")))
            run(local(commands["revoke-ddl"]))
            for process in processes.values():
                stop(process)
            for kind in ("pms", "ads"):
                path = work / "mysql-demo" / (kind + ".json")
                config = json.loads(path.read_text())
                config["storeConfig"]["storeProps"]["MySQLInitSchema"] = False
                path.write_text(json.dumps(config))
                start(kind)
            decision(commands["allow"], False, 1)
            # PMS can still write using only its documented DML privileges.
            run(local("curl -fsS -X DELETE http://127.0.0.1:6733/policy-mgmt/v1/service/mysql-demo/policy/" + policy_id))
            print("PASS MySQL guide " + locale + ": accounts, auto schema, read-only ADS, allow/deny, live update, restart, no-DDL provisioning", flush=True)
        except BaseException as error:
            details = str(error) + "\n" + "\n".join(path.read_text(errors="replace") for path in logs)
            for password in credentials.values():
                details = details.replace(password, "[redacted]")
            raise RuntimeError(details) from None
        finally:
            cleanup_error = False
            for process in processes.values():
                try:
                    stop(process)
                except RuntimeError:
                    cleanup_error = True
            for stream in streams:
                stream.close()
            if any("WARNING: DATA RACE" in path.read_text(errors="replace") for path in logs):
                cleanup_error = True
            removed = subprocess.run(["docker", "rm", "--force", fixture], capture_output=True)
            removed_volume = subprocess.run(["docker", "volume", "rm", volume], capture_output=True)
            if removed.returncode or removed_volume.returncode or cleanup_error:
                raise RuntimeError("MySQL guide process/race/owned-fixture cleanup failed")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--static-only", action="store_true")
    parser.add_argument("--binaries", type=Path)
    args = parser.parse_args()
    static_checks()
    if args.static_only:
        return
    environment = {key: value for key, value in os.environ.items() if not key.startswith("SPDL_")}
    def interrupted(signum, frame):
        raise RuntimeError("MySQL guide interrupted")
    signal.signal(signal.SIGTERM, interrupted)
    with tempfile.TemporaryDirectory(prefix="speedle-mysql-docs-") as temporary:
        parent = Path(temporary)
        binaries = args.binaries.resolve() if args.binaries else parent / "bin"
        if not args.binaries:
            binaries.mkdir()
            subprocess.run(["go", "build", "-race", "-o", str(binaries) + "/", "./cmd/spctl", "./cmd/speedle-pms", "./cmd/speedle-ads"],
                           cwd=ROOT, env=environment, check=True)
        for locale in ("en", "zh"):
            verify(locale, binaries, parent, environment)


if __name__ == "__main__":
    main()
