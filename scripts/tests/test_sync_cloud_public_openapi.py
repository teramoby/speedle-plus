"""The vendored Cloud public OpenAPI document excludes console /api/ paths."""

import importlib.util
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "sync-cloud-public-openapi.py"
SPEC = importlib.util.spec_from_file_location("sync_cloud_public_openapi", SCRIPT)
sync = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(sync)

VALID = """\
openapi: 3.0.3
info:
  title: Example
  version: 1.0.0
paths:
  /authz-check/v1/is-allowed:
    post:
      responses:
        "200":
          description: Decision
"""


class RecordingHandler(BaseHTTPRequestHandler):
    body = VALID.encode("utf-8")
    status = 200

    def do_GET(self):
        payload = type(self).body
        self.send_response(type(self).status)
        self.send_header("Content-Type", "application/yaml")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt, *args):
        return


PUBLIC_PATHS = [
    "/authz-check/v1/is-allowed",
    "/authz-check/v1/all-granted-roles",
    "/authz-check/v1/all-granted-permissions",
    "/policy-mgmt/v1/service",
    "/policy-mgmt/v1/service/{serviceName}",
    "/policy-mgmt/v1/service/{serviceName}/policy",
    "/policy-mgmt/v1/service/{serviceName}/policy/{policyId}",
    "/policy-mgmt/v1/service/{serviceName}/role-policy",
    "/policy-mgmt/v1/service/{serviceName}/role-policy/{policyId}",
]


class PublicOpenAPISyncTest(unittest.TestCase):
    def test_vendored_file_has_no_api_paths(self):
        text = sync.DEST.read_text(encoding="utf-8")
        self.assertEqual(sync.path_keys(text), PUBLIC_PATHS)
        self.assertNotIn("0.0.0-placeholder", text)
        self.assertNotIn("jsonSchemaDialect:", text)
        self.assertEqual(sync.main(["--check"]), 0)

    def test_json_schema_dialect_is_rejected_until_it_is_removed(self):
        with self.assertRaisesRegex(sync.SpecError, "jsonSchemaDialect"):
            sync.validate_public_spec(
                "jsonSchemaDialect: https://json-schema.org/draft/2020-12/schema\n" + VALID)

    def test_fetch_omits_json_schema_dialect(self):
        published = "jsonSchemaDialect: https://json-schema.org/draft/2020-12/schema\n" + VALID
        self.assertNotIn("jsonSchemaDialect:", sync.prepare_public_spec(published))
        with self._server() as server, tempfile.TemporaryDirectory() as temporary:
            dest = Path(temporary) / "cloud-public.yaml"
            RecordingHandler.body = published.encode("utf-8")
            url = f"http://127.0.0.1:{server.server_address[1]}/openapi.public.yaml"
            try:
                self.assertEqual(sync.main(["--url", url, "--dest", str(dest)]), 0)
            finally:
                RecordingHandler.body = VALID.encode("utf-8")
            written = dest.read_text(encoding="utf-8")
            self.assertNotIn("jsonSchemaDialect:", written)
            self.assertIn("/authz-check/v1/is-allowed:", written)

    def test_api_path_is_rejected(self):
        text = VALID.replace("/authz-check/v1/is-allowed:", "/api/me:")
        with self.assertRaisesRegex(sync.SpecError, "/api/ paths"):
            sync.validate_public_spec(text)

    def test_api_mention_outside_a_path_key_is_rejected(self):
        text = VALID.replace("Decision", "not a console /api/ route")
        with self.assertRaisesRegex(sync.SpecError, "/api/ paths"):
            sync.validate_public_spec(text)

    def test_document_without_paths_is_rejected(self):
        text = "openapi: 3.0.3\ninfo:\n  title: Example\n  version: 1.0.0\n"
        with self.assertRaisesRegex(sync.SpecError, "no paths"):
            sync.validate_public_spec(text)

    def test_non_openapi_document_is_rejected(self):
        with self.assertRaisesRegex(sync.SpecError, "no openapi or swagger"):
            sync.validate_public_spec("title: notes\n")

    def test_fetch_writes_a_valid_document_and_keeps_the_previous_on_failure(self):
        with self._server() as server, tempfile.TemporaryDirectory() as temporary:
            dest = Path(temporary) / "cloud-public.yaml"
            dest.write_text("openapi: 3.0.3\ninfo:\n  title: keep\n  version: 0\npaths:\n  /health:\n    get:\n      responses:\n        \"200\":\n          description: ok\n", encoding="utf-8")
            url = f"http://127.0.0.1:{server.server_address[1]}/openapi.public.yaml"
            self.assertEqual(sync.main(["--url", url, "--dest", str(dest)]), 0)
            self.assertIn("/authz-check/v1/is-allowed:", dest.read_text(encoding="utf-8"))
            before = dest.read_text(encoding="utf-8")
            RecordingHandler.status = 404
            RecordingHandler.body = b"missing"
            try:
                self.assertEqual(sync.main(["--url", url, "--dest", str(dest)]), 1)
            finally:
                RecordingHandler.status = 200
                RecordingHandler.body = VALID.encode("utf-8")
            self.assertEqual(dest.read_text(encoding="utf-8"), before)

    def test_invalid_document_is_not_written(self):
        bad = VALID.replace("/authz-check/v1/is-allowed:", "/api/tokens:")
        with self._server() as server, tempfile.TemporaryDirectory() as temporary:
            dest = Path(temporary) / "cloud-public.yaml"
            dest.write_text(VALID, encoding="utf-8")
            before = dest.read_text(encoding="utf-8")
            RecordingHandler.body = bad.encode("utf-8")
            url = f"http://127.0.0.1:{server.server_address[1]}/openapi.public.yaml"
            try:
                self.assertEqual(sync.main(["--url", url, "--dest", str(dest)]), 1)
            finally:
                RecordingHandler.body = VALID.encode("utf-8")
            self.assertEqual(dest.read_text(encoding="utf-8"), before)

    def _server(self):
        return _Server()


class _Server:
    def __enter__(self):
        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), RecordingHandler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()
        return self.httpd

    def __exit__(self, exc_type, exc, traceback):
        self.httpd.shutdown()
        self.httpd.server_close()
        self.thread.join(timeout=5)
        return False


if __name__ == "__main__":
    unittest.main()
