/* Exercise the protected reusable worker without GitHub, host services or real Go tests. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const test = require('node:test');
const vm = require('node:vm');
const source = fs.readFileSync(path.resolve(__dirname, '../../.github/workflows/go-ci-worker.yml'), 'utf8');
const expected = ['self-hosted', 'Linux', 'X64', 'charming-speedle'];
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
  assert.doesNotMatch(expression, /inputs/);
  const lookup = name => {
    let value = github;
    for (const key of name.split('.').slice(1)) {
      if (value == null || typeof value !== 'object' || !Object.hasOwn(value, key)) return '';
      value = value[key];
    }
    return value ?? '';
  };
  // Restrict evaluation to the actual predicate's string/boolean/fromJSON subset.
  const tokens = /\s+|'(?:[^']|'')*'|github(?:\.[A-Za-z_][\w-]*)+|fromJSON|true|false|null|==|!=|&&|\|\||[(),!]/gy;
  let translated = '', offset = 0;
  while (offset < expression.length) {
    tokens.lastIndex = offset; const match = tokens.exec(expression);
    assert.ok(match, `Unsupported predicate near ${expression.slice(offset)}`);
    const token = match[0]; offset = tokens.lastIndex;
    if (token.startsWith("'")) translated += JSON.stringify(token.slice(1, -1).replaceAll("''", "'"));
    else if (token.startsWith('github.')) translated += `lookup(${JSON.stringify(token)})`;
    else translated += token;
  }
  return JSON.parse(JSON.stringify(vm.runInNewContext(translated, {lookup, inputs, fromJSON: JSON.parse}, {timeout: 100})));
}
function context(eventName, head = repository, repo = repository) {
  return {repository: repo, event_name: eventName,
    ref: eventName === 'pull_request' ? 'refs/pull/7/merge' : 'refs/heads/feature',
    event: {sender: {type: 'User'}, pull_request: {head: {repo: {full_name: head}}, user: {type: 'User'}}}};
}
test('worker owns runner selection and exposes only mode/store inputs, never a caller checkout ref', () => {
  const inputBlock = source.split('\npermissions:', 1)[0];
  assert.match(inputBlock, /^  workflow_call:$/m);
  assert.deepEqual([...inputBlock.matchAll(/^  ([\w-]+):/gm)].map(x => x[1]), ['workflow_call']);
  assert.deepEqual([...source.split(/^jobs:\s*$/m)[1].matchAll(/^  ([\w-]+):\s*$/gm)].map(x => x[1]), ['execute']);
  assert.deepEqual([...inputBlock.matchAll(/^      ([\w-]+):$/gm)].map(x => x[1]), ['mode', 'store_type']);
  assert.match(source, /^permissions:\n  contents: read$/m);
  assert.match(step('Checkout code'), /ref: \$\{\{ github.sha \}\}/);
  assert.match(step('Checkout code'), /persist-credentials: false/);
  assert.match(step('Setup Go'), /uses: actions\/setup-go@[a-f0-9]{40}(?:\s|$)/);
  assert.match(step('Setup Go'), /go-version: '1\.26'/);
  assert.doesNotMatch(source, /secrets:|secrets\.inherit|\$\{\{\s*inputs\.(?:ref|sha|runner)/);
});
test('human pushes, dispatch and own PRs use the runner; trusted master merges permit bot senders', () => {
  const push = context('push'), dispatch = context('workflow_dispatch');
  push.event = {sender: {type: 'User'}}; dispatch.event = {sender: {type: 'User'}};
  const merged = context('push'); merged.ref = 'refs/heads/master'; merged.event = {sender: {type: 'Bot'}};
  for (const github of [push, dispatch, context('pull_request'), merged]) {
    assert.deepEqual(route(github, {mode: 'test', store_type: 'file'}), expected);
  }
});
test('bot PRs/branch pushes, forks, missing metadata and copied repositories stay hosted regardless of inputs', () => {
  const missing = context('pull_request'); missing.event.pull_request.head.repo = null;
  const botPR = context('pull_request'); botPR.event.pull_request.user = {login: 'dependabot[bot]', type: 'Bot'};
  const botSenderPR = context('pull_request'); botSenderPR.event.sender.type = 'Bot'; botSenderPR.event.pull_request.user.type = 'Bot';
  const unknownAuthor = context('pull_request'); delete unknownAuthor.event.pull_request.user;
  const botBranch = context('push'); botBranch.ref = 'refs/heads/dependabot/go'; botBranch.event = {sender: {type: 'Bot'}};
  const botTag = context('push'); botTag.ref = 'refs/tags/master'; botTag.event = {sender: {type: 'Bot'}};
  const falseMaster = context('push'); falseMaster.ref = 'refs/heads/master-copy'; falseMaster.event = {sender: {type: 'Bot'}};
  const missingSender = context('push'); missingSender.event = {};
  const foreignMaster = context('push', 'outsider/speedle-plus', 'outsider/speedle-plus');
  foreignMaster.ref = 'refs/heads/master'; foreignMaster.event = {sender: {type: 'Bot'}};
  const cases = [context('pull_request', 'outsider/speedle-plus'), missing,
    {repository, event_name: 'pull_request', event: {}}, {repository, event_name: 'pull_request', event: {pull_request: {}}},
    context('pull_request_target'), context('issue_comment'), context('unknown'),
    context('push', 'outsider/speedle-plus', 'outsider/speedle-plus'),
    context('workflow_dispatch', 'outsider/speedle-plus', 'outsider/speedle-plus'), context('push', undefined, ''),
    botPR, botSenderPR, unknownAuthor, botBranch, botTag, falseMaster, missingSender, foreignMaster];
  for (const github of cases) {
    for (const inputs of [{mode: 'build', store_type: 'etcd'}, {mode: 'self-hosted', store_type: 'charming-speedle', ref: 'master'}]) {
      assert.equal(route(github, inputs), 'ubuntu-latest');
    }
  }
});
test('task validation permits only exact enum values and treats shell metacharacters as data', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-worker-input-'));
  try {
    const marker = path.join(directory, 'injected');
    for (const mode of ['lint', 'test', 'build']) for (const store of ['file', 'etcd']) {
      const result = spawnSync('bash', ['-c', script('Validate CI task')], {encoding: 'utf8', env: {...process.env, CI_TASK: mode, TEST_STORE: store}});
      assert.equal(result.status, 0, result.stderr);
    }
    for (const [mode, store] of [['', 'file'], ['Lint', 'file'], ['test', 'mongo'],
      [`lint; touch ${marker}`, 'file'], [`$(touch ${marker})`, 'file'], ['test', `file\n$(touch ${marker})`]]) {
      const result = spawnSync('bash', ['-c', script('Validate CI task')], {encoding: 'utf8', env: {...process.env, CI_TASK: mode, TEST_STORE: store}});
      assert.notEqual(result.status, 0); assert.equal(fs.existsSync(marker), false);
    }
    assert.ok(source.indexOf('Validate CI task') < source.indexOf('Checkout code'));
    assert.doesNotMatch(script('Validate CI task'), /\$\{\{/);
  } finally {fs.rmSync(directory, {recursive: true, force: true});}
});
function testEnvironment(store) {
  const block = step('Run tests with coverage').split('        env:\n')[1]?.split('        run:', 1)[0];
  assert.ok(block, 'Missing sanitized test environment');
  const env = {};
  for (const line of block.trimEnd().split('\n')) {
    const match = line.match(/^          ([A-Z_]+): (.*)$/); assert.ok(match, `Unexpected environment entry ${line}`);
    env[match[1]] = match[2] === "''" ? '' : match[2].replace('${{ inputs.store_type }}', store);
  }
  return env;
}
const cleanKeys = ['SP_APP_NAME', 'PMS_ADMIN_TOKEN', 'ADS_ADMIN_TOKEN', 'CA_LOCATION', 'CERT_LOCATION', 'KEY_LOCATION'];
test('test environment neutralizes inherited CLI names, tokens, certificates and remote service endpoints', () => {
  for (const store of ['file', 'etcd']) {
    const env = testEnvironment(store);
    assert.equal(env.STORE_TYPE, store);
    for (const key of cleanKeys) assert.equal(env[key], '', key);
    assert.equal(env.PMS_ENDPOINT, 'http://127.0.0.1:6733'); assert.equal(env.ADS_ENDPOINT, 'http://127.0.0.1:6734');
  }
  assert.doesNotMatch(source, /^\s+(?:export )?(?:HOME|home|CODEX_HOME)=|^\s+(?:HOME|home|CODEX_HOME):/m);
});
function runGo({store = 'file', testExit = 0, listExit = 0} = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-worker-go-'));
  try {
    const calls = path.join(directory, 'calls.jsonl');
    const stub = `#!${process.execPath}\nconst fs=require('node:fs');const a=process.argv.slice(2);if(a[0]==='env'){console.log(process.env.FIXTURE_GOPATH);process.exit(0)}if(a[0]==='list'){console.log('example.invalid/fixture');process.exit(Number(process.env.FIXTURE_LIST_EXIT))}fs.appendFileSync(process.env.FIXTURE_CALLS,JSON.stringify({args:a,env:Object.fromEntries(${JSON.stringify([...cleanKeys, 'STORE_TYPE', 'PMS_ENDPOINT', 'ADS_ENDPOINT', 'GOPATH'])}.map(k=>[k,process.env[k]]))})+'\\n');process.exit(Number(process.env.FIXTURE_TEST_EXIT));\n`;
    fs.writeFileSync(path.join(directory, 'go'), stub); fs.chmodSync(path.join(directory, 'go'), 0o755);
    const inherited = Object.fromEntries(cleanKeys.map(key => [key, 'fixture-inherited-value']));
    const result = spawnSync('bash', ['-c', script('Run tests with coverage')], {encoding: 'utf8', env: {
      ...process.env, ...inherited, GOPATH: '/fixture-inherited-gopath',
      PMS_ENDPOINT: 'https://fixture.invalid/pms', ADS_ENDPOINT: 'https://fixture.invalid/ads', ...testEnvironment(store),
      PATH: `${directory}${path.delimiter}${process.env.PATH}`, FIXTURE_CALLS: calls,
      FIXTURE_GOPATH: directory, FIXTURE_LIST_EXIT: String(listExit), FIXTURE_TEST_EXIT: String(testExit),
    }});
    const records = fs.existsSync(calls) ? fs.readFileSync(calls, 'utf8').trim().split('\n').map(line => JSON.parse(line)) : [];
    return {result, records, directory};
  } finally {fs.rmSync(directory, {recursive: true, force: true});}
}
test('actual test script preserves Go exit status, serializes packages and passes only sanitized environment', () => {
  for (const store of ['file', 'etcd']) for (const testExit of [0, 23]) {
    const {result, records, directory} = runGo({store, testExit}); assert.equal(result.status, testExit, result.stderr);
    assert.equal(records.length, 1); assert.deepEqual(records[0].args.slice(0, 3), ['test', '-p', '1']);
    for (const key of cleanKeys) assert.equal(records[0].env[key], '', key);
    assert.equal(records[0].env.STORE_TYPE, store); assert.equal(records[0].env.GOPATH, directory);
    assert.equal(records[0].env.PMS_ENDPOINT, 'http://127.0.0.1:6733'); assert.equal(records[0].env.ADS_ENDPOINT, 'http://127.0.0.1:6734');
  }
});
test('package discovery failure cannot turn a partial package list into a green test run', () => {
  const {result, records} = runGo({listExit: 17});
  assert.notEqual(result.status, 0); assert.equal(records.length, 0);
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
  const selected = step('Ensure test services are not already running');
  assert.match(selected, /inputs.mode == 'test' && runner.environment == 'self-hosted'/);
  const body = script('Ensure test services are not already running').match(/^python3 - <<'PYTHON'\n([\s\S]+)\nPYTHON$/)?.[1];
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
