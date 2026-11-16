#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const CI_WORKFLOW_NAME = 'Speedle CI';
const STATUS_CONTEXT = 'speedle/ci-route-gate';
const ROUTE_KEYS = ['go', 'website', 'docs', 'ci'];
// The compare API returns at most 300 file entries. An incomplete list enables
// every route rather than making a decision from only part of the diff.
const COMPARE_FILE_LIMIT = 300;

function allRoutes() {
  return Object.fromEntries(ROUTE_KEYS.map(key => [key, true]));
}

function description(value) {
  return String(value).replace(/\s+/g, ' ').trim().slice(0, 140);
}

function assertSha(value, label) {
  if (typeof value !== 'string' || !/^[0-9a-f]{40}$/i.test(value)) {
    throw new Error(`${label} is not a full commit SHA`);
  }
}

function outcome({run, pr, state, message, routes, ignored = false, error}) {
  const status = {
    state,
    description: description(message),
    context: STATUS_CONTEXT,
  };
  if (run?.html_url) status.target_url = run.html_url;
  return {
    sha: run?.head_sha || pr?.head?.sha,
    run_id: run?.id,
    pr_number: pr?.number,
    ignored,
    status,
    ...(routes ? {routes} : {}),
    ...(error ? {error} : {}),
  };
}

function labelsOf(pr) {
  return new Set((Array.isArray(pr?.labels) ? pr.labels : []).map(label => label?.name).filter(Boolean));
}

function requiredJobs(routes, full) {
  const required = [full ? 'test' : 'quick'];
  if (full && routes.go) required.push('lint');
  if ((full && routes.go) || routes.website || routes.docs) required.push('build');
  return required;
}

function uniqueEntry(entries, name, kind) {
  const names = Array.isArray(name) ? name : [name];
  const matches = entries.filter(entry => names.includes(entry?.name));
  if (matches.length === 0) return {error: `Missing required ${kind}: ${names[0]}`};
  if (matches.length > 1) return {error: `Duplicate required ${kind}: ${names[0]}`};
  return {entry: matches[0]};
}

function passed(entry) {
  return entry?.status === 'completed' && entry?.conclusion === 'success';
}

function ignoredRun(run, jobs) {
  const changes = jobs.find(job => job?.name === 'changes');
  return changes?.conclusion === 'skipped'
    && jobs.some(job => job?.name === `Ignored CI event ${run.id}`);
}

// Validate the run before spending API requests on its diff. An ignored label
// event has no active jobs and must not replace the status from an active run.
function preflight(run, pr, jobs) {
  if (run?.name !== CI_WORKFLOW_NAME) return 'Unexpected CI workflow name';
  if (run.event !== 'pull_request') return 'CI run is not a pull request run';
  if (run.status !== 'completed') return 'CI run is not completed';
  if (run.head_sha !== pr?.head?.sha) return 'CI run does not match the pull request head';
  // A pull request to another branch would let that branch's copy of the
  // classifier decide the routes, so only the default branch is a trusted
  // classification source.
  if (!pr?.base?.ref || pr.base.ref !== pr?.base?.repo?.default_branch) {
    return 'Pull request base is not the default branch';
  }
  if (ignoredRun(run, jobs)) return {ignored: true, message: 'Ignored CI label event'};
  return null;
}

function evaluate({run, pr, jobs, routes}) {
  if (!Array.isArray(jobs)) throw new Error('jobs fixture is not an array');
  const early = preflight(run, pr, jobs);
  if (early) {
    return outcome({
      run, pr, routes,
      state: early.ignored ? null : 'failure',
      message: early.ignored ? early.message : early,
      ignored: Boolean(early.ignored),
    });
  }

  const changes = uniqueEntry(jobs, 'changes', 'route');
  if (changes.error) return outcome({run, pr, routes, state: 'failure', message: changes.error});
  if (!passed(changes.entry)) {
    return outcome({run, pr, routes, state: 'failure', message: 'Required route did not pass: changes'});
  }

  const steps = Array.isArray(changes.entry.steps) ? changes.entry.steps : [];
  const requiredSteps = ['Classify changed files'];
  if (routes.ci) requiredSteps.push('Test CI scripts');
  for (const name of requiredSteps) {
    const step = uniqueEntry(steps, name, 'step');
    if (step.error) return outcome({run, pr, routes, state: 'failure', message: step.error});
    if (!passed(step.entry)) {
      return outcome({run, pr, routes, state: 'failure', message: `Required step did not pass: ${name}`});
    }
  }

  const full = labelsOf(pr).has('full-ci') || labelsOf(pr).has('merge-head');
  for (const name of requiredJobs(routes, full)) {
    // A completed reusable workflow appears as "<caller> / <called job>";
    // a skipped reusable workflow retains only the caller's name.
    const job = uniqueEntry(jobs, [name, `${name} / execute`], 'route');
    if (job.error) return outcome({run, pr, routes, state: 'failure', message: job.error});
    if (!passed(job.entry)) {
      return outcome({run, pr, routes, state: 'failure', message: `Required route did not pass: ${name}`});
    }
  }

  if (run.conclusion !== 'success') {
    return outcome({run, pr, routes, state: 'failure', message: `CI run did not pass: ${run.conclusion || 'unknown'}`});
  }
  if (jobs.some(job => !['success', 'skipped'].includes(job?.conclusion))) {
    return outcome({run, pr, routes, state: 'failure', message: 'CI run contains a job that did not pass'});
  }
  return outcome({
    run, pr, routes, state: 'success',
    message: full ? 'Full CI routes passed' : 'Quick CI routes passed',
  });
}

function parseClassifierOutput(output) {
  const lines = output.split(/\r?\n/);
  const routes = {};
  for (const key of ROUTE_KEYS) {
    const values = lines.filter(line => line.startsWith(`${key}=`)).map(line => line.slice(key.length + 1));
    if (values.length !== 1 || !['true', 'false'].includes(values[0])) return allRoutes();
    routes[key] = values[0] === 'true';
  }
  return routes;
}

function classifyWithBase({api, runner, repository, baseSha, headSha, files}) {
  assertSha(baseSha, 'pull request base');
  assertSha(headSha, 'pull request head');
  const metadata = api(`repos/${repository}/contents/scripts/ci/classify-changes.sh?ref=${baseSha}`);
  if (metadata?.type !== 'file' || metadata.encoding !== 'base64' || typeof metadata.content !== 'string') {
    throw new Error('base classifier is not a regular file with inline content');
  }
  const source = Buffer.from(metadata.content, 'base64');
  if (source.length === 0 || source.length > 1024 * 1024) throw new Error('base classifier has an invalid size');

  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-ci-route-gate-'));
  try {
    const classifier = path.join(temporary, 'classify-changes.sh');
    const changed = path.join(temporary, 'changed-files');
    const output = path.join(temporary, 'routes');
    fs.writeFileSync(classifier, source, {mode: 0o600});
    const chunks = [];
    for (const file of files) {
      if (typeof file !== 'string' || file.length === 0 || file.includes('\0')) {
        throw new Error('compare response contains an invalid path');
      }
      chunks.push(Buffer.from(file), Buffer.from([0]));
    }
    fs.writeFileSync(changed, Buffer.concat(chunks), {mode: 0o600});
    fs.writeFileSync(output, '', {mode: 0o600});
    const result = runner('bash', [classifier], {
      cwd: temporary,
      encoding: 'utf8',
      timeout: 10000,
      maxBuffer: 1024 * 1024,
      env: {
        PATH: process.env.PATH || '/usr/bin:/bin',
        TMPDIR: temporary,
        RUNNER_TEMP: temporary,
        EVENT_NAME: 'pull_request',
        PR_BASE: baseSha,
        PUSH_BEFORE: '',
        GITHUB_SHA: headSha,
        GITHUB_OUTPUT: output,
        CI_CHANGED_FILES_FILE: changed,
      },
    });
    if (result.error) throw result.error;
    if (result.status !== 0) {
      throw new Error(`base classifier exited ${result.status}: ${String(result.stderr || '').trim()}`);
    }
    return parseClassifierOutput(fs.readFileSync(output, 'utf8'));
  } finally {
    fs.rmSync(temporary, {recursive: true, force: true});
  }
}

function changedFiles(compare) {
  if (!compare || !Array.isArray(compare.files)) throw new Error('compare response has no file list');
  const files = compare.files.map(file => file?.filename);
  return {files, complete: files.length < COMPARE_FILE_LIMIT};
}

function jobsFromPages(pages) {
  if (!Array.isArray(pages)) throw new Error('jobs response is not paginated');
  return pages.flatMap(page => {
    if (!Array.isArray(page?.jobs)) throw new Error('jobs response has no job list');
    return page.jobs;
  });
}

function latestRun(api, repository, headSha, completed) {
  assertSha(headSha, 'workflow run head');
  const query = completed ? '&status=completed' : '';
  const response = api(`repos/${repository}/actions/workflows/ci.yml/runs?event=pull_request&head_sha=${headSha}&per_page=1${query}`);
  if (!response || !Array.isArray(response.workflow_runs)) throw new Error('workflow runs response has no run list');
  return response.workflow_runs[0] || null;
}

function assessRun({api, runner, repository, run, pr}) {
  const jobs = jobsFromPages(api(`repos/${repository}/actions/runs/${run.id}/jobs?per_page=100&filter=latest`, {paginate: true}));
  const early = preflight(run, pr, jobs);
  if (early) return evaluate({run, pr, jobs, routes: allRoutes()});

  assertSha(pr?.base?.sha, 'pull request base');
  assertSha(run.head_sha, 'workflow run head');
  const comparison = changedFiles(api(`repos/${repository}/compare/${pr.base.sha}...${run.head_sha}`));
  const routes = comparison.complete ? classifyWithBase({
    api, runner, repository, baseSha: pr.base.sha, headSha: run.head_sha, files: comparison.files,
  }) : allRoutes();
  return evaluate({run, pr, jobs, routes});
}

function execute(options, api, runner = spawnSync) {
  let run;
  let pr;
  try {
    if (options.prNumber) {
      pr = api(`repos/${options.repository}/pulls/${options.prNumber}`);
      run = latestRun(api, options.repository, pr?.head?.sha, true);
      if (!run) {
        return outcome({pr, state: 'failure', message: 'No completed Speedle CI run for the pull request head'});
      }
    } else {
      run = api(`repos/${options.repository}/actions/runs/${options.runId}`);
      const number = run?.pull_requests?.[0]?.number;
      if (!Number.isInteger(number) || number < 1) throw new Error('workflow run has no pull request number');
      pr = api(`repos/${options.repository}/pulls/${number}`);
      const latest = latestRun(api, options.repository, run?.head_sha, false);
      if (!latest) throw new Error('cannot confirm the latest CI run for this head');
      if (String(latest.id) !== String(run.id)) {
        return outcome({
          run, pr, state: null, ignored: true,
          message: latest.name === CI_WORKFLOW_NAME ? 'Superseded by a newer CI run' : 'Newer CI run has an unexpected workflow name',
        });
      }
    }
    return assessRun({api, runner, repository: options.repository, run, pr});
  } catch (error) {
    return outcome({
      run, pr, state: 'failure', message: 'CI route gate could not complete',
      error: error instanceof Error ? error.message : String(error),
    });
  }
}

function publish(api, repository, result) {
  if (result.ignored || !['success', 'failure'].includes(result.status.state)) return;
  assertSha(result.sha, 'status target');
  api(`repos/${repository}/statuses/${result.sha}`, {method: 'POST', input: result.status});
}

function runGate(options, api, runner = spawnSync) {
  const result = execute(options, api, runner);
  if (options.publish) publish(api, options.repository, result);
  return result;
}

function ghApi(endpoint, options = {}) {
  const args = ['api', endpoint];
  if (options.paginate) args.push('--paginate', '--slurp');
  if (options.method) args.push('--method', options.method);
  if (options.input !== undefined) args.push('--input', '-');
  const result = spawnSync('gh', args, {
    encoding: 'utf8',
    input: options.input === undefined ? undefined : JSON.stringify(options.input),
    maxBuffer: 32 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`gh api exited ${result.status}: ${String(result.stderr || '').trim()}`);
  if (!result.stdout.trim()) return null;
  try {
    return JSON.parse(result.stdout);
  } catch {
    throw new Error(`gh api returned invalid JSON for ${endpoint}`);
  }
}

function positiveInteger(value, label) {
  if (!/^[1-9][0-9]*$/.test(value || '')) throw new Error(`${label} must be a positive integer`);
  const number = Number(value);
  if (!Number.isSafeInteger(number)) throw new Error(`${label} is too large`);
  return number;
}

function parseArgs(argv, environment = process.env) {
  const options = {repository: environment.GITHUB_REPOSITORY || 'teramoby/speedle-plus'};
  for (let index = 0; index < argv.length; index++) {
    const argument = argv[index];
    if (argument === '--publish' || argument === '--dry-run' || argument === '--help') {
      options[{ '--publish': 'publish', '--dry-run': 'dryRun', '--help': 'help' }[argument]] = true;
      continue;
    }
    if (!['--run-id', '--pr', '--repo'].includes(argument) || index + 1 >= argv.length) {
      throw new Error(`unknown or incomplete argument: ${argument}`);
    }
    const value = argv[++index];
    if (argument === '--run-id') options.runId = positiveInteger(value, 'run ID');
    if (argument === '--pr') options.prNumber = positiveInteger(value, 'pull request number');
    if (argument === '--repo') options.repository = value;
  }
  if (options.help) return options;
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(options.repository)) throw new Error('invalid repository');
  if (Boolean(options.runId) === Boolean(options.prNumber)) throw new Error('specify exactly one of --run-id or --pr');
  if (options.publish && options.dryRun) throw new Error('--publish and --dry-run are mutually exclusive');
  if (options.prNumber && !options.dryRun) throw new Error('--pr requires --dry-run');
  if (options.runId && !options.publish && !options.dryRun) throw new Error('--run-id requires --publish or --dry-run');
  return options;
}

function main(argv) {
  let options;
  try {
    options = parseArgs(argv);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    console.error('Usage: ci-route-gate.cjs --run-id ID --publish | --pr NUMBER --dry-run [--repo OWNER/REPO]');
    process.exitCode = 1;
    return;
  }
  if (options.help) {
    console.log('Usage: ci-route-gate.cjs --run-id ID --publish | --pr NUMBER --dry-run [--repo OWNER/REPO]');
    return;
  }
  let result;
  try {
    result = runGate(options, ghApi);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
    return;
  }
  console.log(JSON.stringify(result, null, 2));
  if (result.error) {
    console.error(result.error);
    process.exitCode = 1;
  }
}

if (require.main === module) main(process.argv.slice(2));

module.exports = {
  CI_WORKFLOW_NAME,
  STATUS_CONTEXT,
  allRoutes,
  changedFiles,
  classifyWithBase,
  evaluate,
  execute,
  parseArgs,
  parseClassifierOutput,
  publish,
  requiredJobs,
  runGate,
};
