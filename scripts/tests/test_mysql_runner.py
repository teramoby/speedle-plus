"""Check the MySQL launcher's summary without Docker or MySQL."""
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

CI = Path(__file__).parents[1] / 'ci'
sys.path.insert(0, str(CI))
SPEC = importlib.util.spec_from_file_location('run_mysql_tests', CI / 'run-mysql-tests.py')
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


class CountTestEventsTest(unittest.TestCase):
    def test_counts_go_test_events_and_skips_the_engine_summary(self):
        with tempfile.TemporaryDirectory() as directory:
            results = Path(directory)
            events = [{'Action': 'pass', 'Test': 'TestA'}, {'Action': 'fail', 'Test': 'TestB'},
                      {'Action': 'skip', 'Test': 'TestC'}, {'Action': 'pass', 'Package': 'p'}]
            (results / 'mysql-go.json').write_text('\n'.join(json.dumps(e) for e in events) + '\nnot json\n7\n')
            # Written by store_regression.run_regression; some of its lines parse as numbers.
            (results / 'mysql-summary.json').write_text(json.dumps(
                {'counts': {'pass': 1, 'fail': 0, 'skip': 0}, 'suites': [['mysql-go', 0]]}, indent=2))
            self.assertEqual(RUNNER.count_test_events(results), {'pass': 1, 'fail': 1, 'skip': 1})


if __name__ == '__main__':
    unittest.main()
