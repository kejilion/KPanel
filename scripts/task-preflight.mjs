#!/usr/bin/env node

// Read-only qualification of a task's local contract; never execute supplied argv or grant permissions.
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, realpathSync, statSync } from 'node:fs';
import { dirname, isAbsolute, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { cleanShellEnvironment, resolveBash } from './run-repo-bash.mjs';

const SHA = /^[a-f0-9]{40}$/;
const TIMESTAMP = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,3})?(?:Z|[+-]\d{2}:\d{2})$/;
const TOOL_NAMES = new Set(['git', 'node', 'npm', 'go', 'gofmt', 'make', 'docker', 'bash']);
const text = (value) => typeof value === 'string' && value.trim().length > 0 && value.length <= 2048 && !value.includes('\0');
const paths = (value) => Array.isArray(value) && value.length > 0 && value.length <= 128 && value.every((path) => text(path)
  && !path.startsWith('/') && !path.includes('\\') && !path.includes(':') && !/[?*]/.test(path)
  && !path.split('/').some((part) => ['.', '..'].includes(part)));
const inside = (root, path) => { const diff = relative(root, path); return !isAbsolute(diff) && diff !== '..' && !diff.startsWith('..' + (process.platform === 'win32' ? '\\' : '/')); };
const covered = (path, rules) => rules.some((rule) => rule.endsWith('/') ? path.startsWith(rule) : path === rule);

export function qualify({ repo, contract, phase, contractDir = process.cwd(), now = Date.now() }) {
  const failures = [];
  const environment = cleanShellEnvironment();
  const root = realpathSync.native(resolve(repo));
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', env: environment, timeout: 20_000, maxBuffer: 2 * 1024 * 1024 }).trim();
  const require = (condition, message) => { if (!condition) failures.push(message); };
  require(['ready', 'handoff'].includes(phase), 'phase must be ready or handoff');
  require(contract?.schemaVersion === 1, 'schemaVersion must be 1');
  require(text(contract?.scope) && text(contract?.nonGoals), 'scope and nonGoals are required');
  require(SHA.test(contract?.baseCommit ?? ''), 'baseCommit must be an exact 40-character commit');
  require(paths(contract?.allowedPaths), 'allowedPaths must contain literal repository paths or directory prefixes');
  require(Array.isArray(contract?.forbiddenPaths) && (contract.forbiddenPaths.length === 0 || paths(contract.forbiddenPaths)), 'invalid forbiddenPaths');
  require(['L0', 'L1', 'L2', 'L3'].includes(contract?.riskLevel), 'invalid riskLevel');
  const linkage = JSON.parse(readFileSync(resolve(root, 'dependency-policy.json'), 'utf8')).crossRepositoryReleaseLinkage.states;
  require(linkage.includes(contract?.scriptLinkageState), 'invalid scriptLinkageState');
  require(text(contract?.authorization) && ['localCommit', 'push', 'main', 'release', 'production'].every((key) => typeof contract?.permissions?.[key] === 'boolean'), 'explicit permission declarations and authorization source are required');
  require(Array.isArray(contract?.requiredTools) && contract.requiredTools.length <= 8 && contract.requiredTools.every((tool) => TOOL_NAMES.has(tool)), 'invalid requiredTools');
  require(paths(contract?.requiredFiles), 'requiredFiles must contain literal repository files');
  require(Array.isArray(contract?.validations) && contract.validations.length > 0 && contract.validations.length <= 32, 'validations are required');
  const validationIds = new Set();
  for (const validation of Array.isArray(contract?.validations) ? contract.validations : []) {
    require(text(validation?.id) && !validationIds.has(validation.id), 'duplicate or missing validation id');
    validationIds.add(validation?.id);
    require(text(validation?.environment) && text(validation?.tools) && text(validation?.parameters), 'validation must bind environment, tools and parameters');
    require(Array.isArray(validation?.argv) && validation.argv.length > 0 && validation.argv.length <= 40 && validation.argv.every(text), 'validation argv must be a bounded string array');
  }
  // Stop before consuming malformed fields. argv is descriptive, even when it looks like shell code.
  if (failures.length) return { schemaVersion: 1, phase, status: 'failed', failures };
  const gitRoot = realpathSync.native(git('rev-parse', '--show-toplevel'));
  require(process.platform === 'win32' ? gitRoot.toLowerCase() === root.toLowerCase() : gitRoot === root, 'repo must be the worktree root');
  require(git('rev-parse', '--verify', contract.baseCommit + '^{commit}') === contract.baseCommit, 'baseCommit does not resolve exactly');
  const candidate = git('rev-parse', 'HEAD');
  const role = spawnSync(process.execPath, [resolve(root, 'scripts/check-collaboration-state.mjs'), '--repo', root, '--role', 'writer',
    '--base-ref', contract.baseCommit, phase === 'handoff' ? '--require-candidate' : '--require-clean'],
  { env: environment, encoding: 'utf8', timeout: 30_000, maxBuffer: 2 * 1024 * 1024, windowsHide: true });
  require(role.status === 0 && !role.error, 'writer checkpoint failed: ' + (role.stderr || role.error?.message || '').trim());
  for (const tool of contract.requiredTools) {
    if (tool === 'bash') {
      try { const bash = resolveBash(); const result = spawnSync(bash, ['--version'], { env: environment, stdio: 'ignore', timeout: 5000, windowsHide: true }); require(result.status === 0, 'missing tool: bash'); }
      catch { failures.push('missing tool: bash'); }
    } else {
      const result = spawnSync(process.platform === 'win32' ? 'where.exe' : 'which', [tool], { env: environment, stdio: 'ignore', timeout: 5000, windowsHide: true });
      require(result.status === 0, 'missing tool: ' + tool);
    }
  }
  for (const path of contract.requiredFiles) {
    const target = resolve(root, path);
    require(existsSync(target) && statSync(target).isFile() && inside(root, realpathSync(target)), 'missing or out-of-repository input: ' + path);
  }
  if (phase === 'handoff') {
    require(contract.candidateCommit === candidate, 'candidateCommit must equal current HEAD');
    const changed = git('diff', '--name-only', '--no-renames', contract.baseCommit + '...HEAD').split(/\r?\n/).filter(Boolean);
    for (const path of changed) require(covered(path, contract.allowedPaths) && !covered(path, contract.forbiddenPaths), 'out-of-scope change: ' + path);
    const evidence = Array.isArray(contract.evidence) ? contract.evidence : [];
    require(evidence.length === contract.validations.length, 'one evidence receipt per validation is required');
    for (const validation of contract.validations) {
      const matches = evidence.filter((entry) => entry.id === validation.id);
      require(matches.length === 1, 'missing or duplicate evidence: ' + validation.id);
      if (matches.length !== 1) continue;
      const receipt = matches[0];
      require(receipt.candidateCommit === candidate && receipt.status === 'passed' && receipt.exitCode === 0, 'failed or stale candidate evidence: ' + validation.id);
      require(['environment', 'tools', 'parameters'].every((key) => receipt[key] === validation[key]) && JSON.stringify(receipt.argv) === JSON.stringify(validation.argv), 'evidence identity mismatch: ' + validation.id);
      const finished = Date.parse(receipt.finishedAt);
      const expires = Date.parse(receipt.expiresAt);
      require(TIMESTAMP.test(receipt.finishedAt ?? '') && TIMESTAMP.test(receipt.expiresAt ?? '') && Number.isFinite(finished) && finished <= now
        && Number.isFinite(expires) && expires >= now && expires >= finished, 'expired or invalid evidence time: ' + validation.id);
      if (!text(receipt.log) || !/^[a-f0-9]{64}$/.test(receipt.sha256 ?? '')) { failures.push('missing original log/digest: ' + validation.id); continue; }
      const log = resolve(contractDir, receipt.log);
      if (!existsSync(log) || !statSync(log).isFile() || statSync(log).size > 32 * 1024 * 1024) { failures.push('missing or oversized original log: ' + validation.id); continue; }
      require(createHash('sha256').update(readFileSync(log)).digest('hex') === receipt.sha256, 'damaged original evidence: ' + validation.id);
    }
  }
  return { schemaVersion: 1, phase, status: failures.length ? 'failed' : 'passed', candidateCommit: candidate,
    baseCommit: contract.baseCommit, riskLevel: contract.riskLevel, failures,
    qualificationOnly: true, permissionsGranted: false };
}

export function main(argv) {
  if (argv.includes('--help')) { process.stdout.write('usage: task-preflight.mjs ready|handoff --contract JSON [--repo PATH]\n'); return 0; }
  const phase = argv.shift();
  const options = { repo: process.cwd() };
  const seen = new Set();
  for (let i = 0; i < argv.length; i += 2) {
    const key = new Map([['--repo', 'repo'], ['--contract', 'contract']]).get(argv[i]);
    if (!key || seen.has(key) || !argv[i + 1]) throw new Error('invalid option: ' + argv[i]);
    seen.add(key); options[key] = argv[i + 1];
  }
  if (!options.contract) throw new Error('--contract is required');
  const contractPath = resolve(options.contract);
  if (statSync(contractPath).size > 64 * 1024) throw new Error('contract exceeds 64 KiB');
  const report = qualify({ repo: options.repo, contract: JSON.parse(readFileSync(contractPath, 'utf8')), phase, contractDir: dirname(contractPath) });
  process.stdout.write(JSON.stringify(report, null, 2) + '\n');
  return report.status === 'passed' ? 0 : 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.exitCode = main(process.argv.slice(2)); }
  catch (error) { process.stdout.write(JSON.stringify({ schemaVersion: 1, status: 'failed', failures: [error.message] }) + '\n'); process.exitCode = 1; }
}
