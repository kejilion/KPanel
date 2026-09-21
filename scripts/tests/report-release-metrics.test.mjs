import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';

import {
  classifyChangeFailure,
  detectRepeatedProcessIncidents,
  durationHours,
  extractAcceptanceMetrics as extractAcceptanceMetricsRaw,
  gitEnvironment,
  isPreviewReleaseTag,
  isStableReleaseTag,
  parseArguments,
  previewReleaseTrain,
  productionLeadHours,
  readAcceptanceHistory,
  renderMarkdown,
  summarizePreviewMetrics,
  summarizeReleaseMetrics,
  validateAcceptanceMetrics as validateAcceptanceMetricsRaw,
  validateProcessIncidentHistory,
} from '../report-release-metrics.mjs';

const ACCEPTANCE_BLOCK_START = '<!-- kpanel-release-metrics:start -->';
const ACCEPTANCE_BLOCK_END = '<!-- kpanel-release-metrics:end -->';
const PROCESS_BLOCK_START = '<!-- kpanel-release-process-metrics:start -->';
const PROCESS_BLOCK_END = '<!-- kpanel-release-process-metrics:end -->';
const PROCESS_INCIDENTS_BLOCK_START = '<!-- kpanel-release-process-incidents:start -->';
const PROCESS_INCIDENTS_BLOCK_END = '<!-- kpanel-release-process-incidents:end -->';
const ACCEPTANCE_FIELD_NAMES = [
  '首个纳入提交时间',
  '候选冻结时间',
  '生产完成时间',
  '提交到生产用时',
  '是否回滚、紧急热修复或重复发布',
  '若发生失败，发现时间、恢复时间和逃逸门禁',
];

function withAcceptanceBlock(markdown) {
  if (markdown.includes(ACCEPTANCE_BLOCK_START) || markdown.includes(ACCEPTANCE_BLOCK_END)) return markdown;
  const lines = markdown.split('\n');
  const indexes = lines.flatMap((line, index) =>
    ACCEPTANCE_FIELD_NAMES.some((field) => line.includes(field)) ? [index] : []);
  if (indexes.length === 0) return markdown + '\n' + ACCEPTANCE_BLOCK_START + '\n' + ACCEPTANCE_BLOCK_END;
  const first = Math.min(...indexes);
  const last = Math.max(...indexes);
  return [...lines.slice(0, first), ACCEPTANCE_BLOCK_START, ...lines.slice(first, last + 1),
    ACCEPTANCE_BLOCK_END, ...lines.slice(last + 1)].join('\n');
}

function extractAcceptanceMetrics(markdown) {
  return extractAcceptanceMetricsRaw(withAcceptanceBlock(markdown));
}

function validateAcceptanceMetrics(markdown, label) {
  return validateAcceptanceMetricsRaw(withAcceptanceBlock(markdown), label);
}

function release(tag, createdAt, acceptance = {}) {
  return {
    tag,
    createdAt: new Date(createdAt),
    commit: tag + '-commit',
    acceptance: {
      exists: acceptance.exists ?? true,
      path: 'docs/release-' + tag + '-acceptance.md',
      metrics: {
        firstIncludedCommitAt: null,
        candidateFrozenAt: null,
        productionCompletedAt: null,
        commitToProduction: null,
        changeFailure: null,
        recovery: null,
        processIncidentCount: null,
        postProductionProcessIncidentCount: null,
        ...acceptance.metrics,
      },
    },
  };
}

function incidentRecord(tag, fingerprints, options = {}) {
  return {
    tag,
    label: options.label ?? tag,
    reported: options.reported ?? true,
    incidents: fingerprints.map((entry) => ({
      fingerprint: typeof entry === 'string' ? entry : entry.fingerprint,
      position: 'before-production-write',
      count: 1,
      impact: 'release entry retried',
      recoveryEvidence: 'gate log shows the retry succeeded',
      permanentAction: 'preflight added to the single repository entry',
      historicalReleases: typeof entry === 'string' ? [] : entry.historicalReleases,
    })),
  };
}

function acceptanceDocument(tag, incidents) {
  return [
    '# KPanel ' + tag + ' 发布验收记录',
    ACCEPTANCE_BLOCK_START,
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
    ACCEPTANCE_BLOCK_END,
    PROCESS_BLOCK_START,
    '- 已记录发布流程异常或无效证据拦截次数：' + incidents.length,
    '- 其中生产写操作开始后异常次数：0',
    PROCESS_BLOCK_END,
    PROCESS_INCIDENTS_BLOCK_START,
    JSON.stringify(incidents, null, 2),
    PROCESS_INCIDENTS_BLOCK_END,
  ].join('\n');
}

test('extractAcceptanceMetrics keeps absent production evidence unreported', () => {
  const metrics = extractAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：2026-08-10T07:00:00+08:00',
    '- 候选冻结时间：2026-08-10T08:00:00+08:00',
    '- 生产完成时间：未验证',
    '- 提交到生产用时：',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
  ].join('\n'));

  assert.equal(metrics.firstIncludedCommitAt, '2026-08-10T07:00:00+08:00');
  assert.equal(metrics.candidateFrozenAt, '2026-08-10T08:00:00+08:00');
  assert.equal(metrics.productionCompletedAt, null);
  assert.equal(metrics.commitToProduction, null);
  assert.equal(metrics.changeFailure, '否');
  assert.equal(metrics.recovery, null);
});

test('durationHours accepts explicit timestamps and rejects invalid intervals', () => {
  assert.equal(durationHours('2026-08-10T08:00:00Z', '2026-08-10T10:30:00Z'), 2.5);
  assert.equal(durationHours('2026-08-10T10:30:00Z', '2026-08-10T08:00:00Z'), null);
  assert.equal(durationHours('未记录', '2026-08-10T08:00:00Z'), null);
});

test('productionLeadHours derives timestamps and accepts standardized hour fallback', () => {
  assert.equal(productionLeadHours({
    firstIncludedCommitAt: '2026-08-10T07:00:00Z',
    productionCompletedAt: '2026-08-10T10:30:00Z',
    commitToProduction: '99 小时',
  }), 3.5);
  assert.equal(productionLeadHours({
    firstIncludedCommitAt: null,
    productionCompletedAt: null,
    commitToProduction: '2.75 小时',
  }), 2.75);
  assert.equal(productionLeadHours({ commitToProduction: '未记录' }), null);
});

test('summarizeReleaseMetrics never treats missing failure data as success', () => {
  const report = summarizeReleaseMetrics([
    release('v1.2.0', '2026-08-10T12:00:00Z', {
      metrics: {
        firstIncludedCommitAt: '2026-08-10T06:00:00Z',
        candidateFrozenAt: '2026-08-10T08:00:00Z',
        productionCompletedAt: '2026-08-10T10:00:00Z',
        changeFailure: '是（已回滚）',
        recovery: '10:05 发现，10:20 恢复',
        processIncidentCount: 1,
        postProductionProcessIncidentCount: 0,
      },
    }),
    release('v1.1.0', '2026-08-09T12:00:00Z', { metrics: { changeFailure: '否' } }),
    release('v1.0.0', '2026-07-01T12:00:00Z', { exists: false }),
  ], {
    days: 14,
    releases: 3,
    now: new Date('2026-08-11T00:00:00Z'),
  });

  assert.equal(report.window.releaseCount, 2);
  assert.equal(report.window.productionDeploymentCount, 1);
  assert.equal(report.window.productionDeploymentDays, 1);
  assert.equal(report.window.maxProductionDeploymentsPerDay, 1);
  assert.equal(report.recent.acceptanceCount, 2);
  assert.equal(report.recent.productionCompletionReported, 1);
  assert.equal(report.recent.productionCompletionCoverage, 0.3333);
  assert.equal(report.recent.changeFailureReported, 2);
  assert.equal(report.recent.failedReleaseCount, 1);
  assert.equal(report.recent.changeFailureRate, 0.5);
  assert.equal(report.recent.productionLeadTimeHoursMedian, 4);
  assert.equal(report.recent.freezeToProductionHoursMedian, 2);
  assert.equal(report.recent.recoveryReported, 1);
  assert.equal(report.recent.processIncidentReported, 1);
  assert.equal(report.recent.processIncidentReleaseCount, 1);
  assert.equal(report.recent.processIncidentReleaseRate, 1);
  assert.equal(report.recent.processIncidentCount, 1);
  assert.equal(report.recent.postProductionProcessIncidentCount, 0);
});

test('markdown output discloses evidence completeness', () => {
  const report = summarizeReleaseMetrics([
    release('v1.0.0', '2026-08-10T12:00:00Z', { exists: false }),
  ], {
    days: 14,
    releases: 1,
    now: new Date('2026-08-11T00:00:00Z'),
  });
  const output = renderMarkdown(report);
  assert.match(output, /验收记录覆盖率 \| 0\/1/);
  assert.match(output, /变更失败率 \| 未报告/);
  assert.match(output, /已报告流程指标版本中的异常占比 \| 未报告/);
  assert.match(output, /已记录发布流程异常\/无效证据拦截总数 \| 未报告/);
  assert.match(output, /有生产完成证据的部署数 \| 0/);
  assert.match(output, /生产完成时间覆盖率 \| 0\/1/);
  assert.match(output, /正式发布频率按稳定标签时间统计/);
  assert.match(output, /不把缺失数据推断为成功/);
});

test('process metrics are separate, explicit, and required from v0.81.2', () => {
  const acceptanceRows = [
    ACCEPTANCE_BLOCK_START,
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
    ACCEPTANCE_BLOCK_END,
  ];
  const processRows = [
    PROCESS_BLOCK_START,
    '- 已记录发布流程异常或无效证据拦截次数：1',
    '- 其中生产写操作开始后异常次数：0',
    PROCESS_BLOCK_END,
  ];
  const valid = ['# KPanel v0.81.2 发布验收记录', ...acceptanceRows, ...processRows].join('\n');
  assert.deepEqual(validateAcceptanceMetricsRaw(valid), []);
  assert.equal(extractAcceptanceMetricsRaw(valid).processIncidentCount, 1);
  assert.equal(extractAcceptanceMetricsRaw(valid).postProductionProcessIncidentCount, 0);

  const oldRecord = ['# KPanel v0.81.1 发布验收记录', ...acceptanceRows].join('\n');
  assert.deepEqual(validateAcceptanceMetricsRaw(oldRecord), []);
  const missing = validateAcceptanceMetricsRaw(['# KPanel v0.81.2 发布验收记录', ...acceptanceRows].join('\n'));
  assert.match(missing.join('\n'), /requires release-process-metrics evidence/);

  const nonCanonicalTitle = validateAcceptanceMetricsRaw(
    ['# KPanel 0.81.2 发布验收记录', ...acceptanceRows].join('\n'),
    'docs/release-v0.81.2-acceptance.md',
  );
  assert.match(nonCanonicalTitle.join('\n'), /requires release-process-metrics evidence/);
  const mismatchedTitle = validateAcceptanceMetricsRaw(
    ['# KPanel v0.81.3 发布验收记录', ...acceptanceRows, ...processRows].join('\n'),
    'docs/release-v0.81.2-acceptance.md',
  );
  assert.match(mismatchedTitle.join('\n'), /title must match the acceptance filename/);

  const unreported = validateAcceptanceMetricsRaw(valid
    .replace('次数：1', '次数：未记录')
    .replace('次数：0', '次数：未记录'));
  assert.deepEqual(unreported, []);

  const postExceedsTotal = validateAcceptanceMetricsRaw(valid.replace('后异常次数：0', '后异常次数：2'));
  assert.match(postExceedsTotal.join('\n'), /cannot exceed total process incidents/);
  const partial = validateAcceptanceMetricsRaw(valid.replace('后异常次数：0', '后异常次数：未记录'));
  assert.match(partial.join('\n'), /reported or unreported together/);
  for (const invalid of ['-1', '1.5', '9007199254740992', '未知值']) {
    const result = validateAcceptanceMetricsRaw(valid.replace('拦截次数：1', '拦截次数：' + invalid));
    assert.match(result.join('\n'), /non-negative integer/, invalid);
  }

  const duplicate = validateAcceptanceMetricsRaw(valid.replace(
    PROCESS_BLOCK_END,
    '- 已记录发布流程异常或无效证据拦截次数：0\n' + PROCESS_BLOCK_END,
  ));
  assert.match(duplicate.join('\n'), /exactly two rows|duplicate structured field/);
});

test('process incident details are closed, count-consistent, and required from v0.90.2', () => {
  const acceptanceRows = [
    ACCEPTANCE_BLOCK_START,
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
    ACCEPTANCE_BLOCK_END,
  ];
  const processRows = [
    PROCESS_BLOCK_START,
    '- 已记录发布流程异常或无效证据拦截次数：2',
    '- 其中生产写操作开始后异常次数：0',
    PROCESS_BLOCK_END,
  ];
  const incident = {
    fingerprint: 'l3-bundle/release-gate/missing-base-tag',
    position: 'before-production-write',
    count: 2,
    impact: 'L3 stopped before candidate execution',
    recoveryEvidence: 'bundle verification and base-tag check passed after rebuild',
    permanentAction: 'release workflow now includes all stable tags and verifies the rollback tag',
    historicalReleases: ['v0.89.1'],
  };
  const incidentRows = [
    PROCESS_INCIDENTS_BLOCK_START,
    JSON.stringify([incident], null, 2),
    PROCESS_INCIDENTS_BLOCK_END,
  ];
  const base = ['# KPanel v0.90.2 发布验收记录', ...acceptanceRows, ...processRows];
  const valid = [...base, ...incidentRows].join('\n');

  assert.deepEqual(validateAcceptanceMetricsRaw(valid), []);
  assert.deepEqual(extractAcceptanceMetricsRaw(valid).processIncidents, [incident]);
  assert.match(validateAcceptanceMetricsRaw(base.join('\n')).join('\n'), /requires release-process-incidents evidence/);

  const oldRecord = ['# KPanel v0.90.1 发布验收记录', ...acceptanceRows, ...processRows].join('\n');
  assert.deepEqual(validateAcceptanceMetricsRaw(oldRecord), []);

  const zero = [
    '# KPanel v0.90.2 发布验收记录',
    ...acceptanceRows,
    PROCESS_BLOCK_START,
    '- 已记录发布流程异常或无效证据拦截次数：0',
    '- 其中生产写操作开始后异常次数：0',
    PROCESS_BLOCK_END,
    PROCESS_INCIDENTS_BLOCK_START,
    '[]',
    PROCESS_INCIDENTS_BLOCK_END,
  ].join('\n');
  assert.deepEqual(validateAcceptanceMetricsRaw(zero), []);
  const unreported = zero
    .replace('拦截次数：0', '拦截次数：未记录')
    .replace('后异常次数：0', '后异常次数：未记录');
  assert.deepEqual(validateAcceptanceMetricsRaw(unreported), []);

  const wrongTotal = valid.replace('"count": 2', '"count": 1');
  assert.match(validateAcceptanceMetricsRaw(wrongTotal).join('\n'), /counts must equal the reported process incident total/);
  const wrongPosition = valid.replace('"position": "before-production-write"', '"position": "after-production-write"');
  assert.match(validateAcceptanceMetricsRaw(wrongPosition).join('\n'), /after-production-write incident counts must equal/);
  const invalidFingerprint = valid.replace(
    'l3-bundle/release-gate/missing-base-tag',
    'L3 bundle / release gate / missing tag',
  );
  assert.match(validateAcceptanceMetricsRaw(invalidFingerprint).join('\n'), /fingerprint must use/);
  const missingEvidence = valid.replace(
    '"permanentAction": "release workflow now includes all stable tags and verifies the rollback tag",\n',
    '',
  );
  assert.match(validateAcceptanceMetricsRaw(missingEvidence).join('\n'), /exactly the canonical fields/);
  const placeholderEvidence = valid.replace(
    'release workflow now includes all stable tags and verifies the rollback tag',
    '<script, runner, fixture, or preflight change>',
  );
  assert.match(validateAcceptanceMetricsRaw(placeholderEvidence).join('\n'), /must contain explicit evidence/);
  const malformedJson = valid.replace('"count": 2', '"count":');
  assert.match(validateAcceptanceMetricsRaw(malformedJson).join('\n'), /must contain valid JSON/);
});

test('repeated process incident fingerprints are detected across the rolling five releases', () => {
  const records = [
    incidentRecord('v0.98.1', ['release-operator/oci-inspect/remote-quoting']),
    incidentRecord('v0.98.0', ['release-operator/oci-inspect/remote-quoting']),
  ];
  const findings = detectRepeatedProcessIncidents(records);
  assert.equal(findings.length, 1);
  assert.deepEqual(findings[0].repeatedIn, ['v0.98.0']);
  assert.deepEqual(findings[0].undeclared, ['v0.98.0']);
  assert.deepEqual(validateProcessIncidentHistory(records), [],
    'records before v0.100.1 keep their frozen evidence and are not rewritten retroactively');

  const current = [
    incidentRecord('v0.100.1', ['release-operator/oci-inspect/remote-quoting']),
    incidentRecord('v0.100.0', ['release-operator/oci-inspect/remote-quoting']),
  ];
  assert.match(validateProcessIncidentHistory(current).join('\n'), /historicalReleases must declare v0\.100\.0/);

  const declared = [
    incidentRecord('v0.100.1', [{
      fingerprint: 'release-operator/oci-inspect/remote-quoting',
      historicalReleases: ['v0.100.0'],
    }]),
    incidentRecord('v0.100.0', ['release-operator/oci-inspect/remote-quoting']),
  ];
  assert.equal(detectRepeatedProcessIncidents(declared)[0].undeclared.length, 0);
  assert.deepEqual(validateProcessIncidentHistory(declared), []);

  const outsideWindow = [
    incidentRecord('v0.100.5', ['release-operator/oci-inspect/remote-quoting']),
    incidentRecord('v0.100.4', []),
    incidentRecord('v0.100.3', []),
    incidentRecord('v0.100.2', []),
    incidentRecord('v0.100.1', []),
    incidentRecord('v0.100.0', ['release-operator/oci-inspect/remote-quoting']),
  ];
  assert.deepEqual(detectRepeatedProcessIncidents(outsideWindow), []);
  assert.deepEqual(validateProcessIncidentHistory(outsideWindow), []);

  const unreported = [
    incidentRecord('v0.100.1', ['release-operator/oci-inspect/remote-quoting'], { reported: false }),
    incidentRecord('v0.100.0', ['release-operator/oci-inspect/remote-quoting']),
  ];
  assert.equal(detectRepeatedProcessIncidents(unreported).length, 1);
  assert.deepEqual(validateProcessIncidentHistory(unreported), [],
    'only the record under validation is blocked; siblings are compared as evidence only');

  const malformed = [
    incidentRecord('v0.100.1', ['Release Operator / OCI Inspect']),
    incidentRecord('v0.100.0', ['Release Operator / OCI Inspect']),
  ];
  assert.deepEqual(detectRepeatedProcessIncidents(malformed), [],
    'structure errors are reported by the existing field validation, not by repeat detection');
});

test('readAcceptanceHistory compares only existing sibling records and blocks undeclared repeats', () => {
  const directory = mkdtempSync(resolve(tmpdir(), 'kpanel-acceptance-history-'));
  const write = (tag, incidents) =>
    writeFileSync(resolve(directory, 'release-' + tag + '-acceptance.md'), acceptanceDocument(tag, incidents));
  const incident = (historicalReleases) => ({
    fingerprint: 'release-operator/oci-inspect/remote-quoting',
    position: 'before-production-write',
    count: 1,
    impact: 'remote OCI inspect returned an unusable payload',
    recoveryEvidence: 'run log shows the fixed entry returned the digest',
    permanentAction: 'quoting fixed in the single repository release script plus regression',
    historicalReleases,
  });

  write('v0.100.0', [incident([])]);
  write('v0.100.1', [incident([])]);
  const undeclared = resolve(directory, 'release-v0.100.1-acceptance.md');
  const history = readAcceptanceHistory(undeclared);
  assert.deepEqual(history.map((record) => record.tag), ['v0.100.1', 'v0.100.0']);
  assert.deepEqual(history.map((record) => record.reported), [true, false]);
  assert.match(validateProcessIncidentHistory(history).join('\n'), /must declare v0\.100\.0/);

  write('v0.100.1', [incident(['v0.100.0'])]);
  assert.deepEqual(validateProcessIncidentHistory(readAcceptanceHistory(undeclared)), []);

  assert.deepEqual(readAcceptanceHistory(resolve(directory, 'release-acceptance-template.md')), []);
  assert.deepEqual(readAcceptanceHistory(resolve(directory, 'release-v0.100.9-acceptance.md')), []);
});

function rollbackPatchFixture() {
  // v1.16.0 shipped, was rolled back, then v1.15.1 shipped on the older line and repeated its incidents.
  const directory = mkdtempSync(resolve(tmpdir(), 'kpanel-acceptance-order-'));
  const incident = {
    fingerprint: 'release-ci/github-api/rate-limit',
    position: 'before-production-write',
    count: 1,
    impact: 'release run waited on the GitHub API rate limit',
    recoveryEvidence: 'retried run log shows the job finished',
    permanentAction: 'authenticated API calls in the single release entry',
    historicalReleases: [],
  };
  for (const tag of ['v1.15.0', 'v1.16.0', 'v1.15.1']) {
    writeFileSync(resolve(directory, 'release-' + tag + '-acceptance.md'),
      acceptanceDocument(tag, tag === 'v1.15.0' ? [] : [incident]));
  }
  const times = new Map([
    ['v1.15.0', Date.parse('2026-09-12T10:00:00+08:00')],
    ['v1.16.0', Date.parse('2026-09-13T14:36:49+08:00')],
    ['v1.15.1', Date.parse('2026-09-13T16:36:54+08:00')],
  ]);
  return { directory, times, path: (tag) => resolve(directory, 'release-' + tag + '-acceptance.md') };
}

test('readAcceptanceHistory orders the window by release time, not version', () => {
  const { times, path } = rollbackPatchFixture();

  const newer = readAcceptanceHistory(path('v1.16.0'), 5, times);
  assert.deepEqual(newer.map((record) => record.tag), ['v1.16.0', 'v1.15.0']);
  assert.deepEqual(validateProcessIncidentHistory(newer), []);

  const patch = readAcceptanceHistory(path('v1.15.1'), 5, times);
  assert.deepEqual(patch.map((record) => record.tag), ['v1.15.1', 'v1.16.0', 'v1.15.0']);
  assert.match(validateProcessIncidentHistory(patch).join('\n'), /must declare v1\.16\.0/);
});

test('readAcceptanceHistory treats an untagged target as newest and never loosens without times', () => {
  const { times, path } = rollbackPatchFixture();

  const untagged = new Map(times);
  untagged.delete('v1.15.1');
  assert.deepEqual(readAcceptanceHistory(path('v1.15.1'), 5, untagged).map((record) => record.tag),
    ['v1.15.1', 'v1.16.0', 'v1.15.0']);

  const gap = new Map(times);
  gap.delete('v1.15.0');
  assert.deepEqual(readAcceptanceHistory(path('v1.16.0'), 5, gap).map((record) => record.tag),
    ['v1.16.0', 'v1.15.1', 'v1.15.0']);
  assert.deepEqual(readAcceptanceHistory(path('v1.16.0'), 5, null).map((record) => record.tag),
    ['v1.16.0', 'v1.15.1', 'v1.15.0']);
});

test('readAcceptanceHistory reads release times from repository tags', () => {
  const { directory, path } = rollbackPatchFixture();
  const git = (args, date) => execFileSync('git', ['-C', directory, '-c', 'commit.gpgSign=false', '-c', 'tag.gpgSign=false', ...args], {
    encoding: 'utf8',
    env: { ...process.env, GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date,
      GIT_AUTHOR_NAME: 'KPanel Test', GIT_AUTHOR_EMAIL: 'kpanel-test@example.invalid',
      GIT_COMMITTER_NAME: 'KPanel Test', GIT_COMMITTER_EMAIL: 'kpanel-test@example.invalid' },
  });
  git(['init', '--quiet'], '2026-09-12T09:00:00+08:00');
  git(['add', '.'], '2026-09-12T09:00:00+08:00');
  git(['commit', '--quiet', '-m', 'records'], '2026-09-12T09:00:00+08:00');
  git(['tag', '-a', 'v1.15.0', '-m', 'v1.15.0'], '2026-09-12T10:00:00+08:00');
  git(['tag', '-a', 'v1.16.0', '-m', 'v1.16.0'], '2026-09-13T14:36:49+08:00');
  git(['tag', '-a', 'v1.15.1', '-m', 'v1.15.1'], '2026-09-13T16:36:54+08:00');

  assert.deepEqual(readAcceptanceHistory(path('v1.16.0')).map((record) => record.tag), ['v1.16.0', 'v1.15.0']);
  assert.deepEqual(readAcceptanceHistory(path('v1.15.1')).map((record) => record.tag),
    ['v1.15.1', 'v1.16.0', 'v1.15.0']);
});

test('rolling report surfaces repeated fingerprints without inferring missing records', () => {
  const repeated = (historicalReleases) => ({
    fingerprint: 'release-operator/oci-inspect/remote-quoting',
    position: 'before-production-write',
    count: 1,
    impact: 'remote OCI inspect returned an unusable payload',
    recoveryEvidence: 'run log shows the fixed entry returned the digest',
    permanentAction: 'quoting fixed in the single repository release script plus regression',
    historicalReleases,
  });
  const report = summarizeReleaseMetrics([
    release('v0.100.1', '2026-09-03T12:00:00Z', {
      metrics: { processIncidentCount: 1, postProductionProcessIncidentCount: 0, processIncidents: [repeated([])] },
    }),
    release('v0.100.0', '2026-09-02T12:00:00Z', {
      metrics: { processIncidentCount: 1, postProductionProcessIncidentCount: 0, processIncidents: [repeated([])] },
    }),
  ], { days: 14, releases: 20, now: new Date('2026-09-04T00:00:00Z') });

  assert.equal(report.recent.repeatedProcessIncidentCount, 1);
  assert.equal(report.recent.undeclaredRepeatedProcessIncidentCount, 1);
  assert.deepEqual(report.repeatedProcessIncidents[0].repeatedIn, ['v0.100.0']);
  const output = renderMarkdown(report);
  assert.match(output, /滚动 5 个版本内重复的流程异常指纹数 \| 1/);
  assert.match(output, /release-operator\/oci-inspect\/remote-quoting/);
  assert.match(output, /缺失记录不推断为无重复/);

  const missing = summarizeReleaseMetrics([
    release('v0.100.1', '2026-09-03T12:00:00Z', { exists: false }),
  ], { days: 14, releases: 20, now: new Date('2026-09-04T00:00:00Z') });
  assert.equal(missing.recent.repeatedProcessIncidentCount, 0);
  assert.match(renderMarkdown(missing), /\| 无 \| 无 \| 无 \| 无 \|/);
});

test('argument parser rejects invalid windows', () => {
  assert.throws(() => parseArguments(['--days', '0']), /positive integer/);
  assert.throws(() => parseArguments(['--format', 'csv']), /markdown or json/);
  assert.equal(parseArguments(['--ref', 'v1.2.3']).ref, 'v1.2.3');
  assert.equal(classifyChangeFailure('否（未发生）'), 'no');
  assert.equal(classifyChangeFailure('是（已回滚）'), 'yes');
  assert.equal(classifyChangeFailure(null), 'unreported');
  assert.equal(isStableReleaseTag('v1.2.3'), true);
  for (const tag of ['v1.2.3-rc.1', 'v1.2.3+build.1', 'v1.2.3-nightly', 'vfoo', 'v1', 'v1.2']) {
    assert.equal(isStableReleaseTag(tag), false, tag);
  }

  const isolated = gitEnvironment('/candidate/repo', {
    GIT_DIR: '/foreign/repo/.git',
    GIT_WORK_TREE: '/foreign/repo',
    GIT_INDEX_FILE: '/foreign/repo/.git/index',
    PATH: 'kept',
  });
  assert.equal(isolated.GIT_DIR, undefined);
  assert.equal(isolated.GIT_WORK_TREE, undefined);
  assert.equal(isolated.GIT_INDEX_FILE, undefined);
  assert.equal(isolated.PATH, 'kept');

  const sameWorkTree = gitEnvironment('/candidate/repo', {
    GIT_DIR: '/candidate/repo/.git',
    GIT_WORK_TREE: '/candidate/repo',
    GIT_INDEX_FILE: '/candidate/repo/.git/index',
  });
  assert.equal(sameWorkTree.GIT_DIR, '/candidate/repo/.git');

  const mixedMetadata = gitEnvironment('/candidate/repo', {
    GIT_DIR: '/foreign/repo/.git',
    GIT_WORK_TREE: '/candidate/repo',
    GIT_INDEX_FILE: '/foreign/repo/.git/index',
    GIT_COMMON_DIR: '/foreign/repo/.git',
    PATH: 'kept',
  });
  assert.equal(mixedMetadata.GIT_DIR, undefined);
  assert.equal(mixedMetadata.GIT_WORK_TREE, undefined);
  assert.equal(mixedMetadata.GIT_INDEX_FILE, undefined);
  assert.equal(mixedMetadata.GIT_COMMON_DIR, undefined);
  assert.equal(mixedMetadata.PATH, 'kept');

  const relativeMetadata = gitEnvironment(resolve('candidate/repo'), {
    GIT_DIR: 'candidate/repo/.git',
    GIT_WORK_TREE: 'candidate/repo',
    GIT_INDEX_FILE: 'candidate/repo/.git/index',
  });
  assert.equal(relativeMetadata.GIT_DIR, resolve('candidate/repo/.git'));
  assert.equal(relativeMetadata.GIT_WORK_TREE, resolve('candidate/repo'));
  assert.equal(relativeMetadata.GIT_INDEX_FILE, resolve('candidate/repo/.git/index'));
});

test('acceptance validation requires one closed machine evidence block', () => {
  const rows = [
    '- 首个纳入提交时间：2026-08-15T11:21:11+08:00',
    '- 候选冻结时间：2026-08-15T11:26:43+08:00',
    '- 生产完成时间：2026-08-15T12:00:39+08:00',
    '- 提交到生产用时：0.66 小时',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
  ];
  const valid = ['## 交付节奏数据', ACCEPTANCE_BLOCK_START, ...rows, ACCEPTANCE_BLOCK_END].join('\n');
  assert.deepEqual(validateAcceptanceMetricsRaw(valid), []);

  const missingMarkers = validateAcceptanceMetricsRaw(['## 交付节奏数据', ...rows].join('\n'));
  assert.match(missingMarkers.join('\n'), /exactly one ordered release-metrics marker pair/);

  const reversedMarkers = validateAcceptanceMetricsRaw([
    ACCEPTANCE_BLOCK_END, ...rows, ACCEPTANCE_BLOCK_START,
  ].join('\n'));
  assert.match(reversedMarkers.join('\n'), /exactly one ordered release-metrics marker pair/);

  const duplicateMarkers = validateAcceptanceMetricsRaw([
    ACCEPTANCE_BLOCK_START, ...rows, ACCEPTANCE_BLOCK_END, ACCEPTANCE_BLOCK_START,
  ].join('\n'));
  assert.match(duplicateMarkers.join('\n'), /exactly one ordered release-metrics marker pair/);

  const extraRow = validateAcceptanceMetricsRaw(valid.replace(
    ACCEPTANCE_BLOCK_END,
    '- 额外字段：不得进入机器区块\n' + ACCEPTANCE_BLOCK_END,
  ));
  assert.match(extraRow.join('\n'), /exactly six rows|unknown field/);

  const blankValues = validateAcceptanceMetricsRaw([
    ACCEPTANCE_BLOCK_START,
    ...ACCEPTANCE_FIELD_NAMES.map((field) => '- ' + field + '：   '),
    ACCEPTANCE_BLOCK_END,
  ].join('\n'));
  assert.match(blankValues.join('\n'), /values must be explicit and non-blank/);

  const duplicateField = validateAcceptanceMetricsRaw(valid.replace(
    rows[4],
    rows[4] + '\n- 是否回滚、紧急热修复或重复发布：是（已回滚）',
  ));
  assert.match(duplicateField.join('\n'), /duplicate structured field/);

  const reorderedRows = validateAcceptanceMetricsRaw([
    ACCEPTANCE_BLOCK_START, rows[1], rows[0], ...rows.slice(2), ACCEPTANCE_BLOCK_END,
  ].join('\n'));
  assert.match(reorderedRows.join('\n'), /canonical order/);

  const malformedRow = validateAcceptanceMetricsRaw(valid.replace(rows[4], '* 是否回滚、紧急热修复或重复发布：否'));
  assert.match(malformedRow.join('\n'), /must use "- 字段：值" syntax/);

  const equalsSeparator = validateAcceptanceMetricsRaw(valid.replace(rows[4], '- 是否回滚、紧急热修复或重复发布=否'));
  assert.match(equalsSeparator.join('\n'), /must use "- 字段：值" syntax/);

  const asciiColon = validateAcceptanceMetricsRaw(valid.replace(rows[4], '- 是否回滚、紧急热修复或重复发布:否'));
  assert.match(asciiColon.join('\n'), /must use "- 字段：值" syntax/);

  const hiddenMarkdownControl = validateAcceptanceMetricsRaw(valid.replace(rows[4], rows[4] + ' <!-- 示例 -->'));
  assert.match(hiddenMarkdownControl.join('\n'), /plain text without Markdown code or HTML comment controls/);

  for (const control of ['否（｀control｀）', '否（＜！－－ forged －－＞）']) {
    const normalizedControl = validateAcceptanceMetricsRaw(valid.replace('：否', '：' + control));
    assert.match(normalizedControl.join('\n'), /plain text without Markdown code or HTML comment controls/, control);
  }

  const invisibleField = validateAcceptanceMetricsRaw(valid.replace('重复发布', '重复\u200b发布'));
  assert.match(invisibleField.join('\n'), /default-ignorable characters/);

  const outsideMarkdownIsNonAuthoritative = [
    '`unclosed',
    '<script>const sample = "是否回滚、紧急热修复或重复发布：是";</script>',
    '***',
    valid,
    '```text',
    '- 是否回滚、紧急热修复或重复发布：是（示例）',
    '```',
  ].join('\n');
  assert.deepEqual(validateAcceptanceMetricsRaw(outsideMarkdownIsNonAuthoritative), []);

  const inconsistent = validateAcceptanceMetricsRaw(valid.replace('0.66 小时', '2.00 小时'));
  assert.match(inconsistent.join('\n'), /does not match/);

  const looseDate = validateAcceptanceMetricsRaw(valid.replace(
    '2026-08-15T11:21:11+08:00',
    'August 15, 2026 11:21:11 GMT+0800',
  ));
  assert.match(looseDate.join('\n'), /must be an ISO timestamp/);

  const impossibleDate = validateAcceptanceMetricsRaw(valid.replace(
    '2026-08-15T11:21:11+08:00',
    '2026-02-30T11:21:11+08:00',
  ));
  assert.match(impossibleDate.join('\n'), /must be an ISO timestamp/);
});
test('acceptance validation permits explicit non-production evidence without inventing success', () => {
  const errors = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：2026-08-15T11:21:11+08:00',
    '- 候选冻结时间：2026-08-15T11:26:43+08:00',
    '- 生产完成时间：未验证',
    '- 提交到生产用时：未验证',
    '- 是否回滚、紧急热修复或重复发布：未验证',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：未验证',
  ].join('\n'));
  assert.deepEqual(errors, []);

  const historicalUnknown = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：未记录',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：未记录',
  ].join('\n'));
  assert.deepEqual(historicalUnknown, []);

  const successWithRecovery = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：未逃逸：发布后健康检查阻断并回滚',
  ].join('\n'));
  assert.match(successWithRecovery.join('\n'), /require an explicit failed change state/);
});

test('acceptance validation keeps known failure state when historical completion time is missing', () => {
  const knownSuccess = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：2026-08-15T14:04:08+08:00',
    '- 候选冻结时间：2026-08-15T14:37:23+08:00',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
  ].join('\n'));
  assert.deepEqual(knownSuccess, []);

  const failedWithoutRecovery = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：已回滚',
  ].join('\n'));
  assert.match(failedWithoutRecovery.join('\n'), /requires discovery, recovery/);

  const failedWithDetails = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：已逃逸：候选冻结后缺少回归',
  ].join('\n'));
  assert.deepEqual(failedWithDetails, []);

  const caughtByGate = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：未逃逸：发布后健康检查阻断并回滚',
  ].join('\n'));
  assert.deepEqual(caughtByGate, []);

  const unknownDetails = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：不知道；恢复时间：稍后；逃逸门禁：未知',
  ].join('\n'));
  assert.match(unknownDetails.join('\n'), /requires discovery, recovery/);

  const reversedRecovery = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T13:00:00+08:00；恢复时间：2026-08-15T12:30:00+08:00；逃逸门禁：候选冻结后缺少回归',
  ].join('\n'));
  assert.match(reversedRecovery.join('\n'), /requires discovery, recovery/);

  for (const placeholder of ['待确认', '待分析', '待进一步调查', '无', '无缺口', 'TBD', 'TODO', 'none', 'null', 'unknown', 'N/A', '<具体缺口>']) {
    const placeholderGate = validateAcceptanceMetrics([
      '## 交付节奏数据',
      '- 首个纳入提交时间：未记录',
      '- 候选冻结时间：未记录',
      '- 生产完成时间：未记录',
      '- 提交到生产用时：未记录',
      '- 是否回滚、紧急热修复或重复发布：是',
      '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：已逃逸：' + placeholder,
    ].join('\n'));
    assert.match(placeholderGate.join('\n'), /requires discovery, recovery/, placeholder);
  }

  const missingGateState = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：候选冻结后缺少回归',
  ].join('\n'));
  assert.match(missingGateState.join('\n'), /requires discovery, recovery/);

  const adversarialGateValues = [
    '已逃逸：<遗漏门禁和原因> / 未逃逸：<实际拦截门禁>',
    '已逃逸：未逃逸：发布后健康检查阻断',
    '已逃逸：候选冻结后缺少回归；逃逸门禁：未逃逸：发布后健康检查阻断',
    '已逃逸：候选冻结后缺少回归；待进一步调查',
    '已逃逸：T B D',
    '未逃逸：N / A',
    '已逃逸：仍待确认',
    '已逃逸：<请填写具体缺口>',
    '已逃逸：Ｔ Ｂ Ｄ',
    '未逃逸：Ｎ ／ Ａ',
    '已逃逸：T\u200bB\u200bD',
    '已逃逸：仍待\u200b确认',
    '已逃逸：〈具体缺口〉',
    '已逃逸：T\uFE0FB\uFE0FD',
    '已逃逸：T\u034FB\u034FD',
    '已逃逸：T\u180BB\u180BD',
    '已逃逸：仍待\uFE0F确认',
  ];
  for (const gate of adversarialGateValues) {
    const adversarial = validateAcceptanceMetrics([
      '## 交付节奏数据',
      '- 首个纳入提交时间：未记录',
      '- 候选冻结时间：未记录',
      '- 生产完成时间：未记录',
      '- 提交到生产用时：未记录',
      '- 是否回滚、紧急热修复或重复发布：是',
      '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：' + gate,
    ].join('\n'));
    assert.match(adversarial.join('\n'), /requires discovery, recovery|default-ignorable characters/, gate);
  }

  const equalsSeparators = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间=2026-08-15T10:05:00+08:00；恢复时间=2026-08-15T10:20:00+08:00；逃逸门禁=已逃逸：候选冻结后缺少回归',
  ].join('\n'));
  assert.match(equalsSeparators.join('\n'), /requires discovery, recovery/);

  const duplicateRecovery = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：已回滚',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：已逃逸：候选冻结后缺少回归',
  ].join('\n'));
  assert.match(duplicateRecovery.join('\n'), /duplicate structured field/);

  const hiddenDuplicateKey = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 是否回滚、紧急热修复或重复\u200b发布：否',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-08-15T10:05:00+08:00；恢复时间：2026-08-15T10:20:00+08:00；逃逸门禁：已逃逸：候选冻结后缺少回归',
  ].join('\n'));
  assert.match(hiddenDuplicateKey.join('\n'), /default-ignorable characters/);

  for (const separator of ['=', '＝', '﹕', '∶']) {
    const hiddenConflictingField = validateAcceptanceMetrics([
      '## 交付节奏数据',
      '- 首个纳入提交时间：未记录',
      '- 候选冻结时间：未记录',
      '- 生产完成时间：未记录',
      '- 提交到生产用时：未记录',
      '- 是否回滚、紧急热修复或重复发布：否',
      '- 是否回滚、紧急热修复或重复发布' + separator + '是（已回滚）',
      '- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用',
    ].join('\n'));
    assert.match(hiddenConflictingField.join('\n'), /must use "- 字段：值" syntax/, separator);
  }

  const keywordsWithoutStructure = validateAcceptanceMetrics([
    '## 交付节奏数据',
    '- 首个纳入提交时间：未记录',
    '- 候选冻结时间：未记录',
    '- 生产完成时间：未记录',
    '- 提交到生产用时：未记录',
    '- 是否回滚、紧急热修复或重复发布：是',
    '- 若发生失败，发现时间、恢复时间和逃逸门禁：已发现并恢复，复查逃逸门禁',
  ].join('\n'));
  assert.match(keywordsWithoutStructure.join('\n'), /requires discovery, recovery/);
});

test('preview tags are classified apart from stable tags', () => {
  assert.equal(isPreviewReleaseTag('v1.21.0-rc.4'), true);
  assert.equal(isPreviewReleaseTag('v1.21.0'), false);
  assert.equal(isStableReleaseTag('v1.21.0-rc.4'), false);
  assert.equal(previewReleaseTrain('v1.21.0-rc.4'), 'v1.21.0');
  assert.equal(previewReleaseTrain('v1.21.0'), null);
  // A malformed suffix must not silently join a train it does not belong to.
  assert.equal(isPreviewReleaseTag('v1.21.0-rc'), false);
  assert.equal(isPreviewReleaseTag('v1.21.0-beta.1'), false);
});

test('preview metrics aggregate per release train without touching stable numbers', () => {
  const options = { days: 14, releases: 20, now: new Date('2026-09-20T13:00:00Z') };
  const preview = summarizePreviewMetrics([
    release('v1.21.0-rc.2', '2026-09-20T00:12:02Z', {
      metrics: {
        processIncidentCount: 4,
        postProductionProcessIncidentCount: 0,
        processIncidents: [{ fingerprint: 'release-monitor/github-api/anonymous-rate-limit', count: 1 }],
      },
    }),
    release('v1.21.0-rc.1', '2026-09-19T18:09:04Z', {
      metrics: {
        processIncidentCount: 2,
        postProductionProcessIncidentCount: 0,
        processIncidents: [{ fingerprint: 'release-monitor/github-api/anonymous-rate-limit', count: 1 }],
      },
    }),
    release('v1.20.0-rc.1', '2026-09-18T07:30:32Z', {
      metrics: { processIncidentCount: 3, postProductionProcessIncidentCount: 0, processIncidents: [] },
    }),
  ], options, new Set(['v1.20.0']));

  assert.equal(preview.available, 3);
  assert.equal(preview.processIncidentReported, 3);
  assert.equal(preview.processIncidentCount, 9);
  assert.equal(preview.postProductionProcessIncidentCount, 0);
  assert.equal(preview.repeatedProcessIncidentCount, 1);
  assert.equal(preview.repeatedProcessIncidents[0].tag, 'v1.21.0-rc.2');
  assert.deepEqual(preview.repeatedProcessIncidents[0].repeatedIn, ['v1.21.0-rc.1']);

  assert.equal(preview.trains.length, 2);
  const [inFlight, shipped] = preview.trains;
  assert.equal(inFlight.train, 'v1.21.0');
  assert.equal(inFlight.inFlight, true);
  assert.equal(inFlight.previewCount, 2);
  assert.equal(inFlight.processIncidentCount, 6);
  assert.equal(inFlight.repeatedProcessIncidentCount, 1);
  assert.equal(shipped.train, 'v1.20.0');
  assert.equal(shipped.inFlight, false);
  assert.equal(shipped.processIncidentCount, 3);
  assert.equal(shipped.repeatedProcessIncidentCount, 0);
});

test('preview metrics never infer missing RC evidence as zero', () => {
  const options = { days: 14, releases: 20, now: new Date('2026-09-20T13:00:00Z') };
  const preview = summarizePreviewMetrics([
    release('v1.21.0-rc.2', '2026-09-20T00:12:02Z', { exists: false }),
    release('v1.21.0-rc.1', '2026-09-19T18:09:04Z', {
      metrics: { processIncidentCount: 2, postProductionProcessIncidentCount: 0 },
    }),
  ], options, new Set());

  assert.equal(preview.available, 2);
  assert.equal(preview.acceptanceCount, 1);
  assert.equal(preview.acceptanceCoverage, 0.5);
  // The unreported RC leaves the denominator at 1 instead of contributing a zero.
  assert.equal(preview.processIncidentReported, 1);
  assert.equal(preview.processIncidentCount, 2);
  assert.equal(preview.trains[0].processIncidentReported, 1);

  const empty = summarizePreviewMetrics([], options, new Set());
  assert.equal(empty.available, 0);
  assert.equal(empty.acceptanceCoverage, null);
  assert.equal(empty.processIncidentCount, null);
  assert.equal(empty.postProductionProcessIncidentCount, null);
  assert.deepEqual(empty.trains, []);
});

test('markdown keeps the preview view separate from the stable view', () => {
  const options = { days: 14, releases: 20, now: new Date('2026-09-20T13:00:00Z') };
  const report = summarizeReleaseMetrics([
    release('v1.20.0', '2026-09-19T09:44:35Z', {
      metrics: { processIncidentCount: 1, postProductionProcessIncidentCount: 0 },
    }),
  ], options);
  const withoutPreview = renderMarkdown(report);
  assert.equal(withoutPreview.includes('## 预览通道（RC）流程异常'), false);

  report.preview = summarizePreviewMetrics([
    release('v1.21.0-rc.1', '2026-09-19T18:09:04Z', {
      metrics: { processIncidentCount: 2, postProductionProcessIncidentCount: 0 },
    }),
  ], options, new Set(['v1.20.0']));
  const output = renderMarkdown(report);

  assert.match(output, /## 预览通道（RC）流程异常/);
  assert.match(output, /RC 流程异常合计 \| 2/);
  assert.match(output, /v1\.21\.0（进行中）/);
  assert.match(output, /本节只统计预览通道，独立于上述正式版本指标/);
  assert.match(output, /RC 重复指纹只作观察/);
  // The stable rows and their note keep their existing meaning.
  assert.match(output, /已记录发布流程异常\/无效证据拦截总数 \| 1/);
  assert.match(output, /正式发布频率按稳定标签时间统计/);
  assert.equal(output.indexOf('正式发布频率按稳定标签时间统计') < output.indexOf('## 预览通道（RC）流程异常'), true);
  assert.equal(report.recent.processIncidentCount, 1);
});

test('preview repeats may span trains and are attributed to the newer RC', () => {
  const options = { days: 14, releases: 20, now: new Date('2026-09-20T13:00:00Z') };
  const shared = [{ fingerprint: 'release-monitor/github-api/anonymous-rate-limit', count: 1 }];
  const preview = summarizePreviewMetrics([
    release('v1.21.0-rc.1', '2026-09-19T18:09:04Z', {
      metrics: { processIncidentCount: 1, postProductionProcessIncidentCount: 0, processIncidents: shared },
    }),
    release('v1.20.0-rc.3', '2026-09-19T06:34:39Z', {
      metrics: { processIncidentCount: 1, postProductionProcessIncidentCount: 0, processIncidents: shared },
    }),
  ], options, new Set(['v1.20.0']));

  assert.equal(preview.repeatedProcessIncidentCount, 1);
  assert.equal(preview.repeatedProcessIncidents[0].tag, 'v1.21.0-rc.1');
  assert.deepEqual(preview.repeatedProcessIncidents[0].repeatedIn, ['v1.20.0-rc.3']);
  // The finding belongs to the train of the newer RC; the matched RC stays in the earlier train.
  const [newer, older] = preview.trains;
  assert.equal(newer.train, 'v1.21.0');
  assert.equal(newer.repeatedProcessIncidentCount, 1);
  assert.equal(older.train, 'v1.20.0');
  assert.equal(older.repeatedProcessIncidentCount, 0);
  // Per-train counts still sum to the reported total, so the attribution loses nothing.
  assert.equal(preview.trains.reduce((total, train) => total + train.repeatedProcessIncidentCount, 0),
    preview.repeatedProcessIncidentCount);

  const output = renderMarkdown({ ...summarizeReleaseMetrics([], options), preview });
  assert.match(output, /重复指纹（发生于本列车）/);
  assert.match(output, /滚动窗口按 RC 标签时间排序，可以跨发布列车/);
});

test('summarizePreviewMetrics refuses to guess whether a train already shipped', () => {
  const options = { days: 14, releases: 20, now: new Date('2026-09-20T13:00:00Z') };
  assert.throws(() => summarizePreviewMetrics([
    release('v1.21.0-rc.1', '2026-09-19T18:09:04Z'),
  ], options), TypeError);
});

test('stable records from v1.21.0 must state the coverage-check decision and how a pending one was resolved', () => {
  const record = (line) => acceptanceDocument('v1.21.0', []) + (line === null ? '' : '\n' + line) + '\n';
  const stable = 'docs/release-v1.21.0-acceptance.md';
  const errors = (line, label = stable) => validateAcceptanceMetricsRaw(record(line), label).join('\n');
  assert.match(errors(null), /must record the 覆盖检查 decision/);
  assert.match(errors('- 覆盖检查：'), /must record the 覆盖检查 decision/);
  assert.match(errors('- 覆盖检查：已运行'), /must name the decision/);
  assert.equal(errors('- 覆盖检查（decision、未审计提交数）：decision=ok，未审计 0'), '');
  assert.match(errors('- 覆盖检查：decision=scoped-required'), /must name the completed run-<N> or the user waiver/);
  assert.equal(errors('- 覆盖检查：decision=scoped-required，补完 run-7 后重跑为 ok'), '');
  assert.match(errors('- 覆盖检查：decision=full-required，用户豁免'), /user waiver/);
  assert.equal(errors('- 覆盖检查：decision=full-required；用户原话"先发"，豁免，补审截止 2026-10-05'), '');
  assert.equal(errors(null, 'docs/release-v1.21.0-rc.9-acceptance.md'), '');
  assert.equal(validateAcceptanceMetricsRaw(acceptanceDocument('v1.20.0', []) + '\n', 'docs/release-v1.20.0-acceptance.md').join('\n'), '');
});
