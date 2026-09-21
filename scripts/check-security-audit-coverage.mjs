#!/usr/bin/env node

// Trust-boundary audit trigger (PROJECT_RULES.md 5.4). For one target commit it answers: which
// trust-boundary changes are not yet covered by a completed audit run, and is a scoped or full run due?
// Coverage is the union of completed runs (see classifyRuns); an interrupted or partial run never counts.
// Boundary scope is default-deny: every package under the policy's
// package roots is boundary unless named non-boundary with a reason, so a new package cannot silently
// fall outside the trigger. Ages are measured from commit dates, never the wall clock, so a result is
// reproducible for the same commits.

import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const POLICY_PATH = '.governance/security-audit/boundary-policy.json';
export const RUNS_DIR = '.governance/security-audit';
// run-1..run-3 predate the metadata contract; they are read as recorded, never rewritten.
export const LEGACY_RUN_LIMIT = 3;
const SHA = /\b[0-9a-f]{40}\b/;
const EXACT_SHA = /^[0-9a-f]{40}$/;
const DAY_SECONDS = 86400;

// Caller GIT_* overrides (GIT_DIR, GIT_WORK_TREE, ...) must not redirect which history is judged.
function gitRunner(repo) {
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('GIT_')));
  return (...args) => execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8', env }).trim();
}

function isAncestor(git, ancestor, descendant) {
  try {
    git('merge-base', '--is-ancestor', ancestor, descendant);
    return true;
  } catch {
    return false;
  }
}

export function validatePolicy(policy, repo = repoRoot) {
  const failures = [];
  if (policy?.schemaVersion !== 1) failures.push('policy: schemaVersion must be 1');
  for (const key of ['scopedMaxAgeDays', 'fullMaxAgeDays']) {
    if (!Number.isInteger(policy?.[key]) || policy[key] <= 0) failures.push('policy: ' + key + ' must be a positive integer');
  }
  for (const key of ['boundaryRoots', 'packageRoots', 'ignoreSuffixes', 'ignoreSegments']) {
    if (!Array.isArray(policy?.[key])) failures.push('policy: ' + key + ' must be an array');
  }
  for (const root of policy?.boundaryRoots ?? []) {
    if (!existsSync(join(repo, root))) failures.push('policy: boundary root ' + root + ' does not exist');
  }
  for (const root of policy?.packageRoots ?? []) {
    if (!(policy.boundaryRoots ?? []).includes(root)) failures.push('policy: package root ' + root + ' must also be a boundary root');
  }
  for (const [path, reason] of Object.entries(policy?.nonBoundary ?? {})) {
    if (!(policy.packageRoots ?? []).some((root) => path.startsWith(root + '/'))) failures.push('policy: non-boundary ' + path + ' is not under a package root');
    if (!existsSync(join(repo, path))) failures.push('policy: non-boundary ' + path + ' no longer exists; remove it');
    if (typeof reason !== 'string' || reason.trim().length < 8) failures.push('policy: non-boundary ' + path + ' needs a concrete reason');
  }
  return failures;
}

// Accepts the three historical metadata shapes; new runs must use scope_mode and exact identities.
export function normalizeRun(name, meta) {
  const number = Number(/^run-(\d+)$/.exec(name)?.[1] ?? NaN);
  const mode = meta.scope_mode ?? meta.project_mode ?? (meta.scope_paths == null ? 'full' : 'scoped');
  const status = String(meta.run_status ?? '');
  return {
    name,
    number,
    mode,
    complete: status === 'complete' || status === 'completed',
    status,
    source: SHA.exec(String(meta.source_ref ?? ''))?.[0] ?? null,
    base: SHA.exec(String(meta.comparison_base ?? ''))?.[0] ?? null,
    // Legacy scoped runs recorded their range in free text; run-4+ must declare it explicitly.
    scopeComplete: meta.scope_complete ?? number <= LEGACY_RUN_LIMIT,
    dirty: meta.source_dirty === true || /dirty/i.test(String(meta.source_ref ?? '')),
    meta,
  };
}

export function validateRun(run) {
  const failures = [];
  if (!Number.isInteger(run.number)) return [run.name + ': directory must be named run-<N>'];
  if (!run.source) failures.push(run.name + ': source_ref has no commit');
  if (run.number <= LEGACY_RUN_LIMIT) return failures;
  const meta = run.meta;
  // `project_mode` is the skill's own key; runs begun before 5.4 named scope_mode keep it as a synonym.
  const mode = meta.scope_mode ?? meta.project_mode;
  if (!['full', 'scoped'].includes(mode)) failures.push(run.name + ': scope_mode (or project_mode) must be full or scoped');
  if (meta.scope_mode !== undefined && meta.project_mode !== undefined && meta.scope_mode !== meta.project_mode) {
    failures.push(run.name + ': scope_mode and project_mode disagree');
  }
  if (!EXACT_SHA.test(String(meta.source_ref ?? ''))) failures.push(run.name + ': source_ref must be an exact 40-hex commit');
  if (meta.source_dirty !== false) failures.push(run.name + ': source_dirty must be false');
  if (!run.status) failures.push(run.name + ': run_status is required (complete, or the interruption reason)');
  if (mode === 'scoped' && !EXACT_SHA.test(String(meta.comparison_base ?? ''))) {
    failures.push(run.name + ': scoped runs must record comparison_base as an exact 40-hex commit');
  }
  if (mode === 'scoped' && typeof meta.scope_complete !== 'boolean') {
    failures.push(run.name + ': scoped runs must record scope_complete (true only when every listed change was audited)');
  }
  return failures;
}

export function loadRuns(repo = repoRoot) {
  const directory = join(repo, RUNS_DIR);
  return readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && entry.name.startsWith('run-'))
    .map((entry) => normalizeRun(entry.name, JSON.parse(readFileSync(join(directory, entry.name, 'run-metadata.json'), 'utf8'))))
    .sort((left, right) => left.number - right.number);
}

// Coverage is the union of completed runs in the target history, not a chain: full runs usually audit a
// release candidate while scoped runs audit feature branches beside it, so neither need descend from the
// other. A completed full run covers every commit it contains; a completed scoped run that declares
// scope_complete covers exactly comparison_base..source_ref. Interrupted, partial and out-of-history runs
// are reported and never count.
export function classifyRuns(git, runs, target) {
  const fulls = [];
  const scoped = [];
  const interrupted = [];
  const partial = [];
  const outside = [];
  for (const run of runs) {
    if (!run.complete) interrupted.push(run);
    else if (!run.source || !isAncestor(git, run.source, target)) outside.push(run);
    else if (run.mode === 'full') fulls.push(run);
    else if (run.scopeComplete === true && run.base) scoped.push(run);
    else partial.push(run);
  }
  // The full-run age clock uses the newest full by source commit time, independent of run numbering.
  const time = (run) => Number(git('show', '-s', '--format=%ct', run.source));
  const lastFull = fulls.map((run) => ({ run, at: time(run) })).sort((left, right) => right.at - left.at)[0]?.run ?? null;
  return { fulls, scoped, lastFull, interrupted, partial, outside };
}

function pathspecs(policy) {
  return [
    ...policy.boundaryRoots,
    ...Object.keys(policy.nonBoundary).map((path) => ':(exclude)' + path),
    ...policy.ignoreSuffixes.map((suffix) => ':(exclude,glob)**/*' + suffix),
    ...policy.ignoreSegments.map((segment) => ':(exclude,glob)**/' + segment + '/**'),
  ];
}

// A package is any directory holding non-test Go source, nested ones included (e.g. internal/cluster/sshlogin).
function packages(git, policy, ref) {
  const excluded = Object.keys(policy.nonBoundary);
  const directories = new Set();
  for (const root of policy.packageRoots) {
    const listed = git('ls-tree', '-r', '--name-only', ref, root + '/');
    for (const path of listed ? listed.split('\n') : []) {
      const segments = path.split('/');
      if (!path.endsWith('.go') || policy.ignoreSuffixes.some((suffix) => path.endsWith(suffix))) continue;
      if (segments.some((segment) => policy.ignoreSegments.includes(segment))) continue;
      directories.add(segments.slice(0, -1).join('/'));
    }
  }
  return [...directories].filter((path) => !excluded.some((prefix) => path === prefix || path.startsWith(prefix + '/'))).sort();
}

export function loadPolicy(repo = repoRoot) {
  return JSON.parse(readFileSync(join(repo, POLICY_PATH), 'utf8'));
}

// Candidate-time signal (5.4): boundary packages a writer candidate adds relative to its exact base.
export function addedBoundaryPackages(repo, base, head = 'HEAD', policy = loadPolicy(repo)) {
  const git = gitRunner(repo);
  const known = new Set(packages(git, policy, base));
  return packages(git, policy, head).filter((path) => !known.has(path));
}

export function assessCoverage({ repo = repoRoot, target = 'HEAD', policy, runs }) {
  const git = gitRunner(repo);
  const targetSha = git('rev-parse', '--verify', '--end-of-options', target + '^{commit}');
  const commitTime = (ref) => Number(git('show', '-s', '--format=%ct', ref));
  const targetTime = commitTime(targetSha);
  const ageDays = (seconds) => Math.floor((targetTime - seconds) / DAY_SECONDS);
  const classes = classifyRuns(git, runs, targetSha);
  const report = {
    target: targetSha, ...classes, fullAgeDays: null, commits: [], covered: {}, files: [], newPackages: [], reasons: [],
  };
  if (!classes.lastFull) {
    report.decision = 'full-required';
    report.reasons.push('no completed full run in target history');
    return report;
  }
  report.fullAgeDays = ageDays(commitTime(classes.lastFull.source));
  // Boundary commits no completed full run contains, each with the boundary files it touched.
  const exclusions = [...new Set(classes.fulls.map((run) => '^' + run.source))];
  const log = git('log', '--no-merges', '--name-only', '--format=%x1e%H%x09%ct%x09%s', targetSha, ...exclusions,
    '--', ...pathspecs(policy));
  const pending = log.split('\x1e').filter((record) => record.trim()).map((record) => {
    const [header, ...paths] = record.trim().split('\n');
    const [sha, time, subject] = header.split('\t');
    return { sha, ageDays: ageDays(Number(time)), subject, paths: paths.filter(Boolean) };
  });
  const ranges = classes.scoped.map((run) => ({
    run,
    commits: new Set(git('rev-list', run.base + '..' + run.source).split('\n').filter(Boolean)),
  }));
  const files = new Set();
  for (const commit of pending) {
    const range = ranges.find((candidate) => candidate.commits.has(commit.sha));
    if (range) {
      report.covered[range.run.name] = (report.covered[range.run.name] ?? 0) + 1;
      continue;
    }
    report.commits.push({ sha: commit.sha, ageDays: commit.ageDays, subject: commit.subject });
    for (const path of commit.paths) files.add(path);
    const parent = git('rev-parse', commit.sha + '^');
    const known = new Set(packages(git, policy, parent));
    for (const path of packages(git, policy, commit.sha)) if (!known.has(path)) report.newPackages.push(path);
  }
  report.files = [...files].sort();
  report.newPackages = [...new Set(report.newPackages)].sort();
  report.nextScoped = { comparison_base: classes.lastFull.source, source_ref: targetSha };
  report.oldestAgeDays = report.commits.length ? Math.max(...report.commits.map((commit) => commit.ageDays)) : 0;
  if (report.fullAgeDays > policy.fullMaxAgeDays) report.reasons.push('last full run is ' + report.fullAgeDays + ' days old (max ' + policy.fullMaxAgeDays + ')');
  if (report.reasons.length) {
    report.decision = 'full-required';
    return report;
  }
  if (report.newPackages.length) report.reasons.push('new boundary packages: ' + report.newPackages.join(', '));
  if (report.oldestAgeDays > policy.scopedMaxAgeDays) {
    report.reasons.push('oldest unaudited boundary change is ' + report.oldestAgeDays + ' days old (max ' + policy.scopedMaxAgeDays + ')');
  }
  report.decision = report.reasons.length ? 'scoped-required' : 'ok';
  return report;
}

function short(sha) {
  return sha ? sha.slice(0, 12) : 'none';
}

export function render(report, policy) {
  const lines = ['security_audit_coverage target=' + short(report.target) + ' decision=' + report.decision];
  lines.push(...report.reasons.map((reason) => '  reason: ' + reason));
  if (report.lastFull) {
    lines.push('last_full run=' + report.lastFull.name + ' source=' + short(report.lastFull.source)
      + ' age_days=' + report.fullAgeDays + ' max=' + policy.fullMaxAgeDays
      + (report.lastFull.dirty ? ' source_dirty=true (not fully reproducible)' : ''));
    const covered = Object.entries(report.covered).map(([name, count]) => name + '=' + count).join(' ');
    lines.push('covered_by_scoped ' + (covered || 'none') + ' (boundary commits outside the full run, audited by these scoped runs)');
    lines.push('unaudited commits=' + report.commits.length + ' files=' + report.files.length
      + ' oldest_age_days=' + report.oldestAgeDays + ' max=' + policy.scopedMaxAgeDays);
    lines.push(...report.commits.map((commit) => '  ' + short(commit.sha) + ' ' + commit.ageDays + 'd ' + commit.subject));
    lines.push('new_boundary_packages=' + (report.newPackages.join(',') || 'none'));
    if (report.decision !== 'ok') {
      lines.push('next_scoped comparison_base=' + report.nextScoped.comparison_base + ' source_ref=' + report.nextScoped.source_ref
        + ' (audit the unaudited files listed by --format=json)');
    }
  }
  for (const run of report.interrupted) lines.push('interrupted ' + run.name + ' status=' + run.status + ' (not coverage)');
  for (const run of report.partial) lines.push('partial ' + run.name + ' (scope_complete is not true or comparison_base is missing; not coverage)');
  for (const run of report.outside) lines.push('outside_history ' + run.name + ' (source is not in target history; not coverage)');
  return lines.join('\n');
}

function usage() {
  return 'usage: node scripts/check-security-audit-coverage.mjs [--validate] [--target <ref>] [--require] [--format text|json]\n';
}

// Workflows write `--target HEAD` and `--format=json`; both spellings must parse the same way.
export function parseArguments(argv) {
  const options = { validate: false, require: false, target: 'HEAD', format: 'text' };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    const match = /^--(target|format)(?:=(.*))?$/.exec(argument);
    if (argument === '--validate') options.validate = true;
    else if (argument === '--require') options.require = true;
    else if (match) {
      const value = match[2] ?? argv[++index];
      if (!value || value.startsWith('-')) return null;
      options[match[1]] = value;
    } else return null;
  }
  return ['text', 'json'].includes(options.format) ? options : null;
}

export function main(argv, repo = repoRoot) {
  const options = parseArguments(argv);
  if (!options) {
    process.stderr.write(usage());
    return 2;
  }
  let policy;
  let runs;
  try {
    policy = loadPolicy(repo);
    runs = loadRuns(repo);
  } catch (error) {
    process.stderr.write('check-security-audit-coverage: ' + error.message + '\n');
    return 1;
  }
  const failures = [...validatePolicy(policy, repo), ...runs.flatMap(validateRun)];
  if (failures.length) {
    process.stderr.write('Security audit coverage validation failed:\n- ' + failures.join('\n- ') + '\n');
    return 1;
  }
  if (options.validate) {
    process.stdout.write('Security audit coverage validation passed (' + runs.length + ' runs).\n');
    return 0;
  }
  let report;
  try {
    report = assessCoverage({ repo, target: options.target, policy, runs });
  } catch (error) {
    process.stderr.write('check-security-audit-coverage: ' + error.message + '\n');
    return 1;
  }
  if (options.format === 'json') {
    const strip = (run) => run && { name: run.name, mode: run.mode, status: run.status, source: run.source, dirty: run.dirty };
    process.stdout.write(JSON.stringify({
      ...report,
      lastFull: strip(report.lastFull),
      fulls: report.fulls.map(strip),
      scoped: report.scoped.map(strip),
      interrupted: report.interrupted.map(strip),
      partial: report.partial.map(strip),
      outside: report.outside.map(strip),
    }, null, 2) + '\n');
  } else {
    process.stdout.write(render(report, policy) + '\n');
  }
  return options.require && report.decision !== 'ok' ? 3 : 0;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main(process.argv.slice(2));
}
