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
// The newest started runs for one head that the supersession check inspects.
// A newer run that never publishes a status must not suppress one that does.
const RUN_SAMPLE_LIMIT = 100;
// The runs list pages at 100 entries. Runs that never started do not count
// toward RUN_SAMPLE_LIMIT, so the scan may read further pages; this bounds the
// API budget for a head with a very large number of approval-pending runs.
const RUN_PAGE_SIZE = 100;
const RUN_PAGE_LIMIT = 10;
const UNRESOLVED_RUN_ORDER = Symbol('unresolved run order');
const UNKNOWN_OWNERSHIP = Symbol('unknown run ownership');
// A gate that finds a newer completed run after it published re-evaluates
// that run; past this many rounds the head fails closed.
const PUBLICATION_ROUNDS = 3;
// The workflow job is killed after 10 minutes. Each gh call is bounded, and
// reads stop at the assessment deadline, so the failure path always has time
// to write its status: the last read ends by 7 minutes and the write by 8.
const API_TIMEOUT_MS = 60 * 1000;
const ASSESSMENT_DEADLINE_MS = 6 * 60 * 1000;
// The run and pull request a published verdict belongs to, for the check
// after publication. Kept beside the result so the printed result is unchanged.
const verdictContext = new WeakMap();
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
  if (full && (routes.go || routes.website || routes.docs)) required.push('build');
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

// A run that never started has no route verdict. `action_required` is a
// completed run whose approval was never granted; queued, requested, waiting
// and pending runs have not begun. Such a run neither supersedes another run
// nor publishes a status of its own.
function startedRun(run) {
  if (run?.status !== 'completed' && run?.status !== 'in_progress') return false;
  return run?.conclusion !== 'action_required';
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

function evaluate({run, pr, jobs, routes, incomplete = false}) {
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

  const mode = runMode(jobs);
  if (mode.error) return outcome({run, pr, routes, state: 'failure', message: mode.error});
  const full = mode.full;
  // An incomplete compare enables every route, but a quick run never
  // schedules them. It cannot prove a wider matrix, so require a full run
  // instead of naming a route the workflow does not produce for this change.
  if (incomplete && !full) {
    return outcome({
      run, pr, routes, state: 'failure',
      message: 'changed-file list too large to classify; a full CI run is required',
    });
  }

  const changes = uniqueEntry(jobs, 'changes', 'route');
  if (changes.error) return outcome({run, pr, routes, state: 'failure', message: changes.error});
  if (!passed(changes.entry)) {
    return outcome({run, pr, routes, state: 'failure', message: 'Required route did not pass: changes'});
  }

  const steps = Array.isArray(changes.entry.steps) ? changes.entry.steps : [];
  const requiredSteps = ['Classify changed files'];
  // The CI-script test is route-driven, so a run on code-only changes skips
  // it even when it is full. An incomplete compare cannot decide that route;
  // the widened job requirements below carry the verdict instead.
  if (routes.ci && !incomplete) requiredSteps.push('Test CI scripts');
  for (const name of requiredSteps) {
    const step = uniqueEntry(steps, name, 'step');
    if (step.error) return outcome({run, pr, routes, state: 'failure', message: step.error});
    if (!passed(step.entry)) {
      return outcome({run, pr, routes, state: 'failure', message: `Required step did not pass: ${name}`});
    }
  }

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

function classifyWithBase({api, runner, repository, baseSha, rulesSha = baseSha, headSha, files}) {
  assertSha(baseSha, 'pull request base');
  assertSha(rulesSha, 'classifier revision');
  assertSha(headSha, 'pull request head');
  const metadata = api(`repos/${repository}/contents/scripts/ci/classify-changes.sh?ref=${rulesSha}`);
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

// The classifier always comes from the default branch tip, never from the
// pull request's base commit: the routes are decided by the repository's
// current trusted rules, so every pull request that shares a head classifies it
// the same way, whichever base tip it was opened against.
function defaultBranchTip(api, repository, pr) {
  const branch = pr?.base?.repo?.default_branch;
  if (typeof branch !== 'string' || branch.length === 0) throw new Error('pull request base has no default branch');
  const sha = api(`repos/${repository}/branches/${encodeURIComponent(branch)}`)?.commit?.sha;
  assertSha(sha, 'default branch tip');
  return sha;
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
  const pages = api(`repos/${repository}/commits/${run.head_sha}/pulls?per_page=100`, {paginate: true});
  if (!Array.isArray(pages)) throw new Error('pull request lookup response is not paginated');
  return pages.flatMap(page => {
    if (!Array.isArray(page)) throw new Error('pull request lookup page is not an array');
    return page;
  }).filter(pr => pr?.state === 'open' && pr?.head?.sha === run.head_sha);
}

// The base repository and ref select the classification: the head's file list
// against that base, classified by the default branch's current rules. Two
// pull requests with one base identity therefore produce one verdict. The pull
// request and workflow run shapes describe the repository differently; the
// numeric id is present in both and is the strongest identity.
function baseIdentity(base) {
  if (!base || typeof base.ref !== 'string' || base.ref.length === 0) return null;
  const repository = base.repo;
  if (!repository || typeof repository !== 'object') return null;
  if (repository.id !== undefined && repository.id !== null) return `${repository.id}#${base.ref}`;
  if (typeof repository.full_name === 'string' && repository.full_name.length > 0) {
    return `${repository.full_name}#${base.ref}`;
  }
  if (typeof repository.name === 'string' && repository.name.length > 0) return `${repository.name}#${base.ref}`;
  return null;
}

// Rule 1: ownership comes from the commit lookup. The status is keyed by SHA,
// so every open pull request at the head wears it: when they disagree on the
// base identity, one status cannot represent them and the run fails closed,
// whether or not it is associated with one of them. Otherwise the run's own
// association decides which pull request it belongs to; a run without an
// association that never started publishes nothing.
//
// Availability trade-off, accepted: anyone who can open a pull request can open
// one that reuses another pull request's head on another base. Its presence
// makes the shared head fail until that pull request is closed. The shared
// status is never turned green for a pull request the gate does not certify.
function resolveOwnership(api, repository, run) {
  const open = openPullRequests(api, repository, run);
  const distinctBases = prs => {
    const identities = new Set(prs.map(pr => baseIdentity(pr?.base)));
    return identities.size !== 1 || identities.has(null);
  };
  const numbers = associatedNumbers(run);
  if (numbers.length > 0) {
    const owned = open.filter(pr => numbers.includes(pr?.number));
    if (owned.length === 0) return {ignored: 'The run pull request is closed or has moved on'};
    if (distinctBases(open)) {
      return {failure: `${open.length} open pull requests share this head; one SHA status cannot represent them`};
    }
    return {pr: owned[0]};
  }
  if (!startedRun(run)) return {ignored: 'CI run did not start'};
  if (open.length === 0) return {ignored: 'Cannot resolve one open pull request for this commit'};
  if (distinctBases(open)) {
    return {failure: `${open.length} open pull requests share this head; one SHA status cannot represent them`};
  }
  return {pr: open[0]};
}

// Rule 3: a newer run can publish this head's verdict, provably cannot, or
// its ownership is unknown. The status is keyed by SHA, so a run for any pull
// request with this head and this base identity publishes the same status,
// whatever its pull request number. A run whose pull requests are all on
// another base cannot represent this classification. When a run has no
// association, the open pull requests for its head decide the same way. A
// missing base, or a mix of this base and another, is unknown.
function candidateOwnership(api, repository, candidate, pr) {
  const associated = Array.isArray(candidate?.pull_requests) ? candidate.pull_requests : [];
  const owners = associated.length > 0 ? associated : openPullRequests(api, repository, candidate);
  return ownershipByBase(owners.map(entry => baseIdentity(entry?.base)), baseIdentity(pr?.base));
}

function ownershipByBase(identities, base) {
  if (base === null || identities.length === 0 || identities.includes(null)) return 'unknown';
  if (identities.every(identity => identity === base)) return 'same';
  if (identities.every(identity => identity !== base)) return 'other';
  return 'unknown';
}

// The start of a run's latest attempt, or null when it is not a valid time.
function attemptStart(run) {
  const time = Date.parse(run?.run_started_at);
  return Number.isFinite(time) ? time : null;
}

// Whether candidate's latest attempt started after run's. A re-run keeps its
// ID and its place in the creation-ordered runs list but starts later, so the
// order is by attempt start; equal starts fall back to the newer ID.
function startedAfter(candidate, run, runStart) {
  const start = attemptStart(candidate);
  if (start === null) return null;
  return start > runStart || (start === runStart && Number(candidate.id) > Number(run.id));
}

// A completed ignored label run never publishes a status, so it must not
// suppress the status of the newest run that can: a newer run that is not
// ignored supersedes this one. Every started run of the head is compared,
// because a re-run of an older ID can be the newest. A run that never started
// is invisible here: it cannot publish, so it neither supersedes this run nor
// consumes the sample. Another pull request's run cannot publish here and is
// skipped. An unknown owner is reported so the caller can fail this run's
// success closed.
function supersedingRun(api, repository, run, pr) {
  assertSha(run?.head_sha, 'workflow run head');
  const runStart = attemptStart(run);
  if (runStart === null) return UNRESOLVED_RUN_ORDER;
  let unknown = false;
  let considered = 0;
  let listed = false;
  for (let page = 1; page <= RUN_PAGE_LIMIT; page++) {
    const response = api(`repos/${repository}/actions/workflows/ci.yml/runs?event=pull_request&head_sha=${run.head_sha}&per_page=${RUN_PAGE_SIZE}&page=${page}`);
    if (!response || !Array.isArray(response.workflow_runs)) throw new Error('workflow runs response has no run list');
    const runs = response.workflow_runs;
    for (const candidate of runs) {
      if (String(candidate?.id) === String(run.id)) {
        listed = true;
        continue;
      }
      if (!startedRun(candidate)) continue;
      // More started runs exist than the sample holds, so a newer attempt
      // may be among those that were not read.
      if (considered >= RUN_SAMPLE_LIMIT) return UNRESOLVED_RUN_ORDER;
      considered += 1;
      const newer = startedAfter(candidate, run, runStart);
      if (newer === null) return UNRESOLVED_RUN_ORDER;
      if (!newer) continue;
      const ownership = candidateOwnership(api, repository, candidate, pr);
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
    if (runs.length < RUN_PAGE_SIZE) {
      if (!listed) return UNRESOLVED_RUN_ORDER;
      return unknown ? UNKNOWN_OWNERSHIP : null;
    }
  }
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
  // A pull request from a fork can run only after a maintainer approves it.
  // The approval-pending run never executed, so it has no verdict to publish;
  // publishing a failure would let an unrelated pull request decide the status
  // of a head it shares. This follows the workflow check, which stays first.
  if (!startedRun(run)) return outcome({run, pr, state: null, ignored: true, message: 'CI run did not start'});
  const routes = compared.complete ? classifyWithBase({
    api, runner, repository, baseSha: pr.base.sha, rulesSha: defaultBranchTip(api, repository, pr),
    // A rename changes both paths, so the old path selects routes too.
    headSha: run.head_sha, files: [...new Set([...compared.files, ...compared.previous])],
  }) : allRoutes();
  return evaluate({run, pr, jobs, routes, incomplete: !compared.complete});
}

// Gate executions for different CI runs of one head run in parallel: the
// workflow groups them per CI run, because a per-head group keeps only one
// pending execution and could drop the gate of a newer run. A newer run can
// therefore complete while this one is assessed, so the order is checked again
// just before the verdict is returned; a run superseded in the meantime
// publishes nothing.
function recheckOrder(api, repository, run, pr, result) {
  if (result.ignored || !['success', 'failure'].includes(result.status?.state)) return result;
  return supersession(supersedingRun(api, repository, run, pr), run, pr, () => result) || result;
}

// The supersession policy, shared by the check before the assessment and the
// recheck after it. It returns null when no newer run affects this one.
function supersession(superseding, run, pr, assess) {
  if (superseding === UNRESOLVED_RUN_ORDER) {
    return outcome({run, pr, routes: allRoutes(), state: 'failure', message: 'Run order for this head is unresolved'});
  }
  if (superseding === UNKNOWN_OWNERSHIP) {
    // Rule 3: an unknown newer owner never suppresses a failure, and a
    // success becomes a failure because the newest result is unknown.
    const result = assess();
    if (result.status?.state === 'success') {
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
  return null;
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
      // A run that never started has no verdict. It must not acquire one from
      // the supersession check either, because it cannot publish a status.
      if (!startedRun(run)) return outcome({run, pr, state: null, ignored: true, message: 'CI run did not start'});
      const assess = () => assessRun({api, runner, repository: options.repository, run, pr, comparison, trusted: true});
      const result = supersession(supersedingRun(api, options.repository, run, pr), run, pr, assess)
        || recheckOrder(api, options.repository, run, pr, assess());
      verdictContext.set(result, {run, pr});
      return result;
    }
    return assessRun({api, runner, repository: options.repository, run, pr});
  } catch (error) {
    return outcome({
      run, pr, state: 'failure', message: 'CI route gate could not complete',
      error: error instanceof Error ? error.message : String(error),
    });
  }
}

// A failure is published even when the evaluation did not complete: leaving
// the status alone would keep an earlier success for a head whose newest run
// is unverified. Only a result without a status target publishes nothing; the
// workflow step still exits non-zero and the run is visible. It returns
// whether a status was written.
function publish(api, repository, result) {
  if (result.ignored || !['success', 'failure'].includes(result.status.state)) return false;
  if (result.error && !result.sha) return false;
  assertSha(result.sha, 'status target');
  api(`repos/${repository}/statuses/${result.sha}`, {method: 'POST', input: result.status});
  return true;
}

// The order checks and the status write are not atomic: a newer run's gate
// can publish between this gate's last check and its write, which then leaves
// a stale verdict. So the order is checked again after the write. A newer run
// still in progress publishes when it completes. A newer completed run may
// already have published before this write, so its verdict is evaluated and
// published again here. An order that cannot be established turns a success
// into a failure.
function afterPublication(api, repository, result) {
  const context = verdictContext.get(result);
  if (!context) return null;
  const superseding = supersedingRun(api, repository, context.run, context.pr);
  if (superseding === UNRESOLVED_RUN_ORDER || superseding === UNKNOWN_OWNERSHIP) {
    if (result.status.state !== 'success') return null;
    return {
      result: outcome({
        run: context.run, pr: context.pr, routes: allRoutes(), state: 'failure',
        message: superseding === UNRESOLVED_RUN_ORDER ? 'Run order for this head is unresolved' : 'a newer run on this head has unknown ownership',
      }),
    };
  }
  if (!superseding || superseding.run.status !== 'completed') return null;
  return {runId: superseding.run.id};
}

function runGate(options, api, runner = spawnSync) {
  let result = execute(options, api, runner);
  if (!options.publish) return result;
  for (let round = 1; publish(api, options.repository, result); round++) {
    let next;
    try {
      next = afterPublication(api, options.repository, result);
    } catch (error) {
      next = {result: {...result, status: {...result.status, state: 'failure', description: 'CI route gate could not complete'},
        error: error instanceof Error ? error.message : String(error)}};
    }
    if (!next) return result;
    if (next.result) {
      publish(api, options.repository, next.result);
      return next.result;
    }
    if (round >= PUBLICATION_ROUNDS) {
      const context = verdictContext.get(result);
      const unstable = outcome({
        run: context.run, pr: context.pr, routes: allRoutes(), state: 'failure',
        message: 'Run order for this head kept changing while the verdict was published',
      });
      publish(api, options.repository, unstable);
      return unstable;
    }
    result = execute({...options, runId: next.runId, runAttempt: undefined}, api, runner);
  }
  return result;
}

// A gh API client whose reads fail once the assessment deadline has passed. A
// status write is still allowed after it, so a failure can be published.
function createGhApi({spawn = spawnSync, now = Date.now, start = now()} = {}) {
  return (endpoint, options = {}) => {
    if (!options.method && now() - start > ASSESSMENT_DEADLINE_MS) {
      throw new Error('CI route gate exceeded its assessment deadline');
    }
    return ghApiCall(spawn, endpoint, options);
  };
}

function ghApiCall(spawn, endpoint, options) {
  const args = ['api', endpoint];
  if (options.paginate) args.push('--paginate', '--slurp');
  if (options.method) args.push('--method', options.method);
  if (options.input !== undefined) args.push('--input', '-');
  const result = spawn('gh', args, {
    encoding: 'utf8',
    input: options.input === undefined ? undefined : JSON.stringify(options.input),
    maxBuffer: 32 * 1024 * 1024,
    timeout: API_TIMEOUT_MS,
    killSignal: 'SIGKILL',
  });
  if (result.error?.code === 'ETIMEDOUT') throw new Error(`gh api timed out for ${endpoint}`);
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
    result = runGate(options, createGhApi());
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
  createGhApi,
  evaluate,
  execute,
  parseArgs,
  parseClassifierOutput,
  publish,
  requiredJobs,
  runGate,
};
