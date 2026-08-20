import assert from 'node:assert/strict';
import {
  chmodSync,
  cpSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

const repoRoot = resolve(import.meta.dirname, '..', '..');

test('change routing includes deletions and every GitHub workflow', () => {
  const script = readFileSync(join(repoRoot, 'scripts', 'verify-change.sh'), 'utf8');
  const filters = [...script.matchAll(/--diff-filter=([A-Z]+)/g)].map((match) => match[1]);
  assert.ok(filters.length >= 3);
  assert.ok(filters.every((filter) => filter.includes('D')));
  assert.match(script, /Release acceptance records are immutable; deletion detected/);
  assert.match(script, /\.github\/workflows\/\*\.yml\|\.github\/workflows\/\*\.yaml/);
  assert.match(script, /scripts\/check-collaboration-state\.mjs/);
  assert.match(script, /scripts\/tests\/collaboration-state\.test\.mjs/);
});

function executable(path, body) {
  writeFileSync(path, `#!/usr/bin/env bash\nset -eu\n${body}\n`, 'utf8');
  chmodSync(path, 0o755);
}

function makeRemovable(path) {
  for (const entry of readdirSync(path)) {
    const child = join(path, entry);
    if (statSync(child).isDirectory()) makeRemovable(child);
    chmodSync(child, 0o777);
  }
  chmodSync(path, 0o777);
}

test('forced release verification runs on a clean checkout', () => {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'kpanel-verify-change-'));
  try {
    const scripts = join(root, 'scripts');
    const tests = join(scripts, 'tests');
    mkdirSync(tests, { recursive: true });
    mkdirSync(join(root, 'web', 'node_modules'), { recursive: true });
    cpSync(join(repoRoot, 'scripts', 'verify-change.sh'), join(scripts, 'verify-change.sh'));

    for (const name of [
      'check-ecosystem-policy.sh',
      'check-version-consistency.sh',
      'check-managed-script-contract.sh',
      'verify-governance.sh',
      'security-scan.sh',
    ]) {
      executable(join(scripts, name), 'exit 0');
    }

    const bashEnv = join(root, 'bash-env.sh');
    writeFileSync(bashEnv, String.raw`
git() {
  case "$*" in
    "rev-parse --verify HEAD") return 0 ;;
    "cat-file -e"*) return 1 ;;
    "diff"*|"ls-files"*) return 0 ;;
    *) return 0 ;;
  esac
}
node() { return 0; }
npm() { printf 'npm %s\n' "$*" >>"$VERIFY_STUB_LOG"; }
make() { printf 'make %s\n' "$*" >>"$VERIFY_STUB_LOG"; }
go() { printf 'go %s\n' "$*" >>"$VERIFY_STUB_LOG"; }
docker() { printf 'docker %s\n' "$*" >>"$VERIFY_STUB_LOG"; }
export -f git node npm make go docker
`, 'utf8');
    const bashEnvPath = bashEnv
      .replace(/^([A-Za-z]):/, (_, drive) => `/${drive.toLowerCase()}`)
      .replaceAll('\\', '/');

    const log = join(root, 'commands.log');
    const bashCommand = process.env.KPANEL_TEST_BASH ||
      (process.platform === 'win32' ? 'C:\\Program Files\\Git\\bin\\bash.exe' : 'bash');
    const result = spawnSync(bashCommand, ['scripts/verify-change.sh'], {
      cwd: root,
      encoding: 'utf8',
      env: {
        ...process.env,
        BASH_ENV: bashEnvPath,
        VERIFY_LEVEL: 'release',
        VERIFY_STUB_LOG: log,
      },
    });

    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
    assert.match(result.stdout, /L3 release verification completed\./);
    assert.doesNotMatch(result.stdout, /No changes require verification\./);
    const commands = readFileSync(log, 'utf8');
    assert.match(commands, /make test/);
    assert.match(commands, /go test -race \.\/internal\/panel/);
    assert.match(commands, /docker build /);
  } finally {
    makeRemovable(root);
    rmSync(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  }
});
