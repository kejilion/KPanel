import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const orchestrator = resolve(repoRoot, 'scripts', 'run-production-evidence.mjs');
const remoteScript = resolve(repoRoot, 'scripts', 'run-production-evidence-remote.sh');

function createFixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-production-evidence-test-'));
  const repo = join(root, 'repo');
  mkdirSync(join(repo, 'scripts'), { recursive: true });
  spawnSync('git', ['init', '--initial-branch=main', repo], { encoding: 'utf8', shell: false });
  writeFileSync(join(repo, 'scripts', 'run-production-evidence-remote.sh'), readFileSync(remoteScript));
  return { root, repo };
}

function cleanup(root) {
  const prefix = resolve(tmpdir()) + (process.platform === 'win32' ? '\\' : '/');
  assert.ok(resolve(root).startsWith(prefix));
  rmSync(root, { recursive: true, force: true });
}

const revision = 'a'.repeat(40);
const digest = `sha256:${'b'.repeat(64)}`;
const baseline = 'baseline-run';
const phaseFields = {
  preflight: {},
  backup: { '--baseline-run-id': baseline },
  postdeploy: {
    '--expected-revision': revision,
    '--expected-image-digest': digest,
    '--baseline-run-id': baseline,
  },
};

for (const [name, phase, fields] of [
  ...Object.entries(phaseFields).map(([phase, fields]) => [phase, phase, fields]),
  ['preflight placeholders', 'preflight', {
    '--expected-revision': '-', '--expected-image-digest': '-', '--baseline-run-id': '-',
  }],
  ['backup placeholders', 'backup', {
    ...phaseFields.backup, '--expected-revision': '-', '--expected-image-digest': '-',
  }],
]) {
  test(`prepare-only accepts ${name} and preserves phase fields`, () => {
    const fixture = createFixture();
    try {
      const artifactDir = join(fixture.root, 'artifacts');
      const result = spawnSync(process.execPath, [
        orchestrator, '--repo', fixture.repo, '--phase', phase,
        '--run-id', 'phase-contract', '--expected-version', '1.4.1',
        '--artifact-dir', artifactDir, '--prepare-only', ...Object.entries(fields).flat(),
      ], { cwd: fixture.repo, encoding: 'utf8', shell: false });
      assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
      const plan = readFileSync(join(artifactDir, 'plan.env'), 'utf8');
      const manifest = JSON.parse(readFileSync(join(artifactDir, 'manifest.json'), 'utf8'));
      assert.equal(manifest.phase, phase);
      for (const [option, key, manifestKey] of [
        ['--expected-revision', 'EXPECTED_REVISION', 'expectedRevision'],
        ['--expected-image-digest', 'EXPECTED_IMAGE_DIGEST', 'expectedImageDigest'],
        ['--baseline-run-id', 'BASELINE_RUN_ID', 'baselineRunId'],
      ]) {
        assert.ok(plan.split('\n').includes(`${key}=${fields[option] ?? '-'}`));
        assert.equal(manifest[manifestKey], fields[option] ?? null);
      }
    } finally {
      cleanup(fixture.root);
    }
  });
}

const invalidCases = [];
for (const [phase, options] of [
  ['preflight', Object.entries(phaseFields.postdeploy)],
  ['backup', [['--expected-revision', revision], ['--expected-image-digest', digest]]],
]) {
  for (const [option, value] of options) {
    invalidCases.push({ phase, name: `inapplicable ${option}`, fields: { ...phaseFields[phase], [option]: value },
      message: `${phase} does not accept ${option}` });
  }
}
invalidCases.push({
  phase: 'preflight', name: 'postdeploy revision and digest together',
  fields: { '--expected-revision': revision, '--expected-image-digest': digest },
  message: 'preflight does not accept --expected-revision',
});
for (const [phase, option, message] of [
  ['backup', '--baseline-run-id', 'backup requires a valid baseline run ID'],
  ['postdeploy', '--expected-revision', 'postdeploy requires a full expected revision'],
  ['postdeploy', '--expected-image-digest', 'postdeploy requires an immutable expected image digest'],
  ['postdeploy', '--baseline-run-id', 'postdeploy requires a valid baseline run ID'],
]) {
  for (const [name, value] of [['missing', undefined], ['placeholder', '-'], ['malformed', 'bad/value']]) {
    const fields = { ...phaseFields[phase], [option]: value };
    if (value === undefined) delete fields[option];
    invalidCases.push({ phase, name: `${name} ${option}`, fields, message });
  }
}

for (const prepareOnly of [false, true]) {
  for (const { phase, name, fields, message } of invalidCases) {
    test(`${phase} rejects ${name} before artifacts or subprocesses (prepare-only=${prepareOnly})`, () => {
      const fixture = createFixture();
      try {
        const artifactDir = join(fixture.root, 'artifacts');
        const calls = join(fixture.root, 'subprocess-calls');
        const ready = join(fixture.root, 'guard-ready');
        // Intercept the imported builtin before the CLI loads, including ordinary execution mode.
        const guard = `
          import childProcess from 'node:child_process';
          import { syncBuiltinESMExports } from 'node:module';
          import { appendFileSync, writeFileSync } from 'node:fs';
          childProcess.spawnSync = (command) => {
            appendFileSync(${JSON.stringify(calls)}, command + '\\n');
            throw new Error('unexpected subprocess: ' + command);
          };
          syncBuiltinESMExports();
          writeFileSync(${JSON.stringify(ready)}, 'ready');
        `;
        const result = spawnSync(process.execPath, [
          '--import', `data:text/javascript,${encodeURIComponent(guard)}`,
          orchestrator, '--repo', fixture.repo, '--phase', phase,
          '--run-id', 'phase-contract', '--expected-version', '1.4.1',
          '--artifact-dir', artifactDir, ...(prepareOnly ? ['--prepare-only'] : []),
          ...Object.entries(fields).flat(),
        ], { cwd: fixture.repo, encoding: 'utf8', shell: false });
        assert.equal(result.status, 1, `${result.stdout}\n${result.stderr}`);
        assert.ok(existsSync(ready), 'subprocess guard must have loaded');
        assert.equal(existsSync(artifactDir), false, 'invalid arguments must not create artifacts');
        assert.equal(existsSync(calls), false, 'invalid arguments must not start Git, SSH, or SCP');
        assert.ok(result.stderr.includes(message), result.stderr);
        assert.doesNotMatch(result.stdout, /production_evidence_prepare=pass/);
      } finally {
        cleanup(fixture.root);
      }
    });
  }
}

test('prepare-only creates a hashed preflight plan without shell interpolation', () => {
  const fixture = createFixture();
  try {
    const artifactDir = join(fixture.root, 'artifacts');
    const result = spawnSync(process.execPath, [
      orchestrator,
      '--repo', fixture.repo,
      '--phase', 'preflight',
      '--run-id', 'v0.98.0-production',
      '--expected-version', '0.97.3',
      '--artifact-dir', artifactDir,
      '--prepare-only',
    ], { cwd: fixture.repo, encoding: 'utf8', shell: false });
    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
    const plan = readFileSync(join(artifactDir, 'plan.env'), 'utf8');
    assert.match(plan, /^PHASE=preflight$/m);
    assert.match(plan, /^EXPECTED_VERSION=0\.97\.3$/m);
    assert.match(plan, /^REMOTE_SCRIPT_SHA256=[0-9a-f]{64}$/m);
  } finally {
    cleanup(fixture.root);
  }
});

test('postdeploy fails closed without immutable revision, digest, and baseline', () => {
  const fixture = createFixture();
  try {
    const result = spawnSync(process.execPath, [
      orchestrator,
      '--repo', fixture.repo,
      '--phase', 'postdeploy',
      '--run-id', 'v0.98.0-production',
      '--expected-version', '0.98.0',
      '--artifact-dir', join(fixture.root, 'postdeploy'),
      '--prepare-only',
    ], { cwd: fixture.repo, encoding: 'utf8', shell: false });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /full expected revision/);
  } finally {
    cleanup(fixture.root);
  }
});

test('production evidence target is fixed to arena-154', () => {
  const fixture = createFixture();
  try {
    const result = spawnSync(process.execPath, [
      orchestrator,
      '--repo', fixture.repo,
      '--target', '108',
      '--phase', 'preflight',
      '--run-id', 'v0.98.0-production',
      '--expected-version', '0.97.3',
      '--artifact-dir', join(fixture.root, 'wrong-target'),
      '--prepare-only',
    ], { cwd: fixture.repo, encoding: 'utf8', shell: false });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /must be arena-154/);
  } finally {
    cleanup(fixture.root);
  }
});

test('remote production entrypoint is strict and never evaluates the plan', () => {
  const bash = process.env.KPANEL_TEST_BASH ||
    (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');
  const syntax = spawnSync(bash, ['-n', 'scripts/run-production-evidence-remote.sh'], {
    cwd: repoRoot,
    encoding: 'utf8',
  });
  assert.equal(syntax.status, 0, `${syntax.stdout}\n${syntax.stderr}`);
  const content = readFileSync(remoteScript, 'utf8');
  assert.match(content, /duplicate production plan key/);
  assert.match(content, /docker compose -f "\$compose" stop/);
  assert.match(content, /protected\.sha256/);
  assert.match(content, /sha256sum -c SHA256SUMS/);
  assert.match(content, /for _ in \$\(seq 1 10\); do/);
  assert.match(content, /\[ "\$health_ready" = true \]/);
  assert.match(content, /production_ready\(\)/);
  assert.match(content, /if production_ready; then break; fi/);
  assert.doesNotMatch(content, /\beval\b/);
  assert.doesNotMatch(content, /(?:^|\n)\s*(?:source|\.)\s+"?\$plan/m);
});
