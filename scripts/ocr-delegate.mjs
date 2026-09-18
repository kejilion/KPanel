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
  ['preview', ['delegate', 'preview']],
  ['rule', ['delegate', 'rule']],
  ['rules-check', ['rules', 'check']],
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
  const mapped = ALLOWED.get(command);
  if (!mapped) {
    throw new Error('unsupported command "' + (command ?? '') + '"; allowed: ' + [...ALLOWED.keys()].join(', ')
      + ' (review/scan/config are forbidden: they reach an OCR-managed LLM endpoint)');
  }
  if (rest.some((value) => value === '--rule' || value.startsWith('--rule=') || value === '--repo' || value.startsWith('--repo='))) {
    throw new Error('--rule/--repo are forbidden; the tracked .opencodereview/rule.json of the current worktree is the only selection policy');
  }
  return [...mapped, ...rest];
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
  const env = { ...environment, OCR_NO_UPDATE: '1' };
  const directory = toolDirectory(pinnedVersion, environment);
  if (installedVersion(directory, env) !== pinnedVersion) {
    mkdirSync(directory, { recursive: true });
    const npm = npmInvocation(process.platform, environment);
    execFileSync(npm.command, [...npm.prefixArguments, 'install', '--prefix', directory, '--no-audit', '--no-fund',
      PACKAGE + '@' + pinnedVersion], { env, stdio: ['ignore', 'ignore', 'inherit'] });
    const actual = installedVersion(directory, env);
    if (actual !== pinnedVersion) throw new Error('installed OCR ' + actual + ' does not match pin ' + pinnedVersion);
  }
  const result = runOcr(directory, [...forwarded, '--repo', process.cwd()], env, 'inherit');
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
