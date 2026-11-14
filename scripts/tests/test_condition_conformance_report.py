"""Coverage evidence must merge profiles and reject unexplained gaps."""
import contextlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'ci' / 'condition-conformance-report.py'
spec = importlib.util.spec_from_file_location('condition_report', SCRIPT)
reporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reporter)


class ConditionCoverageReportTest(unittest.TestCase):
    def run_report(self, extra=''):
        files = ['pkg/eval/condition/condition.go', 'pkg/eval/function/builtin_functions.go']
        files += [reporter.FORK + name for name in (
            'EvaluableExpression.go', 'parsing.go', 'stagePlanner.go',
            'evaluationStage.go', 'lexerState.go', 'lexerStream.go',
            'parameters.go', 'sanitizedParameters.go')]
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / 'coverage.out'
            profile.write_text('mode: count\n' + ''.join(
                f'github.com/teramoby/speedle-plus/{file}:1.1,1.2 1 1\n' for file in files
            ) + extra)
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                failed = reporter.report(profile)
            return failed, output.getvalue()

    def test_duplicate_blocks_are_merged_not_added(self):
        failed, output = self.run_report(
            'github.com/teramoby/speedle-plus/pkg/eval/condition/condition.go:1.1,1.2 1 0\n')
        self.assertFalse(failed)
        self.assertIn('`pkg/eval/condition/condition.go` | 1/1 | 100.0%', output)

    def test_unexplained_gap_fails(self):
        failed, output = self.run_report(
            'github.com/teramoby/speedle-plus/pkg/eval/condition/condition.go:2.1,2.2 2 0\n')
        self.assertTrue(failed)
        self.assertIn('uncovered reachable block', output)

    def test_stale_unreachable_annotation_fails(self):
        failed, output = self.run_report(
            f'github.com/teramoby/speedle-plus/{reporter.FORK}parsing.go:49.4,50.1 1 1\n')
        self.assertTrue(failed)
        self.assertIn('stale unreachable annotation', output)

    def test_missing_files_and_empty_profiles_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / 'coverage.out'
            profile.write_text('mode: count\n')
            with self.assertRaisesRegex(ValueError, 'no condition-path'):
                reporter.report(profile)
            profile.write_text('mode: count\n' +
                'github.com/teramoby/speedle-plus/pkg/eval/condition/condition.go:1.1,1.2 1 1\n')
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertTrue(reporter.report(profile))
