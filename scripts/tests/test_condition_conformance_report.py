"""Coverage evidence must merge profiles and reject unexplained gaps."""
import contextlib
import importlib.util
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).resolve().parents[1] / 'ci' / 'condition-conformance-report.py'
spec = importlib.util.spec_from_file_location('condition_report', SCRIPT)
reporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reporter)

VALID_MATRIX = (
    '    condition_conformance_test.go:1: | Reference / row | other |\n'
    '    condition_conformance_test.go:2: |---|---|\n'
    '    condition_conformance_test.go:3: | 2.1 / string | 1/1/0 |\n'
    '    condition_conformance_test.go:4: Outcome totals: 1 match / 1 no-match / 0 error\n')


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
            matrix_file = Path(directory) / 'matrix.log'
            matrix_file.write_text(VALID_MATRIX if matrix is None else matrix)
            with contextlib.redirect_stdout(output):
                failed = reporter.report(profile,matrix_file, go_version='go1.26.11' if go126 else 'go1.27.4')
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
                reporter.report(profile, None, go_version='go1.27.4')
            profile.write_text('mode: count\n' +
                'github.com/teramoby/speedle-plus/pkg/eval/condition/condition.go:1.1,1.2 1 1\n')
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertTrue(reporter.report(profile, None, go_version='go1.27.4'))

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

    def test_changed_excluded_sql_sources_fail(self):
        for file in ('EvaluableExpression_sql.go', 'expressionOutputStream.go'):
            with self.subTest(file=file), mock.patch.dict(reporter.AUDITED_SOURCE_SHA256, {file: 'changed'}):
                failed, output = self.run_report()
                self.assertTrue(failed)
                self.assertIn(f'audited source changed; re-audit required: {reporter.FORK}{file}', output)

    def test_unaudited_toolchain_fails_once_before_reading_profile(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            failed = reporter.report('profile-must-not-be-read', None, go_version='go1.28.0')
        self.assertTrue(failed)
        self.assertEqual(output.getvalue(), 'ERROR: coverage layout for go1.28.0 is not audited '
                         '(expected Go 1.26/1.27); re-audit required\n')

    def test_go126_layout_matches_same_annotations(self):
        failed, output = self.run_report(go126=True)
        self.assertFalse(failed, output)

    def test_mixed_layouts_are_rejected(self):
        failed, output = self.run_report(
            f'github.com/teramoby/speedle-plus/{reporter.FORK}parsing.go:48.17,50.4 1 0\n')
        self.assertTrue(failed)
        self.assertIn('mixed coverage layouts', output)

    def test_known_differences_are_in_generated_report(self):
        failed, output = self.run_report(matrix=VALID_MATRIX + (
            '    condition_conformance_test.go:1: Known differences (name / expression / expected authorization):\n'
            '    condition_conformance_test.go:2: Difference: string/mismatch-!=/wrong-type: x != y => true (known difference, behaviour pinned)\n'
            '    condition_conformance_test.go:3: Section 2.1: 196 cases\n'))
        self.assertFalse(failed, output)
        self.assertIn('Known differences (name / expression / expected authorization):', output)
        self.assertIn('Difference: string/mismatch-!=/wrong-type', output)

    def test_missing_or_incomplete_matrix_fails(self):
        for matrix, reason in (
            ('', 'no case rows'),
            ('| Reference / row | other |\n|---|---|\nOutcome totals: 1 match / 1 no-match / 0 error\n', 'no case rows'),
            ('| 2.1 / string | 1/1/0 |\n', 'missing Outcome totals'),
        ):
            with self.subTest(matrix=matrix):
                failed, output = self.run_report(matrix=matrix)
                self.assertTrue(failed, output)
                self.assertIn(reason, output)

    def test_matrix_counts_must_match_nonempty_totals(self):
        for cell, totals, reason in (
            ('1/1/0', '2 match / 1 no-match / 0 error', 'do not match Outcome totals'),
            ('0/0/0', '0 match / 0 no-match / 0 error', 'contains no cases'),
        ):
            with self.subTest(cell=cell):
                failed, output = self.run_report(matrix=(
                    f'| 2.1 / string | {cell} |\n'
                    f'    condition_conformance_test.go:1: Outcome totals: {totals}\n'))
                self.assertTrue(failed, output)
                self.assertIn(reason, output)

    def test_matrix_is_required_by_cli(self):
        result = subprocess.run([sys.executable, str(SCRIPT), '--go-version', 'go1.27.4',
                                 'profile-must-not-be-read'], text=True, capture_output=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn('matrix', result.stderr)

    def test_matrix_preserves_error_outcomes_and_reasons(self):
        failed, output = self.run_report(matrix=(
            '    condition_conformance_test.go:1: Matrix cells are match / no-match / error counts.\n'
            '    condition_conformance_test.go:2: | 2.4 / IsSubSet | 1/1/2 |\n'
            '    condition_conformance_test.go:3: Outcome totals: 1 match / 1 no-match / 2 error\n'
            '    condition_conformance_test.go:4: Error cases (runtime errors are currently discarded by both public authorization paths):\n'
            '    condition_conformance_test.go:5: Error: 2.4/IsSubSet/call/arity-zero: IsSubSet() => SPDL-2003 Usage: IsSubSet(S1, S2) - homogeneous scalar sets required; API allowed=false, runtime error discarded\n'))
        self.assertFalse(failed, output)
        for expected in ('Matrix cells are match / no-match / error', '| 2.4 / IsSubSet | 1/1/2 |',
                         'Outcome totals: 1 match / 1 no-match / 2 error', 'Error cases (runtime errors',
                         'Error: 2.4/IsSubSet/call/arity-zero:', 'SPDL-2003 Usage: IsSubSet(S1, S2)',
                         'API allowed=false, runtime error discarded'):
            self.assertIn(expected, output)
