#!/usr/bin/env node

import { existsSync, realpathSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join, resolve } from 'node:path';

import { addedBoundaryPackages, POLICY_PATH } from './check-security-audit-coverage.mjs';

function usage() {
  return [
    'Usage: node scripts/check-collaboration-state.mjs [options]',
    '  --repo <path>       Repository worktree (default: current directory)',
    '  --role <role>       management, writer, or auto',
    '  --base-ref <ref>    Approved baseline (default: origin/main)',
    '  --require-clean     Require a clean writer checkpoint',
    '  --require-candidate Require a clean, non-empty writer commit above an explicit base',
  ].join('\n');
}

function parseArgs(argv) {
  const options = {
    repo: process.cwd(),
    role: '',
    baseRef: 'origin/main',
    baseRefExplicit: false,
    requireClean: false,
    requireCandidate: false,
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
    if (argument === '--require-candidate') {
      if (seen.has(argument)) throw new Error('duplicate option: ' + argument);
      seen.add(argument);
      options.requireCandidate = true;
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
    if (argument === '--base-ref') {
      options.baseRef = value;
      options.baseRefExplicit = true;
    }
  }

  if (!['management', 'writer', 'auto'].includes(options.role)) {
    throw new Error('--role must be management, writer, or auto');
  }
  if (options.role === 'management' && options.requireClean) {
    throw new Error('--require-clean is implicit for the management role');
  }
  if (options.requireCandidate && options.role !== 'writer') {
    throw new Error('--require-candidate requires --role writer');
  }
  if (options.requireCandidate && !options.baseRefExplicit) {
    throw new Error('--require-candidate requires an explicit --base-ref');
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

const OCR_CODE_PATH = /\.(go|ts|tsx|vue|js|mjs|cjs|sh)$/;
const OCR_AUTO_MIN_CODE_LINES = 30;

function codeChanges(root, range) {
  let paths = 0;
  let lines = 0;
  for (const row of git(root, ['diff', '--numstat', '--no-renames', range]).split(/\r?\n/).filter(Boolean)) {
    const [added, deleted, path] = row.split('\t');
    if (!OCR_CODE_PATH.test(path)) continue;
    paths += 1;
    lines += (Number(added) || 0) + (Number(deleted) || 0);
  }
  return { paths, lines };
}

// PROJECT_RULES.md 5.5 advisory: small code candidates are exempt, and a trailer followed by more code is stale.
function ocrLineReviewState(root, baseRef) {
  const changes = codeChanges(root, baseRef + '...HEAD');
  if (changes.lines < OCR_AUTO_MIN_CODE_LINES) return null;
  const summary = ' code_paths=' + changes.paths + ' code_lines=' + changes.lines;
  const newest = git(root, ['log', '--format=%H%x1f%(trailers:key=OCR-Review,valueonly)%x1e', baseRef + '..HEAD'])
    .split('\x1e').map((record) => record.trim().split('\x1f')).find(([, trailer]) => trailer?.trim());
  if (!newest) return 'missing' + summary;
  return (codeChanges(root, newest[0] + '..HEAD').paths > 0 ? 'stale' : 'recorded') + summary;
}

// PROJECT_RULES.md 5.4 advisory: a candidate that adds a trust-boundary package should be audited while its
// scope is one feature, before an RC ships it. The stable preflight still enforces coverage either way.
function securityAuditState(root, baseRef) {
  if (!existsSync(join(root, POLICY_PATH))) return null;
  try {
    const added = addedBoundaryPackages(root, baseRef, 'HEAD');
    if (added.length === 0) return null;
    const summary = ' new_boundary_packages=' + added.join(',');
    const newest = git(root, ['log', '--format=%H%x1f%(trailers:key=Security-Audit,valueonly)%x1e', baseRef + '..HEAD'])
      .split('\x1e').map((record) => record.trim().split('\x1f')).find(([, trailer]) => trailer?.trim());
    if (!newest) return 'missing' + summary;
    return (addedBoundaryPackages(root, newest[0], 'HEAD').length > 0 ? 'stale' : 'recorded') + summary;
  } catch (error) {
    return 'unavailable reason=' + JSON.stringify(error.message.split('\n')[0]);
  }
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
  let ocrLineReview = null;
  let securityAudit = null;
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
  const isPrimaryWorktree = Boolean(
    primaryWorktree && normalizedPath(root) === normalizedPath(primaryWorktree),
  );
  const effectiveRole = options.role === 'auto'
    ? (isPrimaryWorktree ? (worktrees.length > 1 ? 'management' : 'standalone') : 'writer')
    : options.role;

  let ahead = 0;
  let behind = 0;
  const divergence = git(root, ['rev-list', '--left-right', '--count', 'HEAD...' + options.baseRef])
    .split(/\s+/)
    .map(Number);
  [ahead, behind] = divergence;

  if (effectiveRole === 'management') {
    if (!isPrimaryWorktree) {
      failures.push('management role must run from the primary worktree');
    }
    if (branch !== 'main') failures.push('management worktree must stay on main; found ' + branch);
    if (dirtyPaths.length > 0) {
      failures.push('management worktree must be clean; found ' + dirtyPaths.length + ' changed path(s)');
    }
    if (ahead > 0) {
      failures.push('management main contains ' + ahead + ' local commit(s) absent from ' + options.baseRef);
    }
  } else if (effectiveRole === 'writer') {
    if (isPrimaryWorktree) {
      failures.push('writer role must use a linked task worktree, not the primary management worktree');
    }
    if (branch === '(detached)') failures.push('writer role must use an attached task branch, not detached HEAD');
    if (branch === 'main') failures.push('writer role must not use the main branch');
    if (options.requireClean && dirtyPaths.length > 0) {
      failures.push('writer checkpoint must be clean; found ' + dirtyPaths.length + ' changed path(s)');
    }
    if (options.role !== 'auto' || options.baseRefExplicit) {
      try {
        git(root, ['merge-base', '--is-ancestor', baseCommit, headCommit]);
      } catch {
        failures.push(options.baseRef + ' is not an ancestor of the writer HEAD');
      }
    }
    if (options.requireCandidate) {
      if (headCommit === baseCommit || ahead === 0) {
        failures.push('writer completion requires at least one candidate commit above ' + options.baseRef);
      } else {
        const changedPaths = git(root, ['diff', '--name-only', options.baseRef + '...HEAD'])
          .split(/\r?\n/)
          .filter(Boolean);
        if (changedPaths.length === 0) {
          failures.push('writer candidate must contain a non-empty task diff; empty commits do not satisfy completion');
        }
        // PROJECT_RULES.md 5.5 and 5.4: advisory only, never a failure.
        ocrLineReview = ocrLineReviewState(root, options.baseRef);
        securityAudit = securityAuditState(root, options.baseRef);
      }
    }
  } else {
    if (options.requireClean && dirtyPaths.length > 0) {
      failures.push('standalone checkpoint must be clean; found ' + dirtyPaths.length + ' changed path(s)');
    }
    if (options.baseRefExplicit) {
      try {
        git(root, ['merge-base', '--is-ancestor', baseCommit, headCommit]);
      } catch {
        failures.push(options.baseRef + ' is not an ancestor of the standalone HEAD');
      }
    }
  }

  const summary = [
    'role=' + effectiveRole,
    ...(options.role === 'auto' ? ['requested_role=auto'] : []),
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
  if (ocrLineReview) {
    const hint = !ocrLineReview.startsWith('recorded')
      ? ' advisory: run .codex-workflows/ocr-line-review.workflow.yaml (profile=candidate) and add an OCR-Review trailer,'
        + ' or record "OCR-Review: skipped reason=<why>"'
      : '';
    process.stdout.write('ocr_line_review=' + ocrLineReview + hint + '\n');
  }
  if (securityAudit) {
    const hint = securityAudit.startsWith('missing') || securityAudit.startsWith('stale')
      ? ' advisory: run .codex-workflows/security-boundary-audit.workflow.yaml (profile=scoped) on this candidate and add'
        + ' "Security-Audit: scoped run-<N>", or record "Security-Audit: deferred reason=<why>"'
      : '';
    process.stdout.write('security_audit=' + securityAudit + hint + '\n');
  }
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
