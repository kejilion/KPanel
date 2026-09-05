import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

import {
  classifyUpdate,
  compareVersions,
  goExecutable,
  githubActionVersionCandidate,
  immutableDigestCandidate,
  isStableVersion,
  maintenanceStatus,
  managedScriptRevisionCandidate,
  npmCandidatesFromOutdated,
  npmInvocation,
  parseConcatenatedJson,
  requestHeaders,
  renderMarkdown,
  summarize,
  validatePolicy,
} from '../report-dependency-freshness.mjs';

test('stable version filter excludes prereleases and floating labels', () => {
  assert.equal(isStableVersion('v1.2.3'), true);
  assert.equal(isStableVersion('1.2.3-rc.1'), false);
  assert.equal(isStableVersion('latest'), false);
});

test('npm invocation avoids Windows command-shell wrappers and supports overrides', () => {
  assert.deepEqual(npmInvocation('win32', {}, 'C:\\Node\\node.exe'), {
    command: 'C:\\Node\\node.exe',
    prefixArguments: ['C:\\Node\\node_modules\\npm\\bin\\npm-cli.js'],
  });
  assert.deepEqual(npmInvocation('linux', {}, '/usr/bin/node'), { command: 'npm', prefixArguments: [] });
  assert.deepEqual(npmInvocation('win32', { NPM: 'custom-npm' }, 'node'), { command: 'custom-npm', prefixArguments: [] });
  assert.equal(goExecutable({}), 'go');
  assert.equal(goExecutable({ GO: 'custom-go' }), 'custom-go');
});

test('GitHub token is never sent to non-GitHub upstreams', () => {
  const environment = { GITHUB_TOKEN: 'secret' };
  assert.equal(requestHeaders('https://api.github.com/repos/actions/checkout', environment).Authorization, 'Bearer secret');
  assert.equal(requestHeaders('https://hub.docker.com/v2/repositories/library/node', environment).Authorization, undefined);
  assert.equal(requestHeaders('https://nodejs.org/dist/index.json', environment).Authorization, undefined);
});

test('version comparison and classification separate patch, minor, and major/toolchain changes', () => {
  assert.equal(compareVersions('1.2.3', '1.2.4'), -1);
  assert.equal(compareVersions('2.0.0', '1.9.9'), 1);
  assert.equal(classifyUpdate('1.2.3', '1.2.4'), 'compatible-patch');
  assert.equal(classifyUpdate('1.2.3', '1.3.0'), 'minor');
  assert.equal(classifyUpdate('1.2.3', '2.0.0'), 'major-toolchain-base');
  assert.equal(classifyUpdate('1.2.3', '1.2.4', 'toolchain'), 'compatible-patch');
});

test('npm candidates separate actionable direct updates from transitive ownership signals', () => {
  const candidates = npmCandidatesFromOutdated({
    direct: { current: '1.0.0', wanted: '1.0.1', latest: '2.0.0' },
    transitive: { current: '3.0.0', wanted: '3.0.1', latest: '4.0.0' },
  }, new Set(['direct']));
  assert.deepEqual(candidates.map((item) => [item.component, item.candidate, item.dependencyScope, item.adoptionLane]), [
    ['direct', '1.0.1', 'direct', 'declared-range'],
    ['direct', '2.0.0', 'direct', 'outside-declared-range'],
    ['transitive', '3.0.1', 'transitive', 'parent-compatible-range'],
    ['transitive', '4.0.0', 'transitive', 'outside-parent-range'],
  ]);
});

test('npm candidates use the root dependent instead of promoting peer records to direct actions', () => {
  const candidates = npmCandidatesFromOutdated({
    typescript: [
      { current: '5.9.3', wanted: '7.0.2', latest: '7.0.2', dependent: 'vue-tsc' },
      { current: '5.9.3', wanted: '5.9.3', latest: '7.0.2', dependent: 'web' },
    ],
  }, new Set(['typescript']), 'web');
  assert.deepEqual(candidates.map((item) => [item.dependencyScope, item.adoptionLane, item.candidate]), [
    ['transitive', 'parent-compatible-range', '7.0.2'],
    ['direct', 'outside-declared-range', '7.0.2'],
  ]);
});

test('immutable digest drift is visible even when the image tag is unchanged', () => {
  assert.equal(immutableDigestCandidate('image', 'sha256:' + 'a'.repeat(64), 'sha256:' + 'a'.repeat(64), 'registry'), null);
  const drift = immutableDigestCandidate('image', 'sha256:' + 'a'.repeat(64), 'sha256:' + 'b'.repeat(64), 'registry');
  assert.equal(drift.updateClass, 'major-toolchain-base');
  assert.match(drift.current, /^sha256:a/);
  assert.match(drift.candidate, /^sha256:b/);
});

test('GitHub action upgrades report the candidate tag SHA instead of the current pin', () => {
  const update = githubActionVersionCandidate(
    'actions/checkout',
    { version: 'v6.0.2', sha: 'a'.repeat(40) },
    'v7.0.1',
    'b'.repeat(40),
  );
  assert.equal(update.pinnedSha, 'b'.repeat(40));
});

test('managed script revisions ignore unrelated repository commits but expose content drift', () => {
  assert.equal(managedScriptRevisionCandidate('a'.repeat(40), 'b'.repeat(40), 'same', 'same'), null);
  const update = managedScriptRevisionCandidate('a'.repeat(40), 'b'.repeat(40), 'before', 'after');
  assert.equal(update.component, 'managed kejilion.sh');
  assert.notEqual(update.currentSha256, update.candidateSha256);
  assert.match(update.source, /managed file content/);
});

test('concatenated Go JSON parser keeps nested values and escaped braces intact', () => {
  const values = parseConcatenatedJson('{"Path":"a","Update":{"Version":"v1.2.3"}}\n{"Path":"b","Note":"}"}');
  assert.equal(values.length, 2);
  assert.equal(values[0].Update.Version, 'v1.2.3');
  assert.equal(values[1].Note, '}');
});

test('policy validation requires complete coverage and prohibits automatic main changes', () => {
  const policy = {
    schemaVersion: 1,
    requiredGroups: [],
    groups: [],
    exceptionRequiredFields: ['reason', 'owner', 'reviewDate', 'exitCondition', 'rollbackPoint'],
    exceptions: [],
    automationBoundary: { automaticMainCommit: true, automaticRelease: false, automaticProductionDeployment: false },
  };
  const failures = validatePolicy(policy, process.cwd());
  assert.ok(failures.some((failure) => failure.includes('go-modules')));
  assert.ok(failures.some((failure) => failure.includes('automatic main')));
});

test('policy validation keeps automation, cadence, and workflow triggers enforceable', () => {
  const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  assert.deepEqual(validatePolicy(policy, process.cwd()), []);
  policy.automationBoundary.automaticSecurityAdvisoryCheck = false;
  policy.cadence.eolReviewMaximumDays = 93;
  const failures = validatePolicy(policy, process.cwd());
  assert.ok(failures.some((failure) => failure.includes('security advisory')));
  assert.ok(failures.some((failure) => failure.includes('governed maximum') && failure.includes('eolReviewMaximumDays')));
});

test('policy validation fixes the cross-repository linkage vocabulary and release boundaries', () => {
  const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  policy.crossRepositoryReleaseLinkage.states = ['not-required', 'coupled'];
  policy.crossRepositoryReleaseLinkage.decisionRules['not-required'].scriptRelease = 'deferred';
  policy.crossRepositoryReleaseLinkage.decisionRules.coupled.ifScriptNotReady = 'continue';
  policy.crossRepositoryReleaseLinkage.decisionRules['script-only'].kpanelVersionChange = true;
  const failures = validatePolicy(policy, process.cwd());
  assert.ok(failures.some((failure) => failure.includes('not-required, coupled, and script-only')));
  assert.ok(failures.some((failure) => failure.includes('script release is not in scope')));
  assert.ok(failures.some((failure) => failure.includes('compatible script release')));
  assert.ok(failures.some((failure) => failure.includes('script-only linkage')));
});

test('policy validation requires bounded adoption decisions without forcing unsafe adoption', () => {
  const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  policy.adoptionLifecycle.classes['major-toolchain-base'].decisionMaximumDays = 91;
  policy.adoptionLifecycle.deferralRequiresException = false;
  const failures = validatePolicy(policy, process.cwd());
  assert.ok(failures.some((failure) => failure.includes('time-bounded exception')));
  assert.ok(failures.some((failure) => failure.includes('governed maximum') && failure.includes('major-toolchain-base.decisionMaximumDays')));
});

test('cross-repository linkage cannot silently default or extend its states', () => {
  const cases = [
    ['missing linkage', (policy) => { delete policy.crossRepositoryReleaseLinkage; }, /schemaVersion/],
    ['missing decision', (policy) => { delete policy.crossRepositoryReleaseLinkage.requiresExplicitDecision; }, /explicit decision/],
    ['false decision', (policy) => { policy.crossRepositoryReleaseLinkage.requiresExplicitDecision = false; }, /explicit decision/],
    ['default not-required', (policy) => { policy.crossRepositoryReleaseLinkage.defaultState = 'not-required'; }, /must not define defaultState/],
    ['null default', (policy) => { policy.crossRepositoryReleaseLinkage.defaultState = null; }, /must not define defaultState/],
    ['extra state', (policy) => { policy.crossRepositoryReleaseLinkage.states.push('deferred'); }, /must expose not-required/],
    ['duplicate state', (policy) => { policy.crossRepositoryReleaseLinkage.states.push('coupled'); }, /must expose not-required/],
  ];
  for (const [name, mutate, expected] of cases) {
    const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
    mutate(policy);
    assert.ok(validatePolicy(policy, process.cwd()).some((failure) => expected.test(failure)), name);
  }
});

test('each linkage state requires its own evidence and complete handoff fields', () => {
  const baseline = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  for (const [state, rule] of Object.entries(baseline.crossRepositoryReleaseLinkage.decisionRules)) {
    for (const evidence of rule.requiredEvidence) {
      const policy = structuredClone(baseline);
      policy.crossRepositoryReleaseLinkage.decisionRules[state].requiredEvidence = rule.requiredEvidence.filter((item) => item !== evidence);
      assert.ok(validatePolicy(policy, process.cwd()).includes('cross-repository linkage evidence is incomplete: ' + state), state + ': ' + evidence);
    }
  }
  for (const [field, prefix] of [
    ['requiredDeliveryFields', 'cross-repository linkage delivery field is missing: '],
    ['releaseBlockers', 'cross-repository linkage release blocker is missing: '],
  ]) {
    for (const item of baseline.crossRepositoryReleaseLinkage[field]) {
      const policy = structuredClone(baseline);
      policy.crossRepositoryReleaseLinkage[field] = policy.crossRepositoryReleaseLinkage[field].filter((value) => value !== item);
      assert.ok(validatePolicy(policy, process.cwd()).includes(prefix + item), field + ': ' + item);
    }
  }
});

test('Go toolchain policy covers immutable Codex workflow images', () => {
  const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  const goToolchain = policy.groups.find((group) => group.id === 'go-toolchain');
  goToolchain.manifests = goToolchain.manifests.filter((manifest) => manifest !== '.codex-workflows');
  const failures = validatePolicy(policy, process.cwd());
  assert.ok(failures.some((failure) => failure.includes('go-toolchain must cover .codex-workflows')));
});

test('maintenance status exposes due exceptions and EOL review deadlines', () => {
  const policy = {
    cadence: { eolReviewMaximumDays: 92, exceptionReviewMaximumDays: 31 },
    reviewState: { lastEolReview: '2026-01-01', eolReviewEvidence: 'evidence.md' },
    exceptions: [
      { component: 'due', reviewDate: '2026-04-01' },
      { component: 'active', reviewDate: '2026-05-01' },
      { component: 'overlong', reviewDate: '2026-07-01' },
    ],
  };
  const status = maintenanceStatus(policy, new Date('2026-04-15T00:00:00Z'));
  assert.equal(status.exceptionActionRequiredCount, 2);
  assert.equal(status.exceptions[0].status, 'due');
  assert.equal(status.exceptions[1].status, 'active');
  assert.equal(status.exceptions[2].status, 'review-window-exceeds-policy');
  assert.equal(status.eolReviewStatus, 'due');
});

test('summary does not turn missing sources into an all-current conclusion', () => {
  const report = summarize({ policyVersion: 'test' }, [
    { id: 'go-modules', status: 'ok', candidates: [{ component: 'example', current: '1.0.0', candidate: '1.0.1', updateClass: 'compatible-patch', source: 'test' }], error: null },
    { id: 'npm-packages', status: 'error', candidates: [], error: 'offline' },
  ], new Date('2026-08-12T00:00:00Z'));
  assert.equal(report.failedSources, 1);
  assert.equal(report.candidateCount, 1);
  assert.equal(report.classCounts['compatible-patch'], 1);
  assert.equal(report.actionableCandidateCount, 1);
  assert.equal(report.transitiveSignalCount, 0);
  const markdown = renderMarkdown(report);
  assert.match(markdown, /只发现候选/);
  assert.match(markdown, /数据缺口/);
});

test('summary renders lifecycle deadlines and keeps transitive latest signals visible but non-actionable', () => {
  const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  const report = summarize(policy, [{
    id: 'npm-packages',
    status: 'ok',
    candidates: [
      { component: 'vue', current: '3.5.40', candidate: '3.5.41', updateClass: 'compatible-patch', dependencyScope: 'direct', source: 'test' },
      { component: 'nested', current: '1.0.0', candidate: '2.0.0', updateClass: 'major-toolchain-base', dependencyScope: 'transitive', source: 'test' },
      { component: 'Node.js LTS', current: '24.18.0', candidate: '24.19.0', updateClass: 'minor', dependencyScope: 'foundation', verificationFloor: 'L2-or-L3', source: 'test' },
    ],
    error: null,
  }], new Date('2026-08-24T00:00:00Z'));
  assert.equal(report.actionableCandidateCount, 2);
  assert.equal(report.transitiveSignalCount, 1);
  assert.equal(report.actionableCandidates.find((item) => item.component === 'Node.js LTS').minimumVerification, 'L2-or-L3');
  const markdown = renderMarkdown(report);
  assert.match(markdown, /直接依赖与基座行动项/);
  assert.match(markdown, /传递依赖归属信号/);
  assert.match(markdown, /compatible-patch \| 7 天 \| 14 天 \| 30 天/);
});

test('markdown report keeps multiline and pipe errors inside one bounded table cell', () => {
  const markdown = renderMarkdown(summarize({ policyVersion: 'test' }, [
    { id: 'source', status: 'error', candidates: [], error: 'line one\nline | two ' + 'x'.repeat(400) },
  ], new Date('2026-08-12T00:00:00Z')));
  assert.doesNotMatch(markdown, /line one\nline/);
  assert.match(markdown, /line \\| two/);
  assert.ok(markdown.split('\n').find((line) => line.includes('line one')).length < 400);
});
