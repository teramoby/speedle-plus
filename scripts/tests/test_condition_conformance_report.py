"""Coverage evidence must merge profiles and reject unexplained gaps."""
import contextlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).resolve().parents[1] / 'ci' / 'condition-conformance-report.py'
spec = importlib.util.spec_from_file_location('condition_report', SCRIPT)
reporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reporter)


class ConditionCoverageReportTest(unittest.TestCase):
    def run_report(self, extra='', omit=None, go126=False, matrix=None):
        files = ['pkg/eval/condition/condition.go', 'pkg/eval/function/builtin_functions.go']
        files += [reporter.FORK + name for name in (
            'EvaluableExpression.go', 'parsing.go', 'stagePlanner.go',
            'evaluationStage.go', 'lexerState.go', 'lexerStream.go',
            'parameters.go', 'sanitizedParameters.go')]
        annotations = ''
        for file, blocks in reporter.UNREACHABLE.items():
            for span, n in blocks:
                if (file, span, n) == omit:
                    continue
                if go126:
                    span = next(actual for (source, actual, count), canonical in reporter.GO126_SPANS.items()
                                if source == file and count == n and canonical == (span, n))
                annotations += f'github.com/teramoby/speedle-plus/{reporter.FORK}{file}:{span} {n} 0\n'
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / 'coverage.out'
            profile.write_text('mode: count\n' + ''.join(
                f'github.com/teramoby/speedle-plus/{file}:1.1,1.2 1 1\n' for file in files
            ) + annotations + extra)
            output = io.StringIO()
            matrix_file=None
            if matrix is not None:
                matrix_file=Path(directory) / 'matrix.log'
                matrix_file.write_text(matrix)
            with contextlib.redirect_stdout(output):
                failed = reporter.report(profile,matrix_file)
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

    def test_unmatched_annotation_fails(self):
        failed, output = self.run_report(omit=('parsing.go', '49.4,50.1', 1))
        self.assertTrue(failed)
        self.assertIn('unmatched unreachable annotation', output)

    def test_moved_block_is_not_excluded_by_start_line(self):
        failed, output = self.run_report(
            f'github.com/teramoby/speedle-plus/{reporter.FORK}parsing.go:49.4,51.1 1 0\n',
            omit=('parsing.go', '49.4,50.1', 1))
        self.assertTrue(failed)
        self.assertIn('uncovered reachable block', output)
        self.assertIn('unmatched unreachable annotation', output)

    def test_changed_audited_source_fails(self):
        with mock.patch.dict(reporter.AUDITED_SOURCE_SHA256, {'parsing.go': 'not-the-source-hash'}):
            failed, output = self.run_report()
        self.assertTrue(failed)
        self.assertIn('audited source changed; re-audit required', output)

    def test_go126_layout_matches_same_annotations(self):
        failed, output = self.run_report(go126=True)
        self.assertFalse(failed, output)

    def test_mixed_layouts_are_rejected(self):
        failed, output = self.run_report(
            f'github.com/teramoby/speedle-plus/{reporter.FORK}parsing.go:48.17,50.4 1 0\n')
        self.assertTrue(failed)
        self.assertIn('mixed coverage layouts', output)

    def test_known_differences_are_in_generated_report(self):
        failed, output = self.run_report(matrix=(
            '    condition_conformance_test.go:1: Known differences (name / expression / expected authorization):\n'
            '    condition_conformance_test.go:2: Difference: string/mismatch-!=/wrong-type: x != y => true (known difference, behaviour pinned)\n'
            '    condition_conformance_test.go:3: Section 2.1: 196 cases\n'))
        self.assertFalse(failed, output)
        self.assertIn('Known differences (name / expression / expected authorization):', output)
        self.assertIn('Difference: string/mismatch-!=/wrong-type', output)
