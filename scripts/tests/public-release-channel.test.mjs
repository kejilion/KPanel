import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { expectedReleaseAssetNames } from '../stable-release-recovery.mjs';

const repoRoot = new URL('../../', import.meta.url);
const moduleUrl = new URL('../verify-github-public-release.mjs', import.meta.url).href;
const registry = 'https://registry-1.docker.io/v2/kjlion/kejilion-panel';
const repositoryApi = 'https://api.github.com/repos/kejilion/KPanel';
const releaseApi = `${repositoryApi}/releases`;
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const digestOf = bytes => 'sha256:' + sha256(bytes);

function runControllerOverlay(options = {}) {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-verifier-overlay-'));
  const evidence = mkdtempSync(join(tmpdir(), 'kpanel-verifier-proof-'));
  const runner = 'packaging/release-runner/Dockerfile';
  const fixturePath = 'packaging/tests/release-browser/real-feature-public.sh';
  const browserPath = 'packaging/tests/release-browser/real-feature-browser-public.cjs';
  const entrypoint = 'packaging/tests/release-browser/run.sh';
  const git = (...args) => {
    const result = spawnSync('git', ['-C', root, ...args], { encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    return result.stdout.trim();
  };
  try {
    git('init', '--quiet');
    for (const path of [runner, fixturePath, browserPath, entrypoint]) {
      mkdirSync(join(root, path, '..'), { recursive: true });
      writeFileSync(join(root, path), 'original verifier\n');
    }
    writeFileSync(join(root, 'VERSION'), '1.28.0-rc.3\n');
    writeFileSync(join(root, 'Dockerfile'), 'immutable product\n');
    git('add', '.');
    const commit = message => git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
      'commit', '--quiet', '-m', message);
    commit('published candidate');
    const candidate = git('rev-parse', 'HEAD');
    writeFileSync(join(root, fixturePath), 'corrected verifier\n');
    if (options.missingEntrypoint) git('rm', entrypoint);
    else writeFileSync(join(root, entrypoint), 'corrected explicit environment\n');
    if (options.missingBrowser) git('rm', browserPath);
    else writeFileSync(join(root, browserPath), 'corrected browser companion\n');
    git('add', fixturePath, ...(options.missingBrowser ? [] : [browserPath]), ...(options.missingEntrypoint ? [] : [entrypoint])); commit('trusted verifier');
    const controller = git('rev-parse', 'HEAD');
    git('update-ref', 'refs/remotes/origin/main', options.untrusted ? candidate : controller);
    git('checkout', '--quiet', '--detach', candidate);
    if (options.dirty) writeFileSync(join(root, 'Dockerfile'), 'unapproved edit\n');
    const workflow = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8').replace(/\r\n/g, '\n');
    const source = workflow.split('name: Bind controller-owned test runner recipe')[1]
      .split("<<'NODE'\n")[1].split('\n          NODE')[0].replace(/^          /gm, '');
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', source], {
      cwd: root, encoding: 'utf8', env: { ...process.env, GITHUB_SHA: controller, EXPECTED_SHA: candidate,
        EXPECTED_VERSION: '1.28.0-rc.3', GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', PUBLIC_EVIDENCE: evidence },
    });
    if (options.dirty || options.untrusted || options.missingBrowser || options.missingEntrypoint) {
      assert.notEqual(result.status, 0);
      assert.equal(existsSync(join(evidence, 'test-fixture-source.json')), false);
      assert.equal(readFileSync(join(root, fixturePath), 'utf8'), 'original verifier\n');
      assert.equal(readFileSync(join(root, browserPath), 'utf8'), 'original verifier\n');
      assert.equal(existsSync(join(evidence, 'test-browser-source.json')), false);
      assert.equal(readFileSync(join(root, entrypoint), 'utf8'), 'original verifier\n');
      assert.equal(existsSync(join(evidence, 'test-entrypoint-source.json')), false);
    } else {
      assert.equal(result.status, 0, result.stderr);
      assert.equal(git('rev-parse', 'HEAD'), candidate);
      assert.deepEqual(git('diff', '--name-only').split('\n').sort(), [fixturePath, browserPath, entrypoint].sort());
      assert.equal(readFileSync(join(root, 'Dockerfile'), 'utf8'), 'immutable product\n');
      assert.equal(readFileSync(join(root, 'VERSION'), 'utf8'), '1.28.0-rc.3\n');
      const record = JSON.parse(readFileSync(join(evidence, 'test-fixture-source.json')));
      assert.equal(record.candidate, candidate); assert.equal(record.controller, controller);
      assert.equal(record.path, fixturePath); assert.equal(record.productImageUnchanged, true);
      assert.equal(record.original, 'original verifier\n'); assert.equal(record.recipe, 'corrected verifier\n');
      assert.equal(record.originalSha256, sha256(Buffer.from(record.original)));
      assert.equal(record.recipeSha256, sha256(Buffer.from(record.recipe)));
      assert.match(record.diff, /corrected verifier/);
      assert.ok(existsSync(join(evidence, 'test-runner-recipe.json')));
      const browserRecord = JSON.parse(readFileSync(join(evidence, 'test-browser-source.json')));
      assert.equal(browserRecord.path, browserPath); assert.equal(browserRecord.candidate, candidate);
      assert.equal(browserRecord.controller, controller); assert.equal(browserRecord.productImageUnchanged, true);
      assert.equal(browserRecord.original, 'original verifier\n'); assert.equal(browserRecord.recipe, 'corrected browser companion\n');
      assert.equal(browserRecord.originalSha256, sha256(Buffer.from(browserRecord.original)));
      assert.equal(browserRecord.recipeSha256, sha256(Buffer.from(browserRecord.recipe)));
      assert.match(browserRecord.diff, /corrected browser companion/);
      const entryRecord = JSON.parse(readFileSync(join(evidence, 'test-entrypoint-source.json')));
      assert.equal(entryRecord.path, entrypoint); assert.equal(entryRecord.candidate, candidate);
      assert.equal(entryRecord.controller, controller); assert.equal(entryRecord.productImageUnchanged, true);
      assert.equal(entryRecord.original, 'original verifier\n'); assert.equal(entryRecord.recipe, 'corrected explicit environment\n');
      assert.equal(entryRecord.originalSha256, sha256(Buffer.from(entryRecord.original)));
      assert.equal(entryRecord.recipeSha256, sha256(Buffer.from(entryRecord.recipe)));
      assert.match(entryRecord.diff, /corrected explicit environment/);
    }
  } finally {
    rmSync(root, { recursive: true, force: true }); rmSync(evidence, { recursive: true, force: true });
  }
}

test('read-only controller fixture overlay preserves product identity and records exact bytes', () => runControllerOverlay());
test('read-only controller fixture overlay rejects dirty product source', () => runControllerOverlay({ dirty: true }));
test('read-only controller fixture overlay rejects a controller outside trusted main', () => runControllerOverlay({ untrusted: true }));

test('read-only controller fixture overlay rejects a missing browser companion before any overlay', () => runControllerOverlay({ missingBrowser: true }));
test('read-only controller fixture overlay rejects a missing entrypoint before any overlay', () => runControllerOverlay({ missingEntrypoint: true }));

test('public acceptance passes only explicit required inputs through an environment-clearing sudo', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-verifier-env-'));
  const harness = join(root, 'harness.sh');
  const entrypoint = fileURLToPath(new URL('../../packaging/tests/release-browser/run.sh', import.meta.url));
  const required = {
    GITHUB_ACTIONS: 'true', RUNNER_ENVIRONMENT: 'github-hosted', RUNNER_TEMP: '',
    EXPECTED_VERSION: '1.31.0-rc.2', EXPECTED_SHA: 'a'.repeat(40), IMAGE_DIGEST: 'sha256:' + 'b'.repeat(64),
    PREVIOUS_PREVIEW_VERSION: '1.31.0-rc.1', PREVIOUS_PREVIEW_IMAGE: 'docker.io/example@sha256:' + 'c'.repeat(64),
    EXPECTED_NOTES: '2\n literal $value `command` "quotes"', EXPECTED_UPGRADE_NOTES: '0', EXPECTED_STABLE_VERSION: '1.30.0',
  };
  try {
    writeFileSync(harness, `#!/usr/bin/env bash
set -Eeuo pipefail
probe=$2; count=0
npm() { printf 'npm\\n' >> "$probe.activity"; }
docker() { printf 'docker\\n' >> "$probe.activity"; }
sudo() {
  if [[ "$1" = chown ]]; then return; fi
  test "$1" = env && test "$2" = -i
  shift 2
  local assignments=()
  while [[ "$1" == *=* ]]; do assignments+=("$1"); shift; done
  test "$1" = timeout && test "$2" = --kill-after=30s && test "$3" = 15m && test "$4" = bash
  shift 4
  count=$((count + 1))
  env -i "\${assignments[@]}" bash -c 'env -0 > "$1"; printf "%s\\0" "\${@:3}" >> "$2"' _ "$probe-$count.env" "$probe.calls" "$@"
}
source "$1"
`);
    for (const missing of [null, ...Object.keys(required)]) {
      const owned = join(root, missing ?? 'complete inputs'); mkdirSync(owned);
      const probe = join(owned, 'probe');
      const env = { ...process.env, ...required, RUNNER_TEMP: owned.replaceAll('\\', '/'),
        GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', PUBLIC_EVIDENCE: join(owned, 'evidence'),
        GITHUB_STEP_SUMMARY: join(owned, 'summary'), GH_TOKEN: 'secret-must-not-pass',
        GITHUB_TOKEN: 'another-secret', UNRELATED_KEEP: 'sudo-env-keep-must-not-pass', BASH_ENV: '' };
      // No real package installation, Docker operation, sudo or browser journey runs in this fixture.
      if (missing) delete env[missing];
      mkdirSync(env.PUBLIC_EVIDENCE);
      const result = spawnSync(process.execPath, [fileURLToPath(new URL('../run-repo-bash.mjs', import.meta.url)), harness, entrypoint, probe],
        { encoding: 'utf8', env, timeout: 15_000 });
      assert.ifError(result.error);
      if (missing) {
        assert.notEqual(result.status, 0, missing);
        assert.equal(existsSync(probe + '.activity'), false, missing + ' must fail before dependencies');
        assert.equal(existsSync(probe + '.calls'), false, missing + ' must fail before sudo');
        continue;
      }
      assert.equal(result.status, 0, result.stderr);
      for (let index = 1; index <= 3; index++) {
        const observed = Object.fromEntries(readFileSync(probe + '-' + index + '.env', 'utf8').split('\0').filter(Boolean)
          .map(assignment => { const at = assignment.indexOf('='); return [assignment.slice(0, at), assignment.slice(at + 1)]; }));
        for (const name of Object.keys(required)) assert.equal(observed[name], env[name], name);
        assert.equal(observed.RELEASE_TEST_RUNNER, 'kpanel-release-test:123-1');
        assert.equal(observed.RELEASE_BROWSER_RUNTIME, owned.replaceAll('\\', '/') + '/kpanel-browser-runtime');
        assert.equal(observed.PATH, '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin');
        for (const name of ['GH_TOKEN', 'GITHUB_TOKEN', 'UNRELATED_KEEP', 'PUBLIC_EVIDENCE', 'BASH_ENV']) assert.equal(observed[name], undefined, name);
      }
      const calls = readFileSync(probe + '.calls', 'utf8').replaceAll('\\', '/').split('\0');
      assert.ok(calls.some(value => value.endsWith('/real-feature-public.sh')));
      assert.equal(calls.filter(value => value.endsWith('/public-browser-complete.sh')).length, 2);
      for (const mode of ['public', 'published', 'upgrade']) assert.ok(calls.includes(mode), mode);
      assert.equal(existsSync(probe + '-4.env'), false);
    }
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('real public fixture binds script checks to the canonical frozen-source contract', () => {
  const source = readFileSync(new URL('../../packaging/tests/release-browser/real-feature-public.sh', import.meta.url), 'utf8');
  assert.match(source, /KPANEL_REPO_ROOT="\$repo" bash "\$repo\/scripts\/check-managed-script-contract\.sh" "\$image"/);
  assert.match(source, /printf '%s  %s\\n' "\$script_sha256" "\$data\/app-home\/kpanel\/bin\/kejilion\.sh" \| sha256sum -c -/);
  assert.doesNotMatch(source, /39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1/);
});

function fixture(version, options = {}) {
  const routes = {};
  const addRaw = (url, bytes, headers = {}, status = 200) => {
    routes[url] = { body: Buffer.from(bytes).toString('base64'), headers, status };
  };
  const addJson = (url, value, headers = {}) => addRaw(url, JSON.stringify(value), headers);
  const addManifest = (ref, value) => {
    const bytes = Buffer.from(JSON.stringify(value));
    const digest = digestOf(bytes);
    addRaw(`${registry}/manifests/${ref}`, bytes, { 'docker-content-digest': digest });
    return digest;
  };
  const dockerfile = readFileSync(new URL('../../Dockerfile', import.meta.url), 'utf8');
  const scriptRevision = dockerfile.match(/io\.kejilion\.script\.revision="([^"]+)"/)?.[1];
  const scriptDigest = dockerfile.match(/io\.kejilion\.script\.sha256="([^"]+)"/)?.[1];
  assert.ok(scriptRevision && scriptDigest, 'Dockerfile pins the managed script identity');

  const image = (imageVersion, arch) => {
    const config = Buffer.from(JSON.stringify({ architecture: arch, config: { Labels: {
      'org.opencontainers.image.version': imageVersion,
      'org.opencontainers.image.revision': 'b'.repeat(40),
      'io.kejilion.script.revision': scriptRevision,
      'io.kejilion.script.sha256': scriptDigest,
    } } }));
    const configDigest = digestOf(config);
    addRaw(`${registry}/blobs/${configDigest}`, config);
    const child = { schemaVersion: 2, config: { digest: configDigest }, layers: [] };
    const bytes = Buffer.from(JSON.stringify(child));
    const childDigest = digestOf(bytes);
    addRaw(`${registry}/manifests/${childDigest}`, bytes, { 'docker-content-digest': childDigest });
    return { digest: childDigest, size: bytes.length, platform: { os: 'linux', architecture: arch } };
  };
  const index = imageVersion => ({ schemaVersion: 2, manifests: [image(imageVersion, 'amd64'), image(imageVersion, 'arm64')] });
  const stableIndex = index('1.26.0');
  const previousPreviewIndex = index('1.27.0-rc.23');
  const targetIndex = index(version);
  const oldStableDigest = addManifest('stable-before-fixture', stableIndex);
  const previousPreviewDigest = addManifest('preview-before-fixture', previousPreviewIndex);
  const targetDigest = addManifest('target-fixture', targetIndex);
  const isPreview = version.includes('-rc.');
  const mode = options.mode ?? 'verify';
  const latestChannel = mode === 'snapshot' ? 'stable' : (options.latestChannel ?? (isPreview ? 'stable' : 'target'));
  const previewChannel = mode === 'snapshot' ? 'previous' : (options.previewChannel ?? (isPreview ? 'target' : 'previous'));
  const latestIndex = latestChannel === 'target' ? targetIndex : stableIndex;
  const previewIndex = previewChannel === 'target' ? targetIndex : previousPreviewIndex;
  addManifest('latest', latestIndex);
  addManifest('preview', previewIndex);
  addManifest(version, targetIndex);
  addJson('https://auth.docker.io/token?service=registry.docker.io&scope=repository:kjlion/kejilion-panel:pull', { token: 'fixture' });
  addJson(`${releaseApi}/latest`, { tag_name: options.latestTag ?? (mode === 'snapshot' || isPreview ? 'v1.26.0' : `v${version}`) });

  // Invalid-version cases must still reach the verifier's own input rejection.
  let inventory;
  try { inventory=expectedReleaseAssetNames(version); } catch { inventory=expectedReleaseAssetNames('1.0.0'); }
  const binaryNames = inventory.filter(name => !['SHA256SUMS','LICENSE','THIRD_PARTY_NOTICES.md','KPANEL_MODIFIED_COMPONENTS.json'].includes(name));
  const assetBytes = new Map(binaryNames.map(name => [name, Buffer.from(`fixture:${name}\n`)]));
  const sums = binaryNames.map(name => `${sha256(assetBytes.get(name))}  ${name}`).join('\n') + '\n';
  assetBytes.set('SHA256SUMS', Buffer.from(sums));
  assetBytes.set('LICENSE', Buffer.from('fixture license\n'));
  assetBytes.set('THIRD_PARTY_NOTICES.md', Buffer.from('fixture notices\n'));
  assetBytes.set('KPANEL_MODIFIED_COMPONENTS.json', Buffer.from('{}\n'));
  const assets = [...assetBytes].map(([name, bytes]) => {
    addRaw(`https://github.com/kejilion/KPanel/releases/download/v${version}/${name}`, bytes);
    return { name, browser_download_url: `https://github.com/kejilion/KPanel/releases/download/v${version}/${name}`,
      size: bytes.length, digest: digestOf(bytes) };
  });
  const release = { tag_name: `v${version}`, draft: false, prerelease: options.prerelease ?? isPreview, assets,
    body: `### 版本更新内容\n#### 新增\n- Stable channel fixture\n#### 升级注意事项\n- Keep existing data\n\n${targetDigest}` };
  addJson(`${releaseApi}/tags/v${version}`, release);
  addJson(`${repositoryApi}/git/ref/tags/v${version}`, { object: { type: 'tag', url: `${repositoryApi}/git/tags/fixture` } });
  addJson(`${repositoryApi}/git/tags/fixture`, { object: { type: 'commit', sha: 'b'.repeat(40) } });
  return {
    routes,
    targetDigest,
    snapshot: { tag: 'v1.26.0', digest: oldStableDigest,
      previousPreview: { version: '1.27.0-rc.23', digest: previousPreviewDigest } },
  };
}

function runVerifier(mode, version, options = {}) {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-public-channel-'));
  const evidencePath = join(root, 'evidence');
  const rootEnv = root.replaceAll('\\', '/');
  const evidenceEnv = evidencePath.replaceAll('\\', '/');
  const current = fixture(version, { ...options, mode });
  const artifactDirectory = join(root, 'snapshot-artifact');
  if (mode === 'restore-artifact') {
    mkdirSync(artifactDirectory);
    const snapshotBytes = Buffer.from(options.snapshotBytes ?? `${JSON.stringify(options.artifactSnapshot ?? current.snapshot)}\n`);
    writeFileSync(join(artifactDirectory, 'stable-before.json'), snapshotBytes);
    writeFileSync(join(artifactDirectory, 'publication-identity.json'), JSON.stringify({
      schemaVersion: 1, candidate: options.identityCandidate ?? 'b'.repeat(40), version: options.identityVersion ?? version,
      controllerSha: 'c'.repeat(40), runId: options.identityRunId ?? '123', attempt: options.identityAttempt ?? '1',
      snapshotSha256: options.identitySnapshotSha256 ?? sha256(snapshotBytes), phase: options.identityPhase ?? 'before-publication',
    }));
  } else if (mode !== 'snapshot' && mode !== 'restore-snapshot') {
    mkdirSync(evidencePath);
    writeFileSync(join(evidencePath, 'stable-before.json'), JSON.stringify(options.snapshot ?? current.snapshot));
  }
  const bootstrap = `const routes = new Map(Object.entries(JSON.parse(process.env.PUBLIC_FIXTURE_ROUTES)));
globalThis.fetch = async input => {
  const url = String(input), route = routes.get(url);
  if (!route) throw new Error('Unexpected fixture request: ' + url);
  return new Response(Buffer.from(route.body, 'base64'), { status: route.status, headers: route.headers });
};
process.argv[2] = process.env.PUBLIC_FIXTURE_MODE;
await import(${JSON.stringify(moduleUrl)});`;
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', bootstrap], {
    cwd: repoRoot,
    encoding: 'utf8',
    timeout: 20_000,
    env: { ...process.env, GITHUB_ACTIONS: 'true', RUNNER_ENVIRONMENT: 'github-hosted', GITHUB_REPOSITORY: 'kejilion/KPanel',
      GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', GITHUB_SHA: 'c'.repeat(40), EXPECTED_SHA: 'b'.repeat(40),
      EXPECTED_VERSION: version, IMAGE_DIGEST: current.targetDigest, RUNNER_TEMP: rootEnv,
      PUBLIC_EVIDENCE: evidenceEnv, GITHUB_ENV: `${rootEnv}/github-env`, GITHUB_OUTPUT: `${rootEnv}/github-output`,
      SNAPSHOT_ARTIFACT_DIR: `${rootEnv}/snapshot-artifact`, PUBLICATION_RUN_ID: options.publicationRunId ?? '123',
      PUBLICATION_ATTEMPT: options.publicationAttempt ?? '1',
      STABLE_SNAPSHOT: JSON.stringify(options.snapshot ?? { ...current.snapshot, tag: `v${version}` }),
      PUBLIC_FIXTURE_MODE: mode, PUBLIC_FIXTURE_ROUTES: JSON.stringify(current.routes),
    },
  });
  try {
    return {
      status: result.status,
      error: result.error,
      stdout: result.stdout,
      stderr: result.stderr,
      release: existsSync(join(evidencePath, 'public-release.json')) ? JSON.parse(readFileSync(join(evidencePath, 'public-release.json'))) : null,
      snapshot: existsSync(join(evidencePath, 'stable-before.json')) ? JSON.parse(readFileSync(join(evidencePath, 'stable-before.json'))) : null,
      identity: existsSync(join(evidencePath, 'publication-identity.json')) ? JSON.parse(readFileSync(join(evidencePath, 'publication-identity.json'))) : null,
      env: existsSync(join(root, 'github-env')) ? readFileSync(join(root, 'github-env'), 'utf8') : '',
    };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test('stable and preview verification bind their own channels and preserve the other channel', () => {
  const stable = runVerifier('verify', '1.27.0', { latestChannel: 'target', previewChannel: 'previous' });
  assert.ifError(stable.error);
  assert.equal(stable.status, 0, stable.stderr);
  assert.equal(stable.release.version, '1.27.0');
  assert.equal(stable.env.includes('EXPECTED_STABLE_VERSION=1.27.0'), true);

  const preview = runVerifier('verify', '1.27.0-rc.24', { latestChannel: 'stable', previewChannel: 'target' });
  assert.ifError(preview.error);
  assert.equal(preview.status, 0, preview.stderr);
  assert.equal(preview.release.version, '1.27.0-rc.24');
  assert.equal(preview.env.includes('EXPECTED_STABLE_VERSION=1.26.0'), true);
});

test('public verifier rejects a missing stable promotion, preview drift, and changed stable baseline', () => {
  const unpromoted = runVerifier('verify', '1.27.0', { latestChannel: 'stable', previewChannel: 'previous' });
  assert.notEqual(unpromoted.status, 0);
  assert.match(unpromoted.stderr, /Version\/channel digest mismatch/);

  const previewDrift = runVerifier('verify', '1.27.0', { latestChannel: 'target', previewChannel: 'target' });
  assert.notEqual(previewDrift.status, 0);
  assert.match(previewDrift.stderr, /Stable publication changed preview/);

  const stableDrift = runVerifier('verify', '1.27.0-rc.24', { latestChannel: 'target', previewChannel: 'target', latestTag: 'v1.27.0' });
  assert.notEqual(stableDrift.status, 0);
  assert.match(stableDrift.stderr, /Preview publication changed the stable channel/);
});

test('stable snapshot captures the prior Latest and preview, while recovery cannot invent a baseline', () => {
  const snapshot = runVerifier('snapshot', '1.27.0');
  assert.ifError(snapshot.error);
  assert.equal(snapshot.status, 0, snapshot.stderr);
  assert.equal(snapshot.snapshot.tag, 'v1.26.0');
  assert.equal(snapshot.snapshot.previousPreview.version, '1.27.0-rc.23');
  assert.equal(snapshot.identity.phase, 'before-publication');
  assert.equal(snapshot.identity.candidate, 'b'.repeat(40));

  const recovery = runVerifier('restore-snapshot', '1.27.0', { snapshot: { ...fixture('1.27.0').snapshot, tag: 'v1.27.0' } });
  assert.notEqual(recovery.status, 0);
  assert.match(recovery.stderr, /original pre-publication baseline/);
  assert.equal(recovery.snapshot, null);
});

test('manual recovery restores only the exact original publication artifact', () => {
  const valid = runVerifier('restore-artifact', '1.27.0');
  assert.ifError(valid.error);
  assert.equal(valid.status, 0, valid.stderr);
  assert.deepEqual(valid.snapshot, fixture('1.27.0').snapshot);
  assert.match(valid.env, /PREVIOUS_PREVIEW_VERSION=1\.27\.0-rc\.23/);

  for (const [options, message] of [
    [{ identityRunId: '999' }, /identity mismatch/],
    [{ identityCandidate: 'a'.repeat(40) }, /identity mismatch/],
    [{ identityVersion: '1.27.1' }, /identity mismatch/],
    [{ identitySnapshotSha256: '0'.repeat(64) }, /identity mismatch/],
    [{ snapshotBytes: '{"tag":"v1.26.0"}\n', identitySnapshotSha256: sha256(Buffer.from(`${JSON.stringify(fixture('1.27.0').snapshot)}\n`)) }, /identity mismatch/],
  ]) {
    const rejected = runVerifier('restore-artifact', '1.27.0', options);
    assert.notEqual(rejected.status, 0);
    assert.match(rejected.stderr, message);
    assert.equal(rejected.snapshot, null);
  }
});

test('manual recovery downloads the exact original run artifact instead of accepting a snapshot JSON input', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8');
  const dispatch = workflow.split('  workflow_dispatch:')[1].split('\npermissions:')[0];
  assert.match(dispatch, /publication-run-id:/);
  assert.match(dispatch, /publication-attempt:/);
  assert.doesNotMatch(dispatch, /stable-snapshot:/);
  assert.match(workflow, /actions\/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333/);
  assert.match(workflow, /restore-artifact/);
});

test('public verifier rejects noncanonical version identities', () => {
  const invalid = runVerifier('verify', '01.27.0');
  assert.notEqual(invalid.status, 0);
  assert.match(invalid.stderr, /Exact release identity required/);
});
