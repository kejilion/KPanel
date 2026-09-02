import test from 'node:test';
import assert from 'node:assert/strict';

import {
  COVERAGE_BASELINE,
  acceptancePath,
  compareTags,
  evaluateAcceptanceCoverage,
  parseArguments,
  parseTagVersion,
  recordTag,
} from '../check-release-acceptance-coverage.mjs';

test('stable release tags parse into comparable versions', () => {
  assert.deepEqual(parseTagVersion('v0.100.0'), [0, 100, 0]);
  assert.deepEqual(parseTagVersion(' v1.2.3 '), [1, 2, 3]);
  assert.equal(parseTagVersion('v0.100.0-rc1'), null);
  assert.equal(parseTagVersion('v0.100'), null);
  assert.equal(parseTagVersion('release-v0.100.0'), null);
  assert.equal(parseTagVersion(undefined), null);
});

test('tag comparison orders numerically rather than lexically', () => {
  assert.ok(compareTags('v0.100.0', 'v0.99.6') > 0);
  assert.ok(compareTags('v0.9.0', 'v0.10.0') < 0);
  assert.equal(compareTags('v0.83.0', 'v0.83.0'), 0);
  assert.throws(() => compareTags('v0.83.0', 'main'), /non-stable tags/);
});

test('acceptance record filenames map to their tag', () => {
  assert.equal(recordTag('release-v0.100.0-acceptance.md'), 'v0.100.0');
  assert.equal(recordTag('release-acceptance-template.md'), null);
  assert.equal(recordTag('release-v0.100.0-acceptance.md.bak'), null);
  assert.equal(recordTag('project-management.md'), null);
  assert.equal(acceptancePath('v0.100.0'), 'docs/release-v0.100.0-acceptance.md');
});

test('coverage passes when every in-scope tag owns a record', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.82.0', 'v0.83.0', 'v0.99.6', 'v0.100.0'],
    records: ['v0.83.0', 'v0.99.6', 'v0.100.0'],
  });
  assert.equal(result.ok, true);
  assert.deepEqual(result.missing, []);
  assert.deepEqual(result.inScope, ['v0.83.0', 'v0.99.6', 'v0.100.0']);
  assert.equal(result.stableTagCount, 4);
});

test('a published tag without a record fails the reverse gate', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.99.6', 'v0.100.0'],
    records: ['v0.99.6'],
  });
  assert.equal(result.ok, false);
  assert.deepEqual(result.missing, ['v0.100.0']);
});

test('history before the baseline is not required to be backfilled', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.13.0', 'v0.54.0', 'v0.82.0', 'v0.83.0'],
    records: ['v0.83.0'],
  });
  assert.equal(result.ok, true);
  assert.deepEqual(result.inScope, ['v0.83.0']);
  assert.equal(result.baseline, COVERAGE_BASELINE);
});

test('acceptance records before the baseline are outside the orphan check', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.83.0'],
    records: ['v0.20.3', 'v0.83.0'],
  });
  assert.equal(result.ok, true);
  assert.deepEqual(result.orphans, []);
});

test('a lowered baseline exposes the historical gap instead of hiding it', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.82.0', 'v0.83.0'],
    records: ['v0.83.0'],
    baseline: 'v0.82.0',
  });
  assert.equal(result.ok, false);
  assert.deepEqual(result.missing, ['v0.82.0']);
});

test('the newest tag is exempt only while the evaluated commit is still the tag commit', () => {
  const inFlight = evaluateAcceptanceCoverage({
    tags: ['v0.99.6', 'v0.100.0'],
    records: ['v0.99.6'],
    inFlightTag: 'v0.100.0',
  });
  assert.equal(inFlight.ok, true);
  assert.equal(inFlight.exempt, 'v0.100.0');
  assert.deepEqual(inFlight.missing, []);

  const afterFollowUpCommit = evaluateAcceptanceCoverage({
    tags: ['v0.99.6', 'v0.100.0'],
    records: ['v0.99.6'],
    inFlightTag: null,
  });
  assert.equal(afterFollowUpCommit.ok, false);
  assert.equal(afterFollowUpCommit.exempt, null);
  assert.deepEqual(afterFollowUpCommit.missing, ['v0.100.0']);
});

test('the in-flight exemption covers at most the newest tag', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.99.5', 'v0.99.6', 'v0.100.0'],
    records: ['v0.99.5'],
    inFlightTag: 'v0.100.0',
  });
  assert.equal(result.ok, false);
  assert.deepEqual(result.missing, ['v0.99.6']);
  assert.equal(result.exempt, 'v0.100.0');
});

test('an in-flight tag that already has a record is not reported as exempt', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.99.6', 'v0.100.0'],
    records: ['v0.99.6', 'v0.100.0'],
    inFlightTag: 'v0.100.0',
  });
  assert.equal(result.ok, true);
  assert.equal(result.exempt, null);
});

test('an out-of-scope in-flight tag cannot exempt an in-scope gap', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.82.0', 'v0.99.6'],
    records: [],
    inFlightTag: 'v0.82.0',
  });
  assert.equal(result.ok, false);
  assert.equal(result.exempt, null);
  assert.deepEqual(result.missing, ['v0.99.6']);
});

test('a record naming an unpublished tag fails unless it is the version being prepared', () => {
  const prepared = evaluateAcceptanceCoverage({
    tags: ['v0.99.6'],
    records: ['v0.99.6', 'v0.100.0'],
    currentVersion: '0.100.0',
  });
  assert.equal(prepared.ok, true);
  assert.deepEqual(prepared.orphans, []);

  const typo = evaluateAcceptanceCoverage({
    tags: ['v0.99.6'],
    records: ['v0.99.6', 'v0.101.0'],
    currentVersion: '0.100.0',
  });
  assert.equal(typo.ok, false);
  assert.deepEqual(typo.orphans, ['v0.101.0']);
});

test('non-stable tags and duplicates never enter the required set', () => {
  const result = evaluateAcceptanceCoverage({
    tags: ['v0.100.0', 'v0.100.0', 'v0.100.1-rc1', 'main'],
    records: ['v0.100.0'],
  });
  assert.equal(result.ok, true);
  assert.equal(result.stableTagCount, 1);
  assert.deepEqual(result.inScope, ['v0.100.0']);
});

test('malformed inputs are rejected instead of silently passing', () => {
  assert.throws(() => evaluateAcceptanceCoverage({ tags: 'v0.100.0', records: [] }), /tags must be an array/);
  assert.throws(() => evaluateAcceptanceCoverage({ tags: [], records: null }), /records must be an array/);
  assert.throws(() => evaluateAcceptanceCoverage({ tags: [], records: [], baseline: 'nope' }), /stable release tag/);
});

test('argument parsing defaults to the continuity baseline and rejects unknown input', () => {
  const defaults = parseArguments([]);
  assert.equal(defaults.baseline, COVERAGE_BASELINE);
  assert.equal(defaults.ref, null);
  assert.equal(defaults.format, 'text');

  const explicit = parseArguments(['--ref', 'origin/main', '--baseline', 'v0.90.2', '--format', 'json']);
  assert.equal(explicit.ref, 'origin/main');
  assert.equal(explicit.baseline, 'v0.90.2');
  assert.equal(explicit.format, 'json');

  assert.throws(() => parseArguments(['--baseline']), /Missing value/);
  assert.throws(() => parseArguments(['--baseline', 'v0.90']), /stable release tag/);
  assert.throws(() => parseArguments(['--format', 'yaml']), /text or json/);
  assert.throws(() => parseArguments(['--unknown']), /Unknown argument/);
  assert.equal(parseArguments(['--help']).help, true);
});
