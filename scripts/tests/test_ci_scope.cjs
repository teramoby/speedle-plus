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
test('change classification preserves raw Git paths and fails on diff errors', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-ci-paths-'));
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
    const base = git('rev-parse', 'HEAD');
    fs.mkdirSync(path.join(dir, 'scripts/ci'), {recursive: true});
    fs.symlinkSync(path.join(root, 'scripts/ci/classify-changes.sh'), path.join(dir, 'scripts/ci/classify-changes.sh'));
    const classify = (head, overrides = {}) => {
      const output = path.join(dir, 'output');
      fs.writeFileSync(output, '');
      const result = spawnSync('bash', ['-c', script(ci, 'Classify changed files')], {
        cwd: dir, encoding: 'utf8', env: {...process.env, EVENT_NAME: 'pull_request',
          PR_BASE: base, PUSH_BEFORE: base, GITHUB_SHA: head, GITHUB_OUTPUT: output, ...overrides},
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
    const failed = classify('missing-head');
    assert.notEqual(failed.status, 0);
    assert.equal(failed.output, '', 'diff errors must not publish successful outputs');
  } finally { fs.rmSync(dir, {recursive: true, force: true}); }
});
test('CI discovers every script suite, isolates Docker config and propagates failures', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-script-suites-'));
  try {
    fs.mkdirSync(path.join(dir, 'bin'));
    const temporary = path.join(dir, 'temp dir');
    fs.mkdirSync(temporary);
    fs.mkdirSync(path.join(dir, 'scripts/tests'), {recursive: true});
    // Include new filenames to catch a return to an explicit subset of suites.
    const suites = [...fs.readdirSync(path.join(root, 'scripts/tests')).filter(name => name.endsWith('.cjs')), 'test_future.cjs'].sort();
    for (const suite of suites) fs.writeFileSync(path.join(dir, 'scripts/tests', suite), '');
    fs.writeFileSync(path.join(dir, 'scripts/tests/test_future.py'), '');
    const record = path.join(dir, 'calls');
    for (const tool of ['node', 'python3']) {
      fs.writeFileSync(path.join(dir, 'bin', tool), `#!${process.execPath}
const fs = require('node:fs');
const config = process.env.DOCKER_CONFIG;
fs.appendFileSync(process.env.RECORD, JSON.stringify({tool: ${JSON.stringify(tool)}, args: process.argv.slice(2), config, files: fs.readdirSync(config)}) + '\\n');
process.exit(process.env.FAIL_TOOL === ${JSON.stringify(tool)} ? 7 : 0);
`);
      fs.chmodSync(path.join(dir, 'bin', tool), 0o755);
    }
    const block = job('changes');
    assert.match(block, /name: Test CI scripts\n        if: steps.scope.outputs.ci == 'true'/);
    assert.ok(block.indexOf('Setup Node for CI script tests') < block.indexOf('Test CI scripts'));
    for (const failure of ['', 'node', 'python3']) {
      fs.writeFileSync(record, '');
      const result = spawnSync('bash', ['-c', script(ci, 'Test CI scripts')], {cwd: dir, encoding: 'utf8', env: {...process.env,
        PATH: path.join(dir, 'bin') + path.delimiter + process.env.PATH, RECORD: record, FAIL_TOOL: failure,
        TMPDIR: temporary, DOCKER_CONFIG: '/unused-host-config'}});
      assert.equal(result.status, failure ? 7 : 0, result.stderr);
      const calls = fs.readFileSync(record, 'utf8').trim().split('\n').map(JSON.parse);
      assert.deepEqual(calls[0].args, ['--test', ...suites.map(name => `scripts/tests/${name}`)]);
      assert.equal(calls.length, failure === 'node' ? 1 : 2);
      if (calls.length === 2) assert.deepEqual(calls[1].args, ['-m', 'unittest', 'discover', '-s', 'scripts/tests', '-p', 'test_*.py']);
      for (const call of calls) {
        assert.notEqual(call.config, '/unused-host-config');
        assert.equal(path.dirname(call.config), temporary);
        assert.match(path.basename(call.config), /^speedle-docker-config\.[A-Za-z0-9]+$/);
        assert.deepEqual(call.files, []);
        assert.equal(fs.existsSync(call.config), false, 'temporary Docker config must be cleaned on success and failure');
      }
    }
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
