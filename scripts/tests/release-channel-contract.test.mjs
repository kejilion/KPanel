import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { basename, join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { runInNewContext } from 'node:vm';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const bash = process.env.KPANEL_TEST_BASH ||
  (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');

function render(version, windowsEnabled = false) {
  const temporary = mkdtempSync(join(repoRoot, '.release-channel-test-'));
  const relativeDirectory = basename(temporary);
  const changelog = join(temporary, 'CHANGELOG.md');
  const output = join(temporary, 'NOTES.md');
  writeFileSync(changelog, `## [${version}]\n\n### Added\n\n- release channel fixture\n`);
  const result = spawnSync(bash, [
    'scripts/render-release-notes.sh',
    version,
    'docker.io/kjlion/kejilion-panel',
    `sha256:${'a'.repeat(64)}`,
    `${relativeDirectory}/NOTES.md`,
  ], {
    cwd: repoRoot,
    encoding: 'utf8',
    env: { ...process.env, CHANGELOG_FILE: `${relativeDirectory}/CHANGELOG.md`, KPANEL_WINDOWS_NODE_RELEASE: String(windowsEnabled) },
  });
  const notes = result.status === 0 ? readFileSync(output, 'utf8') : '';
  rmSync(temporary, { recursive: true, force: true });
  return { ...result, notes };
}

test('release notes distinguish stable and preview image contracts', () => {
  const stable = render('1.2.3');
  assert.equal(stable.status, 0, `${stable.stdout}\n${stable.stderr}`);
  assert.match(stable.notes, /- 生产镜像：`docker\.io\/kjlion\/kejilion-panel@sha256:[0-9a-f]{64}`/);
  assert.doesNotMatch(stable.notes, /这是 RC 预览版/);

  const preview = render('1.3.0-rc.2');
  assert.equal(preview.status, 0, `${preview.stdout}\n${preview.stderr}`);
  assert.match(preview.notes, /这是 RC 预览版/);
  assert.match(preview.notes, /- 预览镜像：`docker\.io\/kjlion\/kejilion-panel@sha256:[0-9a-f]{64}`/);
  assert.doesNotMatch(preview.notes, /- 生产镜像：/);
  assert.match(preview.notes, /退出预览版计划只切回稳定版来源，不会自动降级/);
});

test('release notes reject unsupported prerelease names', () => {
  const invalid = render('1.3.0-beta.1');
  assert.notEqual(invalid.status, 0);
  assert.match(invalid.stderr, /X\.Y\.Z or X\.Y\.Z-rc\.N/);
});

test('only releases with verified Windows assets advertise the unsigned installer and integrity boundary', () => {
  assert.doesNotMatch(render('1.2.3').notes, /install-windows\.ps1/);
  for (const version of ['1.2.3', '1.3.0-rc.2']) {
    const result = render(version, true);
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.notes, /install-windows\.ps1/);
    assert.match(result.notes, /未签名/);
    assert.match(result.notes, /SHA256SUMS/);
    assert.match(result.notes, /不提供证书发布者身份保证/);
  }
});

test('release workflow publishes isolated stable and preview channels', () => {
  const workflow = readFileSync(join(repoRoot, '.github', 'workflows', 'release.yml'), 'utf8');
  assert.match(workflow, /preview_pattern=.*-rc/);
  assert.match(workflow, /channel=preview/);
  assert.match(workflow, /channel_tag=preview/);
  assert.match(workflow, /channel_tag=latest/);
  assert.match(workflow, /--prerelease/);
  assert.match(workflow, /--latest=false/);
  assert.match(workflow, /if: steps\.release\.outputs\.stable == 'true'/);
  assert.match(workflow, /node scripts\/archive-release-candidate\.mjs/);
  assert.match(workflow, /--tag "\$GITHUB_REF_NAME" --release-sha "\$GITHUB_SHA" --apply/);
  assert.doesNotMatch(workflow, /gh api --method DELETE/);
});

test('release requires successful Windows build and checksum merge before public writes', () => {
  const workflow = readFileSync(join(repoRoot, '.github', 'workflows', 'release.yml'), 'utf8').replaceAll('\r\n', '\n');
  const windows = workflow.match(/^  windows-node:\n([\s\S]+?)^  release:/m)?.[1];
  assert.ok(windows, 'mandatory Windows job is present');
  assert.doesNotMatch(windows, /^    if:/m, 'Windows assets cannot be disabled by an optional flag');
  assert.doesNotMatch(windows, /artifact-signing|windows-node-signing|KPANEL_AZURE|KPANEL_SIGNING|Authenticode/);
  const release = workflow.slice(workflow.indexOf('  release:\n'));
  assert.match(release, /^    needs: windows-node$/m);
  const guard = release.match(/^    if: \$\{\{ (.+) \}\}$/m)?.[1];
  assert.ok(guard, 'downstream release has an explicit Windows result guard');
  const expression = guard.replaceAll('needs.windows-node.result', 'windowsResult');
  for (const windowsResult of ['success', 'failure', 'cancelled', 'skipped']) {
    for (const wasCancelled of [false, true]) {
      assert.equal(runInNewContext(expression, { windowsResult, cancelled: () => wasCancelled }),
        windowsResult === 'success' && !wasCancelled, `${windowsResult}, cancelled=${wasCancelled}`);
    }
  }
  let previous = -1;
  for (const marker of ['-Mode Check', 'name: Build Windows node and installer',
    '-Mode Verify', 'name: Transfer verified Windows release assets']) {
    const position = windows.indexOf(marker);
    assert.ok(position > previous, `${marker} must follow verified prerequisites`);
    previous = position;
  }
  const merge = release.indexOf('node scripts/merge-windows-release.mjs');
  assert.ok(merge >= 0, 'four verified assets are merged into release checksums');
  for (const marker of ['name: Prepare draft GitHub release', 'name: Build and push multi-architecture image',
    'name: Promote image to its release channel', 'name: Publish GitHub release']) {
    assert.ok(release.indexOf(marker) > merge, `${marker} must follow Windows checksum merge`);
  }
});

test('release metadata archive is named and described consistently', () => {
  const workflow = readFileSync(join(repoRoot, '.github', 'workflows', 'release.yml'), 'utf8');
  const archive = 'kejilion-panel-meta-$VERSION.tar.gz';
  assert.equal(workflow.split(archive).length - 1, 4, 'build, checksum, and both upload paths must use the same name');
  assert.doesNotMatch(workflow, /kejilion-panel-deploy-\$VERSION\.tar\.gz/);
  assert.match(workflow, /git archive --format=tar HEAD \\\s+deploy docs README\.md CHANGELOG\.md VERSION \\\s+LICENSE NOTICE LICENSES THIRD_PARTY_NOTICES\.md TRADEMARKS\.md/);
  const releaseProcedure = readFileSync(join(repoRoot, '.codex-workflows', 'release-kpanel.workflow.yaml'), 'utf8');
  assert.match(releaseProcedure, /kejilion-panel-meta-\$\{\{version\}\}\.tar\.gz/);
  assert.doesNotMatch(releaseProcedure, /kejilion-panel-deploy-\$\{\{version\}\}\.tar\.gz/);

  for (const version of ['1.2.3', '1.3.0-rc.2']) {
    const result = render(version);
    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
    assert.match(result.notes, /面板本体仅支持 Docker 部署/);
    assert.match(result.notes, new RegExp(`kejilion-panel-meta-${version.replaceAll('.', '\\.')}\\.tar\\.gz`));
    assert.match(result.notes, /不是可构建源码包/);
  }
});
