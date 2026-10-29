/* The executable documentation gate must run for every code change. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const workflow = fs.readFileSync(path.resolve(__dirname, '../../.github/workflows/ci.yml'), 'utf8');
const job = workflow.split(/^  test:\n/m)[1].split(/^  [a-z][\w-]*:\n/m)[0];

test('the global test gate cannot be skipped by any changed-file classification', () => {
  assert.doesNotMatch(job, /^    if:/m);
  assert.doesNotMatch(job, /outputs\.(?:go|website|docs)/);
  assert.match(job, /uses: teramoby\/speedle-plus\/\.github\/workflows\/go-ci-worker\.yml@master/);
  assert.match(job, /mode: test/);
});
test('CI triggers the global gate on all PRs and master pushes', () => {
  assert.match(workflow, /^  pull_request:\s*$/m);
  assert.match(workflow, /push:\n    branches: \[master\]/);
});
