#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const CI_WORKFLOW_NAME = 'Speedle CI';
const TRUSTED_REPOSITORY = 'teramoby/speedle-plus';
// The workflow files whose job and step names this gate verifies. A
// pull_request run executes them from the request's merge commit when the
// caller references them locally, so the names are only trustworthy when the
// request does not change any of these files.
const CI_WORKFLOW_PATH = '.github/workflows/ci.yml';
const GUARDED_WORKFLOW_PATHS = [CI_WORKFLOW_PATH, '.github/workflows/go-ci-worker.yml'];
const STATUS_CONTEXT = 'speedle/ci-route-gate';
// The newest runs for one head that the supersession check inspects. A newer
// run that never publishes a status must not suppress one that does.
const RUN_SAMPLE_LIMIT = 100;
const UNRESOLVED_RUN_ORDER = Symbol('unresolved run order');
const UNKNOWN_OWNERSHIP = Symbol('unknown run ownership');
// The pull request files endpoint pages through at most 3,000 entries.
const PR_FILE_PAGES = 30;
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

// The run's own job set records which mode it executed: a full run contains a
// test job, a quick run leaves it skipped. Labels can change between the run
// and this gate, so the live labels cannot select the required routes.
function runMode(jobs) {
  const test = uniqueEntry(jobs, ['test', 'test / execute'], 'route');
  if (test.error) return {error: test.error};
  return {full: test.entry?.conclusion !== 'skipped'};
}

function ignoredRun(run, jobs) {
  const changes = jobs.find(job => job?.name === 'changes');
  return changes?.conclusion === 'skipped'
    && jobs.some(job => job?.name === `Ignored CI event ${run.id}`);
}

// Validate the run before using its job set. An ignored label event has no
// active jobs and must not replace the status from an active run; its shape is
// checked against the guarded workflow files before it is honored.
function preflight(run, pr) {
  if (run?.name !== CI_WORKFLOW_NAME) return 'Unexpected CI workflow name';
  if (run.path !== CI_WORKFLOW_PATH) return 'Unexpected CI workflow path';
  if (run.event !== 'pull_request') return 'CI run is not a pull request run';
  if (run.status !== 'completed') return 'CI run is not completed';
  // A run for another commit, or for a pull request based on another branch,
  // has no verdict for this pull request. Statuses are keyed by SHA, so
  // publishing one could overwrite the result of a different pull request
  // that shares the commit.
  if (run.head_sha !== pr?.head?.sha) return {ignored: true, message: 'CI run does not match the pull request head'};
  // A pull request to another branch would let that branch's copy of the
  // classifier decide the routes, so only the default branch is a trusted
  // classification source. The failure replaces any earlier success on the
  // commit, because that success was computed for another base.
  if (!pr?.base?.ref || pr.base.ref !== pr?.base?.repo?.default_branch) {
    return `base is ${pr?.base?.ref || 'unknown'}; the route gate verifies only ${pr?.base?.repo?.default_branch || 'the default branch'}`;
  }
  return null;
}

function evaluate({run, pr, jobs, routes}) {
  if (!Array.isArray(jobs)) throw new Error('jobs fixture is not an array');
  const early = preflight(run, pr);
  if (early) {
    return outcome({
      run, pr, routes,
      state: early.ignored ? null : 'failure',
      message: early.ignored ? early.message : early,
      ignored: Boolean(early.ignored),
    });
  }
  // An ignored label event leaves any existing status alone. It is honoured
  // only after assessRun has checked the guarded workflow files, because the
  // shape comes from the run and the pull request controls the workflow.
  if (ignoredRun(run, jobs)) {
    return outcome({run, pr, routes, state: null, ignored: true, message: 'Ignored CI label event'});
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

  const mode = runMode(jobs);
  if (mode.error) return outcome({run, pr, routes, state: 'failure', message: mode.error});
  const full = mode.full;
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
  const previous = compare.files.map(file => file?.previous_filename).filter(name => typeof name === 'string');
  return {files, previous, complete: files.length < COMPARE_FILE_LIMIT};
}

function guardPaths(files) {
  return files.some(file => GUARDED_WORKFLOW_PATHS.includes(file?.filename) || GUARDED_WORKFLOW_PATHS.includes(file?.previous_filename));
}

// A pull_request run executes these workflow files from the request's merge
// commit, so their job and step names are only trustworthy when the request
// does not change any of them. The compare API reports the request's own side
// of the change since the merge base, so a base-branch update does not count.
// The worker is guarded even though callers pin it to @master: a local
// reference would execute the request's copy. When the compare list is
// capped, the pull request files endpoint can still prove the files are
// untouched without rejecting a large pull request.
function workflowTrust(api, repository, prNumber, comparison) {
  const message = 'The pull request changes the CI workflow; the route cannot be verified from its own run';
  if (comparison.complete) {
    const changed = [...comparison.files, ...comparison.previous];
    return changed.some(name => GUARDED_WORKFLOW_PATHS.includes(name)) ? message : null;
  }
  for (let page = 1; page <= PR_FILE_PAGES; page++) {
    const files = api(`repos/${repository}/pulls/${prNumber}/files?per_page=100&page=${page}`);
    if (!Array.isArray(files)) throw new Error('pull request files response is not an array');
    if (guardPaths(files)) return message;
    if (files.length < 100) return null;
  }
  throw new Error('pull request file list is too large to verify the guarded workflows');
}

function jobsFromPages(pages) {
  if (!Array.isArray(pages)) throw new Error('jobs response is not paginated');
  return pages.flatMap(page => {
    if (!Array.isArray(page?.jobs)) throw new Error('jobs response has no job list');
    return page.jobs;
  });
}

function latestCompletedRun(api, repository, headSha) {
  assertSha(headSha, 'workflow run head');
  const response = api(`repos/${repository}/actions/workflows/ci.yml/runs?event=pull_request&head_sha=${headSha}&per_page=1&status=completed`);
  if (!response || !Array.isArray(response.workflow_runs)) throw new Error('workflow runs response has no run list');
  return response.workflow_runs[0] || null;
}

function associatedNumbers(run) {
  const associated = Array.isArray(run?.pull_requests) ? run.pull_requests : [];
  return associated.map(pr => pr?.number).filter(number => Number.isInteger(number) && number > 0);
}

function openPullRequests(api, repository, run) {
  assertSha(run?.head_sha, 'workflow run head');
  const response = api(`repos/${repository}/commits/${run.head_sha}/pulls`);
  if (!Array.isArray(response)) throw new Error('pull request lookup response is not an array');
  return response.filter(pr => pr?.state === 'open' && pr?.head?.sha === run.head_sha);
}

// Rule 1: ownership always comes from the commit lookup. More than one open
// pull request for the commit cannot be represented by one SHA status. The
// run's association selects the owner among them; a fork run with no
// association uses the unique open pull request. A run whose pull request is
// closed or has moved on leaves the status alone.
function resolveOwnership(api, repository, run) {
  const open = openPullRequests(api, repository, run);
  if (open.length > 1) {
    return {failure: `${open.length} open pull requests share this head; one SHA status cannot represent them`};
  }
  const numbers = associatedNumbers(run);
  if (numbers.length > 0) {
    const owned = open.filter(pr => numbers.includes(pr?.number));
    if (owned.length !== 1) return {ignored: 'The run pull request is closed or has moved on'};
    return {pr: owned[0]};
  }
  if (open.length === 0) return {ignored: 'Cannot resolve one open pull request for this commit'};
  return {pr: open[0]};
}

// Rule 3: a newer run is provably this pull request's, provably another
// pull request's, or unknown. Unknown ownership must never be treated as
// "publish nothing".
function candidateOwnership(api, repository, candidate, number) {
  const numbers = associatedNumbers(candidate);
  if (numbers.length > 0) return numbers.includes(number) ? 'same' : 'other';
  const open = openPullRequests(api, repository, candidate);
  if (open.length === 1) return open[0].number === number ? 'same' : 'other';
  return 'unknown';
}

// A completed ignored label run never publishes a status, so it must not
// suppress the status of the newest run that can. The runs list is ordered by
// creation, newest first, so the first newer run that is not ignored decides.
// Another pull request's run cannot publish here and is skipped. An unknown
// owner is reported so the caller can fail this run's success closed.
function supersedingRun(api, repository, run, pr) {
  assertSha(run?.head_sha, 'workflow run head');
  const response = api(`repos/${repository}/actions/workflows/ci.yml/runs?event=pull_request&head_sha=${run.head_sha}&per_page=${RUN_SAMPLE_LIMIT}`);
  if (!response || !Array.isArray(response.workflow_runs)) throw new Error('workflow runs response has no run list');
  const runs = response.workflow_runs;
  let unknown = false;
  for (const candidate of runs) {
    if (String(candidate?.id) === String(run.id)) return unknown ? UNKNOWN_OWNERSHIP : null;
    const ownership = candidateOwnership(api, repository, candidate, pr.number);
    if (ownership === 'other') continue;
    if (ownership === 'unknown') {
      unknown = true;
      continue;
    }
    const jobs = jobsFromPages(api(`repos/${repository}/actions/runs/${candidate.id}/jobs?per_page=100&filter=latest`, {paginate: true}));
    // The ignored shape is only meaningful from the guarded workflow file.
    // supersedingRun runs only after workflowTrust passed for this commit.
    if (candidate.name === CI_WORKFLOW_NAME && candidate.path === CI_WORKFLOW_PATH && ignoredRun(candidate, jobs)) continue;
    return {run: candidate};
  }
  // Without this run in the sample, newer runs between the sample and this one
  // may exist, so its place in the order cannot be established.
  return UNRESOLVED_RUN_ORDER;
}

function assessRun({api, runner, repository, run, pr, comparison, trusted = false}) {
  const jobs = jobsFromPages(api(`repos/${repository}/actions/runs/${run.id}/jobs?per_page=100&filter=latest`, {paginate: true}));
  const early = preflight(run, pr);
  if (early) return evaluate({run, pr, jobs, routes: allRoutes()});

  assertSha(pr?.base?.sha, 'pull request base');
  assertSha(run.head_sha, 'workflow run head');
  const compared = comparison || changedFiles(api(`repos/${repository}/compare/${pr.base.sha}...${run.head_sha}`));
  // Workflow integrity comes before every run-shape decision: never read job
  // or step names, including an ignored label-event shape, from a run whose
  // workflow the pull request controls.
  const untrusted = trusted ? null : workflowTrust(api, repository, pr.number, compared);
  if (untrusted) return outcome({run, pr, routes: allRoutes(), state: 'failure', message: untrusted});
  const routes = compared.complete ? classifyWithBase({
    api, runner, repository, baseSha: pr.base.sha, headSha: run.head_sha, files: compared.files,
  }) : allRoutes();
  return evaluate({run, pr, jobs, routes});
}

function execute(options, api, runner = spawnSync) {
  let run;
  let pr;
  try {
    if (options.prNumber) {
      pr = api(`repos/${options.repository}/pulls/${options.prNumber}`);
      run = latestCompletedRun(api, options.repository, pr?.head?.sha);
      if (!run) {
        return outcome({pr, state: 'failure', message: 'No completed Speedle CI run for the pull request head'});
      }
    } else {
      run = api(`repos/${options.repository}/actions/runs/${options.runId}`);
      // A re-run creates a newer attempt of the same run. The attempted event
      // is passed by the workflow, so an event for an older attempt is skipped
      // instead of publishing from a superseded execution.
      if (options.runAttempt !== undefined) {
        const latestAttempt = Number(run?.run_attempt);
        if (!Number.isInteger(latestAttempt) || latestAttempt < options.runAttempt) {
          return outcome({run, state: 'failure', message: 'Workflow run attempt cannot be verified'});
        }
        if (latestAttempt > options.runAttempt) {
          return outcome({run, state: null, ignored: true, message: 'Superseded by a newer CI attempt'});
        }
      }
      const ownership = resolveOwnership(api, options.repository, run);
      if (ownership.failure) return outcome({run, state: 'failure', message: ownership.failure});
      if (ownership.ignored) return outcome({run, state: null, ignored: true, message: ownership.ignored});
      pr = ownership.pr;
      // Rule 2: a base other than the default branch publishes failure so an
      // earlier success for another base cannot stand. A head mismatch is
      // left alone.
      const early = preflight(run, pr);
      if (early) {
        return outcome({
          run, pr, routes: allRoutes(),
          state: early.ignored ? null : 'failure',
          message: early.ignored ? early.message : early,
          ignored: Boolean(early.ignored),
        });
      }
      // Rule 1: the guarded workflow files are a property of the head commit,
      // so this verdict holds for every run of it and is published before any
      // supersession or ignored-run decision.
      assertSha(pr?.base?.sha, 'pull request base');
      assertSha(run.head_sha, 'workflow run head');
      const comparison = changedFiles(api(`repos/${options.repository}/compare/${pr.base.sha}...${run.head_sha}`));
      const untrusted = workflowTrust(api, options.repository, pr.number, comparison);
      if (untrusted) return outcome({run, pr, routes: allRoutes(), state: 'failure', message: untrusted});
      const superseding = supersedingRun(api, options.repository, run, pr);
      if (superseding === UNRESOLVED_RUN_ORDER) {
        return outcome({run, pr, routes: allRoutes(), state: 'failure', message: 'Run order for this head is unresolved'});
      }
      if (superseding === UNKNOWN_OWNERSHIP) {
        // Rule 3: an unknown newer owner never suppresses a failure, and a
        // success becomes a failure because the newest result is unknown.
        const result = assessRun({api, runner, repository: options.repository, run, pr, comparison, trusted: true});
        if (result.status.state === 'success') {
          result.status.state = 'failure';
          result.status.description = 'a newer run on this head has unknown ownership';
        }
        return result;
      }
      if (superseding) {
        return outcome({
          run, pr, state: null, ignored: true,
          message: superseding.run.name === CI_WORKFLOW_NAME ? 'Superseded by a newer CI run' : 'Newer CI run has an unexpected workflow name',
        });
      }
      return assessRun({api, runner, repository: options.repository, run, pr, comparison, trusted: true});
    }
    return assessRun({api, runner, repository: options.repository, run, pr});
  } catch (error) {
    return outcome({
      run, pr, state: 'failure', message: 'CI route gate could not complete',
      error: error instanceof Error ? error.message : String(error),
    });
  }
}

// An incomplete evaluation is not a route verdict. Leave the status alone so a
// transport or API failure cannot replace the result of a completed run; the
// workflow step still exits non-zero and the run is visible.
function publish(api, repository, result) {
  if (result.ignored || result.error || !['success', 'failure'].includes(result.status.state)) return;
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
  const options = {repository: environment.GITHUB_REPOSITORY || TRUSTED_REPOSITORY};
  if (environment.WORKFLOW_RUN_ATTEMPT) {
    options.runAttempt = positiveInteger(environment.WORKFLOW_RUN_ATTEMPT, 'workflow run attempt');
  }
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
  // The read-only evaluation must not take its classifier from another
  // repository, which would make its verdict meaningless.
  if (options.prNumber && options.repository !== TRUSTED_REPOSITORY) {
    throw new Error(`--pr dry-run only supports ${TRUSTED_REPOSITORY}`);
  }
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
  CI_WORKFLOW_PATH,
  GUARDED_WORKFLOW_PATHS,
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
