#!/usr/bin/env python3
"""Report the SPDL condition path, keeping raw and scoped coverage separate.

Run from the repository root (profile uses covermode=count or atomic as well):
  go test -count=1 -coverpkg=./pkg/eval/condition,./pkg/eval/function,./pkg/condexpr -coverprofile=condition.out ./pkg/eval/... ./pkg/svcs/adsrest/... ./pkg/condexpr
  go test -count=1 -v ./pkg/eval -run '^TestConditionConformanceMatrix$' > condition-matrix.log
  python3 scripts/ci/condition-conformance-report.py --go-version "$(go env GOVERSION)" condition.out condition-matrix.log

Does not alter the existing repository coverage gate. Every uncovered block in
scope must have an explicit audited reason. Duplicate blocks from -coverpkg are
merged by maximum count (never counted as separate statements).
"""
import argparse
import hashlib
from collections import defaultdict
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
FORK = 'pkg/condexpr/'
# These files implement SQL output only; neither the condition validator nor
# the evaluator invokes ToSQLQuery or its stream. Package unit tests may execute
# some of their blocks (for example the empty-token guard); covered blocks are
# counted normally, and uncovered blocks are excluded for this reason.
OFF_PATH_FILES = {
    FORK + 'EvaluableExpression_sql.go': 'SQL serialization is not called by the Engine condition path.',
    FORK + 'expressionOutputStream.go': 'Output stream is used only by SQL serialization, not condition evaluation.',
}
# Exact coverage spans and statement counts, not broad function exclusions. A new uncovered block
# fails the report, requiring either a test or a fresh reachability audit.
UNREACHABLE = {'EvaluableExpression.go': {('87.3,88.1', 1): 'Validated token kinds always produce a stage; the '
                                              'token-constructor planner error cannot follow successful syntax '
                                              'validation.',
                            ('161.3,162.1', 1): 'parseTokens already checked balance and returned its error; the '
                                                'repeated balance check cannot fail.',
                            ('176.3,177.1', 1): 'The text parser and syntax validator reject unplannable tokens '
                                                'before stage planning.'},
 'lexerState.go': {('346.4,347.1', 1): 'canTransitionTo accepted this kind, which necessarily has an entry in '
                                       'validLexerStates.'},
 'parsing.go': {('49.4,50.1', 1): 'readToken emits only kinds in validLexerStates; unknown token errors return '
                                  'before this lookup.',
                ('88.4,88.12', 1): 'parseTokens consumes whitespace before calling readToken; this redundant '
                                   'whitespace branch is not reached by the text parser.',
                ('344.5,344.13', 1): 'All non-whitespace readers start on a token character; subsequent '
                                     'whitespace breaks a nonempty token at line 341. No production call supplies '
                                     'a leading whitespace reader.',
                ('443.2,444.1', 1): 'isDigit has no callers in the fork or Engine (numeric and hex readers use '
                                    'unicode.IsDigit directly).'},
 'stagePlanner.go': {('257.3,258.1', 1): 'planTokens can error only through unplannable token kinds, already '
                                         'rejected by syntax validation.',
                     ('271.3,272.1', 1): 'Nonempty validated expressions reach planTokens; token streams consumed '
                                         'within clauses are handled by planValue CLAUSE_CLOSE.',
                     ('301.4,302.1', 1): 'Left precedence planner error propagation; all validated kinds have '
                                         'value planners.',
                     ('327.5,327.10', 1): 'The valid-symbol tables receive only string operator tokens from the '
                                          'lexer and syntax validator.',
                     ('339.5,340.1', 1): 'Right precedence planner error propagation; all validated kinds have '
                                         'value planners.',
                     ('381.3,382.1', 1): 'Function argument planner error propagation after successful syntax '
                                         'validation.',
                     ('400.3,401.1', 1): 'A validated FUNCTION always has a following clause, so planAccessor '
                                         'cannot receive an exhausted stream.',
                     ('422.5,423.1', 1): 'Accessor argument planner error propagation after successful syntax '
                                         'validation.',
                     ('451.3,452.1', 1): 'A validated expression or prefix always has a following value or '
                                         'clause-close token.',
                     ('462.4,463.1', 1): 'Clause planner error propagation; invalid contents have already failed '
                                         'syntax validation.',
                     ('508.3,510.1', 2): 'All value token kinds accepted by syntax validation have an operator; '
                                         'this rejects malformed internal token streams only.',
                     ('666.3,667.1', 1): 'A planned operator chain ends at an operand with distinct precedence; '
                                         'matching chains are mirrored at the precedence break, not after the '
                                         'final leaf.',
                     ('763.3,764.1', 1): 'LITERAL stages use makeLiteralStage, whose closure always returns a '
                                         'value with nil error.',
                     ('768.3,769.1', 1): 'LITERAL stages use makeLiteralStage, whose closure always returns a '
                                         'value with nil error.',
                     ('789.3,790.1', 1): 'This is the root.operator error return after the elideStage type '
                                         'checks at lines 711-723. Literal type errors such as true+false '
                                         'exit at those earlier checks and are later reported by Evaluate; '
                                         'they never reach this return. Pure well-typed operators return nil '
                                         'errors; literal regex errors were rejected by optimizeTokens. '
                                         'Functions/accessors are not LITERAL.'}}

# Bind the audit to source contents as well as coverage spans. A source edit
# requires an explicit reachability re-audit even if its line numbers survive.
AUDITED_SOURCE_SHA256 = {'EvaluableExpression.go': '74536aabf0bd17a0ae73eb1143d9fc6532a4ee7182b8a101546e67a8b89584dc',
 'EvaluableExpression_sql.go': '5e1066d5c85f91105b7dbd457571da1835966f09389f58ebe183bb81a90e8188',
 'expressionOutputStream.go': '300d9803ed5b07eee5c1af16fa81d3cb7c39b38e4d784a4fa6ef376b079bc8c5',
 'lexerState.go': '664c31b50bddfc18bdfbf188dd1b064cdce783e6a467335af3182a55670bac95',
 'parsing.go': 'e7444af3bf0687f426573534f2d8d07c2f995d3db750669db6656578c9eaf988',
 'stagePlanner.go': 'f2a3447f41e154d627b0762c86501af7d72017df31cbda8ba425c52d97c8b442'}


# Go 1.26 records the enclosing control-flow boundary; Go 1.27 starts at
# the first statement. Both layouts below were verified against the identical
# source hashes above. Unexplained blocks still fail closed, independently of
# the reported toolchain version. Values identify the
# canonical annotation, so every annotation must match in either layout.
GO126_SPANS = {('EvaluableExpression.go', '86.16,88.3', 1): ('87.3,88.1', 1),
 ('EvaluableExpression.go', '160.16,162.3', 1): ('161.3,162.1', 1),
 ('EvaluableExpression.go', '175.16,177.3', 1): ('176.3,177.1', 1),
 ('lexerState.go', '345.17,347.4', 1): ('346.4,347.1', 1),
 ('parsing.go', '48.17,50.4', 1): ('49.4,50.1', 1),
 ('parsing.go', '87.33,88.12', 1): ('88.4,88.12', 1),
 ('parsing.go', '343.26,344.13', 1): ('344.5,344.13', 1),
 ('parsing.go', '442.35,444.2', 1): ('443.2,444.1', 1),
 ('stagePlanner.go', '256.16,258.3', 1): ('257.3,258.1', 1),
 ('stagePlanner.go', '270.23,272.3', 1): ('271.3,272.1', 1),
 ('stagePlanner.go', '300.17,302.4', 1): ('301.4,302.1', 1),
 ('stagePlanner.go', '326.30,327.10', 1): ('327.5,327.10', 1),
 ('stagePlanner.go', '338.18,340.5', 1): ('339.5,340.1', 1),
 ('stagePlanner.go', '380.16,382.3', 1): ('381.3,382.1', 1),
 ('stagePlanner.go', '399.23,401.3', 1): ('400.3,401.1', 1),
 ('stagePlanner.go', '421.18,423.5', 1): ('422.5,423.1', 1),
 ('stagePlanner.go', '450.23,452.3', 1): ('451.3,452.1', 1),
 ('stagePlanner.go', '461.17,463.4', 1): ('462.4,463.1', 1),
 ('stagePlanner.go', '507.21,510.3', 2): ('508.3,510.1', 2),
 ('stagePlanner.go', '665.35,667.3', 1): ('666.3,667.1', 1),
 ('stagePlanner.go', '762.16,764.3', 1): ('763.3,764.1', 1),
 ('stagePlanner.go', '767.16,769.3', 1): ('768.3,769.1', 1),
 ('stagePlanner.go', '788.16,790.3', 1): ('789.3,790.1', 1)}

def report(profile, matrix, *, go_version):
    # GOVERSION is provenance, not an allow-list for the merge gate. Match the
    # actual profile spans below; another compiler may emit the same layout.
    print(f'Go toolchain (go env GOVERSION, informational): {go_version}')
    blocks = {}
    for line in Path(profile).read_text().splitlines()[1:]:
        location, statements, count = line.split()
        file, span = location.rsplit(':', 1)
        file = file.split('github.com/teramoby/speedle-plus/', 1)[-1]
        if not (file.startswith(FORK) or file.startswith(('pkg/eval/condition/', 'pkg/eval/function/'))):
            continue
        key = (file, span)
        n, previous = blocks.get(key, (int(statements), 0))
        if n != int(statements):
            raise ValueError(f'inconsistent statement count: {location}')
        blocks[key] = (n, max(previous, int(count)))
    if not blocks:
        raise ValueError('no condition-path coverage blocks found')
    totals = defaultdict(lambda: [0, 0, 0])
    exclusions = []
    failures = []
    consumed = {}
    for file, expected_hash in AUDITED_SOURCE_SHA256.items():
        try:
            data = (ROOT / FORK / file).read_bytes()
        except OSError as exc:
            failures.append(f'audited source unavailable: {FORK}{file}: {exc}')
            continue
        actual_hash = hashlib.sha256(data).hexdigest()
        if actual_hash != expected_hash:
            failures.append(f'audited source changed; re-audit required: {FORK}{file}')
    for (file, span), (n, count) in sorted(blocks.items()):
        totals[file][0] += n
        if count:
            totals[file][1] += n
        reason = OFF_PATH_FILES.get(file)
        off_path = reason is not None
        if reason is None and file.startswith(FORK):
            short_file = file[len(FORK):]
            canonical = GO126_SPANS.get((short_file, span, n), (span, n))
            reason = UNREACHABLE.get(short_file, {}).get(canonical)
            if reason:
                annotation = (short_file, *canonical)
                if annotation in consumed and consumed[annotation] != span:
                    failures.append(f'mixed coverage layouts for one annotation: {file}:{span}')
                consumed[annotation] = span
        # An unreachable annotation becoming covered signals a stale audit. An
        # off-path file may be covered by its package unit tests; that does not
        # put it on the condition path.
        if count and reason and not off_path:
            failures.append(f'stale unreachable annotation: {file}:{span}')
        if not count and reason:
            totals[file][2] += n
            exclusions.append((file, span, n, reason))
        elif not count:
            failures.append(f'uncovered reachable block: {file}:{span} ({n} statements)')
    for file, annotations in UNREACHABLE.items():
        for span, n in annotations:
            if (file, span, n) not in consumed:
                failures.append(f'unmatched unreachable annotation: {FORK}{file}:{span} ({n} statements)')
    expected = {FORK + name for name in ('EvaluableExpression.go', 'parsing.go', 'stagePlanner.go', 'evaluationStage.go', 'lexerState.go', 'lexerStream.go', 'parameters.go', 'sanitizedParameters.go')}
    expected |= {'pkg/eval/condition/condition.go', 'pkg/eval/function/builtin_functions.go'}
    for missing in sorted(expected - totals.keys()):
        failures.append(f'missing instrumented file: {missing}')
    print('| File | Covered / statements | Raw coverage | Excluded | Reachable coverage |')
    print('|---|---:|---:|---:|---:|')
    for file, (total, covered, excluded) in sorted(totals.items()):
        scoped = f'{100 * covered / (total - excluded):.1f}%' if total != excluded else 'outside path'
        print(f'| `{file}` | {covered}/{total} | {100 * covered / total:.1f}% | {excluded} | {scoped} |')
    print('\nAudited exclusions (full Go coverage block spans, including closing braces):\n')
    print('| File:block | Statements | Reason |\n|---|---:|---|')
    for file, span, n, reason in exclusions:
        print(f'| `{file}:{span}` | {n} | {reason} |')
    if matrix is None:
        failures.append('missing decision matrix evidence')
    else:
        print('\nGenerated decision matrix (each case runs through both paths):\n')
        matrix_rows = 0
        matrix_totals = [0, 0, 0]
        declared_totals = None
        for line in Path(matrix).read_text().splitlines():
            match = re.search(r'condition_conformance_test.go:\d+:\s?(.*)', line)
            content = match.group(1).strip() if match else line.strip()
            if match:
                if content.startswith(('|', 'Section ', 'Known differences', 'Difference:',
                                       'By-design behaviours', 'Design:',
                                       'Matrix cells ', 'Outcome totals:', 'Error cases ', 'Error:')):
                    print(content)
            elif content.startswith('|'):
                print(content)
            cells = re.findall(r'\|\s*(\d+)/(\d+)/(\d+)\s*(?=\|)', content) if content.startswith('|') else []
            if cells:
                matrix_rows += 1
                for cell in cells:
                    for i, count in enumerate(cell):
                        matrix_totals[i] += int(count)
            outcome = re.fullmatch(r'Outcome totals: (\d+) match / (\d+) no-match / (\d+) error', content)
            if outcome:
                declared_totals = [int(n) for n in outcome.groups()]
        if not matrix_rows:
            failures.append('decision matrix has no case rows (matrix test missing or skipped)')
        if declared_totals is None:
            failures.append('decision matrix is missing Outcome totals')
        elif not sum(declared_totals):
            failures.append('decision matrix contains no cases')
        elif matrix_totals != declared_totals:
            failures.append(f'decision matrix row counts {matrix_totals} do not match Outcome totals {declared_totals}')
    for failure in failures:
        print(f'ERROR: {failure}')
    return bool(failures)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go-version', required=True, help='go env GOVERSION from the coverage run')
    parser.add_argument('profile')
    parser.add_argument('matrix')
    args = parser.parse_args()
    raise SystemExit(report(args.profile, args.matrix, go_version=args.go_version))
