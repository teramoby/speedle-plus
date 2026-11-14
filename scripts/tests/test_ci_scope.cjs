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
test('actual summary blocks failed/cancelled/missing selected jobs and allows intentional skips', () => {
  for (const full of [false, true]) {
    const jobs = Object.fromEntries(['changes', 'quick', 'lint', 'test', 'build'].map(name => [name, {result: 'skipped'}]));
    jobs.changes = {result: 'success', outputs: {full: String(full), go: 'true', docs: 'false', website: 'false'}};
    const selected = full ? ['lint', 'test', 'build'] : ['quick'];
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
});
test('actual package selector includes nested fixtures/deletions and module-wide changes', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cxmq-selection-'));
  try {
    fs.mkdirSync(path.join(dir, 'bin'));
    for (const tool of ['git', 'go']) {
      fs.writeFileSync(path.join(dir, 'bin', tool), `#!${process.execPath}\nconst p=process.env;process.stdout.write(${JSON.stringify(tool)}==='git'?p.CHANGED.split('|').join('\\0'):p.PACKAGES.split('|').map(d=>p.FIXTURE_ROOT+'/'+d).join('\\n')+'\\n');\n`);
      fs.chmodSync(path.join(dir, 'bin', tool), 0o755);
    }
    const execute = (changed, packages) => {
      const result = spawnSync('bash', ['-c', script(worker, 'Select changed packages')], {cwd: dir, encoding: 'utf8', env: {...process.env,
        PR_BASE: 'base', GITHUB_SHA: 'head', FIXTURE_ROOT: dir, CHANGED: changed.join('|'), PACKAGES: packages.join('|'), PATH: path.join(dir, 'bin') + path.delimiter + process.env.PATH}});
      assert.equal(result.status, 0, result.stderr);
      return fs.readFileSync(path.join(dir, 'quick-packages.txt'), 'utf8').trim().split('\n').filter(Boolean);
    };
    const packages = ['pkg/a', 'pkg/a/sub', 'cmd/b'];
    assert.deepEqual(execute(['pkg/a/testdata/policy.json', 'pkg/a/sub/deleted.go'], packages), ['./pkg/a', './pkg/a/sub']);
    assert.deepEqual(execute(['pkg/a/sub/file.go'], packages), ['./pkg/a/sub']);
    assert.deepEqual(execute(['docs/change.md', '.github/workflows/ci.yml'], packages), []);
    for (const file of ['go.mod', 'go.sum', 'testutil/storetest/fixture.go', '3rdparty/example/x.go']) {
      assert.deepEqual(execute([file], packages), ['./cmd/b', './pkg/a', './pkg/a/sub']);
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
test('image smoke runs for all full PRs and master pushes with isolated ignored events', () => {
  const image = fs.readFileSync(path.join(root, '.github/workflows/image-smoke.yml'), 'utf8');
  const condition = image.match(/    if: >-\n((?:      .*\n)+)/)[1].trim();
  const group = image.match(/  group: (.*)\n/)[1];
  const name = image.match(/    name: \$\{\{ (.*) \}\}/)[1];
  const selectedGroup = c => group.replace(/\$\{\{ (.*?) \}\}/g, (_, expr) => evaluate(expr, c));
  assert.match(image, /push:\n    branches: \[master\]/);
  assert.doesNotMatch(image, /paths:/);
  assert.match(image, /types: \[opened, synchronize, reopened, labeled\]/);
  for (const labels of [[], ['full-ci'], ['merge-head'], ['other'], ['full-ci', 'merge-head']]) {
    for (const action of ['opened', 'synchronize', 'reopened', 'labeled', 'unlabeled']) {
      for (const label of ['other', 'full-ci', 'merge-head']) {
        const c = context('pull_request', labels, action, label);
        const ignored = action === 'unlabeled' || (action === 'labeled' && label === 'other');
        assert.equal(evaluate(condition, c), !ignored && labels.some(x => ['full-ci', 'merge-head'].includes(x)));
        assert.equal(selectedGroup(c) === selectedGroup(context()), !ignored);
        assert.equal(evaluate(name, c) === 'image smoke', !ignored);
      }
    }
  }
  const push = context('push'); delete push.github.event.pull_request;
  assert.equal(evaluate(condition, push), true);
});
