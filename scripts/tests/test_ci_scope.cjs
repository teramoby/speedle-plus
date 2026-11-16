const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {spawnSync} = require('node:child_process');
const os = require('node:os');
const test = require('node:test');
const root = path.resolve(__dirname, '../..');
const ci = fs.readFileSync(path.join(root, '.github/workflows/ci.yml'), 'utf8');
const worker = fs.readFileSync(path.join(root, '.github/workflows/go-ci-worker.yml'), 'utf8');
function evaluate(expression, context) {
  const lookup = key => {
    function walk(value, keys) {
      if (!keys.length) return value ?? '';
      const [key, ...tail] = keys;
      if (key === '*') return (Array.isArray(value) ? value : []).map(item => walk(item, tail));
      return walk(value?.[key], tail);
    }
    return walk(context, key.split('.'));
  };
  const tokens = /\s+|'(?:[^']|'')*'|(?:github|vars|inputs|needs)(?:\.(?:[A-Za-z_][\w-]*|\*))+|fromJSON|contains|format|always|true|false|null|==|!=|&&|\|\||[(),!]/gy;
  let code = '', offset = 0;
  while (offset < expression.length) {
    tokens.lastIndex = offset;
    const match = tokens.exec(expression);
    assert.ok(match, `Unsupported expression: ${expression.slice(offset)}`);
    const token = match[0]; offset = tokens.lastIndex;
    if (token.startsWith("'")) code += JSON.stringify(token.slice(1, -1).replaceAll("''", "'"));
    else if (/^(github|vars|inputs|needs)\./.test(token)) code += `lookup(${JSON.stringify(token)})`;
    else code += token;
  }
  return JSON.parse(JSON.stringify(vm.runInNewContext(code, {
    lookup, fromJSON: JSON.parse, contains: (values, value) => values.includes(value),
    format: (pattern, value) => pattern.replace('{0}', value), always: () => true,
  }, {timeout: 100})));
}
function job(name) { return ci.split(`  ${name}:\n`)[1].split(/^  [a-z][\w-]*:\n/m)[0]; }
function condition(name) {
  const body = job(name);
  const single = body.match(/^    if: (.+)$/m)?.[1];
  if (single && single !== '>-') return single;
  return body.match(/^    if: >-\n((?:      .*\n)+)/m)?.[1].trim();
}
const fullExpression = job('changes').match(/full: \$\{\{ (.*) \}\}/)[1];
const concurrency = ci.match(/  group: (.*)\n/)[1];
const repository = 'teramoby/speedle-plus';
function context(event = 'pull_request', labels = [], action = 'synchronize', label = '') {
  const github = {repository, event_name: event, sha: 'sha', ref: 'refs/heads/master', run_id: 123,
    event: {action, label: {name: label}, sender: {type: 'User'}, pull_request: {number: 407,
      labels: labels.map(name => ({name})), head: {repo: {full_name: repository}}, user: {type: 'User'}}}};
  return {github, vars: {}, inputs: {mode: 'test'}, needs: {changes: {result: 'success', outputs: {}}}};
}
function group(context) { return concurrency.replace(/\$\{\{ (.*?) \}\}/g, (_, expression) => evaluate(expression, context)); }
test('actual label expressions select quick/full and ignore unrelated/removal events', () => {
  for (const event of ['pull_request', 'push', 'workflow_dispatch']) {
    for (const labels of [[], ['full-ci'], ['merge-head'], ['other'], ['full-ci', 'merge-head']]) {
      for (const action of ['opened', 'synchronize', 'reopened', 'labeled', 'unlabeled']) {
        for (const label of ['other', 'full-ci', 'merge-head']) {
          const c = context(event, labels, action, label);
          const runs = action !== 'unlabeled' && (action !== 'labeled' || ['full-ci', 'merge-head'].includes(label));
          assert.equal(evaluate(condition('changes'), c), runs);
          const full = event !== 'pull_request' || labels.includes('full-ci') || labels.includes('merge-head');
          assert.equal(evaluate(fullExpression, c), full);
          c.needs.changes.outputs.full = String(full);
          assert.equal(evaluate(condition('quick'), c), !full);
          assert.equal(evaluate(condition('test'), c), full);
        }
      }
    }
  }
  assert.match(ci, /types: \[opened, synchronize, reopened, labeled\]/);
  assert.doesNotMatch(ci.match(/types: \[(.*?)\]/)[1], /unlabeled/);
});
test('only relevant additions and pushes share the active PR concurrency group', () => {
  const active = group(context());
  for (const label of ['full-ci', 'merge-head']) assert.equal(group(context('pull_request', [label], 'labeled', label)), active);
  for (const action of ['labeled', 'unlabeled']) {
    const c = context('pull_request', [], action, 'other');
    assert.notEqual(group(c), active);
    c.github.run_id++;
    assert.notEqual(group(c), group(context('pull_request', [], action, 'other')));
  }
});
function script(source, name) {
  return source.split(`      - name: ${name}\n`)[1].split(/^      - /m)[0]
    .split('        run: |\n')[1].split('\n').filter((line, i, lines) => lines.slice(0, i + 1).every(item => !item || item.startsWith('          '))).map(line => line.slice(10)).join('\n').trimEnd();
}
test('change classification preserves raw Git paths, uses the base classifier and fails on diff errors', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-ci-paths-'));
  // The workflow stages the trusted classifier in RUNNER_TEMP, outside the
  // checkout, and runs it with the worktree as the working directory.
  const runnerTemp = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-ci-runner-temp-'));
  try {
    const git = (...args) => {
      const result = spawnSync('git', args, {cwd: dir, encoding: 'utf8'});
      assert.equal(result.status, 0, result.stderr);
      return result.stdout.trim();
    };
    git('init', '-q');
    git('config', 'user.name', 'CI fixture');
    git('config', 'user.email', 'ci@example.invalid');
    git('config', 'commit.gpgsign', 'false');
    git('config', 'core.quotePath', 'true');
    git('commit', '-qm', 'base', '--allow-empty');
    const empty = git('rev-parse', 'HEAD');
    fs.mkdirSync(path.join(dir, 'scripts/ci'), {recursive: true});
    fs.copyFileSync(path.join(root, 'scripts/ci/classify-changes.sh'), path.join(dir, 'scripts/ci/classify-changes.sh'));
    git('add', '--', 'scripts/ci/classify-changes.sh');
    git('commit', '-qm', 'add the classifier');
    const base = git('rev-parse', 'HEAD');
    const classify = (head, overrides = {}) => {
      const output = path.join(dir, 'output');
      fs.writeFileSync(output, '');
      const result = spawnSync('bash', ['-c', script(ci, 'Classify changed files')], {
        cwd: dir, encoding: 'utf8', env: {...process.env, EVENT_NAME: 'pull_request',
          PR_BASE: base, PUSH_BEFORE: base, GITHUB_SHA: head, GITHUB_OUTPUT: output, RUNNER_TEMP: runnerTemp, ...overrides},
      });
      return {...result, output: fs.readFileSync(output, 'utf8')};
    };
    const flags = (go, website, docs, ci) => `go=${go}\nwebsite=${website}\ndocs=${docs}\nci=${ci}\n`;
    const cases = [
      ['docs/使用指南.md', flags(false, false, true, false)],
      ['docs/guide with spaces.md', flags(false, false, true, false)],
      ['website/页面.md', flags(false, true, false, false)],
      // A newline followed by another prefix must remain part of one path.
      ['docs/guide\nwebsite/spoof.md', flags(false, false, true, false)],
      ['pkg/a/文件.go', flags(true, false, false, false)],
      ['scripts/test with spaces.sh', flags(true, false, false, true)],
      ['.github/workflows/ci.yml', flags(true, false, false, true)],
      // Root Markdown runs the CI script tests, including the README/go.mod fixture.
      ['README.md', flags(false, false, false, true)],
      // Module files also carry the toolchain contract those tests assert.
      ['go.mod', flags(true, false, false, true)],
      ['go.sum', flags(true, false, false, true)],
      // Nested Markdown belongs to its Go package, not to the root-only arm.
      ['pkg/a/README.md', flags(true, false, false, false)],
      ['testutil/notes.md', flags(true, false, false, false)],
    ];
    let previous = base;
    for (const [file, expected] of cases) {
      fs.mkdirSync(path.dirname(path.join(dir, file)), {recursive: true});
      fs.writeFileSync(path.join(dir, file), 'fixture\n');
      git('add', '--', file);
      git('commit', '-qm', 'add fixture');
      const head = git('rev-parse', 'HEAD');
      const result = classify(head, {PR_BASE: previous});
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.output, expected, JSON.stringify(file));
      previous = head;
    }
    // A stale merge ref: the base tip has a scripts/ci commit that the pull
    // request side does not contain. Only the pull request's own changes count,
    // so a docs-only change keeps its narrow route.
    const mainline = git('rev-parse', '--abbrev-ref', 'HEAD');
    git('checkout', '-q', '--detach', previous);
    fs.writeFileSync(path.join(dir, 'scripts/ci/base-only.sh'), '#!/usr/bin/env bash\ntrue\n');
    git('add', '--', 'scripts/ci/base-only.sh');
    git('commit', '-qm', 'base-only CI script');
    const staleBase = git('rev-parse', 'HEAD');
    git('checkout', '-q', '--detach', previous);
    fs.mkdirSync(path.join(dir, 'docs'), {recursive: true});
    fs.writeFileSync(path.join(dir, 'docs/stale-ref.md'), 'fixture\n');
    git('add', '--', 'docs/stale-ref.md');
    git('commit', '-qm', 'docs change on an older base');
    const staleHead = git('rev-parse', 'HEAD');
    git('checkout', '-q', mainline);
    const stale = classify(staleHead, {PR_BASE: staleBase});
    assert.equal(stale.status, 0, stale.stderr);
    assert.equal(stale.output, flags(false, false, true, false), 'base-only commits must not widen the route of a pull request');
    assert.equal(classify(previous).output, flags(true, true, true, true), 'mixed paths');
    assert.equal(classify(previous, {PR_BASE: previous}).output, flags(false, false, false, false), 'empty diff');
    const push = classify(previous, {EVENT_NAME: 'push', PR_BASE: 'missing', PUSH_BEFORE: previous});
    assert.equal(push.status, 0, push.stderr);
    assert.equal(push.output, flags(false, false, false, false), 'push uses PUSH_BEFORE, not PR_BASE or the fallback');
    for (const overrides of [{EVENT_NAME: 'workflow_dispatch'}, {PR_BASE: ''},
      {PR_BASE: '0'.repeat(40)}, {PR_BASE: 'missing'}]) {
      const result = classify(previous, overrides);
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.output, flags(true, true, true, true));
    }
    // A real base revision that predates the classifier enables every route.
    const noClassifier = classify(previous, {PR_BASE: empty});
    assert.equal(noClassifier.status, 0, noClassifier.stderr);
    assert.equal(noClassifier.output, flags(true, true, true, true));
    const failed = classify('missing-head');
    assert.notEqual(failed.status, 0);
    assert.equal(failed.output, '', 'diff errors must not publish successful outputs');
    // When the head tree rewrites the classifier (here to report "nothing
    // changed"), the head copy never decides: every route runs, so routes the
    // base copy does not know cannot be skipped either.
    fs.writeFileSync(path.join(dir, 'scripts/ci/classify-changes.sh'),
      '#!/usr/bin/env bash\nprintf "go=false\\nwebsite=false\\ndocs=false\\nci=false\\n" >> "${GITHUB_OUTPUT}"\n');
    fs.mkdirSync(path.join(dir, 'docs'), {recursive: true});
    fs.writeFileSync(path.join(dir, 'docs/guide.md'), 'fixture\n');
    git('add', '--', 'docs/guide.md', 'scripts/ci/classify-changes.sh');
    git('commit', '-qm', 'rewrite the classifier');
    const rewritten = git('rev-parse', 'HEAD');
    const guarded = classify(rewritten, {PR_BASE: previous});
    assert.equal(guarded.status, 0, guarded.stderr);
    assert.equal(guarded.output, flags(true, true, true, true), 'a pull request that changes the classifier must enable every route');
    // A base classifier that omits a route key must not silently disable that
    // route: the step validates the output contract and enables every route.
    fs.mkdirSync(path.join(dir, 'website'), {recursive: true});
    fs.writeFileSync(path.join(dir, 'scripts/ci/classify-changes.sh'),
      '#!/usr/bin/env bash\nprintf "go=false\\nwebsite=false\\ndocs=true\\n" >> "${GITHUB_OUTPUT}"\n');
    git('add', '--', 'scripts/ci/classify-changes.sh');
    git('commit', '-qm', 'classifier that omits a key');
    const partialBase = git('rev-parse', 'HEAD');
    fs.writeFileSync(path.join(dir, 'docs/guide.md'), 'fixture 2\n');
    git('add', '--', 'docs/guide.md');
    git('commit', '-qm', 'docs change under a partial classifier');
    const partialHead = git('rev-parse', 'HEAD');
    const partial = classify(partialHead, {PR_BASE: partialBase});
    assert.equal(partial.status, 0, partial.stderr);
    assert.equal(partial.output, flags(true, true, true, true), 'a missing route key must enable every route');
    assert.match(partial.stdout, /did not set exactly one true\/false value for: ci/);
    // Pushes run the merged classifier under the same output contract.
    const partialPush = classify(partialHead, {EVENT_NAME: 'push', PUSH_BEFORE: partialBase});
    assert.equal(partialPush.status, 0, partialPush.stderr);
    assert.equal(partialPush.output, flags(true, true, true, true), 'a push whose classifier omits a key must enable every route');
    assert.match(partialPush.stdout, /did not set exactly one true\/false value for: ci/);
    // A change to any other CI script also runs every route, even when the
    // classifier itself is unchanged.
    fs.writeFileSync(path.join(dir, 'scripts/ci/helper.sh'), '#!/usr/bin/env bash\ntrue\n');
    git('add', '--', 'scripts/ci/helper.sh');
    git('commit', '-qm', 'another CI script changes');
    const helperHead = git('rev-parse', 'HEAD');
    const helper = classify(helperHead, {PR_BASE: partialHead});
    assert.equal(helper.status, 0, helper.stderr);
    assert.equal(helper.output, flags(true, true, true, true), 'a pull request that changes scripts/ci must enable every route');
    assert.match(helper.stdout, /changes scripts\/ci/);
    // The base classifier's output is forwarded only as one true/false value
    // per route: a repeated, empty, non-boolean or multiline value enables every
    // route, and other lines are not forwarded.
    const baseOutput = (body, label) => {
      fs.writeFileSync(path.join(dir, 'scripts/ci/classify-changes.sh'),
        `#!/usr/bin/env bash\nprintf ${JSON.stringify(body)} >> "\${GITHUB_OUTPUT}"\n`);
      git('add', '--', 'scripts/ci/classify-changes.sh');
      git('commit', '-qm', `classifier: ${label}`);
      const classifierBase = git('rev-parse', 'HEAD');
      fs.writeFileSync(path.join(dir, 'docs/guide.md'), `fixture ${label}\n`);
      git('add', '--', 'docs/guide.md');
      git('commit', '-qm', `docs change: ${label}`);
      return classify(git('rev-parse', 'HEAD'), {PR_BASE: classifierBase});
    };
    for (const [label, body] of [
      ['non-boolean value', 'go=yes\\nwebsite=false\\ndocs=true\\nci=false\\n'],
      ['repeated key', 'go=false\\nwebsite=false\\ndocs=true\\nci=false\\ngo=true\\n'],
      ['empty value', 'go=false\\nwebsite=\\ndocs=true\\nci=false\\n'],
      ['multiline value', 'go<<EOF\\nfalse\\nEOF\\nwebsite=false\\ndocs=true\\nci=false\\n'],
    ]) {
      const result = baseOutput(body, label);
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.output, flags(true, true, true, true), `a base classifier with a ${label} must enable every route`);
      assert.match(result.stdout, /did not set exactly one true\/false value for: /);
    }
    const extra = baseOutput('go=false\\nwebsite=false\\ndocs=true\\nci=false\\nfull=true\\n', 'extra key');
    assert.equal(extra.status, 0, extra.stderr);
    assert.equal(extra.output, flags(false, false, true, false), 'only the route values may be forwarded');
    // The trusted classifier and its output are staged in fresh mktemp files,
    // never in a fixed RUNNER_TEMP path.
    const step = script(ci, 'Classify changed files');
    assert.match(step, /classifier=\$\(mktemp "\$\{RUNNER_TEMP\}\/classify-changes\.XXXXXX"\)/);
    assert.doesNotMatch(step, /"\$\{RUNNER_TEMP\}\/classify-changes\.sh"/);
    // A base entry that is a symlink is not a regular file: it must take the
    // fallback instead of being staged and executed as text.
    fs.rmSync(path.join(dir, 'scripts/ci/classify-changes.sh'));
    fs.symlinkSync('../classify-target.sh', path.join(dir, 'scripts/ci/classify-changes.sh'));
    git('add', '--', 'scripts/ci/classify-changes.sh');
    git('commit', '-qm', 'classifier becomes a symlink');
    const linked = git('rev-parse', 'HEAD');
    fs.mkdirSync(path.join(dir, 'website'), {recursive: true});
    fs.writeFileSync(path.join(dir, 'website/page.md'), 'fixture\n');
    git('add', '--', 'website/page.md');
    git('commit', '-qm', 'website change');
    const linkedHead = git('rev-parse', 'HEAD');
    const linkedFallback = classify(linkedHead, {PR_BASE: linked});
    assert.equal(linkedFallback.status, 0, linkedFallback.stderr);
    assert.equal(linkedFallback.output, flags(true, true, true, true), 'a non-regular base classifier must enable every route');
    // A directory at the classifier path is a tree, not a regular file, even
    // when it holds a regular file: it takes the fallback too.
    fs.rmSync(path.join(dir, 'scripts/ci/classify-changes.sh'));
    fs.mkdirSync(path.join(dir, 'scripts/ci/classify-changes.sh'));
    fs.writeFileSync(path.join(dir, 'scripts/ci/classify-changes.sh/run.sh'), '#!/usr/bin/env bash\nexit 1\n');
    git('add', '-A', '--', 'scripts/ci');
    git('commit', '-qm', 'classifier path becomes a directory');
    const tree = git('rev-parse', 'HEAD');
    fs.writeFileSync(path.join(dir, 'website/page.md'), 'fixture 2\n');
    git('add', '--', 'website/page.md');
    git('commit', '-qm', 'website change under a directory');
    const treeFallback = classify(git('rev-parse', 'HEAD'), {PR_BASE: tree});
    assert.equal(treeFallback.status, 0, treeFallback.stderr);
    assert.equal(treeFallback.output, flags(true, true, true, true), 'a directory at the classifier path must enable every route');
    assert.match(treeFallback.stdout, /No trusted regular-file classifier/);
  } finally {
    fs.rmSync(dir, {recursive: true, force: true});
    fs.rmSync(runnerTemp, {recursive: true, force: true});
  }
});
test('the classifier fallback and output contract cover every route the workflow reads', () => {
  const step = script(ci, 'Classify changed files');
  const declared = step.match(/^route_keys="([^"]+)"$/m)[1].split(' ').sort();
  // Routes come from the scope step; needs.changes.outputs.full follows labels, not paths.
  // Consumers read them in dotted or bracket form in job conditions, and the summary's
  // Python reads them as outputs['<route>'] or from a tuple of names iterated with outputs[key].
  const summary = script(ci, 'Verify the selected CI path');
  const read = [...new Set([
    ...[...ci.matchAll(/\b(?:steps\.scope|needs\.changes)\.outputs(?:\.([A-Za-z_][\w-]*)|\[\s*['"]([A-Za-z_][\w-]*)['"]\s*\])/g)].map(match => match[1] || match[2]),
    ...[...summary.matchAll(/\boutputs\[\s*['"]([A-Za-z_][\w-]*)['"]\s*\]/g)].map(match => match[1]),
    ...[...summary.matchAll(/\bfor\s+\w+\s+in\s+\(([^)]*)\)/g)].flatMap(match => [...match[1].matchAll(/['"]([A-Za-z_][\w-]*)['"]/g)].map(name => name[1])),
  ])].filter(key => key !== 'full').sort();
  assert.deepEqual(declared, read, 'route_keys must list exactly the routes that jobs and the summary read');
  // Every job gated on a route output must be one the summary waits for and requires,
  // or it could fail while the stable CI check stays green.
  const needs = ci.match(/^  summary:\n[\s\S]*?^    needs: \[([^\]]+)\]/m)[1].split(',').map(name => name.trim());
  const jobs = ci.split(/^jobs:\n/m)[1].split(/^(?=  [a-z][a-z0-9_-]*:\n)/m).map(block => block.match(/^  ([a-z][a-z0-9_-]*):\n/)?.[1] && block)
    .filter(Boolean);
  // A job's condition: its `if:` line plus any continuation lines indented below it.
  const condition = block => block.match(/^    if:([^\n]*(?:\n {6,}[^\n]*)*)/m)?.[1] || '';
  const gated = jobs.filter(block => /needs\.changes\.outputs/.test(condition(block)))
    .map(block => block.match(/^  ([a-z][a-z0-9_-]*):/)[1]);
  assert.ok(gated.length > 0, 'expected route-gated jobs');
  // The summary's required list: its initial assignment (both branches of the
  // conditional) and every required.append(...).
  const assignment = summary.match(/^required = ([^\n]+)$/m);
  assert.ok(assignment, 'the summary must build a required list');
  const required = new Set([
    ...[...assignment[1].matchAll(/['"]([A-Za-z_][\w-]*)['"]/g)].map(match => match[1]),
    ...[...summary.matchAll(/\brequired\.append\(\s*['"]([A-Za-z_][\w-]*)['"]\s*\)/g)].map(match => match[1]),
  ]);
  for (const job of gated) {
    assert.ok(needs.includes(job), `summary must wait for ${job}`);
    assert.ok(required.has(job), `the summary's required list must include ${job}`);
  }
  // Both the fallback and the contract check iterate the list instead of keeping their own copies.
  assert.match(step, /^route_all\(\) \{ local key; for key in \$\{route_keys\}; do echo "\$\{key\}=true"/m);
  assert.match(step, /for key in \$\{route_keys\}; do\n\s+count=\$\(grep -c "\^\$\{key\}=" "\$\{file\}"/);
  // Every event's classifier output goes through the contract, pushes included.
  assert.equal([...step.matchAll(/^\s*GITHUB_OUTPUT="\$\{routes\}" bash [^\n]+\n\s*forward_routes "\$\{routes\}"$/gm)].length, 2);
  assert.doesNotMatch(step, /^\s*bash scripts\/ci\/classify-changes\.sh\s*$/m, 'no classifier may write the step output directly');
  assert.doesNotMatch(step, /cat "\$\{(?:routes|file)\}"/, 'the classifier output must not be forwarded wholesale');
  assert.doesNotMatch(step, /(?:go|website|docs|ci)=true/);
});
test('CI discovers every script suite, isolates Docker config and propagates failures from every invocation', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-script-suites-'));
  try {
    fs.mkdirSync(path.join(dir, 'bin'));
    const temporary = path.join(dir, 'temp dir');
    fs.mkdirSync(temporary);
    fs.mkdirSync(path.join(dir, 'scripts/tests'), {recursive: true});
    // This deletion canary is intentionally coupled to the current layout:
    // update it in the same change as any suite rename, split or consolidation.
    const requiredSuites = [
      'test_ci_scope.cjs',
      'test_ci_route_gate.cjs',
      'test_go_ci_worker.cjs',
      'test_pages_workflow.cjs',
      'test_release_workflow.cjs',
      'test_reviewbot_automerge.cjs',
      'test_reviewbot_commands.cjs',
      'test_reviewbot_handoff.cjs',
      'test_runner_routing.cjs',
      'test_workflow_python_isolation.cjs',
    ];
    for (const name of requiredSuites) {
      assert.ok(fs.existsSync(path.join(root, 'scripts/tests', name)), `missing CI suite: ${name}`);
    }
    // Include new filenames to catch a return to an explicit subset of suites,
    // including a name whose glob collation can differ from JavaScript sort().
    const suites = [...fs.readdirSync(path.join(root, 'scripts/tests')).filter(name => name.endsWith('.cjs')), 'test_future.cjs', 'test_ReviewBot.cjs'].sort();
    const globSuites = suites.filter(name => name !== 'test_ci_scope.cjs');
    for (const suite of suites) fs.writeFileSync(path.join(dir, 'scripts/tests', suite), '');
    fs.writeFileSync(path.join(dir, 'scripts/tests/test_future.py'), '');
    const record = path.join(dir, 'calls');
    const counters = ['node', 'python3'].map(tool => `${record}.${tool}.count`);
    for (const tool of ['node', 'python3']) {
      // FAIL_AT selects the one invocation that exits 7, so a failing bulk
      // `node --test` run can be distinguished from a failing first run.
      fs.writeFileSync(path.join(dir, 'bin', tool), `#!${process.execPath}
const fs = require('node:fs');
const config = process.env.DOCKER_CONFIG;
fs.appendFileSync(process.env.RECORD, JSON.stringify({tool: ${JSON.stringify(tool)}, args: process.argv.slice(2), config, files: fs.readdirSync(config)}) + '\\n');
const counter = process.env.RECORD + '.${tool}.count';
let invocation = 0;
try { invocation = Number(fs.readFileSync(counter, 'utf8')); } catch {}
invocation += 1;
fs.writeFileSync(counter, String(invocation));
process.exit(process.env.FAIL_TOOL === ${JSON.stringify(tool)} && invocation === Number(process.env.FAIL_AT || 0) ? 7 : 0);
`);
      fs.chmodSync(path.join(dir, 'bin', tool), 0o755);
    }
    const block = job('changes');
    assert.match(block, /name: Test CI scripts\n        if: steps.scope.outputs.ci == 'true'/);
    const setup = block.indexOf('Setup Node for CI script tests');
    const run = block.indexOf('Test CI scripts');
    assert.notEqual(setup, -1, 'Node setup step missing');
    assert.notEqual(run, -1, 'CI script test step missing');
    assert.ok(setup < run, 'Node setup must precede CI script tests');
    assert.match(block.slice(setup, run), /node-version: '22'/);
    const reset = () => {
      fs.writeFileSync(record, '');
      for (const counter of counters) fs.rmSync(counter, {force: true});
    };
    const execute = (overrides = {}) => spawnSync('bash', ['-c', script(ci, 'Test CI scripts')], {cwd: dir, encoding: 'utf8', env: {...process.env,
      PATH: path.join(dir, 'bin') + path.delimiter + process.env.PATH, RECORD: record, FAIL_TOOL: '', FAIL_AT: '',
      TMPDIR: temporary, DOCKER_CONFIG: '/unused-host-config', ...overrides}});
    const calls = () => {
      const log = fs.readFileSync(record, 'utf8').trim();
      return log ? log.split('\n').map(JSON.parse) : [];
    };
    const verifyCalls = (recorded, expected) => {
      assert.equal(recorded.length, expected.length);
      for (const [index, invocation] of expected.entries()) {
        const call = recorded[index];
        if (invocation === 'inventory') {
          assert.equal(call.tool, 'node');
          assert.deepEqual(call.args, ['--test', 'scripts/tests/test_ci_scope.cjs']);
        } else if (invocation === 'bulk') {
          assert.equal(call.tool, 'node');
          assert.equal(call.args[0], '--test');
          const expectedSuites = globSuites.map(name => `scripts/tests/${name}`);
          assert.deepEqual([...call.args.slice(1)].sort(), [...expectedSuites].sort());
        } else {
          assert.equal(call.tool, 'python3');
          assert.deepEqual(call.args, ['-I', '-m', 'unittest', 'discover', '-s', 'scripts/tests', '-p', 'test_*.py']);
        }
      }
      for (const call of recorded) {
        assert.notEqual(call.config, '/unused-host-config');
        assert.equal(path.dirname(call.config), temporary);
        assert.match(path.basename(call.config), /^speedle-docker-config\.[A-Za-z0-9]+$/);
        assert.deepEqual(call.files, []);
        assert.equal(fs.existsSync(call.config), false, 'temporary Docker config must be cleaned on success and failure');
      }
    };
    const steps = [
      {label: 'all invocations succeed', env: {}, status: 0, expected: ['inventory', 'bulk', 'python']},
      {label: 'the inventory invocation fails', env: {FAIL_TOOL: 'node', FAIL_AT: '1'}, status: 7, expected: ['inventory']},
      {label: 'the bulk invocation fails', env: {FAIL_TOOL: 'node', FAIL_AT: '2'}, status: 7, expected: ['inventory', 'bulk']},
      {label: 'the python discovery invocation fails', env: {FAIL_TOOL: 'python3', FAIL_AT: '1'}, status: 7, expected: ['inventory', 'bulk', 'python']},
    ];
    for (const {label, env, status, expected} of steps) {
      reset();
      const result = execute(env);
      assert.equal(result.status, status, `${label}\n${result.stderr}`);
      verifyCalls(calls(), expected);
    }
    for (const suite of suites) fs.rmSync(path.join(dir, 'scripts/tests', suite), {force: true});
    // The inventory suite still runs when it is the only discovered suite;
    // the empty bulk invocation is skipped rather than treated as no discovery.
    fs.writeFileSync(path.join(dir, 'scripts/tests/test_ci_scope.cjs'), '');
    reset();
    const inventoryOnly = execute();
    assert.equal(inventoryOnly.status, 0, inventoryOnly.stderr);
    verifyCalls(calls(), ['inventory', 'python']);

    // A completely empty glob must fail before either runner is invoked,
    // rather than letting Node report a missing inventory file instead.
    fs.rmSync(path.join(dir, 'scripts/tests/test_ci_scope.cjs'));
    reset();
    const empty = execute();
    assert.equal(empty.status, 1, empty.stderr);
    assert.match(empty.stderr, /no CI test suites discovered/);
    verifyCalls(calls(), []);

    // Another suite must not hide a missing inventory guard. Its dedicated
    // diagnostic is emitted before either runner is invoked.
    fs.writeFileSync(path.join(dir, 'scripts/tests/test_future.cjs'), '');
    reset();
    const missingInventory = execute();
    assert.equal(missingInventory.status, 1, missingInventory.stderr);
    assert.match(missingInventory.stderr, /CI scope guard suite scripts\/tests\/test_ci_scope\.cjs is missing/);
    verifyCalls(calls(), []);
  } finally {fs.rmSync(dir, {recursive: true, force: true});}
});
test('actual summary blocks failed/cancelled/missing selected jobs and allows intentional skips', () => {
  for (const full of [false, true]) {
    for (const content of [false, true]) {
      const jobs = Object.fromEntries(['changes', 'quick', 'lint', 'test', 'build'].map(name => [name, {result: 'skipped'}]));
      jobs.changes = {result: 'success', outputs: {full: String(full), go: 'true', docs: String(content), website: 'false'}};
      const selected = full ? ['lint', 'test', 'build'] : ['quick'];
      if (!full && content) selected.push('build');
      for (const name of selected) jobs[name].result = 'success';
      const run = () => spawnSync('bash', ['-c', script(ci, 'Verify the selected CI path')], {encoding: 'utf8', env: {...process.env, RESULTS: JSON.stringify(jobs)}});
      assert.equal(run().status, 0);
      for (const name of selected) {
        for (const result of ['failure', 'cancelled', 'skipped']) {
          jobs[name].result = result;
          assert.notEqual(run().status, 0, `${name}/${result}`);
        }
        jobs[name].result = 'success';
      }
      jobs.changes.result = 'failure';
      assert.notEqual(run().status, 0);
    }
  }
});
test('actual package selector includes nested fixtures/deletions and module-wide changes', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cxmq-selection-'));
  try {
    fs.mkdirSync(path.join(dir, 'bin'));
    for (const tool of ['git', 'go']) {
      fs.writeFileSync(path.join(dir, 'bin', tool), `#!${process.execPath}\nconst p=process.env;process.stdout.write(${JSON.stringify(tool)}==='git'?p.CHANGED.split('|').join('\\0'):p.PACKAGES.split('|').filter(d=>process.argv.slice(2).includes('./'+d.split('/')[0]+'/...')).map(d=>p.FIXTURE_ROOT+'/'+d).join('\\n')+'\\n');\n`);
      fs.chmodSync(path.join(dir, 'bin', tool), 0o755);
    }
    const execute = (changed, packages) => {
      const result = spawnSync('bash', ['-c', script(worker, 'Select changed packages')], {cwd: dir, encoding: 'utf8', env: {...process.env,
        PR_BASE: 'base', GITHUB_SHA: 'head', FIXTURE_ROOT: dir, CHANGED: changed.join('|'), PACKAGES: packages.join('|'), PATH: path.join(dir, 'bin') + path.delimiter + process.env.PATH}});
      assert.equal(result.status, 0, result.stderr);
      return fs.readFileSync(path.join(dir, 'quick-packages.txt'), 'utf8').trim().split('\n').filter(Boolean);
    };
    const packages = ['pkg/a', 'pkg/a/sub', 'cmd/b', 'deployment/container/dispatch'];
    const unusual = 'pkg/a/file\n::warning::spoof.go';
    fs.mkdirSync(path.join(dir, 'pkg/a'), {recursive: true});
    fs.writeFileSync(path.join(dir, unusual), 'package a\n');
    assert.deepEqual(execute([unusual], packages), ['./pkg/a']);
    assert.equal(fs.readFileSync(path.join(dir, 'quick-go-files.txt'), 'utf8'), unusual + '\0');
    assert.deepEqual(execute(['deployment/container/dispatch/main.go'], packages), ['./deployment/container/dispatch']);
    assert.deepEqual(execute(['pkg/a/testdata/policy.json', 'pkg/a/sub/deleted.go'], packages), ['./pkg/a', './pkg/a/sub']);
    assert.deepEqual(execute(['pkg/a/sub/file.go'], packages), ['./pkg/a/sub']);
    assert.deepEqual(execute(['docs/change.md', '.github/workflows/ci.yml'], packages), []);
    for (const file of ['go.mod', 'go.sum', 'testutil/storetest/fixture.go', '3rdparty/example/x.go']) {
      assert.deepEqual(execute([file], packages), ['./cmd/b', './deployment/container/dispatch', './pkg/a', './pkg/a/sub']);
    }
  } finally { fs.rmSync(dir, {recursive: true, force: true}); }
});
test('quick runner compiles service-backed packages and tests file-store packages without coverage', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cxmq-quick-'));
  try {
    fs.writeFileSync(path.join(dir, 'quick-packages.txt'), './pkg/store/etcd\n./pkg/store/mongodb\n./pkg/eval\n');
    fs.writeFileSync(path.join(dir, 'go'), `#!${process.execPath}\nrequire('node:fs').appendFileSync(process.env.RECORD,JSON.stringify({args:process.argv.slice(2),store:process.env.STORE_TYPE})+'\\n');process.exit(Number(process.env.GO_EXIT||0));\n`);
    fs.chmodSync(path.join(dir, 'go'), 0o755);
    const env = {...process.env, RECORD: path.join(dir, 'calls.jsonl'), RUNNER_TEMP: dir, STORE_TYPE: 'file', PATH: dir + path.delimiter + process.env.PATH};
    const execute = () => spawnSync('bash', ['-c', script(worker, 'Test changed packages without integration services')], {cwd: dir, encoding: 'utf8', env});
    assert.equal(execute().status, 0);
    const records = fs.readFileSync(env.RECORD, 'utf8').trim().split('\n').map(JSON.parse);
    assert.deepEqual(records.map(r => r.args.includes('-c')), [true, true, false]);
    assert.ok(records.every(r => r.store === 'file' && !r.args.some(arg => arg.includes('cover'))));
    env.GO_EXIT = '7';
    assert.equal(execute().status, 7);
  } finally { fs.rmSync(dir, {recursive: true, force: true}); }
});

test('quick uses the protected worker and keeps its sole mode input', () => {
  assert.match(job('quick'), /uses: teramoby\/speedle-plus\/\.github\/workflows\/go-ci-worker\.yml@master/);
  assert.match(job('quick'), /mode: quick/);
  assert.doesNotMatch(job('quick'), /runs-on:|steps:/);
  assert.doesNotMatch(job('test'), /outputs\.(?:go|website|docs)/);
});
test('ignored label events cannot publish the stable CI check name', () => {
  const name = job('summary').match(/name: \$\{\{ (.*) \}\}/)[1];
  const failed = context(); failed.needs.changes.result = 'failure';
  assert.equal(evaluate(name, failed), 'CI');
  for (const action of ['labeled', 'unlabeled']) {
    for (const label of ['other', 'full-ci', 'merge-head']) {
      const c = context('pull_request', [], action, label);
      const ignored = action === 'unlabeled' || label === 'other';
      assert.equal(evaluate(name, c) === 'CI', !ignored);
      if (ignored) assert.match(evaluate(name, c), /^Ignored CI event /);
    }
  }
});
test('image smoke unions original paths with full labels and isolates ignored events', () => {
  const image = fs.readFileSync(path.join(root, '.github/workflows/image-smoke.yml'), 'utf8');
  const changes = image.split('  changes:\n')[1].split('  smoke:\n')[0];
  const condition = changes.match(/    if: >-\n((?:      .*\n)+)/)[1].trim();
  const always = changes.match(/ALWAYS_SMOKE: \$\{\{ (.*) \}\}/)[1];
  const group = image.match(/  group: (.*)\n/)[1];
  const name = image.match(/    name: \$\{\{ (.*) \}\}/)[1];
  const smokeCondition = image.split('  smoke:\n')[1].match(/    if: (.*)/)[1];
  const selectedGroup = c => group.replace(/\$\{\{ (.*?) \}\}/g, (_, expr) => evaluate(expr, c));
  assert.match(image, /push:\n    branches: \[master\]/);
  assert.doesNotMatch(image, /paths:/); // Classify paths inside the workflow to preserve label events.
  assert.match(image, /types: \[opened, synchronize, reopened, labeled\]/);
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cxmq-image-'));
  try {
    fs.writeFileSync(path.join(dir, 'git'), `#!${process.execPath}\nprocess.stdout.write(process.env.CHANGED.split('|').join('\\0'));\n`);
    fs.chmodSync(path.join(dir, 'git'), 0o755);
    const select = (paths, force = false) => {
      const output = path.join(dir, 'output'); fs.writeFileSync(output, '');
      const result = spawnSync('bash', ['-c', script(image, 'Select image smoke')], {cwd: dir, encoding: 'utf8', env: {...process.env,
        PATH: dir + path.delimiter + process.env.PATH, CHANGED: paths.join('|'), ALWAYS_SMOKE: String(force), PR_BASE: 'base', GITHUB_SHA: 'head', GITHUB_OUTPUT: output}});
      assert.equal(result.status, 0, result.stderr);
      return fs.readFileSync(output, 'utf8').trim() === 'smoke=true';
    };
    for (const file of ['deployment/container/Dockerfile', 'deployment/container/dispatch/main.go', 'deployment/helm/speedle/Chart.yaml', 'cmd/spctl/main.go',
      '.github/workflows/release.yml', '.github/workflows/image-smoke.yml', 'scripts/release-smoke.sh']) assert.equal(select([file]), true, file);
    for (const files of [[], ['deployment/helm/speedle/templates/deployment.yaml'], ['deployment/helm/render-check.sh'], ['docs/page.md'], ['pkg/eval/eval.go'], ['cmd-like/main.go'], ['scripts/release-smoke.sh.bak']]) {
      assert.equal(select(files), false, files.join(','));
      assert.equal(select(files, true), true);
    }
    for (const labels of [[], ['full-ci'], ['merge-head'], ['other'], ['full-ci', 'merge-head']]) {
      for (const action of ['opened', 'synchronize', 'reopened', 'labeled', 'unlabeled']) {
        for (const label of ['other', 'full-ci', 'merge-head']) {
          const c = context('pull_request', labels, action, label);
          const ignored = action === 'unlabeled' || (action === 'labeled' && label === 'other');
          assert.equal(evaluate(condition, c), !ignored);
          assert.equal(evaluate(always, c), labels.some(x => ['full-ci', 'merge-head'].includes(x)));
          assert.equal(selectedGroup(c) === selectedGroup(context()), !ignored);
          for (const pathChanged of [false, true]) {
            const selected = !ignored && (pathChanged || evaluate(always, c));
            c.needs.changes.outputs.smoke = String(selected);
            assert.equal(evaluate(smokeCondition, c), selected);
            assert.equal(evaluate(name, c) === 'image smoke', selected);
          }
        }
      }
    }
    const push = context('push'); delete push.github.event.pull_request;
    assert.equal(evaluate(always, push), true);
  } finally { fs.rmSync(dir, {recursive: true, force: true}); }
});

test('content changes retain the build worker on quick and full routes', () => {
  for (const full of [false, true]) {
    for (const go of [false, true]) {
      for (const docs of [false, true]) {
        for (const website of [false, true]) {
          const c = context();
          c.needs.changes.outputs = Object.fromEntries(Object.entries({full, go, docs, website}).map(([k,v]) => [k, String(v)]));
          assert.equal(evaluate(condition('build'), c), (full && go) || docs || website);
          assert.equal(evaluate(condition('quick'), c), !full);
          assert.equal(evaluate(condition('test'), c), full);
        }
      }
    }
  }
});

test('quick formatting rejects parse errors, command failures and unformatted files', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-quick-gofmt-'));
  try {
    fs.writeFileSync(path.join(dir, 'quick-go-files.txt'), 'changed.go\0');
    fs.writeFileSync(path.join(dir, 'gofmt'), `#!${process.execPath}\nprocess.stdout.write(process.env.FORMAT_OUTPUT);process.stderr.write(process.env.FORMAT_ERROR||'');process.exit(Number(process.env.FORMAT_EXIT));\n`);
    fs.chmodSync(path.join(dir, 'gofmt'), 0o755);
    for (const [FORMAT_EXIT, FORMAT_OUTPUT, allowed] of [['0', '', true], ['0', 'changed.go', false], ['2', '', false]]) {
      const result = spawnSync('/bin/bash', ['-c', script(worker, 'Check changed Go formatting')], {
        cwd: dir, encoding: 'utf8', env: {...process.env, FORMAT_EXIT, FORMAT_OUTPUT, PATH: dir + path.delimiter + process.env.PATH},
      });
      assert.equal(result.status === 0, allowed, JSON.stringify([FORMAT_EXIT, FORMAT_OUTPUT, result]));
    }
    fs.writeFileSync(path.join(dir, 'quick-go-files.txt'), 'changed\n::warning::spoof.go\0');
    for (const FORMAT_EXIT of ['0', '2']) {
      const result = spawnSync('/bin/bash', ['-c', script(worker, 'Check changed Go formatting')], {
        cwd: dir, encoding: 'utf8', env: {...process.env, FORMAT_EXIT, FORMAT_OUTPUT: 'unformatted', FORMAT_ERROR: 'changed\n::warning::spoof.go: parse error\n', PATH: dir + path.delimiter + process.env.PATH},
      });
      assert.notEqual(result.status, 0);
      assert.doesNotMatch(result.stdout + result.stderr, /^::/m);
      assert.match(result.stderr, /spoof\.go/);
    }
  } finally {fs.rmSync(dir, {recursive: true, force: true});}
});

// The chart integration job consumes these candidate-image inputs, including
// transitive packages, rather than an already-published image.
test('Helm pull requests and pushes select every candidate image input', () => {
  const helm = fs.readFileSync(path.join(root, '.github/workflows/helm.yml'), 'utf8');
  for (const event of ['pull_request', 'push']) {
    const block = helm.split(`  ${event}:\n`)[1].split(/\n  [a-z_]+:|\npermissions:/)[0];
    const paths = [...block.matchAll(/^      - (.+)$/gm)].map(match => match[1]);
    for (const input of ['deployment/helm/**', 'deployment/container/**', 'cmd/**', 'pkg/**', 'api/**', '3rdparty/**', 'go.mod', 'go.sum']) {
      assert.ok(paths.includes(input), `${event} must select ${input}`);
    }
  }
});
