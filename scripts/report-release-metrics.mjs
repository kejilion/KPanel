#!/usr/bin/env node

import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { loadRuns } from './check-security-audit-coverage.mjs';

const RELEASE_TAG = /^v\d+\.\d+\.\d+$/;
const PREVIEW_RELEASE_TAG = /^(v\d+\.\d+\.\d+)-rc\.\d+$/;
const EMPTY_VALUE = /^(?:|[-—]|待填写|未记录|未验证|未知|不知道|稍后|不适用|N\/A)$/i;

export function isStableReleaseTag(value) {
  return RELEASE_TAG.test(value);
}

// Preview (RC) tags are reported separately and never enter the stable metrics above: a preview
// release has no production write, so mixing it into lead time, change failure or deployment
// frequency would restate what those numbers have always meant.
export function isPreviewReleaseTag(value) {
  return PREVIEW_RELEASE_TAG.test(value);
}

export function previewReleaseTrain(value) {
  const match = PREVIEW_RELEASE_TAG.exec(value ?? '');
  return match === null ? null : match[1];
}

export function parseArguments(argv) {
  const options = {
    days: 14,
    releases: 20,
    format: 'markdown',
    repo: process.cwd(),
    ref: null,
    now: new Date(),
    acceptanceFiles: [],
  };

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    const value = argv[index + 1];
    if (argument === '--validate-acceptance') {
      if (value === undefined) throw new Error('Missing value for ' + argument);
      options.acceptanceFiles.push(value);
      index += 1;
      continue;
    }
    if (argument === '--days' || argument === '--releases' || argument === '--format' || argument === '--repo' || argument === '--ref' || argument === '--now') {
      if (value === undefined) {
        throw new Error('Missing value for ' + argument);
      }
      index += 1;
      if (argument === '--days') options.days = Number(value);
      if (argument === '--releases') options.releases = Number(value);
      if (argument === '--format') options.format = value;
      if (argument === '--repo') options.repo = value;
      if (argument === '--ref') options.ref = value;
      if (argument === '--now') options.now = new Date(value);
      continue;
    }
    if (argument === '--help' || argument === '-h') {
      options.help = true;
      continue;
    }
    throw new Error('Unknown argument: ' + argument);
  }

  if (!Number.isInteger(options.days) || options.days < 1) throw new Error('--days must be a positive integer');
  if (!Number.isInteger(options.releases) || options.releases < 1) throw new Error('--releases must be a positive integer');
  if (!['markdown', 'json'].includes(options.format)) throw new Error('--format must be markdown or json');
  if (Number.isNaN(options.now.getTime())) throw new Error('--now must be a valid date');
  options.repo = resolve(options.repo);
  options.acceptanceFiles = options.acceptanceFiles.map((path) => resolve(options.repo, path));
  return options;
}

export function gitEnvironment(repo, environment = process.env) {
  const env = { ...environment };
  const comparablePath = (value, base = process.cwd()) => {
    if (!value) return null;
    let normalized = String(value).trim().replaceAll('\\', '/');
    if (process.platform === 'win32' && normalized.startsWith('/')) {
      normalized = resolve(normalized).replaceAll('\\', '/');
    }
    if (!/^(?:[A-Za-z]:\/|\/)/.test(normalized)) {
      normalized = resolve(base, normalized).replaceAll('\\', '/');
    }
    const wslPath = normalized.match(/^\/mnt\/([A-Za-z])\/(.*)$/);
    const windowsPath = normalized.match(/^([A-Za-z]):\/(.*)$/);
    if (wslPath) normalized = wslPath[1].toLowerCase() + ':/' + wslPath[2].toLowerCase();
    if (windowsPath) normalized = windowsPath[1].toLowerCase() + ':/' + windowsPath[2].toLowerCase();
    return normalized.replace(/\/$/, '');
  };
  const dotGit = resolve(repo, '.git');
  let expectedGitDir = dotGit;
  try {
    const match = readFileSync(dotGit, 'utf8').trim().match(/^gitdir:\s*(.+)$/i);
    if (match) {
      const configured = match[1].trim();
      expectedGitDir = /^(?:[A-Za-z]:[\\/]|\/)/.test(configured)
        ? configured
        : resolve(repo, configured);
    }
  } catch {
    // A normal clone has a .git directory rather than a linked-worktree pointer file.
  }
  const expectedIndex = expectedGitDir.replace(/[\\/]$/, '') + '/index';
  const absoluteEnvironmentPath = (value) => {
    const normalized = String(value).trim().replaceAll('\\', '/');
    if (/^(?:[A-Za-z]:\/|\/)/.test(normalized)) return normalized;
    return resolve(process.cwd(), normalized);
  };
  const explicitLocationMatches =
    comparablePath(env.GIT_WORK_TREE) === comparablePath(repo) &&
    comparablePath(env.GIT_DIR) === comparablePath(expectedGitDir) &&
    (!env.GIT_INDEX_FILE || comparablePath(env.GIT_INDEX_FILE) === comparablePath(expectedIndex));
  if (!explicitLocationMatches) {
    delete env.GIT_DIR;
    delete env.GIT_WORK_TREE;
    delete env.GIT_INDEX_FILE;
  } else {
    env.GIT_DIR = absoluteEnvironmentPath(env.GIT_DIR);
    env.GIT_WORK_TREE = absoluteEnvironmentPath(env.GIT_WORK_TREE);
    if (env.GIT_INDEX_FILE) env.GIT_INDEX_FILE = absoluteEnvironmentPath(env.GIT_INDEX_FILE);
  }
  delete env.GIT_COMMON_DIR;
  delete env.GIT_OBJECT_DIRECTORY;
  delete env.GIT_ALTERNATE_OBJECT_DIRECTORIES;
  return env;
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

function validValue(value) {
  const normalized = (value ?? '').trim();
  return EMPTY_VALUE.test(normalized) ? null : normalized;
}

function normalizedFieldName(value) {
  return value.trim().normalize('NFKC');
}

const DEFAULT_IGNORABLE = /\p{Default_Ignorable_Code_Point}/u;
const ACCEPTANCE_BLOCK_START = '<!-- kpanel-release-metrics:start -->';
const ACCEPTANCE_BLOCK_END = '<!-- kpanel-release-metrics:end -->';
const ACCEPTANCE_FIELDS = [
  '首个纳入提交时间',
  '候选冻结时间',
  '生产完成时间',
  '提交到生产用时',
  '是否回滚、紧急热修复或重复发布',
  '若发生失败，发现时间、恢复时间和逃逸门禁',
];
const PROCESS_BLOCK_START = '<!-- kpanel-release-process-metrics:start -->';
const PROCESS_BLOCK_END = '<!-- kpanel-release-process-metrics:end -->';
const PROCESS_FIELDS = [
  '已记录发布流程异常或无效证据拦截次数',
  '其中生产写操作开始后异常次数',
];
const PROCESS_METRICS_REQUIRED_FROM = [0, 81, 2];
const PROCESS_INCIDENTS_BLOCK_START = '<!-- kpanel-release-process-incidents:start -->';
const PROCESS_INCIDENTS_BLOCK_END = '<!-- kpanel-release-process-incidents:end -->';
const PROCESS_INCIDENTS_REQUIRED_FROM = [0, 90, 2];
const PROCESS_INCIDENT_FIELDS = [
  'fingerprint',
  'position',
  'count',
  'impact',
  'recoveryEvidence',
  'permanentAction',
  'historicalReleases',
];
const PROCESS_INCIDENT_FINGERPRINT = /^[a-z0-9][a-z0-9._-]*(?:\/[a-z0-9][a-z0-9._-]*){2}$/;
const PROCESS_INCIDENT_POSITIONS = new Set(['before-production-write', 'after-production-write']);
const PROCESS_INCIDENT_PLACEHOLDER = /^(?:<.*>|无|未知|未记录|未验证|不适用|待.*|n\/?a|none|null|unknown|tbd|todo)$/i;
const PROCESS_INCIDENT_REPEAT_WINDOW = 5;
const PROCESS_INCIDENT_REPEAT_REQUIRED_FROM = [0, 100, 1];
// PROJECT_RULES.md 5.4: from v1.21.0 every stable record states the coverage-check decision, and a pending
// decision names the completed run or the user's waiver with its deadline. RC records only record it.
const SECURITY_COVERAGE_REQUIRED_FROM = [1, 21, 0];
const SECURITY_COVERAGE_LINE = /^- 覆盖检查[^：\n]*：[ \t]*(.*)$/m;
const SECURITY_COVERAGE_DECISIONS = /\b(?:ok|scoped-required|full-required)\b/g;

function completedAuditRuns() {
  return new Set(loadRuns().filter((run) => run.complete).map((run) => run.name));
}

// A pending decision is resolved only by a run recorded as complete, not by mentioning an interrupted one.
export function securityCoverageErrors(markdown, label = 'acceptance record', completedRuns = completedAuditRuns()) {
  const value = validValue(markdown.replace(/\r\n/g, '\n').match(SECURITY_COVERAGE_LINE)?.[1]);
  if (value === null) {
    return [label + ': release v1.21.0 and later must record the 覆盖检查 decision (PROJECT_RULES.md 5.4)'];
  }
  const decisions = new Set(value.match(SECURITY_COVERAGE_DECISIONS) ?? []);
  if (decisions.size === 0) return [label + ': 覆盖检查 must name the decision: ok, scoped-required or full-required'];
  const pending = [...decisions].some((decision) => decision !== 'ok');
  const namedComplete = (value.match(/\brun-\d+\b/g) ?? []).some((name) => completedRuns.has(name));
  const waived = /豁免/.test(value) && /\b\d{4}-\d{2}-\d{2}\b/.test(value);
  return pending && !namedComplete && !waived
    ? [label + ': a pending 覆盖检查 decision must name a run-<N> recorded as complete in .governance/security-audit,'
      + ' or the user waiver with its YYYY-MM-DD deadline']
    : [];
}

function acceptanceFields(markdown) {
  const fields = new Map();
  const duplicates = new Set();
  const structureErrors = [];
  let defaultIgnorables = false;
  const lines = markdown.replace(/\r\n/g, '\n').split('\n');
  const starts = lines.flatMap((line, index) => line === ACCEPTANCE_BLOCK_START ? [index] : []);
  const ends = lines.flatMap((line, index) => line === ACCEPTANCE_BLOCK_END ? [index] : []);
  if (starts.length !== 1 || ends.length !== 1 || ends[0] <= starts[0]) {
    structureErrors.push('requires exactly one ordered release-metrics marker pair');
    return { fields, duplicates, structureErrors, defaultIgnorables };
  }

  const rows = lines.slice(starts[0] + 1, ends[0]);
  if (rows.length !== ACCEPTANCE_FIELDS.length) {
    structureErrors.push('release-metrics block must contain exactly six rows');
  }
  for (const [index, line] of rows.entries()) {
    if (DEFAULT_IGNORABLE.test(line)) defaultIgnorables = true;
    const normalizedLine = line.normalize('NFKC');
    if (/`|<!--|-->/.test(normalizedLine)) {
      structureErrors.push('release-metrics rows must be plain text without Markdown code or HTML comment controls');
    }
    const match = line.match(/^- ([^：]+)：\s*(.*)$/);
    if (!match) {
      structureErrors.push('release-metrics rows must use "- 字段：值" syntax');
      continue;
    }
    if (match[2].trim() === '') {
      structureErrors.push('release-metrics values must be explicit and non-blank');
    }
    const field = normalizedFieldName(match[1]);
    if (!ACCEPTANCE_FIELDS.some((expected) => normalizedFieldName(expected) === field)) {
      structureErrors.push('release-metrics block contains an unknown field');
      continue;
    }
    if (normalizedFieldName(ACCEPTANCE_FIELDS[index] ?? '') !== field) {
      structureErrors.push('release-metrics fields must keep the canonical order');
    }
    if (fields.has(field)) duplicates.add(field);
    fields.set(field, match[2].trim().normalize('NFKC'));
  }
  return { fields, duplicates, structureErrors, defaultIgnorables };
}

function acceptanceVersions(markdown, label) {
  const title = markdown.match(/^#\s+KPanel\s+v(\d+)\.(\d+)\.(\d+)\b/m);
  const filename = String(label ?? '').replaceAll('\\', '/').match(/release-v(\d+)\.(\d+)\.(\d+)-acceptance\.md(?:$|:)/i);
  return {
    title: title ? title.slice(1).map(Number) : null,
    filename: filename ? filename.slice(1).map(Number) : null,
  };
}

function sameVersion(left, right) {
  return left !== null && right !== null && left.every((value, index) => value === right[index]);
}

function versionAtLeast(version, minimum) {
  if (version === null) return false;
  for (let index = 0; index < minimum.length; index += 1) {
    if (version[index] !== minimum[index]) return version[index] > minimum[index];
  }
  return true;
}

function compareVersionDescending(left, right) {
  for (let index = 0; index < 3; index += 1) {
    const difference = (right?.[index] ?? 0) - (left?.[index] ?? 0);
    if (difference !== 0) return difference;
  }
  return 0;
}

function releaseTagVersion(tag) {
  return isStableReleaseTag(tag) ? tag.slice(1).split('.').map(Number) : null;
}

function incidentFingerprint(incident) {
  return typeof incident?.fingerprint === 'string' ? incident.fingerprint.trim() : '';
}

export function detectRepeatedProcessIncidents(records, window = PROCESS_INCIDENT_REPEAT_WINDOW) {
  const findings = [];
  for (const [index, record] of records.entries()) {
    const priorReleases = records.slice(index + 1, index + window);
    for (const incident of record.incidents ?? []) {
      const fingerprint = incidentFingerprint(incident);
      if (!PROCESS_INCIDENT_FINGERPRINT.test(fingerprint)) continue;
      const repeatedIn = priorReleases
        .filter((prior) => (prior.incidents ?? []).some((prior_) => incidentFingerprint(prior_) === fingerprint))
        .map((prior) => prior.tag);
      if (repeatedIn.length === 0) continue;
      const declared = new Set(Array.isArray(incident.historicalReleases) ? incident.historicalReleases : []);
      findings.push({
        tag: record.tag,
        version: record.version ?? releaseTagVersion(record.tag),
        label: record.label ?? record.tag,
        reported: record.reported ?? true,
        fingerprint,
        repeatedIn,
        undeclared: repeatedIn.filter((tag) => !declared.has(tag)),
      });
    }
  }
  return findings;
}

export function validateProcessIncidentHistory(records, window = PROCESS_INCIDENT_REPEAT_WINDOW) {
  return detectRepeatedProcessIncidents(records, window)
    .filter((finding) => finding.reported && finding.undeclared.length > 0 &&
      versionAtLeast(finding.version, PROCESS_INCIDENT_REPEAT_REQUIRED_FROM))
    .map((finding) => finding.label + ': release-process-incidents fingerprint "' + finding.fingerprint +
      '" already appeared within the rolling ' + window + ' releases; historicalReleases must declare ' +
      finding.undeclared.join(', '));
}

const ACCEPTANCE_FILENAME = /^release-(v\d+\.\d+\.\d+)-acceptance\.md$/i;

function stableTagTimes(directory) {
  const output = tryGit(directory, ['for-each-ref', '--format=%(refname:short)%09%(creatordate:iso-strict)', 'refs/tags/v*']);
  if (!output) return null;
  const times = new Map();
  for (const line of output.split(/\r?\n/)) {
    const [tag, created] = line.split('\t');
    const at = Date.parse(created);
    if (isStableReleaseTag(tag) && !Number.isNaN(at)) times.set(tag.toLowerCase(), at);
  }
  return times.size > 0 ? times : null;
}

// "Prior" means released earlier, not a lower version: a patch on an older line can ship after a
// newer release (v1.15.1 followed the v1.16.0 rollback). Tag creation time orders the window; the
// record being written may not be tagged yet and counts as newest. Without a time for every other
// record (no Git, shallow clone without tags) the whole list keeps the previous version order, so
// the check never becomes looser than before.
function releaseOrder(entries, targetTag, times) {
  const byVersion = [...entries].sort((left, right) => compareVersionDescending(left.version, right.version));
  if (times === null) return byVersion;
  const timed = byVersion.map((entry) => ({
    ...entry,
    releasedAt: times.get(entry.tag) ?? (entry.tag === targetTag ? Number.POSITIVE_INFINITY : null),
  }));
  if (timed.some((entry) => entry.releasedAt === null)) return byVersion;
  return timed.sort((left, right) => (right.releasedAt - left.releasedAt) ||
    compareVersionDescending(left.version, right.version));
}

export function readAcceptanceHistory(path, window = PROCESS_INCIDENT_REPEAT_WINDOW, times = undefined) {
  const absolutePath = resolve(path);
  const directory = dirname(absolutePath);
  const target = ACCEPTANCE_FILENAME.exec(absolutePath.replaceAll('\\', '/').split('/').pop() ?? '');
  if (target === null) return [];

  const targetTag = target[1].toLowerCase();
  const entries = readdirSync(directory)
    .flatMap((name) => {
      const match = ACCEPTANCE_FILENAME.exec(name);
      if (match === null) return [];
      return [{ name, tag: match[1].toLowerCase(), version: releaseTagVersion(match[1].toLowerCase()) }];
    })
    .filter((entry) => entry.version !== null);
  const siblings = releaseOrder(entries, targetTag, times === undefined ? stableTagTimes(directory) : times);

  const startIndex = siblings.findIndex((entry) => entry.tag === targetTag);
  if (startIndex === -1) return [];

  return siblings.slice(startIndex, startIndex + window).map((entry, index) => {
    const filePath = index === 0 ? absolutePath : resolve(directory, entry.name);
    return {
      tag: entry.tag,
      version: entry.version,
      label: index === 0 ? path : filePath,
      reported: index === 0,
      incidents: extractProcessIncidents(readFileSync(filePath, 'utf8')),
    };
  });
}

function processFields(markdown) {
  const fields = new Map();
  const duplicates = new Set();
  const structureErrors = [];
  let defaultIgnorables = false;
  const lines = markdown.replace(/\r\n/g, '\n').split('\n');
  const starts = lines.flatMap((line, index) => line === PROCESS_BLOCK_START ? [index] : []);
  const ends = lines.flatMap((line, index) => line === PROCESS_BLOCK_END ? [index] : []);
  const present = starts.length > 0 || ends.length > 0;
  if (!present) return { fields, duplicates, structureErrors, defaultIgnorables, present };
  if (starts.length !== 1 || ends.length !== 1 || ends[0] <= starts[0]) {
    structureErrors.push('requires exactly one ordered release-process-metrics marker pair');
    return { fields, duplicates, structureErrors, defaultIgnorables, present };
  }

  const rows = lines.slice(starts[0] + 1, ends[0]);
  if (rows.length !== PROCESS_FIELDS.length) {
    structureErrors.push('release-process-metrics block must contain exactly two rows');
  }
  for (const [index, line] of rows.entries()) {
    if (DEFAULT_IGNORABLE.test(line)) defaultIgnorables = true;
    const normalizedLine = line.normalize('NFKC');
    if (/`|<!--|-->/.test(normalizedLine)) {
      structureErrors.push('release-process-metrics rows must be plain text without Markdown code or HTML comment controls');
    }
    const match = line.match(/^- ([^：]+)：\s*(.*)$/);
    if (!match) {
      structureErrors.push('release-process-metrics rows must use "- 字段：值" syntax');
      continue;
    }
    if (match[2].trim() === '') {
      structureErrors.push('release-process-metrics values must be explicit and non-blank');
    }
    const field = normalizedFieldName(match[1]);
    if (!PROCESS_FIELDS.some((expected) => normalizedFieldName(expected) === field)) {
      structureErrors.push('release-process-metrics block contains an unknown field');
      continue;
    }
    if (normalizedFieldName(PROCESS_FIELDS[index] ?? '') !== field) {
      structureErrors.push('release-process-metrics fields must keep the canonical order');
    }
    if (fields.has(field)) duplicates.add(field);
    fields.set(field, match[2].trim().normalize('NFKC'));
  }
  return { fields, duplicates, structureErrors, defaultIgnorables, present };
}

function processIncidentDetails(markdown) {
  const incidents = [];
  const structureErrors = [];
  const lines = markdown.replace(/\r\n/g, '\n').split('\n');
  const starts = lines.flatMap((line, index) => line === PROCESS_INCIDENTS_BLOCK_START ? [index] : []);
  const ends = lines.flatMap((line, index) => line === PROCESS_INCIDENTS_BLOCK_END ? [index] : []);
  const present = starts.length > 0 || ends.length > 0;
  if (!present) return { incidents, structureErrors, present };
  if (starts.length !== 1 || ends.length !== 1 || ends[0] <= starts[0]) {
    structureErrors.push('requires exactly one ordered release-process-incidents marker pair');
    return { incidents, structureErrors, present };
  }

  const source = lines.slice(starts[0] + 1, ends[0]).join('\n').trim();
  if (source === '') {
    structureErrors.push('release-process-incidents block must contain a JSON array');
    return { incidents, structureErrors, present };
  }
  if (DEFAULT_IGNORABLE.test(source)) {
    structureErrors.push('release-process-incidents evidence must not contain default-ignorable characters');
    return { incidents, structureErrors, present };
  }

  let parsed;
  try {
    parsed = JSON.parse(source);
  } catch {
    structureErrors.push('release-process-incidents block must contain valid JSON');
    return { incidents, structureErrors, present };
  }
  if (!Array.isArray(parsed)) {
    structureErrors.push('release-process-incidents JSON must be an array');
    return { incidents, structureErrors, present };
  }

  const fingerprints = new Set();
  for (const [index, incident] of parsed.entries()) {
    const prefix = 'release-process-incidents item ' + (index + 1);
    if (incident === null || Array.isArray(incident) || typeof incident !== 'object') {
      structureErrors.push(prefix + ' must be an object');
      continue;
    }
    const keys = Object.keys(incident).sort();
    const expectedKeys = [...PROCESS_INCIDENT_FIELDS].sort();
    if (keys.length !== expectedKeys.length || keys.some((key, keyIndex) => key !== expectedKeys[keyIndex])) {
      structureErrors.push(prefix + ' must contain exactly the canonical fields');
      continue;
    }

    const fingerprint = typeof incident.fingerprint === 'string' ? incident.fingerprint.trim() : '';
    if (!PROCESS_INCIDENT_FINGERPRINT.test(fingerprint)) {
      structureErrors.push(prefix + ' fingerprint must use "stage/authoritative-entry/root-cause" slug syntax');
    } else if (fingerprints.has(fingerprint)) {
      structureErrors.push(prefix + ' duplicates fingerprint "' + fingerprint + '"');
    } else {
      fingerprints.add(fingerprint);
    }
    if (!PROCESS_INCIDENT_POSITIONS.has(incident.position)) {
      structureErrors.push(prefix + ' position must be before-production-write or after-production-write');
    }
    if (!Number.isSafeInteger(incident.count) || incident.count < 1) {
      structureErrors.push(prefix + ' count must be a positive safe integer');
    }
    for (const field of ['impact', 'recoveryEvidence', 'permanentAction']) {
      const value = typeof incident[field] === 'string' ? incident[field].trim().normalize('NFKC') : '';
      if (value.length < 4 || PROCESS_INCIDENT_PLACEHOLDER.test(value) || DEFAULT_IGNORABLE.test(value)) {
        structureErrors.push(prefix + ' ' + field + ' must contain explicit evidence');
      }
    }
    if (!Array.isArray(incident.historicalReleases) ||
        incident.historicalReleases.some((tag) => typeof tag !== 'string' || !isStableReleaseTag(tag)) ||
        new Set(incident.historicalReleases).size !== incident.historicalReleases.length) {
      structureErrors.push(prefix + ' historicalReleases must be a unique array of stable release tags');
    }
    incidents.push(incident);
  }
  return { incidents, structureErrors, present };
}

function explicitCount(value) {
  const normalized = validValue(value);
  if (normalized === null || !/^\d+$/.test(normalized)) return null;
  const count = Number(normalized);
  return Number.isSafeInteger(count) ? count : null;
}

export function extractProcessMetrics(markdown) {
  const { fields } = processFields(markdown);
  const field = (name) => fields.get(normalizedFieldName(name));
  return {
    processIncidentCount: explicitCount(field('已记录发布流程异常或无效证据拦截次数')),
    postProductionProcessIncidentCount: explicitCount(field('其中生产写操作开始后异常次数')),
  };
}

export function extractProcessIncidents(markdown) {
  return processIncidentDetails(markdown).incidents;
}

export function extractAcceptanceMetrics(markdown) {
  const { fields } = acceptanceFields(markdown);
  const field = (name) => fields.get(normalizedFieldName(name));

  return {
    firstIncludedCommitAt: validValue(field('首个纳入提交时间')),
    candidateFrozenAt: validValue(field('候选冻结时间')),
    productionCompletedAt: validValue(field('生产完成时间')),
    commitToProduction: validValue(field('提交到生产用时')),
    changeFailure: validValue(field('是否回滚、紧急热修复或重复发布')),
    recovery: validValue(field('若发生失败，发现时间、恢复时间和逃逸门禁')),
    ...extractProcessMetrics(markdown),
    processIncidents: extractProcessIncidents(markdown),
  };
}

const ISO_TIMESTAMP = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,3})?(?:Z|[+-](\d{2}):(\d{2}))$/;

function validDate(value) {
  if (value === null) return false;
  const match = value.match(ISO_TIMESTAMP);
  if (!match) return false;
  const [, year, month, day, hour, minute, second, offsetHour = '00', offsetMinute = '00'] = match;
  const parts = [year, month, day, hour, minute, second, offsetHour, offsetMinute].map(Number);
  const [y, mo, d, h, mi, s, oh, om] = parts;
  if (mo < 1 || mo > 12 || d < 1 || h > 23 || mi > 59 || s > 59 || oh > 23 || om > 59) return false;
  const calendar = new Date(Date.UTC(y, mo - 1, d, h, mi, s));
  if (calendar.getUTCFullYear() !== y || calendar.getUTCMonth() !== mo - 1 || calendar.getUTCDate() !== d ||
      calendar.getUTCHours() !== h || calendar.getUTCMinutes() !== mi || calendar.getUTCSeconds() !== s) return false;
  return !Number.isNaN(Date.parse(value));
}

function hasFailureRecoveryDetails(value) {
  if (value === null || DEFAULT_IGNORABLE.test(value)) return false;
  const normalizedValue = value.normalize('NFKC');
  const segments = normalizedValue.split(';').map((segment) => segment.trim());
  if (segments.length !== 3) return false;
  const detail = (segment, label) => validValue(segment.match(
    new RegExp('^' + label + '\\s*:\\s*(.+)$'),
  )?.[1]);
  const discoveredAt = detail(segments[0], '发现时间');
  const recoveredAt = detail(segments[1], '恢复时间');
  const escapedGate = detail(segments[2], '逃逸门禁');
  const escapedGateMatch = escapedGate?.match(/^(?:已逃逸|未逃逸)\s*:\s*(.+)$/);
  const escapedGateDetail = validValue(escapedGateMatch?.[1]);
  const compactGateDetail = escapedGateDetail?.toLowerCase().replace(/[\p{White_Space}\p{P}\p{S}]+/gu, '') ?? '';
  const escapedGatePlaceholder = /^(?:无|无(?:缺口|门禁|异常|问题)|待.*|tbd|todo|none|null|unknown|na|notapplicable|具体(?:缺口|原因))$/i;
  const concreteEscapedGate = escapedGateDetail !== null && escapedGateDetail.length >= 4 &&
    !escapedGatePlaceholder.test(compactGateDetail) &&
    !/待(?:进一步)?(?:确认|分析|调查|复核|补充|填写|定)|请填写|<[^>]*>|已逃逸|未逃逸|逃逸门禁/.test(escapedGateDetail);
  return validDate(discoveredAt) && validDate(recoveredAt) && concreteEscapedGate &&
    Date.parse(recoveredAt) >= Date.parse(discoveredAt);
}

export function validateAcceptanceMetrics(markdown, label = 'acceptance record') {
  const errors = [];
  const { fields, duplicates, structureErrors, defaultIgnorables } = acceptanceFields(markdown);
  for (const error of structureErrors) errors.push(label + ': ' + error);
  if (defaultIgnorables) {
    errors.push(label + ': structured acceptance evidence must not contain default-ignorable characters');
  }
  for (const field of ACCEPTANCE_FIELDS) {
    const normalizedField = normalizedFieldName(field);
    if (!fields.has(normalizedField)) errors.push(label + ': missing structured field "' + field + '"');
    if (duplicates.has(normalizedField)) errors.push(label + ': duplicate structured field "' + field + '"');
  }
  const versions = acceptanceVersions(markdown, label);
  if (versions.title !== null && versions.filename !== null && !sameVersion(versions.title, versions.filename)) {
    errors.push(label + ': release version in title must match the acceptance filename');
  }
  const acceptanceVersion = versions.filename ?? versions.title;
  const process = processFields(markdown);
  const processIncidents = processIncidentDetails(markdown);
  if (versionAtLeast(acceptanceVersion, PROCESS_METRICS_REQUIRED_FROM) && !process.present) {
    errors.push(label + ': release v0.81.2 and later requires release-process-metrics evidence');
  }
  if (versionAtLeast(acceptanceVersion, PROCESS_INCIDENTS_REQUIRED_FROM) && !processIncidents.present) {
    errors.push(label + ': release v0.90.2 and later requires release-process-incidents evidence');
  }
  // Only a stable filename identifies a stable record; RC titles also parse as X.Y.Z.
  if (versionAtLeast(versions.filename, SECURITY_COVERAGE_REQUIRED_FROM)) {
    errors.push(...securityCoverageErrors(markdown, label));
  }
  for (const error of process.structureErrors) errors.push(label + ': ' + error);
  for (const error of processIncidents.structureErrors) errors.push(label + ': ' + error);
  if (process.defaultIgnorables) {
    errors.push(label + ': structured process evidence must not contain default-ignorable characters');
  }
  if (process.present) {
    for (const field of PROCESS_FIELDS) {
      const normalizedField = normalizedFieldName(field);
      if (!process.fields.has(normalizedField)) errors.push(label + ': missing structured field "' + field + '"');
      if (process.duplicates.has(normalizedField)) errors.push(label + ': duplicate structured field "' + field + '"');
    }
  }
  if (errors.length > 0) return errors;

  let processIncidentCount = null;
  let postProductionProcessIncidentCount = null;
  if (process.present) {
    const totalValue = process.fields.get(normalizedFieldName(PROCESS_FIELDS[0]));
    const postProductionValue = process.fields.get(normalizedFieldName(PROCESS_FIELDS[1]));
    const total = explicitCount(totalValue);
    const postProduction = explicitCount(postProductionValue);
    processIncidentCount = total;
    postProductionProcessIncidentCount = postProduction;
    const totalUnreported = validValue(totalValue) === null;
    const postProductionUnreported = validValue(postProductionValue) === null;
    if (!totalUnreported && total === null) {
      errors.push(label + ': 发布流程异常次数 must be a non-negative integer or an explicit unreported marker');
    }
    if (!postProductionUnreported && postProduction === null) {
      errors.push(label + ': 生产写操作开始后异常次数 must be a non-negative integer or an explicit unreported marker');
    }
    if (totalUnreported !== postProductionUnreported) {
      errors.push(label + ': process incident counts must be reported or unreported together');
    }
    if (total !== null && postProduction !== null && postProduction > total) {
      errors.push(label + ': 生产写操作开始后异常次数 cannot exceed total process incidents');
    }
  }

  if (processIncidents.present && processIncidentCount !== null && postProductionProcessIncidentCount !== null) {
    const detailedTotal = processIncidents.incidents.reduce((sum, incident) => sum + incident.count, 0);
    const detailedPostProduction = processIncidents.incidents
      .filter((incident) => incident.position === 'after-production-write')
      .reduce((sum, incident) => sum + incident.count, 0);
    if (detailedTotal !== processIncidentCount) {
      errors.push(label + ': release-process-incidents counts must equal the reported process incident total');
    }
    if (detailedPostProduction !== postProductionProcessIncidentCount) {
      errors.push(label + ': after-production-write incident counts must equal the reported post-production total');
    }
  } else if (processIncidents.present && processIncidents.incidents.length > 0) {
    errors.push(label + ': unreported process incident counts require an empty release-process-incidents array');
  }

  const metrics = extractAcceptanceMetrics(markdown);
  if (metrics.firstIncludedCommitAt !== null && !validDate(metrics.firstIncludedCommitAt)) {
    errors.push(label + ': 首个纳入提交时间 must be an ISO timestamp or an explicit unverified marker');
  }
  if (metrics.candidateFrozenAt !== null && !validDate(metrics.candidateFrozenAt)) {
    errors.push(label + ': 候选冻结时间 must be an ISO timestamp or an explicit unverified marker');
  }
  if (metrics.productionCompletedAt !== null && !validDate(metrics.productionCompletedAt)) {
    errors.push(label + ': 生产完成时间 must be an ISO timestamp or an explicit unverified marker');
  }

  if (validDate(metrics.firstIncludedCommitAt) && validDate(metrics.candidateFrozenAt) &&
      new Date(metrics.candidateFrozenAt) < new Date(metrics.firstIncludedCommitAt)) {
    errors.push(label + ': 候选冻结时间 cannot precede 首个纳入提交时间');
  }

  if (metrics.productionCompletedAt === null) {
    if (metrics.commitToProduction !== null) {
      errors.push(label + ': 提交到生产用时 must stay unreported when production completion is unverified');
    }
    const failure = classifyChangeFailure(metrics.changeFailure);
    if (failure === 'yes' && !hasFailureRecoveryDetails(metrics.recovery)) {
      errors.push(label + ': a failed change requires discovery, recovery, and escaped-gate details');
    }
    if (failure !== 'yes' && metrics.recovery !== null) {
      errors.push(label + ': recovery details require an explicit failed change state');
    }
    return errors;
  }

  if (!validDate(metrics.firstIncludedCommitAt) || !validDate(metrics.candidateFrozenAt)) {
    errors.push(label + ': a completed production deployment requires explicit included-commit and candidate-freeze timestamps');
  }

  if (validDate(metrics.candidateFrozenAt) &&
      new Date(metrics.productionCompletedAt) < new Date(metrics.candidateFrozenAt)) {
    errors.push(label + ': 生产完成时间 cannot precede 候选冻结时间');
  }

  const derivedHours = durationHours(metrics.firstIncludedCommitAt, metrics.productionCompletedAt);
  const reportedHours = metrics.commitToProduction?.match(/^(\d+(?:\.\d+)?)\s*(?:小时|h(?:ours?)?)?$/i);
  if (derivedHours === null || !reportedHours) {
    errors.push(label + ': 提交到生产用时 must be a decimal hour value when production completed');
  } else if (Math.abs(derivedHours - Number(reportedHours[1])) > 0.02) {
    errors.push(label + ': 提交到生产用时 does not match the structured timestamps');
  }

  const failure = classifyChangeFailure(metrics.changeFailure);
  if (failure === 'unreported') {
    errors.push(label + ': production completion requires an explicit 是/否 change failure state');
  }
  if (failure === 'yes' && !hasFailureRecoveryDetails(metrics.recovery)) {
    errors.push(label + ': a failed change requires discovery, recovery, and escaped-gate details');
  }
  if (failure !== 'yes' && metrics.recovery !== null) {
    errors.push(label + ': recovery details require an explicit failed change state');
  }
  return errors;
}

export function classifyChangeFailure(value) {
  if (!value) return 'unreported';
  if (/^否(?:\b|（|\(|$)/.test(value)) return 'no';
  if (/^是(?:\b|（|\(|$)/.test(value)) return 'yes';
  return 'unreported';
}

export function durationHours(startValue, endValue) {
  if (!startValue || !endValue) return null;
  const start = new Date(startValue);
  const end = new Date(endValue);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()) || end < start) return null;
  return Number(((end - start) / 3_600_000).toFixed(2));
}

export function productionLeadHours(metrics) {
  const derived = durationHours(metrics.firstIncludedCommitAt, metrics.productionCompletedAt);
  if (derived !== null) return derived;
  if (!metrics.commitToProduction) return null;
  const match = metrics.commitToProduction.match(/^(\d+(?:\.\d+)?)\s*(?:小时|h(?:ours?)?)?$/i);
  return match ? Number(match[1]) : null;
}

export function summarizeReleaseMetrics(releases, options) {
  const cutoff = new Date(options.now.getTime() - options.days * 86_400_000);
  const windowReleases = releases.filter((release) => release.createdAt >= cutoff && release.createdAt <= options.now);
  const releasesPerDay = new Map();
  for (const release of windowReleases) {
    const day = release.createdAt.toISOString().slice(0, 10);
    releasesPerDay.set(day, (releasesPerDay.get(day) ?? 0) + 1);
  }
  const productionDeployments = releases.filter((release) => {
    const completedAt = release.acceptance.metrics.productionCompletedAt;
    if (!validDate(completedAt)) return false;
    const timestamp = new Date(completedAt);
    return timestamp >= cutoff && timestamp <= options.now;
  });
  const productionDeploymentDays = new Map();
  for (const release of productionDeployments) {
    const day = new Date(release.acceptance.metrics.productionCompletedAt).toISOString().slice(0, 10);
    productionDeploymentDays.set(day, (productionDeploymentDays.get(day) ?? 0) + 1);
  }

  const selected = releases.slice(0, options.releases);
  const acceptanceCount = selected.filter((release) => release.acceptance.exists).length;
  const productionCompletionReported = selected.filter((release) =>
    validDate(release.acceptance.metrics.productionCompletedAt)).length;
  const failureStates = selected.map((release) => classifyChangeFailure(release.acceptance.metrics.changeFailure));
  const reportedFailureCount = failureStates.filter((state) => state !== 'unreported').length;
  const failedReleaseCount = failureStates.filter((state) => state === 'yes').length;
  const leadTimes = selected
    .map((release) => productionLeadHours(release.acceptance.metrics))
    .filter((value) => value !== null);
  const freezeTimes = selected
    .map((release) => durationHours(release.acceptance.metrics.candidateFrozenAt, release.acceptance.metrics.productionCompletedAt))
    .filter((value) => value !== null);
  const processIncidentReleases = selected.filter((release) =>
    release.acceptance.metrics.processIncidentCount !== null &&
    release.acceptance.metrics.postProductionProcessIncidentCount !== null);
  const processIncidentReleaseCount = processIncidentReleases.filter((release) =>
    release.acceptance.metrics.processIncidentCount > 0).length;
  const processIncidentCount = processIncidentReleases.length === 0 ? null :
    processIncidentReleases.reduce((total, release) =>
      total + release.acceptance.metrics.processIncidentCount, 0);
  const postProductionProcessIncidentCount = processIncidentReleases.length === 0 ? null :
    processIncidentReleases.reduce((total, release) =>
      total + release.acceptance.metrics.postProductionProcessIncidentCount, 0);
  const repeatedProcessIncidents = detectRepeatedProcessIncidents(selected.map((release) => ({
    tag: release.tag,
    version: releaseTagVersion(release.tag),
    label: release.acceptance.path,
    reported: release.acceptance.exists,
    incidents: release.acceptance.metrics.processIncidents ?? [],
  })));

  return {
    generatedAt: options.now.toISOString(),
    source: {
      ref: options.evidenceRef ?? null,
      commit: options.evidenceCommit ?? null,
    },
    window: {
      days: options.days,
      releaseCount: windowReleases.length,
      releaseDays: releasesPerDay.size,
      maxReleasesPerDay: Math.max(0, ...releasesPerDay.values()),
      productionDeploymentCount: productionDeployments.length,
      productionDeploymentDays: productionDeploymentDays.size,
      maxProductionDeploymentsPerDay: Math.max(0, ...productionDeploymentDays.values()),
    },
    recent: {
      requested: options.releases,
      available: selected.length,
      acceptanceCount,
      acceptanceCoverage: selected.length === 0 ? null : Number((acceptanceCount / selected.length).toFixed(4)),
      productionCompletionReported,
      productionCompletionCoverage: selected.length === 0 ? null :
        Number((productionCompletionReported / selected.length).toFixed(4)),
      productionLeadTimeReported: leadTimes.length,
      productionLeadTimeHoursMedian: median(leadTimes),
      freezeToProductionReported: freezeTimes.length,
      freezeToProductionHoursMedian: median(freezeTimes),
      changeFailureReported: reportedFailureCount,
      failedReleaseCount,
      changeFailureRate: reportedFailureCount === 0 ? null : Number((failedReleaseCount / reportedFailureCount).toFixed(4)),
      recoveryReported: selected.filter((release) => release.acceptance.metrics.recovery !== null).length,
      processIncidentReported: processIncidentReleases.length,
      processIncidentReleaseCount,
      processIncidentReleaseRate: processIncidentReleases.length === 0 ? null :
        Number((processIncidentReleaseCount / processIncidentReleases.length).toFixed(4)),
      processIncidentCount,
      postProductionProcessIncidentCount,
      repeatedProcessIncidentCount: repeatedProcessIncidents.length,
      undeclaredRepeatedProcessIncidentCount: repeatedProcessIncidents
        .filter((finding) => finding.reported && finding.undeclared.length > 0).length,
    },
    repeatedProcessIncidents,
    releases: selected,
  };
}

// Preview metrics answer one question the stable view structurally cannot: how much release friction
// was spent before a stable tag existed. RC records carry the same process-incident evidence, so the
// counts and the rolling repeat window are the existing ones; only the population differs. Nothing
// here gates: `historicalReleases` accepts stable tags only, so an RC repeat cannot be declared yet
// and must not be failed on.
// `stableTags` is required rather than defaulted: an empty default would silently label every train
// as in flight, which is exactly the kind of quiet wrong answer this view exists to remove.
export function summarizePreviewMetrics(previewReleases, options, stableTags) {
  const cutoff = new Date(options.now.getTime() - options.days * 86_400_000);
  const windowReleases = previewReleases.filter((release) =>
    release.createdAt >= cutoff && release.createdAt <= options.now);
  const releasesPerDay = new Map();
  for (const release of windowReleases) {
    const day = release.createdAt.toISOString().slice(0, 10);
    releasesPerDay.set(day, (releasesPerDay.get(day) ?? 0) + 1);
  }

  const selected = previewReleases.slice(0, options.releases);
  const acceptanceCount = selected.filter((release) => release.acceptance.exists).length;
  const reportsProcessMetrics = (release) =>
    release.acceptance.metrics.processIncidentCount !== null &&
    release.acceptance.metrics.postProductionProcessIncidentCount !== null;
  const reported = selected.filter(reportsProcessMetrics);
  const sumIncidents = (records) => records.length === 0 ? null :
    records.reduce((total, release) => total + release.acceptance.metrics.processIncidentCount, 0);
  const sumPostProduction = (records) => records.length === 0 ? null :
    records.reduce((total, release) => total + release.acceptance.metrics.postProductionProcessIncidentCount, 0);

  const repeated = detectRepeatedProcessIncidents(selected.map((release) => ({
    tag: release.tag,
    version: releaseTagVersion(previewReleaseTrain(release.tag) ?? ''),
    label: release.acceptance.path,
    reported: release.acceptance.exists,
    incidents: release.acceptance.metrics.processIncidents ?? [],
  })));

  const trains = [];
  for (const release of selected) {
    const train = previewReleaseTrain(release.tag);
    if (train === null) continue;
    let entry = trains.find((candidate) => candidate.train === train);
    if (entry === undefined) {
      entry = { train, inFlight: !stableTags.has(train), previewCount: 0, releases: [] };
      trains.push(entry);
    }
    entry.previewCount += 1;
    entry.releases.push(release);
  }
  for (const entry of trains) {
    const entryReported = entry.releases.filter(reportsProcessMetrics);
    entry.processIncidentReported = entryReported.length;
    entry.processIncidentCount = sumIncidents(entryReported);
    entry.postProductionProcessIncidentCount = sumPostProduction(entryReported);
    entry.repeatedProcessIncidentCount = repeated
      .filter((finding) => previewReleaseTrain(finding.tag) === entry.train).length;
    delete entry.releases;
  }

  return {
    window: {
      days: options.days,
      releaseCount: windowReleases.length,
      releaseDays: releasesPerDay.size,
      maxReleasesPerDay: Math.max(0, ...releasesPerDay.values()),
    },
    requested: options.releases,
    available: selected.length,
    acceptanceCount,
    acceptanceCoverage: selected.length === 0 ? null : Number((acceptanceCount / selected.length).toFixed(4)),
    processIncidentReported: reported.length,
    processIncidentCount: sumIncidents(reported),
    postProductionProcessIncidentCount: sumPostProduction(reported),
    repeatedProcessIncidentCount: repeated.length,
    trains,
    repeatedProcessIncidents: repeated,
    releases: selected,
  };
}

function median(values) {
  if (values.length === 0) return null;
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  if (sorted.length % 2 === 1) return sorted[middle];
  return Number(((sorted[middle - 1] + sorted[middle]) / 2).toFixed(2));
}

function collectTaggedReleases(repo, evidenceCommit, acceptanceLimit, accepts) {
  const mergedTags = new Set(
    runGit(repo, ['tag', '--merged', evidenceCommit, '--list', 'v*']).split(/\r?\n/).filter(Boolean),
  );
  const output = runGit(repo, ['for-each-ref', '--sort=-creatordate', '--format=%(refname:short)%09%(creatordate:iso-strict)', 'refs/tags']);
  if (!output) return [];

  return output
    .split(/\r?\n/)
    .map((line) => {
      const [tag, created] = line.split('\t');
      return { tag, createdAt: new Date(created) };
    })
    .filter((release) => mergedTags.has(release.tag) && accepts(release.tag) && !Number.isNaN(release.createdAt.getTime()))
    .map((release, index) => {
      const acceptancePath = 'docs/release-' + release.tag + '-acceptance.md';
      const markdown = index < acceptanceLimit ? tryGit(repo, ['show', evidenceCommit + ':' + acceptancePath]) : null;
      return {
        tag: release.tag,
        createdAt: release.createdAt,
        commit: runGit(repo, ['rev-parse', release.tag + '^{commit}']),
        acceptance: {
          exists: markdown !== null,
          path: acceptancePath,
          metrics: extractAcceptanceMetrics(markdown ?? ''),
        },
      };
    });
}

function collectReleases(repo, evidenceCommit, acceptanceLimit) {
  return collectTaggedReleases(repo, evidenceCommit, acceptanceLimit, isStableReleaseTag);
}

function collectPreviewReleases(repo, evidenceCommit, acceptanceLimit) {
  return collectTaggedReleases(repo, evidenceCommit, acceptanceLimit, isPreviewReleaseTag);
}

function percentage(value) {
  return value === null ? '未报告' : (value * 100).toFixed(1) + '%';
}

function metric(value, suffix = '') {
  return value === null ? '未报告' : String(value) + suffix;
}

function failureLabel(value) {
  return { yes: '是', no: '否', unreported: '未报告' }[classifyChangeFailure(value)];
}

export function renderMarkdown(report) {
  const lines = [
    '# KPanel 滚动发布指标',
    '',
    '- 生成时间：' + report.generatedAt,
    '- 证据引用：' + report.source.ref + ' @ ' + report.source.commit,
    '- 窗口：最近 ' + report.window.days + ' 天、最近 ' + report.recent.requested + ' 个正式版本',
    '- 数据来源：Git 标签与版本验收记录；未记录的生产事实保持“未报告”',
    '',
    '## 自动可验证指标',
    '',
    '| 指标 | 结果 |',
    '| --- | --- |',
    '| 窗口内正式版本标签数 | ' + report.window.releaseCount + ' |',
    '| 有正式版本标签的自然日 | ' + report.window.releaseDays + ' |',
    '| 单日最大正式版本标签数 | ' + report.window.maxReleasesPerDay + ' |',
    '| 有生产完成证据的部署数 | ' + report.window.productionDeploymentCount + ' |',
    '| 有生产部署的自然日 | ' + report.window.productionDeploymentDays + ' |',
    '| 单日最大生产部署数 | ' + report.window.maxProductionDeploymentsPerDay + ' |',
    '| 验收记录覆盖率 | ' + report.recent.acceptanceCount + '/' + report.recent.available + '（' + percentage(report.recent.acceptanceCoverage) + '） |',
    '| 生产完成时间覆盖率 | ' + report.recent.productionCompletionReported + '/' + report.recent.available + '（' + percentage(report.recent.productionCompletionCoverage) + '） |',
    '',
    '## 生产交付指标',
    '',
    '| 指标 | 结果 | 数据完整性 |',
    '| --- | --- | --- |',
    '| 首个纳入提交到生产完成中位数 | ' + metric(report.recent.productionLeadTimeHoursMedian, ' 小时') + ' | ' + report.recent.productionLeadTimeReported + '/' + report.recent.available + ' |',
    '| 候选冻结到生产完成中位数 | ' + metric(report.recent.freezeToProductionHoursMedian, ' 小时') + ' | ' + report.recent.freezeToProductionReported + '/' + report.recent.available + ' |',
    '| 变更失败率 | ' + percentage(report.recent.changeFailureRate) + ' | ' + report.recent.changeFailureReported + '/' + report.recent.available + ' |',
    '| 已报告失败恢复详情 | ' + report.recent.recoveryReported + ' | ' + report.recent.recoveryReported + '/' + report.recent.available + ' |',
    '| 已报告流程指标版本中的异常占比 | ' + percentage(report.recent.processIncidentReleaseRate) + ' | ' + report.recent.processIncidentReported + '/' + report.recent.available + ' |',
    '| 已记录发布流程异常/无效证据拦截总数 | ' + metric(report.recent.processIncidentCount) + ' | ' + report.recent.processIncidentReported + '/' + report.recent.available + ' |',
    '| 其中生产写操作开始后异常数 | ' + metric(report.recent.postProductionProcessIncidentCount) + ' | ' + report.recent.processIncidentReported + '/' + report.recent.available + ' |',
    '| 滚动 5 个版本内重复的流程异常指纹数 | ' + metric(report.recent.repeatedProcessIncidentCount) + ' | ' + report.recent.acceptanceCount + '/' + report.recent.available + ' |',
    '| 其中未在 historicalReleases 声明的重复数 | ' + metric(report.recent.undeclaredRepeatedProcessIncidentCount) + ' | ' + report.recent.acceptanceCount + '/' + report.recent.available + ' |',
    '',
    '## 重复流程异常指纹',
    '',
    '| 版本 | 指纹 | 滚动 5 版本内的历史版本 | 未声明历史版本 |',
    '| --- | --- | --- | --- |',
  ];

  if ((report.repeatedProcessIncidents ?? []).length === 0) {
    lines.push('| 无 | 无 | 无 | 无 |');
  }
  for (const finding of report.repeatedProcessIncidents ?? []) {
    lines.push('| ' + finding.tag + ' | ' + finding.fingerprint + ' | ' + finding.repeatedIn.join('、') +
      ' | ' + (finding.undeclared.length === 0 ? '无' : finding.undeclared.join('、')) + ' |');
  }

  lines.push(
    '',
    '## 最近版本证据',
    '',
    '| 标签 | 标签时间 | 验收记录 | 提交到生产 | 失败状态 | 流程异常 |',
    '| --- | --- | --- | --- | --- | --- |',
  );

  for (const release of report.releases) {
    const leadTime = productionLeadHours(release.acceptance.metrics);
    lines.push('| ' + release.tag + ' | ' + release.createdAt.toISOString() + ' | ' +
      (release.acceptance.exists ? release.acceptance.path : '缺失') + ' | ' + metric(leadTime, ' 小时') + ' | ' +
      failureLabel(release.acceptance.metrics.changeFailure) + ' | ' +
      metric(release.acceptance.metrics.processIncidentCount) + ' |');
  }

  lines.push('', '> 正式发布频率按稳定标签时间统计，生产部署频率只按验收记录中的生产完成时间统计；标签时间不等于生产完成时间。变更失败率只以明确填报“是/否”的验收记录为分母。发布流程异常独立统计，不把基础设施或无效证据问题歪曲为产品失败，也不把缺失数据推断为成功。重复指纹只按已存在验收记录比较，缺失记录不推断为无重复；机器不判断根因与永久处置真实性。');

  if (report.preview) lines.push('', ...renderPreviewSection(report.preview));
  return lines.join('\n');
}

function renderPreviewSection(preview) {
  const lines = [
    '## 预览通道（RC）流程异常',
    '',
    '| 指标 | 结果 | 数据完整性 |',
    '| --- | --- | --- |',
    '| 窗口内 RC 标签数 | ' + preview.window.releaseCount + ' |  |',
    '| 有 RC 标签的自然日 | ' + preview.window.releaseDays + ' |  |',
    '| 单日最大 RC 标签数 | ' + preview.window.maxReleasesPerDay + ' |  |',
    '| RC 验收记录覆盖率 | ' + preview.acceptanceCount + '/' + preview.available +
      '（' + percentage(preview.acceptanceCoverage) + '） |  |',
    '| RC 流程异常合计 | ' + metric(preview.processIncidentCount) + ' | ' +
      preview.processIncidentReported + '/' + preview.available + ' |',
    '| 其中生产写操作开始后异常数 | ' + metric(preview.postProductionProcessIncidentCount) + ' | ' +
      preview.processIncidentReported + '/' + preview.available + ' |',
    '| 滚动 ' + PROCESS_INCIDENT_REPEAT_WINDOW + ' 个 RC 内重复的流程异常指纹数 | ' +
      preview.repeatedProcessIncidentCount + ' | ' + preview.acceptanceCount + '/' + preview.available + ' |',
    '',
    '### 按发布列车',
    '',
    '| 发布列车 | RC 数 | 流程异常 | 生产写操作开始后 | 重复指纹（发生于本列车） |',
    '| --- | --- | --- | --- | --- |',
  ];

  if (preview.trains.length === 0) lines.push('| 无 | 无 | 无 | 无 | 无 |');
  for (const train of preview.trains) {
    lines.push('| ' + train.train + (train.inFlight ? '（进行中）' : '') + ' | ' + train.previewCount + ' | ' +
      metric(train.processIncidentCount) + ' | ' + metric(train.postProductionProcessIncidentCount) + ' | ' +
      train.repeatedProcessIncidentCount + ' |');
  }

  lines.push(
    '',
    '### RC 重复流程异常指纹',
    '',
    '| RC | 指纹 | 滚动 ' + PROCESS_INCIDENT_REPEAT_WINDOW + ' 个 RC 内的更早 RC |',
    '| --- | --- | --- |',
  );
  if (preview.repeatedProcessIncidents.length === 0) lines.push('| 无 | 无 | 无 |');
  for (const finding of preview.repeatedProcessIncidents) {
    lines.push('| ' + finding.tag + ' | ' + finding.fingerprint + ' | ' + finding.repeatedIn.join('、') + ' |');
  }

  lines.push(
    '',
    '### RC 证据',
    '',
    '| 标签 | 标签时间 | 验收记录 | 流程异常 | 生产写操作开始后 |',
    '| --- | --- | --- | --- | --- |',
  );
  for (const release of preview.releases) {
    lines.push('| ' + release.tag + ' | ' + release.createdAt.toISOString() + ' | ' +
      (release.acceptance.exists ? release.acceptance.path : '缺失') + ' | ' +
      metric(release.acceptance.metrics.processIncidentCount) + ' | ' +
      metric(release.acceptance.metrics.postProductionProcessIncidentCount) + ' |');
  }

  lines.push('', '> 本节只统计预览通道，独立于上述正式版本指标，不并入发布频率、前置时间、变更失败率或稳定版重复指纹。RC 没有生产写操作，“生产写操作开始后异常数”预期为 0，非 0 表示记录或流程有误。滚动窗口按 RC 标签时间排序，可以跨发布列车：“重复指纹（发生于本列车）”按发生重复的那个较新 RC 归属，被匹配到的更早 RC 可能属于上一个列车，逐条归属以下表为准。RC 重复指纹只作观察：`historicalReleases` 目前只接受稳定版标签，RC 重复无法声明，因此不构成门禁，也不推断根因。指纹按精确字符串比较，同一根因写成不同指纹时不会被识别为重复。');
  return lines;
}

function help() {
  return [
    'Usage: node scripts/report-release-metrics.mjs [options]',
    '  --days <n>       Rolling calendar window (default: 14)',
    '  --releases <n>   Number of recent release tags (default: 20)',
    '  --format <type>  markdown or json (default: markdown)',
    '  --repo <path>    Repository root (default: current directory)',
    '  --ref <ref>      Evidence ref (default: origin/main, fallback: HEAD)',
    '  --now <date>     Override current time for reproducible checks',
    '  --validate-acceptance <path>  Validate one release record; repeat for multiple files',
  ].join('\n');
}

export function main(argv) {
  const options = parseArguments(argv);
  if (options.help) {
    process.stdout.write(help() + '\n');
    return;
  }
  if (options.acceptanceFiles.length > 0) {
    const errors = options.acceptanceFiles.flatMap((path) =>
      validateAcceptanceMetrics(readFileSync(path, 'utf8'), path));
    if (errors.length > 0) throw new Error('\n- ' + errors.join('\n- '));
    const historyErrors = options.acceptanceFiles.flatMap((path) =>
      validateProcessIncidentHistory(readAcceptanceHistory(path)));
    if (historyErrors.length > 0) throw new Error('\n- ' + [...new Set(historyErrors)].join('\n- '));
    process.stdout.write('Release acceptance metrics validation passed (' + options.acceptanceFiles.length + ' file(s)).\n');
    return;
  }
  runGit(options.repo, ['rev-parse', '--is-inside-work-tree']);
  options.evidenceRef = options.ref ?? (tryGit(options.repo, ['rev-parse', '--verify', 'origin/main^{commit}']) ? 'origin/main' : 'HEAD');
  options.evidenceCommit = runGit(options.repo, ['rev-parse', '--verify', options.evidenceRef + '^{commit}']);
  const stableReleases = collectReleases(options.repo, options.evidenceCommit, Number.MAX_SAFE_INTEGER);
  const report = summarizeReleaseMetrics(stableReleases, options);
  report.preview = summarizePreviewMetrics(
    collectPreviewReleases(options.repo, options.evidenceCommit, Number.MAX_SAFE_INTEGER),
    options,
    new Set(stableReleases.map((release) => release.tag)),
  );
  process.stdout.write((options.format === 'json' ? JSON.stringify(report, null, 2) : renderMarkdown(report)) + '\n');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    process.stderr.write('release metrics failed: ' + error.message + '\n');
    process.exitCode = 1;
  }
}
