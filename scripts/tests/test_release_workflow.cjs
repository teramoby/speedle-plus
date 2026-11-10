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
const publish = script('Publish the container image');

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

function expectedTags(version, latest) {
  const imageVersion = version.slice(1);
  const minor = imageVersion.slice(0, imageVersion.lastIndexOf('.'));
  const names = ['docker.io/speedle/speedle', 'ghcr.io/teramoby/speedle'];
  const tags = [];
  for (const name of names) tags.push(`${name}:${imageVersion}`, `${name}:${minor}`);
  if (latest) for (const name of names) tags.push(`${name}:latest`);
  return tags;
}

function assertPublished(result, builds, version, latest) {
  assert.equal(result.status, 0, result.stderr);
  assert.equal(builds.length, 1);
  const build = builds[0];
  assert.deepEqual(imageTags(build), expectedTags(version, latest));
  assert.ok(build.args.includes('--provenance=mode=max'));
  assert.ok(build.args.includes('--sbom=true'));
  assert.ok(build.args.includes('--push'));
  assert.ok(build.args.includes('deployment/container/Dockerfile'));
  assert.equal(build.args[build.args.indexOf('--platform') + 1], 'linux/amd64,linux/arm64');
}

test('latest accompanies the highest version built at the current remote master tip', () => {
  const {result, builds} = runPublish({version: 'v1.6.0', commit: master, master, tags: ['v1.5.0', 'v1.6.0']});
  assertPublished(result, builds, 'v1.6.0', true);
});

test('a higher version from an older master commit cannot move latest', () => {
  const {result, builds} = runPublish({version: 'v1.7.0', commit: older, master, tags: ['v1.6.0', 'v1.7.0']});
  assertPublished(result, builds, 'v1.7.0', false);
});

test('an older semantic version at the master tip cannot move latest', () => {
  const {result, builds} = runPublish({version: 'v1.5.1', commit: master, master, tags: ['v1.5.0', 'v1.5.1', 'v1.6.0']});
  assertPublished(result, builds, 'v1.5.1', false);
});

test('the first release can create latest and an unresolved master blocks publishing', () => {
  const first = runPublish({version: 'v1.0.0', commit: master, master, tags: []});
  assertPublished(first.result, first.builds, 'v1.0.0', true);
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
  for (const name of ['Build release binaries', 'Write checksums', 'Require Docker Hub credentials', 'Verify the container image builds and starts', 'Log in to container registries', 'Publish the GitHub release', 'Publish the container image']) {
    const result = spawnSync('bash', ['-n'], {input: script(name), encoding: 'utf8'});
    assert.equal(result.status, 0, `${name}: ${result.stderr}`);
  }
});

test('Docker Hub credentials are required before a tag and their values are read only by the login step', () => {
  const requireStep = step('Require Docker Hub credentials');
  const login = step('Log in to container registries');
  assert.ok(source.indexOf('- name: Require Docker Hub credentials') < source.indexOf('- name: Create and push the tag'));
  assert.ok(source.indexOf('- name: Create and push the tag') < source.indexOf('- name: Log in to container registries'));
  assert.ok(source.indexOf('- name: Log in to container registries') < source.indexOf('- name: Publish the container image'));
  assert.match(source, /\n    environment: release\n/);
  assert.match(requireStep, /DOCKERHUB_USERNAME_SET/);
  assert.match(requireStep, /DOCKERHUB_TOKEN_SET/);
  assert.match(requireStep, /No release tag was created/);
  assert.match(login, /docker login docker\.io --username "\$\{DOCKERHUB_USERNAME\}" --password-stdin/);
  assert.match(login, /docker login ghcr\.io --username "\$\{GITHUB_ACTOR\}" --password-stdin/);
  const rawSecret = /secrets\.DOCKERHUB_(?:USERNAME|TOKEN)\s*\}\}/g;
  assert.deepEqual([...login.matchAll(rawSecret)].map(match => match[0]), [
    'secrets.DOCKERHUB_USERNAME }}',
    'secrets.DOCKERHUB_TOKEN }}',
  ]);
  assert.equal([...source.replace(login, '').matchAll(rawSecret)].length, 0);
  assert.doesNotMatch(source, /Dockerfile\.(?:pms|ads)/);
  const prepare = script('Prepare the container build context');
  assert.match(prepare, /spctl-linux-/);
  assert.match(prepare, /dispatch-linux-\$\{arch\}/);
  assert.match(prepare, /\.\/deployment\/container\/dispatch/);
});

test('container smoke writes a policy through PMS and requires ADS to allow it from the shared volume', () => {
  const smoke = step('Verify the container image builds and starts');
  assert.match(smoke, /host_arch=\$\(go env GOHOSTARCH\)/);
  assert.match(smoke, /spctl="dist\/spctl_linux_\$\{host_arch\}"/);
  assert.equal([...smoke.matchAll(/"\$\{spctl\}" --pms-endpoint/g)].length, 3);
  assert.doesNotMatch(smoke, /dist\/spctl_linux_(?:amd64|arm64) --pms-endpoint/);
  assert.match(smoke, /create service smoke/);
  assert.match(smoke, /create policy[\s\S]*grant user smoke-user read smoke-resource/);
  assert.match(smoke, /get service --all/);
  assert.match(smoke, /speedle-smoke-ads[\s\S]*-v speedle-smoke:\/data/);
  assert.match(smoke, /authz-check\/v1\/is-allowed/);
  assert.match(smoke, /result\.get\("allowed"\) is True and result\.get\("reason"\) == 0/);
});

function runSmoke(hostArch) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'speedle-release-smoke-'));
  try {
    const calls = path.join(directory, 'calls.jsonl');
    const stub = tool => `#!${process.execPath}
const fs=require('node:fs');const a=process.argv.slice(2);const tool=${JSON.stringify(tool)};
fs.appendFileSync(process.env.FIXTURE_CALLS,JSON.stringify({tool,args:a})+'\\n');
if(tool==='go'&&a.join(' ')==='env GOHOSTARCH'){console.log(process.env.FIXTURE_HOST_ARCH);process.exit(0)}
if(tool==='go')process.exit(2);
if(tool==='curl')console.log('{"allowed":true,"reason":0}');
process.exit(0);
`;
    for (const tool of ['go', 'docker', 'curl']) {
      const file = path.join(directory, tool);
      fs.writeFileSync(file, stub(tool)); fs.chmodSync(file, 0o755);
    }
    fs.mkdirSync(path.join(directory, 'dist'));
    for (const arch of ['amd64', 'arm64']) {
      const file = path.join(directory, 'dist', `spctl_linux_${arch}`);
      fs.writeFileSync(file, `#!${process.execPath}
const fs=require('node:fs');const a=process.argv.slice(2);
fs.appendFileSync(process.env.FIXTURE_CALLS,JSON.stringify({tool:'spctl',arch:${JSON.stringify(arch)},args:a})+'\\n');
if(a.includes('get'))console.log('[{"name":"smoke","policies":[{}]}]');
`);
      fs.chmodSync(file, 0o755);
    }
    const result = spawnSync('bash', ['-c', script('Verify the container image builds and starts')], {
      cwd: directory, encoding: 'utf8', env: {
        ...process.env, PATH: `${directory}${path.delimiter}${process.env.PATH}`,
        FIXTURE_CALLS: calls, FIXTURE_HOST_ARCH: hostArch, CONTAINER_CONTEXT: 'container-context',
      },
    });
    const records = fs.existsSync(calls) ? fs.readFileSync(calls, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
    return {result, records};
  } finally {
    fs.rmSync(directory, {recursive: true, force: true});
  }
}

test('container smoke selects the host spctl binary and native container image on both release architectures', () => {
  for (const arch of ['amd64', 'arm64']) {
    const {result, records} = runSmoke(arch);
    assert.equal(result.status, 0, `${arch}: ${result.stderr}`);
    const spctl = records.filter(record => record.tool === 'spctl');
    assert.equal(spctl.length, 3, arch);
    assert.ok(spctl.every(record => record.arch === arch), JSON.stringify(spctl));
    const loads = records.filter(record => record.tool === 'docker' && record.args[0] === 'buildx' && record.args[1] === 'build' && record.args.includes('--load'));
    assert.equal(loads.length, 1, arch);
    assert.equal(loads[0].args[loads[0].args.indexOf('--platform') + 1], `linux/${arch}`);
    assert.ok(loads[0].args.includes('speedle:smoke'));
    const starts = records.filter(record => record.tool === 'docker' && record.args[0] === 'run' && record.args.includes('-d'));
    assert.equal(starts.length, 2, arch);
    assert.ok(starts[0].args.includes('speedle-pms'), JSON.stringify(starts[0].args));
    assert.ok(starts[1].args.includes('speedle-ads'), JSON.stringify(starts[1].args));
    assert.ok(starts.every(record => record.args.includes('speedle:smoke') && record.args.includes('speedle-smoke:/data')));
  }
  const unsupported = runSmoke('s390x');
  assert.notEqual(unsupported.result.status, 0);
  assert.match(unsupported.result.stderr, /Unsupported release smoke host architecture: s390x/);
  assert.ok(!unsupported.records.some(record => record.tool === 'docker' || record.tool === 'spctl'));
});
