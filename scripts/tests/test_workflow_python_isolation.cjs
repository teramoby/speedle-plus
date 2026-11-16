/* Inline workflow Python must not import from the checkout or runner-local paths. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const workflows = path.resolve(__dirname, '../../.github/workflows');

function inlinePythonInvocations(source) {
  // Match Python at a shell command position. Script filenames, `command -v`,
  // version reporting, comments and quoted examples do not execute inline code.
  const pattern = /(?:^[ \t]*(?:run:[ \t]+)?|[;&|()][ \t]*|\b(?:if|then|elif|else|while|until|do)[ \t]+)(?:![ \t]+)?(?:[A-Za-z_][A-Za-z0-9_]*=\S+[ \t]+)*python3(?<options>(?:[ \t]+-[A-Za-z]+)*?)[ \t]+(?<mode>-c(?=[ \t]|$)|-(?=[ \t]|$))/gm;
  return [...source.matchAll(pattern)].map(match => {
    const commandOffset = match[0].indexOf('python3');
    return {
      line: source.slice(0, match.index + commandOffset).split('\n').length,
      kind: match.groups.mode === '-c' ? 'command' : 'stdin',
      isolated: /(?:^|[ \t])-I(?=[ \t]|$)/.test(match.groups.options),
    };
  });
}

test('every inline Python invocation in every workflow uses isolated mode', () => {
  const files = fs.readdirSync(workflows).filter(name => /\.ya?ml$/.test(name)).sort();
  assert.ok(files.length > 0, 'no workflows discovered');
  const invocations = files.flatMap(file => inlinePythonInvocations(fs.readFileSync(path.join(workflows, file), 'utf8'))
    .map(invocation => ({file, ...invocation})));
  assert.deepEqual(invocations.filter(invocation => !invocation.isolated), []);

  // Keep concrete coverage of the original call sites as well as the general
  // guard, so a scanner regression cannot pass by discovering no invocations.
  const remaining = [...invocations];
  const expected = [
    ['ci.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['image-smoke.yml', 'stdin'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'stdin'],
  ];
  for (const [file, kind] of expected) {
    const index = remaining.findIndex(invocation => invocation.file === file && invocation.kind === kind);
    assert.notEqual(index, -1, `missing expected ${kind} invocation in ${file}`);
    remaining.splice(index, 1);
  }
});

test('the guard detects unisolated inline code at shell command positions', () => {
  const invocations = inlinePythonInvocations([
    "python3 - <<'PYTHON'",
    "  python3 -I - <<'PYTHON'",
    "if python3 -c \"print('selected')\"; then",
    "  CI_TASK=nightly python3 -E -c 'print(1)'",
    "  true && python3 -I -c 'print(1)'",
    "        run: python3 -c 'print(1)'",
    '  python3 scripts/ci/test_report.py -c',
    '  command -v python3 >/dev/null',
    '  bash --version; python3 --version',
    "  # python3 -c 'comment only'",
    "  echo 'python3 -c quoted text'",
  ].join('\n'));
  assert.deepEqual(invocations, [
    {line: 1, kind: 'stdin', isolated: false},
    {line: 2, kind: 'stdin', isolated: true},
    {line: 3, kind: 'command', isolated: false},
    {line: 4, kind: 'command', isolated: false},
    {line: 5, kind: 'command', isolated: true},
    {line: 6, kind: 'command', isolated: false},
  ]);
});
