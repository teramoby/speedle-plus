"""Public standalone schemas must retain the server's complete binary contract."""
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'website/tests'))
import check_docs


class ProtoContractTests(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)
        for kind in ('ads', 'pms'):
            for relative in (
                f'docs/protobuf/{kind}.proto',
                f'docs/hugo/static/protobuf/{kind}.proto',
                f'pkg/svcs/{kind}grpc/pb/service.proto',
                f'pkg/svcs/{kind}grpc/pb/messages.proto',
            ):
                destination = self.root / relative
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / relative, destination)
        self.override = patch.object(check_docs, 'ROOT', self.root)
        self.override.start()
        self.addCleanup(self.override.stop)

    def change(self, relative, before, after):
        path = self.root / relative
        source = path.read_text()
        self.assertIn(before, source)
        path.write_text(source.replace(before, after, 1))

    def test_legacy_downloads_match_split_schemas(self):
        check_docs.check_proto_contracts()

    def test_field_number_drift_is_rejected(self):
        for kind in ('ads', 'pms'):
            with self.subTest(kind=kind):
                relative = f'pkg/svcs/{kind}grpc/pb/messages.proto'
                self.change(relative, 'string type = 1;', 'string type = 99;')
                with self.assertRaisesRegex(AssertionError, 'wire contract drift'):
                    check_docs.check_proto_contracts()
                self.change(relative, 'string type = 99;', 'string type = 1;')

    def test_rpc_name_drift_is_rejected(self):
        self.change('pkg/svcs/adsgrpc/pb/service.proto', 'rpc IsAllowed(', 'rpc IsAllowedChanged(')
        with self.assertRaisesRegex(AssertionError, 'wire contract drift'):
            check_docs.check_proto_contracts()

    def test_published_copy_drift_is_rejected(self):
        self.change('docs/protobuf/pms.proto', 'string type = 1;', 'string type = 99;')
        with self.assertRaisesRegex(AssertionError, 'published protobuf copy drift'):
            check_docs.check_proto_contracts()


if __name__ == '__main__':
    unittest.main()
