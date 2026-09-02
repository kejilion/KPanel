import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const orchestrator = resolve(repoRoot, 'scripts', 'run-release-l3.mjs');
const remoteScript = resolve(repoRoot, 'scripts', 'run-release-l3-remote.sh');

function run(command, args, cwd) {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8', shell: false });
  assert.equal(result.status, 0, `${command} ${args.join(' ')}\n${result.stdout}\n${result.stderr}`);
  return result.stdout.trim();
}

function git(cwd, ...args) {
  return run('git', ['-c', 'core.autocrlf=false', ...args], cwd);
}

function createFixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-l3-test-'));
  const remote = join(root, 'remote.git');
  const repo = join(root, 'repo');
  git(root, 'init', '--bare', remote);
  mkdirSync(repo);
  git(repo, 'init', '--initial-branch=main');
  git(repo, 'config', 'user.name', 'KPanel Test');
  git(repo, 'config', 'user.email', 'kpanel-test@example.invalid');
  writeFileSync(join(repo, 'baseline.txt'), 'baseline\n');
  git(repo, 'add', 'baseline.txt');
  git(repo, 'commit', '-m', 'baseline');
  const baseline = git(repo, 'rev-parse', 'HEAD');
  git(repo, 'tag', '-a', 'v0.83.0', '-m', 'v0.83.0');
  git(repo, 'tag', '-a', 'v1.0.0', '-m', 'v1.0.0');

  mkdirSync(join(repo, 'docs'));
  mkdirSync(join(repo, 'scripts'));
  writeFileSync(
    join(repo, 'docs', 'product-quality-review-current.md'),
    `# Current business context\n\n- 基线提交：\`${baseline}\`\n- 基线版本：\`v1.0.0\`\n`,
  );
  writeFileSync(join(repo, 'scripts', 'check-business-context-freshness.mjs'), 'process.exit(0);\n');
  writeFileSync(join(repo, 'scripts', 'run-release-l3-remote.sh'), readFileSync(remoteScript));
  git(repo, 'add', 'docs', 'scripts');
  git(repo, 'commit', '-m', 'candidate');
  const candidate = git(repo, 'rev-parse', 'HEAD');
  git(repo, 'remote', 'add', 'origin', remote);
  git(repo, 'push', 'origin', 'main', 'refs/tags/v0.83.0', 'refs/tags/v1.0.0');
  git(remote, 'symbolic-ref', 'HEAD', 'refs/heads/main');
  git(repo, 'fetch', 'origin', '--no-tags', 'refs/heads/main:refs/remotes/origin/main');
  return { root, repo, candidate };
}

function removeFixture(root) {
  const prefix = resolve(tmpdir()) + (process.platform === 'win32' ? '\\' : '/');
  assert.ok(resolve(root).startsWith(prefix));
  rmSync(root, { recursive: true, force: true });
}

test('prepare-only builds and verifies a self-contained exact-SHA L3 kit', () => {
  const fixture = createFixture();
  try {
    const artifactDir = join(fixture.root, 'artifacts');
    const result = spawnSync(
      process.execPath,
      [
        orchestrator,
        '--repo', fixture.repo,
        '--candidate', fixture.candidate,
        '--base-tag', 'v1.0.0',
        '--runner-image', 'example/release-runner:stable',
        '--run-id', 'v1.1.0-test1',
        '--artifact-dir', artifactDir,
        '--prepare-only',
      ],
      { cwd: fixture.repo, encoding: 'utf8', shell: false },
    );
    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
    assert.match(result.stdout, /release_l3_prepare=pass/);
    const plan = readFileSync(join(artifactDir, 'plan.env'), 'utf8');
    assert.match(plan, new RegExp(`EXPECTED_COMMIT=${fixture.candidate}`));
    assert.match(plan, new RegExp(`BASE_MAIN_COMMIT=${fixture.candidate}`));
  assert.match(plan, /REQUIRED_TAGS=v0\.83\.0,v1\.0\.0/);
    assert.match(plan, /BUNDLE_SHA256=[0-9a-f]{64}/);
    assert.match(plan, /REMOTE_SCRIPT_SHA256=[0-9a-f]{64}/);
    const manifest = JSON.parse(readFileSync(join(artifactDir, 'manifest.json'), 'utf8'));
    assert.equal(manifest.candidate, fixture.candidate);
    assert.deepEqual(manifest.requiredTags, ['v0.83.0', 'v1.0.0']);
  } finally {
    removeFixture(fixture.root);
  }
});

test('prepare-only fails closed on a dirty candidate and preserves the existing tree', () => {
  const fixture = createFixture();
  try {
    writeFileSync(join(fixture.repo, 'baseline.txt'), 'dirty\n');
    const result = spawnSync(
      process.execPath,
      [
        orchestrator,
        '--repo', fixture.repo,
        '--candidate', fixture.candidate,
        '--base-tag', 'v1.0.0',
        '--runner-image', 'example/release-runner:stable',
        '--run-id', 'v1.1.0-test2',
        '--artifact-dir', join(fixture.root, 'dirty-artifacts'),
        '--prepare-only',
      ],
      { cwd: fixture.repo, encoding: 'utf8', shell: false },
    );
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /candidate worktree must be clean/);
    assert.equal(readFileSync(join(fixture.repo, 'baseline.txt'), 'utf8'), 'dirty\n');
  } finally {
    removeFixture(fixture.root);
  }
});

test('prepare-only fails closed when origin main moves after the candidate was frozen', () => {
  const fixture = createFixture();
  try {
    const publisher = join(fixture.root, 'publisher');
    git(fixture.root, 'clone', join(fixture.root, 'remote.git'), publisher);
    git(publisher, 'config', 'user.name', 'KPanel Publisher');
    git(publisher, 'config', 'user.email', 'kpanel-publisher@example.invalid');
    writeFileSync(join(publisher, 'later.txt'), 'later\n');
    git(publisher, 'add', 'later.txt');
    git(publisher, 'commit', '-m', 'later main change');
    git(publisher, 'push', 'origin', 'main');

    const result = spawnSync(
      process.execPath,
      [
        orchestrator,
        '--repo', fixture.repo,
        '--candidate', fixture.candidate,
        '--base-tag', 'v1.0.0',
        '--runner-image', 'example/release-runner:stable',
        '--run-id', 'v1.1.0-test3',
        '--artifact-dir', join(fixture.root, 'stale-main-artifacts'),
        '--prepare-only',
      ],
      { cwd: fixture.repo, encoding: 'utf8', shell: false },
    );
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /local origin\/main is stale/);
  } finally {
    removeFixture(fixture.root);
  }
});

test('prepare-only refuses to place release evidence inside the candidate repository', () => {
  const fixture = createFixture();
  try {
    const result = spawnSync(
      process.execPath,
      [
        orchestrator,
        '--repo', fixture.repo,
        '--candidate', fixture.candidate,
        '--base-tag', 'v1.0.0',
        '--runner-image', 'example/release-runner:stable',
        '--run-id', 'v1.1.0-test4',
        '--artifact-dir', join(fixture.repo, 'release-artifacts'),
        '--prepare-only',
      ],
      { cwd: fixture.repo, encoding: 'utf8', shell: false },
    );
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /artifact directory must stay outside/);
    assert.equal(git(fixture.repo, 'status', '--short'), '');
  } finally {
    removeFixture(fixture.root);
  }
});

test('remote L3 entrypoint is syntax-valid and keeps verification inside fixed scripts', () => {
  const bash = process.env.KPANEL_TEST_BASH ||
    (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');
  const syntax = spawnSync(bash, ['-n', 'scripts/run-release-l3-remote.sh'], {
    cwd: repoRoot,
    encoding: 'utf8',
  });
  assert.equal(syntax.status, 0, `${syntax.stdout}\n${syntax.stderr}`);
  const content = readFileSync(remoteScript, 'utf8');
  assert.match(content, /git init --bare/);
  assert.match(content, /git -C "\$verify_repo" bundle verify/);
  assert.match(content, /run-release-gate\.sh/);
  assert.match(content, /PIPESTATUS\[0\]/);
  assert.match(content, /use a new run ID for every attempt/);
  assert.match(content, /duplicate release plan key/);
  assert.doesNotMatch(content, /\beval\b/);
  assert.doesNotMatch(content, /(?:^|\n)\s*(?:source|\.)\s+"?\$plan/m);
});
