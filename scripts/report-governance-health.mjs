#!/usr/bin/env node

// Governance execution health (PROJECT_RULES.md 5.2.7): proposal lifecycle, review SLA and reviewer
// independence. `--validate` checks every proposal outside the frozen legacy list (legacy proposals are
// classified from their original text, never rewritten); `--strict` exits 3 while overdue or
// unclassified proposals remain. Independent-Review trailer debt is reported only: history is immutable.

import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const REVIEW_SLA_DAYS = 14;
export const MAX_DEFERRAL_DAYS = 14;
// Proposals that predate 5.2.7 (2026-09-19); every other proposal file must follow the new contract.
export const LEGACY_PROPOSALS = new Set([
  'quality-improvement-2026-08-13-background-browser-validation.md',
  'quality-improvement-2026-08-15-governance-feedback-loop.md',
  'quality-improvement-2026-08-17-governance-acceptance-contract.md',
  'quality-improvement-2026-08-17-local-feature-preview-standard.md',
  'quality-improvement-2026-08-20-execution-efficiency.md',
  'quality-improvement-2026-08-23-governance-candidate-ci.md',
  'quality-improvement-2026-08-24-execution-compatibility.md',
  'quality-improvement-2026-09-02-release-process-repeat-fingerprint.md',
  'quality-improvement-2026-09-05-release-source.md',
  'quality-improvement-2026-09-05-standards-alignment.md',
  'quality-improvement-2026-09-15-candidate-completion.md',
  'quality-improvement-2026-09-18-ocr-line-review.md',
]);
const CANONICAL = new Map([
  ['草案', 'draft'], ['待复核', 'pending-review'], ['试行', 'trial'],
  ['已采纳', 'adopted'], ['已拒绝', 'rejected'], ['已回滚', 'rolled-back'],
]);
export const PROVIDERS = ['claude', 'codex', 'gemini', 'qwen', 'deepseek', 'copilot'];
const RESULTS = ['pass', 'pass with follow-up', 'fail'];
const DAY = 24 * 60 * 60 * 1000;

function field(text, pattern) {
  return text.match(pattern)?.[1]?.trim() ?? null;
}

function days(from, to) {
  return Math.floor((Date.parse(to + 'T00:00:00Z') - Date.parse(from + 'T00:00:00Z')) / DAY);
}

// Legacy free text: substring priority, kept only for proposals frozen before 5.2.7.
export function classifyLegacyStatus(status) {
  if (!status) return 'missing';
  if (/已拒绝/.test(status)) return 'rejected';
  if (/已回滚/.test(status)) return 'rolled-back';
  if (/待[^；;。]*复核/.test(status)) return 'pending-review';
  if (/草案/.test(status)) return 'draft';
  if (/试行/.test(status)) return 'trial';
  if (/已采纳/.test(status)) return 'adopted';
  return 'unclassified';
}

// New contract: the status starts with exactly one template value; an unedited template line is not a status.
export function classifyStatus(status) {
  if (!status) return 'missing';
  if (status.includes(' / ')) return 'unclassified';
  for (const [token, category] of CANONICAL) {
    if (status.startsWith(token)) return category;
  }
  return 'unclassified';
}

export function normalizeProvider(value) {
  const word = String(value ?? '').replace(/[（(].*?[)）]/g, ' ').trim().toLowerCase().split(/[\s\-_]+/)[0];
  return PROVIDERS.find((provider) => word === provider) ?? null;
}

export function parseProposal(name, text) {
  const legacy = LEGACY_PROPOSALS.has(name);
  const status = field(text, /^- (?:提案)?状态[：:]\s*(.+)$/m);
  const providers = field(text, /^- 复核提供商 \/ 实现提供商[：:]\s*(.+)$/m);
  const [reviewer, author] = (providers ?? '').replace(/[（(][^)）]*[)）]\s*$/, '').split('/').map(normalizeProvider);
  const deferral = text.match(/^- 复核延期至[：:]\s*(\d{4}-\d{2}-\d{2})\s*[（(](.+)[)）]\s*$/m);
  return {
    name,
    legacy,
    date: field(text, /^- 提案日期[：:]\s*(\d{4}-\d{2}-\d{2})/m),
    fileDate: name.match(/(\d{4}-\d{2}-\d{2})/)?.[1] ?? null,
    status,
    category: legacy ? classifyLegacyStatus(status) : classifyStatus(status),
    reviewPassed: /^- 复核状态[：:]\s*(?:通过|修改后通过)\s*(?:[（(。].*)?$/m.test(text),
    deferredUntil: deferral?.[1] ?? null,
    reviewer: reviewer && author ? reviewer : null,
    author: reviewer && author ? author : null,
    providerNote: providers,
  };
}

export function validateProposal(proposal) {
  if (proposal.legacy) return [];
  const failures = [];
  const where = 'docs/' + proposal.name;
  if (!proposal.date || proposal.date !== proposal.fileDate) failures.push(where + ': 提案日期 must equal the file name date');
  if (['missing', 'unclassified'].includes(proposal.category)) {
    failures.push(where + ': 提案状态 must start with one of ' + [...CANONICAL.keys()].join(' / '));
  }
  if (['trial', 'adopted'].includes(proposal.category)) {
    if (!proposal.reviewPassed) failures.push(where + ': 试行/已采纳 requires 复核状态：通过');
    if (!proposal.reviewer) {
      failures.push(where + ': 试行/已采纳 requires 复核提供商 / 实现提供商 from ' + PROVIDERS.join(', '));
    } else if (proposal.reviewer === proposal.author && !/不可用/.test(proposal.providerNote)) {
      failures.push(where + ': same-provider review must record why another provider was 不可用');
    }
  }
  return failures;
}

export function assess(proposals, today) {
  const overdue = [];
  const deferred = [];
  const unclassified = [];
  for (const proposal of proposals) {
    if (['missing', 'unclassified'].includes(proposal.category)) unclassified.push(proposal);
    const start = proposal.date ?? proposal.fileDate;
    if (proposal.category !== 'pending-review' || !start) continue;
    const pending = days(start, today);
    const deferral = proposal.deferredUntil;
    const deferralValid = deferral && deferral >= today && days(today, deferral) <= MAX_DEFERRAL_DAYS;
    if (deferral) deferred.push({ ...proposal, deferralValid });
    if (pending > REVIEW_SLA_DAYS && !deferralValid) overdue.push({ ...proposal, days: pending });
  }
  const reviewed = proposals.filter((proposal) => proposal.reviewer);
  return {
    total: proposals.length,
    byCategory: Object.fromEntries([...new Set(proposals.map((p) => p.category))].sort()
      .map((category) => [category, proposals.filter((p) => p.category === category).length])),
    pending: proposals.filter((p) => p.category === 'pending-review').length,
    overdue,
    deferred,
    unclassified,
    providerRecorded: reviewed.length,
    crossProvider: reviewed.filter((proposal) => proposal.reviewer !== proposal.author).length,
    failures: proposals.flatMap(validateProposal),
  };
}

export function parseReviewTrailer(value) {
  const pairs = Object.fromEntries([...value.matchAll(/([A-Za-z]+)\s*=\s*(.+?)(?=\s+[A-Za-z]+\s*=|$)/g)]
    .map(([, key, raw]) => [key.toLowerCase(), raw.trim()]));
  const reviewer = normalizeProvider(pairs.reviewer);
  const author = normalizeProvider(pairs.author);
  const valid = Boolean(reviewer && author && RESULTS.includes(pairs.result?.toLowerCase()));
  return {
    valid,
    cross: valid && reviewer !== author,
    unexplained: valid && reviewer === author && !pairs.fallback,
  };
}

export function assessReviewTrailers(values) {
  const reviews = values.map(parseReviewTrailer);
  return {
    total: reviews.length,
    invalid: reviews.filter((review) => !review.valid).length,
    cross: reviews.filter((review) => review.cross).length,
    unexplained: reviews.filter((review) => review.unexplained).length,
  };
}

function reviewTrailers(since) {
  return execFileSync('git', ['-C', repoRoot, 'log',
    '--format=%(trailers:key=Independent-Review,valueonly,unfold,separator=%x1e)%x1e', since + '..HEAD'],
  { encoding: 'utf8' }).split('\x1e').map((value) => value.trim()).filter(Boolean);
}

export function loadProposals(repo = repoRoot) {
  return readdirSync(join(repo, 'docs'))
    .filter((name) => /^quality-improvement-\d{4}-\d{2}-\d{2}-.+\.md$/.test(name))
    .sort()
    .map((name) => parseProposal(name, readFileSync(join(repo, 'docs', name), 'utf8')));
}

export function render(report, today) {
  return [
    'governance_health date=' + today + ' proposals=' + report.total + ' '
      + Object.entries(report.byCategory).map(([key, value]) => key + '=' + value).join(' '),
    'review_sla days=' + REVIEW_SLA_DAYS + ' pending=' + report.pending + ' overdue=' + report.overdue.length
      + ' deferred=' + report.deferred.length + ' deferred_valid=' + report.deferred.filter((p) => p.deferralValid).length,
    ...report.overdue.map((p) => '  overdue ' + p.name + ' pending ' + p.days + 'd: ' + p.status),
    'status unclassified=' + report.unclassified.length,
    ...report.unclassified.map((p) => '  unclassified ' + p.name + ': ' + (p.status ?? '<no status line>')),
    'reviewer_independence recorded=' + report.providerRecorded + ' cross_provider=' + report.crossProvider
      + (report.providerRecorded ? '' : ' (未报告)'),
  ].join('\n');
}

export function main(argv, today = new Date().toISOString().slice(0, 10)) {
  const date = argv.find((value) => value.startsWith('--today='))?.slice('--today='.length) ?? today;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(Date.parse(date))) {
    process.stderr.write('report-governance-health: --today must be YYYY-MM-DD\n');
    return 2;
  }
  const report = assess(loadProposals(), date);
  if (argv.includes('--validate')) {
    if (report.failures.length > 0) {
      process.stderr.write('Governance health validation failed:\n- ' + report.failures.join('\n- ') + '\n');
      return 1;
    }
    process.stdout.write('Governance health validation passed (' + report.total + ' proposals).\n');
    return 0;
  }
  let output = render(report, date) + '\n';
  const since = argv.find((value) => value.startsWith('--since='))?.slice('--since='.length);
  if (since) {
    const reviews = assessReviewTrailers(reviewTrailers(since));
    output += 'independent_review_trailers since=' + since + ' total=' + reviews.total + ' cross_provider='
      + reviews.cross + ' invalid=' + reviews.invalid + ' same_provider_without_fallback=' + reviews.unexplained
      + (reviews.total ? '' : ' (未报告)') + '\n';
  }
  process.stdout.write(output);
  if (report.failures.length > 0) return 1;
  if (argv.includes('--strict') && (report.overdue.length > 0 || report.unclassified.length > 0)) return 3;
  return 0;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main(process.argv.slice(2));
}
