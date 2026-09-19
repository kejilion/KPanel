#!/usr/bin/env node

// Governance execution health (PROJECT_RULES.md 5.2.7): proposal lifecycle, review SLA and reviewer
// independence. `--validate` checks proposal structure dated on or after the effective date (legacy
// proposals are never rewritten); `--strict` exits 3 while overdue or unclassified proposals remain.

import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const EFFECTIVE_DATE = '2026-09-19';
export const REVIEW_SLA_DAYS = 14;
const CANONICAL = ['草案', '待复核', '试行', '已采纳', '已拒绝', '已回滚'];
const DAY = 24 * 60 * 60 * 1000;

function field(text, pattern) {
  return text.match(pattern)?.[1]?.trim() ?? null;
}

export function classifyStatus(status) {
  if (!status) return 'missing';
  if (/已拒绝/.test(status)) return 'rejected';
  if (/已回滚/.test(status)) return 'rolled-back';
  if (/待[^；;。]*复核/.test(status)) return 'pending-review';
  if (/草案/.test(status)) return 'draft';
  if (/试行/.test(status)) return 'trial';
  if (/已采纳/.test(status)) return 'adopted';
  return 'unclassified';
}

export function parseProposal(name, text) {
  const status = field(text, /^- (?:提案)?状态[：:]\s*(.+)$/m);
  const providers = field(text, /^- 复核提供商 \/ 实现提供商[：:]\s*(.+)$/m);
  const [reviewer, author] = (providers ?? '').split(/[（(]/)[0].split('/').map((value) => value.trim().toLowerCase());
  return {
    name,
    date: field(text, /^- 提案日期[：:]\s*(\d{4}-\d{2}-\d{2})/m) ?? name.match(/(\d{4}-\d{2}-\d{2})/)?.[1] ?? null,
    status,
    category: classifyStatus(status),
    canonical: CANONICAL.some((token) => status?.startsWith(token)),
    reviewPassed: /^- 复核状态[：:]\s*(?:通过|修改后通过)/m.test(text),
    deferredUntil: field(text, /^- 复核延期至[：:]\s*(\d{4}-\d{2}-\d{2})\s*[（(].+[)）]/m),
    reviewer: providers ? reviewer || null : null,
    author: providers ? author || null : null,
    providerNote: providers,
  };
}

export function validateProposal(proposal) {
  if (!proposal.date || proposal.date < EFFECTIVE_DATE) return [];
  const failures = [];
  const where = 'docs/' + proposal.name;
  if (!proposal.canonical) failures.push(where + ': 提案状态 must start with one of ' + CANONICAL.join(' / '));
  if (['trial', 'adopted'].includes(proposal.category)) {
    if (!proposal.reviewPassed) failures.push(where + ': 试行/已采纳 requires 复核状态：通过');
    if (!proposal.reviewer || !proposal.author) {
      failures.push(where + ': 试行/已采纳 requires 复核提供商 / 实现提供商');
    } else if (proposal.reviewer === proposal.author && !/不可用/.test(proposal.providerNote)) {
      failures.push(where + ': same-provider review must record why another provider was 不可用');
    }
  }
  return failures;
}

export function assess(proposals, today) {
  const now = Date.parse(today + 'T00:00:00Z');
  const overdue = [];
  const unclassified = [];
  for (const proposal of proposals) {
    if (['missing', 'unclassified'].includes(proposal.category)) unclassified.push(proposal);
    if (proposal.category !== 'pending-review' || !proposal.date) continue;
    const days = Math.floor((now - Date.parse(proposal.date + 'T00:00:00Z')) / DAY);
    const deferred = proposal.deferredUntil && proposal.deferredUntil >= today;
    if (days > REVIEW_SLA_DAYS && !deferred) overdue.push({ ...proposal, days });
  }
  const reviewed = proposals.filter((proposal) => proposal.reviewer && proposal.author);
  return {
    total: proposals.length,
    byCategory: Object.fromEntries([...new Set(proposals.map((p) => p.category))].sort()
      .map((category) => [category, proposals.filter((p) => p.category === category).length])),
    overdue,
    unclassified,
    providerRecorded: reviewed.length,
    crossProvider: reviewed.filter((proposal) => proposal.reviewer !== proposal.author).length,
    failures: proposals.flatMap(validateProposal),
  };
}

export function parseReviewTrailer(value) {
  const pairs = Object.fromEntries([...value.matchAll(/(\w+)=([^\s]+(?:\s(?!\w+=)[^\s]+)*)/g)]
    .map(([, key, raw]) => [key, raw.trim().toLowerCase()]));
  const valid = Boolean(pairs.reviewer && pairs.author && pairs.result);
  return {
    valid,
    cross: valid && pairs.reviewer !== pairs.author,
    unexplained: valid && pairs.reviewer === pairs.author && !pairs.fallback,
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
  return execFileSync('git', ['-C', repoRoot, 'log', '--format=%(trailers:key=Independent-Review,valueonly,separator=%x1e)',
    since + '..HEAD'], { encoding: 'utf8' })
    .split(/[\n\x1e]/).map((value) => value.trim()).filter(Boolean);
}

export function loadProposals(repo = repoRoot) {
  return readdirSync(join(repo, 'docs'))
    .filter((name) => /^quality-improvement-\d{4}-\d{2}-\d{2}-.+\.md$/.test(name))
    .sort()
    .map((name) => parseProposal(name, readFileSync(join(repo, 'docs', name), 'utf8')));
}

export function render(report, today) {
  const lines = [
    'governance_health date=' + today + ' proposals=' + report.total + ' '
      + Object.entries(report.byCategory).map(([key, value]) => key + '=' + value).join(' '),
    'review_sla days=' + REVIEW_SLA_DAYS + ' overdue=' + report.overdue.length,
    ...report.overdue.map((p) => '  overdue ' + p.name + ' pending ' + p.days + 'd: ' + p.status),
    'status unclassified=' + report.unclassified.length,
    ...report.unclassified.map((p) => '  unclassified ' + p.name + ': ' + (p.status ?? '<no status line>')),
    'reviewer_independence recorded=' + report.providerRecorded + ' cross_provider=' + report.crossProvider
      + (report.providerRecorded ? '' : ' (未报告)'),
  ];
  return lines.join('\n');
}

export function main(argv, today = new Date().toISOString().slice(0, 10)) {
  const todayArg = argv.find((value) => value.startsWith('--today='));
  const date = todayArg ? todayArg.slice('--today='.length) : today;
  const report = assess(loadProposals(), date);
  if (argv.includes('--validate')) {
    if (report.failures.length > 0) {
      process.stderr.write('Governance health validation failed:\n- ' + report.failures.join('\n- ') + '\n');
      return 1;
    }
    process.stdout.write('Governance health validation passed (' + report.total + ' proposals).\n');
    return 0;
  }
  process.stdout.write(render(report, date) + '\n');
  const since = argv.find((value) => value.startsWith('--since='))?.slice('--since='.length);
  const reviews = since ? assessReviewTrailers(reviewTrailers(since)) : null;
  if (reviews) {
    process.stdout.write('independent_review_trailers since=' + since + ' total=' + reviews.total
      + ' cross_provider=' + reviews.cross + ' invalid=' + reviews.invalid
      + ' same_provider_without_fallback=' + reviews.unexplained + (reviews.total ? '' : ' (未报告)') + '\n');
  }
  if (report.failures.length > 0) return 1;
  const reviewDebt = reviews ? reviews.invalid + reviews.unexplained : 0;
  const debt = report.overdue.length + report.unclassified.length + reviewDebt;
  if (argv.includes('--strict') && debt > 0) return 3;
  return 0;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = main(process.argv.slice(2));
}
