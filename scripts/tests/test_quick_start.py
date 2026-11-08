"""Unit checks for the Quick Start install-version freshness guard."""
import importlib.util
import re
from pathlib import Path
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "check_quick_start", ROOT / "website" / "tests" / "check_quick_start.py"
)
QUICK_START = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(QUICK_START)


class LatestInstallVersionTest(unittest.TestCase):
    def test_queries_public_tags_and_accepts_the_highest_version(self):
        env = {"GOTOOLCHAIN": "local"}
        listing = (
            "a\trefs/tags/v1.5.0\n"
            "b\trefs/tags/v1.6.0\n"
            "c\trefs/tags/v1.6.0^{}\n"
            "d\trefs/tags/v1.6\n"
        )
        with mock.patch.object(QUICK_START, "run", return_value=listing) as run:
            QUICK_START.check_latest_install_version(
                "github.com/teramoby/speedle-plus", "v1.6.0", env
            )
        run.assert_called_once_with(
            ["git", "ls-remote", "--tags", "https://github.com/teramoby/speedle-plus.git", "refs/tags/v*"],
            ROOT,
            env,
            timeout=120,
        )

    def test_rejects_a_stale_pin_and_names_the_replacement(self):
        listing = "a\trefs/tags/v1.6.0\nb\trefs/tags/v1.7.0\n"
        with mock.patch.object(QUICK_START, "run", return_value=listing):
            with self.assertRaisesRegex(
                AssertionError,
                r"stale github\.com/teramoby/speedle-plus@v1\.6\.0.*v1\.7\.0",
            ):
                QUICK_START.check_latest_install_version(
                    "github.com/teramoby/speedle-plus", "v1.6.0", {}
                )

    def test_detects_a_new_major_and_compares_versions_numerically(self):
        for latest in ("v1.10.0", "v2.0.0"):
            listing = f"a\trefs/tags/v1.9.0\nb\trefs/tags/{latest}\n"
            with self.subTest(latest=latest), mock.patch.object(QUICK_START, "run", return_value=listing):
                with self.assertRaisesRegex(AssertionError, re.escape(latest)):
                    QUICK_START.check_latest_install_version(
                        "github.com/teramoby/speedle-plus", "v1.9.0", {}
                    )

    def test_rejects_a_response_without_a_full_release_version(self):
        with mock.patch.object(QUICK_START, "run", return_value="a\trefs/tags/v1.6\n"):
            with self.assertRaisesRegex(AssertionError, "No full semantic version tags"):
                QUICK_START.check_latest_install_version(
                    "github.com/teramoby/speedle-plus", "v1.6.0", {}
                )


if __name__ == "__main__":
    unittest.main()
