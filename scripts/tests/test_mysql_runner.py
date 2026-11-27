"""Check the MySQL launcher's summary and image pull retry without Docker or MySQL."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

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


class EnsureImageTest(unittest.TestCase):
    def test_skips_pull_when_image_is_already_cached(self):
        log = io.StringIO()
        with mock.patch.object(RUNNER.subprocess, 'run',
                               return_value=subprocess.CompletedProcess(['docker'], 0)) as inspect_call, \
                mock.patch.object(RUNNER, 'run') as pull_call:
            RUNNER.ensure_image(RUNNER.IMAGE, log)
        inspect_call.assert_called_once_with(
            ['docker', 'image', 'inspect', RUNNER.IMAGE], cwd=RUNNER.ROOT,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        pull_call.assert_not_called()

    def test_retries_transient_pull_failures_with_bounded_backoff(self):
        log = io.StringIO()
        stdout = io.StringIO()
        failure = subprocess.CalledProcessError(1, ['docker', 'pull', RUNNER.IMAGE])
        with mock.patch.object(RUNNER.subprocess, 'run',
                               return_value=subprocess.CompletedProcess(['docker'], 1)), \
                mock.patch.object(RUNNER, 'run', side_effect=[failure, failure, None]) as pull_call, \
                mock.patch.object(RUNNER.time, 'sleep') as sleep, \
                contextlib.redirect_stdout(stdout):
            RUNNER.ensure_image(RUNNER.IMAGE, log)
        self.assertEqual(pull_call.call_count, 3)
        self.assertEqual(sleep.call_args_list, [mock.call(5), mock.call(15)])
        self.assertIn('Docker pull failed (attempt 1/5); retrying in 5s: exit 1', log.getvalue())
        self.assertIn('Docker pull failed (attempt 2/5); retrying in 15s: exit 1', stdout.getvalue())

    def test_reraises_after_exhausting_pull_retries_and_does_not_retry_timeouts(self):
        log = io.StringIO()
        failures = [subprocess.CalledProcessError(125, ['docker', 'pull', RUNNER.IMAGE]) for _ in range(5)]
        with mock.patch.object(RUNNER.subprocess, 'run',
                               return_value=subprocess.CompletedProcess(['docker'], 1)), \
                mock.patch.object(RUNNER, 'run', side_effect=failures) as pull_call, \
                mock.patch.object(RUNNER.time, 'sleep') as sleep, \
                contextlib.redirect_stdout(io.StringIO()), \
                self.assertRaises(subprocess.CalledProcessError) as raised:
            RUNNER.ensure_image(RUNNER.IMAGE, log)
        self.assertIs(raised.exception, failures[-1])
        self.assertEqual(pull_call.call_count, 5)
        self.assertEqual(sleep.call_args_list, [mock.call(5), mock.call(15), mock.call(30), mock.call(60)])

        timeout = subprocess.TimeoutExpired(cmd=['docker', 'pull', RUNNER.IMAGE], timeout=300)
        with mock.patch.object(RUNNER.subprocess, 'run',
                               return_value=subprocess.CompletedProcess(['docker'], 1)), \
                mock.patch.object(RUNNER, 'run', side_effect=timeout) as timeout_call, \
                mock.patch.object(RUNNER.time, 'sleep') as sleep, \
                self.assertRaises(subprocess.TimeoutExpired):
            RUNNER.ensure_image(RUNNER.IMAGE, io.StringIO())
        timeout_call.assert_called_once()
        sleep.assert_not_called()


if __name__ == '__main__':
    unittest.main()
