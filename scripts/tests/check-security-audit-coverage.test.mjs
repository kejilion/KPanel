import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  assessCoverage,
  loadRuns,
  main,
  normalizeRun,
  parseArguments,
  validatePolicy,
  validateRun,
} from '../check-security-audit-coverage.mjs';

const POLICY = {
  schemaVersion: 1,
  boundaryRoots: ['internal', 'cmd'],
  packageRoots: ['internal', 'cmd'],
  nonBoundary: { 'internal/version': 'constant version string only' },
  ignoreSuffixes: ['_test.go'],
  ignoreSegments: ['testdata'],
  scopedMaxAgeDays: 14,
  fullMaxAgeDays: 30,
};
const DAY = 86400;
const START = Date.parse('2026-09-01T00:00:00Z') / 1000;

function fixture(t) {
  const repo = mkdtempSync(join(tmpdir(), 'kpanel-audit-coverage-'));
  t.after(() => rmSync(repo, { recursive: true, force: true }));
  const git = (...args) => execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8' }).trim();
  git('init', '-q');
  git('config', 'user.name', 'Audit test');
  git('config', 'user.email', 'audit@example.invalid');
  git('config', 'commit.gpgsign', 'false');
  const write = (path, text) => {
    mkdirSync(dirname(join(repo, path)), { recursive: true });
    writeFileSync(join(repo, path), text);
  };
  const commit = (day, files, message = 'change') => {
    for (const [path, text] of Object.entries(files)) write(path, text);
    const date = new Date((START + day * DAY) * 1000).toISOString();
    execFileSync('git', ['-C', repo, 'add', '-A'], { encoding: 'utf8' });
    execFileSync('git', ['-C', repo, 'commit', '-qm', message], {
      encoding: 'utf8',
      env: { ...process.env, GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date },
    });
    return git('rev-parse', 'HEAD');
  };
  const run = (name, meta) => write('.governance/security-audit/' + name + '/run-metadata.json', JSON.stringify(meta));
  write('.governance/security-audit/boundary-policy.json', JSON.stringify(POLICY));
  const base = commit(0, { 'internal/agent/server.go': 'v1', 'internal/version/version.go': 'v1', 'cmd/paneld/main.go': 'v1' }, 'base');
  return { repo, git, commit, run, base };
}

function full(source) {
  return { run_id: 'x', scope_mode: 'full', run_status: 'complete', source_ref: source, source_dirty: false };
}

function scoped(base, source, status = 'complete', complete = true) {
  return { ...full(source), scope_mode: 'scoped', run_status: status, comparison_base: base, scope_complete: complete };
}

function assess(repo) {
  return assessCoverage({ repo, policy: POLICY, runs: loadRuns(repo) });
}

test('no completed full run always requires a full run', (t) => {
  const { repo } = fixture(t);
  mkdirSync(join(repo, '.governance/security-audit'), { recursive: true });
  const report = assess(repo);
  assert.equal(report.decision, 'full-required');
});

test('recent boundary changes are pending, while tests and non-boundary packages never count', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  commit(2, { 'internal/agent/server_test.go': 'test', 'internal/version/version.go': 'v2', 'internal/agent/testdata/a': 'x' });
  let report = assess(repo);
  assert.equal(report.decision, 'ok');
  assert.equal(report.commits.length, 0);
  commit(3, { 'internal/agent/server.go': 'v2' }, 'agent change');
  report = assess(repo);
  assert.equal(report.decision, 'ok');
  assert.deepEqual(report.files, ['internal/agent/server.go']);
  commit(20, { 'README.md': 'later' });
  report = assess(repo);
  assert.equal(report.decision, 'scoped-required');
  assert.match(report.reasons[0], /17 days old/);
});

test('a new package is a new boundary and triggers a scoped run immediately', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  commit(1, { 'internal/mcpaccess/access.go': 'new' });
  const report = assess(repo);
  assert.equal(report.decision, 'scoped-required');
  assert.deepEqual(report.newPackages, ['internal/mcpaccess']);
});

test('only completed, contiguous scoped runs advance coverage', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  const first = commit(1, { 'internal/mcpaccess/access.go': 'new' });
  run('run-5', scoped(base, first, 'incomplete-platform-interruption'));
  let report = assess(repo);
  assert.equal(report.decision, 'scoped-required');
  assert.deepEqual(report.interrupted.map((r) => r.name), ['run-5']);
  const second = commit(2, { 'internal/agent/server.go': 'v2' });
  run('run-6', scoped(first, second));
  run('run-7', scoped(base, second, 'complete', false));
  report = assess(repo);
  assert.deepEqual(report.gaps.map((r) => r.name), ['run-6', 'run-7']);
  assert.equal(report.through.name, 'run-4');
  run('run-8', scoped(base, second));
  report = assess(repo);
  assert.equal(report.through.name, 'run-8');
  assert.equal(report.decision, 'ok');
});

test('an old full run requires a full run even without pending changes', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  commit(31, { 'README.md': 'x' });
  const report = assess(repo);
  assert.equal(report.decision, 'full-required');
  assert.match(report.reasons[0], /31 days old/);
});

test('runs outside the target history are not coverage', (t) => {
  const { repo, git, commit, run, base } = fixture(t);
  git('checkout', '-qb', 'side');
  const side = commit(1, { 'internal/agent/server.go': 'side' });
  git('checkout', '-q', '-');
  run('run-4', full(side));
  let report = assess(repo);
  assert.equal(report.decision, 'full-required');
  assert.deepEqual(report.outside.map((r) => r.name), ['run-4']);
  run('run-5', full(base));
  report = assess(repo);
  assert.equal(report.lastFull.name, 'run-5');
  assert.deepEqual(report.outside.map((r) => r.name), ['run-4']);
});

test('nested packages count as new boundary packages; nested non-boundary paths do not', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  commit(1, { 'internal/agent/sshlogin/login.go': 'new', 'internal/version/sub/x.go': 'v', 'internal/agent/assets/a.txt': 'x' });
  const report = assess(repo);
  assert.equal(report.decision, 'scoped-required');
  assert.deepEqual(report.newPackages, ['internal/agent/sshlogin']);
});

test('every invocation written in the workflows parses', () => {
  const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
  const invocations = ['release-kpanel', 'security-boundary-audit'].flatMap((name) =>
    readFileSync(join(root, '.codex-workflows', name + '.workflow.yaml'), 'utf8')
      .split('\n')
      .filter((line) => line.includes('check-security-audit-coverage.mjs') && line.trim().startsWith('node')));
  assert.equal(invocations.length, 2);
  for (const line of invocations) {
    const args = line.split('check-security-audit-coverage.mjs"')[1] ?? line.split('check-security-audit-coverage.mjs')[1];
    const argv = args.trim().split(/\s+/).map((part) => part.replace(/"\$\{\{[a-z_]+\}\}"/, 'HEAD'));
    assert.ok(parseArguments(argv), line);
  }
});

test('the exact command spellings used by the workflows parse', () => {
  assert.deepEqual(parseArguments(['--target', 'HEAD', '--require']), { validate: false, require: true, target: 'HEAD', format: 'text' });
  assert.equal(parseArguments(['--target', 'abc', '--format=json']).format, 'json');
  assert.equal(parseArguments(['--target=HEAD', '--format', 'json']).target, 'HEAD');
  assert.equal(parseArguments(['--target']), null);
  assert.equal(parseArguments(['--target', '--require']), null);
  assert.equal(parseArguments(['--format=yaml']), null);
});

test('legacy metadata shapes are read as recorded; new runs need exact identities', () => {
  const sha = 'a'.repeat(40);
  const legacyFull = normalizeRun('run-1', { run_status: 'complete', source_ref: sha + ' (main); worktree dirty: 19 files', scope_paths: null });
  assert.equal(legacyFull.mode, 'full');
  assert.equal(legacyFull.dirty, true);
  assert.deepEqual(validateRun(legacyFull), []);
  const legacyScoped = normalizeRun('run-3', { project_mode: 'scoped', run_status: 'incomplete-platform-interruption', source_ref: sha });
  assert.equal(legacyScoped.mode, 'scoped');
  assert.equal(legacyScoped.complete, false);
  const loose = normalizeRun('run-4', { scope_mode: 'scoped', run_status: 'complete', source_ref: sha + ' dirty' });
  const failures = validateRun(loose).join('\n');
  assert.match(failures, /exact 40-hex commit/);
  assert.match(failures, /source_dirty must be false/);
  assert.match(failures, /comparison_base/);
  assert.match(failures, /scope_complete/);
  assert.deepEqual(validateRun(normalizeRun('run-4', scoped(sha, sha))), []);
  assert.equal(normalizeRun('run-2', { scope_mode: 'scoped', source_ref: sha }).scopeComplete, true);
  assert.equal(normalizeRun('run-4', { scope_mode: 'scoped', source_ref: sha }).scopeComplete, false);
});

test('policy validation rejects stale or unexplained exclusions', (t) => {
  const { repo } = fixture(t);
  assert.deepEqual(validatePolicy(POLICY, repo), []);
  const stale = { ...POLICY, nonBoundary: { 'internal/gone': 'package was removed long ago', 'internal/version': 'x' } };
  const failures = validatePolicy(stale, repo).join('\n');
  assert.match(failures, /internal\/gone no longer exists/);
  assert.match(failures, /internal\/version needs a concrete reason/);
});

test('main exits 3 only with --require and a pending decision', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  commit(1, { 'internal/mcpaccess/access.go': 'new' });
  const write = process.stdout.write;
  process.stdout.write = () => true;
  try {
    assert.equal(main(['--validate'], repo), 0);
    assert.equal(main([], repo), 0);
    assert.equal(main(['--require'], repo), 3);
    assert.equal(main(['--require', '--format=json'], repo), 3);
    assert.equal(main(['--target', 'HEAD', '--require'], repo), 3);
  } finally {
    process.stdout.write = write;
  }
  const error = process.stderr.write;
  process.stderr.write = () => true;
  try {
    assert.equal(main(['--unknown'], repo), 2);
  } finally {
    process.stderr.write = error;
  }
});
