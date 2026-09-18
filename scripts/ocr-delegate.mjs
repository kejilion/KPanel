#!/usr/bin/env node

// Single KPanel entry for open-code-review (PROJECT_RULES.md 5.5). It installs the version pinned in
// dependency-policy.json outside the repository, disables OCR self-update, and only forwards the
// LLM-free delegation subcommands so source never leaves for an OCR-configured model endpoint.

import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { npmInvocation } from './report-dependency-freshness.mjs';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const PACKAGE = '@alibaba-group/open-code-review';
const ALLOWED = new Map([
  ['preview', { mapped: ['delegate', 'preview'], flags: ['--from', '--to', '-c', '--commit', '-f', '--format'], paths: false }],
  ['rule', { mapped: ['delegate', 'rule'], flags: ['-f', '--format'], paths: true }],
  ['rules-check', { mapped: ['rules', 'check'], flags: [], paths: true }],
]);

export function pinnedComponent(policy) {
  const group = policy.groups?.find((item) => item.id === 'code-review-assistant');
  const component = group?.components?.[PACKAGE];
  if (!component || !/^\d+\.\d+\.\d+$/.test(component.pinnedVersion ?? '')) {
    throw new Error('dependency-policy.json code-review-assistant must pin ' + PACKAGE + ' to X.Y.Z');
  }
  return component;
}

export function ocrArguments(argv) {
  const [command, ...rest] = argv;
  const spec = ALLOWED.get(command);
  if (!spec) {
    throw new Error('unsupported command "' + (command ?? '') + '"; allowed: ' + [...ALLOWED.keys()].join(', ')
      + ' (review/scan/config are forbidden: they reach an OCR-managed LLM endpoint)');
  }
  // Whitelist flags: --rule/--repo/--exclude would replace the tracked .opencodereview/rule.json selection policy.
  for (let index = 0; index < rest.length; index += 1) {
    const value = rest[index];
    if (!value.startsWith('-')) {
      if (!spec.paths) throw new Error('unexpected argument "' + value + '" for ' + command);
      continue;
    }
    const flag = value.split('=')[0];
    if (!spec.flags.includes(flag)) {
      throw new Error(flag + ' is forbidden for ' + command + '; allowed: ' + (spec.flags.join(', ') || 'none')
        + ' (the tracked .opencodereview/rule.json is the only selection policy)');
    }
    if (!value.includes('=')) index += 1;
  }
  return [...spec.mapped, ...rest];
}

// Isolate OCR from user-level ~/.opencodereview (global rules override rule text per machine) and telemetry env.
export function ocrEnvironment(environment, home) {
  const env = Object.fromEntries(Object.entries(environment).filter(([key]) => !/^(OCR_|OTEL_)/i.test(key)));
  return { ...env, OCR_NO_UPDATE: '1', HOME: home, USERPROFILE: home };
}

export function parseVersion(output) {
  return String(output).match(/open-code-review v(\d+\.\d+\.\d+)/)?.[1] ?? null;
}

export function toolDirectory(version, environment = process.env) {
  return resolve(environment.KPANEL_OCR_TOOL_DIR || join(homedir(), '.cache', 'kpanel-tools', 'ocr-' + version));
}

function launcher(directory) {
  return join(directory, 'node_modules', '@alibaba-group', 'open-code-review', 'bin', 'ocr.js');
}

// Run the npm launcher through node without a shell: cmd.exe would eat the caret in refs like `abc^`.
function runOcr(directory, arguments_, env, stdio) {
  return spawnSync(process.execPath, [launcher(directory), ...arguments_], { env, stdio, encoding: 'utf8' });
}

function installedVersion(directory, env) {
  if (!existsSync(launcher(directory))) return null;
  const result = runOcr(directory, ['--version'], env, 'pipe');
  return result.status === 0 ? parseVersion(result.stdout) : null;
}

export function main(argv, environment = process.env) {
  const policy = JSON.parse(readFileSync(resolve(repoRoot, 'dependency-policy.json'), 'utf8'));
  const { pinnedVersion } = pinnedComponent(policy);
  const forwarded = ocrArguments(argv);
  const directory = toolDirectory(pinnedVersion, environment);
  const home = join(directory, 'isolated-home');
  mkdirSync(home, { recursive: true });
  const env = ocrEnvironment(environment, home);
  if (installedVersion(directory, env) !== pinnedVersion) {
    mkdirSync(directory, { recursive: true });
    const npm = npmInvocation(process.platform, environment);
    execFileSync(npm.command, [...npm.prefixArguments, 'install', '--prefix', directory, '--no-audit', '--no-fund',
      PACKAGE + '@' + pinnedVersion], { env: { ...environment, OCR_NO_UPDATE: '1' }, stdio: ['ignore', 'ignore', 'inherit'] });
    const actual = installedVersion(directory, env);
    if (actual !== pinnedVersion) throw new Error('installed OCR ' + actual + ' does not match pin ' + pinnedVersion);
  }
  // OCR reads .opencodereview/rule.json from --repo; a subdirectory would silently fall back to upstream selection.
  const worktree = execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim();
  const result = runOcr(directory, [...forwarded, '--repo', worktree], env, 'inherit');
  return result.status ?? 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = main(process.argv.slice(2));
  } catch (error) {
    process.stderr.write('ocr-delegate: ' + error.message + '\n');
    process.exitCode = 2;
  }
}
