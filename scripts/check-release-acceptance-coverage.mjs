#!/usr/bin/env node

// Reverse gate for release acceptance records.
//
// scripts/verify-change.sh already validates an acceptance record whenever one appears in the
// changed-file set, and refuses deletions. That is a forward check: it only fires when someone
// touches a record. It cannot notice a published stable tag whose record was never written at all,
// which is how v0.100.0 reached production without an acceptance record.
//
// This check runs in the opposite direction: every merged stable tag at or after the continuity
// baseline must own docs/release-<tag>-acceptance.md. It asserts existence only. Historical records
// predate the machine-marker blocks and some carry inconsistencies that must not be rewritten
// (docs/release-acceptance-template.md: 不回填或批量改写历史记录), so content validation stays with
// the forward gate that runs on the records actually being changed.

import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { gitEnvironment } from './report-release-metrics.mjs';

// v0.82.0 is the newest stable tag missing a record; from v0.83.0 forward the only gap was
// v0.100.0. The baseline therefore matches the factual record instead of demanding backfill.
export const COVERAGE_BASELINE = 'v0.83.0';

const STABLE_RELEASE_TAG = /^v(\d+)\.(\d+)\.(\d+)$/;
const ACCEPTANCE_RECORD = /^release-(v\d+\.\d+\.\d+)-acceptance\.md$/;

export function parseTagVersion(value) {
  const match = STABLE_RELEASE_TAG.exec(String(value ?? '').trim());
  return match ? match.slice(1, 4).map(Number) : null;
}

export function compareTags(left, right) {
  const leftVersion = parseTagVersion(left);
  const rightVersion = parseTagVersion(right);
  if (leftVersion === null || rightVersion === null) throw new Error('cannot compare non-stable tags');
  for (let index = 0; index < 3; index += 1) {
    if (leftVersion[index] !== rightVersion[index]) return leftVersion[index] - rightVersion[index];
  }
  return 0;
}

export function acceptancePath(tag) {
  return 'docs/release-' + tag + '-acceptance.md';
}

export function recordTag(filename) {
  const match = ACCEPTANCE_RECORD.exec(String(filename ?? '').trim());
  return match ? match[1] : null;
}

export function parseArguments(argv) {
  const options = { repo: process.cwd(), ref: null, baseline: COVERAGE_BASELINE, format: 'text' };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    const value = argv[index + 1];
    if (argument === '--help' || argument === '-h') {
      options.help = true;
      continue;
    }
    if (argument === '--repo' || argument === '--ref' || argument === '--baseline' || argument === '--format') {
      if (value === undefined) throw new Error('Missing value for ' + argument);
      index += 1;
      if (argument === '--repo') options.repo = value;
      if (argument === '--ref') options.ref = value;
      if (argument === '--baseline') options.baseline = value;
      if (argument === '--format') options.format = value;
      continue;
    }
    throw new Error('Unknown argument: ' + argument);
  }
  if (parseTagVersion(options.baseline) === null) throw new Error('--baseline must be a stable release tag such as v0.83.0');
  if (!['text', 'json'].includes(options.format)) throw new Error('--format must be text or json');
  options.repo = resolve(options.repo);
  return options;
}

// Pure core so the unit test does not need a fixture repository.
//
// `inFlightTag` is the single newest stable tag whose record is not due yet, decided by the caller
// from git topology. The documented release order tags the candidate, deploys to production, and
// only then commits the record, so the tag legitimately exists without a record while the evaluated
// commit is still the tag commit itself. As soon as any commit lands after the tag the record is
// mandatory, which bounds the escape window to zero follow-up commits instead of the open-ended
// drift that let v0.100.0 slip.
export function evaluateAcceptanceCoverage({ tags, records, baseline = COVERAGE_BASELINE, inFlightTag = null, currentVersion = null } = {}) {
  if (!Array.isArray(tags)) throw new Error('tags must be an array');
  if (!Array.isArray(records)) throw new Error('records must be an array');
  if (parseTagVersion(baseline) === null) throw new Error('baseline must be a stable release tag');

  const stableTags = [...new Set(tags.filter((tag) => parseTagVersion(tag) !== null))].sort(compareTags);
  const recordTags = new Set(records.filter((tag) => parseTagVersion(tag) !== null));
  const inScope = stableTags.filter((tag) => compareTags(tag, baseline) >= 0);
  const inScopeRecordTags = new Set([...recordTags].filter((tag) => compareTags(tag, baseline) >= 0));

  const exempt = inFlightTag !== null && inScope.includes(inFlightTag) && !recordTags.has(inFlightTag)
    ? inFlightTag
    : null;
  const missing = inScope.filter((tag) => !recordTags.has(tag) && tag !== exempt);

  const versionTag = currentVersion === null ? null : 'v' + String(currentVersion).trim();
  const published = new Set(stableTags);
  const orphans = [...inScopeRecordTags]
    .filter((tag) => !published.has(tag) && tag !== versionTag)
    .sort(compareTags);

  return {
    baseline,
    stableTagCount: stableTags.length,
    inScope,
    missing,
    orphans,
    exempt,
    ok: missing.length === 0 && orphans.length === 0,
  };
}

function runGit(repo, arguments_) {
  return execFileSync('git', ['-C', repo, ...arguments_], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    env: gitEnvironment(repo),
  }).trim();
}

function tryGit(repo, arguments_) {
  try {
    return runGit(repo, arguments_);
  } catch {
    return null;
  }
}

function gitSucceeds(repo, arguments_) {
  try {
    execFileSync('git', ['-C', repo, ...arguments_], {
      stdio: ['ignore', 'ignore', 'ignore'],
      env: gitEnvironment(repo),
    });
    return true;
  } catch {
    return false;
  }
}

function mergedStableTags(repo, commit) {
  return runGit(repo, ['tag', '--merged', commit, '--list', 'v*'])
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((tag) => parseTagVersion(tag) !== null);
}

function recordsFromWorkingTree(repo) {
  let entries;
  try {
    entries = readdirSync(resolve(repo, 'docs'));
  } catch {
    throw new Error('docs directory is unreadable; run this check from a full checkout');
  }
  return entries.map((entry) => recordTag(entry)).filter((tag) => tag !== null);
}

function recordsFromRef(repo, ref) {
  const listing = tryGit(repo, ['ls-tree', '-r', '--name-only', ref, '--', 'docs']);
  if (listing === null) throw new Error('cannot list docs at ' + ref);
  return listing
    .split(/\r?\n/)
    .map((path) => recordTag(path.trim().replace(/^docs\//, '')))
    .filter((tag) => tag !== null);
}

function currentVersion(repo, ref) {
  const raw = ref === null
    ? (() => {
        try {
          return readFileSync(resolve(repo, 'VERSION'), 'utf8');
        } catch {
          return null;
        }
      })()
    : tryGit(repo, ['show', ref + ':VERSION']);
  return raw === null ? null : raw.replace(/[\r\n]/g, '').trim() || null;
}

export function checkAcceptanceCoverage(options) {
  const { repo, ref, baseline } = options;
  runGit(repo, ['rev-parse', '--is-inside-work-tree']);
  const evaluatedRef = ref ?? 'HEAD';
  const evaluatedCommit = runGit(repo, ['rev-parse', '--verify', evaluatedRef + '^{commit}']);

  const tags = mergedStableTags(repo, evaluatedCommit);
  if (tags.length === 0) {
    // Fail closed. A tagless checkout cannot prove coverage, and silently passing here would
    // reopen exactly the hole this gate exists to close.
    throw new Error('no stable release tags are visible at ' + evaluatedRef +
      '; run "git fetch origin --tags" before verifying, because acceptance coverage cannot be proven without tags');
  }

  const records = ref === null ? recordsFromWorkingTree(repo) : recordsFromRef(repo, ref);
  const newest = [...tags].sort(compareTags).at(-1);
  const newestCommit = tryGit(repo, ['rev-parse', '--verify', 'refs/tags/' + newest + '^{commit}']);
  const inFlightTag = newestCommit !== null &&
    gitSucceeds(repo, ['merge-base', '--is-ancestor', evaluatedCommit, newestCommit])
    ? newest
    : null;

  return {
    ...evaluateAcceptanceCoverage({
      tags,
      records,
      baseline,
      inFlightTag,
      currentVersion: currentVersion(repo, ref),
    }),
    ref: evaluatedRef,
    commit: evaluatedCommit,
  };
}

function help() {
  return [
    'Usage: node scripts/check-release-acceptance-coverage.mjs [options]',
    '',
    'Asserts that every merged stable release tag at or after the continuity baseline owns',
    'docs/release-<tag>-acceptance.md. Existence only; record content stays with the forward',
    'gate in scripts/verify-change.sh.',
    '',
    'Options:',
    '  --repo <path>       Repository to inspect (default: current directory)',
    '  --ref <ref>         Evaluate a specific commit instead of the working tree',
    '  --baseline <tag>    Oldest tag required to own a record (default: ' + COVERAGE_BASELINE + ')',
    '  --format <fmt>      text or json (default: text)',
    '  -h, --help          Show this help',
  ].join('\n');
}

export function main(argv) {
  const options = parseArguments(argv);
  if (options.help) {
    process.stdout.write(help() + '\n');
    return;
  }
  const result = checkAcceptanceCoverage(options);
  if (options.format === 'json') {
    process.stdout.write(JSON.stringify(result, null, 2) + '\n');
  }
  if (!result.ok) {
    const details = [];
    for (const tag of result.missing) {
      details.push('published tag ' + tag + ' has no acceptance record at ' + acceptancePath(tag));
    }
    for (const tag of result.orphans) {
      details.push('acceptance record ' + acceptancePath(tag) + ' names a tag that is not a published stable release');
    }
    throw new Error('\n- ' + details.join('\n- ') +
      '\nAssemble the record from real release evidence using docs/release-acceptance-template.md; do not backfill unrelated history.');
  }
  if (options.format !== 'json') {
    // Report the exemption explicitly. A bounded skip that is not printed reads as full coverage.
    const exempt = result.exempt === null
      ? 'none'
      : result.exempt + ' (record not yet due; the evaluated commit is still the tag commit)';
    process.stdout.write('release_acceptance_coverage=pass baseline=' + result.baseline +
      ' in_scope=' + result.inScope.length + ' tags=' + result.stableTagCount +
      ' in_flight_exempt=' + exempt + '\n');
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    process.stderr.write('Release acceptance coverage failed: ' + error.message + '\n');
    process.exitCode = 1;
  }
}
