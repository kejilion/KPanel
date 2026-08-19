#!/usr/bin/env node

import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, resolve, win32 as win32Path } from 'node:path';
import { fileURLToPath } from 'node:url';

const DEFAULT_POLICY = 'dependency-policy.json';
const REQUIRED_GROUPS = [
  'go-modules',
  'npm-packages',
  'go-toolchain',
  'node-toolchain',
  'docker-base-images',
  'dockerfile-frontend',
  'github-actions',
  'security-tools',
  'managed-kejilion-script',
];
const REQUIRED_FRESHNESS_TRIGGERS = [
  'dependency-policy.json',
  'go.mod',
  'go.sum',
  'web/package.json',
  'web/package-lock.json',
  'Dockerfile',
  'Makefile',
  'scripts/report-dependency-freshness.mjs',
  'scripts/security-scan.sh',
  'THIRD_PARTY_NOTICES.md',
  '.codex-workflows/**',
  '.github/workflows/**',
];

export function parseArguments(argv) {
  const options = {
    repo: process.cwd(),
    policy: DEFAULT_POLICY,
    format: 'markdown',
    output: null,
    validateOnly: false,
    allowPartial: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (['--repo', '--policy', '--format', '--output'].includes(argument)) {
      const value = argv[index + 1];
      if (value === undefined) throw new Error('Missing value for ' + argument);
      index += 1;
      if (argument === '--repo') options.repo = value;
      if (argument === '--policy') options.policy = value;
      if (argument === '--format') options.format = value;
      if (argument === '--output') options.output = value;
      continue;
    }
    if (argument === '--validate-only') options.validateOnly = true;
    else if (argument === '--allow-partial') options.allowPartial = true;
    else if (argument === '--help' || argument === '-h') options.help = true;
    else throw new Error('Unknown argument: ' + argument);
  }
  if (!['markdown', 'json'].includes(options.format)) throw new Error('--format must be markdown or json');
  options.repo = resolve(options.repo);
  options.policy = resolve(options.repo, options.policy);
  if (options.output) options.output = resolve(options.repo, options.output);
  return options;
}

export function normalizeVersion(value) {
  return String(value ?? '').trim().replace(/^v/, '');
}

export function npmInvocation(platform = process.platform, environment = process.env, nodeExecutable = process.execPath) {
  if (environment.NPM) return { command: environment.NPM, prefixArguments: [] };
  if (platform === 'win32') {
    return {
      command: nodeExecutable,
      prefixArguments: [environment.NPM_CLI_JS || win32Path.resolve(win32Path.dirname(nodeExecutable), 'node_modules/npm/bin/npm-cli.js')],
    };
  }
  return { command: 'npm', prefixArguments: [] };
}

export function goExecutable(environment = process.env) {
  return environment.GO || 'go';
}

export function isStableVersion(value) {
  const normalized = normalizeVersion(value);
  return /^\d+\.\d+\.\d+(?:\.\d+)?$/.test(normalized);
}

export function compareVersions(left, right) {
  const a = normalizeVersion(left).split('.').map(Number);
  const b = normalizeVersion(right).split('.').map(Number);
  if (!a.every(Number.isInteger) || !b.every(Number.isInteger)) return null;
  for (let index = 0; index < Math.max(a.length, b.length); index += 1) {
    const delta = (a[index] ?? 0) - (b[index] ?? 0);
    if (delta !== 0) return Math.sign(delta);
  }
  return 0;
}

export function classifyUpdate(current, candidate, componentKind = 'package') {
  if (!isStableVersion(candidate)) return 'prerelease';
  if (componentKind !== 'package') return 'major-toolchain-base';
  const currentParts = normalizeVersion(current).split('.').map(Number);
  const candidateParts = normalizeVersion(candidate).split('.').map(Number);
  if (!currentParts.every(Number.isInteger) || !candidateParts.every(Number.isInteger)) return 'major-toolchain-base';
  if (candidateParts[0] !== currentParts[0]) return 'major-toolchain-base';
  if (candidateParts[1] !== currentParts[1]) return 'minor';
  return 'compatible-patch';
}

export function validatePolicy(policy, repo) {
  const failures = [];
  const actionPins = new Map();
  if (policy.schemaVersion !== 1) failures.push('schemaVersion must be 1');
  const ids = new Set((policy.groups ?? []).map((group) => group.id));
  for (const id of REQUIRED_GROUPS) {
    if (!ids.has(id)) failures.push('missing required dependency group: ' + id);
  }
  for (const id of policy.requiredGroups ?? []) {
    if (!ids.has(id)) failures.push('requiredGroups references unknown group: ' + id);
  }
  for (const group of policy.groups ?? []) {
    if (!group.id || !group.detector || !Array.isArray(group.manifests) || group.manifests.length === 0) {
      failures.push('dependency group is incomplete: ' + (group.id ?? '<unknown>'));
      continue;
    }
    for (const manifest of group.manifests) {
      if (!existsSync(resolve(repo, manifest))) failures.push(group.id + ' manifest is missing: ' + manifest);
    }
  }
  const goToolchain = policy.groups?.find((group) => group.id === 'go-toolchain');
  if (!goToolchain?.manifests?.includes('.codex-workflows')) {
    failures.push('go-toolchain must cover .codex-workflows');
  }
  const goVersion = readFileSync(resolve(repo, 'go.mod'), 'utf8').match(/^go\s+(\d+\.\d+\.\d+)$/m)?.[1];
  if (!goVersion) {
    failures.push('go.mod must declare a three-part Go toolchain version');
  } else {
    const workflowDirectory = resolve(repo, '.codex-workflows');
    for (const filename of readdirSync(workflowDirectory).filter((name) => name.endsWith('.workflow.yaml'))) {
      const text = readFileSync(resolve(workflowDirectory, filename), 'utf8');
      for (const match of text.matchAll(/golang:(\d+\.\d+\.\d+)-(?:alpine|bookworm)(?:@(sha256:[0-9a-f]{64}))?/g)) {
        if (match[1] !== goVersion) {
          failures.push('.codex-workflows/' + filename + ': Go image must match go.mod ' + goVersion);
        }
        if (!match[2]) {
          failures.push('.codex-workflows/' + filename + ': Go image must use an immutable digest');
        }
      }
    }
  }
  const automation = policy.automationBoundary ?? {};
  if (automation.automaticDetection !== true || automation.automaticReport !== true || automation.automaticSecurityAdvisoryCheck !== true) {
    failures.push('automation boundary must require detection, reporting, and security advisory checks');
  }
  if (automation.automaticMainCommit !== false || automation.automaticRelease !== false || automation.automaticProductionDeployment !== false) {
    failures.push('automation boundary must prohibit automatic main, release, and production changes');
  }
  const requiredExceptionFields = new Set(policy.exceptionRequiredFields ?? []);
  for (const field of ['reason', 'owner', 'reviewDate', 'exitCondition', 'rollbackPoint']) {
    if (!requiredExceptionFields.has(field)) failures.push('exception policy is missing field: ' + field);
  }
  if (!Array.isArray(policy.exceptions)) failures.push('exceptions must be an array');
  for (const [index, exception] of (policy.exceptions ?? []).entries()) {
    for (const field of requiredExceptionFields) {
      if (exception[field] === undefined || exception[field] === null || exception[field] === '') {
        failures.push('exception ' + index + ' is missing field: ' + field);
      }
    }
    if (exception.reviewDate && Number.isNaN(new Date(exception.reviewDate).getTime())) {
      failures.push('exception ' + index + ' reviewDate is invalid');
    }
  }
  const cadence = policy.cadence ?? {};
  const cadenceLimits = {
    scheduledDetectionMaximumDays: 7,
    securityAdvisoryMaximumHours: 24,
    exceptionReviewMaximumDays: 31,
    eolReviewMaximumDays: 92,
  };
  for (const [field, maximum] of Object.entries(cadenceLimits)) {
    if (!Number.isInteger(cadence[field]) || cadence[field] <= 0) failures.push('cadence field must be a positive integer: ' + field);
    else if (cadence[field] > maximum) failures.push('cadence field exceeds the governed maximum: ' + field);
  }
  for (const field of ['scheduledDetectionCron', 'securityAdvisoryCron']) {
    if (!String(cadence[field] ?? '').trim()) failures.push('cadence field is missing: ' + field);
  }
  for (const exception of policy.exceptions ?? []) {
    const status = maintenanceStatus(policy).exceptions.find((item) =>
      item.component === exception.component && item.currentVersion === exception.currentVersion)?.status;
    if (status && status !== 'active') failures.push('dependency exception review is not active: ' + exception.component + ' (' + status + ')');
  }
  const reviewState = policy.reviewState ?? {};
  if (Number.isNaN(new Date(reviewState.lastEolReview).getTime())) failures.push('reviewState.lastEolReview is invalid');
  if (!reviewState.eolReviewEvidence || !existsSync(resolve(repo, reviewState.eolReviewEvidence))) {
    failures.push('reviewState.eolReviewEvidence must reference an existing file');
  }
  const qualification = policy.detectorQualification ?? {};
  if (qualification.candidateLevel !== 'version-channel-stable' || !Array.isArray(qualification.adoptionEvidenceStillRequired) || qualification.adoptionEvidenceStillRequired.length === 0) {
    failures.push('detector qualification must distinguish discovery from adoption evidence');
  }
  const freshnessWorkflow = readFileSync(resolve(repo, '.github/workflows/dependency-freshness.yml'), 'utf8');
  for (const trigger of REQUIRED_FRESHNESS_TRIGGERS) {
    if (!freshnessWorkflow.includes('- ' + trigger)) failures.push('dependency freshness push trigger is missing: ' + trigger);
  }
  for (const cron of [cadence.scheduledDetectionCron, cadence.securityAdvisoryCron].filter(Boolean)) {
    if (!freshnessWorkflow.includes('cron: "' + cron + '"')) failures.push('dependency freshness schedule is missing: ' + cron);
  }
  if (!freshnessWorkflow.includes('group: dependency-freshness-${{ github.ref }}')) {
    failures.push('dependency freshness concurrency must be isolated by ref');
  }
  for (const token of ['security-advisories:', 'make security-audit', 'bash scripts/security-scan.sh source', 'contents: read']) {
    if (!freshnessWorkflow.includes(token)) failures.push('dependency freshness security monitor is missing: ' + token);
  }
  if (/^\s+[A-Za-z0-9_-]+:\s*write\s*$/m.test(freshnessWorkflow)) {
    failures.push('dependency freshness workflow must not request write permissions');
  }
  const trivy = policy.groups?.find((group) => group.id === 'security-tools')?.components?.trivy;
  if (trivy) {
    const scanner = readFileSync(resolve(repo, 'scripts/security-scan.sh'), 'utf8');
    if (!scanner.includes(trivy.pinnedDigest)) failures.push('Trivy policy digest does not match scripts/security-scan.sh');
  }
  for (const filename of readdirSync(resolve(repo, '.github/workflows')).filter((name) => /\.ya?ml$/i.test(name))) {
    const lines = readFileSync(resolve(repo, '.github/workflows', filename), 'utf8').split(/\r?\n/);
    for (const [index, line] of lines.entries()) {
      if (!/uses:\s*[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+@/.test(line)) continue;
      if (!/uses:\s*[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+@[0-9a-f]{40}\s*#\s*v?\d+\.\d+\.\d+\s*$/.test(line)) {
        failures.push('.github/workflows/' + filename + ':' + (index + 1) + ': external action must use a full SHA and stable version comment');
        continue;
      }
      const match = line.match(/uses:\s*([A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)@([0-9a-f]{40})\s*#\s*(v?\d+\.\d+\.\d+)/);
      if (!actionPins.has(match[1])) actionPins.set(match[1], new Set());
      actionPins.get(match[1]).add(match[2] + '@' + normalizeVersion(match[3]));
    }
  }
  for (const [repository, pins] of actionPins) {
    if (pins.size > 1) failures.push(repository + ' uses inconsistent SHA/version pins across workflows');
  }
  const govulnPins = new Set(
    ['Makefile', '.github/workflows/ci.yml', '.github/workflows/release.yml']
      .flatMap((file) => [...readFileSync(resolve(repo, file), 'utf8').matchAll(/govulncheck@(v?\d+\.\d+\.\d+)/g)].map((match) => normalizeVersion(match[1]))),
  );
  if (govulnPins.size !== 1) failures.push('govulncheck must use one stable version across Makefile and CI/Release workflows');
  return failures;
}

function endOfUtcDate(value) {
  const text = String(value ?? '');
  return new Date(/^\d{4}-\d{2}-\d{2}$/.test(text) ? text + 'T23:59:59.999Z' : text);
}

export function maintenanceStatus(policy, now = new Date()) {
  const exceptions = (policy.exceptions ?? []).map((exception) => {
    const reviewAt = endOfUtcDate(exception.reviewDate);
    const maximumReviewAt = now.getTime() + (policy.cadence?.exceptionReviewMaximumDays ?? 0) * 86_400_000;
    const status = reviewAt.getTime() < now.getTime()
      ? 'due'
      : reviewAt.getTime() > maximumReviewAt
        ? 'review-window-exceeds-policy'
        : 'active';
    return { ...exception, status };
  });
  const lastEolReview = endOfUtcDate(policy.reviewState?.lastEolReview);
  const maximumDays = policy.cadence?.eolReviewMaximumDays;
  const hasEolReview = !Number.isNaN(lastEolReview.getTime()) && Number.isInteger(maximumDays) && maximumDays > 0;
  const nextEolReview = hasEolReview ? new Date(lastEolReview.getTime() + maximumDays * 86_400_000) : null;
  return {
    exceptions,
    exceptionActionRequiredCount: exceptions.filter((exception) => exception.status !== 'active').length,
    lastEolReview: policy.reviewState?.lastEolReview,
    nextEolReview: nextEolReview?.toISOString() ?? 'unreported',
    eolReviewEvidence: policy.reviewState?.eolReviewEvidence,
    eolReviewStatus: !nextEolReview ? 'unreported' : nextEolReview.getTime() < now.getTime() ? 'due' : 'current',
  };
}

function run(repo, command, arguments_, acceptedStatuses = [0]) {
  const result = spawnSync(command, arguments_, {
    cwd: repo,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  if (result.error) throw result.error;
  if (!acceptedStatuses.includes(result.status)) {
    throw new Error(command + ' exited ' + result.status + ': ' + String(result.stderr ?? '').trim());
  }
  return String(result.stdout ?? '').trim();
}

export function parseConcatenatedJson(input) {
  const values = [];
  let depth = 0;
  let start = -1;
  let inString = false;
  let escaped = false;
  for (let index = 0; index < input.length; index += 1) {
    const character = input[index];
    if (inString) {
      if (escaped) escaped = false;
      else if (character === '\\') escaped = true;
      else if (character === '"') inString = false;
      continue;
    }
    if (character === '"') inString = true;
    else if (character === '{') {
      if (depth === 0) start = index;
      depth += 1;
    } else if (character === '}') {
      depth -= 1;
      if (depth === 0 && start >= 0) {
        values.push(JSON.parse(input.slice(start, index + 1)));
        start = -1;
      }
    }
  }
  if (depth !== 0 || inString) throw new Error('incomplete concatenated JSON');
  return values;
}

export function requestHeaders(url, environment = process.env) {
  const host = new URL(url).hostname.toLowerCase();
  const result = { Accept: 'application/json', 'User-Agent': 'KPanel-Dependency-Freshness' };
  if (host === 'api.github.com') {
    result.Accept = 'application/vnd.github+json';
    if (environment.GITHUB_TOKEN) result.Authorization = 'Bearer ' + environment.GITHUB_TOKEN;
  }
  return result;
}

async function fetchJson(url) {
  const response = await fetch(url, { headers: requestHeaders(url), signal: AbortSignal.timeout(20_000) });
  if (!response.ok) throw new Error(url + ' returned HTTP ' + response.status);
  return response.json();
}

async function dockerHubTag(repository, tag) {
  return fetchJson('https://hub.docker.com/v2/repositories/' + repository + '/tags/' + encodeURIComponent(tag));
}

export function immutableDigestCandidate(component, currentDigest, upstreamDigest, source) {
  if (!upstreamDigest || currentDigest === upstreamDigest) return null;
  return {
    component,
    current: currentDigest.slice(0, 19),
    candidate: upstreamDigest.slice(0, 19),
    updateClass: 'major-toolchain-base',
    source,
  };
}

function candidate(component, current, latest, kind, source, extra = {}) {
  if (!latest || compareVersions(current, latest) >= 0) return null;
  return {
    component,
    current: normalizeVersion(current),
    candidate: normalizeVersion(latest),
    updateClass: classifyUpdate(current, latest, kind),
    source,
    ...extra,
  };
}

async function collectGoModules(repo) {
  const values = parseConcatenatedJson(run(repo, goExecutable(), ['list', '-m', '-u', '-json', 'all']));
  return values.filter((module) => module.Update?.Version && isStableVersion(module.Update.Version))
    .map((module) => candidate(module.Path, module.Version, module.Update.Version, 'package', 'go list -m -u'))
    .filter(Boolean);
}

async function collectNpm(repo) {
  const invocation = npmInvocation();
  const raw = run(repo, invocation.command, [...invocation.prefixArguments, 'outdated', '--all', '--json', '--prefix', 'web'], [0, 1]);
  if (!raw) return [];
  const packageJson = JSON.parse(readFileSync(resolve(repo, 'web/package.json'), 'utf8'));
  const direct = new Set([...Object.keys(packageJson.dependencies ?? {}), ...Object.keys(packageJson.devDependencies ?? {})]);
  const candidates = Object.entries(JSON.parse(raw)).flatMap(([name, value]) => {
    const entries = Array.isArray(value) ? value : [value];
    return entries.filter((entry) => entry.current).map((entry) => candidate(name, entry.current, entry.latest, 'package', 'npm outdated --all', {
      dependencyScope: direct.has(name) ? 'direct' : 'transitive',
      wanted: normalizeVersion(entry.wanted),
    })).filter(Boolean);
  });
  return [...new Map(candidates.map((item) => [item.component + '\0' + item.current + '\0' + item.candidate, item])).values()];
}

function currentToolchains(repo) {
  const dockerfile = readFileSync(resolve(repo, 'Dockerfile'), 'utf8');
  const goVersion = dockerfile.match(/golang:(\d+\.\d+\.\d+)-alpine/)?.[1];
  const nodeVersion = dockerfile.match(/node:(\d+\.\d+\.\d+)-alpine/)?.[1];
  if (!goVersion || !nodeVersion) throw new Error('Dockerfile toolchain versions could not be parsed');
  return { goVersion, nodeVersion, dockerfile };
}

async function collectToolchains(repo) {
  const current = currentToolchains(repo);
  const [goReleases, nodeReleases] = await Promise.all([
    fetchJson('https://go.dev/dl/?mode=json'),
    fetchJson('https://nodejs.org/dist/index.json'),
  ]);
  const latestGo = goReleases.map((release) => release.version).find(isStableVersion);
  const latestNode = nodeReleases.find((release) => release.lts && isStableVersion(release.version))?.version;
  return [
    candidate('Go toolchain', current.goVersion, latestGo, 'toolchain', 'go.dev stable releases'),
    candidate('Node.js LTS', current.nodeVersion, latestNode, 'toolchain', 'nodejs.org LTS releases'),
  ].filter(Boolean);
}

async function collectBaseImages(repo) {
  const dockerfile = readFileSync(resolve(repo, 'Dockerfile'), 'utf8');
  const matches = [...dockerfile.matchAll(/^FROM[^\n]*\s(node|golang):(\d+\.\d+\.\d+-alpine)@((?:sha256:)[0-9a-f]{64})/gm)];
  if (matches.length !== 2) throw new Error('expected pinned node and golang base images in Dockerfile');
  const results = await Promise.all(matches.map(async (match) => {
    const repository = 'library/' + match[1];
    const metadata = await dockerHubTag(repository, match[2]);
    return immutableDigestCandidate(match[1] + ':' + match[2] + ' index digest', match[3], metadata.digest, 'Docker Hub tag metadata');
  }));
  return results.filter(Boolean);
}

function workflowActionPins(repo) {
  const directory = resolve(repo, '.github/workflows');
  const pins = new Map();
  for (const filename of readdirSync(directory).filter((name) => /\.ya?ml$/i.test(name))) {
    const text = readFileSync(resolve(directory, filename), 'utf8');
    const pattern = /uses:\s*([A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)@([0-9a-f]{40})\s*#\s*(v?\d+\.\d+\.\d+)/g;
    for (const match of text.matchAll(pattern)) pins.set(match[1], { sha: match[2], version: match[3] });
  }
  return pins;
}

async function latestGitHubRelease(repository) {
  const release = await fetchJson('https://api.github.com/repos/' + repository + '/releases/latest');
  if (release.draft || release.prerelease || !isStableVersion(release.tag_name)) {
    throw new Error(repository + ' latest release is not a stable release');
  }
  return release.tag_name;
}

async function resolveGitHubTag(repository, tag) {
  let object = (await fetchJson('https://api.github.com/repos/' + repository + '/git/ref/tags/' + encodeURIComponent(tag))).object;
  for (let depth = 0; depth < 3 && object.type === 'tag'; depth += 1) {
    object = (await fetchJson('https://api.github.com/repos/' + repository + '/git/tags/' + object.sha)).object;
  }
  if (object.type !== 'commit') throw new Error(repository + ' tag ' + tag + ' does not resolve to a commit');
  return object.sha;
}

async function collectActions(repo) {
  const pins = workflowActionPins(repo);
  const results = await Promise.all([...pins].map(async ([repository, pin]) => {
    const [latest, expectedSha] = await Promise.all([
      latestGitHubRelease(repository),
      resolveGitHubTag(repository, pin.version),
    ]);
    const updates = [];
    if (pin.sha !== expectedSha) {
      updates.push({
        component: repository + ' pinned SHA integrity',
        current: pin.sha.slice(0, 12),
        candidate: expectedSha.slice(0, 12),
        updateClass: 'major-toolchain-base',
        source: 'GitHub tag object',
      });
    }
    const versionUpdate = candidate(repository, pin.version, latest, 'action', 'GitHub releases/latest', { pinnedSha: pin.sha });
    if (versionUpdate) updates.push(versionUpdate);
    return updates;
  }));
  return results.flat();
}

async function collectSecurityTools(repo, policy) {
  const makefile = readFileSync(resolve(repo, 'Makefile'), 'utf8');
  const govulnVersion = makefile.match(/govulncheck@(v?\d+\.\d+\.\d+)/)?.[1];
  if (!govulnVersion) throw new Error('govulncheck version could not be parsed');
  const tools = policy.groups.find((group) => group.id === 'security-tools').components;
  const [latestGovuln, latestTrivy, trivyTag] = await Promise.all([
    latestGitHubRelease(tools.govulncheck.repository),
    latestGitHubRelease(tools.trivy.repository),
    dockerHubTag('aquasec/trivy', tools.trivy.currentVersion),
  ]);
  const trivyDigests = new Set([trivyTag.digest, ...(trivyTag.images ?? []).map((image) => image.digest)]);
  if (!trivyDigests.has(tools.trivy.pinnedDigest)) {
    throw new Error('Trivy ' + tools.trivy.currentVersion + ' does not contain the pinned digest from dependency-policy.json');
  }
  return [
    candidate('govulncheck', govulnVersion, latestGovuln, 'scanner', 'GitHub releases/latest'),
    candidate('Trivy', tools.trivy.currentVersion, latestTrivy, 'scanner', 'GitHub releases/latest', { pinnedDigest: tools.trivy.pinnedDigest }),
  ].filter(Boolean);
}

async function collectDockerfileFrontend(repo) {
  const dockerfile = readFileSync(resolve(repo, 'Dockerfile'), 'utf8');
  const current = dockerfile.match(/^# syntax=docker\/dockerfile:(\d+\.\d+\.\d+)@sha256:/m)?.[1];
  if (!current) throw new Error('Dockerfile frontend version could not be parsed');
  const pinnedDigest = dockerfile.match(/^# syntax=docker\/dockerfile:\d+\.\d+\.\d+@(sha256:[0-9a-f]{64})/m)?.[1];
  const [response, currentTag] = await Promise.all([
    fetchJson('https://hub.docker.com/v2/repositories/docker/dockerfile/tags?page_size=100&ordering=last_updated'),
    dockerHubTag('docker/dockerfile', current),
  ]);
  const stable = response.results.map((tag) => tag.name).filter(isStableVersion)
    .sort((left, right) => compareVersions(right, left))[0];
  return [
    candidate('docker/dockerfile frontend', current, stable, 'frontend', 'Docker Hub stable tags'),
    immutableDigestCandidate('docker/dockerfile:' + current + ' digest', pinnedDigest, currentTag.digest, 'Docker Hub tag metadata'),
  ].filter(Boolean);
}

async function collectManagedScript(repo, policy) {
  const group = policy.groups.find((item) => item.id === 'managed-kejilion-script');
  const dockerfile = readFileSync(resolve(repo, 'Dockerfile'), 'utf8');
  const current = dockerfile.match(/raw\.githubusercontent\.com\/kejilion\/sh\/([0-9a-f]{40})\/kejilion\.sh/)?.[1];
  if (!current) throw new Error('managed kejilion.sh revision could not be parsed');
  const commit = await fetchJson('https://api.github.com/repos/' + group.repository + '/commits/' + group.branch);
  if (commit.sha === current) return [];
  return [{
    component: 'managed kejilion.sh',
    current: current.slice(0, 12),
    candidate: commit.sha.slice(0, 12),
    updateClass: 'major-toolchain-base',
    source: 'GitHub branch head',
  }];
}

async function collectSource(id, collector) {
  try {
    return { id, status: 'ok', candidates: await collector(), error: null };
  } catch (error) {
    return { id, status: 'error', candidates: [], error: error.message };
  }
}

export function summarize(policy, sources, now = new Date()) {
  const candidates = sources.flatMap((source) => source.candidates.map((item) => ({ ...item, group: source.id })));
  const classCounts = Object.fromEntries(
    ['emergency-security', 'compatible-patch', 'minor', 'major-toolchain-base', 'prerelease']
      .map((name) => [name, candidates.filter((item) => item.updateClass === name).length]),
  );
  return {
    generatedAt: now.toISOString(),
    policyVersion: policy.policyVersion,
    sourceCount: sources.length,
    successfulSources: sources.filter((source) => source.status === 'ok').length,
    failedSources: sources.filter((source) => source.status === 'error').length,
    candidateCount: candidates.length,
    classCounts,
    candidates,
    sources,
    maintenance: maintenanceStatus(policy, now),
    detectorQualification: policy.detectorQualification,
    decision: 'report-only; every adoption requires risk classification, evidence, and the existing integration authorization',
  };
}

export function renderMarkdown(report) {
  const lines = [
    '# KPanel 依赖新鲜度报告',
    '',
    '- 生成时间：' + report.generatedAt,
    '- 策略版本：' + report.policyVersion,
    '- 检测源完整性：' + report.successfulSources + '/' + report.sourceCount,
    '- 版本通道稳定候选数量：' + report.candidateCount,
    '- 候选资格：仅表示版本通道稳定；EOL、许可证、架构、行为、资源与回滚仍须采用决策证据。',
    '- 需处理依赖例外：' + report.maintenance.exceptionActionRequiredCount,
    '- EOL 复核：' + report.maintenance.eolReviewStatus + '（下次截止 ' + report.maintenance.nextEolReview + '）',
    '- 决策边界：本报告只发现候选，不代表应升级，也不授权提交、合入、发布或部署。',
    '',
    '## 版本通道稳定候选',
    '',
    '| 分类 | 数量 |',
    '| --- | ---: |',
    ...Object.entries(report.classCounts).map(([name, count]) => '| ' + name + ' | ' + count + ' |'),
    '',
    '| 分组 | 范围 | 组件 | 当前 | 候选 | 分类 | 来源 |',
    '| --- | --- | --- | --- | --- | --- | --- |',
  ];
  if (report.candidates.length === 0) lines.push('| - | - | 未发现更高稳定候选 | - | - | - | - |');
  for (const item of report.candidates) {
    lines.push('| ' + markdownCell(item.group) + ' | ' + markdownCell(item.dependencyScope ?? 'foundation') + ' | ' + markdownCell(item.component) + ' | ' + markdownCell(item.current) + ' | ' + markdownCell(item.candidate) + ' | ' + markdownCell(item.updateClass) + ' | ' + markdownCell(item.source) + ' |');
  }
  lines.push('', '## 例外与 EOL 复核', '', '| 组件 | 当前 | 候选 | 负责人 | 复核日期 | 状态 | 退出条件 |', '| --- | --- | --- | --- | --- | --- | --- |');
  if (report.maintenance.exceptions.length === 0) lines.push('| - | - | - | - | - | 无例外 | - |');
  for (const exception of report.maintenance.exceptions) {
    lines.push('| ' + markdownCell(exception.component) + ' | ' + markdownCell(exception.currentVersion) + ' | ' + markdownCell(exception.candidateVersion) + ' | ' + markdownCell(exception.owner) + ' | ' + markdownCell(exception.reviewDate) + ' | ' + markdownCell(exception.status) + ' | ' + markdownCell(exception.exitCondition) + ' |');
  }
  lines.push('', '- 最近 EOL 复核：' + report.maintenance.lastEolReview, '- 证据：`' + report.maintenance.eolReviewEvidence + '`');
  lines.push('', '## 检测源状态', '', '| 检测源 | 状态 | 错误 |', '| --- | --- | --- |');
  for (const source of report.sources) lines.push('| ' + markdownCell(source.id) + ' | ' + markdownCell(source.status) + ' | ' + markdownCell(source.error ?? '-') + ' |');
  lines.push('', '> “未发现候选”只有在所有检测源成功时才有效；任何检测源失败都必须按数据缺口处理。');
  return lines.join('\n');
}

function markdownCell(value) {
  const normalized = String(value ?? '-').replace(/\s+/g, ' ').replaceAll('|', '\\|').trim();
  return normalized.length > 320 ? normalized.slice(0, 317) + '...' : normalized;
}

function help() {
  return [
    'Usage: node scripts/report-dependency-freshness.mjs [options]',
    '  --repo <path>       Repository root (default: current directory)',
    '  --policy <path>     Policy path relative to repository (default: dependency-policy.json)',
    '  --format <type>     markdown or json (default: markdown)',
    '  --output <path>     Also write the rendered report to this path',
    '  --validate-only     Validate inventory and automation boundaries without network access',
    '  --allow-partial     Return success when an online source is unavailable',
  ].join('\n');
}

export async function main(argv) {
  const options = parseArguments(argv);
  if (options.help) {
    process.stdout.write(help() + '\n');
    return 0;
  }
  execFileSync('git', ['-C', options.repo, 'rev-parse', '--is-inside-work-tree'], { stdio: 'ignore' });
  const policy = JSON.parse(readFileSync(options.policy, 'utf8'));
  const failures = validatePolicy(policy, options.repo);
  if (failures.length > 0) throw new Error('dependency policy invalid:\n- ' + failures.join('\n- '));
  if (options.validateOnly) {
    process.stdout.write('Dependency policy validation passed (' + policy.groups.length + ' groups).\n');
    return 0;
  }
  const sources = await Promise.all([
    collectSource('go-modules', () => collectGoModules(options.repo)),
    collectSource('npm-packages', () => collectNpm(options.repo)),
    collectSource('toolchains-and-base-images', () => collectToolchains(options.repo)),
    collectSource('docker-base-images', () => collectBaseImages(options.repo)),
    collectSource('github-actions', () => collectActions(options.repo)),
    collectSource('security-tools', () => collectSecurityTools(options.repo, policy)),
    collectSource('dockerfile-frontend', () => collectDockerfileFrontend(options.repo)),
    collectSource('managed-kejilion-script', () => collectManagedScript(options.repo, policy)),
  ]);
  const report = summarize(policy, sources);
  const rendered = options.format === 'json' ? JSON.stringify(report, null, 2) : renderMarkdown(report);
  process.stdout.write(rendered + '\n');
  if (options.output) writeFileSync(options.output, rendered + '\n', 'utf8');
  if (report.failedSources > 0 && !options.allowPartial) return 2;
  if (report.maintenance.exceptionActionRequiredCount > 0 || report.maintenance.eolReviewStatus === 'due') return 3;
  return 0;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).then((status) => {
    process.exitCode = status;
  }).catch((error) => {
    process.stderr.write('dependency freshness report failed: ' + error.message + '\n');
    process.exitCode = 1;
  });
}
