/* Exercise the actual github-script handlers with in-memory GitHub API stubs. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const workflow = fs.readFileSync(path.join(__dirname, '../../.github/workflows/reviewbot-command.yml'), 'utf8');
function scriptFor(name) {
  const step = workflow.split(`      - name: ${name}\n`)[1];
  assert.ok(step, `Missing workflow step: ${name}`);
  const block = step.split('          script: |\n')[1];
  assert.ok(block, `Missing github-script body: ${name}`);
  const lines = [];
  for (const line of block.split('\n')) {
    if (line && !line.startsWith('            ')) break;
    lines.push(line.slice(12));
  }
  return `(async () => {\n${lines.join('\n')}\n})()`;
}
const scripts = {
  prepare: scriptFor('Parse and authorize command'),
  history: scriptFor('Collect prior ReviewBot findings for recheck'),
  publish: scriptFor('Publish idempotent response'),
};
const base = 'a'.repeat(40), head = 'b'.repeat(40);
const marker = '<!-- speedle-plus-reviewbot-command:review:issue_comment:123 -->';
const bot = {id: 2, login: 'github-actions[bot]', type: 'Bot'};
const human = {id: 1, login: 'maintainer', type: 'User'};
const pullUrl = 'https://api.github.com/repos/teramoby/speedle-plus/pulls/7';
const encode = value => Buffer.from(value).toString('base64');
const decode = value => Buffer.from(value, 'base64').toString('utf8');
function harness(options = {}) {
  const eventName = options.eventName || 'issue_comment';
  const calls = {permissions: [], pulls: [], requests: [], reviews: [], comments: [], notices: [], warnings: [], reviewCommentReads: [], associations: []};
  const outputs = {};
  const comment = {
    id: 123, body: options.body ?? '@reviewbot review', user: options.user ?? human,
    html_url: 'https://github.com/teramoby/speedle-plus/pull/7#issuecomment-123',
    created_at: '2026-01-01T00:00:00Z', pull_request_url: pullUrl,
    ...(options.comment || {}),
  };
  const run = {
    id: 456, name: 'ReviewBot Inline Command', path: '.github/workflows/reviewbot-inline.yml',
    event: 'pull_request_review_comment', conclusion: 'success', display_title: 'ReviewBot inline comment 123',
    actor: human, head_repository: {full_name: 'teramoby/speedle-plus'}, head_sha: head,
    created_at: '2026-01-01T00:00:01Z', pull_requests: [{number: 7, url: pullUrl}],
    ...(options.run || {}),
  };
  const context = {
    eventName, repo: {owner: 'teramoby', repo: 'speedle-plus'},
    serverUrl: 'https://github.com', apiUrl: 'https://api.github.com', runId: 999,
    payload: {comment: options.payloadComment || comment, workflow_run: run, issue: {number: 7, pull_request: {}}, pull_request: {number: 7}},
  };
  const endpoint = name => Object.assign(() => {}, {kind: name});
  const github = {
    rest: {
      repos: {
        getCollaboratorPermissionLevel: async args => {
          calls.permissions.push(args); return {data: {permission: options.permission || 'write'}};
        },
        listPullRequestsAssociatedWithCommit: endpoint('associatedPulls'),
      },
      pulls: {
        getReviewComment: async args => {
          calls.reviewCommentReads.push(args); return {data: {...comment, ...(options.apiComment || {})}};
        },
        get: async args => {
          calls.pulls.push(args);
          return {data: {state: options.state || 'open', base: {sha: base}, head: {
            sha: head, repo: {full_name: options.fork ? 'outsider/speedle-plus' : 'teramoby/speedle-plus'},
          }}};
        },
        listReviews: endpoint('reviews'), listReviewComments: endpoint('reviewComments'),
        createReview: async args => {calls.reviews.push(args); return {data: {id: 10}};},
      },
      issues: {
        listComments: endpoint('comments'),
        createComment: async args => {calls.comments.push(args); return {data: {id: 11}};},
      },
    },
    paginate: async (endpoint, args) => {
      if (endpoint.kind === 'associatedPulls') {
        calls.associations.push(args);
        return options.associatedPulls ?? [{number: 7, url: pullUrl}];
      }
      return options.history?.[endpoint.kind] || [];
    },
    request: async (route, args) => {
      calls.requests.push({route, ...args});
      if (options.reactionFailure && route.endsWith('/reactions')) throw new Error('Temporary reaction failure');
      return {data: {}};
    },
  };
  const env = {
    REVIEWBOT_MESSAGE: '## ReviewBot review\n\nNo actionable findings.\n\n— **ReviewBot**',
    MODEL_ID_B64: encode('kimi-k3'), ANALYZE_RESULT: 'success', REVIEWBOT_ENABLED: 'true',
    COMMAND: 'review', PULL_NUMBER: '7', HEAD_SHA: head, ACTOR: human.login,
    COMMENT_ID: '123', REPLY_TO_COMMENT_ID: '100', SOURCE_KIND: 'issue_comment',
    SOURCE_URL: comment.html_url, ...(options.env || {}),
  };
  const core = {
    setOutput: (key, value) => {outputs[key] = value;},
    notice: text => calls.notices.push(text), warning: text => calls.warnings.push(text),
  };
  return {
    calls, outputs,
    run: name => vm.runInNewContext(scripts[name], {github, context, core, Buffer, URL, process: {env}}),
  };
}
function published(h) {return [...h.calls.reviews, ...h.calls.comments, ...h.calls.requests.filter(x => x.body)];}
function reactions(h) {return h.calls.requests.filter(x => x.route.endsWith('/reactions')).map(x => x.content);}

test('secret-bearing command workflow accepts default-branch events and inline router has no privileges', () => {
  const triggers = workflow.split('\npermissions:', 1)[0];
  assert.match(triggers, /^  issue_comment:/m); assert.match(triggers, /^  workflow_run:/m);
  assert.doesNotMatch(triggers, /^  pull_request_review_comment:/m);
  const router = fs.readFileSync(path.join(__dirname, '../../.github/workflows/reviewbot-inline.yml'), 'utf8');
  assert.match(router, /^name: ReviewBot Inline Command$/m);
  assert.match(router, /^run-name: ReviewBot inline comment \$\{\{ github.event.comment.id \}\}$/m);
  assert.match(router, /^permissions: \{\}$/m); assert.match(router, /^  pull_request_review_comment:/m);
  assert.match(router, /^        run: ':'$/m);
  assert.doesNotMatch(router, /\buses:|\bsecrets\.|\bcheckout@|\b(?:upload|download)-artifact|^\s+env:/m);
  assert.match(workflow, /ref: \$\{\{ github.workflow_sha \}\}/);
  assert.match(workflow, /git show "\$\{TRUSTED_SHA\}:\.github\/codex\/reviewbot_response\.py"/);
});
test('direct review-comment events cannot enter the secret-bearing command handler', async () => {
  const h = harness({eventName: 'pull_request_review_comment'}); await h.run('prepare');
  assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0);
  assert.equal(h.calls.reviewCommentReads.length, 0); assert.deepEqual(reactions(h), []);
});
test('inline bridge reads authoritative comment body, identity and PR from GitHub instead of source payload', async () => {
  const h = harness({eventName: 'workflow_run', body: '@reviewbot explain actual question', payloadComment: {
    id: 999, body: '@reviewbot review FORGED_PAYLOAD', user: {id: 90, login: 'forged', type: 'User'},
  }}); await h.run('prepare');
  assert.equal(h.outputs.should_run, 'true'); assert.equal(h.outputs.command, 'explain');
  assert.equal(decode(h.outputs.request_b64), 'actual question'); assert.equal(h.outputs.actor, human.login);
  assert.equal(h.outputs.comment_id, '123'); assert.equal(h.outputs.pull_number, '7');
  assert.equal(h.outputs.source_kind, 'review_comment'); assert.equal(h.calls.reviewCommentReads.length, 1);
  assert.equal(h.calls.reviewCommentReads[0].comment_id, 123); assert.deepEqual(reactions(h), ['eyes']);
});
test('inline bridge rejects wrong workflow identity, event, outcome, repository and title before comment lookup', async () => {
  for (const run of [
    {event: 'push'}, {conclusion: 'failure'}, {name: 'Other workflow'},
    {path: '.github/workflows/other.yml'}, {head_repository: {full_name: 'outsider/speedle-plus'}},
    {actor: bot}, {display_title: 'ReviewBot inline comment 123 extra'},
    {display_title: 'ReviewBot inline comment 0'}, {display_title: 'ReviewBot inline comment 01'},
    {display_title: 'ReviewBot inline comment 9007199254740993'}, {display_title: undefined},
  ]) {
    const h = harness({eventName: 'workflow_run', run}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false', JSON.stringify(run)); assert.equal(h.calls.reviewCommentReads.length, 0);
    assert.equal(h.calls.permissions.length, 0); assert.deepEqual(reactions(h), []);
  }
});
test('inline bridge binds pointer to exact API comment and original run actor ID and login', async () => {
  for (const options of [
    {apiComment: {id: 124}}, {run: {actor: {...human, id: 90}}},
    {run: {actor: {...human, login: 'different-user'}}}, {apiComment: {user: {login: human.login, type: 'User'}}},
  ]) {
    const h = harness({eventName: 'workflow_run', ...options}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0); assert.deepEqual(reactions(h), []);
  }
});
test('inline bridge rejects stale, future and invalid comment or router timestamps', async () => {
  for (const options of [
    {run: {created_at: '2026-01-01T00:02:01Z'}}, {apiComment: {created_at: '2026-01-01T00:00:02Z'}},
    {run: {created_at: 'not-a-time'}}, {apiComment: {created_at: undefined}},
  ]) {
    const h = harness({eventName: 'workflow_run', ...options}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0); assert.deepEqual(reactions(h), []);
  }
  const boundary = harness({eventName: 'workflow_run', run: {created_at: '2026-01-01T00:02:00Z'}});
  await boundary.run('prepare'); assert.equal(boundary.outputs.should_run, 'true');
});
test('inline bridge only accepts the GitHub API pull-request URL of this repository', async () => {
  for (const url of [
    'https://api.github.com.evil.test/repos/teramoby/speedle-plus/pulls/7',
    'https://api.github.com/repos/outsider/speedle-plus/pulls/7',
    'https://api.github.com/repos/teramoby/speedle-plus/issues/7', `${pullUrl}?x=1`, `${pullUrl}#fragment`,
    'not a URL', 'https://api.github.com/repos/teramoby/speedle-plus/pulls/9007199254740993',
  ]) {
    const h = harness({eventName: 'workflow_run', apiComment: {pull_request_url: url}}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0); assert.deepEqual(reactions(h), []);
  }
});
test('populated source-run association must identify exactly the API comment PR', async () => {
  for (const pull_requests of [[{number: 8, url: pullUrl}], [{number: 7, url: 'https://api.github.com/repos/outsider/repo/pulls/7'}]]) {
    const h = harness({eventName: 'workflow_run', run: {pull_requests}}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.associations.length, 0);
    assert.equal(h.calls.permissions.length, 0); assert.deepEqual(reactions(h), []);
  }
});
test('empty source-run PR metadata requires GitHub commit association rather than trusting the title', async () => {
  for (const pull_requests of [[], undefined]) {
    const h = harness({eventName: 'workflow_run', run: {pull_requests}}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'true'); assert.equal(h.calls.associations.length, 1);
    assert.equal(h.calls.associations[0].commit_sha, head); assert.equal(h.calls.associations[0].repo, 'speedle-plus');
  }
  for (const associatedPulls of [[], [{number: 8, url: pullUrl}], [{number: 7, url: 'https://api.github.com/repos/outsider/repo/pulls/7'}]]) {
    const h = harness({eventName: 'workflow_run', run: {pull_requests: []}, associatedPulls}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0); assert.deepEqual(reactions(h), []);
  }
  const badCommit = harness({eventName: 'workflow_run', run: {pull_requests: [], head_sha: 'not-a-commit'}});
  await badCommit.run('prepare'); assert.equal(badCommit.outputs.should_run, 'false'); assert.equal(badCommit.calls.associations.length, 0);
});
test('inline bridge retains human write-permission, open-PR and current same-repository guards', async () => {
  for (const options of [{permission: 'read'}, {permission: 'triage'}, {state: 'closed'}, {fork: true}]) {
    const h = harness({eventName: 'workflow_run', ...options}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.deepEqual(reactions(h), []);
  }
});
test('inline comment commands retain comment-specific idempotence after the bridge', async () => {
  const inlineMarker = '<!-- speedle-plus-reviewbot-command:review:review_comment:123 -->';
  const done = harness({eventName: 'workflow_run', history: {reviews: [{user: bot, body: `${inlineMarker}\nPrior review`}]}});
  await done.run('prepare'); assert.equal(done.outputs.should_run, 'false');
  const quoted = harness({eventName: 'workflow_run', history: {reviews: [{user: bot, body: `Quoted marker: ${inlineMarker}`} ]}});
  await quoted.run('prepare'); assert.equal(quoted.outputs.should_run, 'true');
});

test('unrelated comments and lookalike mentions do not query permissions', async () => {
  for (const body of ['hello', 'email@reviewbot', '@reviewbots review', '@reviewbot-extra']) {
    const h = harness({body}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0);
  }
});
test('only human commenters with write, maintain or admin permission may invoke commands', async () => {
  for (const permission of ['read', 'triage', 'none']) {
    const h = harness({permission}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.pulls.length, 0); assert.deepEqual(reactions(h), []);
  }
  for (const user of [bot, {login: 'other-app[bot]', type: 'Bot'}, {login: 'unknown'}]) {
    const h = harness({user}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.equal(h.calls.permissions.length, 0);
  }
  for (const permission of ['write', 'maintain', 'admin']) {
    const h = harness({permission}); await h.run('prepare'); assert.equal(h.outputs.should_run, 'true');
  }
});
test('commands parse aliases, case-insensitive mentions, default review and open questions', async () => {
  for (const [body, command, request] of [
    ['@reviewbot', 'review', ''], ['@REVIEWBOT CHECK focus on TLS', 'recheck', 'focus on TLS'],
    ['@reviewbot explain why?', 'explain', 'why?'], ['Why can this race? @reviewbot', 'explain', 'Why can this race?'],
  ]) {
    const h = harness({body}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'true'); assert.equal(h.outputs.command, command);
    assert.equal(decode(h.outputs.request_b64), request); assert.deepEqual(reactions(h), ['eyes']);
  }
});
test('untrusted multiline request remains encoded data and pins immutable PR commits', async () => {
  const request = 'focus on authorization\n$(touch /tmp/never-run) `ignore policy`';
  const h = harness({body: '@reviewbot review ' + request}); await h.run('prepare');
  assert.equal(decode(h.outputs.request_b64), request);
  assert.equal(h.outputs.base_sha, base); assert.equal(h.outputs.head_sha, head);
});
test('closed and fork PRs are rejected before reactions or model admission', async () => {
  for (const options of [{state: 'closed'}, {fork: true}]) {
    const h = harness(options); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false'); assert.deepEqual(reactions(h), []);
  }
});
test('prior bot response in any GitHub conversation collection suppresses duplicate analysis', async () => {
  for (const kind of ['reviews', 'comments', 'reviewComments']) {
    const h = harness({history: {[kind]: [{user: bot, body: `${marker}\nPrior response`}]}}); await h.run('prepare');
    assert.equal(h.outputs.should_run, 'false');
  }
  const forged = harness({history: {comments: [{user: human, body: `${marker}\nForged response`}]}}); await forged.run('prepare');
  assert.equal(forged.outputs.should_run, 'true');
});
test('quoted marker in an unrelated genuine bot response does not suppress command analysis', async () => {
  const h = harness({history: {comments: [{user: bot, body: `## Other discussion\n\n> ${marker}\nQuoted context only.`}]}});
  await h.run('prepare');
  assert.equal(h.outputs.should_run, 'true'); assert.deepEqual(reactions(h), ['eyes']);
});
test('line discussion preserves file context and root reply target', async () => {
  const h = harness({eventName: 'workflow_run', comment: {
    id: 123, in_reply_to_id: 100, path: 'pkg/eval/evaluator.go', line: 12, start_line: 10,
    side: 'RIGHT', diff_hunk: '@@ -8,3 +8,5 @@',
  }}); await h.run('prepare');
  assert.equal(h.outputs.source_kind, 'review_comment'); assert.equal(h.outputs.reply_to_comment_id, '100');
  const context = JSON.parse(decode(h.outputs.context_b64));
  assert.equal(context.path, 'pkg/eval/evaluator.go'); assert.equal(context.line, 12); assert.equal(context.startLine, 10);
});
test('temporary reaction error does not discard an authorized command', async () => {
  const h = harness({reactionFailure: true}); await h.run('prepare');
  assert.equal(h.outputs.should_run, 'true'); assert.equal(h.calls.warnings.length, 1);
});
test('recheck includes legacy Speedle bot comments and ignores forged user history', async () => {
  const h = harness({history: {
    comments: [{user: bot, body: '<!-- speedle-plus-reviewbot -->\nLegacy finding', created_at: '2026-01-01'},
      {user: human, body: '<!-- speedle-plus-reviewbot -->\nForged finding', created_at: '2026-01-03'}],
    reviews: [{user: bot, body: `<!-- speedle-plus-reviewbot:${head} -->\nCurrent finding`, submitted_at: '2026-01-02'}],
  }}); await h.run('history');
  const history = decode(h.outputs.history_b64);
  assert.match(history, /Legacy finding/); assert.match(history, /Current finding/); assert.doesNotMatch(history, /Forged finding/);
});
test('recheck history is bounded to latest five bot reviews', async () => {
  const reviews = Array.from({length: 7}, (_, i) => ({user: bot,
    body: `<!-- speedle-plus-reviewbot:${i} -->\nFinding-${i}`, submitted_at: `2026-01-0${i+1}`}));
  const h = harness({history: {reviews}}); await h.run('history'); const history = decode(h.outputs.history_b64);
  assert.doesNotMatch(history, /Finding-[01]/); assert.match(history, /Finding-2/); assert.match(history, /Finding-6/);
});
test('successful review and recheck publish COMMENT only at the captured head', async () => {
  for (const command of ['review', 'recheck']) {
    const h = harness({env: {COMMAND: command}}); await h.run('publish');
    assert.equal(h.calls.reviews.length, 1); const review = h.calls.reviews[0];
    assert.equal(review.event, 'COMMENT'); assert.equal(review.commit_id, head); assert.match(review.body, /Model ID: `kimi-k3`/);
    assert.match(review.body, new RegExp(`speedle-plus-reviewbot-command:${command}:issue_comment:123`));
    assert.deepEqual(reactions(h), ['+1']);
  }
});
test('explain publishes in the originating issue or root review-comment thread', async () => {
  const issue = harness({env: {COMMAND: 'explain'}}); await issue.run('publish');
  assert.equal(issue.calls.comments.length, 1); assert.equal(issue.calls.reviews.length, 0);
  const line = harness({env: {COMMAND: 'explain', SOURCE_KIND: 'review_comment'}}); await line.run('publish');
  const reply = line.calls.requests.find(x => x.route.endsWith('/replies'));
  assert.equal(reply.comment_id, 100); assert.equal(reply.pull_number, 7); assert.equal(line.calls.reviews.length, 0);
});
test('failed or disabled analysis emits generic status without model text, ID or completion reaction', async () => {
  for (const env of [{ANALYZE_RESULT: 'failure'}, {REVIEWBOT_ENABLED: 'false'}]) {
    const h = harness({env: {...env, REVIEWBOT_MESSAGE: 'PRIVATE_MODEL_OUTPUT'}}); await h.run('publish');
    const output = published(h); assert.equal(output.length, 1);
    assert.doesNotMatch(output[0].body, /PRIVATE_MODEL_OUTPUT|Model ID:/); assert.deepEqual(reactions(h), []);
  }
});
test('publish is idempotent for bot markers and cannot be suppressed by a human forged marker', async () => {
  const done = harness({history: {reviews: [{user: bot, body: `${marker}\nPrior response`}]}}); await done.run('publish');
  assert.equal(published(done).length, 0); assert.deepEqual(reactions(done), ['+1']);
  const forged = harness({history: {comments: [{user: human, body: `${marker}\nForged response`}]}}); await forged.run('publish');
  assert.equal(forged.calls.reviews.length, 1);
});
test('quoted marker in an unrelated genuine bot response does not suppress publication', async () => {
  const h = harness({history: {reviews: [{user: bot, body: `## Other discussion\n\n> ${marker}\nQuoted context only.`}]}});
  await h.run('publish');
  assert.equal(h.calls.reviews.length, 1); assert.deepEqual(reactions(h), ['+1']);
});
test('reaction failure preserves a successfully published review', async () => {
  const h = harness({reactionFailure: true}); await h.run('publish');
  assert.equal(h.calls.reviews.length, 1); assert.equal(h.calls.warnings.length, 1);
});
test('long output stays within GitHub limit while retaining idempotence and commit metadata', async () => {
  const h = harness({env: {REVIEWBOT_MESSAGE: 'X'.repeat(70000)}}); await h.run('publish');
  const body = h.calls.reviews[0].body;
  assert.ok(body.length <= 65000, `Published length ${body.length} exceeds 65000`);
  assert.ok(body.startsWith(marker)); assert.ok(body.includes(head)); assert.match(body, /output was truncated/);
});
