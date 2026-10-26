# Adapted from Pavu a7b458e; modified for Speedle+. Apache-2.0; see .github/codex/PAVU-LICENSE.txt.
"""Exercise the workflows' actual prompt construction without calling Ark."""

import base64
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = (
    ("reviewbot.yml", "Run ReviewBot with Ark", "reviewbot-input.txt"),
    ("reviewbot-command.yml", "Run comment command with Ark", "reviewbot-command-input.txt"),
)


def workflow_script(workflow, step):
    # Read only the named YAML literal block. No YAML package is needed.
    # No YAML package, credentials, model call, or PR code execution is needed.
    text = (ROOT / ".github/workflows" / workflow).read_text()
    block = text.split(f"      - name: {step}\n", 1)[1].split("        run: |\n", 1)[1]
    lines = []
    for line in block.splitlines():
        if line and not line.startswith("          "):
            break
        lines.append(line[10:])
    return "\n".join(lines)


def prompt_script(workflow, step):
    # Stop before request creation: no credentials, model call, or PR execution.
    script, separator, _ = workflow_script(workflow, step).partition("\ninput_limit=750000")
    if not separator:
        raise ValueError("ReviewBot input/request boundary not found")
    return script


class ReviewBotDiffTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.name", "ReviewBot fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.git("config", "commit.gpgsign", "false")
        self.write(".github/codex/prompts/reviewbot.md", "TRUSTED_INITIAL_PROMPT\n")
        self.write(".github/codex/prompts/reviewbot-command.md", "TRUSTED_COMMAND_POLICY\n")
        self.write("AGENTS.md", "TRUSTED_INITIAL_POLICY\n")
        self.write("main-only.txt", "initial base content\n")
        self.write("feature.txt", "initial feature content\n")
        self.fork = self.commit("Common ancestor")
        self.git("checkout", "-qb", "feature")
        self.write("feature.txt", "PR feature change\n")
        self.head = self.commit("Feature change")
        self.git("checkout", "-q", "main")
        self.write("main-only.txt", "MAIN_ONLY_CHANGE_MUST_NOT_BE_REVIEWED\n")
        self.write(".github/codex/prompts/reviewbot.md", "TRUSTED_CURRENT_PROMPT\n")
        self.write("AGENTS.md", "TRUSTED_CURRENT_POLICY\n")
        self.base = self.commit("Main advances after the fork")

    def git(self, *args, check=True):
        return subprocess.run(
            ["git", *args], cwd=self.repo, check=check, text=True, capture_output=True
        ).stdout.strip()

    def write(self, path, content):
        destination = self.repo / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(content)

    def commit(self, message):
        self.git("add", ".")
        self.git("commit", "-qm", message)
        return self.git("rev-parse", "HEAD")

    def construct_prompt(self, workflow, step, filename, base, head, mode="review", check=True):
        # Mirror automatic head checkout and command trusted-workflow checkout.
        self.git("checkout", "-q", "--detach", head if workflow == "reviewbot.yml" else base)
        output = self.root / f"{workflow}-{mode}"
        output.mkdir(exist_ok=True)
        encode = lambda value: base64.b64encode(value.encode()).decode()
        env = {
            **os.environ,
            "RUNNER_TEMP": str(output),
            "BASE_SHA": base,
            "HEAD_SHA": head,
            "TRUSTED_SHA": base,
            "COMMAND": mode,
            "REQUEST_B64": encode("Review the synthetic feature change."),
            "CONTEXT_B64": encode("{}"),
            "HISTORY_B64": "",
        }
        result = subprocess.run(
            ["bash", "-c", prompt_script(workflow, step)],
            cwd=self.repo, env=env, check=check, capture_output=True, text=True,
        )
        path = output / filename
        return result, path.read_text() if path.exists() else ""

    def assert_feature_only(self, prompt):
        stat = prompt.split("## Unified diff" if "## Unified diff" in prompt
                            else "## Untrusted unified diff", 1)[0]
        self.assertIn("feature.txt", stat)
        self.assertIn("diff --git a/feature.txt b/feature.txt", prompt)
        self.assertIn("+PR feature change", prompt)
        self.assertNotIn("main-only.txt", prompt)
        self.assertNotIn("MAIN_ONLY_CHANGE_MUST_NOT_BE_REVIEWED", prompt)

    def test_advanced_base_excludes_main_only_changes_in_stat_and_patch(self):
        # Establish that the previous two-commit diff reproduces the regression.
        old_diff = self.git("diff", self.base, self.head)
        self.assertIn("main-only.txt", old_diff)
        for workflow, step, filename in WORKFLOWS:
            modes = ("review", "recheck", "explain") if "command" in workflow else ("review",)
            for mode in modes:
                with self.subTest(workflow=workflow, mode=mode):
                    _, prompt = self.construct_prompt(workflow, step, filename, self.base, self.head, mode)
                    self.assert_feature_only(prompt)
                    self.assertIn("TRUSTED_CURRENT_PROMPT", prompt)
                    self.assertIn("TRUSTED_CURRENT_POLICY", prompt)
                    self.assertNotIn("TRUSTED_INITIAL_POLICY", prompt)
                    self.assertIn(f"Merge base: `{self.fork}`", prompt)
                    print(f"{workflow} ({mode}): stat and patch contain only feature.txt; main-only.txt excluded")

    def test_base_ancestor_keeps_feature_diff(self):
        for workflow, step, filename in WORKFLOWS:
            with self.subTest(workflow=workflow):
                _, prompt = self.construct_prompt(workflow, step, filename, self.fork, self.head)
                self.assert_feature_only(prompt)

    def test_merged_base_keeps_feature_diff(self):
        self.git("checkout", "-q", "feature")
        self.git("merge", "-qm", "Merge current main", "main")
        head = self.git("rev-parse", "HEAD")
        for workflow, step, filename in WORKFLOWS:
            with self.subTest(workflow=workflow):
                _, prompt = self.construct_prompt(workflow, step, filename, self.base, head)
                self.assert_feature_only(prompt)
                self.assertIn(f"Merge base: `{self.base}`", prompt)

    def test_unrelated_histories_fail_before_prompt_construction(self):
        self.git("checkout", "-q", "--orphan", "unrelated")
        self.git("rm", "-rfq", ".")
        self.write("unrelated.txt", "unrelated history\n")
        head = self.commit("Unrelated root")
        for workflow, step, filename in WORKFLOWS:
            with self.subTest(workflow=workflow):
                result, prompt = self.construct_prompt(workflow, step, filename, self.base, head, check=False)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(prompt, "")


if __name__ == "__main__":
    unittest.main()
