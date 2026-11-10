/* Contract for the engine.speedle.io Pages workflow. No GitHub calls. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const root = path.resolve(__dirname, '../..');
const workflow = fs.readFileSync(path.join(root, '.github/workflows/pages.yml'), 'utf8');

const pins = [
  ['actions/checkout', '3d3c42e5aac5ba805825da76410c181273ba90b1', 'v7.0.1'],
  ['actions/jekyll-build-pages', '44a6e6beabd48582f863aeeb6cb2151cc1716697', 'v1.0.13'],
  ['actions/upload-pages-artifact', 'fc324d3547104276b827a68afc52ff2a11cc49c9', 'v5.0.0'],
  ['actions/deploy-pages', '368f82528645a54fb793d4d04e342629a3f51346', 'v5.0.1'],
];

function jobs(source) {
  const section = source.split(/^jobs:\s*$/m)[1];
  assert.ok(section, 'Missing jobs mapping');
  const matches = [...section.matchAll(/^  ([\w-]+):\s*$/gm)];
  return Object.fromEntries(matches.map((match, i) => [
    match[1], section.slice(match.index, matches[i + 1]?.index ?? section.length),
  ]));
}

const job = jobs(workflow);

test('Pages triggers build docs changes and the workflow file', () => {
  assert.match(workflow, /^name: Pages\n/);
  assert.match(workflow, /^permissions:\n  contents: read\n/m);
  assert.doesNotMatch(workflow, /pull_request_target|secrets\.|contents: write/);
  assert.match(workflow, /^concurrency:\n  group: pages\n  cancel-in-progress: false\n/m);
  const triggers = workflow.split(/^permissions:/m)[0];
  assert.match(triggers, /push:\n    branches: \[master\]\n    paths:\n      - docs\/\*\*\n      - \.github\/workflows\/pages\.yml\n/);
  assert.match(triggers, /pull_request:\n    paths:\n      - docs\/\*\*\n      - \.github\/workflows\/pages\.yml\n/);
  assert.match(triggers, /^  workflow_dispatch:\n$/m);
  assert.doesNotMatch(triggers, /workflow_dispatch:[\s\S]*paths:/);
});

test('every action is pinned to a full commit SHA and a version comment', () => {
  const uses = [...workflow.matchAll(/^(\s*)uses: (\S+)(.*)$/gm)];
  assert.equal(uses.length, pins.length);
  for (const [action, sha, version] of pins) {
    const line = uses.find((match) => match[2].startsWith(`${action}@`));
    assert.ok(line, action);
    assert.equal(line[2], `${action}@${sha}`);
    assert.equal(line[3], ` # ${version}`);
  }
});

test('the build job runs Jekyll on docs and uploads ./_site on every event', () => {
  assert.deepEqual(Object.keys(job), ['build', 'deploy']);
  const build = job.build;
  assert.equal(build.includes('\n    if:'), false);
  assert.match(build, /runs-on: ubuntu-latest\n/);
  assert.match(build, /ref: \$\{\{ github\.sha \}\}\n/);
  assert.match(build, /persist-credentials: false\n/);
  assert.match(build, /fetch-depth: 1\n/);
  assert.match(build, /source: \.\/docs\n/);
  assert.match(build, /destination: \.\/_site\n/);
  assert.match(build, /path: \.\/_site\n/);
  assert.doesNotMatch(build, /pages: write|id-token: write|environment:/);
});

test('the deploy job publishes only outside pull requests', () => {
  const deploy = job.deploy;
  assert.match(deploy, /needs: build\n/);
  assert.match(deploy, /if: github\.event_name != 'pull_request'\n/);
  assert.match(deploy, /runs-on: ubuntu-latest\n/);
  assert.match(deploy, /timeout-minutes: 40\n/);
  assert.match(deploy, /permissions:\n      pages: write\n      id-token: write\n/);
  assert.match(deploy, /environment:\n      name: github-pages\n      url: \$\{\{ steps\.deployment\.outputs\.page_url \}\}\n/);
  assert.match(deploy, /id: deployment\n/);
  assert.match(deploy, /# actions\/deploy-pages caps timeout at 600000 ms\n/);
  assert.match(deploy, /timeout: 600000\n/);
  assert.doesNotMatch(deploy, /timeout: 1800000/);
  assert.doesNotMatch(deploy, /contents:|pull-requests:|actions:|checks:|statuses:/);
});

test('docs remains a Jekyll source with the Cayman theme and a CNAME file', () => {
  assert.equal(fs.existsSync(path.join(root, 'docs/.nojekyll')), false);
  const config = fs.readFileSync(path.join(root, 'docs/_config.yml'), 'utf8');
  assert.doesNotMatch(config, /\.nojekyll/);
  const themes = [...config.matchAll(/^theme:\s*(\S+)\s*$/gm)].map((match) => match[1]);
  assert.deepEqual(themes, ['jekyll-theme-cayman']);
  const exclude = config.split(/^exclude:\s*$/m)[1];
  assert.ok(exclude, 'docs/_config.yml must exclude the Hugo sources');
  const entries = [...exclude.matchAll(/^  - (\S+)\s*$/gm)].map((match) => match[1]);
  assert.deepEqual(entries, ['hugo']);
  assert.equal(fs.readFileSync(path.join(root, 'docs/CNAME'), 'utf8').trim(), 'engine.speedle.io');
});
