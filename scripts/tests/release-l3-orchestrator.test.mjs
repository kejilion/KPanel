import assert from 'node:assert/strict';
import { existsSync, lstatSync, mkdtempSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { gitResult, removeOwnedDirectory, transportEnvironment } from '../run-release-l3.mjs';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const orchestrator = resolve(repoRoot, 'scripts', 'run-release-l3.mjs');
const remoteScript = resolve(repoRoot, 'scripts', 'run-release-l3-remote.sh');
const canonicalOrigin = 'git@github.com:kejilion/KPanel.git';

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
  // Exercise the canonical SSH identity through a local upload-pack transport.
  // Production gets no allow-local option or alternate source-validation path.
  const shim = join(root, 'ssh transport.mjs');
  const control = join(root, 'transport.json');
  writeFileSync(shim, `import { readFileSync, existsSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
const remote = ${JSON.stringify(remote)};
const controlPath = ${JSON.stringify(control)};
const mode = existsSync(controlPath) ? JSON.parse(readFileSync(controlPath, 'utf8')) : {};
if (mode.fail) { process.stderr.write(mode.fail); process.exit(23); }
if (mode.delay) await new Promise((resolve) => setTimeout(resolve, mode.delay));
if (mode.action && !mode.done) {
  mode.calls = (mode.calls || 0) + 1;
  if (mode.calls === mode.at) {
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', mode.action], { encoding: 'utf8' });
    if (result.status !== 0) process.exit(24);
    mode.done = true;
  }
  writeFileSync(controlPath, JSON.stringify(mode));
}
const result = spawnSync('git', ['upload-pack', remote], { stdio: 'inherit' });
process.exit(result.status ?? 1);
`);
  const sshCommand = `"${process.execPath.replaceAll('\\', '/')}" "${shim.replaceAll('\\', '/')}"`;
  git(repo, 'remote', 'set-url', 'origin', canonicalOrigin);
  git(repo, 'config', 'core.sshCommand', sshCommand);
  git(repo, 'config', 'ssh.variant', 'ssh');
  return { root, repo, candidate, baseline, remote, control, sshCommand };
}

function prepareFixture(fixture, extra = [], environment = process.env) {
  return spawnSync(process.execPath, [orchestrator, '--repo', fixture.repo,
    '--candidate', fixture.candidate, '--base-tag', 'v1.0.0',
    '--runner-image', 'example/release-runner:stable', '--run-id', 'source-test',
    '--artifact-dir', join(fixture.root, 'artifacts'), '--prepare-only', ...extra],
  { cwd: fixture.repo, encoding: 'utf8', shell: false, env: environment });
}

function sourceSnapshot(repo) {
  return { refs: git(repo, 'show-ref'), head: git(repo, 'rev-parse', 'HEAD'),
    status: git(repo, 'status', '--porcelain=v1', '--untracked-files=all'), config: git(repo, 'config', '--local', '--list') };
}

function artifactText(directory) {
  if (!existsSync(directory)) return '';
  return readdirSync(directory, { withFileTypes: true }).map((entry) => entry.isDirectory()
    ? artifactText(join(directory, entry.name)) : readFileSync(join(directory, entry.name)).toString('utf8')).join('\n');
}

function removeFixture(root) {
  const prefix = resolve(tmpdir()) + (process.platform === 'win32' ? '\\' : '/');
  assert.ok(resolve(root).startsWith(prefix));
  rmSync(root, { recursive: true, force: true });
}

test('prepare-only builds and verifies a self-contained exact-SHA L3 kit', async () => {
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

test('prepare-only fails closed on a dirty candidate and preserves the existing tree', async () => {
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

test('prepare-only fails closed when origin main moves after the candidate was frozen', async () => {
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

test('prepare-only refuses to place release evidence inside the candidate repository', async () => {
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

test('remote L3 entrypoint is syntax-valid and keeps verification inside fixed scripts', async () => {
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

test('isolates conflicting or missing tags without changing a linked candidate or its shared refs', async () => {
  const fixture = createFixture();
  try {
    git(fixture.repo, 'tag', '-f', 'v1.0.0', fixture.candidate);
    git(fixture.repo, 'tag', '-d', 'v0.83.0');
    const linked = join(fixture.root, 'linked');
    git(fixture.repo, 'worktree', 'add', '--detach', linked, fixture.candidate);
    const before = sourceSnapshot(linked);
    const result = prepareFixture({ ...fixture, repo: linked });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(sourceSnapshot(linked), before);
    const state = JSON.parse(readFileSync(join(fixture.root, 'artifacts', 'source-prepare.json')));
    assert.deepEqual(state.sourceTagDifferences, ['v0.83.0', 'v1.0.0']);
    assert.equal(state.cleanup, 'removed');
    assert.equal(state.status, 'pass');
    const bundle = join(fixture.root, 'artifacts', 'kpanel-source-test.bundle');
    const heads = git(fixture.repo, 'bundle', 'list-heads', bundle);
    for (const tag of ['v0.83.0', 'v1.0.0']) {
      assert.ok(heads.includes(`${git(fixture.remote, 'rev-parse', `refs/tags/${tag}`)} refs/tags/${tag}`));
    }
  } finally { removeFixture(fixture.root); }
});

test('supports an unpublished candidate, lightweight tags and explicit business tag below coverage baseline', async () => {
  const fixture = createFixture();
  try {
    git(fixture.remote, 'tag', 'v0.1.0', fixture.baseline);
    git(fixture.remote, 'tag', 'v0.84.0', fixture.baseline);
    writeFileSync(join(fixture.repo, 'docs', 'product-quality-review-current.md'),
      `- 基线提交：\`${fixture.baseline}\`\n- 基线版本：\`v0.1.0\`\n`);
    git(fixture.repo, 'add', 'docs');
    git(fixture.repo, 'commit', '-m', 'unpublished candidate');
    fixture.candidate = git(fixture.repo, 'rev-parse', 'HEAD');
    const before = sourceSnapshot(fixture.repo);
    const result = prepareFixture(fixture);
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(sourceSnapshot(fixture.repo), before);
    const manifest = JSON.parse(readFileSync(join(fixture.root, 'artifacts', 'manifest.json')));
    assert.deepEqual(manifest.requiredTags, ['v0.1.0', 'v0.83.0', 'v0.84.0', 'v1.0.0']);
    assert.equal(manifest.candidate, fixture.candidate);
  } finally { removeFixture(fixture.root); }
});

test('fails on missing remote required tags, untracked dirt, wrong HEAD, noncanonical origin or shallow history', async (t) => {
  for (const scenario of ['missing-base', 'missing-business', 'untracked', 'wrong-head', 'origin', 'shallow']) {
    await t.test(scenario, async () => {
      const fixture = createFixture();
      try {
        let extra = [];
        if (scenario === 'missing-base') git(fixture.remote, 'tag', '-d', 'v1.0.0');
        if (scenario === 'missing-business') {
          writeFileSync(join(fixture.repo, 'docs', 'product-quality-review-current.md'),
            `- 基线提交：\`${fixture.baseline}\`\n- 基线版本：\`v0.1.0\`\n`);
          git(fixture.repo, 'add', 'docs'); git(fixture.repo, 'commit', '-m', 'business tag');
          fixture.candidate = git(fixture.repo, 'rev-parse', 'HEAD');
        }
        if (scenario === 'untracked') writeFileSync(join(fixture.repo, 'unknown.txt'), 'preserve');
        if (scenario === 'wrong-head') extra = ['--candidate', fixture.baseline];
        if (scenario === 'origin') git(fixture.repo, 'remote', 'set-url', 'origin', 'https://example.invalid/repo');
        if (scenario === 'shallow') writeFileSync(join(fixture.repo, '.git', 'shallow'), fixture.baseline + '\n');
        const before = sourceSnapshot(fixture.repo);
        const result = prepareFixture(fixture, extra);
        assert.notEqual(result.status, 0);
        assert.doesNotMatch(result.stdout, /release_l3_prepare=pass|release_l3_remote/);
        assert.deepEqual(sourceSnapshot(fixture.repo), before);
        const state = JSON.parse(readFileSync(join(fixture.root, 'artifacts', 'source-prepare.json')));
        assert.equal(state.status, 'failed');
        assert.equal(existsSync(join(fixture.root, 'artifacts', 'manifest.json')), false);
      } finally { removeFixture(fixture.root); }
    });
  }
});

test('retains SSH precedence and keeps transport diagnostics and credentials out of all artifacts', async () => {
  const fixture = createFixture();
  try {
    const derived = await transportEnvironment(fixture.repo, { GIT_SSH: 'lower-priority' });
    assert.equal(derived.GIT_SSH_COMMAND, fixture.sshCommand);
    assert.equal(derived.GIT_SSH_VARIANT, 'ssh');
    const override = await transportEnvironment(fixture.repo, { GIT_SSH_COMMAND: 'override', GIT_SSH_VARIANT: 'simple' });
    assert.equal(override.GIT_SSH_COMMAND, 'override');
    assert.equal(override.GIT_SSH_VARIANT, 'simple');
    const sentinel = 'PRIVATE_TRANSPORT_SENTINEL';
    const keyPath = 'C:/private identities/release.key';
    writeFileSync(fixture.control, JSON.stringify({ fail: `${sentinel} ${keyPath} ProxyCommand=secret` }));
    const before = sourceSnapshot(fixture.repo);
    const result = prepareFixture(fixture);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /git ls-remote failed exit=/);
    assert.deepEqual(sourceSnapshot(fixture.repo), before);
    const output = result.stdout + result.stderr + artifactText(join(fixture.root, 'artifacts'));
    for (const secret of [sentinel, keyPath, fixture.sshCommand]) assert.equal(output.includes(secret), false);
  } finally { removeFixture(fixture.root); }
});

test('environment SSH command can override broken local config without persisting either command', async () => {
  const fixture = createFixture();
  try {
    git(fixture.repo, 'config', 'core.sshCommand', 'missing-SSH-PRIVATE_SENTINEL');
    const before = sourceSnapshot(fixture.repo);
    const result = prepareFixture(fixture, [], { ...process.env, GIT_SSH_COMMAND: fixture.sshCommand });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(sourceSnapshot(fixture.repo), before);
    const output = result.stdout + result.stderr + artifactText(join(fixture.root, 'artifacts'));
    assert.equal(output.includes('PRIVATE_SENTINEL'), false);
    assert.equal(output.includes(fixture.sshCommand), false);
  } finally { removeFixture(fixture.root); }
});

test('rejects source/main/required-tag changes during preparation but ignores unrelated new branch tags', async (t) => {
  for (const scenario of ['source-dirty', 'main', 'tag-object', 'tag-delete', 'tag-add', 'unrelated-tag']) {
    await t.test(scenario, async () => {
      const fixture = createFixture();
      try {
        const remote = JSON.stringify(fixture.remote);
        const gitAction = (args) => `spawnSync('git', ['-C', ${remote}, ...${JSON.stringify(args)}]);`;
        let action = "import { spawnSync } from 'node:child_process';\nimport { writeFileSync } from 'node:fs';\n";
        if (scenario === 'source-dirty') action += `writeFileSync(${JSON.stringify(join(fixture.repo, 'baseline.txt'))}, 'external change');`;
        if (scenario === 'main') action += gitAction(['update-ref', 'refs/heads/main', fixture.baseline]);
        if (scenario === 'tag-object') action += gitAction(['-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'tag', '-fa', 'v1.0.0', fixture.baseline, '-m', 'replacement object']);
        if (scenario === 'tag-delete') action += gitAction(['tag', '-d', 'v0.83.0']);
        if (scenario === 'tag-add') action += gitAction(['tag', 'v0.84.0', fixture.baseline]);
        if (scenario === 'unrelated-tag') action += `const tree = spawnSync('git', ['-C', ${remote}, 'rev-parse', '${fixture.baseline}^{tree}'], { encoding: 'utf8' }).stdout.trim();
const commit = spawnSync('git', ['-C', ${remote}, '-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'commit-tree', tree, '-m', 'unrelated'], { encoding: 'utf8' }).stdout.trim();
spawnSync('git', ['-C', ${remote}, 'tag', 'v9.0.0', commit]);`;
        writeFileSync(fixture.control, JSON.stringify({ at: 3, action }));
        const result = prepareFixture(fixture);
        if (scenario === 'unrelated-tag') {
          assert.equal(result.status, 0, result.stderr);
          const manifest = JSON.parse(readFileSync(join(fixture.root, 'artifacts', 'manifest.json')));
          assert.deepEqual(manifest.requiredTags, ['v0.83.0', 'v1.0.0']);
        } else {
          assert.notEqual(result.status, 0, result.stdout);
          assert.doesNotMatch(result.stdout, /release_l3_prepare=pass/);
          const state = JSON.parse(readFileSync(join(fixture.root, 'artifacts', 'source-prepare.json')));
          assert.equal(state.status, 'failed');
          assert.equal(state.cleanup, 'removed');
          if (scenario === 'source-dirty') assert.equal(readFileSync(join(fixture.repo, 'baseline.txt'), 'utf8'), 'external change');
        }
      } finally { removeFixture(fixture.root); }
    });
  }
});

test('keeps failed evidence immutable and only removes the exact owned temporary directory', async () => {
  const fixture = createFixture();
  try {
    writeFileSync(fixture.control, JSON.stringify({ fail: 'private failure' }));
    assert.notEqual(prepareFixture(fixture).status, 0);
    const before = artifactText(join(fixture.root, 'artifacts'));
    const retry = prepareFixture(fixture);
    assert.match(retry.stderr, /artifact directory already exists/);
    assert.equal(artifactText(join(fixture.root, 'artifacts')), before);
    const owned = join(fixture.root, 'owned');
    mkdirSync(owned);
    const stat = lstatSync(owned);
    const identity = { dev: stat.dev, ino: stat.ino, real: process.platform === 'win32' ? owned.toLowerCase() : owned };
    renameSync(owned, join(fixture.root, 'preserved'));
    mkdirSync(owned);
    writeFileSync(join(owned, 'unknown.txt'), 'keep');
    assert.throws(() => removeOwnedDirectory(owned, identity), /identity changed/);
    assert.equal(readFileSync(join(owned, 'unknown.txt'), 'utf8'), 'keep');
  } finally { removeFixture(fixture.root); }
});

test('bounds a real stalled SSH subprocess and redacts a missing executable', async () => {
  const fixture = createFixture();
  try {
    writeFileSync(fixture.control, JSON.stringify({ delay: 2000 }));
    const start = Date.now();
    const environment = await transportEnvironment(fixture.repo);
    await assert.rejects(() => gitResult(fixture.repo, ['ls-remote', 'origin'], environment, 250), /code=timeout/);
    assert.ok(Date.now() - start < 10_000, 'stalled SSH must remain bounded');
    git(fixture.repo, 'config', 'core.sshCommand', 'missing-PRIVATE_EXECUTABLE_SENTINEL');
    const result = prepareFixture(fixture);
    assert.notEqual(result.status, 0);
    const output = result.stdout + result.stderr + artifactText(join(fixture.root, 'artifacts'));
    assert.equal(output.includes('PRIVATE_EXECUTABLE_SENTINEL'), false);
  } finally { removeFixture(fixture.root); }
});
