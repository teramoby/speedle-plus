"""The vendored Swagger UI bundle is the recorded swagger-ui-dist release."""

import hashlib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STATIC = ROOT / "docs" / "hugo" / "static"
RECORDED = {
    "js/swagger-js/swagger-ui-bundle.js":
        "36eeb4d5640b9c036b1d0f854499af1cceebbdb260b790a03a18be5e8488fbf5",
    "js/swagger-js/swagger-ui-bundle.js.LICENSE.txt":
        "37eb6e7e59aed57b2c1acf076574b818ea07b74265717e0bd67b59e0baf9abbb",
    "css/swagger-ui.css":
        "c3cfe8248c4b0b89673600fb829924eafbac6de4bc4b2b06f5f5243483074d0a",
}


class SwaggerUIVendorTest(unittest.TestCase):
    def test_upstream_record_matches_swagger_ui_dist_5_34_1(self):
        record = (STATIC / "js" / "swagger-js" / "UPSTREAM.txt").read_text(encoding="utf-8")
        self.assertIn("swagger-ui-dist 5.34.1", record)
        for relative, digest in RECORDED.items():
            payload = (STATIC / relative).read_bytes()
            self.assertEqual(hashlib.sha256(payload).hexdigest(), digest, relative)
            self.assertIn(digest, record)
        bundle = (STATIC / "js" / "swagger-js" / "swagger-ui-bundle.js").read_text(
            encoding="utf-8", errors="replace")
        self.assertIn("5.34.1", bundle)
        self.assertIn("supportedSubmitMethods", bundle)
        stylesheet = (STATIC / "css" / "swagger-ui.css").read_text(encoding="utf-8", errors="replace")
        self.assertNotIn("url(http", stylesheet)
        self.assertIn(".opblock-tag", stylesheet)


if __name__ == "__main__":
    unittest.main()
