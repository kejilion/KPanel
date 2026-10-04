import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { qualify } from '../task-preflight.mjs';

const repoRoot = resolve(import.meta.dirname, '../..');
function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-task-qualification-'));
  const management = join(root, 'management');
  const writer = join(root, 'writer');
  const git = (cwd, ...args) => execFileSync('git', ['-C', cwd, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  mkdirSync(join(management, 'scripts'), { recursive: true });
  git(management, 'init', '--initial-branch=main');
  git(management, 'config', 'user.name', 'KPanel Test');
  git(management, 'config', 'user.email', 'kpanel@example.invalid');
  for (const name of ['check-collaboration-state.mjs', 'check-security-audit-coverage.mjs', 'report-governance-health.mjs']) cpSync(join(repoRoot, 'scripts', name), join(management, 'scripts', name));
  cpSync(join(repoRoot, 'dependency-policy.json'), join(management, 'dependency-policy.json'));
  writeFileSync(join(management, 'notes.md'), 'baseline\n');
  git(management, 'add', '.'); git(management, 'commit', '-m', 'test: baseline');
  const base = git(management, 'rev-parse', 'HEAD');
  git(management, 'worktree', 'add', '-b', 'docs/test', writer, base);
  const contract = { schemaVersion: 1, scope: 'test governance', nonGoals: 'release and production', baseCommit: base, allowedPaths: ['notes.md'], forbiddenPaths: [],
    riskLevel: 'L2', scriptLinkageState: 'not-required', authorization: 'test local contract; declarations grant no permissions',
    permissions: { localCommit: true, push: false, main: false, release: false, production: false }, requiredTools: ['git', 'node'], requiredFiles: ['notes.md'],
    validations: [{ id: 'governance', environment: 'fixture', tools: 'node-test', parameters: 'fixed-regression', argv: ['node', '--test'] }] };
  const run = (phase, value = contract) => qualify({ repo: writer, contract: value, phase, contractDir: root, now: Date.parse('2026-10-04T00:00:00Z') });
  const candidate = () => { writeFileSync(join(writer, 'notes.md'), 'candidate\n'); git(writer, 'add', '.'); git(writer, 'commit', '-m', 'docs: candidate'); contract.candidateCommit = git(writer, 'rev-parse', 'HEAD'); };
  const receipt = () => {
    writeFileSync(join(root, 'original.log'), 'actual fixture result\n');
    contract.evidence = [{ ...contract.validations[0], candidateCommit: contract.candidateCommit, status: 'passed', exitCode: 0,
      finishedAt: '2026-10-03T23:00:00Z', expiresAt: '2026-10-05T00:00:00Z', log: 'original.log', sha256: createHash('sha256').update(readFileSync(join(root, 'original.log'))).digest('hex') }];
  };
  return { root, management, writer, git, contract, run, candidate, receipt, cleanup: () => rmSync(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }) };
}

test('ready and exact committed handoff pass while remaining read-only qualifications', () => {
  const f = fixture();
  try {
    const ready = f.run('ready');
    assert.equal(ready.status, 'passed', ready.failures.join('\n'));
    assert.equal(f.run('handoff').status, 'failed', 'empty candidate must fail');
    f.candidate(); f.receipt();
    const result = f.run('handoff');
    assert.equal(result.status, 'passed', result.failures.join('\n'));
    assert.equal(result.permissionsGranted, false);
    assert.equal(result.qualificationOnly, true);
    assert.equal(f.git(f.writer, 'status', '--porcelain'), '');
    assert.equal(qualify({ repo: f.management, contract: f.contract, phase: 'ready' }).status, 'failed');
  } finally { f.cleanup(); }
});

test('preflight groups malformed and missing inputs without executing supplied argv', () => {
  const f = fixture();
  try {
    const bad = { ...f.contract, baseCommit: 'origin/main', allowedPaths: ['../escape'], riskLevel: 'L9' };
    assert.ok(f.run('ready', bad).failures.length >= 3);
    f.contract.requiredFiles = ['missing-one', 'missing-two'];
    f.contract.validations[0].argv = ['sh', '-c', 'touch should-never-exist'];
    const result = f.run('ready');
    assert.equal(result.status, 'failed');
    assert.ok(result.failures.some((x) => x.includes('missing-one')));
    assert.ok(result.failures.some((x) => x.includes('missing-two')));
    assert.ok(!existsSync(join(f.writer, 'should-never-exist')));
  } finally { f.cleanup(); }
});

test('handoff refuses dirty, out-of-scope and forbidden changes', () => {
  const f = fixture();
  try {
    f.candidate(); f.receipt();
    f.contract.allowedPaths = ['scripts/'];
    assert.ok(f.run('handoff').failures.some((x) => x.includes('out-of-scope')));
    f.contract.allowedPaths = ['notes.md']; f.contract.forbiddenPaths = ['notes.md'];
    assert.ok(f.run('handoff').failures.some((x) => x.includes('out-of-scope')));
    f.contract.forbiddenPaths = []; writeFileSync(join(f.writer, 'notes.md'), 'dirty\n');
    assert.ok(f.run('handoff').failures.some((x) => x.includes('checkpoint')));
  } finally { f.cleanup(); }
});

test('candidate, environment, tool, parameter, argv, exit, expiry and log damage invalidate evidence', () => {
  const f = fixture();
  try {
    f.candidate(); f.receipt();
    for (const [key, value] of Object.entries({ candidateCommit: '0'.repeat(40), environment: 'other', tools: 'changed', parameters: 'changed', argv: ['other'], exitCode: 1, status: 'running', expiresAt: '2026-10-03T00:00:00Z', finishedAt: 'tomorrow', log: 'missing.log', sha256: '0'.repeat(64) })) {
      const old = f.contract.evidence[0][key]; f.contract.evidence[0][key] = value;
      assert.equal(f.run('handoff').status, 'failed', key); f.contract.evidence[0][key] = old;
    }
    f.contract.evidence.push(f.contract.evidence[0]); assert.equal(f.run('handoff').status, 'failed'); f.contract.evidence.pop();
    writeFileSync(join(f.root, 'original.log'), 'corrupted\n'); assert.equal(f.run('handoff').status, 'failed');
  } finally { f.cleanup(); }
});

test('CLI rejects oversized and malformed contracts with JSON failure', () => {
  const f = fixture();
  try {
    for (const body of ['{bad', 'x'.repeat(65 * 1024)]) {
      writeFileSync(join(f.root, 'contract.json'), body);
      const result = spawnSync(process.execPath, [join(repoRoot, 'scripts/task-preflight.mjs'), 'ready', '--repo', f.writer, '--contract', join(f.root, 'contract.json')], { encoding: 'utf8' });
      assert.notEqual(result.status, 0); assert.equal(JSON.parse(result.stdout).status, 'failed');
    }
  } finally { f.cleanup(); }
});
