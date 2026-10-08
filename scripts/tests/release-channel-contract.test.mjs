import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { basename, join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const bash = process.env.KPANEL_TEST_BASH ||
  (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');

function render(version, section = '### Added\n\n- release channel fixture\n', check = false) {
  const temporary = mkdtempSync(join(repoRoot, '.release-channel-test-'));
  const relativeDirectory = basename(temporary);
  const changelog = join(temporary, 'CHANGELOG.md');
  const output = join(temporary, 'NOTES.md');
  writeFileSync(changelog, `## [${version}]\n\n${section}`);
  const result = spawnSync(bash, [
    check ? 'scripts/check-release-notes.sh' : 'scripts/render-release-notes.sh',
    version,
    'docker.io/kjlion/kejilion-panel',
    `sha256:${'a'.repeat(64)}`,
    `${relativeDirectory}/NOTES.md`,
  ], {
    cwd: repoRoot,
    encoding: 'utf8',
    env: { ...process.env, CHANGELOG_FILE: `${relativeDirectory}/CHANGELOG.md` },
  });
  const notes = result.status === 0 ? readFileSync(output, 'utf8') : '';
  rmSync(temporary, { recursive: true, force: true });
  return { ...result, notes };
}

test('release renderer normalizes supported and historical headings for the panel', () => {
  const section = [
    ['Added', '新增'], ['新增与改进', '变更'], ['新增与修复', '变更'],
    ['修复与整理', '修复'], ['修复与改进', '修复'], ['Documentation', '变更'],
    ['Deprecated', '变更'], ['Removed', '变更'], ['Compatibility', '兼容性'],
    ['使用与升级注意', '升级注意事项'],
  ];
  for (const [heading, normalized] of section) {
    const result = render('1.3.0-rc.2', `### ${heading}\n\n- visible item\n`);
    assert.equal(result.status, 0, result.stderr);
    assert.ok(result.notes.includes(`#### ${normalized}\n`), result.notes);
  }
  const indented = render('1.3.0-rc.2', ' ### Added ###\n- visible item\n');
  assert.equal(indented.status, 0, indented.stderr);
  assert.match(indented.notes, /#### 新增\n- visible item/);
});

test('stable and preview publication gates accept readable updates and upgrade warnings', () => {
  for (const version of ['1.2.3', '1.3.0-rc.2']) {
    const result = render(version, '### Added\n- visible update\n### Upgrade Notes\n- keep existing data\n', true);
    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
    assert.match(result.notes, /#### 新增\n- visible update/);
    assert.match(result.notes, /#### 升级注意事项\n- keep existing data/);
  }
});

test('both publication channels reject unsupported, uncategorized and metadata-only updates', () => {
  for (const version of ['1.2.3', '1.3.0-rc.2']) {
    for (const section of [
      '### 未知分类\n- visible update\n',
      '- uncategorized update\n',
      '### 发布边界\n- preview channel only\n',
      '### Upgrade Notes\n- warning without an update\n',
      '### Added\n- retained update\n ### 未知分类\n- silently lost update\n',
      '### Added\n- retained update\n## 未知分类\n- silently lost update\n',
      `### Added\n${'- supported update\n'.repeat(9)}#### 未知子分类\n- lost update\n`,
    ]) {
      const result = render(version, section, true);
      assert.notEqual(result.status, 0, `unsafe ${version} release was accepted: ${section}`);
      assert.match(`${result.stdout}\n${result.stderr}`, /unsupported|supported category|readable user-visible item/);
    }
  }
});

test('the runtime artifact gate rejects unexpected outer headings in either channel', () => {
  for (const version of ['1.2.3', '1.3.0-rc.2']) {
    for (const heading of [' ### 未知分类', '## 未知分类']) {
      const temporary = mkdtempSync(join(repoRoot, '.release-channel-test-'));
      const output = join(temporary, 'NOTES.md');
      writeFileSync(output, `## KPanel ${version}\n### 版本更新内容\n#### 新增\n- retained update\n${heading}\n- silently lost update\n### 发布产物与完整性\n`);
      const result = spawnSync('go', ['test', './internal/selfupdate', '-run', '^TestRenderedReleaseNotesForPublication$', '-count=1'], {
        cwd: repoRoot,
        encoding: 'utf8',
        env: { ...process.env, KPANEL_RELEASE_NOTES_FILE: output, KPANEL_RELEASE_NOTES_VERSION: version },
      });
      rmSync(temporary, { recursive: true, force: true });
      assert.ifError(result.error);
      assert.notEqual(result.status, 0, `${heading} was silently accepted for ${version}`);
      assert.match(`${result.stdout}\n${result.stderr}`, /unsupported rendered release/);
    }
  }
});

test('both release publication stages use the same runtime notes gate without a channel bypass', () => {
  const workflow = readFileSync(join(repoRoot, '.github', 'workflows', 'release.yml'), 'utf8');
  const gate = 'bash scripts/check-release-notes.sh';
  assert.equal(workflow.split(gate).length - 1, 2);
  assert.doesNotMatch(workflow, /bash scripts\/render-release-notes\.sh/);
  const validation = workflow.slice(workflow.indexOf('- name: Validate release notes'), workflow.indexOf('- name: Verify source'));
  assert.ok(validation.includes(gate));
  assert.doesNotMatch(validation, /\bif:/);
  assert.ok(workflow.indexOf('- name: Set up Go') < workflow.indexOf('- name: Validate release notes'));
  assert.ok(workflow.indexOf('- name: Set up Node') < workflow.indexOf('- name: Validate release notes'));
  const finalValidation = workflow.slice(workflow.indexOf('- name: Validate final release notes'), workflow.indexOf('- name: Promote image'));
  assert.ok(finalValidation.includes(gate));
  assert.doesNotMatch(finalValidation, /\bif:/);
  assert.ok(workflow.lastIndexOf(gate) < workflow.indexOf('- name: Promote image'));
  assert.ok(workflow.lastIndexOf(gate) < workflow.indexOf('- name: Publish GitHub release'));
});

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

test('release notes describe only Linux node artifacts', () => {
  for (const version of ['1.2.3', '1.3.0-rc.2']) {
    const result = render(version);
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.notes, /Linux Agent、Linux 轻量节点/);
    assert.doesNotMatch(result.notes, /Windows|install-windows\.ps1|bootstrap-windows\.ps1/);
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

test('release has no Windows node build or release-artifact dependency', () => {
  const workflow = readFileSync(join(repoRoot, '.github', 'workflows', 'release.yml'), 'utf8').replaceAll('\r\n', '\n');
  assert.doesNotMatch(workflow, /^  windows-node:/m);
  assert.doesNotMatch(workflow, /needs\.windows-node|windows-node-release|merge-windows-release|kejilion-node-windows|bootstrap-windows\.ps1|install-windows\.ps1/);
  const release = workflow.slice(workflow.indexOf('  release:\n'));
  for (const marker of ['name: Prepare draft GitHub release', 'name: Build and push multi-architecture image',
    'name: Promote image to its release channel', 'name: Publish GitHub release']) {
    assert.ok(release.includes(marker), `${marker} remains part of the release lane`);
  }
  const ci = readFileSync(join(repoRoot, '.github', 'workflows', 'ci.yml'), 'utf8').replaceAll('\r\n', '\n');
  assert.doesNotMatch(ci, /^  windows-node:/m);
  assert.doesNotMatch(ci, /internal\/windowsnode|cmd\/kejilion-node|windows-node-release/);
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
