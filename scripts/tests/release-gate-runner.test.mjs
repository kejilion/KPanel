import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const scriptPath = resolve(repoRoot, 'scripts', 'run-release-gate.sh');

test('release gate runner fixes candidate identity and nested Docker wiring', () => {
  const script = readFileSync(scriptPath, 'utf8');
  assert.match(script, /git show-ref --verify --quiet/);
  assert.match(script, /git merge-base --is-ancestor/);
  assert.match(script, /git status --short --untracked-files=all/);
  assert.match(script, /--entrypoint sh/);
  assert.match(script, /-v \/var\/run\/docker\.sock:\/var\/run\/docker\.sock/);
  assert.match(script, /-v "\$repo_root:\$repo_root"/);
  assert.match(script, /-w "\$repo_root"/);
  assert.match(script, /"\$runner_id"/);
  assert.match(script, /make verify-release/);
  assert.match(script, /packaging\/tests\/app-conf-lifecycle\.sh/);
});

test('release gate runner has valid Bash syntax', () => {
  const bash = process.env.KPANEL_TEST_BASH ||
    (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');
  const result = spawnSync(bash, ['-n', 'scripts/run-release-gate.sh'], {
    cwd: repoRoot,
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
});
