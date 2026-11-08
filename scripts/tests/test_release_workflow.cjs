/* Exercise release publishing decisions with stubbed git/docker, never a registry. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const test = require('node:test');

const source = fs.readFileSync(path.resolve(__dirname, '../../.github/workflows/release.yml'), 'utf8');
function step(name) {
  const parts = source.split(`      - name: ${name}\n`);
  assert.equal(parts.length, 2, `Missing or ambiguous step ${name}`);
  return parts[1].split(/^      - /m, 1)[0];
}
function script(name) {
  const literal = step(name).split('        run: |\n')[1];
  assert.ok(literal, `Missing shell script ${name}`);
  const lines = [];
  for (const line of literal.split('\n')) {
    if (line && !line.startsWith('          ')) break;
    lines.push(line.slice(10));
  }
  return lines.join('\n').trimEnd();
}
const publish = script('Publish container images');

function runPublish({version, commit, master, tags, validMaster = true}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-release-publish-'));
  try {
    const calls = path.join(directory, 'calls.jsonl');
    const stub = tool => `#!${process.execPath}
const fs=require('node:fs');const a=process.argv.slice(2);const tool=${JSON.stringify(tool)};
fs.appendFileSync(process.env.FIXTURE_CALLS,JSON.stringify({tool,args:a})+'\\n');
if(tool==='git'&&a[0]==='ls-remote'){if(process.env.FIXTURE_VALID_MASTER==='true')console.log(process.env.FIXTURE_MASTER+'\\trefs/heads/master');process.exit(0)}
if(tool==='git'&&a[0]==='tag'&&a[1]==='--list'){const tags=process.env.FIXTURE_TAGS.split(',').filter(Boolean);if(tags.length)console.log(tags.join('\\n'));process.exit(0)}
if(tool==='git')process.exit(2);
process.exit(0);
`;
    for (const tool of ['git', 'docker']) {
      const file = path.join(directory, tool);
      fs.writeFileSync(file, stub(tool)); fs.chmodSync(file, 0o755);
    }
    // macOS still ships Bash 3.2; the fixture owner is already lowercase, so
    // replace only Bash 4's case-expansion syntax when running this shell locally.
    const portable = publish.replace('owner="${GITHUB_REPOSITORY_OWNER,,}"', 'owner="${GITHUB_REPOSITORY_OWNER}"');
    const result = spawnSync('bash', ['-c', portable], {encoding: 'utf8', env: {
      ...process.env, PATH: `${directory}${path.delimiter}${process.env.PATH}`,
      FIXTURE_CALLS: calls, FIXTURE_MASTER: master, FIXTURE_TAGS: tags.join(','), FIXTURE_VALID_MASTER: String(validMaster),
      VERSION: version, COMMIT: commit, GITHUB_REPOSITORY_OWNER: 'teramoby', GITHUB_ACTOR: 'release-test',
      GITHUB_TOKEN: 'fixture-token', REGISTRY: 'ghcr.io', CONTAINER_CONTEXT: 'container-context',
    }});
    const records = fs.existsSync(calls) ? fs.readFileSync(calls, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
    return {result, builds: records.filter(record => record.tool === 'docker' && record.args[0] === 'buildx' && record.args[1] === 'build')};
  } finally {
    fs.rmSync(directory, {recursive: true, force: true});
  }
}

function runBuild() {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-release-build-'));
  try {
    const go = path.join(directory, 'go');
    fs.writeFileSync(go, `#!${process.execPath}
const fs=require('node:fs');const path=require('node:path');const args=process.argv.slice(2);
const output=args[args.indexOf('-o')+1];
fs.mkdirSync(path.dirname(output),{recursive:true});
fs.writeFileSync(output,JSON.stringify({args,goos:process.env.GOOS,goarch:process.env.GOARCH,cgo:process.env.CGO_ENABLED}));
`);
    fs.chmodSync(go, 0o755);
    const result = spawnSync('bash', ['-c', script('Build release binaries')], {
      cwd: directory, encoding: 'utf8',
      env: {...process.env, PATH: `${directory}${path.delimiter}${process.env.PATH}`},
    });
    const dist = path.join(directory, 'dist');
    const artifacts = fs.existsSync(dist)
      ? Object.fromEntries(fs.readdirSync(dist).map(name => [name, fs.readFileSync(path.join(dist, name), 'utf8')]))
      : {};
    return {result, artifacts};
  } finally {
    fs.rmSync(directory, {recursive: true, force: true});
  }
}

const master = 'a'.repeat(40);
const older = 'b'.repeat(40);
function imageTags(build) {
  const tags = [];
  build.args.forEach((arg, index) => { if (arg === '--tag') tags.push(build.args[index + 1]); });
  return tags;
}

test('latest accompanies the highest version built at the current remote master tip', () => {
  const {result, builds} = runPublish({version: 'v1.6.0', commit: master, master, tags: ['v1.5.0', 'v1.6.0']});
  assert.equal(result.status, 0, result.stderr);
  assert.equal(builds.length, 2);
  for (const build of builds) {
    const tags = imageTags(build);
    assert.ok(tags.some(tag => tag.endsWith(':v1.6.0')));
    assert.ok(tags.some(tag => tag.endsWith(':latest')), JSON.stringify(tags));
  }
});

test('a higher version from an older master commit cannot move latest', () => {
  const {result, builds} = runPublish({version: 'v1.7.0', commit: older, master, tags: ['v1.6.0', 'v1.7.0']});
  assert.equal(result.status, 0, result.stderr);
  assert.equal(builds.length, 2);
  for (const build of builds) {
    const tags = imageTags(build);
    assert.ok(tags.some(tag => tag.endsWith(':v1.7.0')));
    assert.ok(!tags.some(tag => tag.endsWith(':latest')), JSON.stringify(tags));
  }
});

test('an older semantic version at the master tip cannot move latest', () => {
  const {result, builds} = runPublish({version: 'v1.5.1', commit: master, master, tags: ['v1.5.0', 'v1.5.1', 'v1.6.0']});
  assert.equal(result.status, 0, result.stderr);
  for (const build of builds) assert.ok(!imageTags(build).some(tag => tag.endsWith(':latest')));
});

test('the first release can create latest and an unresolved master blocks publishing', () => {
  const first = runPublish({version: 'v1.0.0', commit: master, master, tags: []});
  assert.equal(first.result.status, 0, first.result.stderr);
  for (const build of first.builds) assert.ok(imageTags(build).some(tag => tag.endsWith(':latest')));
  const unresolved = runPublish({version: 'v1.0.0', commit: master, master: '', tags: [], validMaster: false});
  assert.notEqual(unresolved.result.status, 0);
  assert.equal(unresolved.builds.length, 0);
});

test('every macOS and Linux release keeps spctl and adds an identical speedle-spctl alias', () => {
  const {result, artifacts} = runBuild();
  assert.equal(result.status, 0, result.stderr);
  assert.equal(Object.keys(artifacts).length, 16);
  for (const target of ['darwin_amd64', 'darwin_arm64', 'linux_amd64', 'linux_arm64']) {
    const original = `spctl_${target}`;
    const alias = `speedle-spctl_${target}`;
    assert.ok(artifacts[original], `Missing ${original}`);
    assert.equal(artifacts[alias], artifacts[original], `${alias} must be a copy of ${original}`);
    const build = JSON.parse(artifacts[original]);
    assert.equal(build.goos, target.split('_')[0]);
    assert.equal(build.goarch, target.split('_')[1]);
    assert.equal(build.cgo, '0');
    assert.ok(build.args.includes('./cmd/spctl'));
  }
});

test('release checksums and uploaded assets include both spctl names', () => {
  assert.match(script('Write checksums'), /sha256sum spctl_\* speedle-spctl_\* speedle-ads_\* speedle-pms_\*/);
  assert.match(script('Publish the GitHub release'), /assets=\(dist\/spctl_\* dist\/speedle-spctl_\* dist\/speedle-pms_\* dist\/speedle-ads_\* dist\/checksums\.txt\)/);
});

test('changed release shell blocks parse before any publishing or container operation', () => {
  for (const name of ['Build release binaries', 'Write checksums', 'Verify the container images build and start', 'Publish the GitHub release', 'Publish container images']) {
    const result = spawnSync('bash', ['-n'], {input: script(name), encoding: 'utf8'});
    assert.equal(result.status, 0, `${name}: ${result.stderr}`);
  }
});

test('container smoke writes a policy through PMS and requires ADS to allow it from the shared volume', () => {
  const smoke = step('Verify the container images build and start');
  assert.match(smoke, /dist\/spctl_linux_amd64 --pms-endpoint/);
  assert.doesNotMatch(smoke, /dist\/spctl --pms-endpoint/);
  assert.match(smoke, /create service smoke/);
  assert.match(smoke, /create policy[\s\S]*grant user smoke-user read smoke-resource/);
  assert.match(smoke, /get service --all/);
  assert.match(smoke, /speedle-smoke-ads[\s\S]*-v speedle-smoke:\/data/);
  assert.match(smoke, /authz-check\/v1\/is-allowed/);
  assert.match(smoke, /result\.get\("allowed"\) is True and result\.get\("reason"\) == 0/);
});
