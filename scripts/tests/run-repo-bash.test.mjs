import test from 'node:test';
import assert from 'node:assert/strict';
import { cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

import {
  cleanShellEnvironment,
  gitBashCandidates,
  parseRunArguments,
  resolveBash,
  repoShellEnvironment,
} from '../run-repo-bash.mjs';

test('Windows resolves Git for Windows Bash instead of an ambiguous WSL bash', () => {
  const gitExecutable = 'C:\\Program Files\\Git\\cmd\\git.exe';
  const candidates = gitBashCandidates(gitExecutable, { ProgramFiles: 'C:\\Program Files' });
  assert.equal(candidates[0], 'C:\\Program Files\\Git\\bin\\bash.exe');
  assert.equal(resolveBash({
    platform: 'win32',
    environment: { ProgramFiles: 'C:\\Program Files' },
    gitExecutable,
    pathExists: (path) => path === 'C:\\Program Files\\Git\\bin\\bash.exe',
  }), 'C:\\Program Files\\Git\\bin\\bash.exe');
});

test('explicit Go selection reaches native and Bash children without cache discovery or global mutation', () => {
  const original = { GOROOT: 'C:\\selected Go', Path: 'C:\\tools', GIT_DIR: 'foreign', KEEP: 'yes' };
  const environment = repoShellEnvironment(original, { platform: 'win32', pathExists: path => path === 'C:\\selected Go\\bin\\go.exe' });
  assert.equal(environment.PATH, 'C:\\selected Go\\bin;C:\\tools');
  assert.equal(environment.Path, undefined);
  assert.equal(environment.GIT_DIR, undefined);
  assert.equal(environment.KEEP, 'yes');
  assert.equal(original.Path, 'C:\\tools');
  assert.equal(repoShellEnvironment({ GOROOT: '/selected/go', PATH: '/usr/bin' }, {
    platform: 'linux', pathExists: path => path === '/selected/go/bin/go',
  }).PATH, '/selected/go/bin:/usr/bin');
  assert.deepEqual(repoShellEnvironment({ PATH: '/usr/bin' }, { platform: 'linux' }), { PATH: '/usr/bin' });
  assert.throws(() => repoShellEnvironment({ GOROOT: 'relative' }), /absolute/);
  assert.throws(() => repoShellEnvironment({ GOROOT: '/absent' }, { platform: 'linux', pathExists: () => false }), /Go executable/);
});

test('launcher runs from its owning repository even when invoked in web and preserves arguments and failure exit', () => {
  const root = mkdtempSync(join(tmpdir(), 'kpanel launcher '));
  try {
    mkdirSync(join(root, 'scripts')); mkdirSync(join(root, 'web'));
    cpSync(new URL('../run-repo-bash.mjs', import.meta.url), join(root, 'scripts/run-repo-bash.mjs'));
    writeFileSync(join(root, 'scripts/fixture.sh'), 'printf "%s\\n" "$PWD" "$1"\nexit 19\n');
    const argument = 'literal $() `text` with spaces';
    const result = spawnSync(process.execPath, [join(root, 'scripts/run-repo-bash.mjs'), 'scripts/fixture.sh', argument], {
      cwd: join(root, 'web'), env: process.env, encoding: 'utf8', timeout: 10000,
    });
    assert.equal(result.status, 19, result.stderr);
    assert.equal(result.stdout.trim().split(/\r?\n/).at(-1), argument);
    assert.doesNotMatch(result.stderr, /does not exist/);
    assert.notEqual(result.stdout.trim().split(/\r?\n/)[0].split('/').at(-1), 'web');
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('non-Windows keeps the platform Bash and Windows fails closed when Git Bash is absent', () => {
  assert.equal(resolveBash({ platform: 'linux', environment: {} }), 'bash');
  assert.throws(() => resolveBash({
    platform: 'win32',
    environment: {},
    gitExecutable: null,
    pathExists: () => false,
  }), /Git for Windows Bash was not found/);
});

test('launcher arguments keep verification environment explicit', () => {
  assert.deepEqual(parseRunArguments([
    '--env', 'VERIFY_LEVEL=l2', '--', 'scripts/verify-change.sh', 'origin/main',
  ]), {
    environment: { VERIFY_LEVEL: 'l2' },
    script: 'scripts/verify-change.sh',
    arguments: ['origin/main'],
  });
  assert.throws(() => parseRunArguments(['--env', 'VERIFY_LEVEL']), /KEY=VALUE/);
  assert.throws(() => parseRunArguments(['--env', 'GIT_DIR=foreign', 'script.sh']), /only accepts/);
  assert.throws(() => parseRunArguments(['--env', 'VERIFY_LEVEL=unknown', 'script.sh']), /only accepts/);
  assert.throws(() => parseRunArguments([
    '--env', 'VERIFY_LEVEL=l2', '--env', 'VERIFY_LEVEL=l3', 'script.sh',
  ]), /duplicate/);
});

test('launcher removes caller Git repository overrides without dropping unrelated variables', () => {
  const environment = cleanShellEnvironment({
    GIT_DIR: 'foreign',
    GIT_WORK_TREE: 'foreign',
    GIT_INDEX_FILE: 'foreign',
    GIT_COMMON_DIR: 'foreign',
    GIT_OBJECT_DIRECTORY: 'foreign',
    GIT_ALTERNATE_OBJECT_DIRECTORIES: 'foreign',
    PATH: 'kept',
  });
  assert.equal(environment.PATH, 'kept');
  for (const key of Object.keys(environment)) assert.equal(key.startsWith('GIT_'), false);
});
