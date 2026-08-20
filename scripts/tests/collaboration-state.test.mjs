import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const script = resolve(fileURLToPath(new URL('../check-collaboration-state.mjs', import.meta.url)));

function git(cwd, ...args) {
  return execFileSync('git', ['-C', cwd, ...args], { encoding: 'utf8' }).trim();
}

function run(repo, ...args) {
  return spawnSync(process.execPath, [script, '--repo', repo, ...args], {
    encoding: 'utf8',
  });
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-collaboration-state-'));
  const remote = join(root, 'remote.git');
  const management = join(root, 'management');
  const writer = join(root, 'writer');
  mkdirSync(remote);
  execFileSync('git', ['init', '--bare', '--initial-branch=main', remote]);
  execFileSync('git', ['clone', remote, management]);
  git(management, 'config', 'user.name', 'KPanel Test');
  git(management, 'config', 'user.email', 'kpanel@example.invalid');
  writeFileSync(join(management, 'README.md'), 'baseline\n');
  git(management, 'add', 'README.md');
  git(management, 'commit', '-m', 'test: baseline');
  git(management, 'push', '-u', 'origin', 'main');
  const baseline = git(management, 'rev-parse', 'HEAD');
  git(management, 'worktree', 'add', '-b', 'feature/test', writer, baseline);
  return {
    root,
    remote,
    management,
    writer,
    baseline,
    cleanup: () => rmSync(root, { recursive: true, force: true }),
  };
}

test('clean primary main passes management policy', () => {
  const state = fixture();
  try {
    const result = run(state.management, '--role', 'management', '--base-ref', 'origin/main');
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /collaboration_state=pass role=management branch=main clean=true ahead=0 behind=0/);
  } finally {
    state.cleanup();
  }
});

test('dirty or locally advanced management main is quarantined', () => {
  const state = fixture();
  try {
    writeFileSync(join(state.management, 'README.md'), 'dirty\n');
    let result = run(state.management, '--role', 'management', '--base-ref', 'origin/main');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /management worktree must be clean/);

    writeFileSync(join(state.management, 'README.md'), 'local commit\n');
    git(state.management, 'add', 'README.md');
    git(state.management, 'commit', '-m', 'test: local main commit');
    result = run(state.management, '--role', 'management', '--base-ref', 'origin/main');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /local commit\(s\) absent from origin\/main/);

    git(state.management, 'checkout', '-b', 'feature/management-misuse');
    result = run(state.management, '--role', 'management', '--base-ref', 'origin/main');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /management worktree must stay on main/);
  } finally {
    state.cleanup();
  }
});

test('management main may be behind because task worktrees use the exact remote baseline', () => {
  const state = fixture();
  try {
    const publisher = join(state.root, 'publisher');
    execFileSync('git', ['clone', state.remote, publisher]);
    git(publisher, 'config', 'user.name', 'KPanel Test');
    git(publisher, 'config', 'user.email', 'kpanel@example.invalid');
    writeFileSync(join(publisher, 'README.md'), 'remote update\n');
    git(publisher, 'add', 'README.md');
    git(publisher, 'commit', '-m', 'test: remote update');
    git(publisher, 'push', 'origin', 'main');
    git(state.management, 'fetch', 'origin');

    let result = run(state.management, '--role', 'management', '--base-ref', 'origin/main');
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /ahead=0 behind=1/);

    result = run(state.writer, '--role', 'writer', '--base-ref', 'origin/main');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /origin\/main is not an ancestor of the writer HEAD/);
  } finally {
    state.cleanup();
  }
});

test('writer must use a linked non-main worktree and may require a clean checkpoint', () => {
  const state = fixture();
  try {
    let result = run(state.management, '--role', 'writer', '--base-ref', state.baseline);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /linked task worktree/);

    result = run(state.writer, '--role', 'writer', '--base-ref', state.baseline, '--require-clean');
    assert.equal(result.status, 0, result.stderr);
    writeFileSync(join(state.writer, 'draft.txt'), 'draft\n');

    result = run(state.writer, '--role', 'writer', '--base-ref', state.baseline);
    assert.equal(result.status, 0, result.stderr);
    result = run(state.writer, '--role', 'writer', '--base-ref', state.baseline, '--require-clean');
    assert.equal(result.status, 1);
    assert.match(result.stderr, /writer checkpoint must be clean/);

    rmSync(join(state.writer, 'draft.txt'));
    git(state.writer, 'checkout', '--detach');
    result = run(state.writer, '--role', 'writer', '--base-ref', state.baseline);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /attached task branch/);
  } finally {
    state.cleanup();
  }
});

test('caller Git repository overrides cannot redirect the evidence source', () => {
  const state = fixture();
  try {
    const result = spawnSync(process.execPath, [
      script,
      '--repo', state.management,
      '--role', 'management',
      '--base-ref', 'origin/main',
    ], {
      encoding: 'utf8',
      env: {
        ...process.env,
        GIT_DIR: join(state.root, 'missing.git'),
        GIT_WORK_TREE: state.writer,
        GIT_INDEX_FILE: join(state.root, 'foreign-index'),
        GIT_CONFIG_COUNT: '1',
        GIT_CONFIG_KEY_0: 'core.worktree',
        GIT_CONFIG_VALUE_0: state.writer,
      },
    });
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, new RegExp('head=' + state.baseline));
  } finally {
    state.cleanup();
  }
});

test('invalid or duplicate arguments fail closed', () => {
  let result = spawnSync(process.execPath, [script, '--role', 'reviewer'], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /--role must be management or writer/);

  result = spawnSync(process.execPath, [script, '--role', 'writer', '--role', 'writer'], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /duplicate option/);
});
