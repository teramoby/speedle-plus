const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const {spawnSync} = require('node:child_process');
const test = require('node:test');

const root = path.resolve(__dirname, '../..');
const gate = require('../ci/ci-route-gate.cjs');
const classifierSource = fs.readFileSync(path.join(root, 'scripts/ci/classify-changes.sh'), 'utf8');
const repository = 'teramoby/speedle-plus';
const base = '1'.repeat(40);
const head = '2'.repeat(40);

function step(name, conclusion = 'success') {
  return {name, status: 'completed', conclusion};
}

function job(name, conclusion = 'success', steps) {
  return {name, status: 'completed', conclusion, ...(steps ? {steps} : {})};
}

function run(overrides = {}) {
  return {
    id: 101,
    name: gate.CI_WORKFLOW_NAME,
    path: gate.CI_WORKFLOW_PATH,
    event: 'pull_request',
    status: 'completed',
    conclusion: 'success',
    head_sha: head,
    html_url: 'https://example.invalid/actions/runs/101',
    pull_requests: [{number: 590}],
    ...overrides,
  };
}

function pullRequest(labels = [], overrides = {}) {
  return {
    number: 590,
    base: {sha: base, ref: 'master', repo: {default_branch: 'master'}},
    head: {sha: head},
    labels: labels.map(name => ({name})),
    ...overrides,
  };
}

// Fixture jobs mirror the names and conclusions exposed by the jobs API. The
// arguments describe what the completed run actually did, not what it should do.
function jobs({full = false, ci = false, build = false, lint = full, test: testRoute = full} = {}) {
  return [
    job('changes', 'success', [
      step('Classify changed files'),
      step('Test CI scripts', ci ? 'success' : 'skipped'),
    ]),
    job(lint ? 'lint / execute' : 'lint', lint ? 'success' : 'skipped'),
    job(testRoute ? 'test / execute' : 'test', testRoute ? 'success' : 'skipped'),
    job(build ? 'build / execute' : 'build', build ? 'success' : 'skipped'),
    job(full ? 'quick' : 'quick / execute', full ? 'skipped' : 'success'),
    job('CI'),
  ];
}

function routesFor(files) {
  return gate.classifyWithBase({
    api: endpoint => {
      assert.equal(endpoint, `repos/${repository}/contents/scripts/ci/classify-changes.sh?ref=${base}`);
      return {type: 'file', encoding: 'base64', content: Buffer.from(classifierSource).toString('base64')};
    },
    runner: spawnSync,
    repository,
    baseSha: base,
    headSha: head,
    files,
  });
}

function fixtureApi({run: completedRun, pr, files, jobs: runJobs, latest, latestCompleted}) {
  const calls = [];
  const api = (endpoint, options = {}) => {
    calls.push({endpoint, options});
    if (options.method === 'POST') {
      assert.match(endpoint, new RegExp(`^repos/${repository}/statuses/${head}$`));
      return {};
    }
    if (endpoint === `repos/${repository}/actions/runs/${completedRun.id}`) return completedRun;
    if (endpoint === `repos/${repository}/pulls/${pr.number}`) return pr;
    if (endpoint === `repos/${repository}/actions/runs/${completedRun.id}/jobs?per_page=100&filter=latest`) {
      assert.equal(options.paginate, true);
      return [{jobs: runJobs}];
    }
    if (endpoint === `repos/${repository}/compare/${base}...${head}`) {
      return {files: files.map(filename => ({filename}))};
    }
    if (endpoint === `repos/${repository}/contents/scripts/ci/classify-changes.sh?ref=${base}`) {
      return {type: 'file', encoding: 'base64', content: Buffer.from(classifierSource).toString('base64')};
    }
    if (endpoint.startsWith(`repos/${repository}/actions/workflows/ci.yml/runs?`)) {
      const completed = endpoint.includes('&status=completed');
      const selected = completed ? (latestCompleted === undefined ? [completedRun] : latestCompleted) : (latest === undefined ? [completedRun] : latest);
      return {workflow_runs: selected};
    }
    throw new Error(`unexpected API request: ${endpoint}`);
  };
  return {api, calls};
}

test('a classifier edit requires every route even when the completed run narrowed them', () => {
  const routes = routesFor(['scripts/ci/classify-changes.sh']);
  assert.deepEqual(routes, {go: true, website: true, docs: true, ci: true});

  const narrowed = gate.evaluate({run: run(), pr: pullRequest(), jobs: jobs(), routes});
  assert.equal(narrowed.status.state, 'failure');
  assert.equal(narrowed.status.description, 'Required step did not pass: Test CI scripts');

  const withoutBuild = gate.evaluate({run: run(), pr: pullRequest(), jobs: jobs({ci: true}), routes});
  assert.equal(withoutBuild.status.state, 'failure');
  assert.equal(withoutBuild.status.description, 'Required route did not pass: build');

  const complete = gate.evaluate({run: run(), pr: pullRequest(), jobs: jobs({ci: true, build: true}), routes});
  assert.equal(complete.status.state, 'success');
});

test('a ci.yml edit that skips the full Go lint route fails the gate', () => {
  const routes = routesFor(['.github/workflows/ci.yml']);
  assert.deepEqual(routes, {go: true, website: false, docs: false, ci: true});
  const pr = pullRequest(['full-ci']);
  const skipped = gate.evaluate({run: run(), pr, jobs: jobs({full: true, ci: true, build: true, lint: false}), routes});
  assert.equal(skipped.status.state, 'failure');
  assert.equal(skipped.status.description, 'Required route did not pass: lint');

  const missingJobs = jobs({full: true, ci: true, build: true}).filter(job => !['lint', 'lint / execute'].includes(job.name));
  const missing = gate.evaluate({run: run(), pr, jobs: missingJobs, routes});
  assert.equal(missing.status.state, 'failure');
  assert.equal(missing.status.description, 'Missing required route: lint');

  const complete = gate.evaluate({run: run(), pr, jobs: jobs({full: true, ci: true, build: true}), routes});
  assert.equal(complete.status.state, 'success');
  assert.equal(complete.status.description, 'Full CI routes passed');
});

test('a renamed workflow is rejected instead of publishing a route success', () => {
  const result = gate.evaluate({
    run: run({name: 'Renamed Speedle CI'}),
    pr: pullRequest(),
    jobs: jobs({build: true}),
    routes: routesFor(['docs/guide.md']),
  });
  assert.equal(result.status.state, 'failure');
  assert.equal(result.status.description, 'Unexpected CI workflow name');
});

test('a pull request based on another branch fails closed without reading a classifier', () => {
  const pr = pullRequest([], {base: {sha: base, ref: 'release', repo: {default_branch: 'master'}}});
  const fixture = fixtureApi({run: run(), pr, files: ['docs/guide.md'], jobs: jobs({build: true})});
  const result = gate.runGate({repository, runId: 101, publish: true}, fixture.api);
  assert.equal(result.status.state, 'failure');
  assert.equal(result.status.description, 'Pull request base is not the default branch');
  assert.equal(fixture.calls.some(call => call.endpoint.includes('/compare/')), false);
  assert.equal(fixture.calls.some(call => call.endpoint.includes('/contents/')), false);
  const writes = fixture.calls.filter(call => call.options.method === 'POST');
  assert.equal(writes.length, 1);
  assert.equal(writes[0].options.input.state, 'failure');
});

test('the gate subscription matches the name and path of the CI workflow', () => {
  const ci = fs.readFileSync(path.join(root, '.github/workflows/ci.yml'), 'utf8');
  const name = /^name: (.+)$/m.exec(ci)?.[1].trim();
  assert.equal(name, gate.CI_WORKFLOW_NAME, 'renaming the CI workflow stops the gate from running');
  assert.equal(gate.CI_WORKFLOW_PATH, '.github/workflows/ci.yml');
  assert.ok(fs.existsSync(path.join(root, gate.CI_WORKFLOW_PATH)), 'the gate must name the CI workflow file');
});

test('the gate job and step names match the CI workflow', () => {
  const ci = fs.readFileSync(path.join(root, '.github/workflows/ci.yml'), 'utf8');
  for (const name of ['changes', 'lint', 'test', 'build', 'quick']) {
    assert.match(ci, new RegExp(`^  ${name}:\\n`, 'm'), `the ${name} job is missing from ci.yml`);
  }
  assert.match(ci, /^    name: changes$/m);
  for (const name of ['Classify changed files', 'Test CI scripts']) {
    assert.match(ci, new RegExp(`^      - name: ${name}$`, 'm'), `the ${name} step is missing from ci.yml`);
  }
  // Ignored label events are detected by this job name.
  assert.match(ci, /format\('Ignored CI event \{0\}', github\.run_id\)/);
  // Reusable jobs are matched as "<caller> / execute"; the called name is
  // defined by the worker workflow pinned at @master.
  const worker = fs.readFileSync(path.join(root, '.github/workflows/go-ci-worker.yml'), 'utf8');
  assert.match(worker, /^  execute:\n    name: execute$/m);
});

test('a pull request that rewrites the CI workflow cannot be verified by its own run', () => {
  // The rewritten workflow can report every job and step name the gate
  // requires, all successful, without running the real jobs.
  const rewritten = fixtureApi({
    run: run(),
    pr: pullRequest(['full-ci']),
    files: ['pkg/eval/eval.go', '.github/workflows/ci.yml'],
    jobs: jobs({full: true, ci: true, build: true}),
  });
  const result = gate.runGate({repository, runId: 101, publish: true}, rewritten.api);
  assert.equal(result.error, undefined);
  assert.equal(result.status.state, 'failure');
  assert.equal(result.status.description,
    'The pull request changes the CI workflow; the route cannot be verified from its own run');
  assert.equal(rewritten.calls.some(call => call.endpoint.endsWith(`/classify-changes.sh?ref=${base}`)), false,
    'an untrusted run must not reach classification');
  const writes = rewritten.calls.filter(call => call.options.method === 'POST');
  assert.equal(writes.length, 1);
  assert.equal(writes[0].options.input.state, 'failure');

  // The compare API returns at most 300 files. An incomplete list cannot prove
  // the workflow file was untouched either.
  const truncated = fixtureApi({
    run: run(),
    pr: pullRequest(),
    files: Array.from({length: 300}, (_, index) => `pkg/f${index}.go`),
    jobs: jobs(),
  });
  const incomplete = gate.runGate({repository, runId: 101, publish: true}, truncated.api);
  assert.equal(incomplete.status.state, 'failure');
  assert.equal(incomplete.status.description,
    'The changed-file list is incomplete; the CI workflow cannot be verified');
});

test('a code-only pull request is still assessed from its base classification', () => {
  const fixture = fixtureApi({
    run: run(),
    pr: pullRequest(['full-ci']),
    files: ['pkg/eval/eval.go'],
    jobs: jobs({full: true, build: true}),
  });
  const result = gate.runGate({repository, runId: 101, publish: true}, fixture.api);
  assert.equal(result.status.state, 'success');
  assert.equal(result.status.description, 'Full CI routes passed');
  assert.ok(fixture.calls.some(call => call.endpoint.endsWith(`/classify-changes.sh?ref=${base}`)));
});

test('PR dry-run reports a rewritten CI workflow without publishing', () => {
  const fixture = fixtureApi({
    run: run(),
    pr: pullRequest(),
    files: ['.github/workflows/ci.yml'],
    jobs: jobs({build: true}),
  });
  const result = gate.runGate({repository, prNumber: 590, dryRun: true}, fixture.api);
  assert.equal(result.status.state, 'failure');
  assert.match(result.status.description, /changes the CI workflow/);
  assert.equal(fixture.calls.filter(call => call.options.method === 'POST').length, 0);
});

test('a run from an unexpected workflow path fails closed', () => {
  const fixture = fixtureApi({
    run: run({path: '.github/workflows/other.yml'}),
    pr: pullRequest(),
    files: ['docs/guide.md'],
    jobs: jobs({build: true}),
  });
  const result = gate.runGate({repository, runId: 101, publish: true}, fixture.api);
  assert.equal(result.status.state, 'failure');
  assert.equal(result.status.description, 'Unexpected CI workflow path');
});

test('a docs-only pull request requires quick and build but no Go lint or CI scripts', () => {
  const routes = routesFor(['docs/guide.md']);
  assert.deepEqual(routes, {go: false, website: false, docs: true, ci: false});
  const result = gate.evaluate({run: run(), pr: pullRequest(), jobs: jobs({build: true}), routes});
  assert.equal(result.status.state, 'success');
  assert.equal(result.status.description, 'Quick CI routes passed');

  const withoutBuild = gate.evaluate({run: run(), pr: pullRequest(), jobs: jobs(), routes});
  assert.equal(withoutBuild.status.state, 'failure');
  assert.equal(withoutBuild.status.description, 'Required route did not pass: build');
});

test('merge-head selects full routes and missing classification steps fail closed', () => {
  const goRoutes = routesFor(['pkg/eval/eval.go']);
  assert.deepEqual(goRoutes, {go: true, website: false, docs: false, ci: false});
  assert.deepEqual(gate.requiredJobs(goRoutes, true), ['test', 'lint', 'build']);
  const pr = pullRequest(['merge-head']);
  const missingLint = gate.evaluate({run: run(), pr, jobs: jobs({full: true, build: true, lint: false}), routes: goRoutes});
  assert.equal(missingLint.status.state, 'failure');
  assert.match(missingLint.status.description, /lint/);

  const changed = jobs({build: true});
  changed[0].steps[0].conclusion = 'skipped';
  const skippedClassify = gate.evaluate({run: run(), pr: pullRequest(), jobs: changed, routes: routesFor(['docs/guide.md'])});
  assert.equal(skippedClassify.status.state, 'failure');
  assert.equal(skippedClassify.status.description, 'Required step did not pass: Classify changed files');
});

test('classifier output contract and incomplete compare responses widen every route', () => {
  assert.deepEqual(gate.parseClassifierOutput('go=false\nwebsite=false\ndocs=true\nci=false\n'),
    {go: false, website: false, docs: true, ci: false});
  for (const output of ['go=false\n', 'go=yes\nwebsite=false\ndocs=true\nci=false\n',
    'go=false\nwebsite=false\ndocs=true\nci=false\ngo=true\n']) {
    assert.deepEqual(gate.parseClassifierOutput(output), gate.allRoutes());
  }
  const comparison = gate.changedFiles({files: Array.from({length: 300}, (_, index) => ({filename: `pkg/f${index}.go`}))});
  assert.equal(comparison.complete, false);
  assert.equal(comparison.files.length, 300);
});

test('publishing evaluates API fixtures with the base classifier and writes the stable status', () => {
  const completedRun = run();
  const pr = pullRequest();
  const {api, calls} = fixtureApi({run: completedRun, pr, files: ['docs/guide.md'], jobs: jobs({build: true})});
  const result = gate.runGate({repository, runId: 101, publish: true}, api);
  assert.equal(result.error, undefined);
  assert.equal(result.status.state, 'success');
  assert.deepEqual(result.routes, {go: false, website: false, docs: true, ci: false});
  const writes = calls.filter(call => call.options.method === 'POST');
  assert.equal(writes.length, 1);
  assert.deepEqual(writes[0].options.input, {
    state: 'success',
    description: 'Quick CI routes passed',
    context: 'speedle/ci-route-gate',
    target_url: completedRun.html_url,
  });
  assert.ok(calls.some(call => call.endpoint.endsWith(`/classify-changes.sh?ref=${base}`)));
  assert.ok(calls.some(call => call.endpoint.endsWith(`/compare/${base}...${head}`)));
});

test('PR dry-run evaluates the latest completed run without writing a status', () => {
  const completedRun = run();
  const pr = pullRequest();
  const {api, calls} = fixtureApi({run: completedRun, pr, files: ['docs/guide.md'], jobs: jobs({build: true})});
  const result = gate.runGate({repository, prNumber: 590, dryRun: true}, api);
  assert.equal(result.status.state, 'success');
  assert.equal(calls.filter(call => call.options.method === 'POST').length, 0);
  assert.ok(calls.some(call => call.endpoint.includes('&status=completed')));
});

test('ignored label events and superseded runs do not replace an existing status', () => {
  const pr = pullRequest();
  const ignoredJobs = [
    job('changes', 'skipped', []),
    job('quick', 'skipped'),
    job('lint', 'skipped'),
    job('test', 'skipped'),
    job('build', 'skipped'),
    job('Ignored CI event 101', 'skipped'),
  ];
  const ignoredFixture = fixtureApi({run: run(), pr, files: [], jobs: ignoredJobs});
  const ignored = gate.runGate({repository, runId: 101, publish: true}, ignoredFixture.api);
  assert.equal(ignored.ignored, true);
  assert.equal(ignored.status.state, null);
  assert.equal(ignoredFixture.calls.filter(call => call.options.method === 'POST').length, 0);
  assert.equal(ignoredFixture.calls.some(call => call.endpoint.includes('/compare/')), false);

  const supersededFixture = fixtureApi({run: run(), pr, files: [], jobs: jobs(), latest: [run({id: 102})]});
  const superseded = gate.runGate({repository, runId: 101, publish: true}, supersededFixture.api);
  assert.equal(superseded.ignored, true);
  assert.equal(superseded.status.description, 'Superseded by a newer CI run');
  assert.equal(supersededFixture.calls.filter(call => call.options.method === 'POST').length, 0);
});

test('dry-run wrapper forwards exactly one PR number and never enables publishing', () => {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-route-gate-dry-run-'));
  try {
    const record = path.join(temporary, 'arguments.json');
    const node = path.join(temporary, 'node');
    fs.writeFileSync(node, `#!${process.execPath}\nrequire('node:fs').writeFileSync(process.env.RECORD, JSON.stringify(process.argv.slice(2)));\n`, {mode: 0o755});
    const wrapper = path.join(root, 'scripts/ci/ci-route-gate-dry-run.sh');
    assert.ok(fs.statSync(wrapper).mode & 0o111, 'dry-run wrapper must be executable');
    const result = spawnSync('bash', [wrapper, '590'], {
      encoding: 'utf8',
      env: {...process.env, PATH: temporary + path.delimiter + process.env.PATH, RECORD: record},
    });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(JSON.parse(fs.readFileSync(record, 'utf8')), [
      path.join(root, 'scripts/ci/ci-route-gate.cjs'), '--pr', '590', '--dry-run',
    ]);
    const usage = spawnSync('bash', [wrapper], {encoding: 'utf8'});
    assert.equal(usage.status, 2);
    assert.match(usage.stderr, /Usage: .*PR_NUMBER/);
  } finally {
    fs.rmSync(temporary, {recursive: true, force: true});
  }
});

test('workflow_run gate uses the default branch, Node 22 and minimal permissions', () => {
  const workflow = fs.readFileSync(path.join(root, '.github/workflows/ci-route-gate.yml'), 'utf8');
  assert.match(workflow, /^  workflow_run:\n    workflows: \[Speedle CI\]\n    types: \[completed\]$/m);
  const permissions = workflow.match(/^permissions:\n((?:  [a-z-]+: (?:read|write)\n)+)/m)?.[1];
  assert.equal(permissions, '  contents: read\n  statuses: write\n  actions: read\n');
  assert.match(workflow, /^    if: github\.event\.workflow_run\.event == 'pull_request'$/m);
  assert.match(workflow, /^  cancel-in-progress: false$/m);

  const checkout = workflow.split('      - name: Checkout default branch\n')[1].split('      - name:')[0];
  assert.match(checkout, /ref: \$\{\{ github\.event\.repository\.default_branch \}\}/);
  assert.match(checkout, /persist-credentials: false/);
  assert.doesNotMatch(checkout, /workflow_run\.head|pull_request/);
  assert.match(workflow, /node-version: '22'/);
  assert.match(workflow, /WORKFLOW_RUN_ID: \$\{\{ github\.event\.workflow_run\.id \}\}/);
  assert.match(workflow, /run: node scripts\/ci\/ci-route-gate\.cjs --run-id "\$\{WORKFLOW_RUN_ID\}" --publish/);
  assert.match(workflow, /GH_TOKEN: \$\{\{ github\.token \}\}/);
  assert.doesNotMatch(workflow, /pull_request_target|secrets\.|actions\/upload-artifact/);
  for (const match of workflow.matchAll(/^        uses: ([^\s]+)/gm)) {
    assert.match(match[1], /^[^@]+@[0-9a-f]{40}$/);
  }
});

test('command arguments require one explicit input and keep PR mode read-only', () => {
  assert.deepEqual(gate.parseArgs(['--run-id', '101', '--publish'], {GITHUB_REPOSITORY: repository}),
    {repository, runId: 101, publish: true});
  assert.deepEqual(gate.parseArgs(['--pr', '590', '--dry-run'], {GITHUB_REPOSITORY: repository}),
    {repository, prNumber: 590, dryRun: true});
  for (const args of [['--pr', '590'], ['--run-id', '101'], ['--pr', '0', '--dry-run'],
    ['--pr', '590', '--dry-run', '--publish'], ['--run-id', '1', '--pr', '2', '--dry-run']]) {
    assert.throws(() => gate.parseArgs(args, {GITHUB_REPOSITORY: repository}));
  }
});
