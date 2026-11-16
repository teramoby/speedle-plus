/* Guard the reviewed inline-Python inventory in .github/workflows.
 *
 * This is deliberately a line-oriented inventory, not a shell or YAML parser.
 * The runtime guarantee is python3 -I plus two statements in every inline
 * program: sys.flags.isolated must be true, and neither the checkout nor the
 * current directory may be on sys.path. Module invocations only require -I.
 * Script files execute their own path on purpose and are outside this guard.
 *
 * Every known -c, -m and heredoc invocation is listed below. A new inline-shaped
 * Python invocation fails the inventory assertion so a reviewer must add it
 * explicitly. Backslash continuations are folded, and -c assertions are read
 * only from the single-quoted payload shape used by the reviewed invocations.
 * Comments, printed examples and other commands' heredoc bodies are ignored;
 * this test does not attempt to classify arbitrary shell grammar. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const workflows = path.resolve(__dirname, '../../.github/workflows');
const isolationAssertion = 'import sys; assert sys.flags.isolated';
const pathAssertion = 'import os; assert not any(p in sys.path for p in ("", ".", os.getcwd(), os.environ.get("GITHUB_WORKSPACE")) if p is not None)';
const interpreter = String.raw`\bpython(?:3(?:\.[0-9]+)?)?\b`;
const commandPattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*?\s+-[A-Za-z]*c(?=\s|['"]|$)`);
const modulePattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*?\s+-[A-Za-z]*m(?=\s|['"]|$)`);
const stdinPattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*(?:(?:\s+-)?\s*<<|\s+-(?=\s|$))`);
const isolatedPattern = new RegExp(String.raw`${interpreter}\s+-I(?=\s|$)`);
const printedExample = /^(?:-\s*)?(?:run:\s*)?(?:echo|printf)\b/;
const pipedPython = /\|\s*python(?:3(?:\.[0-9]+)?)?\b/;

function commandAssertsSafePath(payload) {
  if (!payload.startsWith(isolationAssertion)) return false;
  const rest = payload.slice(isolationAssertion.length);
  return rest.startsWith(`; ${pathAssertion}`) ||
    rest.startsWith(`, "run with python3 -I"; ${pathAssertion}`) ||
    rest.startsWith(`, 'run with python3 -I'; ${pathAssertion}`);
}

function inspectWorkflow(file, source) {
  const lines = source.split('\n');
  const invocations = [];
  let heredoc = null;
  for (let index = 0; index < lines.length; index++) {
    let line = lines[index];
    let text = line.trim();
    if (heredoc) {
      if (text === heredoc) heredoc = null;
      continue;
    }
    const start = index;
    while (line.endsWith('\\') && index + 1 < lines.length) {
      line = line.slice(0, -1) + lines[++index];
    }
    text = line.trim();
    if (!text || text.startsWith('#') || (printedExample.test(text) && !pipedPython.test(text))) continue;
    let kind;
    let payload = '';
    const commandMatch = line.match(commandPattern);
    if (commandMatch) {
      kind = 'command';
      const afterOption = line.slice(commandMatch.index + commandMatch[0].length);
      payload = afterOption.match(/^\s+'([^']*)'/)?.[1] ?? '';
    } else if (modulePattern.test(line)) kind = 'module';
    else if (stdinPattern.test(line)) kind = 'stdin';
    if (kind) {
      invocations.push({
        file, line: start + 1, kind,
        isolated: isolatedPattern.test(line),
        asserted: kind === 'module' || (kind === 'command'
          ? payload.startsWith(isolationAssertion)
          : lines[index + 1]?.trim().startsWith(isolationAssertion) ?? false),
        pathSafe: kind === 'module' || (kind === 'command'
          ? commandAssertsSafePath(payload)
          : lines[index + 2]?.trim().startsWith(pathAssertion) ?? false),
      });
    }
    // Skip data heredocs as well as Python ones. This small delimiter tracker
    // avoids treating their contents as shell without parsing shell grammar.
    const marker = line.match(/<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?/);
    if (marker) heredoc = marker[1];
  }
  return invocations;
}

function assertInventory(actual, expected) {
  const remaining = [...actual];
  for (const [file, kind] of expected) {
    const index = remaining.findIndex(invocation => invocation.file === file && invocation.kind === kind);
    assert.notEqual(index, -1, `missing expected ${kind} invocation in ${file}`);
    remaining.splice(index, 1);
  }
  assert.deepEqual(remaining, [], 'unexpected inline Python invocation; review it and update the explicit inventory');
}

test('the explicit workflow inline-Python inventory is isolated and path-safe', () => {
  const files = fs.readdirSync(workflows).filter(name => /\.ya?ml$/.test(name)).sort();
  assert.ok(files.length > 0, 'no workflows discovered');
  const invocations = files.flatMap(file => inspectWorkflow(file, fs.readFileSync(path.join(workflows, file), 'utf8')));
  assert.equal(invocations.length, 13, 'discovered invocation count must match the reviewed inventory');
  assert.deepEqual(invocations.filter(invocation => !invocation.isolated), []);
  assert.deepEqual(invocations.filter(invocation => !invocation.asserted), []);
  assert.deepEqual(invocations.filter(invocation => !invocation.pathSafe), []);

  assertInventory(invocations, [
    ['ci.yml', 'module'],
    ['ci.yml', 'stdin'],
    ['go-ci-worker.yml', 'command'],
    ['go-ci-worker.yml', 'command'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['go-ci-worker.yml', 'stdin'],
    ['image-smoke.yml', 'stdin'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'command'],
    ['mongodb-nightly-validation.yml', 'stdin'],
  ]);
});

test('the inventory recognizes inline forms and ignores files, examples and data heredocs', () => {
  const source = [
    `python3 -I -c '${isolationAssertion}; ${pathAssertion}; print(1)'`,
    'python3 -I \\',
    `-c '${isolationAssertion}; ${pathAssertion}; print(2)'`,
    'python3.12 -I -m unittest discover',
    "python -I - <<'PY'",
    isolationAssertion,
    pathAssertion,
    "print('stdin')",
    'PY',
    'python3 scripts/ci/test_report.py -c',
    'python3 scripts/ci/run.py < input.txt',
    "echo python3 -c 'not an executed example'",
    "# python3 -c 'comment only'",
    "cat <<'DATA'",
    "python3 -c 'heredoc data, not a command'",
    'DATA',
  ].join('\n');
  assert.deepEqual(inspectWorkflow('fixture.yml', source).map(({kind, isolated, asserted, pathSafe}) =>
    ({kind, isolated, asserted, pathSafe})), [
    {kind: 'command', isolated: true, asserted: true, pathSafe: true},
    {kind: 'command', isolated: true, asserted: true, pathSafe: true},
    {kind: 'module', isolated: true, asserted: true, pathSafe: true},
    {kind: 'stdin', isolated: true, asserted: true, pathSafe: true},
  ]);
});

test('missing runtime assertions and unlisted invocations fail', () => {
  const unsafe = inspectWorkflow('fixture.yml', [
    "python3 -c 'print(1)'",
    `python3 -I -c '${isolationAssertion}; print(1)'`,
    `python3 -I -c 'print(1)' # ${isolationAssertion}; ${pathAssertion}`,
    "python3 -I - <<'PY'",
    isolationAssertion,
    "print('stdin')",
    'PY',
    "printf 'print(1)' | python3 -I -",
  ].join('\n'));
  assert.deepEqual(unsafe.map(({kind, isolated, asserted, pathSafe}) => ({kind, isolated, asserted, pathSafe})), [
    {kind: 'command', isolated: false, asserted: false, pathSafe: false},
    {kind: 'command', isolated: true, asserted: true, pathSafe: false},
    {kind: 'command', isolated: true, asserted: false, pathSafe: false},
    {kind: 'stdin', isolated: true, asserted: true, pathSafe: false},
    {kind: 'stdin', isolated: true, asserted: false, pathSafe: false},
  ]);
  assert.throws(() => assertInventory(unsafe, []), /unexpected inline Python invocation/);
  assert.throws(() => assertInventory([], [['fixture.yml', 'command']]), /missing expected command invocation/);
});
