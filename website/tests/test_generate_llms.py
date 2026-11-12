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
        return {
            lang: {key: page(key) for key in GENERATOR["FULL_PAGES"]} if lang == "en" else {}
            for lang in ("en", "zh", "ja", "ko", "es")
        }

    def test_draft_page_in_full_pages_is_rejected(self):
        pages = self.full_pages()
        pages["en"]["docs/spdl"]["draft"] = True
        with self.assertRaisesRegex(GENERATOR["ContentError"], "draft page"):
            GENERATOR["render_full"](pages, "definition")

    def test_published_full_pages_are_rendered(self):
        text = GENERATOR["render_full"](self.full_pages(), "definition")
        self.assertIn("# Speedle — full documentation", text)
        self.assertIn("Body of docs/spdl.", text)

    def test_swagger_shortcode_with_and_without_tryitout_is_replaced(self):
        disabled = '{{< swagger-spec url="/openapi/cloud-public.yaml" tryitout="false" >}}'
        enabled = '{{< swagger-spec url="/swagger/policy-check.yaml" >}}'
        replaced = GENERATOR["replace_shortcodes"](disabled + "\n" + enabled)
        self.assertNotIn("swagger-spec", replaced)
        self.assertIn("https://engine.speedle.io/openapi/cloud-public.yaml", replaced)
        self.assertIn("https://engine.speedle.io/swagger/policy-check.yaml", replaced)


if __name__ == "__main__":
    unittest.main()
