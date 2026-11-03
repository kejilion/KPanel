import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { qualify, qualifyGovernance, qualifySourceIdentity } from '../task-preflight.mjs';
import { firstPartySources } from '../torrent-backport-baseline.mjs';

const repoRoot = resolve(import.meta.dirname, '../..');

function governanceFixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-governance-preflight-'));
  mkdirSync(join(root, 'scripts/tests'), { recursive: true });
  for (const path of ['go.mod', 'go.sum', 'scripts/check-release-notes.sh', 'scripts/tests/example.test.mjs']) {
    writeFileSync(join(root, path), 'fixture\n');
  }
  writeFileSync(join(root, 'scripts/verify-governance.sh'), 'node --test scripts/tests/example.test.mjs\n');
  return root;
}

test('governance native and repo Bash probes share the explicitly selected Go environment', () => {
  const root = governanceFixture();
  try {
    const goRoot = join(root, 'selected-go'); mkdirSync(join(goRoot, 'bin'), { recursive: true });
    writeFileSync(join(goRoot, 'bin', process.platform === 'win32' ? 'go.exe' : 'go'), 'probe fixture only');
    const probes = [];
    const report = qualifyGovernance({ repo: root, environment: { ...process.env, GOROOT: goRoot },
      probe(command, args, options) { probes.push(options.env); return { status: 0 }; }, checkSources() {} });
    assert.equal(report.status, 'passed');
    assert.equal(probes.length, 4);
    assert.ok(probes.every(env => env.PATH.startsWith(join(goRoot, 'bin')) && env.GOTOOLCHAIN === 'local'));
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('governance preflight groups tool and input failures before the unchanged suite', () => {
  const root = governanceFixture();
  try {
    rmSync(join(root, 'go.sum'));
    rmSync(join(root, 'scripts/tests/example.test.mjs'));
    const probes = [];
    const report = qualifyGovernance({ repo: root, probe(command, args, options) {
      probes.push({ command, args });
      assert.equal(options.shell, false);
      assert.equal(options.timeout, 5000);
      assert.equal(options.env.GOTOOLCHAIN, 'local');
      return { status: null, error: new Error('broken shim; private output must not be logged') };
    }, checkSources() { throw new Error('Backport source digest mismatch: fixture'); } });
    assert.equal(report.status, 'failed');
    assert.equal(report.failures.length, 6);
    assert.match(report.failures.join('\n'), /go.sum/);
    assert.match(report.failures.join('\n'), /example.test.mjs/);
    assert.doesNotMatch(JSON.stringify(report), /private output/);
    assert.deepEqual(probes.map((p) => p.args), [['--version'], ['--version'], ['version'], ['-c', 'go version']]);
    assert.ok(probes.every((p) => !p.args.includes('--test')));
    assert.equal(report.qualificationOnly, true);
    assert.equal(report.permissionsGranted, false);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('governance preflight derives newly added tests from the gate and rejects empty inventories', () => {
  const root = governanceFixture();
  const probe = () => ({ status: 0 });
  const checkSources = () => {};
  try {
    assert.equal(qualifyGovernance({ repo: root, probe, checkSources }).status, 'passed');
    writeFileSync(join(root, 'scripts/verify-governance.sh'), 'node --test scripts/tests/new-check.test.mjs\n');
    assert.match(qualifyGovernance({ repo: root, probe, checkSources }).failures.join('\n'), /new-check.test.mjs/);
    writeFileSync(join(root, 'scripts/verify-governance.sh'), 'node --test\n');
    assert.match(qualifyGovernance({ repo: root, probe, checkSources }).failures.join('\n'), /inventory is empty/);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('governance preflight rejects signals and keeps canonical source byte validation', () => {
  const root = governanceFixture();
  try {
    const report = qualifyGovernance({ repo: root, probe: () => ({ status: 0, signal: 'SIGTERM' }) });
    assert.equal(report.checks.filter((check) => check.status === 'failed').length, 5);
    assert.ok(report.failures.some((failure) => failure.startsWith('vendor-source-bytes:')));
    assert.equal(report.status, 'failed');
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('canonical governance gate stops on failed preflight instead of launching tests', () => {
  const root = governanceFixture();
  try {
    cpSync(join(repoRoot, 'scripts/verify-governance.sh'), join(root, 'scripts/verify-governance.sh'));
    const envFile = join(root, 'bash-env.sh');
    const log = join(root, 'commands.log');
    writeFileSync(envFile, 'node() { printf "%s\\n" "$*" >> "$PREFLIGHT_CALL_LOG"; return 17; }\nexport -f node\n');
    const result = spawnSync(process.execPath, [join(repoRoot, 'scripts/run-repo-bash.mjs'), 'scripts/verify-governance.sh'], {
      cwd: root, encoding: 'utf8', env: { ...process.env, PREFLIGHT_CALL_LOG: log,
        BASH_ENV: envFile.replace(/^([A-Za-z]):/, (_, drive) => '/' + drive.toLowerCase()).replaceAll('\\', '/') },
      timeout: 10_000, windowsHide: true,
    });
    assert.equal(result.status, 17, result.stderr);
    const commands = readFileSync(log, 'utf8').trim().split(/\r?\n/);
    assert.equal(commands.length, 1);
    assert.match(commands[0], /^scripts\/task-preflight.mjs governance --repo /);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('governance CLI cannot execute a supplied contract or arbitrary probe', () => {
  for (const args of [['--contract', 'should-not-be-read.json'], ['--command', 'echo unsafe']]) {
    const result = spawnSync(process.execPath, [join(repoRoot, 'scripts/task-preflight.mjs'), 'governance', ...args], { encoding: 'utf8' });
    assert.notEqual(result.status, 0);
    assert.equal(JSON.parse(result.stdout).status, 'failed');
  }
});
function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-task-qualification-'));
  const management = join(root, 'management');
  const writer = join(root, 'writer');
  const git = (cwd, ...args) => execFileSync('git', ['-C', cwd, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  mkdirSync(join(management, 'scripts'), { recursive: true });
  git(management, 'init', '--initial-branch=main');
  git(management, 'config', 'user.name', 'KPanel Test');
  git(management, 'config', 'user.email', 'kpanel@example.invalid');
  for (const name of ['check-collaboration-state.mjs', 'check-security-audit-coverage.mjs', 'report-governance-health.mjs', 'security-review-scope.mjs']) cpSync(join(repoRoot, 'scripts', name), join(management, 'scripts', name));
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

test('L3 ready and handoff reject stale first-party observations before release validation', () => {
  const f = fixture();
  try {
    mkdirSync(join(f.writer, 'cmd'));
    mkdirSync(join(f.writer, 'internal/version'), { recursive: true });
    const versionFile = join(f.writer, 'internal/version/version.go');
    writeFileSync(versionFile, 'package version\nvar Version = "1.29.0-rc.6-dev"\n');
    const scope = join(f.writer, 'scripts/torrent-backport-scan-scope.json');
    const policy = { firstPartySourceSha256: firstPartySources(f.writer).sha256 };
    writeFileSync(scope, JSON.stringify(policy));
    f.git(f.writer, 'add', '.'); f.git(f.writer, 'commit', '-m', 'test: observed source');
    f.contract.riskLevel = 'L3';
    assert.equal(f.run('ready').status, 'passed');
    const before = readFileSync(scope, 'utf8');
    writeFileSync(versionFile, 'package version\nvar Version = "1.29.0-dev"\n');
    f.git(f.writer, 'add', '.'); f.git(f.writer, 'commit', '-m', 'test: stable version');
    for (const phase of ['ready', 'handoff']) {
      const result = f.run(phase);
      assert.equal(result.status, 'failed');
      assert.ok(result.failures.some(message => message.includes('first-party source observation changed')));
    }
    assert.equal(readFileSync(scope, 'utf8'), before, 'qualification never refreshes reviewed policy');
    policy.firstPartySourceSha256 = firstPartySources(f.writer).sha256;
    writeFileSync(scope, JSON.stringify(policy));
    f.git(f.writer, 'add', '.'); f.git(f.writer, 'commit', '-m', 'test: reviewed rebind');
    assert.equal(f.run('ready').status, 'passed');
    rmSync(scope); f.git(f.writer, 'add', '.'); f.git(f.writer, 'commit', '-m', 'test: missing observation');
    assert.ok(f.run('ready').failures.some(message => message.startsWith('source-identity:')));
    f.contract.riskLevel = 'L2';
    assert.equal(f.run('ready').status, 'passed', 'ordinary tasks do not gain a release-only prerequisite');
  } finally { f.cleanup(); }
});

test('source-identity CLI is read-only and fails closed on malformed or stale observations', () => {
  const root = governanceFixture();
  try {
    mkdirSync(join(root, 'cmd')); mkdirSync(join(root, 'internal'));
    writeFileSync(join(root, 'cmd/main.go'), 'package main\n');
    const policy = join(root, 'scripts/torrent-backport-scan-scope.json');
    writeFileSync(policy, JSON.stringify({ firstPartySourceSha256: firstPartySources(root).sha256 }));
    assert.equal(qualifySourceIdentity(root).status, 'passed');
    for (const body of ['{bad', '{}', JSON.stringify({ firstPartySourceSha256: '0'.repeat(64) })]) {
      writeFileSync(policy, body);
      const result = spawnSync(process.execPath, [join(repoRoot, 'scripts/task-preflight.mjs'), 'source-identity', '--repo', root], { encoding: 'utf8' });
      assert.equal(result.status, 1);
      assert.equal(JSON.parse(result.stdout).status, 'failed');
      assert.equal(readFileSync(policy, 'utf8'), body);
    }
  } finally { rmSync(root, { recursive: true, force: true }); }
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
