/* Protected-callee and ReviewBot routing regressions. No GitHub or Ark calls
 * unless the optional live runner-inventory check is enabled. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const test = require('node:test');
const vm = require('node:vm');

const root = path.resolve(__dirname, '../..');
const canonical = 'teramoby/speedle-plus';
const labels = ['self-hosted', 'speedle-runner'];
const read = name => fs.readFileSync(path.join(root, '.github/workflows', name), 'utf8');
const sources = {
  ci: read('ci.yml'), review: read('reviewbot.yml'), preflight: read('reviewbot-preflight.yml'),
  commands: read('reviewbot-command.yml'), inline: read('reviewbot-inline.yml'),
  nightly: read('nightly.yml'), weekly: read('weekly.yml'), postMerge: read('post-merge.yml'),
  mongodb: read('mongodb-nightly-validation.yml'),
  worker: read('go-ci-worker.yml'),
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
function evaluate(expression, context, vars = {}, inputs = null) {
  // Evaluate the actual workflow predicate, not a duplicate routing function.
  // This intentionally accepts only its current string/boolean comparison,
  // boolean operator, contains, array wildcard and fromJSON subset; other
  // expression syntax fails closed. Workflow inputs are opt-in so every other
  // gate still rejects an `inputs.*` lookup instead of trusting it.
  const lookup = name => {
    assert.ok(!name.startsWith('inputs.') || inputs !== null, `Unsupported inputs lookup: ${name}`);
    const walk = (item, parts) => {
      if (!parts.length) return item ?? '';
      const [part, ...rest] = parts;
      if (part === '*') return (Array.isArray(item) ? item : []).map(entry => walk(entry, rest));
      if (item == null || typeof item !== 'object' || !Object.hasOwn(item, part)) return '';
      return walk(item[part], rest);
    };
    const parts = name.split('.');
    return walk(parts[0] === 'vars' ? vars : parts[0] === 'inputs' ? inputs : context, parts.slice(1));
  };
  const tokens = /\s+|'(?:[^']|'')*'|(?:github|vars|inputs)(?:\.(?:[A-Za-z_][\w-]*|\*))+|fromJSON|contains|true|false|null|==|!=|&&|\|\||[(),!]/gy;
  let translated = '', position = 0;
  while (position < expression.length) {
    tokens.lastIndex = position;
    const found = tokens.exec(expression);
    assert.ok(found, `Unsupported routing expression near ${expression.slice(position)}`);
    const token = found[0]; position = tokens.lastIndex;
    if (/^\s+$/.test(token)) translated += token;
    else if (token.startsWith("'")) translated += JSON.stringify(token.slice(1, -1).replaceAll("''", "'"));
    else if (token.startsWith('github.') || token.startsWith('vars.') || token.startsWith('inputs.')) translated += `lookup(${JSON.stringify(token)})`;
    else translated += token;
  }
  const output = vm.runInNewContext(translated,
    {lookup, fromJSON: JSON.parse, contains: (values, value) => values.includes(value)}, {timeout: 100});
  return JSON.parse(JSON.stringify(output));
}
function route(block, context = github('workflow_dispatch'), vars = {}, inputs = null) {
  const raw = value(block, 'runs-on');
  if (raw.startsWith('${{') && raw.endsWith('}}')) return evaluate(raw.slice(3, -2).trim(), context, vars, inputs);
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
  assert.deepEqual(Object.keys(job.ci), ['changes', 'lint', 'test', 'build', 'quick', 'summary']);
  assert.doesNotMatch(sources.ci.replace(/^\s*#.*$/gm, ''), /mysql|mongo|check_etcd/i, 'database backends run nightly, not per PR');
  assert.deepEqual(Object.keys(job.review),
    ['runner-preflight', 'diff-regression', 'review', 'publish', 'automerge-dependabot']);
  assert.deepEqual(Object.keys(job.commands), ['prepare', 'runner-preflight', 'analyze', 'publish']);
  assert.deepEqual(Object.keys(job.inline), ['route']);
  for (const name of ['diff-regression']) assert.deepEqual(route(job.review[name]), labels);
  assert.equal(route(job.commands.prepare), 'ubuntu-24.04');
  for (const id of ['analyze', 'publish']) assert.deepEqual(route(job.commands[id]), labels);
  assert.equal(route(job.review['automerge-dependabot']), 'ubuntu-24.04');
  assert.equal(route(job.inline.route), 'ubuntu-24.04');
  // The preflight is a hosted job that calls one reusable workflow with no
  // permissions; the credential-bearing model calls wait for it.
  for (const [name, block] of [['review', job.review['runner-preflight']],
    ['commands', job.commands['runner-preflight']]]) {
    assert.match(block, /^    uses: \.\/\.github\/workflows\/reviewbot-preflight\.yml$/m, name);
    assert.match(block, /^    permissions: \{\}$/m, name);
    assert.doesNotMatch(block, /runs-on:|secrets:|actions\/checkout/, name);
  }
  assert.match(job.commands['runner-preflight'], /^    needs: prepare$/m);
  assert.match(job.review.review, /^    needs: runner-preflight$/m,
    'the model call must wait for the runner-override preflight');
  assert.match(job.commands.analyze, /^    needs: \[prepare, runner-preflight\]$/m,
    'the model call must wait for the runner-override preflight');
  assert.match(job.commands.analyze, /needs\.runner-preflight\.result == 'success'/,
    "always() must not let the model call run after a failed preflight");
  const lint = fs.readFileSync(path.join(root, '.github/actionlint.yaml'), 'utf8');
  for (const label of ['speedle-linux', 'speedle-control', 'speedle-runner', 'fiona-speedle']) {
    assert.match(lint, new RegExp(`^\\s+- ${label}$`, 'm'), `${label} must stay declared for workflow lint`);
  }
  assert.doesNotMatch(lint, /^\s+- charming-speedle$/m, 'the retired runner identity must not stay declared');
});
test('same-repository human and bot authors use Speedle; forks and unknown authors stay hosted', () => {
  const human = github('pull_request_target');
  const bot = github('pull_request_target'); bot.event.pull_request.user = {login: 'dependabot[bot]', type: 'Bot'};
  const otherBot = github('pull_request_target'); otherBot.event.pull_request.user = {login: 'automation[bot]', type: 'Bot'};
  const unknownAuthor = github('pull_request_target'); delete unknownAuthor.event.pull_request.user;
  for (const id of ['review', 'publish']) {
    for (const author of [human, bot, otherBot]) assert.deepEqual(route(job.review[id], author), labels);
    const forkBot = github('pull_request_target', 'outsider/speedle-plus'); forkBot.event.pull_request.user.type = 'Bot';
    for (const context of [forkBot, unknownAuthor, github('pull_request_target', 'outsider/speedle-plus'),
      github('pull_request_target', undefined, {event: {pull_request: {user: {type: 'User'}}}}),
      github('pull_request_target', 'outsider/speedle-plus', {repository: 'outsider/speedle-plus'})]) {
      assert.equal(route(job.review[id], context), 'ubuntu-24.04', `${id}: ${JSON.stringify(context)}`);
    }
  }
  for (const eventName of ['push', 'workflow_dispatch', 'issue_comment', 'unknown']) {
    assert.equal(evaluate(value(job.review.review, 'if'), github(eventName)), false);
  }
});
test('REVIEWBOT_RUNS_ON overrides only the two model-calling ReviewBot jobs and keeps forks hosted', () => {
  const customLabels = ['self-hosted', 'Linux', 'X64', 'reviewbot'];
  const customVars = {REVIEWBOT_RUNS_ON: JSON.stringify(customLabels)};
  const emptyVars = {REVIEWBOT_RUNS_ON: ''};
  const invalidVars = {REVIEWBOT_RUNS_ON: 'not-json'};
  const human = github('pull_request_target');
  const bot = github('pull_request_target'); bot.event.pull_request.user = {login: 'dependabot[bot]', type: 'Bot'};
  const otherBot = github('pull_request_target'); otherBot.event.pull_request.user = {login: 'automation[bot]', type: 'Bot'};
  const unknownAuthor = github('pull_request_target'); delete unknownAuthor.event.pull_request.user;
  const forkBot = github('pull_request_target', 'outsider/speedle-plus'); forkBot.event.pull_request.user.type = 'Bot';

  for (const author of [human, bot, otherBot]) {
    assert.deepEqual(route(job.review.review, author, customVars), customLabels);
    assert.deepEqual(route(job.review.review, author, emptyVars), labels);
    assert.deepEqual(route(job.review.publish, author, customVars), labels);
  }
  assert.deepEqual(route(job.commands.analyze, github('issue_comment'), customVars), customLabels);
  assert.deepEqual(route(job.commands.analyze, github('issue_comment'), emptyVars), labels);
  assert.throws(() => route(job.review.review, human, invalidVars), SyntaxError);
  assert.throws(() => route(job.commands.analyze, github('issue_comment'), invalidVars), SyntaxError);

  for (const context of [forkBot, unknownAuthor, github('pull_request_target', 'outsider/speedle-plus'),
    github('pull_request_target', undefined, {event: {pull_request: {user: {type: 'User'}}}}),
    github('pull_request_target', 'outsider/speedle-plus', {repository: 'outsider/speedle-plus'})]) {
    assert.equal(route(job.review.review, context, customVars), 'ubuntu-24.04', JSON.stringify(context));
    assert.equal(route(job.review.review, context, invalidVars), 'ubuntu-24.04', JSON.stringify(context));
  }

  assert.deepEqual(route(job.review['diff-regression'], github('workflow_dispatch'), customVars), labels);
  assert.match(job.review['diff-regression'],
    /CONFIGURED_REVIEWBOT_RUNS_ON: \$\{\{ vars\.REVIEWBOT_RUNS_ON/,
    'the trusted regression validates the configured override without routing on it');
  assert.equal(route(job.review['automerge-dependabot'], human, customVars), 'ubuntu-24.04');
  assert.equal(route(job.commands.prepare, github('issue_comment'), customVars), 'ubuntu-24.04');
  assert.deepEqual(route(job.commands.publish, github('issue_comment'), customVars), labels);
  assert.equal(route(job.inline.route, github('pull_request_review_comment'), customVars), 'ubuntu-24.04');

  for (const [name, block] of [
    ['review.publish', job.review.publish],
    ['review.automerge-dependabot', job.review['automerge-dependabot']],
    ['commands.prepare', job.commands.prepare],
    ['commands.publish', job.commands.publish],
    ['inline.route', job.inline.route],
  ]) {
    assert.doesNotMatch(block, /REVIEWBOT_RUNS_ON/, `${name} must not depend on REVIEWBOT_RUNS_ON`);
  }
  for (const [name, block] of [['review.review', job.review.review], ['commands.analyze', job.commands.analyze]]) {
    assert.doesNotMatch(block, /RUNNER_NAME|charming-speedle|fiona-speedle|\/tmp\/speedle-plus-/, `${name} must stay host-neutral`);
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
const worker = 'teramoby/speedle-plus/.github/workflows/go-ci-worker.yml@master';
function workerInputs(block, id) {
  assert.equal(value(block, 'uses'), worker, id);
  assert.doesNotMatch(block, /^    (?:runs-on|steps|container|env|secrets|timeout-minutes|strategy):/m, id);
  const mapping = block.split('    with:\n')[1];
  assert.ok(mapping, `Missing worker task inputs for ${id}`);
  return Object.fromEntries([...mapping.matchAll(/^      ([a-z_]+): (.+)$/gm)].map(match => [match[1], match[2]]));
}
test('Go CI caller delegates only the task mode to the protected default-branch worker', () => {
  assert.match(sources.ci, /^permissions:\n  contents: read$/m);
  assert.match(sources.ci, /^on:\n(?:  #.*\n)*  push:\n    branches: \[master\]\n  pull_request:\n    types: \[opened, synchronize, reopened, labeled\]\n  workflow_dispatch:\n/m);
  for (const id of ['lint', 'test', 'build', 'quick']) assert.deepEqual(workerInputs(job.ci[id], id), {mode: id});
  assert.match(job.ci.test, /name: test$/m);
  assert.equal(route(job.ci.changes), 'ubuntu-latest');
  assert.doesNotMatch(job.ci.changes, /secrets\.|uses: teramoby/);
  // The site checks run in the worker's build (verify.sh) for docs changes too.
  assert.match(value(job.ci.build, 'if'), /needs\.changes\.outputs\.docs == 'true'/);
});
test('every Go task uses the protected worker; only classification and reporting jobs run directly on hosted runners', () => {
  const hosted = {ci: ['changes', 'summary'], nightly: ['report'], weekly: ['report'], postMerge: ['report']};
  for (const [kind, ids] of Object.entries(hosted)) {
    for (const [id, block] of Object.entries(job[kind])) {
      if (ids.includes(id)) {
        assert.equal(route(block), 'ubuntu-latest', `${kind}.${id}`);
        continue;
      }
      assert.equal(value(block, 'uses'), worker, `${kind}.${id} must run through the protected worker`);
    }
  }
  assert.deepEqual(Object.keys(job.mongodb), ['mongodb']);
  assert.equal(route(job.mongodb.mongodb), 'speedle-runner');
  // A manual run may pin one Speedle runner by its label, e.g. fiona.
  const pinned = github('workflow_dispatch', canonical, {event: {inputs: {runner: 'speedle-macos'}}});
  assert.equal(route(job.mongodb.mongodb, pinned), 'speedle-macos');
  const trial = github('workflow_dispatch', canonical, {event: {inputs: {runner: 'speedle-linux', nightly_trial: 'true'}}});
  assert.equal(route(job.mongodb.mongodb, trial), 'fiona-speedle');
  for (const task of ['lint', 'test', 'regression', 'weekly', 'reviewbot']) {
    const ci = github('workflow_dispatch', canonical, {event: {inputs: {runner: 'speedle-linux', ci_trial: task}}});
    assert.equal(route(job.mongodb.mongodb, ci), 'fiona-speedle', task);
  }
  const none = github('workflow_dispatch', canonical, {event: {inputs: {runner: 'speedle-linux', ci_trial: 'none'}}});
  assert.equal(route(job.mongodb.mongodb, none), 'speedle-linux');
  assert.match(sources.mongodb, /options: \[speedle-runner, speedle-macos, speedle-linux\]/);
});
test('scheduled suites run through the worker and report from a hosted job', () => {
  const cron = {nightly: "'0 17 * * *'", weekly: "'0 11 * * 0'"};  // 01:00 and Sunday 19:00 GMT+8
  for (const kind of ['nightly', 'weekly']) {
    const source = sources[kind];
    assert.match(source, /^permissions:\n  contents: read$/m);
    assert.match(source, new RegExp(`^    - cron: ${cron[kind].replace(/[*]/g, '\\*')}$`, 'm'));
    assert.match(source, /^  workflow_dispatch:$/m);
    assert.deepEqual(workerInputs(job[kind].run, `${kind}.run`), {mode: kind});
    const report = job[kind].report;
    assert.equal(route(report), 'ubuntu-latest');
    assert.match(report, /needs: run$/m);
    assert.match(report, /if: always\(\) && github.repository == 'teramoby\/speedle-plus'/);
    assert.match(report, /permissions:\n      contents: read\n      issues: write\n/);
    assert.match(report, /persist-credentials: false/);
    assert.match(report, /ARK_API_KEY: \$\{\{ secrets.ARK_API_KEY \}\}/);
    assert.match(report, /ARK_MODEL: \$\{\{ vars.ARK_REVIEW_MODEL \|\| 'kimi-k3' \}\}/);
    assert.match(report, /MAIL_TO: \$\{\{ vars.TEST_REPORT_MAIL_TO \|\| 'caiwl@acm.org' \}\}/);
    assert.match(report, /FEISHU_WEBHOOK: \$\{\{ secrets.FEISHU_WEBHOOK \}\}/);
    assert.match(report, /FEISHU_WEBHOOK_SECRET: \$\{\{ secrets.FEISHU_WEBHOOK_SECRET \}\}/);
    assert.match(report, new RegExp(`scripts/ci/test_report.py --kind ${kind} `));
    for (const [id, block] of Object.entries(job[kind])) {
      if (id !== 'report') assert.doesNotMatch(block, /secrets\./, `${kind}.${id} must not receive secrets`);
    }
  }
  assert.deepEqual(Object.keys(job.nightly), ['run', 'report']);
  assert.deepEqual(Object.keys(job.weekly), ['run', 'report']);
  // The nightly runs MongoDB with the other backends; this workflow is manual.
  assert.doesNotMatch(sources.mongodb, /^  schedule:/m);
  assert.match(sources.mongodb, /^  workflow_dispatch:$/m);
});
// Every registered runner and its labels, as returned by the runner API. The
// scheduling labels are not listed here by hand: the tests below derive them
// from every workflow's `runs-on`. Refresh this snapshot when a registration
// or one of its labels changes:
//   gh api --paginate orgs/teramoby/actions/runners --jq '.runners[] | {name, os, labels: [.labels[].name]}'
//   gh api --paginate repos/teramoby/speedle-plus/actions/runners --jq '.runners[] | {name, os, labels: [.labels[].name]}'
// A task routed to labels no registered runner carries queues until cancelled;
// one routed to a host its identity guard rejects fails before any test runs.
// A runner identity is removed from a guard only after its registration no
// longer carries a scheduling label, so the guard list must equal the hosts
// this inventory says a scheduling label can select.
const runnerInventory = [
  {name: 'charming-speedle-2', os: 'Linux', arch: 'X64',
    labels: ['self-hosted', 'Linux', 'X64', 'speedle-linux', 'speedle-control', 'speedle-runner', 'reviewbot']},
  {name: 'donkey-speedle', os: 'Linux', arch: 'X64',
    labels: ['self-hosted', 'Linux', 'X64', 'speedle-linux', 'speedle-control', 'speedle-runner', 'reviewbot']},
  // The aliyun host of the organization ReviewBot runner group. A
  // REVIEWBOT_RUNS_ON value that selects its labels is checked against this
  // inventory; no workflow default route selects them. The snapshot records
  // the current registration and does not define the group's repository scope.
  {name: 'ecs-light-teramoby', os: 'Linux', arch: 'X64',
    labels: ['self-hosted', 'Linux', 'X64', 'aliyun-ecs', 'reviewbot']},
  {name: 'fiona', os: 'macOS', arch: 'ARM64',
    labels: ['self-hosted', 'macOS', 'ARM64', 'speedle-runner', 'speedle-macos', 'fiona-speedle']},
];
// The guard tests execute the worker's real shell check, which needs these.
const guardTools = spawnSync('bash', ['-c', 'command -v bash git python3'], {encoding: 'utf8'});
assert.equal(guardTools.status, 0, 'the routing guard needs bash, git and python3 in PATH');
function assertRoutedToAcceptedRunners(mode, event, routed) {
  const guard = script(job.worker.execute, 'Verify self-hosted runner identity');
  assert.ok(Array.isArray(routed), `${mode} on ${event}: ${JSON.stringify(routed)}`);
  assert.equal(routed[0], 'self-hosted', `${mode} on ${event} must select the self-hosted pool`);
  const candidates = runnerInventory.filter(runner => routed.every(label => runner.labels.includes(label)));
  assert.ok(candidates.length > 0,
    `${mode} on ${event} selects ${routed.join(', ')}, which no registered runner carries; `
    + 'refresh runnerInventory from the API commands above');
  for (const runner of candidates) {
    const result = spawnSync('bash', ['-c', guard], {encoding: 'utf8',
      env: {...process.env, RUNNER_NAME: runner.name, RUNNER_OS: runner.os,
        RUNNER_ARCH: runner.arch, CI_TASK: mode}});
    assert.ok(!result.error, `the routing guard could not run: ${result.error}`);
    assert.equal(result.status, 0,
      `${mode} on ${event} can land on ${runner.name}, which the identity guard rejects:\n${result.stderr}`);
  }
  return candidates;
}
test('every self-hosted Go task can only land on runners the identity guard accepts', () => {
  const trusted = [
    ['workflow_dispatch', github('workflow_dispatch')],
    ['schedule', {...github('schedule'), ref: 'refs/heads/master'}],
    ['push', {...github('push'), ref: 'refs/heads/master'}],
    ['pull_request', github('pull_request')],
  ];
  for (const mode of ['quick', 'lint', 'test', 'build', 'nightly', 'weekly', 'regression']) {
    for (const [event, context] of trusted) {
      const routed = route(job.worker.execute, context, {}, {mode});
      if (routed === 'ubuntu-latest') {
        // Only quick defaults to a hosted runner, where the guard is skipped.
        assert.equal(mode, 'quick', `${mode} on ${event} must select a Speedle runner`);
        continue;
      }
      const candidates = assertRoutedToAcceptedRunners(mode, event, routed);
      if (mode === 'weekly') {
        assert.deepEqual(candidates.map(runner => runner.name), ['fiona'],
          'weekly must schedule only on the macOS runner');
      }
    }
  }
});
test('configured runner overrides can only land on runners the identity guard accepts', () => {
  const pool = '["self-hosted","Linux","X64","speedle-linux"]';
  const quick = route(job.worker.execute, github('pull_request'), {QUICK_RUNS_ON: pool}, {mode: 'quick'});
  assert.deepEqual(quick, ['self-hosted', 'Linux', 'X64', 'speedle-linux']);
  assertRoutedToAcceptedRunners('quick', 'QUICK_RUNS_ON pull request', quick);

  // A merge-head label routes the full jobs that can run from a pull request
  // to the reserved pool. The schedule-only tasks keep their own pools.
  const head = {...github('pull_request'), event: {pull_request: {
    draft: false, labels: [{name: 'merge-head'}],
    head: {repo: {full_name: canonical}}, user: {login: 'maintainer', type: 'User'}}}};
  for (const mode of ['lint', 'test', 'build']) {
    const routed = route(job.worker.execute, head, {HEAD_RUNS_ON: pool}, {mode});
    assert.deepEqual(routed, ['self-hosted', 'Linux', 'X64', 'speedle-linux'], mode);
    assertRoutedToAcceptedRunners(mode, `merge-head pull request (${mode})`, routed);
  }
  const overrides = {QUICK_RUNS_ON: pool, HEAD_RUNS_ON: pool};
  const scheduled = {...github('schedule'), ref: 'refs/heads/master'};
  assert.deepEqual(route(job.worker.execute, scheduled, overrides, {mode: 'weekly'}),
    ['self-hosted', 'fiona-speedle']);
  assert.deepEqual(route(job.worker.execute, scheduled, overrides, {mode: 'nightly'}), labels);
  assert.deepEqual(route(job.worker.execute, {...github('push'), ref: 'refs/heads/master'}, overrides,
    {mode: 'regression'}), labels);
});
// Extract the name/OS/arch alternatives from the case arms of a guard script.
function acceptedIdentities(guard) {
  return [...guard.matchAll(/^ {2}((?:[\w.-]+\/[\w.-]+\/[\w.-]+)(?:\|[\w.-]+\/[\w.-]+\/[\w.-]+)*)\)(?: ;;)?$/gm)]
    .flatMap(match => match[1].split('|')).sort();
}
const platformLabels = new Set(['self-hosted', 'Linux', 'X64', 'macOS', 'ARM64']);
const hostedLabels = values => values.some(value => /^(?:ubuntu|windows|macos)-(?:latest|\d)/.test(value));
const workflowFiles = fs.readdirSync(path.join(root, '.github/workflows'))
  .filter(name => name.endsWith('.yml')).sort();
const workflowSource = name => fs.readFileSync(path.join(root, '.github/workflows', name), 'utf8');
// A folded `runs-on: >-` value continues on more-indented lines.
function runsOnValues(source) {
  const lines = source.split('\n');
  const values = [];
  for (let i = 0; i < lines.length; i++) {
    const match = lines[i].match(/^\s*runs-on:\s*(.*)$/);
    if (!match) continue;
    const scalar = match[1].trim();
    const keyIndent = lines[i].length - lines[i].trimStart().length;
    if (scalar && !/^[>|][-+]?$/.test(scalar)) { values.push(scalar); continue; }
    const parts = [];
    if (scalar) {
      for (let j = i + 1; j < lines.length; j++) {
        if (!lines[j].trim()) continue;
        if (lines[j].length - lines[j].trimStart().length <= keyIndent) break;
        parts.push(lines[j].trim());
      }
    } else {
      for (let j = i + 1; j < lines.length; j++) {
        if (!lines[j].trim()) continue;
        const item = lines[j].match(/^\s*-\s+(.+)$/);
        if (!item || lines[j].length - lines[j].trimStart().length <= keyIndent) break;
        parts.push(item[1].trim());
      }
    }
    assert.ok(parts.length, `Unsupported runs-on block at line ${i + 1}`);
    if (scalar) values.push(parts.join(' '));
    else values.push(`[${parts.join(',')}]`);
  }
  return values;
}
// Labels from a plain scalar or a flow sequence such as ["self-hosted","Linux"].
function scalarLabels(raw) {
  const text = raw.trim().replace(/^['"]|['"]$/g, '');
  if (!text) return [];
  if (!text.startsWith('[')) return [text];
  try { return JSON.parse(text); } catch {
    return text.slice(1, -1).split(',').map(value => value.trim().replace(/^['"]|['"]$/g, '')).filter(Boolean);
  }
}
// The options of the workflow_dispatch input named `runner`.
function runnerChoices(source) {
  const lines = source.split('\n');
  const start = lines.findIndex(line => /^\s{4,}runner:\s*$/.test(line));
  if (start === -1) return [];
  const indent = lines[start].length - lines[start].trimStart().length;
  for (let i = start + 1; i < lines.length; i++) {
    const line = lines[i];
    if (!line.trim()) continue;
    if (line.length - line.trimStart().length <= indent) break;
    const match = line.match(/^\s+options: \[([^\]]*)\]/);
    if (match) return match[1].split(',').map(value => value.trim().replace(/^['"]|['"]$/g, '')).filter(Boolean);
  }
  return [];
}
// Label sets a workflow can pass to `runs-on`. Expressions contribute their
// quoted label literals; a `runner` choice input contributes its options.
function routeLabelSets(source) {
  return runsOnValues(source).flatMap(raw => {
    const open = raw.indexOf('${{');
    if (open === -1) {
      const values = scalarLabels(raw);
      assert.ok(values.length > 0, `Unsupported runs-on value: ${JSON.stringify(raw)}`);
      return hostedLabels(values) ? [] : [values];
    }
    const close = raw.lastIndexOf('}}');
    assert.ok(close > open, `Unclosed runs-on expression: ${JSON.stringify(raw)}`);
    let expression = raw.slice(open + 3, close);
    // `contains(...)` tests a pull-request label and a comparison right side
    // is a flag value, not a runner label.
    expression = expression.replace(/contains\([^()]*\)/g, ' ');
    expression = expression.replace(/[!=]= '[^']*'/g, ' ');
    const sets = [];
    for (const match of expression.matchAll(/'(\[[^']*\])'/g)) sets.push(JSON.parse(match[1]));
    expression = expression.replace(/'\[[^']*\]'/g, ' ');
    for (const match of expression.matchAll(/'([^']*)'/g)) {
      if (!match[1]) continue;
      if (!match[1].startsWith('"')) { sets.push([match[1]]); continue; }
      const value = JSON.parse(match[1]);
      if (!hostedLabels([value])) sets.push([value]);
    }
    if (/\binputs\.runner\b/.test(expression)) {
      for (const value of runnerChoices(source)) sets.push([value]);
    }
    assert.ok(sets.length > 0, `Unsupported runs-on expression without a label literal: ${JSON.stringify(raw)}`);
    return sets.filter(values => values.length > 0 && !hostedLabels(values));
  });
}
function declaredRoutes() {
  return workflowFiles.flatMap(workflow => routeLabelSets(workflowSource(workflow))
    .map(labels => ({workflow, labels})));
}
function customRoutingLabels() {
  const result = new Set(declaredRoutes().flatMap(route => route.labels));
  for (const label of platformLabels) result.delete(label);
  return result;
}
const routingLabels = customRoutingLabels();
function runnersFor(values) {
  return runnerInventory.filter(runner => values.every(label => runner.labels.includes(label)));
}
function scheduledHosts() {
  return runnerInventory.filter(runner => runner.labels.some(label => routingLabels.has(label)))
    .map(runner => `${runner.name}/${runner.os}/${runner.arch}`).sort();
}
// The approved REVIEWBOT_RUNS_ON label sets. The preflight workflow declares
// them in one constant; this suite reads that constant, uses it for the
// inventory checks and executes the preflight step with the same value.
function approvedReviewBotRunsOn() {
  const match = sources.preflight.match(/^ {6}APPROVED_REVIEWBOT_RUNS_ON: '(.+)'$/m);
  assert.ok(match, 'reviewbot-preflight.yml must declare APPROVED_REVIEWBOT_RUNS_ON');
  const sets = JSON.parse(match[1]);
  assert.ok(Array.isArray(sets) && sets.length > 0 &&
    sets.every(set => Array.isArray(set) && set.length > 0 &&
      set.every(label => typeof label === 'string' && label.length > 0)),
  'APPROVED_REVIEWBOT_RUNS_ON must be a JSON array of non-empty label sets');
  return sets;
}
const approvedRunsOn = approvedReviewBotRunsOn();
const canonicalLabels = values => JSON.stringify([...values].sort());
function approvedOverride(raw) {
  const values = JSON.parse(raw);
  assert.ok(Array.isArray(values) && values.length > 0 &&
    values.every(label => typeof label === 'string' && label.length > 0),
  'REVIEWBOT_RUNS_ON must be a non-empty JSON array of labels');
  assert.ok(approvedRunsOn.some(set => canonicalLabels(set) === canonicalLabels(values)),
    `${raw} is not an approved REVIEWBOT_RUNS_ON label set`);
  return values;
}
function reviewBotOverrideCandidates(raw) {
  const values = approvedOverride(raw);
  const candidates = runnersFor(values);
  assert.ok(candidates.length > 0,
    'REVIEWBOT_RUNS_ON matches no runner in the recorded inventory');
  return candidates;
}
test('ReviewBot overrides resolve only to recorded ReviewBot runners', () => {
  const liveGroup = JSON.stringify(['self-hosted', 'Linux', 'reviewbot']);
  const x64Group = JSON.stringify(['self-hosted', 'Linux', 'X64', 'reviewbot']);
  assert.deepEqual(reviewBotOverrideCandidates(liveGroup).map(runner => runner.name),
    ['charming-speedle-2', 'donkey-speedle', 'ecs-light-teramoby']);
  assert.deepEqual(reviewBotOverrideCandidates(x64Group).map(runner => runner.name),
    ['charming-speedle-2', 'donkey-speedle', 'ecs-light-teramoby']);
  assert.deepEqual(reviewBotOverrideCandidates(JSON.stringify(labels)).map(runner => runner.name),
    ['charming-speedle-2', 'donkey-speedle', 'fiona']);
  for (const raw of ['not-json', '{}', '[]', '["self-hosted",1]',
    JSON.stringify(['self-hosted', 'Linux', 'X64']),
    JSON.stringify(['self-hosted', 'Linux', 'X64', 'aliyun-ecs']),
    JSON.stringify(['self-hosted', 'macOS', 'ARM64'])]) {
    assert.throws(() => reviewBotOverrideCandidates(raw), undefined, raw);
  }
  reviewBotOverrideCandidates(process.env.CONFIGURED_REVIEWBOT_RUNS_ON || JSON.stringify(labels));
});
test('every REVIEWBOT_RUNS_ON fallback names the same approved label set', () => {
  const fallbacks = [];
  for (const [name, source] of [['reviewbot.yml', sources.review],
    ['reviewbot-command.yml', sources.commands], ['reviewbot-preflight.yml', sources.preflight]]) {
    for (const match of source.matchAll(/vars\.REVIEWBOT_RUNS_ON \|\| '((?:[^']|'')*)'/g)) {
      fallbacks.push([name, match[1].replaceAll("''", "'")]);
    }
  }
  assert.ok(fallbacks.length >= 3, `missing REVIEWBOT_RUNS_ON fallbacks: ${JSON.stringify(fallbacks)}`);
  const values = new Set(fallbacks.map(([, value]) => value));
  assert.equal(values.size, 1,
    `REVIEWBOT_RUNS_ON fallbacks differ, so the preflight could approve another value: ${JSON.stringify(fallbacks)}`);
  const fallback = JSON.parse([...values][0]);
  assert.ok(approvedRunsOn.some(set => canonicalLabels(set) === canonicalLabels(fallback)),
    'the REVIEWBOT_RUNS_ON fallback must be an approved label set');
});
test('the ReviewBot preflight enforces the same allowlist the suite reads', () => {
  const preflight = job.preflight.validate;
  const run = script(preflight, 'Reject a runner override outside the approved label sets');
  const execute = raw => spawnSync('bash', ['-c', run], {encoding: 'utf8',
    env: {...process.env, APPROVED_REVIEWBOT_RUNS_ON: JSON.stringify(approvedRunsOn),
      CONFIGURED_REVIEWBOT_RUNS_ON: raw}});
  for (const set of approvedRunsOn) {
    const result = execute(JSON.stringify(set));
    assert.equal(result.status, 0, `${JSON.stringify(set)}: ${result.stderr}`);
  }
  // Label order inside a set does not matter.
  assert.equal(execute(JSON.stringify([...approvedRunsOn[0]].reverse())).status, 0);
  for (const raw of [JSON.stringify(['self-hosted', 'Linux', 'X64']),
    JSON.stringify(['self-hosted', 'Linux', 'X64', 'aliyun-ecs']),
    JSON.stringify(['self-hosted', 'macOS', 'ARM64']), 'not-json', '{}', '[]', '["self-hosted",1]']) {
    const result = execute(raw);
    assert.notEqual(result.status, 0, raw);
    assert.match(result.stderr, /REVIEWBOT_RUNS_ON/, raw);
  }
  // The preflight is hosted, unprivileged and never reads repository content.
  assert.deepEqual(route(preflight), 'ubuntu-latest');
  assert.match(sources.preflight, /^permissions: \{\}$/m);
  assert.match(preflight, /^    permissions: \{\}$/m);
  assert.doesNotMatch(sources.preflight, /secrets\.|actions\/|checkout/,
    'the preflight must not receive secrets or read repository content');
  // The step derives every approved set from the parsed constant.
  assert.doesNotMatch(run, /self-hosted|speedle-runner|reviewbot/);
  assert.match(preflight,
    /CONFIGURED_REVIEWBOT_RUNS_ON: \$\{\{ vars\.REVIEWBOT_RUNS_ON \|\| '\["self-hosted","speedle-runner"\]' \}\}/);
});
test('both identity guards accept exactly the hosts a scheduling label can select', () => {
  const expected = scheduledHosts();
  assert.ok(expected.length >= 3, `the workflows must route to registered hosts, saw ${expected.join(', ')}`);
  assert.deepEqual(acceptedIdentities(script(job.worker.execute, 'Verify self-hosted runner identity')), expected);
  assert.deepEqual(acceptedIdentities(script(job.review['diff-regression'], 'Verify runner identity and tools')), expected);
});
test('the runner inventory covers every scheduling label the workflows declare', () => {
  const routes = declaredRoutes();
  assert.ok(routes.length >= 6, `the workflow scan must find the runner routes, saw ${routes.length}`);
  for (const label of ['fiona-speedle', 'speedle-linux', 'speedle-macos', 'speedle-runner']) {
    assert.ok(routingLabels.has(label), `the workflow scan must find the ${label} route`);
  }
  for (const {workflow, labels: routed} of routes) {
    assert.ok(runnersFor(routed).length > 0,
      `${workflow} routes to ${routed.join(', ')}, which no registered runner carries; `
      + 'refresh runnerInventory from the runner API commands above');
  }
});
test('no declared workflow route can select a runner its identity guard rejects', () => {
  const guarded = scheduledHosts();
  for (const {workflow, labels: routed} of declaredRoutes()) {
    const candidates = runnersFor(routed);
    assert.ok(candidates.length > 0,
      `${workflow} routes to ${routed.join(', ')}, which no registered runner carries; `
      + 'refresh runnerInventory from the runner API commands above');
    for (const runner of candidates) {
      assert.ok(guarded.includes(`${runner.name}/${runner.os}/${runner.arch}`),
        `${workflow} can select ${runner.name}, which no workflow identity guard accepts`);
      if (workflow === 'mongodb-nightly-validation.yml') {
        assert.ok(['Linux/X64', 'macOS/ARM64'].includes(`${runner.os}/${runner.arch}`),
          `${workflow} can select ${runner.name} (${runner.os}/${runner.arch}), which its platform check rejects`);
      }
    }
    // Execute the real worker guard for its declared routes with the least
    // restrictive task; the per-mode tests above prove the weekly and build
    // restrictions, and the preflight guard is compared by identity above
    // because running it would also probe the provider endpoint.
    if (workflow === 'go-ci-worker.yml') assertRoutedToAcceptedRunners('lint', 'declared worker route', routed);
  }
});
test('the retired charming-speedle identity is not declared in workflow configuration', () => {
  for (const file of [...workflowFiles.map(name => path.join('.github/workflows', name)), '.github/actionlint.yaml']) {
    assert.doesNotMatch(fs.readFileSync(path.join(root, file), 'utf8'), /charming-speedle(?!-2)/,
      `${file} must not declare the retired charming-speedle identity`);
  }
});
function normalizedRunner({name, os, labels}) {
  const sorted = [...labels].sort();
  return {name, os, arch: sorted.includes('ARM64') ? 'ARM64' : sorted.includes('X64') ? 'X64' : '', labels: sorted};
}
function assertInventoryMatchesLive(live) {
  const expected = runnerInventory.map(normalizedRunner).sort((a, b) => a.name.localeCompare(b.name));
  const actual = live.map(normalizedRunner).sort((a, b) => a.name.localeCompare(b.name));
  assert.deepEqual(actual, expected,
    'registered runners changed; refresh runnerInventory from the runner API commands above');
}
function liveRunners() {
  const runners = [];
  for (const endpoint of ['orgs/teramoby/actions/runners', 'repos/teramoby/speedle-plus/actions/runners']) {
    const result = spawnSync('gh',
      ['api', '--paginate', endpoint, '--jq', '.runners[] | {name, os, labels: [.labels[].name]}'],
      {encoding: 'utf8'});
    assert.ok(!result.error, `gh is required for the live runner inventory check: ${result.error}`);
    assert.equal(result.status, 0, `gh api ${endpoint} failed: ${String(result.stderr).trim()}`);
    runners.push(...result.stdout.split('\n').filter(Boolean).map(line => JSON.parse(line)));
  }
  return runners;
}
function liveInventoryRequested(env = process.env) {
  return env.SPEEDLE_RUNNERS_LIVE === '1';
}
test('ambient GitHub tokens do not enable the live runner inventory comparison', () => {
  for (const env of [{}, {GH_TOKEN: 'fixture'}, {GITHUB_TOKEN: 'fixture'},
    {GH_TOKEN: 'fixture', GITHUB_TOKEN: 'fixture', SPEEDLE_RUNNERS_LIVE: '0'}]) {
    assert.equal(liveInventoryRequested(env), false);
  }
  assert.equal(liveInventoryRequested({SPEEDLE_RUNNERS_LIVE: '1'}), true);
});
test('the runner inventory matches the live registrations with explicit opt-in', {
  skip: liveInventoryRequested() ? false : 'set SPEEDLE_RUNNERS_LIVE=1 to compare with the runner API through gh',
}, () => {
  assertInventoryMatchesLive(liveRunners());
});
test('the live comparison detects added, removed and relabelled registrations', () => {
  const live = runnerInventory.map(runner => ({name: runner.name, os: runner.os, labels: [...runner.labels]}));
  assertInventoryMatchesLive(live);
  assert.throws(() => assertInventoryMatchesLive(live.filter(runner => runner.name !== 'donkey-speedle')));
  assert.throws(() => assertInventoryMatchesLive(live.map(runner => runner.name === 'fiona'
    ? {...runner, labels: runner.labels.filter(label => label !== 'fiona-speedle')} : runner)));
  assert.throws(() => assertInventoryMatchesLive(live.map(runner => runner.name === 'ecs-light-teramoby'
    ? {...runner, labels: [...runner.labels, 'speedle-runner']} : runner)));
  assert.throws(() => assertInventoryMatchesLive([...live,
    {name: 'charming-speedle', os: 'Linux', labels: ['self-hosted', 'Linux', 'X64', 'speedle-runner']}]));
});
test('the evaluator reads only own properties and opts into workflow inputs explicitly', () => {
  const context = github('workflow_dispatch');
  // Inherited properties must stay invisible to every workflow gate.
  assert.equal(evaluate('github.constructor', context), '');
  assert.equal(evaluate('github.__proto__', context), '');
  // inputs.* is available only to the worker task routes that pass it.
  assert.throws(() => evaluate("inputs.mode == 'weekly'", context), /Unsupported inputs lookup: inputs\.mode/);
  assert.equal(evaluate("inputs.mode == 'weekly'", context, {}, {mode: 'weekly'}), true);
});
test('every merge to master runs the file regression and reports failures from a hosted job', () => {
  const source = sources.postMerge;
  assert.match(source, /^on:\n  push:\n    branches: \[master\]\n  workflow_dispatch:$/m);
  assert.match(source, /^permissions:\n  contents: read$/m);
  assert.doesNotMatch(source, /concurrency:|cancel-in-progress|paths:|paths-ignore:/, 'every merged commit is tested');
  assert.deepEqual(Object.keys(job.postMerge), ['run', 'report']);
  assert.deepEqual(workerInputs(job.postMerge.run, 'postMerge.run'), {mode: 'regression'});
  assert.doesNotMatch(job.postMerge.run, /secrets\./);
  const report = job.postMerge.report;
  assert.equal(route(report), 'ubuntu-latest');
  assert.match(report, /needs: run$/m);
  assert.match(report, /if: always\(\) && github.repository == 'teramoby\/speedle-plus'/);
  assert.match(report, /permissions:\n      contents: read\n      issues: write\n/);
  assert.match(report, /persist-credentials: false/);
  assert.match(report, /name: regression-results/);
  assert.match(report, /ARK_API_KEY: \$\{\{ secrets.ARK_API_KEY \}\}/);
  assert.match(report, /FEISHU_WEBHOOK: \$\{\{ secrets.FEISHU_WEBHOOK \}\}/);
  assert.match(report, /COMMIT_URL: .*\/commit\/\$\{\{ github.sha \}\}/);
  assert.doesNotMatch(report, /SMTP_|MAIL_/, 'no email for the post-merge regression');
  assert.match(report, /scripts\/ci\/test_report.py --kind regression /);
});
test('fixture job installs pinned Node 22 before running Node regression suites', () => {
  const block = job.review['diff-regression'];
  const setup = step(block, 'Set up Node for regression tests');
  assert.match(setup, /uses: actions\/setup-node@[a-f0-9]{40}(?:\s|$)/);
  assert.match(setup, /node-version: '22'/);
  assert.ok(block.indexOf('Set up Node for regression tests') < block.indexOf('node --test'));
  const preflight = script(block, 'Verify runner identity and tools');
  assert.match(preflight, /charming-speedle-2\/Linux\/X64\|donkey-speedle\/Linux\/X64\|fiona\/macOS\/ARM64\) ;;/);
  assert.doesNotMatch(preflight, /charming-speedle\/Linux/,
    'the retired charming-speedle identity must not pass the ReviewBot preflight');
  assert.doesNotMatch(preflight, /fiona-speedle\/macOS/, "the retired fiona-speedle runner name must not pass the identity check");
  assert.match(preflight, /\*\) echo 'Unexpected runner.' >&2; exit 1 ;;/);
  for (const tool of ['git', 'curl', 'jq', 'python3']) assert.ok(preflight.includes(tool));
  assert.ok(preflight.includes('https://ark.cn-beijing.volces.com/api/plan/v3/responses'));
  assert.ok(preflight.includes('https://api.deepseek.com/chat/completions'));
  assert.doesNotMatch(preflight, /Authorization|ARK_API_KEY|DEEPSEEK_API_KEY|--data|--request\s+POST/);
});

test('Fiona trial runs the complete nightly script with host isolation and separate artifacts', () => {
  const full = job.mongodb.mongodb.split('      - name: Full nightly trial on Fiona\n')[1].split('      - name:', 1)[0];
  assert.match(full, /inventory_only != 'true'/);
  assert.match(full, /test "\$\{RUNNER_NAME\}\/\$\{RUNNER_OS\}\/\$\{RUNNER_ARCH\}" = fiona\/macOS\/ARM64/);
  assert.doesNotMatch(full, /fiona-speedle\/macOS/, "the retired fiona-speedle runner name must not pass the identity check");
  assert.match(full, /flock -w 7200 9/);
  assert.match(full, /for port in \(2379, 2380, 27017, 6733, 6734\):/);
  assert.match(full, /bash scripts\/ci\/run-go-tests.sh nightly/);
  assert.match(full, /RESULTS_DIR: .*fiona-nightly-results-.*github.run_id.*github.run_attempt/);
  assert.match(full, /PMS_ADMIN_TOKEN: ''/);
  assert.match(full, /ADS_ADMIN_TOKEN: ''/);
  assert.match(sources.mongodb, /name: fiona-nightly-results/);
  assert.match(sources.mongodb, /run-mysql-tests.py --cleanup/);
});
test('Fiona CI trial runs the worker and ReviewBot commands without touching CI routing', () => {
  assert.match(sources.mongodb, /options: \[none, lint, test, regression, weekly, reviewbot\]/);
  const trial = job.mongodb.mongodb.split('      - name: CI trial on Fiona\n')[1].split('      - name:', 1)[0];
  assert.match(trial, /nightly_trial != 'true' && github.event.inputs.inventory_only != 'true'/);
  assert.match(trial, /test "\$\{RUNNER_NAME\}\/\$\{RUNNER_OS\}\/\$\{RUNNER_ARCH\}" = fiona\/macOS\/ARM64/);
  assert.doesNotMatch(trial, /fiona-speedle\/macOS/, "the retired fiona-speedle runner name must not pass the identity check");
  assert.match(trial, /go vet -composites=false .\/pkg\/\.\.\. .\/cmd\/\.\.\. .\/api\/\.\.\./);
  assert.match(trial, /flock -w 7200 9/);
  assert.match(trial, /test\) bash scripts\/ci\/run-go-tests.sh gate ;;/);
  assert.match(trial, /regression\) bash scripts\/ci\/run-go-tests.sh regression ;;/);
  assert.match(trial, /weekly\) bash scripts\/ci\/run-go-tests.sh weekly ;;/);
  // Suite order can change when the trial gains another regression suite.
  const nodeCommand = trial.match(/^[ \t]*node --test ([^\n]+)$/m);
  assert.ok(nodeCommand, 'Fiona ReviewBot trial must run Node regression suites');
  assert.ok(nodeCommand[1].trim().split(/\s+/).includes('scripts/tests/test_reviewbot_commands.cjs'),
    'Fiona ReviewBot trial must include the command regression suite');
  assert.doesNotMatch(trial, /secrets\.|ARK_API_KEY|Authorization/);
});
