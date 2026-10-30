/* Exercise the protected reusable worker without GitHub, host services or real Go tests. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const test = require('node:test');
const realBash = spawnSync('bash', ['-c', 'command -v bash'], {encoding: 'utf8'}).stdout.trim();
const vm = require('node:vm');
const source = fs.readFileSync(path.resolve(__dirname, '../../.github/workflows/go-ci-worker.yml'), 'utf8');
const expected = ['self-hosted', 'Linux', 'X64', 'charming-speedle'];
const pool = ['self-hosted', 'Linux', 'X64', 'speedle-linux'];
const repository = 'teramoby/speedle-plus';
function step(name) {
  const parts = source.split(`      - name: ${name}\n`);
  assert.equal(parts.length, 2, `Missing or ambiguous step ${name}`);
  return parts[1].split(/^      - /m, 1)[0];
}
function script(name) {
  const block = step(name);
  const literal = block.split('        run: |\n')[1];
  assert.ok(literal, `Missing shell script ${name}`);
  const lines = [];
  for (const line of literal.split('\n')) {
    if (line && !line.startsWith('          ')) break;
    lines.push(line.slice(10));
  }
  return lines.join('\n').trimEnd();
}
function route(github, inputs) {
  const expression = source.match(/runs-on: >-\n\s+\$\{\{ ([^\n]+) \}\}/)?.[1];
  assert.ok(expression, 'Missing protected routing predicate');
  assert.doesNotMatch(expression.split(' && fromJSON(')[0], /inputs/);
  const lookup = name => {
    let value = {github, inputs};
    for (const key of name.split('.')) {
      if (value == null || typeof value !== 'object' || !Object.hasOwn(value, key)) return '';
      value = value[key];
    }
    return value ?? '';
  };
  // Restrict evaluation to the actual predicate's string/boolean/fromJSON subset.
  const tokens = /\s+|'(?:[^']|'')*'|github(?:\.[A-Za-z_][\w-]*)+|inputs\.mode|fromJSON|true|false|null|==|!=|&&|\|\||[(),!]/gy;
  let translated = '', offset = 0;
  while (offset < expression.length) {
    tokens.lastIndex = offset; const match = tokens.exec(expression);
    assert.ok(match, `Unsupported predicate near ${expression.slice(offset)}`);
    const token = match[0]; offset = tokens.lastIndex;
    if (token.startsWith("'")) translated += JSON.stringify(token.slice(1, -1).replaceAll("''", "'"));
    else if (token.startsWith('github.') || token === 'inputs.mode') translated += `lookup(${JSON.stringify(token)})`;
    else translated += token;
  }
  return JSON.parse(JSON.stringify(vm.runInNewContext(translated, {lookup, inputs, fromJSON: JSON.parse}, {timeout: 100})));
}
function context(eventName, head = repository, repo = repository) {
  return {repository: repo, event_name: eventName,
    ref: eventName === 'pull_request' ? 'refs/pull/7/merge' : 'refs/heads/feature',
    event: {sender: {type: 'User'}, pull_request: {head: {repo: {full_name: head}}, user: {type: 'User'}}}};
}
const testModes = ['test', 'nightly', 'weekly', 'regression'];
const allModes = ['lint', 'build', ...testModes];
test('worker owns runner selection and exposes only the mode input, never a caller checkout ref', () => {
  const inputBlock = source.split('\npermissions:', 1)[0];
  assert.match(inputBlock, /^  workflow_call:$/m);
  assert.deepEqual([...inputBlock.matchAll(/^  ([\w-]+):/gm)].map(x => x[1]), ['workflow_call']);
  assert.deepEqual([...source.split(/^jobs:\s*$/m)[1].matchAll(/^  ([\w-]+):\s*$/gm)].map(x => x[1]), ['execute']);
  assert.deepEqual([...inputBlock.matchAll(/^      ([\w-]+):$/gm)].map(x => x[1]), ['mode']);
  assert.match(source, /^permissions:\n  contents: read$/m);
  assert.match(step('Checkout code'), /ref: \$\{\{ github.sha \}\}/);
  assert.match(step('Checkout code'), /persist-credentials: false/);
  assert.match(step('Setup Go'), /uses: actions\/setup-go@[a-f0-9]{40}(?:\s|$)/);
  assert.match(step('Setup Go'), /go-version: '1\.26'/);
  assert.doesNotMatch(source, /secrets:|secrets\.inherit|\$\{\{\s*inputs\.(?:ref|sha|runner|store_type)/);
});
test('trusted events pool lint/build/nightly while test and weekly stay on the original runner', () => {
  const push = context('push'), dispatch = context('workflow_dispatch');
  push.event = {sender: {type: 'User'}}; dispatch.event = {sender: {type: 'User'}};
  const merged = context('push'); merged.ref = 'refs/heads/master'; merged.event = {sender: {type: 'Bot'}};
  const botPR = context('pull_request'); botPR.event.pull_request.user.type = 'Bot';
  const botPush = context('push'); botPush.event.sender.type = 'Bot';
  const botTag = context('push'); botTag.event.sender.type = 'Bot'; botTag.ref = 'refs/tags/release';
  const scheduled = context('schedule'); scheduled.ref = 'refs/heads/master'; scheduled.event = {};
  for (const github of [push, dispatch, context('pull_request'), merged, botPR, botPush, botTag, scheduled]) {
    for (const mode of allModes) assert.deepEqual(route(github, {mode}), ['test', 'weekly'].includes(mode) ? expected : pool);
  }
});
test('forks, missing metadata, copied repositories and off-master schedules stay hosted regardless of inputs', () => {
  const missing = context('pull_request'); missing.event.pull_request.head.repo = null;
  const forkBot = context('pull_request', 'outsider/speedle-plus'); forkBot.event.pull_request.user.type = 'Bot';
  const unknownAuthor = context('pull_request'); delete unknownAuthor.event.pull_request.user;
  const missingSender = context('push'); missingSender.event = {};
  const foreignMaster = context('push', 'outsider/speedle-plus', 'outsider/speedle-plus');
  foreignMaster.ref = 'refs/heads/master'; foreignMaster.event = {sender: {type: 'Bot'}};
  const branchSchedule = context('schedule'); branchSchedule.event = {};
  const foreignSchedule = context('schedule', 'outsider/speedle-plus', 'outsider/speedle-plus'); foreignSchedule.ref = 'refs/heads/master';
  const cases = [context('pull_request', 'outsider/speedle-plus'), missing,
    {repository, event_name: 'pull_request', event: {}}, {repository, event_name: 'pull_request', event: {pull_request: {}}},
    context('pull_request_target'), context('issue_comment'), context('unknown'),
    context('push', 'outsider/speedle-plus', 'outsider/speedle-plus'),
    context('workflow_dispatch', 'outsider/speedle-plus', 'outsider/speedle-plus'), context('push', undefined, ''),
    forkBot, unknownAuthor, missingSender, foreignMaster, branchSchedule, foreignSchedule];
  for (const github of cases) {
    for (const inputs of [...allModes.map(mode => ({mode})), {mode: 'self-hosted', ref: 'master'}]) {
      assert.equal(route(github, inputs), 'ubuntu-latest', JSON.stringify([github, inputs]));
    }
  }
});
test('identity check accepts both pool runners but keeps fixed-port test modes on runner 1', () => {
  for (const RUNNER_NAME of ['charming-speedle', 'charming-speedle-2', 'unexpected-host']) {
    for (const CI_TASK of allModes) {
      const result = spawnSync('bash', ['-c', script('Verify self-hosted runner identity')], {
        encoding: 'utf8', env: {...process.env, RUNNER_NAME, RUNNER_OS: 'Linux', RUNNER_ARCH: 'X64', CI_TASK},
      });
      const allowed = RUNNER_NAME === 'charming-speedle' || (RUNNER_NAME === 'charming-speedle-2' && !['test', 'weekly'].includes(CI_TASK));
      assert.equal(result.status === 0, allowed, `${RUNNER_NAME}/${CI_TASK}: ${result.stderr}`);
    }
  }
});
test('task validation permits only exact enum values and treats shell metacharacters as data', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-worker-input-'));
  try {
    const marker = path.join(directory, 'injected');
    for (const mode of allModes) {
      const result = spawnSync('bash', ['-c', script('Validate CI task')], {encoding: 'utf8', env: {...process.env, CI_TASK: mode}});
      assert.equal(result.status, 0, result.stderr);
    }
    for (const mode of ['', 'Lint', 'gate', 'nightly ', `lint; touch ${marker}`, `$(touch ${marker})`, `test\n$(touch ${marker})`]) {
      const result = spawnSync('bash', ['-c', script('Validate CI task')], {encoding: 'utf8', env: {...process.env, CI_TASK: mode}});
      assert.notEqual(result.status, 0); assert.equal(fs.existsSync(marker), false);
    }
    assert.ok(source.indexOf('Validate CI task') < source.indexOf('Checkout code'));
    assert.doesNotMatch(script('Validate CI task'), /\$\{\{/);
  } finally {fs.rmSync(directory, {recursive: true, force: true});}
});
function testEnvironment(mode) {
  const block = step('Run Go tests').split('        env:\n')[1]?.split('        run:', 1)[0];
  assert.ok(block, 'Missing sanitized test environment');
  const env = {};
  for (const line of block.trimEnd().split('\n')) {
    const match = line.match(/^          ([A-Z_]+): (.*)$/); assert.ok(match, `Unexpected environment entry ${line}`);
    env[match[1]] = match[2] === "''" ? '' : match[2].replace('${{ inputs.mode }}', mode);
  }
  return env;
}
const cleanKeys = ['SP_APP_NAME', 'PMS_ADMIN_TOKEN', 'ADS_ADMIN_TOKEN', 'CA_LOCATION', 'CERT_LOCATION', 'KEY_LOCATION'];
test('test environment neutralizes inherited CLI names, tokens, certificates and remote service endpoints', () => {
  for (const mode of testModes) {
    const env = testEnvironment(mode);
    assert.equal(env.CI_TASK, mode); assert.equal(env.RESULTS_DIR, 'test-results');
    for (const key of cleanKeys) assert.equal(env[key], '', key);
    assert.equal(env.PMS_ENDPOINT, 'http://127.0.0.1:6733'); assert.equal(env.ADS_ENDPOINT, 'http://127.0.0.1:6734');
  }
  assert.doesNotMatch(source, /^\s+(?:export )?(?:HOME|home|CODEX_HOME)=|^\s+(?:HOME|home|CODEX_HOME):/m);
  assert.match(step('Run Go tests'), /if: inputs.mode == 'test' \|\| inputs.mode == 'nightly' \|\| inputs.mode == 'weekly' \|\| inputs.mode == 'regression'$/m);
  assert.match(step('Upload scheduled test results'), /inputs.mode == 'regression'/);
});

// Run the worker step and scripts/ci/run-go-tests.sh against stub go,
// python3 and node that record their calls and exit with fixture statuses.
const repoRoot = path.resolve(__dirname, '../..');
const recorded = ['STORE_TYPE', 'SPEEDLE_CLI_COVERPROFILE', 'GOTOOLCHAIN', 'RESULTS_DIR', ...cleanKeys, 'PMS_ENDPOINT', 'ADS_ENDPOINT', 'GOPATH'];
function runSuite(mode, {listExit = 0, failMatching = '', mysqlOnly = false, docker = 'reachable'} = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-worker-go-'));
  try {
    const calls = path.join(directory, 'calls.jsonl');
    const stub = tool => `#!${process.execPath}
const fs=require('node:fs');const a=process.argv.slice(2);
if(${JSON.stringify(tool)}==='bash'&&!String(a[0]).startsWith('scripts/ci/validate-')){const r=require('node:child_process').spawnSync(process.env.FIXTURE_REAL_BASH,a,{stdio:'inherit'});process.exit(r.status===null?1:r.status)}
if(${JSON.stringify(tool)}==='go'&&a[0]==='env'){console.log(a[1]==='CC'?'true':a[1]==='GOVERSION'?'go1.26.10':process.env.FIXTURE_GOPATH);process.exit(0)}
if(${JSON.stringify(tool)}==='docker')process.exit(process.env.FIXTURE_DOCKER==='reachable'?0:1)
if(${JSON.stringify(tool)}==='id'){console.log(a[0]==='-un'?'runner':process.env.FIXTURE_DOCKER==='group'?'runner docker':'runner');process.exit(0)}
if(${JSON.stringify(tool)}==='go'&&a[0]==='list'){console.log('example.invalid/pkg/a\\nexample.invalid/pkg/store/mongodb\\nexample.invalid/samples/x\\nexample.invalid/pkg/svcs/adsgrpc/pb');process.exit(Number(process.env.FIXTURE_LIST_EXIT))}
fs.appendFileSync(process.env.FIXTURE_CALLS,JSON.stringify({tool:${JSON.stringify(tool)},args:a,env:Object.fromEntries(${JSON.stringify(recorded)}.map(k=>[k,process.env[k]]))})+'\\n');
const fail=process.env.FIXTURE_FAIL;process.exit(fail&&a.join(' ').includes(fail)?7:0);\n`;
    for (const tool of ['go', 'python3', 'node', 'flock', 'docker', 'id', 'sg', 'bash']) {
      fs.writeFileSync(path.join(directory, tool), stub(tool)); fs.chmodSync(path.join(directory, tool), 0o755);
    }
    const inherited = Object.fromEntries(cleanKeys.map(key => [key, 'fixture-inherited-value']));
    const work = path.join(directory, 'work'); fs.mkdirSync(work);
    const event = path.join(directory, 'event.json');
    fs.writeFileSync(event, JSON.stringify({inputs: {mysql_only: mysqlOnly}}));
    const result = spawnSync('bash', ['-c', script('Run Go tests')], {cwd: repoRoot, encoding: 'utf8', env: {
      ...process.env, ...inherited, GOPATH: '/fixture-inherited-gopath',
      PMS_ENDPOINT: 'https://fixture.invalid/pms', ADS_ENDPOINT: 'https://fixture.invalid/ads', ...testEnvironment(mode),
      RESULTS_DIR: path.join(work, 'results'),
      GITHUB_EVENT_NAME: mysqlOnly ? 'workflow_dispatch' : 'push', GITHUB_EVENT_PATH: event,
      PATH: `${directory}${path.delimiter}${process.env.PATH}`, FIXTURE_CALLS: calls,
      FIXTURE_GOPATH: directory, FIXTURE_LIST_EXIT: String(listExit), FIXTURE_FAIL: failMatching, FIXTURE_DOCKER: docker, FIXTURE_REAL_BASH: realBash,
    }});
    const records = fs.existsSync(calls) ? fs.readFileSync(calls, 'utf8').trim().split('\n').map(line => JSON.parse(line)).filter(r => !(r.tool === 'python3' && r.args[0] === '-')) : [];
    const checks = fs.existsSync(path.join(work, 'results', 'checks.txt')) ? fs.readFileSync(path.join(work, 'results', 'checks.txt'), 'utf8') : '';
    return {result, records, checks, directory};
  } finally {fs.rmSync(directory, {recursive: true, force: true}); fs.rmSync(path.join(repoRoot, 'coverage.out'), {force: true});}
}
const goTests = records => records.filter(r => r.tool === 'go' && r.args[0] === 'test');
test('gate runs every package once, only store-dependent packages again with etcd, then the coverage gate', () => {
  const {result, records} = runSuite('test'); assert.equal(result.status, 0, result.stderr);
  const [file, etcd] = goTests(records);
  assert.deepEqual(file.args.slice(0, 3), ['test', '-p', '1']);
  assert.equal(file.env.STORE_TYPE, 'file'); assert.ok(file.args.includes('example.invalid/pkg/a'));
  assert.ok(!file.args.some(a => a.includes('mongodb') || a.includes('samples') || a.endsWith('/pb')),
    'MongoDB, samples and generated protobuf packages are not gated');
  assert.equal(file.env.SPEEDLE_CLI_COVERPROFILE, path.join(repoRoot, 'coverage-cli.out'));
  assert.equal(etcd.env.SPEEDLE_CLI_COVERPROFILE, undefined);
  assert.equal(etcd.env.STORE_TYPE, 'etcd'); assert.deepEqual(etcd.args.slice(-2), ['./pkg/eval', './cmd/spctl/command']);
  assert.equal(goTests(records).length, 3);
  assert.deepEqual(goTests(records)[2].args, ['test', '-count=1', './website/tests/docscheck']);
  const gate = records.find(r => r.tool === 'python3');
  assert.deepEqual(gate.args.slice(0, 6),
    ['scripts/ci/coverage.py', 'coverage.out', '--merge', 'coverage-cli.out', '--min-file', 'scripts/ci/coverage-min.txt']);
  assert.deepEqual(records.filter(r => r.tool === 'python3').slice(1).map(r => r.args), [
    ['website/tests/check_quick_start.py', '--toolchain', 'go1.26.10'],
    ['website/tests/check_embedded.py', '--toolchain', 'go1.26.10', '--candidate'],
    ['website/tests/check_docs.py'],
  ]);
  for (const record of records) {
    for (const key of cleanKeys) assert.equal(record.env[key], '', key);
    assert.equal(record.env.PMS_ENDPOINT, 'http://127.0.0.1:6733'); assert.equal(record.env.ADS_ENDPOINT, 'http://127.0.0.1:6734');
  }
});
test('a failure in either executable guide blocks the merge-gating test job', () => {
  for (const guide of ['check_quick_start.py', 'check_embedded.py', 'check_docs.py', './website/tests/docscheck']) {
    const {result, records} = runSuite('test', {failMatching: guide});
    assert.equal(result.status, 7, guide);
    assert.ok(records.some(r => r.args.some(arg => arg.endsWith(guide))));
  }
});
test('gate preserves the failing status and stops before the coverage gate', () => {
  const {result, records} = runSuite('test', {failMatching: 'example.invalid/pkg/a'});
  assert.equal(result.status, 7); assert.equal(records.filter(r => r.tool === 'python3').length, 0);
});
test('package discovery failure cannot turn a partial package list into a green test run', () => {
  // nightly lists no packages: the shared regression engine tests ./...
  for (const mode of ['test', 'weekly']) {
    const {result, records} = runSuite(mode, {listExit: 17});
    assert.notEqual(result.status, 0, mode); assert.equal(goTests(records).length, 0, mode);
  }
});
test('nightly runs every suite even after a failure, records checks and fails at the end', () => {
  const {result, records, checks} = runSuite('nightly', {failMatching: 'go1.26.10'});
  assert.equal(result.status, 1);
  assert.equal(goTests(records).length, 0, 'every backend runs through the shared regression engine');
  const file = records.find(r => r.tool === 'python3' && r.args[0] === 'scripts/ci/run-store-tests.py');
  assert.equal(file.env.STORE_TYPE, 'file'); assert.ok(file.env.RESULTS_DIR.endsWith('results'));
  for (const launcher of ['scripts/ci/validate-etcd.sh', 'scripts/ci/validate-mongodb.sh']) {
    assert.ok(records.some(r => r.tool === 'bash' && r.args[0] === launcher), launcher);
  }
  for (const backend of ['file', 'etcd', 'mysql', 'mongodb']) assert.match(checks, new RegExp(`^${backend}-full=0:${backend}-full\\.log$`, 'm'));
  for (const toolchain of ['go1.26.10', 'go1.27.3']) assert.match(checks, new RegExp(`quick-start-${toolchain}=\\d+:`));
  assert.match(checks, /quick-start-go1\.26\.10=7:/); assert.match(checks, /quick-start-go1\.27\.3=0:/);
  assert.match(checks, /mysql-full=0:mysql-full\.log/);
  assert.match(checks, /^etcd-docs=0:etcd-docs\.log$/m);
  assert.match(checks, /^mongodb-docs=0:mongodb-docs\.log$/m);
  assert.match(checks, /govulncheck=0:govulncheck\.log/); assert.match(checks, /ci-script-tests=0:/);
});
test('post-merge regression runs every package with the file store and the guides, to the end', () => {
  const {result, records, checks} = runSuite('regression', {failMatching: 'check_embedded.py'});
  assert.equal(result.status, 1, 'a failed guide fails the run at the end');
  const tests = goTests(records);
  assert.equal(tests.length, 2);
  const [file, docs] = tests;
  assert.equal(file.env.STORE_TYPE, 'file');
  for (const flag of ['-race', '-json', '-p']) assert.ok(file.args.includes(flag), flag);
  assert.ok(file.args.includes('example.invalid/pkg/a'));
  assert.ok(!file.args.some(a => a.includes('mongodb') || a.includes('samples') || a.endsWith('/pb')));
  assert.deepEqual(docs.args, ['test', '-count=1', './website/tests/docscheck']);
  assert.ok(!records.some(r => r.env.STORE_TYPE === 'etcd'), 'no etcd run');
  assert.ok(!records.some(r => r.args.some(a => /mysql|govulncheck|staticcheck|coverage\.py/.test(a))));
  for (const name of ['quick-start=0', 'embedded=7', 'docs-contract=0', 'docs=0']) assert.match(checks, new RegExp(`^${name}:`, 'm'));
  assert.equal(runSuite('regression').result.status, 0);
});
test('manual MySQL trial runs the full database suite and CI checks without the other backends', () => {
  const {result, records, checks} = runSuite('nightly', {mysqlOnly: true, failMatching: 'run-mysql-tests.py'});
  assert.equal(result.status, 1);
  assert.equal(goTests(records).length, 0);
  assert.match(checks, /mysql-full=7:mysql-full\.log/);
  assert.match(checks, /ci-script-tests=0:ci-script-tests\.log/);
  assert.doesNotMatch(checks, /etcd-docs|mongodb-docs|file-full|etcd-full|mongodb-full/);
  assert.ok(records.some(r => r.args.includes('scripts/ci/run-mysql-tests.py')));
});
test('Docker guide checks use the account docker group the runner process lacks', () => {
  const guides = ['website/tests/check_etcd.py', 'website/tests/check_mongodb.py'];
  const direct = runSuite('nightly');
  assert.equal(direct.result.status, 0, direct.result.stderr);
  for (const guide of guides) assert.ok(direct.records.some(r => r.tool === 'python3' && r.args[0] === guide), guide);
  assert.ok(!direct.records.some(r => r.tool === 'sg'));
  const {result, records, checks} = runSuite('nightly', {docker: 'group'});
  assert.equal(result.status, 0, result.stderr);
  for (const guide of guides) {
    assert.ok(records.some(r => r.tool === 'sg' && r.args[0] === 'docker' && r.args[1] === '-c' && r.args[2] === `python3 ${guide} `), guide);
    assert.ok(!records.some(r => r.tool === 'python3' && r.args[0] === guide), guide);
  }
  assert.match(checks, /^etcd-docs=0:etcd-docs\.log$/m); assert.match(checks, /^mongodb-docs=0:mongodb-docs\.log$/m);
  const none = runSuite('nightly', {docker: 'none'});
  assert.ok(!none.records.some(r => r.tool === 'sg'), 'without the docker group, the guide runs and reports its own error');
});
test('weekly tests the other supported Go releases and runs staticcheck', () => {
  const {result, records, checks} = runSuite('weekly'); assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(goTests(records).map(r => r.env.GOTOOLCHAIN), ['go1.27.3']);
  assert.match(checks, /^staticcheck=0:staticcheck\.log$/m);
});

function listen(host = '127.0.0.1', handler) {
  const server = net.createServer(handler);
  return new Promise((resolve, reject) => {server.once('error', reject); server.listen({host, port: 0, ipv6Only: host === '::1'}, () => resolve(server));});
}
const close = server => new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
async function freePorts(excluded = []) {
  const servers = [];
  try {
    while (servers.length < 5) {
      const server = await listen();
      if (excluded.includes(server.address().port)) await close(server); else servers.push(server);
    }
    return servers.map(server => server.address().port);
  } finally {await Promise.all(servers.map(close));}
}
function guard(ports) {
  const selected = step('Run Go tests');
  assert.match(selected, /flock -w 7200 9/);
  const body = script('Run Go tests').match(/python3 - <<'PYTHON'\n([\s\S]+?)\nPYTHON/)?.[1];
  assert.ok(body); assert.equal((body.match(/for port in \(2379, 2380, 27017, 6733, 6734\):/g) || []).length, 1);
  assert.equal(new Set(ports).size, 5);
  for (const port of ports) assert.ok(![2379, 2380, 27017, 6733, 6734].includes(port));
  const remapped = body.replace('for port in (2379, 2380, 27017, 6733, 6734):', `for port in (${ports.join(', ')}):`);
  return spawnSync('python3', ['-c', remapped], {encoding: 'utf8'});
}
test('actual service guard accepts five free test-owned ephemeral ports', async () => {
  const result = guard(await freePorts()); assert.equal(result.status, 0, result.stderr);
});
for (const host of ['127.0.0.1', '::1']) {
  test(`actual service guard refuses each occupied port on ${host} without touching its listener`, async () => {
    for (let index = 0; index < 5; index++) {
      let connections = 0; const listener = await listen(host, socket => {connections++; socket.end();});
      try {
        const occupied = listener.address().port, ports = await freePorts([occupied]); ports[index] = occupied;
        const result = guard(ports); assert.notEqual(result.status, 0);
        assert.match(result.stderr, new RegExp(`Test service port ${occupied} is in use`));
        assert.equal(listener.listening, true); assert.equal(connections, 0);
      } finally {await close(listener);}
    }
  });
}
test('SO_REUSEADDR allows a recently closed test connection instead of mistaking TIME_WAIT for a service', async () => {
  const listener = await listen('127.0.0.1', socket => socket.end('done'));
  const port = listener.address().port;
  try {
    await new Promise((resolve, reject) => {
      const client = net.createConnection({host: '127.0.0.1', port});
      client.on('error', reject); client.resume(); client.on('close', resolve);
    });
  } finally {await close(listener);}
  const ports = await freePorts([port]); ports[2] = port;
  const result = guard(ports); assert.equal(result.status, 0, result.stderr);
});
