import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';

import {
  classifyChangeFailure,
  detectRepeatedProcessIncidents,
  durationHours,
  extractAcceptanceMetrics as extractAcceptanceMetricsRaw,
  gitEnvironment,
  isStableReleaseTag,
  parseArguments,
  productionLeadHours,
  readAcceptanceHistory,
  renderMarkdown,
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
