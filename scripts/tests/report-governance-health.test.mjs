import assert from 'node:assert/strict';
import test from 'node:test';

import { assess, classifyStatus, parseProposal, validateProposal } from '../report-governance-health.mjs';

function proposal(date, lines) {
  return parseProposal('quality-improvement-' + date + '-x.md', lines.join('\n') + '\n');
}

test('legacy free-text statuses are classified without rewriting them', () => {
  assert.equal(classifyStatus('待独立复核'), 'pending-review');
  assert.equal(classifyStatus('第一阶段已采纳；第二阶段候选待独立复核与主线集成'), 'pending-review');
  assert.equal(classifyStatus('试行、CI 与独立复核已通过，待主线集成'), 'trial');
  assert.equal(classifyStatus('已拒绝'), 'rejected');
  assert.equal(classifyStatus('治理候选'), 'unclassified');
  assert.equal(classifyStatus(null), 'missing');
});

test('pending reviews older than the SLA are overdue unless explicitly deferred', () => {
  const late = proposal('2026-09-01', ['- 提案状态：待复核', '- 提案日期：2026-09-01']);
  const fresh = proposal('2026-09-10', ['- 提案状态：待复核', '- 提案日期：2026-09-10']);
  const deferred = proposal('2026-09-01', ['- 提案状态：待复核', '- 提案日期：2026-09-01', '- 复核延期至：2026-09-30（等待 Codex 复核）']);
  const expired = proposal('2026-08-01', ['- 提案状态：待复核', '- 提案日期：2026-08-01', '- 复核延期至：2026-09-10（过期）']);
  const bare = proposal('2026-09-01', ['- 提案状态：待复核', '- 提案日期：2026-09-01', '- 复核延期至：2026-09-30']);
  const report = assess([late, fresh, deferred, expired, bare], '2026-09-19');
  assert.deepEqual(report.overdue.map((p) => p.date), ['2026-09-01', '2026-08-01', '2026-09-01']);
  assert.equal(report.overdue[0].days, 18);
});

test('new proposals need a canonical status, a passed review and recorded providers before trial', () => {
  assert.deepEqual(validateProposal(proposal('2026-09-18', ['- 提案状态：随便写'])), []);
  const loose = proposal('2026-09-20', ['- 提案状态：进行中']);
  assert.ok(validateProposal(loose).some((f) => f.includes('must start with')));
  const unreviewed = proposal('2026-09-20', ['- 提案状态：试行']);
  const failures = validateProposal(unreviewed);
  assert.ok(failures.some((f) => f.includes('复核状态')));
  assert.ok(failures.some((f) => f.includes('复核提供商')));
  const sameProvider = proposal('2026-09-20', ['- 提案状态：试行', '- 复核状态：通过', '- 复核提供商 / 实现提供商：Claude / Claude']);
  assert.ok(validateProposal(sameProvider).some((f) => f.includes('不可用')));
  const fallback = proposal('2026-09-20', ['- 提案状态：试行', '- 复核状态：通过',
    '- 复核提供商 / 实现提供商：Claude / Claude（Codex CLI 不可用）']);
  assert.deepEqual(validateProposal(fallback), []);
  const cross = proposal('2026-09-20', ['- 提案状态：已采纳', '- 复核状态：通过', '- 复核提供商 / 实现提供商：Codex / Claude']);
  assert.deepEqual(validateProposal(cross), []);
  const report = assess([sameProvider, fallback, cross], '2026-09-21');
  assert.equal(report.providerRecorded, 3);
  assert.equal(report.crossProvider, 1);
});
