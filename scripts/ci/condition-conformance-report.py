#!/usr/bin/env python3
"""Report the SPDL condition path, keeping raw and scoped coverage separate.

Run from the repository root (profile uses covermode=count or atomic as well):
  go test -count=1 -coverpkg=./pkg/eval/condition,./pkg/eval/function,./3rdparty/github.com/Knetic/govaluate -coverprofile=condition.out ./pkg/eval/... ./pkg/svcs/adsrest/... ./3rdparty/github.com/Knetic/govaluate
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
FORK = '3rdparty/github.com/Knetic/govaluate/'
# These files implement SQL output only; neither the condition validator nor
# the evaluator invokes ToSQLQuery or its stream. Report their raw coverage too.
UNUSED_FILES = {
    FORK + 'EvaluableExpression_sql.go': 'SQL serialization is not called by the Engine condition path.',
    FORK + 'expressionOutputStream.go': 'Output stream is used only by SQL serialization, not condition evaluation.',
}
# Exact coverage spans and statement counts, not broad function exclusions. A new uncovered block
# fails the report, requiring either a test or a fresh reachability audit.
UNREACHABLE = {'EvaluableExpression.go': {('82.3,83.1', 1): 'Validated token kinds always produce a stage; the '
                                              'token-constructor planner error cannot follow successful syntax '
                                              'validation.',
                            ('112.3,113.1', 1): 'parseTokens already checked balance and returned its error; the '
                                                'repeated balance check cannot fail.',
                            ('127.3,128.1', 1): 'The text parser and syntax validator reject unplannable tokens '
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
 'stagePlanner.go': {('187.3,188.1', 1): 'planTokens can error only through unplannable token kinds, already '
                                         'rejected by syntax validation.',
                     ('201.3,202.1', 1): 'Nonempty validated expressions reach planTokens; token streams consumed '
                                         'within clauses are handled by planValue CLAUSE_CLOSE.',
                     ('230.4,231.1', 1): 'Left precedence planner error propagation; all validated kinds have '
                                         'value planners.',
                     ('256.5,256.10', 1): 'The valid-symbol tables receive only string operator tokens from the '
                                          'lexer and syntax validator.',
                     ('268.5,269.1', 1): 'Right precedence planner error propagation; all validated kinds have '
                                         'value planners.',
                     ('310.3,311.1', 1): 'Function argument planner error propagation after successful syntax '
                                         'validation.',
                     ('329.3,330.1', 1): 'A validated FUNCTION always has a following clause, so planAccessor '
                                         'cannot receive an exhausted stream.',
                     ('351.5,352.1', 1): 'Accessor argument planner error propagation after successful syntax '
                                         'validation.',
                     ('380.3,381.1', 1): 'A validated expression or prefix always has a following value or '
                                         'clause-close token.',
                     ('391.4,392.1', 1): 'Clause planner error propagation; invalid contents have already failed '
                                         'syntax validation.',
                     ('437.3,439.1', 2): 'All value token kinds accepted by syntax validation have an operator; '
                                         'this rejects malformed internal token streams only.',
                     ('595.3,596.1', 1): 'A planned operator chain ends at an operand with distinct precedence; '
                                         'matching chains are mirrored at the precedence break, not after the '
                                         'final leaf.',
                     ('692.3,693.1', 1): 'LITERAL stages use makeLiteralStage, whose closure always returns a '
                                         'value with nil error.',
                     ('697.3,698.1', 1): 'LITERAL stages use makeLiteralStage, whose closure always returns a '
                                         'value with nil error.',
                     ('718.3,719.1', 1): 'This is the root.operator error return after the elideStage type '
                                         'checks at lines 701-713. Literal type errors such as true+false '
                                         'exit at those earlier checks and are later reported by Evaluate; '
                                         'they never reach this return. Pure well-typed operators return nil '
                                         'errors; literal regex errors were rejected by optimizeTokens. '
                                         'Functions/accessors are not LITERAL.'}}

# Bind the audit to source contents as well as coverage spans. A source edit
# requires an explicit reachability re-audit even if its line numbers survive.
AUDITED_SOURCE_SHA256 = {'EvaluableExpression.go': '8402d0eadf149b6bf511e992e89ef9fb8f9e29d78e3a433ac09b59edf29f2555',
 'EvaluableExpression_sql.go': '5d6a75e21e68fdd19ee4aafa92e0bc15f14c76ee7d488d36647f541db3300699',
 'expressionOutputStream.go': 'd8fee24cc6d334bbfe5880b382bb9a659ffffeca0f3ed917b12f2594bdde47d2',
 'lexerState.go': '5071044819e265850070b61495692e9fb78f97785cafb829c1296a43b51790e0',
 'parsing.go': 'c77b0949ff985d2a5683c06824eb8529ad4b830c6bab1b166f97d0df6e35fab6',
 'stagePlanner.go': 'e46fc41044ec3b2e6ea66b5bb570de68cce3360e494d5e05e4ebb72187948650'}


# Go 1.26 records the enclosing control-flow boundary; Go 1.27 starts at
# the first statement. Both layouts below were verified against the identical
# source hashes above. Unexplained blocks still fail closed, independently of
# the reported toolchain version. Values identify the
# canonical annotation, so every annotation must match in either layout.
GO126_SPANS = {('EvaluableExpression.go', '81.16,83.3', 1): ('82.3,83.1', 1),
 ('EvaluableExpression.go', '111.16,113.3', 1): ('112.3,113.1', 1),
 ('EvaluableExpression.go', '126.16,128.3', 1): ('127.3,128.1', 1),
 ('lexerState.go', '345.17,347.4', 1): ('346.4,347.1', 1),
 ('parsing.go', '48.17,50.4', 1): ('49.4,50.1', 1),
 ('parsing.go', '87.33,88.12', 1): ('88.4,88.12', 1),
 ('parsing.go', '343.26,344.13', 1): ('344.5,344.13', 1),
 ('parsing.go', '442.35,444.2', 1): ('443.2,444.1', 1),
 ('stagePlanner.go', '186.16,188.3', 1): ('187.3,188.1', 1),
 ('stagePlanner.go', '200.23,202.3', 1): ('201.3,202.1', 1),
 ('stagePlanner.go', '229.17,231.4', 1): ('230.4,231.1', 1),
 ('stagePlanner.go', '255.30,256.10', 1): ('256.5,256.10', 1),
 ('stagePlanner.go', '267.18,269.5', 1): ('268.5,269.1', 1),
 ('stagePlanner.go', '309.16,311.3', 1): ('310.3,311.1', 1),
 ('stagePlanner.go', '328.23,330.3', 1): ('329.3,330.1', 1),
 ('stagePlanner.go', '350.18,352.5', 1): ('351.5,352.1', 1),
 ('stagePlanner.go', '379.23,381.3', 1): ('380.3,381.1', 1),
 ('stagePlanner.go', '390.17,392.4', 1): ('391.4,392.1', 1),
 ('stagePlanner.go', '436.21,439.3', 2): ('437.3,439.1', 2),
 ('stagePlanner.go', '594.35,596.3', 1): ('595.3,596.1', 1),
 ('stagePlanner.go', '691.16,693.3', 1): ('692.3,693.1', 1),
 ('stagePlanner.go', '696.16,698.3', 1): ('697.3,698.1', 1),
 ('stagePlanner.go', '717.16,719.3', 1): ('718.3,719.1', 1)}

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
        actual_hash = hashlib.sha256((ROOT / FORK / file).read_bytes()).hexdigest()
        if actual_hash != expected_hash:
            failures.append(f'audited source changed; re-audit required: {FORK}{file}')
    for (file, span), (n, count) in sorted(blocks.items()):
        totals[file][0] += n
        if count:
            totals[file][1] += n
        reason = UNUSED_FILES.get(file)
        if reason is None and file.startswith(FORK):
            short_file = file[len(FORK):]
            canonical = GO126_SPANS.get((short_file, span, n), (span, n))
            reason = UNREACHABLE.get(short_file, {}).get(canonical)
            if reason:
                annotation = (short_file, *canonical)
                if annotation in consumed and consumed[annotation] != span:
                    failures.append(f'mixed coverage layouts for one annotation: {file}:{span}')
                consumed[annotation] = span
        # An unreachable annotation becoming covered signals a stale audit.
        if count and reason and file not in UNUSED_FILES:
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
