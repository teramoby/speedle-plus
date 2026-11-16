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
 * Every known -c, -m, heredoc and script-file invocation in
 * .github/workflows/*.yml and scripts/ci/*.sh is listed below. A new
 * inline-shaped Python invocation fails the inventory assertion so a reviewer
 * must add it explicitly. Backslash continuations are folded, and -c guards
 * are read only from a single-quoted payload on the invocation line; a
 * multi-line or otherwise unparseable payload fails closed. Comments, printed
 * examples and other commands' heredoc bodies are outside this inventory.
 * Shell scripts outside scripts/ci are outside this change. */
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
const scriptPattern = new RegExp(String.raw`${interpreter}(?:\s+-[A-Za-z]+)*?\s+("[^"]+"|'[^']+'|[^\s'";|&)<>][^\s'";|&)]*/[^\s'";|&)]*|[^\s'";|&)<>][^\s'";|&)]*\.py)(?=\s|$)`);
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
    let operand = '';
    const commandMatch = line.match(commandPattern);
    if (commandMatch) {
      kind = 'command';
      const afterOption = line.slice(commandMatch.index + commandMatch[0].length);
      payload = afterOption.match(/^\s+'([^']*)'/)?.[1] ?? '';
    } else if (modulePattern.test(line)) kind = 'module';
    else if (stdinPattern.test(line)) kind = 'stdin';
    else {
      const scriptMatch = line.match(scriptPattern);
      if (scriptMatch) {
        kind = 'script';
        operand = scriptMatch[1].replace(/^['"]|['"]$/g, '');
      }
    }
    if (kind) {
      const isolated = isolatedPattern.test(line);
      invocations.push({
        file, line: start + 1, kind, isolated, operand,
        // -m and script-file invocations have no inline payload to instrument,
        // so the only available check is that -I is present; -c and stdin
        // programs must also carry the runtime guard.
        guarded: kind === 'module' || kind === 'script' ? isolated : (kind === 'command'
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
  assert.equal(invocations.length, 56, 'discovered invocation count must match the reviewed inventory');
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
    ['.github/workflows/go-ci-worker.yml', 'script'],
    ['.github/workflows/go-ci-worker.yml', 'script'],
    ['.github/workflows/go-ci-worker.yml', 'script'],
    ['.github/workflows/go-ci-worker.yml', 'script'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'script'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'script'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'script'],
    ['.github/workflows/mongodb-nightly-validation.yml', 'script'],
    ['.github/workflows/nightly.yml', 'script'],
    ['.github/workflows/post-merge.yml', 'script'],
    ['.github/workflows/reviewbot-command.yml', 'script'],
    ['.github/workflows/reviewbot-command.yml', 'script'],
    ['.github/workflows/reviewbot.yml', 'script'],
    ['.github/workflows/reviewbot.yml', 'script'],
    ['.github/workflows/reviewbot.yml', 'script'],
    ['.github/workflows/reviewbot.yml', 'script'],
    ['.github/workflows/reviewbot.yml', 'script'],
    ['.github/workflows/weekly.yml', 'script'],
    ['scripts/ci/condition-conformance.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/run-go-tests.sh', 'script'],
    ['scripts/ci/validate-etcd.sh', 'script'],
    ['scripts/ci/validate-mongodb.sh', 'script'],
  ]);
});

test('the inventory recognizes inline and script forms and ignores examples and data heredocs', () => {
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
    'python3 -I scripts/ci/test_report.py --kind nightly',
    'python3 -I "${helper}" --flag',
    'python3 --version',
    'for tool in gcc python3 node jq; do command -v "${tool}"; done',
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
    {kind: 'script', isolated: true, guarded: true},
    {kind: 'script', isolated: true, guarded: true},
  ]);
});

test('missing guards, unparseable -c payloads and unlisted invocations fail', () => {
  const unsafe = inspectSource('fixture.yml', [
    "python3 -c 'print(1)'",
    'python3 -m unittest discover',
    'python3 scripts/ci/run.py',
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
    {kind: 'script', isolated: false, guarded: false},
    {kind: 'command', isolated: true, guarded: false},
    {kind: 'command', isolated: true, guarded: false},
    {kind: 'command', isolated: true, guarded: false},
    {kind: 'stdin', isolated: true, guarded: false},
    {kind: 'stdin', isolated: true, guarded: false},
  ]);
  assert.throws(() => assertInventory(unsafe, []), /unexpected inline Python invocation/);
  assert.throws(() => assertInventory([], [['fixture.yml', 'command']]), /missing expected command invocation/);
});

test('every converted script imports under python3 -I', () => {
  // -I removes the script's own directory from sys.path, so importing each
  // helper without running __main__ proves its sibling loads work under the
  // flag before a merge-blocking suite executes it for real.
  const invocations = sources()
    .flatMap(([file, absolute]) => inspectSource(file, fs.readFileSync(absolute, 'utf8')));
  const scripts = new Set(invocations
    .filter(invocation => invocation.kind === 'script' && invocation.operand.endsWith('.py') &&
      !invocation.operand.includes('$'))
    .map(invocation => invocation.operand));
  // Helpers extracted from the trusted revision at run time still live here.
  scripts.add('.github/codex/reviewbot_response.py');
  scripts.add('.github/codex/reviewbot_handoff.py');
  assert.ok(scripts.size > 10, 'the converted script set must not silently shrink');
  for (const script of [...scripts].sort()) {
    assert.ok(fs.existsSync(path.resolve(repo, script)), `missing converted script ${script}`);
    const result = spawnSync('python3', ['-I', '-B', '-c',
      'import importlib.util, sys; spec = importlib.util.spec_from_file_location("isolated_check", sys.argv[1]); module = importlib.util.module_from_spec(spec); sys.modules["isolated_check"] = module; spec.loader.exec_module(module)',
      script], {cwd: repo, encoding: 'utf8'});
    assert.equal(result.status, 0, `${script} must import under python3 -I: ${result.stderr}`);
  }
});

function importedNames(source) {
  const names = [];
  for (const line of source.split('\n')) {
    const match = line.match(/^\s*(?:from\s+([A-Za-z_][A-Za-z0-9_]*)\s+import|import\s+([A-Za-z_][A-Za-z0-9_]*))/);
    if (match) names.push(match[1] || match[2]);
  }
  return names;
}

test('script-mode helpers do not import same-directory modules by bare name', () => {
  // -I removes the script's own directory from sys.path, so a bare sibling
  // import fails at run time. Helpers load siblings with _load_sibling; this
  // check also inspects the sibling files they load.
  const invocations = sources()
    .flatMap(([file, absolute]) => inspectSource(file, fs.readFileSync(absolute, 'utf8')));
  const pending = invocations
    .filter(invocation => invocation.kind === 'script' && invocation.operand.endsWith('.py'))
    .map(invocation => invocation.operand);
  const checked = new Set();
  while (pending.length > 0) {
    const operand = pending.pop();
    if (operand.includes('$') || checked.has(operand) || !fs.existsSync(path.resolve(repo, operand))) continue;
    checked.add(operand);
    const source = fs.readFileSync(path.resolve(repo, operand), 'utf8');
    const directory = path.dirname(operand);
    const siblings = new Set(fs.readdirSync(path.resolve(repo, directory))
      .filter(name => name.endsWith('.py')).map(name => name.slice(0, -3)));
    assert.deepEqual(importedNames(source).filter(name => siblings.has(name)), [],
      `${operand} imports a same-directory module; -I removes the script directory from sys.path`);
    for (const match of source.matchAll(/_load_sibling\("([A-Za-z_][A-Za-z0-9_]*)"\)/g)) {
      pending.push(path.join(directory, `${match[1]}.py`));
    }
  }
});

test('the runtime guard exits non-zero on an optimizing interpreter', () => {
  const isolated = spawnSync('python3', ['-I', '-c', guard], {encoding: 'utf8'});
  assert.equal(isolated.status, 0, isolated.stderr);
  const optimized = spawnSync('python3', ['-I', '-O', '-c', guard], {encoding: 'utf8'});
  assert.notEqual(optimized.status, 0);
  assert.match(optimized.stderr, /python3 inline checks require optimization to be disabled/);
});
