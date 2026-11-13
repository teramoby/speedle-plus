/* The publish jobs must read a bound artifact and must not widen permissions. */
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;

const root = path.resolve(__dirname, '../..');
const reviewWorkflow = fs.readFileSync(path.join(root, '.github/workflows/reviewbot.yml'), 'utf8');
const commandWorkflow = fs.readFileSync(path.join(root, '.github/workflows/reviewbot-command.yml'), 'utf8');
const inlineWorkflow = fs.readFileSync(path.join(root, '.github/workflows/reviewbot-inline.yml'), 'utf8');
const artifactName = 'name: reviewbot-report-${{ github.run_id }}-${{ github.run_attempt }}';
const head = 'b'.repeat(40);

function scriptFor(workflow, stepName) {
  const start = workflow.indexOf(`      - name: ${stepName}\n`);
  assert.notEqual(start, -1, stepName);
  const marker = '          script: |\n';
  const scriptStart = workflow.indexOf(marker, start);
  assert.notEqual(scriptStart, -1, stepName);
  const lines = [];
  for (const line of workflow.slice(scriptStart + marker.length).split('\n')) {
    if (line && !line.startsWith('            ')) break;
    lines.push(line.slice(12));
  }
  return lines.join('\n');
}

test('review text is an artifact, and workflow permissions stay as they were', () => {
  for (const workflow of [reviewWorkflow, commandWorkflow]) {
    assert.equal((workflow.match(/final_message/g) || []).length, 0);
    assert.equal((workflow.match(/model_id_b64/g) || []).length, 0);
    assert.equal((workflow.match(/toJSON\(secrets\)/g) || []).length, 1);
    assert.match(workflow, /toJSON\(secrets\) \}\} # zizmor: ignore\[overprovisioned-secrets\]/);
    assert.equal((workflow.match(new RegExp(artifactName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g')) || []).length, 2);
    assert.match(workflow, /retention-days: 1/);
    assert.match(workflow, /if-no-files-found: error/);
    assert.match(workflow, /cd "\$\{RUNNER_TEMP\}"/);
    assert.match(workflow, /git -C "\$\{GITHUB_WORKSPACE\}" show "\$\{TRUSTED_SHA\}:\.github\/codex\/reviewbot_handoff\.py"/);
    assert.match(workflow, /python3 -I "\$\{handoff_guard\}"/);
    assert.match(workflow, /id: handoff\n/);
    assert.match(workflow, /rm -rf -- "\$\{RUNNER_TEMP\}\/reviewbot-handoff"/);
    assert.match(workflow, /rm -f -- "\$\{RUNNER_TEMP\}\/reviewbot-handoff\.py"/);
    assert.doesNotMatch(workflow, /actions: write/);
    const seal = workflow.indexOf('name: Validate and seal review handoff');
    const upload = workflow.indexOf('name: Upload validated review handoff');
    const download = workflow.indexOf('name: Download validated review handoff from this run only');
    assert.ok(seal > 0 && seal < upload && upload < download);
  }
  assert.match(reviewWorkflow, /^permissions:\n  contents: read\n/m);
  assert.match(reviewWorkflow, / {4}permissions:\n {6}contents: read\n {6}pull-requests: read\n/);
  assert.match(reviewWorkflow, / {4}permissions:\n {6}pull-requests: write\n/);
  assert.match(reviewWorkflow, / {4}permissions:\n {6}actions: read\n {6}contents: write\n {6}pull-requests: write\n/);
  assert.match(reviewWorkflow, /if: always\(\) && !cancelled\(\) && needs\.review\.result != 'skipped'/);
  assert.match(reviewWorkflow, /steps\.handoff\.outcome == 'success'/);
  assert.match(commandWorkflow, /^permissions:\n  contents: read\n/m);
  assert.match(commandWorkflow, / {4}permissions:\n {6}contents: read\n {6}issues: write\n {6}pull-requests: write\n/);
  assert.match(commandWorkflow, / {4}permissions:\n {6}contents: read\n {6}issues: read\n {6}pull-requests: read\n/);
  assert.match(commandWorkflow, / {4}permissions:\n {6}issues: write\n {6}pull-requests: write\n/);
  assert.match(commandWorkflow, /if: always\(\) && needs\.prepare\.outputs\.should_run == 'true'/);
  assert.match(commandWorkflow, /DOWNLOAD_OUTCOME: \$\{\{ steps\.handoff\.outcome \}\}/);
  assert.match(commandWorkflow, /DOWNLOAD_OUTCOME !== "success"/);
  assert.match(inlineWorkflow, /^permissions: \{\}$/m);
  assert.doesNotMatch(inlineWorkflow, /upload-artifact|download-artifact|toJSON\(secrets\)/);
});

function report(overrides = {}) {
  return {
    version: 1,
    message: '## ReviewBot review\n\nNo actionable findings.',
    model: 'kimi-k3',
    run_id: '999',
    run_attempt: '1',
    pull_number: '70',
    head_sha: head,
    command: 'review',
    ...overrides,
  };
}

function nodeRequire(id) {
  if (id === 'node:fs' || id === 'fs') return fs;
  throw new Error(`unexpected module ${id}`);
}

async function runPublisher(script, {body, raw, link, env = {}} = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'reviewbot-handoff-'));
  const file = path.join(directory, 'reviewbot-handoff', 'report.json');
  const created = [];
  const comments = [];
  const failures = [];
  try {
    if (link) {
      fs.mkdirSync(path.dirname(file));
      fs.symlinkSync(path.join(directory, 'target.json'), file);
      fs.writeFileSync(path.join(directory, 'target.json'), '{}');
    } else if (raw !== undefined) {
      fs.mkdirSync(path.dirname(file));
      fs.writeFileSync(file, raw);
    } else if (body !== undefined) {
      fs.mkdirSync(path.dirname(file));
      fs.writeFileSync(file, JSON.stringify(body));
    }
    const github = {
      rest: {
        pulls: {
          listReviews: Symbol('listReviews'),
          createReview: async (args) => { created.push(args); return {data: {id: 9}}; },
          listReviewComments: Symbol('reviewComments'),
        },
        issues: {
          listComments: Symbol('comments'),
          createComment: async (args) => { comments.push(args); return {data: {id: 11}}; },
        },
      },
      paginate: async () => [],
      request: async () => ({data: {}}),
    };
    const context = {
      repo: {owner: 'teramoby', repo: 'speedle-plus'}, issue: {number: 70},
      serverUrl: 'https://github.com', runId: 999,
    };
    const core = {setFailed: (message) => failures.push(message), notice() {}, warning() {}};
    const processMock = {env: {
      RUNNER_TEMP: directory,
      GITHUB_RUN_ID: '999',
      GITHUB_RUN_ATTEMPT: '1',
      PULL_NUMBER: '70',
      HEAD_SHA: head,
      REVIEWED_COMMIT: head,
      REVIEWED_BASE: 'a'.repeat(40),
      SAFE_TO_MERGE: 'false',
      COMMAND: 'review',
      ANALYZE_RESULT: 'success',
      REVIEWBOT_ENABLED: 'true',
      REVIEWBOT_PROVIDER: 'ark',
      ACTOR: 'maintainer',
      COMMENT_ID: '123',
      REPLY_TO_COMMENT_ID: '100',
      SOURCE_KIND: 'issue_comment',
      SOURCE_URL: 'https://github.com/teramoby/speedle-plus/pull/70#issuecomment-123',
      DOWNLOAD_OUTCOME: 'success',
      ...env,
    }};
    let thrown;
    try {
      await new AsyncFunction('github', 'context', 'core', 'process', 'Buffer', 'require', script)(
        github, context, core, processMock, Buffer, nodeRequire);
    } catch (error) {
      thrown = error;
    }
    return {created, comments, failures, thrown};
  } finally {
    fs.rmSync(directory, {recursive: true, force: true});
  }
}

const publishers = [
  ['automatic', scriptFor(reviewWorkflow, 'Create ReviewBot review for commit')],
  ['command', scriptFor(commandWorkflow, 'Publish idempotent response')],
];

for (const [name, script] of publishers) {
  test(`${name} publisher rejects a missing, oversized, or unbound handoff before any write`, async () => {
    const cases = [
      {},
      {raw: Buffer.concat([Buffer.from('{"message":"'), Buffer.from([0xff]), Buffer.from('"model":"kimi-k3"}')])},
      {raw: 'x'.repeat(262145)},
      {link: true},
      {body: []},
      {body: report({extra: true})},
      {body: report({message: '   '})},
      {body: report({model: 'bad model'})},
      {body: report({model: 'm'.repeat(201)})},
      {body: report({run_id: '1000'})},
      {body: report({run_attempt: '2'})},
      {body: report({pull_number: '71'})},
      {body: report({head_sha: 'c'.repeat(40)})},
      {body: report({command: 'explain'})},
      {body: report(), env: {PULL_NUMBER: '71'}},
      {body: report(), env: {HEAD_SHA: 'c'.repeat(40)}},
    ];
    for (const [index, item] of cases.entries()) {
      const result = await runPublisher(script, item);
      assert.equal(result.created.length, 0, `${name} case ${index} created a review`);
      assert.equal(result.comments.length, 0, `${name} case ${index} created a comment`);
      assert.match(result.thrown?.message || '', /no report published/, `${name} case ${index}`);
    }
  });

  test(`${name} publisher writes one review from a valid artifact`, async () => {
    const result = await runPublisher(script, {body: report()});
    assert.equal(result.thrown, undefined);
    assert.equal(result.created.length, 1);
    assert.equal(result.comments.length, 0);
    assert.equal(result.created[0].commit_id, head);
    assert.match(result.created[0].body, /No actionable findings/);
    assert.match(result.created[0].body, /Model ID: `kimi-k3`/);
    assert.doesNotMatch(JSON.stringify(result.created[0]), /synthetic-|PRIVATE/);
  });
}

function automaticCreateRuns({result = 'success', enabled = 'true', shouldReview = 'true', outcome = 'success'} = {}) {
  const start = reviewWorkflow.indexOf('      - name: Create ReviewBot review for commit\n');
  const line = reviewWorkflow.slice(start).split('\n').find((item) => item.trimStart().startsWith('if:'));
  const expression = line.replace(/^\s*if:\s*/, '');
  const values = {
    'needs.review.result': result,
    'needs.review.outputs.enabled': enabled,
    'needs.review.outputs.should_review': shouldReview,
    'steps.handoff.outcome': outcome,
  };
  const rewritten = expression.replace(
    /needs\.review\.outputs\.enabled|needs\.review\.outputs\.should_review|needs\.review\.result|steps\.handoff\.outcome/g,
    (name) => JSON.stringify(values[name]));
  return new Function(`return (${rewritten});`)();
}

test('automatic publication runs only after the handoff download succeeds', () => {
  assert.equal(automaticCreateRuns(), true);
  for (const outcome of ['failure', 'skipped', 'cancelled']) {
    assert.equal(automaticCreateRuns({outcome}), false, outcome);
  }
  assert.equal(automaticCreateRuns({result: 'failure'}), false);
  assert.equal(automaticCreateRuns({enabled: 'false'}), false);
  assert.equal(automaticCreateRuns({shouldReview: 'false'}), false);
});

test('command publisher does not read a planted handoff when download failed', async () => {
  const script = scriptFor(commandWorkflow, 'Publish idempotent response');
  const result = await runPublisher(script, {body: report(), env: {DOWNLOAD_OUTCOME: 'failure'}});
  assert.equal(result.created.length, 0);
  assert.equal(result.comments.length, 0);
  assert.match(result.thrown?.message || '', /no report published/);
});

test('analysis that does not produce a publishable outcome fails instead of skipping', async () => {
  const automatic = scriptFor(reviewWorkflow, 'Require a completed analysis');
  const command = scriptFor(commandWorkflow, 'Require a completed analysis');
  const failed = await runPublisher(automatic, {env: {ANALYZE_RESULT: 'failure', REVIEWBOT_ENABLED: '', SHOULD_REVIEW: ''}});
  assert.match(failed.failures.join('\n'), /no model text will be published/);
  const disabled = await runPublisher(automatic, {env: {REVIEWBOT_ENABLED: 'false', SHOULD_REVIEW: ''}});
  assert.deepEqual(disabled.failures, []);
  assert.equal(disabled.thrown, undefined);
  const missingFlag = await runPublisher(automatic, {env: {REVIEWBOT_ENABLED: 'true', SHOULD_REVIEW: ''}});
  assert.match(missingFlag.failures.join('\n'), /no model text will be published/);
  const commandFailed = await runPublisher(command, {env: {ANALYZE_RESULT: 'failure'}});
  assert.match(commandFailed.failures.join('\n'), /no model text will be published/);
  const commandDisabled = await runPublisher(command, {env: {REVIEWBOT_ENABLED: 'false'}});
  assert.deepEqual(commandDisabled.failures, []);
});
