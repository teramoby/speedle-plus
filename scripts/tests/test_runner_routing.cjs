/* Offline protected-callee and ReviewBot routing regressions. No GitHub or Ark calls. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
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

test('review and command jobs retain their explicit runner policies and isolated Speedle label', () => {
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
test('Go CI caller delegates only known task inputs to the protected default-branch worker', () => {
  assert.match(sources.ci, /^permissions:\n  contents: read$/m);
  assert.match(sources.ci, /^on: \[push, pull_request, workflow_dispatch\]$/m);
  for (const [id, block] of Object.entries(job.ci)) {
    assert.equal(value(block, 'uses'), 'teramoby/speedle-plus/.github/workflows/go-ci-worker.yml@master');
    assert.doesNotMatch(block, /^    (?:runs-on|steps|container|env|secrets|timeout-minutes):/m);
    const mapping = block.split('    with:\n')[1];
    assert.ok(mapping, `Missing worker task inputs for ${id}`);
    const inputs = Object.fromEntries([...mapping.matchAll(/^      ([a-z_]+): (.+)$/gm)].map(match => [match[1], match[2]]));
    assert.equal(inputs.mode, id);
    assert.deepEqual(Object.keys(inputs), id === 'test' ? ['mode', 'store_type'] : ['mode']);
    if (id === 'test') {
      assert.match(block, /fail-fast: false/);
      assert.equal(inputs.store_type, '${{ matrix.store-type }}');
      assert.match(block, /store-type: \[file, etcd\]/);
      assert.match(block, /name: test \(\$\{\{ matrix.store-type \}\}\)/);
    }
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
