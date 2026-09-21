import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const scriptPath = resolve(repoRoot, 'scripts', 'security-scan.sh');
const ignorePath = resolve(repoRoot, '.trivyignore.yaml');

test('nested Trivy scans receive ambient proxy names without persisting values', () => {
  const script = readFileSync(scriptPath, 'utf8');
  assert.match(script, /HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy/);
  assert.match(script, /proxy_args\+=\(--env "\$proxy_name"\)/);
  assert.match(script, /network_args\+=\(--network host\)/);
  assert.doesNotMatch(script, /--env "\$proxy_name=/);
  assert.doesNotMatch(script, /echo[^\n]*\$\{!proxy_name/);
});

test('the release-runner root exception is narrow, reasoned and time bounded', () => {
  const script = readFileSync(scriptPath, 'utf8');
  const ignore = readFileSync(ignorePath, 'utf8');
  assert.match(script, /--ignorefile \/src\/\.trivyignore\.yaml/);
  assert.match(ignore, /id: AVD-DS-0002/);
  assert.match(ignore, /- packaging\/release-runner\/Dockerfile/);
  assert.match(ignore, /expired_at: 2026-12-21/);
  assert.match(ignore, /host Docker socket/);
  assert.doesNotMatch(ignore, /id:\s*\*/);
});

test('security scan proxy wiring has valid Bash syntax', () => {
  const bash = process.env.KPANEL_TEST_BASH ||
    (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');
  const result = spawnSync(bash, ['-n', 'scripts/security-scan.sh'], {
    cwd: repoRoot,
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
});
