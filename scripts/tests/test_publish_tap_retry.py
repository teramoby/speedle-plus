"""The release publisher recreates a retained Homebrew branch from master."""

import hashlib
import importlib.util
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HOMEBREW = ROOT / "deployment" / "homebrew"
UPDATER_SPEC = importlib.util.spec_from_file_location(
    "update_formulas", HOMEBREW / "update-formulas.py")
update_formulas = importlib.util.module_from_spec(UPDATER_SPEC)
UPDATER_SPEC.loader.exec_module(update_formulas)


def run(command, cwd, env):
    completed = subprocess.run(command, cwd=cwd, env=env, check=True, text=True, capture_output=True)
    return completed


class PublishTapRetryTest(unittest.TestCase):
    def test_retained_branch_without_an_open_pull_request_starts_from_master(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self._run_case(root, open_pr=False)

    def test_open_pull_request_keeps_the_existing_branch(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self._run_case(root, open_pr=True)

    def _run_case(self, root, open_pr):
        source_bare = root / "source.git"
        tap_bare = root / "tap.git"
        workspace = root / "workspace"
        homebrew = root / "homebrew"
        bin_dir = root / "bin"
        bin_dir.mkdir()
        gh_log = root / "gh.log"
        shutil.copytree(HOMEBREW, homebrew)
        self._write_checksums(homebrew)
        self._git("init", "--bare", "-b", "master", source_bare)
        self._git("init", "--bare", "-b", "main", tap_bare)
        self._git("clone", source_bare, workspace)
        homebrew_dest = workspace / "deployment" / "homebrew"
        shutil.copytree(homebrew, homebrew_dest)
        self._git("add", "deployment", cwd=workspace)
        self._git("commit", "-m", "seed", cwd=workspace)
        self._git("push", "origin", "master", cwd=workspace)
        self._git("checkout", "-b", "homebrew-v1.7.0", cwd=workspace)
        (workspace / "RETAINED").write_text("old release branch\n")
        self._git("add", "RETAINED", cwd=workspace)
        self._git("commit", "-m", "retained release branch", cwd=workspace)
        self._git("push", "origin", "homebrew-v1.7.0", cwd=workspace)
        self._git("checkout", "master", cwd=workspace)

        tap_seed = root / "tap-seed"
        self._git("clone", tap_bare, tap_seed)
        (tap_seed / "README.md").write_text("tap\n")
        self._git("add", "README.md", cwd=tap_seed)
        self._git("commit", "-m", "tap readme", cwd=tap_seed)
        self._git("push", "origin", "main", cwd=tap_seed)

        gh = bin_dir / "gh"
        number = "7" if open_pr else ""
        gh.write_text(
            "\n".join([
                "#!/bin/bash",
                "set -euo pipefail",
                'printf "%s\\n" "$*" >> "$GH_LOG"',
                'if [[ "$1" == "auth" ]]; then exit 0; fi',
                'if [[ "$1" == "repo" && "$2" == "clone" ]]; then',
                '  git clone "$TAP_BARE" "$4"',
                "  exit 0",
                "fi",
                'if [[ "$1" == "pr" && "$2" == "list" ]]; then',
                f'  printf "%s" "{number}"',
                "  exit 0",
                "fi",
                'if [[ "$1" == "pr" && "$2" == "create" ]]; then exit 0; fi',
                'echo "unexpected gh $*" >&2',
                "exit 1",
                "",
            ])
        )
        gh.chmod(0o755)
        env = os.environ.copy()
        env.update({
            "PATH": f"{bin_dir}{os.pathsep}{env['PATH']}",
            "GH_LOG": str(gh_log),
            "TAP_BARE": str(tap_bare),
            "HOMEBREW_TAP_TOKEN": "fixture-token",
            "GITHUB_TOKEN": "fixture-source-token",
            "GITHUB_REPOSITORY": "teramoby/speedle-plus",
            "GITHUB_WORKSPACE": str(workspace),
            "VERSION": "v1.7.0",
            "CHECKSUMS": str(homebrew / "checksums.txt"),
            "GIT_AUTHOR_NAME": "Test User",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test User",
            "GIT_COMMITTER_EMAIL": "test@example.com",
        })
        completed = subprocess.run(
            ["bash", str(homebrew / "publish-tap.sh")],
            cwd=workspace,
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )
        combined = completed.stdout + completed.stderr
        self.assertNotIn("fixture-token", combined)
        self.assertNotIn("fixture-source-token", combined)
        self.assertEqual(completed.returncode, 0, combined)
        listed = subprocess.run(
            ["git", "ls-remote", "--heads", str(source_bare), "homebrew-v1.7.0"],
            check=True, text=True, capture_output=True,
        ).stdout
        log = gh_log.read_text()
        if open_pr:
            self.assertIn("homebrew-v1.7.0", listed)
            self.assertNotIn("push origin --delete", log)
            self.assertIn("Pull request #7 already tracks", completed.stdout)
        else:
            self.assertEqual(listed.strip(), "")
            self.assertIn("deployment/homebrew already matches v1.7.0.", completed.stdout)
            self.assertNotIn("pr create", log.split("teramoby/speedle-plus")[-1])

    def _git(self, *args, cwd=None):
        command = ["git", *args]
        env = os.environ.copy()
        env.update({
            "GIT_AUTHOR_NAME": "Test User",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test User",
            "GIT_COMMITTER_EMAIL": "test@example.com",
        })
        subprocess.run(command, cwd=cwd, env=env, check=True, text=True, capture_output=True)

    def _write_checksums(self, homebrew):
        assets = []
        for formula in (homebrew / "Formula").glob("*.rb"):
            for line in formula.read_text().splitlines():
                match = re.search(r"/download/v[^/]+/(?P<asset>[A-Za-z0-9._-]+)\"", line)
                if match and match.group("asset") != "checksums.txt":
                    assets.append(match.group("asset"))
        body = "".join(
            f"{index:064x}  {asset}\n" for index, asset in enumerate(sorted(set(assets)), 1))
        path = homebrew / "checksums.txt"
        path.write_text(body)
        digest = hashlib.sha256(body.encode()).hexdigest()
        update_formulas.update_formulas(
            homebrew / "Formula", "v1.7.0", update_formulas.load_checksums(path), digest)
