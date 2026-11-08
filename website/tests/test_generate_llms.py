"""Exercise the publication guards in the llms.txt/llms-full.txt generator."""
import runpy
import unittest
from pathlib import Path

GENERATOR = runpy.run_path(str(Path(__file__).resolve().parents[1] / "generate-llms.py"))


def page(key, draft=False):
    return {
        "key": key,
        "language": "en",
        "route": "/" + key + "/",
        "title": key,
        "linktitle": "",
        "description": "",
        "bref": "",
        "draft": draft,
        "aliases": [],
        "body": f"Body of {key}.",
        "source": Path(key + ".md"),
    }


class GeneratorGuards(unittest.TestCase):
    def full_pages(self):
        return {"en": {key: page(key) for key in GENERATOR["FULL_PAGES"]}, "zh": {}}

    def test_draft_page_in_full_pages_is_rejected(self):
        pages = self.full_pages()
        pages["en"]["docs/spdl"]["draft"] = True
        with self.assertRaisesRegex(GENERATOR["ContentError"], "draft page"):
            GENERATOR["render_full"](pages, "definition")

    def test_published_full_pages_are_rendered(self):
        text = GENERATOR["render_full"](self.full_pages(), "definition")
        self.assertIn("# Speedle — full documentation", text)
        self.assertIn("Body of docs/spdl.", text)


if __name__ == "__main__":
    unittest.main()
