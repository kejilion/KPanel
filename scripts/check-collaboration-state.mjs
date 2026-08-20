#!/usr/bin/env node

import { realpathSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';

function usage() {
  return [
    'Usage: node scripts/check-collaboration-state.mjs [options]',
    '  --repo <path>       Repository worktree (default: current directory)',
    '  --role <role>       management or writer',
    '  --base-ref <ref>    Approved baseline (default: origin/main)',
    '  --require-clean     Require a clean writer checkpoint',
  ].join('\n');
}

function parseArgs(argv) {
  const options = {
    repo: process.cwd(),
    role: '',
    baseRef: 'origin/main',
    requireClean: false,
  };
  const seen = new Set();

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === '--help' || argument === '-h') return { help: true };
    if (argument === '--require-clean') {
      if (seen.has(argument)) throw new Error('duplicate option: ' + argument);
      seen.add(argument);
      options.requireClean = true;
      continue;
    }
    if (!['--repo', '--role', '--base-ref'].includes(argument)) {
      throw new Error('unknown option: ' + argument);
    }
    if (seen.has(argument)) throw new Error('duplicate option: ' + argument);
    seen.add(argument);
    const value = argv[index + 1];
    if (!value || value.startsWith('--')) throw new Error('missing value for ' + argument);
    index += 1;
    if (argument === '--repo') options.repo = value;
    if (argument === '--role') options.role = value;
    if (argument === '--base-ref') options.baseRef = value;
  }

  if (!['management', 'writer'].includes(options.role)) {
    throw new Error('--role must be management or writer');
  }
  if (options.role === 'management' && options.requireClean) {
    throw new Error('--require-clean is implicit for the management role');
  }
  return options;
}

function gitEnvironment() {
  const environment = { ...process.env };
  for (const key of Object.keys(environment)) {
    if (key.startsWith('GIT_')) delete environment[key];
  }
  return environment;
}

function git(repo, args) {
  return execFileSync('git', ['-C', repo, ...args], {
    encoding: 'utf8',
    env: gitEnvironment(),
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}

function normalizedPath(path) {
  const normalized = realpathSync.native(resolve(path));
  return process.platform === 'win32' ? normalized.toLowerCase() : normalized;
}

function parseWorktrees(output) {
  return output.split(/\r?\n\r?\n/).flatMap((block) => {
    const worktree = {};
    for (const line of block.split(/\r?\n/)) {
      const separator = line.indexOf(' ');
      if (separator === -1) continue;
      worktree[line.slice(0, separator)] = line.slice(separator + 1);
    }
    return worktree.worktree ? [worktree] : [];
  });
}

function check(options) {
  const failures = [];
  const repo = realpathSync.native(resolve(options.repo));
  const root = realpathSync.native(git(repo, ['rev-parse', '--show-toplevel']));
  let branch = '(detached)';
  try {
    branch = git(root, ['symbolic-ref', '--quiet', '--short', 'HEAD']);
  } catch {
    // A writer must use an attached task branch; report this with the other role failures below.
  }
  const baseCommit = git(root, ['rev-parse', '--verify', options.baseRef + '^{commit}']);
  const headCommit = git(root, ['rev-parse', 'HEAD']);
  const dirtyPaths = git(root, ['status', '--porcelain=v1', '--untracked-files=all'])
    .split(/\r?\n/)
    .filter(Boolean);
  const worktrees = parseWorktrees(git(root, ['worktree', 'list', '--porcelain']));
  const primaryWorktree = worktrees[0]?.worktree;

  let ahead = 0;
  let behind = 0;
  const divergence = git(root, ['rev-list', '--left-right', '--count', 'HEAD...' + options.baseRef])
    .split(/\s+/)
    .map(Number);
  [ahead, behind] = divergence;

  if (options.role === 'management') {
    if (!primaryWorktree || normalizedPath(root) !== normalizedPath(primaryWorktree)) {
      failures.push('management role must run from the primary worktree');
    }
    if (branch !== 'main') failures.push('management worktree must stay on main; found ' + branch);
    if (dirtyPaths.length > 0) {
      failures.push('management worktree must be clean; found ' + dirtyPaths.length + ' changed path(s)');
    }
    if (ahead > 0) {
      failures.push('management main contains ' + ahead + ' local commit(s) absent from ' + options.baseRef);
    }
  } else {
    if (primaryWorktree && normalizedPath(root) === normalizedPath(primaryWorktree)) {
      failures.push('writer role must use a linked task worktree, not the primary management worktree');
    }
    if (branch === '(detached)') failures.push('writer role must use an attached task branch, not detached HEAD');
    if (branch === 'main') failures.push('writer role must not use the main branch');
    if (options.requireClean && dirtyPaths.length > 0) {
      failures.push('writer checkpoint must be clean; found ' + dirtyPaths.length + ' changed path(s)');
    }
    try {
      git(root, ['merge-base', '--is-ancestor', baseCommit, headCommit]);
    } catch {
      failures.push(options.baseRef + ' is not an ancestor of the writer HEAD');
    }
  }

  const summary = [
    'role=' + options.role,
    'branch=' + branch,
    'clean=' + String(dirtyPaths.length === 0),
    'ahead=' + ahead,
    'behind=' + behind,
    'head=' + headCommit,
    'base=' + baseCommit,
  ].join(' ');

  if (failures.length > 0) {
    process.stderr.write('Collaboration state check failed (' + summary + '):\n');
    for (const failure of failures) process.stderr.write('- ' + failure + '\n');
    process.exitCode = 1;
    return;
  }
  process.stdout.write('collaboration_state=pass ' + summary + '\n');
}

try {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    process.stdout.write(usage() + '\n');
  } else {
    check(options);
  }
} catch (error) {
  process.stderr.write('Collaboration state check failed: ' + error.message + '\n');
  process.stderr.write(usage() + '\n');
  process.exitCode = 1;
}
