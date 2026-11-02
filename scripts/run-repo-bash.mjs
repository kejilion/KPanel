#!/usr/bin/env node

import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { dirname, posix, resolve, win32 } from 'node:path';
import { fileURLToPath } from 'node:url';

const VERIFY_LEVELS = new Set(['auto', '0', 'l0', '1', 'l1', '2', 'l2', '3', 'l3', 'release']);
const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

function detectedGitExecutable(environment = process.env) {
  if (environment.KPANEL_GIT_EXECUTABLE) return environment.KPANEL_GIT_EXECUTABLE;
  if (process.platform !== 'win32') return null;
  try {
    return execFileSync('where.exe', ['git'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      env: environment,
    }).split(/\r?\n/).map((line) => line.trim()).find(Boolean) ?? null;
  } catch {
    return null;
  }
}

export function gitBashCandidates(gitExecutable, environment = process.env) {
  const candidates = [];
  if (environment.KPANEL_GIT_BASH) candidates.push(win32.resolve(environment.KPANEL_GIT_BASH));
  if (gitExecutable) {
    const gitRoot = win32.dirname(win32.dirname(win32.resolve(gitExecutable)));
    candidates.push(win32.join(gitRoot, 'bin', 'bash.exe'));
    candidates.push(win32.join(gitRoot, 'usr', 'bin', 'bash.exe'));
  }
  for (const programFiles of [environment.ProgramFiles, environment['ProgramFiles(x86)']]) {
    if (!programFiles) continue;
    candidates.push(win32.join(programFiles, 'Git', 'bin', 'bash.exe'));
    candidates.push(win32.join(programFiles, 'Git', 'usr', 'bin', 'bash.exe'));
  }
  const seen = new Set();
  return candidates.filter((candidate) => {
    const key = candidate.toLowerCase();
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function resolveBash({
  platform = process.platform,
  environment = process.env,
  gitExecutable = detectedGitExecutable(environment),
  pathExists = existsSync,
} = {}) {
  if (platform !== 'win32') return environment.KPANEL_BASH || 'bash';
  const candidate = gitBashCandidates(gitExecutable, environment).find((path) => pathExists(path));
  if (candidate) return candidate;
  throw new Error('Git for Windows Bash was not found; install Git for Windows or set KPANEL_GIT_BASH');
}

export function cleanShellEnvironment(environment = process.env) {
  const result = { ...environment };
  for (const variable of Object.keys(result)) {
    if (variable.toUpperCase().startsWith('GIT_')) delete result[variable];
  }
  return result;
}

// Use the caller's explicitly selected toolchain, never scan caches or download one.
export function repoShellEnvironment(environment = process.env, {
  platform = process.platform, pathExists = existsSync,
} = {}) {
  const result = cleanShellEnvironment(environment);
  if (!environment.GOROOT) return result;
  const paths = platform === 'win32' ? win32 : posix;
  if (!paths.isAbsolute(environment.GOROOT)) throw new Error('GOROOT must be an absolute toolchain directory');
  const bin = paths.join(environment.GOROOT, 'bin');
  if (!pathExists(paths.join(bin, platform === 'win32' ? 'go.exe' : 'go'))) {
    throw new Error('GOROOT does not contain a Go executable in bin');
  }
  const key = Object.keys(result).find((name) => platform === 'win32' ? name.toUpperCase() === 'PATH' : name === 'PATH');
  const previous = key ? result[key] : '';
  if (key) delete result[key];
  result.PATH = bin + (previous ? (platform === 'win32' ? ';' : ':') + previous : '');
  return result;
}

export function parseRunArguments(argv) {
  const environment = {};
  let index = 0;
  while (argv[index] === '--env') {
    const assignment = argv[index + 1];
    const match = assignment?.match(/^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/s);
    if (!match) throw new Error('--env requires KEY=VALUE');
    if (match[1] !== 'VERIFY_LEVEL' || !VERIFY_LEVELS.has(match[2])) {
      throw new Error('--env only accepts a supported VERIFY_LEVEL');
    }
    if (Object.hasOwn(environment, match[1])) throw new Error('duplicate --env variable: ' + match[1]);
    environment[match[1]] = match[2];
    index += 2;
  }
  if (argv[index] === '--') index += 1;
  const script = argv[index];
  if (!script) throw new Error('usage: node scripts/run-repo-bash.mjs [--env KEY=VALUE] -- SCRIPT [ARG...]');
  return { environment, script, arguments: argv.slice(index + 1) };
}

export function run(argv = process.argv.slice(2)) {
  const parsed = parseRunArguments(argv);
  const scriptPath = resolve(repoRoot, parsed.script);
  if (!existsSync(scriptPath)) throw new Error('Bash script does not exist: ' + parsed.script);
  const bash = resolveBash();
  const result = spawnSync(bash, [scriptPath.replaceAll('\\', '/'), ...parsed.arguments], {
    cwd: repoRoot,
    env: { ...repoShellEnvironment(), ...parsed.environment },
    stdio: 'inherit',
    shell: false,
  });
  if (result.error) throw result.error;
  if (result.status === null) throw new Error('Bash process ended without an exit status');
  return result.status;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = run();
  } catch (error) {
    process.stderr.write('repo bash launcher failed: ' + error.message + '\n');
    process.exitCode = 1;
  }
}
