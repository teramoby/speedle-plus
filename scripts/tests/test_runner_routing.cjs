/* Offline workflow routing and shell regressions. Does not contact GitHub or Ark. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const test = require('node:test');
const vm = require('node:vm');

const root = path.resolve(__dirname, '../..');
const canonical = 'teramoby/speedle-plus';
const labels = ['self-hosted', 'Linux', 'X64', 'charming-speedle'];
const read = name => fs.readFileSync(path.join(root, '.github/workflows', name), 'utf8');
const sources = {
  ci: read('ci.yml'), review: read('reviewbot.yml'),
  commands: read('reviewbot-command.yml'), inline: read('reviewbot-inline.yml'),
};
function jobs(source) {
  const section = source.split(/^jobs:\s*$/m)[1];
  assert.ok(section, 'Missing jobs mapping');
  const matches = [...section.matchAll(/^  ([\w-]+):\s*$/gm)];
  return Object.fromEntries(matches.map((match, i) => [
    match[1], section.slice(match.index, matches[i+1]?.index ?? section.length),
  ]));
}
const job = Object.fromEntries(Object.entries(sources).map(([name, source]) => [name, jobs(source)]));
function value(block, key) {
  const lines = block.split('\n');
  const i = lines.findIndex(line => line.startsWith(`    ${key}:`));
  assert.notEqual(i, -1, `Missing job ${key}`);
  let result = lines[i].slice(`    ${key}:`.length).trim();
  if (result === '>-' || result === '>') {
    const parts = [];
    for (let j = i+1; j < lines.length && (!lines[j] || lines[j].startsWith('      ')); j++) parts.push(lines[j].trim());
    result = parts.join(' ');
  }
  return result;
}
function github(eventName, headRepo = canonical, overrides = {}) {
  return {
    repository: canonical, event_name: eventName,
    event: {pull_request: {draft: false, head: {repo: {full_name: headRepo}}, user: {login: 'maintainer', type: 'User'}}},
    ...overrides,
  };
}
function evaluate(expression, context) {
  // Evaluate the actual workflow predicate, not a duplicate routing function.
  // This intentionally accepts only its current string/boolean comparison,
  // boolean operator and fromJSON subset; other expression syntax fails closed.
  const lookup = name => {
    let item = context;
    for (const part of name.split('.').slice(1)) {
      if (item == null || typeof item !== 'object' || !Object.hasOwn(item, part)) return '';
      item = item[part];
    }
    return item ?? '';
  };
  const tokens = /\s+|'(?:[^']|'')*'|github(?:\.[A-Za-z_][\w-]*)+|fromJSON|true|false|null|==|!=|&&|\|\||[(),!]/gy;
  let translated = '', position = 0;
  while (position < expression.length) {
    tokens.lastIndex = position;
    const found = tokens.exec(expression);
    assert.ok(found, `Unsupported routing expression near ${expression.slice(position)}`);
    const token = found[0]; position = tokens.lastIndex;
    if (/^\s+$/.test(token)) translated += token;
    else if (token.startsWith("'")) translated += JSON.stringify(token.slice(1, -1).replaceAll("''", "'"));
    else if (token.startsWith('github.')) translated += `lookup(${JSON.stringify(token)})`;
    else translated += token;
  }
  const output = vm.runInNewContext(translated, {lookup, fromJSON: JSON.parse}, {timeout: 100});
  return JSON.parse(JSON.stringify(output));
}
function route(block, context = github('workflow_dispatch')) {
  const raw = value(block, 'runs-on');
  if (raw.startsWith('${{') && raw.endsWith('}}')) return evaluate(raw.slice(3, -2).trim(), context);
  if (raw.startsWith('[')) return JSON.parse(raw);
  return raw.replace(/^['"]|['"]$/g, '');
}
function step(block, name) {
  const parts = block.split(`      - name: ${name}\n`);
  assert.equal(parts.length, 2, `Missing or ambiguous step: ${name}`);
  return parts[1].split(/^      - /m, 1)[0];
}
function script(block, name) {
  const selected = step(block, name);
  const literal = selected.split('        run: |\n')[1];
  assert.ok(literal, `Missing literal shell script: ${name}`);
  const lines = [];
  for (const line of literal.split('\n')) {
    if (line && !line.startsWith('          ')) break;
    lines.push(line.slice(10));
  }
  return lines.join('\n').trimEnd();
}

test('every workflow job has an explicit runner policy and the isolated Speedle label', () => {
  assert.deepEqual(Object.keys(job.ci), ['lint', 'test', 'build']);
  assert.deepEqual(Object.keys(job.review), ['diff-regression', 'review', 'publish', 'automerge-dependabot']);
  assert.deepEqual(Object.keys(job.commands), ['prepare', 'analyze', 'publish']);
  assert.deepEqual(Object.keys(job.inline), ['route']);
  for (const name of ['diff-regression']) assert.deepEqual(route(job.review[name]), labels);
  for (const block of Object.values(job.commands)) assert.deepEqual(route(block), labels);
  assert.equal(route(job.review['automerge-dependabot']), 'ubuntu-24.04');
  assert.equal(route(job.inline.route), 'ubuntu-24.04');
  const lint = fs.readFileSync(path.join(root, '.github/actionlint.yaml'), 'utf8');
  assert.match(lint, /^\s+- charming-speedle$/m);
});
test('Go CI routes canonical pushes, dispatch and same-repository human or Dependabot PRs to Speedle', () => {
  const dependabot = github('pull_request');
  dependabot.event.pull_request.user = {login: 'dependabot[bot]', type: 'Bot'};
  for (const [name, context] of [
    ['push', github('push', undefined, {event: {}})],
    ['dispatch', github('workflow_dispatch', undefined, {event: {}})],
    ['human PR', github('pull_request')], ['Dependabot PR', dependabot],
  ]) {
    for (const [id, block] of Object.entries(job.ci)) assert.deepEqual(route(block, context), labels, `${id}: ${name}`);
  }
});
test('Go CI falls back for forks, deleted heads, unknown events and copied repositories', () => {
  const cases = [
    github('pull_request', 'outsider/speedle-plus'),
    github('pull_request', undefined, {event: {pull_request: {head: {repo: null}}}}),
    github('pull_request', undefined, {event: {pull_request: {}}}),
    github('issue_comment'), github('pull_request_target'), github('unknown'),
    github('push', undefined, {repository: 'outsider/speedle-plus', event: {}}),
    github('workflow_dispatch', undefined, {repository: 'outsider/speedle-plus', event: {}}),
    github('pull_request', 'outsider/speedle-plus', {repository: 'outsider/speedle-plus'}),
    github('push', undefined, {repository: '', event: {}}),
  ];
  for (const context of cases) {
    for (const [id, block] of Object.entries(job.ci)) assert.equal(route(block, context), 'ubuntu-latest', `${id}: ${JSON.stringify(context)}`);
  }
});
test('ReviewBot matches Pavu: same-repository human authors use Speedle, bots and forks use hosted', () => {
  const human = github('pull_request_target');
  const bot = github('pull_request_target'); bot.event.pull_request.user = {login: 'dependabot[bot]', type: 'Bot'};
  const otherBot = github('pull_request_target'); otherBot.event.pull_request.user = {login: 'automation[bot]', type: 'Bot'};
  const unknownAuthor = github('pull_request_target'); delete unknownAuthor.event.pull_request.user;
  for (const id of ['review', 'publish']) {
    assert.deepEqual(route(job.review[id], human), labels);
    for (const context of [bot, otherBot, unknownAuthor, github('pull_request_target', 'outsider/speedle-plus'),
      github('pull_request_target', undefined, {event: {pull_request: {user: {type: 'User'}}}}),
      github('pull_request_target', 'outsider/speedle-plus', {repository: 'outsider/speedle-plus'})]) {
      assert.equal(route(job.review[id], context), 'ubuntu-24.04', `${id}: ${JSON.stringify(context)}`);
    }
  }
  for (const eventName of ['push', 'workflow_dispatch', 'issue_comment', 'unknown']) {
    assert.equal(evaluate(value(job.review.review, 'if'), github(eventName)), false);
  }
});
test('inline pointer stays hosted and secret-bearing command receiver keeps default-branch triggers', () => {
  assert.equal(route(job.inline.route), 'ubuntu-24.04');
  assert.match(sources.inline, /^permissions: \{\}$/m);
  assert.doesNotMatch(sources.inline, /\buses:|\bsecrets\.|\bcheckout@|\b(?:upload|download)-artifact/);
  const triggers = sources.commands.split('\npermissions:', 1)[0];
  assert.match(triggers, /^  issue_comment:/m); assert.match(triggers, /^  workflow_run:/m);
  assert.doesNotMatch(triggers, /^  pull_request_review_comment:/m);
  assert.match(job.commands.analyze, /needs\.prepare\.outputs\.should_run == 'true'/);
  assert.match(job.commands.publish, /needs\.prepare\.outputs\.should_run == 'true'/);
});
test('Go CI uses read-only checkout credentials and sets up the required Go version', () => {
  assert.match(sources.ci, /^permissions:\n  contents: read$/m);
  assert.match(sources.ci, /^on: \[push, pull_request, workflow_dispatch\]$/m);
  for (const block of Object.values(job.ci)) {
    const checkout = block.split(/      - uses: actions\/checkout@[^\n]+\n/)[1]?.split(/^      - /m, 1)[0];
    assert.ok(checkout, 'Missing checkout step'); assert.match(checkout, /persist-credentials: false/);
    assert.match(block, /uses: actions\/setup-go@/); assert.match(block, /go-version: '1\.26'/);
  }
});
test('fixture job installs pinned Node 22 before running Node regression suites', () => {
  const block = job.review['diff-regression'];
  const setup = step(block, 'Set up Node for regression tests');
  assert.match(setup, /uses: actions\/setup-node@[a-f0-9]{40}(?:\s|$)/);
  assert.match(setup, /node-version: '22'/);
  assert.ok(block.indexOf('Set up Node for regression tests') < block.indexOf('node --test'));
  const preflight = script(block, 'Verify runner identity and tools');
  assert.match(preflight, /test "\$\{RUNNER_NAME\}" = "\$\{EXPECTED_RUNNER\}"/);
  assert.match(preflight, /test "\$\{RUNNER_OS\}" = Linux/);
  for (const tool of ['git', 'curl', 'jq', 'python3']) assert.ok(preflight.includes(tool));
  assert.ok(preflight.includes('https://ark.cn-beijing.volces.com/api/plan/v3/responses'));
  assert.doesNotMatch(preflight, /Authorization|ARK_API_KEY|--data|--request\s+POST/);
});

test('actual Go test shell propagates failures and serializes packages for both stores', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-routing-shell-'));
  try {
    const executable = path.join(directory, 'go'), calls = path.join(directory, 'calls');
    fs.writeFileSync(executable, '#!/bin/sh\nif [ "$1" = list ]; then printf "example.invalid/fixture\\n"; exit 0; fi\nprintf "%s\\n" "$STORE_TYPE $*" >> "$FIXTURE_CALLS"\nexit "$FIXTURE_GO_EXIT"\n');
    fs.chmodSync(executable, 0o755);
    const source = script(job.ci.test, 'Run tests with coverage');
    assert.match(source, /go test -p 1\b/);
    for (const store of ['file', 'etcd']) {
      const command = source.replaceAll('${{ matrix.store-type }}', store);
      assert.doesNotMatch(command, /\$\{\{/);
      for (const code of [0, 23]) {
        const result = spawnSync('bash', ['-e', '-c', command], {encoding: 'utf8', env: {
          ...process.env, PATH: `${directory}${path.delimiter}${process.env.PATH}`,
          FIXTURE_CALLS: calls, FIXTURE_GO_EXIT: String(code),
        }});
        assert.equal(result.status, code, result.stderr);
      }
    }
    const log = fs.readFileSync(calls, 'utf8');
    assert.match(log, /file test -p 1/); assert.match(log, /etcd test -p 1/);
  } finally {fs.rmSync(directory, {recursive: true, force: true});}
});

function listen() {
  const server = net.createServer();
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen({host: '127.0.0.1', port: 0}, () => resolve(server));
  });
}
const close = server => new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
async function freePort() {const server = await listen(); const port = server.address().port; await close(server); return port;}
function portGuard(ports) {
  const block = step(job.ci.test, 'Ensure embedded test ports are unused');
  assert.match(block, /if: runner\.environment == 'self-hosted'/);
  const source = script(job.ci.test, 'Ensure embedded test ports are unused');
  const match = source.match(/^python3 - <<'PYTHON'\n([\s\S]+)\nPYTHON$/);
  assert.ok(match, 'Missing actual Python socket guard');
  assert.equal((match[1].match(/for port in \(2379, 2380\):/g) || []).length, 1);
  // Only test-owned ephemeral ports are substituted. Never bind host etcd ports.
  const code = match[1].replace('for port in (2379, 2380):', `for port in (${ports.join(', ')}):`);
  return spawnSync('python3', ['-c', code], {encoding: 'utf8'});
}
test('actual socket guard accepts two free test-owned ephemeral ports', async () => {
  const first = await freePort(); let second = await freePort();
  while (second === first) second = await freePort();
  const result = portGuard([first, second]);
  assert.equal(result.status, 0, result.stderr);
});
test('actual socket guard refuses either occupied port while leaving the existing listener untouched', async () => {
  for (const index of [0, 1]) {
    const listener = await listen(); let connections = 0;
    listener.on('connection', socket => {connections++; socket.end();});
    try {
      const occupied = listener.address().port; let free = await freePort();
      while (free === occupied) free = await freePort();
      const ports = index === 0 ? [occupied, free] : [free, occupied];
      const result = portGuard(ports);
      assert.notEqual(result.status, 0); assert.match(result.stderr, new RegExp(`Test port ${occupied} is already in use`));
      assert.equal(listener.listening, true); assert.equal(listener.address().port, occupied); assert.equal(connections, 0);
    } finally {await close(listener);}
  }
});
