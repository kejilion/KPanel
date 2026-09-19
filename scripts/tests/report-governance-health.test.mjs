import assert from 'node:assert/strict';
import test from 'node:test';

import {
  assess,
  assessReviewTrailers,
  classifyLegacyStatus,
  classifyStatus,
  loadProposals,
  normalizeProvider,
  parseProposal,
  parseReviewTrailer,
  validateProposal,
} from '../report-governance-health.mjs';

function proposal(date, lines, name = 'quality-improvement-' + date + '-x.md') {
  return parseProposal(name, lines.join('\n') + '\n');
}

const TEMPLATE_STATUS = '草案 / 待复核 / 试行 / 已采纳 / 已拒绝 / 已回滚（以其中之一开头，可在括号内补充说明）';

test('legacy free-text statuses are classified without rewriting them', () => {
  assert.equal(classifyLegacyStatus('待独立复核'), 'pending-review');
  assert.equal(classifyLegacyStatus('第一阶段已采纳；第二阶段候选待独立复核与主线集成'), 'pending-review');
  assert.equal(classifyLegacyStatus('试行、CI 与独立复核已通过，待主线集成'), 'trial');
  assert.equal(classifyLegacyStatus('治理候选'), 'unclassified');
  assert.equal(classifyLegacyStatus(null), 'missing');
  const legacy = loadProposals().filter((p) => p.legacy);
  assert.equal(legacy.length, 12);
  assert.ok(legacy.every((p) => validateProposal(p).length === 0));
});

test('new statuses classify by template prefix, so substrings and unedited templates cannot game it', () => {
  assert.equal(classifyStatus('试行（待主线复核）'), 'trial');
  assert.equal(classifyStatus('已采纳（方案 A 已拒绝）'), 'adopted');
  assert.equal(classifyStatus(TEMPLATE_STATUS), 'unclassified');
  assert.equal(classifyStatus('进行中'), 'unclassified');
  const copied = proposal('2026-09-20', ['- 提案状态：' + TEMPLATE_STATUS, '- 提案日期：2026-09-20']);
  assert.ok(validateProposal(copied).some((f) => f.includes('must start with')));
});

test('pending reviews older than the SLA are overdue unless deferred within the cap', () => {
  const base = (date, extra = []) => proposal(date, ['- 提案状态：待复核', '- 提案日期：' + date, ...extra]);
  const report = assess([
    base('2026-09-01'),
    base('2026-09-10'),
    base('2026-09-01', ['- 复核延期至：2026-09-30（等待 Codex 复核）']),
    base('2026-08-01', ['- 复核延期至：2026-09-10（过期）']),
    base('2026-09-01', ['- 复核延期至：2026-09-30']),
    base('2026-09-01', ['- 复核延期至：2099-01-01（永久）']),
  ], '2026-09-19');
  assert.deepEqual(report.overdue.map((p) => p.date), ['2026-09-01', '2026-08-01', '2026-09-01', '2026-09-01']);
  assert.equal(report.overdue[0].days, 18);
  assert.equal(report.deferred.length, 3);
  assert.equal(report.deferred.filter((p) => p.deferralValid).length, 1);
});

test('new proposals cannot backdate, and template placeholders do not pass trial', () => {
  const backdated = proposal('2026-09-01', ['- 提案状态：草案', '- 提案日期：2026-09-01'], 'quality-improvement-2026-09-01-new.md');
  assert.equal(backdated.legacy, false);
  assert.deepEqual(validateProposal(backdated), []);
  const mismatch = proposal('2026-09-20', ['- 提案状态：草案', '- 提案日期：2026-09-01']);
  assert.ok(validateProposal(mismatch).some((f) => f.includes('file name date')));
  const placeholder = proposal('2026-09-20', ['- 提案状态：试行', '- 提案日期：2026-09-20',
    '- 复核提供商 / 实现提供商：<复核者> / <实现者>（同一提供商时在括号内写明另一提供商不可用的原因）',
    '- 复核状态：通过 / 修改后复核 / 拒绝']);
  const failures = validateProposal(placeholder);
  assert.ok(failures.some((f) => f.includes('复核状态')));
  assert.ok(failures.some((f) => f.includes('复核提供商')));
  assert.equal(assess([placeholder, proposal('2026-09-20', ['- 复核提供商 / 实现提供商：待填写 / Claude'])], '2026-09-21').providerRecorded, 0);
});

test('provider names are normalized to a closed vocabulary', () => {
  assert.equal(normalizeProvider('Claude Opus 5'), 'claude');
  assert.equal(normalizeProvider('claude-opus-5'), 'claude');
  assert.equal(normalizeProvider('Codex (GPT-5)'), 'codex');
  assert.equal(normalizeProvider('待填写'), null);
  const sameFamily = proposal('2026-09-20', ['- 提案状态：试行', '- 提案日期：2026-09-20', '- 复核状态：通过',
    '- 复核提供商 / 实现提供商：Claude Sonnet / Claude Opus']);
  assert.ok(validateProposal(sameFamily).some((f) => f.includes('不可用')));
  const fallback = proposal('2026-09-20', ['- 提案状态：试行', '- 提案日期：2026-09-20', '- 复核状态：通过',
    '- 复核提供商 / 实现提供商：Claude / Claude（Codex CLI 不可用）']);
  assert.deepEqual(validateProposal(fallback), []);
  const cross = proposal('2026-09-20', ['- 提案状态：已采纳', '- 提案日期：2026-09-20', '- 复核状态：修改后通过',
    '- 复核提供商 / 实现提供商：Codex (GPT-5) / Claude']);
  assert.deepEqual(validateProposal(cross), []);
  const report = assess([sameFamily, fallback, cross], '2026-09-21');
  assert.equal(report.providerRecorded, 3);
  assert.equal(report.crossProvider, 1);
});

test('independent review trailers tolerate case, spacing and multi-word results', () => {
  const report = assessReviewTrailers([
    'reviewer=codex author=claude result=PASS',
    'Reviewer = Claude Author=claude-opus-5 result=PASS WITH FOLLOW-UP fallback=codex-cli-unavailable',
    'reviewer=claude author=claude result=PASS',
    'reviewer=codex result=PASS',
    'reviewer=codex author=claude result=banana',
  ]);
  assert.deepEqual(report, { total: 5, invalid: 2, cross: 1, unexplained: 1 });
  assert.equal(parseReviewTrailer('reviewer=Codex author=claude result=FAIL').cross, true);
});
