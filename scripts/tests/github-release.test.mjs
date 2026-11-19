import assert from 'node:assert/strict';
import test from 'node:test';
import { existsSync, readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { resolveBash } from '../run-repo-bash.mjs';
import { validateGitHubRun } from '../github-release-l3.mjs';
import { checkEnvironment, loadPolicy } from '../check-environment-policy.mjs';
import { classifyRecoveryState, createReleaseAssetManifest, expectedReleaseAssetNames, validateFailedPublication,
  findReleaseByTag, freezeReleaseAssets, validatePublicationArtifacts, validateRecoveryInputs, validateReleaseAssetInventory } from '../stable-release-recovery.mjs';

const valid = { GITHUB_ACTIONS: 'true', RUNNER_ENVIRONMENT: 'github-hosted', GITHUB_REPOSITORY: 'kejilion/KPanel',
  GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', GITHUB_SHA: 'a'.repeat(40) };

const workflowStep = (workflow, name) => {
  const step = workflow.split(`      - name: ${name}\n`)[1]?.split('\n      - ')[0];
  assert.ok(step, `step ${name} exists`);
  const body = step.split('        run: |\n')[1];
  assert.ok(body, `step ${name} has a shell body`);
  return body.split('\n').filter(line => line.startsWith('          ')).map(line => line.slice(10)).join('\n');
};

test('preview recovery inputs are preserved before publication and bind original snapshot bytes', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release.yml', import.meta.url), 'utf8');
  const start = workflow.indexOf('      - name: Freeze exact release asset identity before channel promotion');
  const promote = workflow.indexOf('      - name: Promote image to its release channel');
  assert.ok(start > workflow.indexOf('      - name: Validate final release notes'));
  assert.ok(start < promote);
  assert.match(workflow.slice(start, promote), /node scripts\/stable-release-recovery\.mjs freeze-assets/);
  assert.match(workflow.slice(start, promote), /asset identity before channel promotion/);
  const artifactStart = workflow.indexOf('      - name: Preserve exact publication recovery inputs before channel promotion');
  assert.ok(artifactStart > start && artifactStart < promote);
  const upload = workflow.slice(artifactStart, promote);
  assert.match(upload, /if-no-files-found: error/);
  assert.match(upload, /snapshot-\$\{\{ github.run_id \}\}-\$\{\{ github.run_attempt \}\}/);
  assert.match(upload, /stable-before.json/);
  assert.match(upload, /publication-identity.json/);
  assert.match(upload, /\$\{\{ env\.PUBLIC_EVIDENCE \}\}\/release-assets\.json/);
  assert.doesNotMatch(upload, /continue-on-error/);
  assert.match(workflow.slice(workflow.indexOf('      - name: Preserve release metadata including failures')),
    /\$\{\{ env\.PUBLIC_EVIDENCE \}\}\/release-assets\.json/);
  const root = mkdtempSync(join(tmpdir(), 'kpanel-frozen-snapshot-')).replaceAll('\\', '/');
  try {
    const moduleUrl = new URL('../verify-github-public-release.mjs', import.meta.url).href;
    const script = `import {createHash} from 'node:crypto';
const digest = body => 'sha256:'+createHash('sha256').update(JSON.stringify(body)).digest('hex');
const image = {config:{digest:'sha256:'+'c'.repeat(64)}};
const index = {manifests:[{platform:{os:'linux',architecture:'amd64'},digest:digest(image)}]};
globalThis.fetch = async url => {
  if(url.includes('auth.docker.io')) return Response.json({token:'fixture'});
  if(url.includes('/releases/latest')) return Response.json({tag_name:'v1.26.0'});
  if(url.includes('/blobs/')) return Response.json({config:{Labels:{'org.opencontainers.image.version':'1.27.0-rc.4'}}});
  const body = url.endsWith(digest(image)) ? image : index;
  return new Response(JSON.stringify(body),{headers:{'docker-content-digest':digest(body)}});
};process.argv[2]='snapshot';await import(${JSON.stringify(moduleUrl)});`;
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', script], {
      encoding: 'utf8', timeout: 5000, env: { ...process.env, ...valid, EXPECTED_VERSION: '1.27.0-rc.5',
        EXPECTED_SHA: 'b'.repeat(40), RUNNER_TEMP: root, PUBLIC_EVIDENCE: root + '/snapshot',
        GITHUB_OUTPUT: root + '/outputs', GITHUB_ENV: root + '/env' },
    });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    const raw = readFileSync(root + '/snapshot/stable-before.json');
    const identity = JSON.parse(readFileSync(root + '/snapshot/publication-identity.json'));
    assert.equal(identity.candidate, 'b'.repeat(40));
    assert.equal(identity.controllerSha, valid.GITHUB_SHA);
    assert.equal(identity.version, '1.27.0-rc.5');
    assert.equal(identity.runId, valid.GITHUB_RUN_ID);
    assert.equal(identity.attempt, valid.GITHUB_RUN_ATTEMPT);
    assert.equal(identity.snapshotSha256, createHash('sha256').update(raw).digest('hex'));
    assert.equal(JSON.parse(raw).previousPreview.version, '1.27.0-rc.4');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('release input validation uses the explicit tag when GitHub retains the caller main ref', () => {
  const read = name => readFileSync(new URL('../../' + name, import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  const workflow = read('.github/workflows/release.yml');
  assert.doesNotMatch(workflow, /GITHUB_REF_NAME/);
  assert.match(workflow, /RELEASE_TAG: \$\{\{ inputs\.tag \|\| github\.ref_name \}\}/);
  const root = mkdtempSync(join(tmpdir(), 'kpanel-release-ref-'));
  try {
    const version = read('VERSION').trim();
    const prefix = `git() {
case "$*" in
  'rev-parse HEAD'|"rev-parse $RELEASE_TAG^{commit}") printf '%s' "$RELEASE_SHA";;
  "cat-file -t refs/tags/$RELEASE_TAG") printf tag;;
  'merge-base --is-ancestor HEAD origin/main') return 0;;
  *) printf 'unexpected Git request: %s' "$*" >&2; return 1;;
esac
}\n`;
    const result = spawnSync(resolveBash(), ['--noprofile', '--norc', '-c', prefix + workflowStep(workflow, 'Validate release inputs')], {
      cwd: new URL('../../', import.meta.url), encoding: 'utf8', timeout: 10_000,
      env: { ...process.env, ...valid, GITHUB_REF_NAME: 'main', RELEASE_TAG: 'v' + version, RELEASE_SHA: 'b'.repeat(40),
        DOCKERHUB_IMAGE: 'kjlion/kejilion-panel', DOCKERHUB_USERNAME: 'kjlion', DOCKERHUB_TOKEN: 'fixture',
        RUNNER_TEMP: root, GITHUB_ENV: join(root, 'env'), GITHUB_OUTPUT: join(root, 'outputs') },
    });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    assert.match(readFileSync(join(root, 'outputs'), 'utf8'), new RegExp(`version=${version.replaceAll('.', '\\.')}`));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('reusable CI explicitly exports the frozen identity after runner default injection', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/ci.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  assert.doesNotMatch(workflow, /^\s+GITHUB_(?:SHA|REF_NAME):/m);
  const result = spawnSync(resolveBash(), ['--noprofile', '--norc', '-c',
    'make() { printf "%s|%s" "$GITHUB_SHA" "$GITHUB_REF_NAME"; }\n' + workflowStep(workflow, 'Run change-aware verification')], {
    encoding: 'utf8', timeout: 10_000,
    env: { ...process.env, GITHUB_SHA: 'a'.repeat(40), GITHUB_REF_NAME: 'main', KPANEL_CI_SHA: 'b'.repeat(40), KPANEL_CI_REF_NAME: 'release/validated-candidate' },
  });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, 'b'.repeat(40) + '|release/validated-candidate');
  const l3 = readFileSync(new URL('../../.github/workflows/release-l3.yml', import.meta.url), 'utf8');
  assert.doesNotMatch(l3, /^\s+GITHUB_(?:SHA|REF_NAME):/m);
  assert.match(l3, /export GITHUB_SHA="\$KPANEL_CANDIDATE_SHA" GITHUB_REF_NAME=release\/validated-candidate/);
});
test('CI reads real Git trailers and only raises the verification floor', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/ci.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  const body = workflowStep(workflow, 'Run change-aware verification');
  for (const [trailers, expected] of [
    ['', 'auto'], ['Verification-Level: L0', 'auto'], ['Verification-Level: L1', 'auto'],
    ['Verification-Level: L2', '2'], ['Verification-Level: L3', '2'],
    ['verification-level: L2', '2'], ['Verification-Level: garbage', null],
    ['Verification-Level: L2\nVerification-Level: L1', null],
    ['Verification-Level: L2\nVerification-Level: L2', null],
    ['Verification-Level: $(echo injected)', null], ['Verification-Level:', null],
  ]) {
    const result = spawnSync(resolveBash(), ['--noprofile', '--norc', '-c',
      'set -euo pipefail\nunset VERIFY_LEVEL\ngit() { if [[ "$1" == log ]]; then printf "%s" "$TEST_COMMIT_MESSAGE"; else command git "$@"; fi; }\n' +
      'make() { printf "%s" "${VERIFY_LEVEL:-auto}"; }\n' + body], {
      encoding: 'utf8', timeout: 10_000,
      env: { ...process.env, TEST_COMMIT_MESSAGE: 'Candidate\n\n' + trailers + '\n',
        KPANEL_CI_SHA: 'b'.repeat(40), KPANEL_CI_REF_NAME: 'feature/candidate' },
    });
    assert.ifError(result.error);
    if (expected === null) {
      assert.notEqual(result.status, 0, trailers);
      assert.equal(result.stdout, '', trailers);
    } else {
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stdout, expected, trailers);
    }
  }
});

test('L3 rejects self-hosted, fork, missing identity and unsupported platform', () => {
  validateGitHubRun(valid, 'linux');
  for (const [field, value] of [['GITHUB_ACTIONS', 'false'], ['RUNNER_ENVIRONMENT', 'self-hosted'], ['GITHUB_REPOSITORY', 'attacker/KPanel'], ['GITHUB_RUN_ID', ''], ['GITHUB_SHA', 'main']]) {
    assert.throws(() => validateGitHubRun({ ...valid, [field]: value }, 'linux'));
  }
  assert.throws(() => validateGitHubRun(valid, 'win32'));
});
test('GitHub environment never grants production access', () => {
  const policy = loadPolicy();
  assert.equal(checkEnvironment(policy, 'github-hosted', 'browser-validation').role, 'validation');
  for (const purpose of ['production-deploy', 'production-safety-check', 'staging-deploy']) {
    assert.throws(() => checkEnvironment(policy, 'github-hosted', purpose));
  }
});
test('preview pipeline binds trusted workflows, atomic Git writes and explicit downstream calls', () => {
  const read = path => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');
  const flow = read('.github/workflows/preview-release.yml');
  assert.match(flow, /l3:\s+needs: inputs/);
  assert.match(flow, /promote:\s+needs: \[inputs, candidate-ci, l3\]/);
  assert.match(flow, /needs: \[inputs, main-ci\]/);
  for (const entry of ['ci.yml', 'release-l3.yml', 'release.yml']) assert.ok(flow.includes(`uses: ./.github/workflows/${entry}`));
  assert.doesNotMatch(flow, /self-hosted|\bssh\b|DOCKERHUB_TOKEN/);
  const controller = read('scripts/github-preview-release.mjs');
  assert.match(controller, /merge-base', '--is-ancestor', 'origin\/main', sha/);
  assert.match(controller, /Version tag already exists/);
  assert.match(controller, /Candidate branch does not match/);
  assert.match(controller, /Release automation must be integrated on main/);
  assert.match(controller, /'ls-tree', '-r', ref/);
  assert.match(controller, /'packaging\/tests'/);
  assert.match(controller, /'Makefile'/);
  assert.match(controller, /'push', '--atomic'/);
  assert.match(controller, /force-with-lease=refs\/heads\/main/);
  assert.match(controller, /before !== process.env.RELEASE_BASE/);
  assert.match(controller, /Preview version must advance/);
  assert.doesNotMatch(controller, /shell:\s*true|console\.log\(.*auth/);
});

test('public acceptance retries cannot rebuild or repush the published image', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release.yml', import.meta.url), 'utf8');
  const publicJob = workflow.slice(workflow.indexOf('\n  public-acceptance:'), workflow.indexOf('\n  preview-completion:'));
  const verifier = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8');
  assert.match(publicJob, /needs: release/);
  assert.match(publicJob, /contents: read/);
  assert.doesNotMatch(publicJob, /build-push-action|DOCKERHUB_TOKEN|contents: write|gh release edit/);
  assert.match(publicJob, /uses: \.\/\.github\/workflows\/release-public-verify\.yml/);
  assert.doesNotMatch(verifier, /build-push-action|DOCKERHUB_TOKEN|contents: write|gh release edit/);
  assert.match(verifier, /workflow_call:/);
  assert.match(verifier, /workflow_dispatch:/);
  assert.match(verifier, /restore-snapshot/);
  assert.match(verifier, /actions\/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333/);
  assert.match(verifier, /publication-run-id:/);
  assert.match(verifier, /publication-attempt:/);
  assert.doesNotMatch(verifier.split('  workflow_dispatch:')[1].split('\npermissions:')[0], /stable-snapshot:/);
  assert.ok(workflow.indexOf('Reject an already occupied') < workflow.indexOf('- name: Build and push multi-architecture image'));
});

test('public acceptance runs for both channels and stable archival waits for its success', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release.yml', import.meta.url), 'utf8');
  const publicJob = workflow.slice(workflow.indexOf('\n  public-acceptance:'), workflow.indexOf('\n  preview-completion:'));
  assert.match(publicJob, /if:.*!failure\(\).* !cancelled\(\).*needs\.release\.result == 'success'/);
  assert.doesNotMatch(publicJob, /stable == 'false'/);
  const completion = workflow.slice(workflow.indexOf('\n  preview-completion:'));
  assert.match(completion, /needs: \[release, public-acceptance\]/);
  assert.match(completion, /if:.*!failure\(\).* !cancelled\(\).*contains\(inputs\.tag \|\| github\.ref_name, '-rc\.'\)/);
  assert.match(completion, /PUBLIC_RESULT: \$\{\{ needs\.public-acceptance\.result \}\}/);
  assert.match(completion, /run: test "\$PUBLIC_RESULT" = success/);
  const stableCompletion = workflow.slice(workflow.indexOf('\n  stable-completion:'));
  assert.match(stableCompletion, /needs: \[release, public-acceptance\]/);
  assert.match(stableCompletion, /needs\.public-acceptance\.result == 'success'.*needs\.release\.outputs\.stable == 'true'/);
  assert.ok(workflow.indexOf('\n  public-acceptance:') < workflow.indexOf('name: Archive published candidate branch'));
});

test('read-only recovery verifies the published source tag before executing candidate helpers', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  assert.match(workflow, /fetch-depth: 0/);
  assert.ok(workflow.indexOf('Validate frozen public source') < workflow.indexOf('node scripts/verify-github-public-release.mjs'));
  const version = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim();
  const prefix = `git() {
case "$*" in
  'rev-parse HEAD') printf '%s' "$EXPECTED_SHA";;
  "cat-file -t refs/tags/v$EXPECTED_VERSION") printf tag;;
  "rev-parse refs/tags/v$EXPECTED_VERSION^{commit}")
    if [[ "$WRONG_TAG" = 1 ]]; then printf '%040d' 0; else printf '%s' "$EXPECTED_SHA"; fi;;
  'merge-base --is-ancestor HEAD origin/main') return 0;;
  *) return 1;;
esac
}\n`;
  const scenarios = [
    { ref: 'refs/heads/main', run: '123', attempt: '1', expected: 0 },
    { ref: 'refs/heads/feature/unreviewed', run: '123', attempt: '1', expected: 1 },
    { ref: 'refs/heads/main', run: '123', attempt: '1', wrongTag: '1', expected: 1 },
    // A called verifier inherits Publish preview's workflow_dispatch event.
    { ref: 'refs/heads/main', snapshot: '{"tag":"v1.27.2"}', expected: 0 },
    { ref: 'refs/tags/v1.27.2', snapshot: '{"tag":"v1.27.1"}', expected: 0 },
    { ref: 'refs/heads/main', expected: 1 },
    { ref: 'refs/heads/main', snapshot: '{}', run: '123', expected: 1 },
    { ref: 'refs/heads/main', snapshot: '{}', attempt: '1', expected: 1 },
    { ref: 'refs/heads/main', run: 'invalid', attempt: '1', expected: 1 },
  ];
  for (const { ref, wrongTag = '0', run = '', attempt = '', snapshot = '', expected } of scenarios) {
    const result = spawnSync(resolveBash(), ['--noprofile', '--norc', '-c', prefix + workflowStep(workflow, 'Validate frozen public source')], {
      cwd: new URL('../../', import.meta.url), encoding: 'utf8', timeout: 10_000,
      env: { ...process.env, GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: ref, EXPECTED_VERSION: version,
        EXPECTED_SHA: 'b'.repeat(40), IMAGE_DIGEST: 'sha256:' + 'a'.repeat(64), PUBLICATION_RUN_ID: run,
        PUBLICATION_ATTEMPT: attempt, STABLE_SNAPSHOT: snapshot, WRONG_TAG: wrongTag },
    });
    assert.ifError(result.error);
    assert.equal(result.status, expected, result.stderr);
  }
});

test('inline publication and artifact recovery select their original snapshot source despite inherited events', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  const download = workflow.split('      - name: Download original publication snapshot\n')[1].split('\n      - ')[0];
  assert.match(download, /if: inputs\['publication-run-id'\] != ''/);
  assert.doesNotMatch(download, /github\.event_name/);
  const restore = workflowStep(workflow.slice(workflow.indexOf('      - name: Download original publication snapshot')), 'Restore frozen publication identity');
  for (const [run, expected] of [['', 'restore-snapshot'], ['123', 'restore-artifact']]) {
    const result = spawnSync(resolveBash(), ['--noprofile', '--norc', '-c', 'node() { printf "%s" "$2"; };\n' + restore], {
      encoding: 'utf8', timeout: 10_000,
      env: { ...process.env, GITHUB_EVENT_NAME: 'workflow_dispatch', PUBLICATION_RUN_ID: run },
    });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, expected);
  }
});

test('stable recovery requires an exact main dispatch identity and stable release inputs', () => {
  const input = { RECOVERY_OPERATION: 'publish-draft', PUBLICATION_RUN_ID: '123', PUBLICATION_ATTEMPT: '2',
    EXPECTED_SHA: 'a'.repeat(40), EXPECTED_VERSION: '1.27.0', IMAGE_DIGEST: 'sha256:' + 'b'.repeat(64),
    GITHUB_REF: 'refs/heads/main' };
  assert.deepEqual(validateRecoveryInputs(input), { operation: 'publish-draft', runId: '123', attempt: '2',
    candidate: 'a'.repeat(40), version: '1.27.0', digest: 'sha256:' + 'b'.repeat(64) });
  for (const [field, value] of [['GITHUB_REF', 'refs/heads/feature/recovery'], ['RECOVERY_OPERATION', 'rebuild-image'],
    ['PUBLICATION_ATTEMPT', '0'], ['EXPECTED_VERSION', '1.27.0-rc.23'], ['IMAGE_DIGEST', 'latest']]) {
    assert.throws(() => validateRecoveryInputs({ ...input, [field]: value }));
  }
});

test('stable recovery accepts only the exact image-promoted and GitHub-publish-failed attempt', () => {
  const expected = { runId: '123', attempt: '2', candidate: 'a'.repeat(40), version: '1.27.0', repository: 'kejilion/KPanel' };
  const controllerSha = expected.candidate;
  const identity = { controllerSha };
  const workflow = { id: 456, path: '.github/workflows/release.yml', state: 'active' };
  const run = { id: 123, run_attempt: 2, event: 'push', status: 'completed', conclusion: 'failure',
    head_sha: controllerSha, head_branch: 'v1.27.0', path: workflow.path, workflow_id: workflow.id };
  const jobs = { jobs: [{ name: 'release', conclusion: 'failure', steps: [
    { name: 'Promote image to its release channel', conclusion: 'success' },
    { name: 'Publish GitHub release', conclusion: 'failure' },
  ] }] };
  assert.doesNotThrow(() => validateFailedPublication({ run, workflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run: { ...run, run_attempt: 1 }, workflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run: { ...run, event: 'workflow_dispatch' }, workflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run: { ...run, head_sha: 'd'.repeat(40) }, workflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run: { ...run, head_branch: 'main' }, workflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run: { ...run, head_branch: 'v1.27.0-rc.23' }, workflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run, workflow, jobs, expected, identity: { controllerSha: 'c'.repeat(40) } }));
  const previewWorkflow = { ...workflow, path: '.github/workflows/preview-release.yml' };
  assert.throws(() => validateFailedPublication({ run: { ...run, path: previewWorkflow.path, event: 'workflow_dispatch' }, workflow: previewWorkflow, jobs, expected, identity }));
  assert.throws(() => validateFailedPublication({ run, workflow, jobs: { jobs: [{ ...jobs.jobs[0], name: 'publish / release' }] }, expected, identity }));
  assert.throws(() => validateFailedPublication({ run, workflow, jobs: { jobs: [{ ...jobs.jobs[0], steps: [
    { name: 'Promote image to its release channel', conclusion: 'failure' },
    { name: 'Publish GitHub release', conclusion: 'failure' },
  ] }] }, expected, identity }));
  assert.throws(() => validateFailedPublication({ run, workflow, jobs: { jobs: [{ ...jobs.jobs[0], steps: [
    { name: 'Freeze exact release asset identity before channel promotion', conclusion: 'failure' },
    { name: 'Promote image to its release channel', conclusion: 'skipped' },
    { name: 'Publish GitHub release', conclusion: 'skipped' },
  ] }] }, expected, identity }));
});

test('frozen legacy 15-asset receipts retain their exact original inventory', () => {
  const version='1.27.0';
  const assets=expectedReleaseAssetNames(version,{windowsCommandNode:false}).map((name,i)=>({name,size:100+i,digest:'sha256:'+String(i+1).padStart(64,'0')}));
  assert.equal(assets.length,15);
  const manifest={version,releaseId:42,assets};
  const release={id:42,tag_name:'v'+version,draft:true,prerelease:false,assets:assets.map(item=>({...item,state:'uploaded'}))};
  assert.doesNotThrow(()=>validateReleaseAssetInventory(release,manifest));
  assert.throws(()=>validateReleaseAssetInventory(release,{...manifest,assetSet:'windows-command-v1'}));
  assert.throws(()=>validateReleaseAssetInventory(release,{...manifest,assetSet:'unrecognized'}));
  assert.throws(()=>validateReleaseAssetInventory({...release,assets:release.assets.slice(1)},manifest));
});

test('importing the release inventory never dispatches CLI operations', () => {
  const url=new URL('../stable-release-recovery.mjs',import.meta.url).href;
  const source='process.argv[2]="unused"; await import('+JSON.stringify(url)+');';
  const result=spawnSync(process.execPath,['--input-type=module','-e',source],{encoding:'utf8',timeout:10_000});
  assert.equal(result.status,0,result.stderr);
  assert.equal(result.stdout,'');
});

test('original release asset inventory binds every uploaded name, byte digest and size', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-release-assets-'));
  const releaseDirectory = join(root, 'release');
  mkdirSync(releaseDirectory);
  const version = '1.27.0';
  const expected = { candidate: 'a'.repeat(40), version, digest: 'sha256:' + 'b'.repeat(64), runId: '123', attempt: '2' };
  const controllerSha = 'c'.repeat(40), snapshotSha256 = 'd'.repeat(64);
  try {
    for (const name of expectedReleaseAssetNames(version)) {
      const directory = ['LICENSE', 'THIRD_PARTY_NOTICES.md'].includes(name) ? root : releaseDirectory;
      writeFileSync(join(directory, name), Buffer.from('original:' + name));
    }
    assert.equal(existsSync(join(releaseDirectory, 'LICENSE')), false);
    const manifest = createReleaseAssetManifest({ releaseDirectory, expected, controllerSha, snapshotSha256, releaseId: 42 });
    assert.equal(manifest.assets.length, 19);
    assert.equal(manifest.imageDigest, expected.digest);
    const release = { id: 42, tag_name: 'v' + version, draft: true, prerelease: false,
      assets: manifest.assets.map(asset => ({ ...asset, state: 'uploaded' })) };
    assert.doesNotThrow(() => validateReleaseAssetInventory(release, manifest));
    assert.throws(() => validateReleaseAssetInventory({ ...release, id: 43 }, manifest));
    assert.throws(() => validateReleaseAssetInventory({ ...release, assets: release.assets.slice(1) }, manifest));
    assert.throws(() => validateReleaseAssetInventory({ ...release, assets: release.assets.map((asset, i) => i
      ? asset : { ...asset, size: asset.size + 1 }) }, manifest));
    assert.throws(() => createReleaseAssetManifest({ releaseDirectory, expected, controllerSha, releaseId: 42, snapshotSha256: 'bad' }));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('freeze-assets binds the original uploaded release inventory before writing the shared identity', async () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-freeze-release-assets-'));
  const evidence = join(root, 'evidence'), releaseDirectory = join(root, 'release');
  mkdirSync(evidence); mkdirSync(releaseDirectory);
  const candidate = 'a'.repeat(40), controllerSha = 'c'.repeat(40), digest = 'sha256:' + 'b'.repeat(64);
  const version = '1.27.0', runId = '123', attempt = '2';
  const snapshotBytes = Buffer.from('{"tag":"v1.26.0"}\n');
  const expected = { candidate, version, digest, runId, attempt };
  for (const name of expectedReleaseAssetNames(version)) {
    const directory = ['LICENSE', 'THIRD_PARTY_NOTICES.md'].includes(name) ? root : releaseDirectory;
    writeFileSync(join(directory, name), Buffer.from('original:' + name));
  }
  const finalNotes = Buffer.from('original frozen final notes ' + digest);
  writeFileSync(join(releaseDirectory, 'RELEASE_NOTES.md'), finalNotes);
  writeFileSync(join(evidence, 'stable-before.json'), snapshotBytes);
  writeFileSync(join(evidence, 'publication-identity.json'), JSON.stringify({ schemaVersion: 1,
    phase: 'before-publication', candidate, version, runId, attempt, controllerSha,
    snapshotSha256: createHash('sha256').update(snapshotBytes).digest('hex') }, null, 2) + '\n');
  const localManifest = createReleaseAssetManifest({ releaseDirectory, expected, controllerSha,
    snapshotSha256: createHash('sha256').update(snapshotBytes).digest('hex'), releaseId: 42 });
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (url, options) => {
    if (String(url).endsWith('/graphql')) {
      assert.equal(options.method, 'POST');
      assert.deepEqual(JSON.parse(options.body).variables, { owner: 'kejilion', name: 'KPanel', tag: 'v1.27.0' });
      return Response.json({ data: { repository: { release: { databaseId: 42 } } } });
    }
    assert.match(String(url), /releases\/42$/);
    return Response.json({ id: 42, tag_name: 'v1.27.0', draft: true, prerelease: false,
      assets: localManifest.assets.map(asset => ({ ...asset, state: 'uploaded' })) });
  };
  try {
    const manifest = await freezeReleaseAssets({ GITHUB_ACTIONS: 'true', GITHUB_REPOSITORY: 'kejilion/KPanel',
      GITHUB_SHA: controllerSha, GITHUB_RUN_ID: runId, GITHUB_RUN_ATTEMPT: attempt,
      EXPECTED_SHA: candidate, VERSION: version, IMAGE_DIGEST: digest, GH_TOKEN: 'fixture',
      GITHUB_API_URL: 'https://api.example.test', PUBLIC_EVIDENCE: evidence, ASSET_DIRECTORY: releaseDirectory });
    assert.equal(manifest.assets.length, 19);
    const rawManifest = readFileSync(join(evidence, 'release-assets.json'));
    const identity = JSON.parse(readFileSync(join(evidence, 'publication-identity.json'), 'utf8'));
    assert.equal(identity.assetManifestSha256, createHash('sha256').update(rawManifest).digest('hex'));
    assert.equal(identity.notesSha256, createHash('sha256').update(finalNotes).digest('hex'));
    assert.equal(identity.releaseId, 42);
    assert.equal(manifest.releaseId, 42);
    await assert.rejects(freezeReleaseAssets({ GITHUB_ACTIONS: 'true', GITHUB_REPOSITORY: 'kejilion/KPanel',
      GITHUB_SHA: controllerSha, GITHUB_RUN_ID: runId, GITHUB_RUN_ATTEMPT: attempt,
      EXPECTED_SHA: candidate, VERSION: version, IMAGE_DIGEST: digest, GH_TOKEN: 'fixture',
      GITHUB_API_URL: 'https://api.example.test', PUBLIC_EVIDENCE: evidence, ASSET_DIRECTORY: releaseDirectory }));
  } finally {
    globalThis.fetch = originalFetch;
    rmSync(root, { recursive: true, force: true });
  }
});

test('draft-aware lookup rejects missing, erroneous or mismatched release identity', async () => {
  const originalFetch = globalThis.fetch;
  const input = { api: 'https://api.example.test', repository: 'kejilion/KPanel', version: '1.27.1', headers: {} };
  try {
    for (const response of [{ data: { repository: { release: null } } },
      { errors: [{ message: 'denied' }], data: { repository: { release: { databaseId: 42 } } } }]) {
      globalThis.fetch = async () => Response.json(response);
      await assert.rejects(findReleaseByTag(input), /draft-aware release lookup failed/);
    }
    globalThis.fetch = async url => String(url).endsWith('/graphql')
      ? Response.json({ data: { repository: { release: { databaseId: 42 } } } })
      : Response.json({ id: 43, tag_name: 'v1.27.1', draft: true });
    await assert.rejects(findReleaseByTag(input), /identity mismatch/);
  } finally { globalThis.fetch = originalFetch; }
});

for (const [version, previewVersion, accepted] of [
  ['1.27.0', '1.27.0-rc.23', true], ['1.27.1', '1.27.0-rc.23', true], ['1.27.1', '1.28.0-rc.1', false],
]) {
test(`stable recovery validates exact artifact identity for ${version} after ${previewVersion}`, () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-stable-recovery-'));
  const snapshotDirectory = join(root, 'snapshot');
  const publicationDirectory = join(root, 'publication');
  const nested = join(publicationDirectory, 'KPanel', 'KPanel', 'release');
  const evidence = join(publicationDirectory, '_temp', 'kpanel-public-123-2');
  const candidate = 'a'.repeat(40), imageDigest = 'sha256:' + 'b'.repeat(64);
  const expected = { runId: '123', attempt: '2', candidate, version, digest: imageDigest };
  const controllerSha = 'e'.repeat(40);
  const snapshotBytes = Buffer.from(JSON.stringify({ tag: 'v1.26.0', digest: 'sha256:' + 'c'.repeat(64),
    previousPreview: { version: previewVersion, digest: 'sha256:' + 'd'.repeat(64) } }, null, 2) + '\n');
  const notes = Buffer.from(`### 版本更新内容\n- image ${imageDigest}\n`);
  const components = Buffer.from('{"schemaVersion":1}\n');
  const componentDigest = createHash('sha256').update(components).digest('hex');
  const assetManifest = { schemaVersion: 1, phase: 'before-channel-promotion', assetSet: 'windows-command-v1', candidate, version: expected.version, releaseId: 42,
    runId: expected.runId, attempt: expected.attempt, controllerSha, imageDigest,
    snapshotSha256: createHash('sha256').update(snapshotBytes).digest('hex'),
    assets: expectedReleaseAssetNames(expected.version).map((name, index) => ({ name,
      size: name === 'KPANEL_MODIFIED_COMPONENTS.json' ? components.length : 100 + index,
      digest: name === 'KPANEL_MODIFIED_COMPONENTS.json' ? 'sha256:' + componentDigest : 'sha256:' + String(index + 1).padStart(64, '0'),
    })) };
  const manifestBytes = Buffer.from(JSON.stringify(assetManifest, null, 2) + '\n');
  const identityBytes = Buffer.from(JSON.stringify({ schemaVersion: 1, candidate, version: expected.version,
    controllerSha, runId: expected.runId, attempt: expected.attempt, releaseId: 42,
    snapshotSha256: createHash('sha256').update(snapshotBytes).digest('hex'),
    assetManifestSha256: createHash('sha256').update(manifestBytes).digest('hex'),
    notesSha256: createHash('sha256').update(notes).digest('hex'), phase: 'before-publication' }, null, 2) + '\n');
  const write = (path, bytes) => { mkdirSync(join(path, '..'), { recursive: true }); writeFileSync(path, bytes); };
  try {
    write(join(snapshotDirectory, 'stable-before.json'), snapshotBytes);
    write(join(snapshotDirectory, 'publication-identity.json'), identityBytes);
    write(join(snapshotDirectory, 'release-assets.json'), manifestBytes);
    write(join(evidence, 'stable-before.json'), snapshotBytes);
    write(join(evidence, 'publication-identity.json'), identityBytes);
    write(join(nested, 'release-assets.json'), manifestBytes);
    write(join(nested, 'RELEASE_NOTES.md'), notes);
    write(join(nested, 'KPANEL_MODIFIED_COMPONENTS.json'), components);
    if (!accepted) {
      assert.throws(() => validatePublicationArtifacts({ snapshotDirectory, publicationDirectory, expected }), /canonical RC older/);
      return;
    }
    const valid = validatePublicationArtifacts({ snapshotDirectory, publicationDirectory, expected });
    assert.equal(valid.snapshot.tag, 'v1.26.0');
    assert.equal(valid.identity.controllerSha, controllerSha);
    assert.equal(valid.assetManifest.assets.length, 19);
    assert.equal(valid.notesSha256, createHash('sha256').update(notes).digest('hex'));
    write(join(evidence, 'stable-before.json'), Buffer.from('tampered'));
    assert.throws(() => validatePublicationArtifacts({ snapshotDirectory, publicationDirectory, expected }));
    write(join(evidence, 'stable-before.json'), snapshotBytes);
    write(join(nested, 'RELEASE_NOTES.md'), Buffer.from('rerendered ' + imageDigest));
    assert.throws(() => validatePublicationArtifacts({ snapshotDirectory, publicationDirectory, expected }), /artifact identity mismatch/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
}

test('stable recovery only publishes the frozen draft or restores the exact prior latest digest', () => {
  const snapshot = { tag: 'v1.26.0', digest: 'sha256:' + 'c'.repeat(64),
    previousPreview: { version: '1.27.0-rc.23', digest: 'sha256:' + 'd'.repeat(64) } };
  const expectedDigest = 'sha256:' + 'b'.repeat(64), componentDigest = createHash('sha256').update('{"schemaVersion":1}\n').digest('hex');
  const assets = expectedReleaseAssetNames('1.27.0').map((name, i) => ({ name, size: name === 'KPANEL_MODIFIED_COMPONENTS.json' ? 19 : 100 + i,
    digest: name === 'KPANEL_MODIFIED_COMPONENTS.json' ? 'sha256:' + componentDigest : 'sha256:' + String(i + 1).padStart(64, '0') }));
  const assetManifest = { version: '1.27.0', assets, releaseId: 42, assetSet: 'windows-command-v1' };
  const release = { id: 42, tag_name: 'v1.27.0', draft: true, prerelease: false, expectedDigest,
    assets: assets.map(asset => ({ ...asset, state: 'uploaded' })) };
  const state = { snapshot, release, assetManifest, version: '1.27.0', expectedDigest, latestReleaseTag: snapshot.tag, latestDigest: expectedDigest,
    versionDigest: expectedDigest, previewDigest: snapshot.previousPreview.digest, componentDigest };
  assert.equal(classifyRecoveryState({ ...state, operation: 'publish-draft' }), 'publish');
  assert.equal(classifyRecoveryState({ ...state, operation: 'restore-latest' }), 'restore-latest');
  assert.equal(classifyRecoveryState({ ...state, operation: 'restore-latest', latestDigest: snapshot.digest }), 'already-restored');
  assert.equal(classifyRecoveryState({ ...state, operation: 'publish-draft', release: { ...release, draft: false },
    latestReleaseTag: release.tag_name }), 'already-published');
  assert.throws(() => classifyRecoveryState({ ...state, operation: 'restore-latest', release: { ...release, draft: false } }));
  assert.throws(() => classifyRecoveryState({ ...state, operation: 'publish-draft', previewDigest: expectedDigest }));
  assert.throws(() => classifyRecoveryState({ ...state, operation: 'restore-latest', latestDigest: 'sha256:' + '9'.repeat(64) }));
  assert.throws(() => classifyRecoveryState({ ...state, release: { ...release, assets: release.assets.map((asset, i) => i ? asset : { ...asset, digest: 'sha256:' + 'f'.repeat(64) }) } }));
});

test('stable recovery workflow separates the read-only check, draft writer and Docker-only rollback', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release-stable-recovery.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  const recovery = readFileSync(new URL('../stable-release-recovery.mjs', import.meta.url), 'utf8');
  assert.match(workflow, /group: kpanel-image-publication/);
  assert.match(workflow, /contents: read\s+actions: read/);
  assert.match(workflow, /publish_draft:\s+needs: preflight[\s\S]*?contents: write\s+actions: read/);
  assert.match(workflow, /restore_latest:\s+needs: preflight[\s\S]*?contents: read\s+actions: read/);
  assert.match(recovery, /Promote image to its release channel/);
  assert.match(recovery, /Publish GitHub release/);
  assert.match(workflow, /snapshot-\$\{\{ inputs\['publication-run-id'\] \}\}-\$\{\{ inputs\['publication-attempt'\] \}\}/);
  assert.match(workflow, /publication-\$\{\{ inputs\['publication-run-id'\] \}\}-\$\{\{ inputs\['publication-attempt'\] \}\}/);
  assert.ok(workflow.indexOf('Validate exact main, tag and release identity before artifact download')
    < workflow.indexOf('Download exact original snapshot artifact'));
  const publish = workflow.split('  publish_draft:')[1].split('  restore_latest:')[0];
  const rollback = workflow.split('  restore_latest:')[1].split('  public-acceptance:')[0];
  assert.doesNotMatch(publish, /DOCKERHUB_TOKEN|docker login|imagetools create/);
  assert.match(publish, /RELEASE_ID:.*steps\.final-inspect\.outputs\.release_id/);
  assert.match(publish, /gh api --method PATCH "repos\/\$GITHUB_REPOSITORY\/releases\/\$RELEASE_ID"/);
  assert.doesNotMatch(workflow, /scripts\/(?:check|render)-release-notes\.sh/);
  assert.match(publish, /KPANEL_RELEASE_NOTES_FILE:.*steps\.inspect\.outputs\.notes_path/);
  assert.match(publish, /go test \.\/internal\/selfupdate -run '\^TestRenderedReleaseNotesForPublication\$' -count=1/);
  assert.doesNotMatch(rollback, /contents: write|gh release edit/);
  assert.ok(publish.indexOf('Revalidate immediately before the GitHub write') < publish.indexOf('Publish the existing frozen draft'));
  assert.ok(rollback.indexOf('Revalidate immediately before the Docker write') < rollback.indexOf('Restore only the frozen Docker latest digest'));
  const verifier = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8');
  assert.match(verifier, /publication-run-id:[\s\S]*?required: false/);
  assert.match(verifier, /publication-attempt:[\s\S]*?required: false/);
});

test('public recovery binds only the trusted controller runner recipe and rejects changed product identities', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/release-public-verify.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n');
  const body = workflowStep(workflow, 'Bind controller-owned test runner recipe');
  assert.ok(workflow.indexOf('Verify public release assets') < workflow.indexOf('Bind controller-owned'));
  assert.ok(workflow.indexOf('Bind controller-owned') < workflow.indexOf('Verify public release in real browsers'));
  for (const scenario of ['updated-recipe', 'untrusted-controller', 'dirty-product', 'wrong-head', 'wrong-version', 'missing-controller']) {
    const root = mkdtempSync(join(tmpdir(), 'kpanel-recipe-'));
    const git = (...args) => {
      const r = spawnSync('git', args, { cwd: root, encoding: 'utf8', timeout: 10_000 });
      assert.equal(r.status, 0, r.stderr);
      return r.stdout.trim();
    };
    try {
      git('init', '-q');
      git('config', 'user.name', 'Fixture'); git('config', 'user.email', 'fixture@example.invalid');
      git('config', 'core.autocrlf', 'false');
      mkdirSync(join(root, 'packaging/release-runner'), { recursive: true });
      const path = 'packaging/release-runner/Dockerfile';
      const original = 'FROM example@sha256:' + 'a'.repeat(64) + '\nRUN old-fixed-recipe\n';
      const recipe = original.replace('old-fixed', 'new-fixed');
      const fixturePath = 'packaging/tests/release-browser/real-feature-public.sh';
      const browserPath = 'packaging/tests/release-browser/real-feature-browser-public.cjs';
      const entrypoint = 'packaging/tests/release-browser/run.sh';
      const fixture = '#!/usr/bin/env bash\n# fixed verifier fixture\n';
      mkdirSync(join(root, 'packaging/tests/release-browser'), { recursive: true });
      writeFileSync(join(root, fixturePath), fixture);
      writeFileSync(join(root, browserPath), 'fixed browser companion\n');
      writeFileSync(join(root, entrypoint), 'fixed test entrypoint\n');
      writeFileSync(join(root, path), original); writeFileSync(join(root, 'VERSION'), '1.27.0-rc.5\n');
      git('add', '.'); git('commit', '-qm', 'frozen product');
      const candidate = git('rev-parse', 'HEAD');
      writeFileSync(join(root, path), recipe); git('add', path); git('commit', '-qm', 'test runner only');
      const controller = git('rev-parse', 'HEAD');
      git('update-ref', 'refs/remotes/origin/main', scenario === 'untrusted-controller' ? candidate : controller);
      git('checkout', '--detach', '-q', candidate);
      if (scenario === 'dirty-product') writeFileSync(join(root, 'VERSION'), 'dirty\n');
      const evidence = mkdtempSync(join(tmpdir(), 'kpanel-recipe-evidence-'));
      try {
        const r = spawnSync(resolveBash(), ['--noprofile', '--norc', '-c', body], { cwd: root, encoding: 'utf8', timeout: 15_000,
          env: { ...process.env, GITHUB_SHA: scenario === 'missing-controller' ? '0'.repeat(40) : controller,
            EXPECTED_SHA: scenario === 'wrong-head' ? controller : candidate,
            EXPECTED_VERSION: scenario === 'wrong-version' ? '1.27.0-rc.4' : '1.27.0-rc.5',
            GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', PUBLIC_EVIDENCE: evidence } });
        assert.ifError(r.error);
        assert.equal(r.status === 0, scenario === 'updated-recipe', `${scenario}: ${r.stderr}`);
        assert.equal(git('rev-parse', 'HEAD'), candidate);
        if (scenario === 'updated-recipe') {
          const receipt = JSON.parse(readFileSync(join(evidence, 'test-runner-recipe.json')));
          assert.equal(receipt.candidate, candidate); assert.equal(receipt.controller, controller);
          assert.equal(receipt.original, original); assert.equal(receipt.recipe, recipe);
          assert.equal(receipt.originalSha256, createHash('sha256').update(original).digest('hex'));
          assert.equal(receipt.recipeSha256, createHash('sha256').update(recipe).digest('hex'));
          assert.match(receipt.diff, /-RUN old-fixed-recipe/); assert.match(receipt.diff, /\+RUN new-fixed-recipe/);
          assert.equal(git('diff', '--name-only'), path);
          const fixtureReceipt = JSON.parse(readFileSync(join(evidence, 'test-fixture-source.json')));
          assert.equal(fixtureReceipt.path, fixturePath);
          assert.equal(fixtureReceipt.candidate, candidate); assert.equal(fixtureReceipt.controller, controller);
          assert.equal(fixtureReceipt.original, fixture); assert.equal(fixtureReceipt.recipe, fixture);
          assert.equal(fixtureReceipt.originalSha256, createHash('sha256').update(fixture).digest('hex'));
          assert.equal(fixtureReceipt.recipeSha256, fixtureReceipt.originalSha256);
          assert.equal(fixtureReceipt.diff, ''); assert.equal(fixtureReceipt.productImageUnchanged, true);
          const browserReceipt = JSON.parse(readFileSync(join(evidence, 'test-browser-source.json')));
          assert.equal(browserReceipt.path, browserPath); assert.equal(browserReceipt.candidate, candidate);
          assert.equal(browserReceipt.controller, controller); assert.equal(browserReceipt.productImageUnchanged, true);
          assert.equal(browserReceipt.original, 'fixed browser companion\n'); assert.equal(browserReceipt.recipe, browserReceipt.original);
          assert.equal(browserReceipt.originalSha256, createHash('sha256').update(browserReceipt.original).digest('hex'));
          assert.equal(browserReceipt.recipeSha256, browserReceipt.originalSha256); assert.equal(browserReceipt.diff, '');
          const entryReceipt = JSON.parse(readFileSync(join(evidence, 'test-entrypoint-source.json')));
          assert.equal(entryReceipt.path, entrypoint); assert.equal(entryReceipt.candidate, candidate);
          assert.equal(entryReceipt.controller, controller); assert.equal(entryReceipt.productImageUnchanged, true);
          assert.equal(entryReceipt.original, 'fixed test entrypoint\n'); assert.equal(entryReceipt.recipe, entryReceipt.original);
          assert.equal(entryReceipt.originalSha256, createHash('sha256').update(entryReceipt.original).digest('hex'));
          assert.equal(entryReceipt.recipeSha256, entryReceipt.originalSha256); assert.equal(entryReceipt.diff, '');
          assert.equal(readFileSync(join(root, 'VERSION'), 'utf8'), '1.27.0-rc.5\n');
        } else assert.equal(readFileSync(join(root, path), 'utf8'), original);
      } finally { rmSync(evidence, { recursive: true, force: true }); }
    } finally { rmSync(root, { recursive: true, force: true }); }
  }
});

test('version image guard admits only an authoritative missing manifest and rejects occupied/uncertain tags', () => {
  const moduleUrl = new URL('../verify-github-public-release.mjs', import.meta.url).href;
  for (const [status, body, expected] of [
    [404, { errors: [{ code: 'MANIFEST_UNKNOWN' }] }, 0],
    [200, { manifests: [] }, 1],
    [503, {}, 1],
    [404, { errors: [{ code: 'NAME_UNKNOWN' }] }, 1],
  ]) {
    const script = `globalThis.fetch=async url=>url.includes('auth.docker.io') ? new Response(JSON.stringify({token:'fixture'})) : new Response(${JSON.stringify(JSON.stringify(body))},{status:${status}});process.argv[2]='unused';await import(${JSON.stringify(moduleUrl)});`;
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', script], { encoding: 'utf8', timeout: 5000,
      env: { ...process.env, ...valid, RUNNER_TEMP: tmpdir(), EXPECTED_VERSION: '1.27.0-rc.3', EXPECTED_SHA: 'a'.repeat(40), PUBLIC_EVIDENCE: tmpdir() + '/fixture' } });
    assert.ifError(result.error);
    assert.equal(result.status, expected, `${status}: ${result.stderr}`);
  }
});
