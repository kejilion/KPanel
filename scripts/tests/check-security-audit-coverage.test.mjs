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
  render,
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

test('a completed scoped run covers exactly its range; interrupted and partial runs cover nothing', (t) => {
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
  assert.deepEqual(validateRun(normalizeRun('run-7', scoped(base, second, 'complete', false)), repo), []);
  report = assess(repo);
  // run-6 starts after `first`, so the new package it did not audit stays pending; run-7 declared a partial scope.
  assert.deepEqual(report.covered, { 'run-6': 1 });
  assert.deepEqual(report.commits.map((c) => c.sha), [first]);
  assert.deepEqual(report.newPackages, ['internal/mcpaccess']);
  assert.deepEqual(report.partial.map((r) => r.name), ['run-7']);
  run('run-8', scoped(base, second));
  report = assess(repo);
  assert.equal(report.commits.length, 0);
  assert.equal(report.decision, 'ok');
});

test('completed commit slices cover only declared commits and compose without consuming pending changes', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  const first = commit(1, { 'internal/alpha/a.go': 'new' });
  const second = commit(2, { 'internal/beta/b.go': 'new' });
  const third = commit(3, { 'internal/alpha/a.go': 'updated' });
  // Even scope_complete=true cannot widen an explicit slice to the surrounding interval.
  const firstSlice = { ...scoped(base, third), reviewed_commits: [first, third] };
  run('run-5', firstSlice);
  assert.deepEqual(validateRun(normalizeRun('run-5', firstSlice), repo), []);
  let report = assess(repo);
  assert.deepEqual(report.covered, { 'run-5': 2 });
  assert.deepEqual(report.commits.map((entry) => entry.sha), [second]);
  assert.deepEqual(report.files, ['internal/beta/b.go']);
  assert.deepEqual(report.newPackages, ['internal/beta']);
  assert.equal(report.decision, 'scoped-required');
  assert.match(render(report, POLICY), /scoped_slice run=run-5 reviewed_commits=2/);
  const lastSlice = { ...scoped(base, third, 'complete', false), reviewed_commits: [second] };
  run('run-6', lastSlice);
  assert.deepEqual(validateRun(normalizeRun('run-6', lastSlice), repo), []);
  report = assess(repo);
  assert.deepEqual(report.covered, { 'run-5': 2, 'run-6': 1 });
  assert.equal(report.commits.length, 0);
  assert.equal(report.decision, 'ok');
  assert.deepEqual(report.partial, []);
  let output = '';
  const write = process.stdout.write;
  process.stdout.write = (chunk) => { output += chunk; return true; };
  try {
    assert.equal(main(['--format=json'], repo), 0);
  } finally {
    process.stdout.write = write;
  }
  const slices = JSON.parse(output).scoped;
  assert.equal(slices[0].coverage_kind, 'reviewed-commits');
  assert.deepEqual(slices[0].reviewed_commits, [first, third]);
});

test('explicit reviewed_commits rejects malformed or missing lists without falling back to interval coverage', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  const source = commit(1, { 'internal/alpha/a.go': 'new' });
  for (const reviewed of [undefined, null, [], source, ['short'], [source, source], [42]]) {
    const meta = { ...scoped(base, source), reviewed_commits: reviewed };
    assert.ok(validateRun(normalizeRun('run-5', meta), repo).length > 0);
  }
  for (const reviewed of [null, [], source, ['short'], [source, source], [42]]) {
    run('run-5', { ...scoped(base, source), reviewed_commits: reviewed });
    assert.throws(() => assess(repo), /reviewed_commits/);
    const stdout = process.stdout.write;
    const stderr = process.stderr.write;
    process.stdout.write = () => true;
    process.stderr.write = () => true;
    try {
      assert.equal(main(['--validate'], repo), 1);
      assert.equal(main(['--require'], repo), 1);
    } finally {
      process.stdout.write = stdout;
      process.stderr.write = stderr;
    }
  }
});

test('reviewed_commits must exist in the declared source interval', (t) => {
  const { repo, git, commit, run, base } = fixture(t);
  run('run-4', full(base));
  const source = commit(1, { 'internal/alpha/a.go': 'new' });
  const later = commit(2, { 'internal/agent/server.go': 'later' });
  git('checkout', '-qb', 'side', base);
  const side = commit(2, { 'internal/side/s.go': 'side' });
  git('checkout', '-q', '-');
  const cases = [
    { ...scoped(base, source), reviewed_commits: ['f'.repeat(40)] },
    { ...scoped(base, source), reviewed_commits: [base] },
    { ...scoped(base, source), reviewed_commits: [later] },
    { ...scoped(base, source), reviewed_commits: [side] },
    { ...scoped('f'.repeat(40), source), reviewed_commits: [source] },
    { ...scoped(base, 'f'.repeat(40)), reviewed_commits: [source] },
  ];
  for (const meta of cases) {
    assert.match(validateRun(normalizeRun('run-5', meta), repo).join('\n'), /does not exist|outside comparison_base\.\.source_ref/);
    run('run-5', meta);
    const stdout = process.stdout.write;
    const stderr = process.stderr.write;
    process.stdout.write = () => true;
    process.stderr.write = () => true;
    try {
      assert.equal(main(['--validate'], repo), 1);
      assert.equal(main(['--require'], repo), 1);
    } finally {
      process.stdout.write = stdout;
      process.stderr.write = stderr;
    }
  }
});

test('slice coverage is bound to complete status and declared source identity', (t) => {
  const { repo, git, commit, run, base } = fixture(t);
  run('run-4', full(base));
  const source = commit(1, { 'internal/alpha/a.go': 'new' });
  const slice = { ...scoped(base, source, 'complete', false), reviewed_commits: [source] };
  run('run-5', slice);
  assert.equal(assess(repo).commits.length, 0);
  for (const status of ['incomplete', 'incomplete-platform-interruption']) {
    run('run-5', { ...slice, run_status: status });
    const report = assess(repo);
    assert.deepEqual(report.covered, {});
    assert.deepEqual(report.commits.map((entry) => entry.sha), [source]);
    assert.deepEqual(report.interrupted.map((entry) => entry.name), ['run-5']);
  }
  run('run-5', { ...slice, source_ref: base });
  assert.throws(() => assess(repo), /outside comparison_base\.\.source_ref/);
  git('checkout', '-qb', 'side', base);
  const side = commit(2, { 'internal/side/s.go': 'side' });
  git('checkout', '-q', '-');
  run('run-5', { ...slice, source_ref: side, reviewed_commits: [side] });
  const report = assess(repo);
  assert.deepEqual(report.covered, {});
  assert.deepEqual(report.commits.map((entry) => entry.sha), [source]);
  assert.deepEqual(report.outside.map((entry) => entry.name), ['run-5']);
});

test('scoped runs on a feature branch beside the full run on a release branch both count after merging', (t) => {
  const { repo, git, commit, run, base } = fixture(t);
  const merge = (day, branch) => execFileSync('git', ['-C', repo, 'merge', '-q', '--no-ff', '-m', 'merge ' + branch, branch], {
    encoding: 'utf8', env: { ...process.env, GIT_AUTHOR_DATE: new Date((START + day * DAY) * 1000).toISOString(),
      GIT_COMMITTER_DATE: new Date((START + day * DAY) * 1000).toISOString() },
  });
  git('checkout', '-qb', 'release');
  const release = commit(1, { 'internal/version/version.go': 'v2', 'internal/agent/server.go': 'release' });
  git('checkout', '-q', '-');
  git('checkout', '-qb', 'feature');
  commit(1, { 'internal/auth/passkey.go': 'store' });
  const feature = commit(2, { 'internal/auth/passkey.go': 'login' });
  git('checkout', '-q', '-');
  merge(3, 'release');
  merge(3, 'feature');
  run('run-4', full(release));
  run('run-6', scoped(base, feature));
  const report = assess(repo);
  assert.equal(report.lastFull.name, 'run-4');
  assert.deepEqual(report.covered, { 'run-6': 2 });
  assert.equal(report.commits.length, 0);
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

test('a package introduced only by a merge resolution is still a new boundary package', (t) => {
  const { repo, git, commit, run, base } = fixture(t);
  run('run-4', full(base));
  git('checkout', '-qb', 'side');
  commit(1, { 'internal/agent/side.go': 'side' });
  git('checkout', '-q', '-');
  commit(1, { 'internal/agent/main.go': 'main' });
  execFileSync('git', ['-C', repo, 'merge', '-q', '--no-ff', '--no-commit', 'side'], { encoding: 'utf8' });
  mkdirSync(join(repo, 'internal/evil'), { recursive: true });
  writeFileSync(join(repo, 'internal/evil/e.go'), 'package evil\n');
  git('add', '-A');
  commit(2, {}, 'merge side');
  const report = assess(repo);
  assert.deepEqual(report.newPackages, ['internal/evil']);
  assert.equal(report.decision, 'scoped-required');
});

test('a scoped run whose base object is missing is partial, and removed packages are not reported as new', (t) => {
  const { repo, commit, run, base } = fixture(t);
  run('run-4', full(base));
  const added = commit(1, { 'internal/temp/t.go': 'temp' });
  run('run-5', scoped('f'.repeat(40), added));
  execFileSync('git', ['-C', repo, 'rm', '-rq', 'internal/temp'], { encoding: 'utf8' });
  commit(2, {}, 'drop temp');
  const report = assess(repo);
  assert.deepEqual(report.partial.map((r) => r.name), ['run-5']);
  assert.deepEqual(report.newPackages, []);
  assert.equal(report.commits.length, 2);
});

test('the newest full run by source time sets the age clock, whatever its run number', (t) => {
  const { repo, commit, run, base } = fixture(t);
  const later = commit(10, { 'internal/agent/server.go': 'v2' });
  run('run-4', full(later));
  run('run-5', full(base));
  commit(35, { 'README.md': 'x' });
  const report = assess(repo);
  assert.equal(report.lastFull.name, 'run-4');
  assert.equal(report.fullAgeDays, 25);
  assert.equal(report.decision, 'ok');
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
  assert.equal(invocations.length, 3);
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
  const skillShape = { run_status: 'complete', source_ref: sha, source_dirty: false, project_mode: 'full' };
  assert.deepEqual(validateRun(normalizeRun('run-4', skillShape)), []);
  assert.equal(normalizeRun('run-4', skillShape).mode, 'full');
  assert.match(validateRun(normalizeRun('run-4', { ...skillShape, project_mode: 'fast' })).join('\n'), /must be full or scoped/);
  assert.match(validateRun(normalizeRun('run-4', { ...skillShape, scope_mode: 'scoped' })).join('\n'), /disagree/);
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
