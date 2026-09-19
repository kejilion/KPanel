import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { archiveReleaseCandidate } from '../archive-release-candidate.mjs';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-archive-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const repo = join(root, 'source');
  const remote = join(root, 'remote.git');
  const env = { ...process.env, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: process.platform === 'win32' ? 'NUL' : '/dev/null' };
  function git(args, cwd = repo) {
    const r = spawnSync('git', args, { cwd, env, encoding: 'utf8' });
    assert.equal(r.status, 0, `${args.join(' ')}: ${r.stderr}`);
    return r.stdout.trim();
  }
  git(['init', '--bare', remote], root);
  git(['init', repo], root);
  git(['config', 'user.name', 'Archive fixture']);
  git(['config', 'user.email', 'archive@example.invalid']);
  git(['config', 'commit.gpgsign', 'false']);
  git(['config', 'tag.gpgsign', 'false']);
  git(['commit', '--allow-empty', '-m', 'base']);
  const base = git(['rev-parse', 'HEAD']);
  git(['commit', '--allow-empty', '-m', 'release']);
  const sha = git(['rev-parse', 'HEAD']);
  git(['tag', '-a', 'v1.2.3', '-m', 'release']);
  git(['remote', 'add', 'origin', remote]);
  git(['push', 'origin', 'refs/tags/v1.2.3', `${base}:refs/heads/release/v1.2.3-candidate`]);
  const run = (options = {}) => archiveReleaseCandidate({ repo, tag: 'v1.2.3', releaseSha: sha, ...options });
  return { root, repo, remote, git, base, sha, run };
}

test('dry run preserves candidate; apply archives exact tip; retry is idempotent', t => {
  const f = fixture(t);
  assert.equal(f.run().status, 'ready');
  assert.match(f.git(['ls-remote', 'origin', 'refs/heads/release/*']), /release\/v1.2.3-candidate/);
  const result = f.run({ apply: true });
  assert.equal(result.status, 'archived');
  assert.equal(result.sha, f.base);
  assert.equal(f.git(['ls-remote', 'origin', 'refs/heads/release/*']), '');
  assert.equal(f.git(['ls-remote', 'origin', result.archive]).split(/\s/)[0], f.base);
  assert.equal(f.run({ apply: true }).status, 'already-archived');
});

test('preview, wrong tag identity, and missing archive cannot report success', t => {
  const f = fixture(t);
  assert.throws(() => f.run({ tag: 'v1.2.3-rc.1', apply: true }), /stable release/);
  assert.throws(() => f.run({ releaseSha: f.base, apply: true }), /exact release SHA/);
  f.git(['push', 'origin', ':refs/heads/release/v1.2.3-candidate']);
  assert.throws(() => f.run({ apply: true }), /without an archive/);
});

test('candidate newer than release and archive collisions preserve branches', t => {
  const f = fixture(t);
  f.git(['push', 'origin', `${f.sha}:refs/heads/archive/release/v1.2.3-candidate`]);
  assert.throws(() => f.run({ apply: true }), /Archive collision/);
  f.git(['push', 'origin', ':refs/heads/archive/release/v1.2.3-candidate']);
  f.git(['commit', '--allow-empty', '-m', 'next work']);
  const next = f.git(['rev-parse', 'HEAD']);
  f.git(['push', 'origin', `${next}:refs/heads/release/v1.2.3-candidate`]);
  assert.throws(() => f.run({ apply: true }), /not contained/);
  assert.match(f.git(['ls-remote', 'origin', 'refs/heads/release/*']), new RegExp(next));
});

test('unreachable remote is an error, not an absent candidate', t => {
  const f = fixture(t);
  f.git(['remote', 'set-url', 'origin', join(f.root, 'missing.git')]);
  assert.throws(() => f.run({ apply: true }), /ls-remote failed/);
});

test('atomic rejection does not leave a deleted candidate or partial archive', t => {
  const f = fixture(t);
  writeFileSync(join(f.remote, 'hooks', 'pre-receive'), '#!/bin/sh\nexit 1\n', { mode: 0o755 });
  assert.throws(() => f.run({ apply: true }), /push failed/);
  assert.match(f.git(['ls-remote', 'origin', 'refs/heads/release/*']), new RegExp(f.base));
  assert.equal(f.git(['ls-remote', 'origin', 'refs/heads/archive/*']), '');
});

test('a concurrent candidate advance is protected by the exact deletion lease', t => {
  const f = fixture(t);
  f.git(['config', 'core.hooksPath', join(f.repo, '.git', 'hooks')]);
  // The push handshake has already advertised refs; change the server tip before
  // the update is submitted. Git must reject the stale expected old SHA.
  const remotePath = f.remote.replaceAll('\\', '/').replaceAll("'", "'\\''");
  writeFileSync(join(f.repo, '.git', 'hooks', 'pre-push'), `#!/bin/sh\ngit --git-dir='${remotePath}' update-ref refs/heads/release/v1.2.3-candidate ${f.sha} ${f.base}\n`, { mode: 0o755 });
  assert.throws(() => f.run({ apply: true }), /push failed/);
  assert.match(f.git(['ls-remote', 'origin', 'refs/heads/release/*']), new RegExp(f.sha));
  assert.equal(f.git(['ls-remote', 'origin', 'refs/heads/archive/*']), '');
});

test('release checkout with shallow history can verify ancestor candidate', t => {
  const f = fixture(t);
  const shallow = join(f.root, 'shallow');
  const url = `file://${f.remote.replaceAll('\\', '/').replace(/^([A-Z]):/, '/$1:')}`;
  f.git(['clone', '--depth=1', '--branch', 'v1.2.3', url, shallow], f.root);
  assert.equal(f.git(['rev-parse', '--is-shallow-repository'], shallow), 'true');
  assert.equal(f.run({ repo: shallow, apply: true }).status, 'archived');
});
