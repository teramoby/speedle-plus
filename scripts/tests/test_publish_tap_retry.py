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

    def test_existing_tap_branch_is_fetched_into_a_single_branch_clone(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self._run_case(root, open_pr=False, existing_tap_branch=True)

    def test_release_pr_bumps_the_quick_start_pin(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self._run_case(root, open_pr=False, quick_start="v1.6.0")

    def _run_case(self, root, open_pr, existing_tap_branch=False, quick_start=None):
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
        if quick_start:
            self._write_quick_start(workspace, quick_start)
        self._git("add", "deployment", cwd=workspace)
        if quick_start:
            self._git("add", "docs", cwd=workspace)
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
        if existing_tap_branch:
            self._git("checkout", "-b", "homebrew-v1.7.0", cwd=tap_seed)
            (tap_seed / "README.md").write_text("old tap branch\n")
            self._git("add", "README.md", cwd=tap_seed)
            self._git("commit", "-m", "old tap branch", cwd=tap_seed)
            self._git("push", "origin", "homebrew-v1.7.0", cwd=tap_seed)
            self._git("checkout", "main", cwd=tap_seed)

        gh = bin_dir / "gh"
        number = "7" if open_pr else ""
        gh.write_text(
            "\n".join([
                "#!/bin/bash",
                "set -euo pipefail",
                'printf "%s\\n" "$*" >> "$GH_LOG"',
                'if [[ "$1" == "auth" ]]; then exit 0; fi',
                'if [[ "$1" == "repo" && "$2" == "clone" ]]; then',
                '  if [[ "${5:-}" == "--" ]]; then',
                '    git clone --no-local "${@:6}" "$TAP_BARE" "$4"',
                "  else",
                '    git clone "$TAP_BARE" "$4"',
                "  fi",
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
        elif quick_start:
            self.assertIn("homebrew-v1.7.0", listed)
            self.assertIn("Quick Start install pin set to v1.7.0", completed.stdout)
            self.assertIn("Quick Start modification time set to", completed.stdout)
            self.assertNotIn("deployment/homebrew already matches", completed.stdout)
            self._assert_quick_start_pin(source_bare, "v1.7.0", quick_start)
        else:
            self.assertEqual(listed.strip(), "")
            self.assertIn("deployment/homebrew already matches v1.7.0.", completed.stdout)
            self.assertNotIn("pr create", log.split("teramoby/speedle-plus")[-1])
        if existing_tap_branch:
            self.assertNotIn("unknown revision", combined)
            self.assertIn("Pushed homebrew-v1.7.0", completed.stdout)
            readme = subprocess.run(
                ["git", "--git-dir", str(tap_bare), "show", "homebrew-v1.7.0:README.md"],
                check=True, text=True, capture_output=True,
            ).stdout
            self.assertNotEqual(readme, "old tap branch\n")
            self.assertIn("brew trust teramoby/tap", readme)
            subprocess.run(
                ["git", "--git-dir", str(tap_bare), "cat-file", "-e", "homebrew-v1.7.0:Formula/spctl.rb"],
                check=True, text=True, capture_output=True,
            )

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

    def _write_quick_start(self, workspace, version):
        pages = {
            "docs/hugo/content/quick-start.md":
                'GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@' + version + "\n"
                f"`{version}` is the latest release tag at the time of writing.\n",
            "docs/hugo/content/quick-start.zh.md":
                'GOBIN="$PWD/bin" go install github.com/teramoby/speedle-plus/cmd/...@' + version + "\n"
                f"`{version}` 是撰写本文时的最新发布标签。\n",
            "docs/quick-start/index.html":
                '<meta property="article:modified_time" content="2020-01-01T00:00:00&#43;00:00">\n'
                "go install github.com/teramoby/speedle-plus/cmd/...@" + version + "\n"
                f"<code>{version}</code> is the latest release tag at the time of writing.\n",
            "docs/zh/quick-start/index.html":
                '<meta property="article:modified_time" content="2020-01-01T00:00:00&#43;00:00">\n'
                "go install github.com/teramoby/speedle-plus/cmd/...@" + version + "\n"
                f"<code>{version}</code> 是撰写本文时的最新发布标签。\n",
            "docs/zh/index.xml":
                "go install github.com/teramoby/speedle-plus/cmd/...@" + version + "\n",
            "docs/en/sitemap.xml":
                "<loc>https://engine.speedle.io/quick-start/</loc>\n"
                "<lastmod>2020-01-01T00:00:00+00:00</lastmod>\n",
            "docs/zh/sitemap.xml":
                "<loc>https://engine.speedle.io/zh/quick-start/</loc>\n"
                "<lastmod>2020-01-01T00:00:00+00:00</lastmod>\n",
        }
        for rel, text in pages.items():
            path = workspace / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text)

    def _assert_quick_start_pin(self, source_bare, version, previous):
        def show(path):
            return subprocess.run(
                ["git", "--git-dir", str(source_bare), "show", f"homebrew-v1.7.0:{path}"],
                check=True, text=True, capture_output=True,
            ).stdout

        author = subprocess.run(
            ["git", "--git-dir", str(source_bare), "log", "-1", "--format=%aI", "homebrew-v1.7.0"],
            check=True, text=True, capture_output=True,
        ).stdout.strip()
        message = subprocess.run(
            ["git", "--git-dir", str(source_bare), "log", "-1", "--format=%B", "homebrew-v1.7.0"],
            check=True, text=True, capture_output=True,
        ).stdout
        self.assertIn("Signed-off-by:", message)
        self.assertIn("Quick Start pin", message)
        for path in (
            "docs/hugo/content/quick-start.md",
            "docs/hugo/content/quick-start.zh.md",
            "docs/quick-start/index.html",
            "docs/zh/quick-start/index.html",
            "docs/zh/index.xml",
        ):
            text = show(path)
            self.assertIn(f"cmd/...@{version}", text)
            self.assertNotIn(previous, text)
        html = show("docs/quick-start/index.html")
        self.assertIn(author.replace("+", "&#43;"), html)
        self.assertIn(f"<code>{version}</code>", html)
        self.assertIn(author, show("docs/en/sitemap.xml"))
        self.assertIn(author, show("docs/zh/sitemap.xml"))

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
