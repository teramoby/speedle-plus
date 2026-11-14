"""Owned-port remapping must not cascade through another documented port."""
from pathlib import Path
import sys
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'website/tests'))
import check_docs


class DocsPortTests(unittest.TestCase):
    def test_replacement_is_not_remapped_again(self):
        ports = {6733: 50002, 50002: 50006}
        command = 'pms --listen 127.0.0.1:6733 --grpc 127.0.0.1:50002'
        self.assertEqual(check_docs.remap_ports(command, ports),
                         'pms --listen 127.0.0.1:50002 --grpc 127.0.0.1:50006')

    def test_only_complete_known_ports_change(self):
        command = 'http://127.0.0.1:6733/path 127.0.0.1:67330 127.0.0.1:0'
        self.assertEqual(check_docs.remap_ports(command, {6733: 49000}),
                         'http://127.0.0.1:49000/path 127.0.0.1:67330 127.0.0.1:0')

    def test_exact_documented_ports_are_unchanged(self):
        command = '127.0.0.1:6733 127.0.0.1:50002'
        self.assertEqual(check_docs.remap_ports(command, {6733: 6733, 50002: 50002}), command)


if __name__ == '__main__':
    unittest.main()
