#!/usr/bin/env python3
"""Execute the paired MongoDB guides against owned Docker fixtures."""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
import signal
import subprocess
import tempfile
import time
import socket

from check_mysql import port, stop

ROOT = Path(__file__).resolve().parents[2]
PAGE = ROOT / "docs/hugo/content/docs/store/mongodb.md"
NAMES = ["build", "database", "ready", "replica-set", "configuration", "pms", "policy", "ads",
         "allow", "deny", "documents", "update", "backup", "restart"]


def steps(locale):
    path = PAGE if locale == "en" else PAGE.with_name("mongodb.zh.md")
    text = path.read_text()
    found = re.findall(r"<!-- mongodb-step: ([a-z-]+) -->\s*```bash\n(.*?)\n```", text, re.S)
    assert [name for name, _ in found] == NAMES, (path, "missing/reordered guide steps")
    assert len(found) == len(re.findall(r"^```bash$", text, re.M)), (path, "unmarked command")
    for name, code in found:
        result = subprocess.run(["bash", "-n"], input=code, text=True, capture_output=True)
        assert result.returncode == 0, (path, name, result.stderr)
    return dict(found)


def static_checks():
    commands = steps("en")
    assert commands == steps("zh"), "MongoDB guide command drift between languages"
    quick = (ROOT / "docs/hugo/content/quick-start.md").read_text()
    build = re.search(r"<!-- quick-start: build -->\s*```bash\n(.*?)\n```", quick, re.S)[1]
    assert build.startswith(commands["build"]), "MongoDB build differs from exercised Quick Start"
    assert "--publish 127.0.0.1:27017:27017" in commands["database"]
    assert "mongo:7.0.16" in commands["database"] and "--replSet speedle-rs" in commands["database"]
    configuration = json.loads(re.search(r"<<'JSON'\n(.*?)\nJSON", commands["configuration"], re.S)[1])
    assert configuration["storeConfig"]["storeType"] == "mongodb"
    assert "replicaSet=speedle-rs" in configuration["storeConfig"]["storeProps"]["MongoURI"]
    assert configuration["enableWatch"] is True
    for locale in ("en", "zh"):
        path = PAGE if locale == "en" else PAGE.with_name("mongodb.zh.md")
        production = json.loads(re.search(r"```json\n(.*?)\n```", path.read_text(), re.S)[1])
        uri = production["storeConfig"]["storeProps"]["MongoURI"]
        assert "tls=true" in uri and "replicaSet=" in uri and "authSource=" in uri, uri
    print("PASS paired MongoDB commands, configuration, Bash syntax and source-build instructions", flush=True)


def verify(locale, binaries, parent, environment):
    commands = steps(locale)
    fixture = "speedle-mongodb-guide-" + secrets.token_hex(6)
    volume = fixture + "-data"
    label = environment.get("GITHUB_RUN_ID", fixture) + "-" + environment.get("GITHUB_RUN_ATTEMPT", "1")
    ports = {}
    for original in (27017, 6733, 6734):
        chosen = port()
        while chosen in ports.values():
            chosen = port()
        ports[original] = chosen
    database = ports[27017]
    processes, streams, logs = {}, [], []
    with tempfile.TemporaryDirectory(prefix=locale + "-", dir=parent) as temporary:
        work = Path(temporary)
        (work / "bin").symlink_to(binaries, target_is_directory=True)

        def local(command):
            command = command.replace("speedle-mongodb-data", "@VOLUME@").replace("speedle-mongodb", fixture)
            command = command.replace("@VOLUME@", volume)
            # The member address must be the server's own: mongod listens on
            # the chosen port inside the container as well.
            command = command.replace("127.0.0.1:27017:27017", "127.0.0.1:%d:%d" % (database, database))
            command = command.replace("--replSet speedle-rs", "--port %d --replSet speedle-rs" % database)
            command = command.replace("mongosh ", "mongosh --port %d " % database)
            command = command.replace("mongodump ", "mongodump --port %d " % database)
            for original, replacement in ports.items():
                command = command.replace("127.0.0.1:" + str(original), "127.0.0.1:" + str(replacement))
            return command

        def run(command, timeout=60, check=True):
            result = subprocess.run(["bash", "-euc", local(command)], cwd=work, env=environment,
                                    capture_output=True, text=True, timeout=timeout)
            if check and result.returncode:
                raise RuntimeError("guide command failed: " + result.stdout + result.stderr)
            return result

        def start(kind):
            path = work / (kind + ".log")
            stream = path.open("a")
            streams.append(stream)
            if path not in logs:
                logs.append(path)
            process = subprocess.Popen(["bash", "-euc", "exec " + local(commands[kind])], cwd=work,
                                       env=environment, stdout=stream, stderr=stream)
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

        def decision(name, allowed, reason):
            deadline = time.monotonic() + 15
            last = ""
            while time.monotonic() < deadline:
                result = run(commands[name], check=False)
                last = result.stdout + result.stderr
                if result.returncode == 0 and json.loads(result.stdout) == {"allowed": allowed, "reason": reason}:
                    return
                time.sleep(.1)
            raise RuntimeError("MongoDB guide decision did not converge: " + last)

        try:
            database_command = commands["database"].replace("--detach", "--detach --label com.speedle.mongodb-docs.run=" + label)
            run(database_command, timeout=300)
            run(commands["ready"], timeout=120)
            run(commands["replica-set"], timeout=120)
            run(commands["configuration"])
            start("pms")
            service = json.loads(run(commands["policy"]).stdout)
            assert service["name"] == "mongodb-demo" and service["policies"][0]["id"], service
            start("ads")
            decision("allow", True, 0)
            decision("deny", False, 3)
            documents = run(commands["documents"]).stdout
            assert "mongodb-demo" in documents and "alice-read" in documents, documents
            run(commands["update"])
            decision("allow", False, 1)
            run(commands["backup"])
            assert (work / "mongodb-demo/speedle-demo.archive").stat().st_size > 0
            for process in processes.values():
                stop(process)
            run(commands["restart"])
            run(commands["ready"], timeout=120)
            # Step 3's wait for a primary, without initiating again.
            wait = commands["replica-set"][commands["replica-set"].index("until "):]
            run(wait, timeout=120)
            start("pms")
            start("ads")
            decision("allow", False, 1)
            print("PASS MongoDB guide " + locale + ": replica set, shared database, allow/deny, live change stream, documents, backup, container/service restart persistence", flush=True)
        except BaseException as error:
            raise RuntimeError(str(error) + "\n" + "\n".join(path.read_text(errors="replace") for path in logs)) from None
        finally:
            failed = False
            for process in processes.values():
                try:
                    stop(process)
                except RuntimeError:
                    failed = True
            for stream in streams:
                stream.close()
            if any("WARNING: DATA RACE" in path.read_text(errors="replace") for path in logs):
                failed = True
            removed = subprocess.run(["docker", "rm", "--force", fixture], capture_output=True)
            removed_volume = subprocess.run(["docker", "volume", "rm", volume], capture_output=True)
            if failed or removed.returncode or removed_volume.returncode:
                raise RuntimeError("MongoDB guide process/race/owned-fixture cleanup failed")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--static-only", action="store_true")
    parser.add_argument("--binaries", type=Path)
    args = parser.parse_args()
    static_checks()
    if args.static_only:
        return
    environment = {key: value for key, value in os.environ.items() if not key.startswith("SPDL_")}
    environment["NO_PROXY"] = environment.get("NO_PROXY", "") + ",127.0.0.1,localhost,::1"
    def interrupted(signum, frame):
        raise RuntimeError("MongoDB guide interrupted")
    signal.signal(signal.SIGTERM, interrupted)
    with tempfile.TemporaryDirectory(prefix="speedle-mongodb-docs-") as temporary:
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
