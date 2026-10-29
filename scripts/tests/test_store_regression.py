"""Verify complete regression stages preserve failures without real services."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import MagicMock, patch

SPEC = importlib.util.spec_from_file_location('store_regression', Path(__file__).parents[1] / 'ci' / 'store_regression.py')
REGRESSION = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REGRESSION)


class Process:
    def __init__(self):
        self.returncode = None

    def poll(self):
        return self.returncode

    def send_signal(self, signal):
        self.returncode = 0

    def wait(self, timeout=None):
        return self.returncode


class StoreRegressionTests(unittest.TestCase):
    def exercise(self, failing):
        commands, processes = [], []

        def run(args, **kwargs):
            commands.append(args)
            stage = None
            if args[:2] == ['go', 'test']:
                stage = 'full'
                if '-tags' in args:
                    stage = 'prepare' if args[args.index('-tags') + 1] == 'runtime_test_prepare' else 'runtime'
            code = 7 if stage == failing else 0
            if stage:
                kwargs['stdout'].write(json.dumps({'Action': 'fail' if code else 'pass', 'Package': 'fixture/package', 'Test': stage}) + '\n')
            return SimpleNamespace(returncode=code)

        def start(*args, **kwargs):
            process = Process()
            processes.append(process)
            return process

        with tempfile.TemporaryDirectory() as directory:
            with patch.object(REGRESSION.subprocess, 'run', side_effect=run), \
                 patch.object(REGRESSION.subprocess, 'Popen', side_effect=start), \
                 patch.object(REGRESSION.socket, 'socket', return_value=MagicMock()), \
                 patch.object(REGRESSION.socket, 'create_connection', return_value=MagicMock()), \
                 patch.object(REGRESSION, 'free_port', side_effect=[31001, 31002]), \
                 patch.object(REGRESSION.CLIENT, 'open', return_value=MagicMock()), \
                 patch.object(REGRESSION.time, 'sleep'), contextlib.redirect_stdout(io.StringIO()):
                failed, suites = REGRESSION.run_regression({}, {'storeConfig': {'storeType': 'mongodb', 'storeProps': {}}}, directory)
            summary = json.loads((Path(directory) / 'mongodb-summary.json').read_text())
            blocked = (Path(directory) / 'mongodb-ads-runtime.stderr').exists()
        self.assertTrue(all(process.returncode == 0 for process in processes), 'owned services were not stopped')
        return failed, suites, summary, commands, blocked

    def test_full_suite_failure_does_not_disappear_when_runtime_passes(self):
        failed, suites, summary, commands, _ = self.exercise('full')
        self.assertTrue(failed)
        self.assertEqual(suites, [('mongodb-go', 7), ('mongodb-ads-prepare', 0), ('mongodb-ads-runtime', 0)])
        self.assertEqual(summary['counts'], {'pass': 2, 'fail': 1, 'skip': 0})
        ordinary = next(command for command in commands if command[:2] == ['go', 'test'])
        self.assertEqual(ordinary[-1], './...')
        self.assertIn('-race', ordinary)

    def test_failed_preparation_blocks_runtime_without_claiming_success(self):
        failed, suites, summary, _, blocked = self.exercise('prepare')
        self.assertTrue(failed)
        self.assertEqual(suites, [('mongodb-go', 0), ('mongodb-ads-prepare', 7)])
        self.assertTrue(blocked)
        self.assertTrue(summary['failed_tests'])

    def test_previous_summary_in_results_directory_is_not_counted(self):
        with tempfile.TemporaryDirectory() as directory:
            previous = {'backend': 'mongodb', 'counts': {'pass': 9, 'fail': 9, 'skip': 9},
                        'suites': [['mongodb-go', 0]], 'failed': False}
            (Path(directory) / 'mongodb-summary.json').write_text(json.dumps(previous, indent=2))
            with patch.object(REGRESSION.tempfile, 'TemporaryDirectory', side_effect=RuntimeError('stop')), \
                 contextlib.redirect_stdout(io.StringIO()):
                failed, _ = REGRESSION.run_regression({}, {'storeConfig': {'storeType': 'mongodb', 'storeProps': {}}}, directory)
            summary = json.loads((Path(directory) / 'mongodb-summary.json').read_text())
        self.assertTrue(failed)
        self.assertEqual(summary['counts'], {'pass': 0, 'fail': 0, 'skip': 0})

    def test_etcd_launcher_rejects_the_ports_the_regression_needs(self):
        spec = importlib.util.spec_from_file_location('run_store_tests', Path(__file__).parents[1] / 'ci' / 'run-store-tests.py')
        launcher = importlib.util.module_from_spec(spec)
        with patch.dict('sys.modules', {'store_regression': REGRESSION}):
            spec.loader.exec_module(launcher)
        for endpoint in (None, 'localhost:2379', '127.0.0.1:2380'):
            environment = {'STORE_TYPE': 'etcd'}
            if endpoint:
                environment['SPEEDLE_ETCD_ENDPOINT'] = endpoint
            with patch.dict(launcher.os.environ, environment, clear=True), \
                 patch.object(launcher, 'run_regression') as run:
                with self.assertRaises(ValueError):
                    launcher.main()
                run.assert_not_called()
        with patch.dict(launcher.os.environ, {'STORE_TYPE': 'etcd', 'SPEEDLE_ETCD_ENDPOINT': '127.0.0.1:23790'}, clear=True), \
             patch.object(launcher, 'run_regression', return_value=(False, [])) as run:
            self.assertEqual(launcher.main(), 0)
        properties = run.call_args[0][1]['storeConfig']['storeProps']
        self.assertEqual(properties['EtcdEndpoint'], '127.0.0.1:23790')
        self.assertRegex(properties['EtcdKeyPrefix'], r'^/speedle_test_runtime_[0-9a-f]{16}/$')

    def test_unknown_backend_cannot_fall_back_to_file(self):
        with self.assertRaises(ValueError):
            REGRESSION.run_regression({}, {'storeConfig': {'storeType': 'unknown'}}, Path('.'))


if __name__ == '__main__':
    unittest.main()
