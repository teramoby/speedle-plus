/* Guard the reviewed inline-Python inventory in .github/workflows and scripts/ci.
 *
 * This is deliberately a line-oriented inventory, not a shell or YAML parser.
 * The runtime guarantee is python3 -I plus two statements at the top of every
 * -c and stdin program: sys.flags.isolated must be true and optimization must
 * be off, and neither the checkout, the current directory nor an empty string
 * may be on sys.path. Those statements exit non-zero instead of using assert,
 * which python -O strips. Module (-m) invocations are verified to carry -I
 * only: a stdlib module body cannot hold the guard, so the cwd staying off
 * sys.path rests on CPython's documented -I behaviour rather than on a check
 * this inventory can run.
 *
 * Every known -c, -m and heredoc invocation in .github/workflows/*.yml and
 * scripts/ci/*.sh is listed below. A new inline-shaped Python invocation fails
 * the inventory assertion so a reviewer must add it explicitly. Backslash
 * continuations are folded, and -c guards are read only from a single-quoted
 * payload on the invocation line; a multi-line or otherwise unparseable
 * payload fails closed. Comments, printed examples, other commands' heredoc
 * bodies and Python script files (which execute their own path) are outside
 * this inventory. Shell scripts outside scripts/ci are outside this change. */
const assert = require('node:assert/strict');
const {spawnSync} = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const repo = path.resolve(__dirname, '../..');
const sourceRoots = [
  {directory: path.join(repo, '.github/workflows'), pattern: /\.ya?ml$/},
  {directory: path.join(repo, 'scripts/ci'), pattern: /\.sh$/},
];
const isolationGuard = 'import sys; sys.flags.isolated or sys.exit("run with python3 -I"); sys.flags.optimize == 0 or sys.exit("python3 inline checks require optimization to be disabled")';
const pathGuard = 'import os; unsafe = any(p in sys.path for p in ("", ".", os.getcwd(), os.environ.get("GITHUB_WORKSPACE")) if p is not None); unsafe and sys.exit("python3 -I must exclude the checkout and working directory from sys.path")';
const guard = `${isolationGuard}; ${pathGuard}`;
const interpreter = String.raw`\bpython(?:3(?:\.[0-9]+)?)?\b`;
const commandPattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*?\s+-[A-Za-z]*c(?=\s|['"]|$)`);
const modulePattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*?\s+-[A-Za-z]*m(?=\s|['"]|$)`);
const stdinPattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*(?:(?:\s+-)?\s*<<|\s+-(?=\s|$))`);
const isolatedPattern = new RegExp(String.raw`${interpreter}\s+-I(?=\s|$)`);
const printedExample = /^(?:-\s*)?(?:run:\s*)?(?:echo|printf)\b/;
const pipedPython = /\|\s*python(?:3(?:\.[0-9]+)?)?\b/;

function commandGuarded(payload) {
  return payload === guard || payload.startsWith(`${guard}; `);
}

function sources() {
  return sourceRoots.flatMap(({directory, pattern}) => fs.readdirSync(directory)
    .filter(name => pattern.test(name)).sort()
    .map(name => {
      const absolute = path.join(directory, name);
      return [path.relative(repo, absolute), absolute];
    }));
}

function inspectSource(file, source) {
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
      const isolated = isolatedPattern.test(line);
      invocations.push({
        file, line: start + 1, kind, isolated,
        // -m has no inline payload to instrument, so the only available check
        // is that -I is present; -c and stdin programs must also carry the
        // runtime guard.
        guarded: kind === 'module' ? isolated : (kind === 'command'
          ? commandGuarded(payload)
          : lines[index + 1]?.trim() === isolationGuard && lines[index + 2]?.trim() === pathGuard),
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

test('the explicit workflow and CI-script inline-Python inventory is isolated and guarded', () => {
  const files = sources();
  assert.ok(files.length > 0, 'no sources discovered');
  const invocations = files.flatMap(([file, absolute]) => inspectSource(file, fs.readFileSync(absolute, 'utf8')));
  assert.equal(invocations.length, 21, 'discovered invocation count must match the reviewed inventory');
  assert.deepEqual(invocations.filter(invocation => !invocation.isolated), []);
  assert.deepEqual(invocations.filter(invocation => !invocation.guarded), []);

  assertInventory(invocations, [
    ['.github/workflows/ci.yml', 'module'],
    ['.github/workflows/ci.yml', 'stdin'],
    ['.github/workflows/go-ci-worker.yml', 'command'],
    ['.github/workflows/go-ci-worker.yml', 'command'],
    ['.github/workflows/go-ci-worker.yml', 'stdin'],
    ['.github/workflows/go-ci-worker.yml', 'stdin'],
    ['.github/workflows/go-ci-worker.yml', 'stdin'],
    ['.github/workflows/go-ci-worker.yml', 'stdin'],
    ['.github/workflows/image-smoke.yml', 'stdin'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'command'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'command'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'command'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'stdin'],
    ['scripts/ci/condition-conformance.sh', 'module'],
    ['scripts/ci/run-go-tests.sh', 'command'],
    ['scripts/ci/run-go-tests.sh', 'command'],
    ['scripts/ci/run-go-tests.sh', 'stdin'],
    ['scripts/ci/run-go-tests.sh', 'module'],
    ['scripts/ci/run-go-tests.sh', 'module'],
    ['scripts/ci/run-go-tests.sh', 'module'],
    ['scripts/ci/validate-mongodb.sh', 'stdin'],
  ]);
});

test('the inventory recognizes inline forms and ignores files, examples and data heredocs', () => {
  const source = [
    `python3 -I -c '${guard}; print(1)'`,
    'python3 -I \\',
    `-c '${guard}; print(2)'`,
    'python3.12 -I -m unittest discover',
    "python -I - <<'PY'",
    isolationGuard,
    pathGuard,
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
  assert.deepEqual(inspectSource('scripts/ci/fixture.sh', source)
    .map(({kind, isolated, guarded}) => ({kind, isolated, guarded})), [
    {kind: 'command', isolated: true, guarded: true},
    {kind: 'command', isolated: true, guarded: true},
    {kind: 'module', isolated: true, guarded: true},
    {kind: 'stdin', isolated: true, guarded: true},
  ]);
});

test('missing guards, unparseable -c payloads and unlisted invocations fail', () => {
  const unsafe = inspectSource('fixture.yml', [
    "python3 -c 'print(1)'",
    'python3 -m unittest discover',
    `python3 -I -c '${isolationGuard}; print(1)'`,
    `python3 -I -c 'print(1)' # ${guard}`,
    "python3 -I -c 'import sys",
    "print(1)'",
    "python3 -I - <<'PY'",
    isolationGuard,
    "print('stdin')",
    'PY',
    "printf 'print(1)' | python3 -I -",
  ].join('\n'));
  assert.deepEqual(unsafe.map(({kind, isolated, guarded}) => ({kind, isolated, guarded})), [
    {kind: 'command', isolated: false, guarded: false},
    {kind: 'module', isolated: false, guarded: false},
    {kind: 'command', isolated: true, guarded: false},
    {kind: 'command', isolated: true, guarded: false},
    {kind: 'command', isolated: true, guarded: false},
    {kind: 'stdin', isolated: true, guarded: false},
    {kind: 'stdin', isolated: true, guarded: false},
  ]);
  assert.throws(() => assertInventory(unsafe, []), /unexpected inline Python invocation/);
  assert.throws(() => assertInventory([], [['fixture.yml', 'command']]), /missing expected command invocation/);
});

test('the runtime guard exits non-zero on an optimizing interpreter', () => {
  const isolated = spawnSync('python3', ['-I', '-c', guard], {encoding: 'utf8'});
  assert.equal(isolated.status, 0, isolated.stderr);
  const optimized = spawnSync('python3', ['-I', '-O', '-c', guard], {encoding: 'utf8'});
  assert.notEqual(optimized.status, 0);
  assert.match(optimized.stderr, /python3 inline checks require optimization to be disabled/);
});
