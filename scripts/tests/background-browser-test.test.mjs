import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { atomicWrite } from '../background-browser-test.mjs';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const runner = resolve(repoRoot, 'scripts', 'background-browser-test.mjs');

test('atomic state replacement retries transient Windows locks without exposing partial JSON', () => {
  for (const code of ['EPERM', 'EACCES', 'EBUSY']) {
    const path = join(mkdtempSync(join(tmpdir(), 'kpanel-state-lock-')), 'state.json');
    writeFileSync(path, JSON.stringify({ status: 'running' }));
    let attempts = 0;
    const delays = [];
    atomicWrite(path, { status: 'timed_out', exitCode: 1 }, {
      platform: 'win32',
      replace: (source, target) => {
        assert.deepEqual(JSON.parse(readFileSync(target, 'utf8')), { status: 'running' });
        if (++attempts < 3) throw Object.assign(new Error('reader holds destination'), { code });
        renameSync(source, target);
      },
      pause: (ms) => delays.push(ms),
    });
    assert.equal(attempts, 3);
    assert.deepEqual(delays, [20, 40]);
    assert.deepEqual(JSON.parse(readFileSync(path, 'utf8')), { status: 'timed_out', exitCode: 1 });
  }
});

test('atomic state replacement fails closed when a Windows lock persists', () => {
  const path = join(mkdtempSync(join(tmpdir(), 'kpanel-state-lock-')), 'state.json');
  writeFileSync(path, JSON.stringify({ status: 'running' }));
  const failure = Object.assign(new Error('persistent lock'), { code: 'EPERM' });
  let attempts = 0;
  const delays = [];
  assert.throws(() => atomicWrite(path, { status: 'timed_out' }, {
    platform: 'win32', replace: () => { attempts += 1; throw failure; }, pause: (ms) => delays.push(ms),
  }), (error) => error === failure);
  assert.equal(attempts, 5);
  assert.deepEqual(delays, [20, 40, 60, 80]);
  assert.deepEqual(JSON.parse(readFileSync(path, 'utf8')), { status: 'running' });
  assert.deepEqual(JSON.parse(readFileSync(`${path}.${process.pid}.tmp`, 'utf8')), { status: 'timed_out' });
});

test('atomic state replacement does not retry unrelated errors or non-Windows platforms', () => {
  for (const [platform, code] of [['linux', 'EPERM'], ['win32', 'ENOSPC'], ['win32', 'EXDEV']]) {
    const path = join(mkdtempSync(join(tmpdir(), 'kpanel-state-lock-')), 'state.json');
    const failure = Object.assign(new Error('permanent failure'), { code });
    let attempts = 0;
    assert.throws(() => atomicWrite(path, { status: 'failed' }, {
      platform, replace: () => { attempts += 1; throw failure; },
      pause: () => assert.fail('unexpected retry'),
    }), (error) => error === failure);
    assert.equal(attempts, 1);
  }
});

function waitForTerminalState(statePath) {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const state = JSON.parse(readFileSync(statePath, 'utf8'));
    if (['passed', 'failed', 'timed_out'].includes(state.status)) return state;
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 50);
  }
  throw new Error('background test did not finish');
}

test('background browser runner returns immediately and preserves result evidence', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-browser-job-'));
  const evidenceDir = join(root, 'evidence');
  const specPath = join(root, 'spec.json');
  writeFileSync(specPath, JSON.stringify({
    command: process.execPath,
    args: ['-e', 'console.log("browser-job-ok")'],
    cwd: repoRoot,
  }));
  const output = execFileSync(process.execPath, [
    runner,
    'start',
    '--environment', 'arena-154',
    '--candidate-ref', '0000000000000000000000000000000000000000',
    '--spec', specPath,
    '--evidence-dir', evidenceDir,
    '--timeout-seconds', '10',
  ], { encoding: 'utf8' });
  const job = JSON.parse(output);
  assert.equal(job.evidenceDir, evidenceDir);
  const state = waitForTerminalState(join(evidenceDir, 'state.json'));
  assert.equal(state.status, 'passed');
  assert.equal(state.exitCode, 0);
  assert.equal(state.candidateRef, '0000000000000000000000000000000000000000');
  assert.match(readFileSync(join(evidenceDir, 'browser-test.log'), 'utf8'), /browser-job-ok/);
  assert.deepEqual(
    JSON.parse(readFileSync(join(evidenceDir, 'command-spec.json'), 'utf8')),
    JSON.parse(readFileSync(specPath, 'utf8')),
  );
});

test('background browser runner rejects prod-108 before launching', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-browser-job-'));
  const specPath = join(root, 'spec.json');
  writeFileSync(specPath, JSON.stringify({ command: process.execPath, args: ['-e', 'process.exit(0)'] }));
  assert.throws(() => execFileSync(process.execPath, [
    runner,
    'start',
    '--environment', 'prod-108',
    '--candidate-ref', '0000000000000000000000000000000000000000',
    '--spec', specPath,
    '--evidence-dir', join(root, 'evidence'),
    '--timeout-seconds', '10',
  ], { encoding: 'utf8', stdio: 'pipe' }));
});

test('background browser runner records a hard timeout as failure evidence', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-browser-job-'));
  const evidenceDir = join(root, 'evidence');
  const specPath = join(root, 'spec.json');
  writeFileSync(specPath, JSON.stringify({
    command: process.execPath,
    args: ['-e', 'setTimeout(() => {}, 5000)'],
  }));
  execFileSync(process.execPath, [
    runner,
    'start',
    '--environment', 'arena-154',
    '--candidate-ref', '0000000000000000000000000000000000000000',
    '--spec', specPath,
    '--evidence-dir', evidenceDir,
    '--timeout-seconds', '1',
  ], { encoding: 'utf8' });
  const state = waitForTerminalState(join(evidenceDir, 'state.json'));
  assert.equal(state.status, 'timed_out');
  assert.notEqual(state.exitCode, 0);
});

test('background browser runner rejects mutable candidate aliases', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-browser-job-'));
  const specPath = join(root, 'spec.json');
  writeFileSync(specPath, JSON.stringify({ command: process.execPath, args: ['-e', 'process.exit(0)'] }));
  assert.throws(() => execFileSync(process.execPath, [
    runner,
    'start',
    '--environment', 'arena-154',
    '--candidate-ref', 'HEAD',
    '--spec', specPath,
    '--evidence-dir', join(root, 'evidence'),
    '--timeout-seconds', '10',
  ], { encoding: 'utf8', stdio: 'pipe' }));
});

test('background browser runner refuses persisted secrets in command specs', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-browser-job-'));
  const specPath = join(root, 'spec.json');
  writeFileSync(specPath, JSON.stringify({
    command: process.execPath,
    args: ['-e', 'process.exit(0)'],
    env: { TEST_TOKEN: 'must-not-be-written' },
  }));
  assert.throws(() => execFileSync(process.execPath, [
    runner,
    'start',
    '--environment', 'arena-154',
    '--candidate-ref', '0000000000000000000000000000000000000000',
    '--spec', specPath,
    '--evidence-dir', join(root, 'evidence'),
    '--timeout-seconds', '10',
  ], { encoding: 'utf8', stdio: 'pipe' }));
});
