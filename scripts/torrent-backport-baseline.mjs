#!/usr/bin/env node

// Bind scans to the verified upstream torrent module and the production graph.
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const MODULE = 'github.com/anacrolix/torrent';
const VERSION = 'v1.61.1-0.20260911233437-76452a2c8a2f';
const MODULE_SUM = 'h1:Qd2S/HI+0pR+EMyPS58AB573G3W92zSngOmH/e0qqP8=';
const SOURCE_MANIFEST_SHA256 = createHash('sha256').update(`${MODULE}@${VERSION} ${MODULE_SUM}`).digest('hex');
const repoRoot = resolve(import.meta.dirname, '..');
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const MAIN_MODULE = 'github.com/kejilion/kejilion-panel';
const BUILD_FILES = ['Dockerfile', '.github/workflows/release.yml', 'scripts/build-mcp-clients.sh', 'Makefile'];
const GO_VERSION = 'go1.27.3';
const SOURCE_ROOTS = ['cmd', 'internal'];
const ascii = (a, b) => {
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
};
const version = value => String(value ?? '').replace(/^v/, '');
export const productionTargets = Object.freeze(['linux', 'darwin', 'windows'].flatMap(goos =>
  ['amd64', 'arm64'].map(goarch => ({ goos, goarch, targets: goos === 'linux'
    ? ['./cmd/paneld', './cmd/kejilion-agent', './cmd/kejilion-node', './cmd/kpctl', './cmd/kpanel-mcp']
    : goos === 'windows' ? ['./cmd/kejilion-node', './cmd/kpanel-mcp'] : ['./cmd/kpanel-mcp'] }))));

export function productionClosure(packages, target, selected) {
  if (!Array.isArray(packages) || !packages.length) throw new Error('Empty production package closure');
  const graph = new Map(selected.map(m => [m.Path, m]));
  const paths = new Set();
  const modules = new Map();
  const normalized = packages.map(p => {
    if (!p.ImportPath || paths.has(p.ImportPath) || p.Error || (p.DepsErrors && (!Array.isArray(p.DepsErrors) || p.DepsErrors.length)) || p.Incomplete) throw new Error('Invalid production package closure');
    paths.add(p.ImportPath);
    if (p.Standard) return { path: p.ImportPath, module: 'stdlib', version: GO_VERSION };
    if (!Array.isArray(p.GoFiles) || !p.GoFiles.length || p.GoFiles.some(f => typeof f !== 'string' || /[/\\]/.test(f) || /(?:\.pb\.go|\.proto)$/.test(f))) throw new Error('Generated-code observation changed or GoFiles are missing');
    const m = p.Module;
    const expected = graph.get(m?.Path);
    if (!expected || m.Error || Boolean(m.Main) !== Boolean(expected.Main) || (m.Version ?? '') !== (expected.Version ?? '')
        || m.Replace || expected.Replace) throw new Error('Production module is outside the selected graph');
    if (!m.Main) {
      if (!m.Version) throw new Error('Production module is missing its version');
      modules.set(m.Path, { Path: m.Path, Version: m.Version });
    } else if (m.Path !== MAIN_MODULE) throw new Error('Unexpected production main module');
    const item = { path: p.ImportPath, module: m.Path, version: m.Version ?? '' };
    if (m.Main) {
      const moduleDir = m.Dir?.replaceAll('\\', '/').replace(/\/$/, '');
      const dir = p.Dir?.replaceAll('\\', '/');
      if (!moduleDir || !dir?.startsWith(moduleDir + '/')) throw new Error('Main-module source directory is outside the repository');
      const sourceDirectory = dir.slice(moduleDir.length + 1);
      if (!SOURCE_ROOTS.some(root => sourceDirectory === root || sourceDirectory.startsWith(root + '/')) || sourceDirectory.split('/').some(v => v === '..' || v === '.')) throw new Error('Main-module source directory is outside the source hash boundary');
      item.sourceDirectory = sourceDirectory;
      item.goFiles = [...p.GoFiles].sort(ascii);
    }
    return item;
  }).sort((a, b) => ascii(a.path, b.path));
  for (const path of target.targets) {
    if (!packages.some(p => p.ImportPath === MAIN_MODULE + path.slice(1) && p.Name === 'main' && p.Module?.Main)) throw new Error('Missing production entry point');
  }
  return { row: { ...target, packages: normalized }, modules: [...modules.values()].sort((a, b) => ascii(a.Path, b.Path)) };
}

export function firstPartySources(repo) {
  const files = [];
  for (const root of SOURCE_ROOTS) {
    const directory = join(repo, root);
    const stat = lstatSync(directory);
    if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error('First-party source root must be a real directory');
    for (const entry of readdirSync(directory, { recursive: true, withFileTypes: true })) {
      if (entry.isSymbolicLink()) throw new Error('First-party source hash boundary contains a symlink');
      if (!entry.isFile() || !/\.(?:go|proto)$/.test(entry.name)) continue;
      if (/(?:\.pb\.go|\.proto)$/.test(entry.name)) throw new Error('Generated protobuf source requires a new vulnerability review');
      const path = join(entry.parentPath, entry.name);
      files.push({ path: relative(repo, path).replaceAll('\\', '/'), sha256: hash(readFileSync(path, 'utf8').replaceAll('\r\n', '\n')) });
    }
  }
  if (!files.length) throw new Error('First-party source inventory is empty');
  files.sort((a, b) => ascii(a.path, b.path));
  return { files, sha256: hash(JSON.stringify(files)) };
}

export function scannerManifest(modules, scope) {
  if (!['selected', 'production'].includes(scope) || !modules.length || new Set(modules.map(m => m.Path)).size !== modules.length
      || modules.some(m => !/^[A-Za-z0-9._~+/-]+$/.test(m.Path) || !/^v[0-9][A-Za-z0-9.+-]*$/.test(m.Version))) throw new Error('Invalid scanner module inventory');
  return `module kpanel.local/diagnostics/${scope}\n\ngo 1.27.3\n\nrequire (\n${modules.map(m => `\t${m.Path} ${m.Version}`).join('\n')}\n)\n`;
}

export function validateScanScope(policy, identity, now = new Date()) {
  const date = value => /^\d{4}-\d{2}-\d{2}$/.test(value ?? '') ? Date.parse(value + 'T00:00:00Z') : NaN;
  const observed = date(policy.observedAt), expires = date(policy.expiresAt);
  if (policy.schemaVersion !== 1 || policy.observedAt !== '2026-11-06' || policy.expiresAt !== '2026-11-20'
      || !Number.isFinite(observed) || !Number.isFinite(expires) || now.getTime() < observed || now.getTime() >= expires
      || expires - observed > 14 * 86400000) throw new Error('Scan scope is not within its reviewed observation window');
  if (policy.sourceManifestSha256 !== SOURCE_MANIFEST_SHA256 || policy.sourceManifestSha256 !== identity.sourceManifestSha256
      || policy.closureSha256 !== identity.closureSha256 || !/^[a-f0-9]{64}$/.test(policy.closureSha256 ?? '')
      || policy.selectedGraphSha256 !== identity.selectedGraphSha256
      || policy.firstPartySourceSha256 !== identity.firstPartySourceSha256 || !/^[a-f0-9]{64}$/.test(policy.firstPartySourceSha256 ?? '')
      || JSON.stringify(policy.buildTargetFilesSha256) !== JSON.stringify(identity.buildTargetFilesSha256)) throw new Error('Scan scope source, targets or closure changed');
  if (policy.review?.state !== 'reviewed' || !policy.owner || !policy.review.reviewerRole
      || !/^[a-f0-9]{64}$/.test(policy.review.evidenceSha256 ?? '')) throw new Error('Scan scope requires independent review');
  if (!Array.isArray(policy.findings) || policy.findings.length !== 38) throw new Error('Unexpected bounded scan scope');
  const keys = new Set();
  for (const f of policy.findings) {
    const key = [f.scope, f.kind, f.id, f.module, f.version].join('|');
    if (!['selected', 'standalone'].includes(f.scope) || !['vulnerability', 'misconfiguration'].includes(f.kind)
        || !/^(?:CVE-\d{4}-\d+|GHSA-[a-z0-9-]+|DS-\d{4})$/.test(f.id ?? '')
        || !f.module || !/^v\d/.test(f.version ?? '') || !['HIGH', 'CRITICAL'].includes(f.severity)
        || !f.reason || !['not-in-production', 'superseded-by-selected-version', 'unused-upstream-development-recipe'].includes(f.disposition)
        || keys.has(key)) throw new Error('Invalid or duplicate bounded scan finding');
    keys.add(key);
  }
  if (policy.findings.filter(f => f.scope === 'selected' && f.kind === 'vulnerability').length !== 10
      || policy.findings.filter(f => f.scope === 'standalone' && f.kind === 'vulnerability').length !== 26
      || policy.findings.filter(f => f.scope === 'standalone' && f.kind === 'misconfiguration').length !== 2) throw new Error('Unexpected bounded scan finding kinds');
}

export function verifyScanReport(report, inventory, exitCode, scope, policy, production, selected) {
  if (!['production', 'selected', 'standalone'].includes(scope) || ![0, 1].includes(exitCode)
      || report.SchemaVersion !== 2 || report.ArtifactType !== 'filesystem' || report.ArtifactName !== '/inventory'
      || !Array.isArray(report.Results) || !report.Results.length || report.Error || report.Errors) throw new Error('Invalid scanner report or exit status');
  const expected = new Map(inventory.map(m => [m.Path, version(m.Version)]));
  if (!expected.size || expected.size !== inventory.length) throw new Error('Invalid expected scanner inventory');
  const found = new Map();
  const findings = [];
  let manifests = 0, configurations = 0;
  const rootName = scope === 'standalone' ? MODULE : `kpanel.local/diagnostics/${scope}`;
  for (const result of report.Results) {
    if (result.Error || result.Errors || result.Secrets?.length || result.SuppressedFindings?.length) throw new Error('Unexpected scanner result');
    if (result.Type === 'gomod') {
      if (++manifests !== 1 || result.Class !== 'lang-pkgs' || result.Target !== 'go.mod' || !Array.isArray(result.Packages) || result.Misconfigurations?.length) throw new Error('Invalid Go inventory target');
      let roots = 0;
      for (const p of result.Packages) {
        if (p.Name === rootName && !p.Version && p.Relationship === 'root') { if (++roots > 1) throw new Error('Duplicate inventory root'); continue; }
        if (!expected.has(p.Name) || expected.get(p.Name) !== version(p.Version) || found.has(p.Name)) throw new Error('Scanner package inventory differs from the resolved modules');
        found.set(p.Name, version(p.Version));
      }
      for (const f of result.Vulnerabilities ?? []) {
        if (!expected.has(f.PkgName) || expected.get(f.PkgName) !== version(f.InstalledVersion)) throw new Error('Finding is outside scanner inventory');
        findings.push({ kind: 'vulnerability', id: f.VulnerabilityID, module: f.PkgName, version: 'v' + version(f.InstalledVersion), severity: f.Severity });
      }
    } else if (scope === 'standalone' && result.Type === 'dockerfile' && result.Target === 'Dockerfile') {
      if (++configurations !== 1 || result.Class !== 'config' || result.Vulnerabilities?.length || result.Packages?.length) throw new Error('Invalid standalone configuration result');
      for (const f of result.Misconfigurations ?? []) {
        if (f.Status !== 'FAIL') throw new Error('Unexpected configuration finding status');
        findings.push({ kind: 'misconfiguration', id: f.ID, module: MODULE, version: VERSION, severity: f.Severity });
      }
    } else throw new Error('Unexpected scanner target');
  }
  if (manifests !== 1 || found.size !== expected.size || configurations !== (scope === 'standalone' ? 1 : 0)) throw new Error('Scanner inventory is incomplete');
  if (findings.some(f => !['HIGH', 'CRITICAL'].includes(f.severity)) || exitCode !== (findings.length ? 1 : 0)) throw new Error('Scanner findings and actual exit status disagree');
  if (scope === 'production' && findings.length) throw new Error('Production dependency vulnerabilities block release');
  const productionNames = new Set(production.map(m => m.Path));
  const selectedVersions = new Map(selected.map(m => [m.Path, m.Version]));
  const keys = new Set();
  for (const f of findings) {
    const key = [f.kind, f.id, f.module, f.version].join('|');
    if (keys.has(key)) throw new Error('Duplicate scanner finding');
    keys.add(key);
    const allowed = policy.findings.find(a => a.scope === scope && a.kind === f.kind && a.id === f.id && a.module === f.module && a.version === f.version && a.severity === f.severity);
    if (!allowed) throw new Error('Unreviewed scanner finding: ' + key);
    if (f.kind === 'misconfiguration') {
      if (allowed.disposition !== 'unused-upstream-development-recipe') throw new Error('Invalid development recipe disposition');
    } else if (allowed.disposition === 'not-in-production') {
      if (productionNames.has(f.module)) throw new Error('Previously nonproduction module is now in production');
    } else if (allowed.disposition === 'superseded-by-selected-version') {
      if (scope !== 'standalone' || !selectedVersions.has(f.module) || selectedVersions.get(f.module) === f.version) throw new Error('Standalone finding is not superseded by the selected version');
    } else throw new Error('Invalid vulnerability disposition');
  }
  return { scope, actualScannerExitCode: exitCode, packageCount: found.size, findingsRetained: findings,
    disposition: scope === 'production' ? 'production-scan-passed' : 'complete-inventory-with-reviewed-findings-retained' };
}

export function baselineMod(source) {
  if (!new RegExp('^[ \\t]*(?:require[ \\t]+)?github\\.com/anacrolix/torrent[ \\t]+' + VERSION.replaceAll('.', '\\.') + '[ \\t]*\\r?$', 'm').test(source)
      || /^replace\s+github\.com\/anacrolix\/torrent\b/m.test(source)) throw new Error('Expected the exact unmodified upstream torrent version');
  return source;
}

export function parseModuleJSON(text) {
  const values = [];
  let depth = 0, start = -1, quoted = false, escaped = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (escaped) escaped = false;
      else if (c === '\\') escaped = true;
      else if (c === '"') quoted = false;
      continue;
    }
    if (c === '"') quoted = true;
    else if (c === '{') { if (depth++ === 0) start = i; }
    else if (c === '}') {
      if (--depth < 0) throw new Error('Malformed module graph');
      if (depth === 0) values.push(JSON.parse(text.slice(start, i + 1)));
    } else if (depth === 0 && !/\s/.test(c)) throw new Error('Malformed module graph');
  }
  if (depth || quoted || !values.length) throw new Error('Incomplete module graph');
  return values;
}

export function verifyModuleDeclarations(actual, baseline) {
  if ((actual.Replace ?? []).length || actual.Require?.filter(r => r.Path === MODULE && r.Version === VERSION).length !== 1) throw new Error('Unexpected torrent module declaration');
  const normalizeEmptyReplace = declaration => {
    const normalized = { ...declaration };
    const absent = normalized.Replace === undefined || normalized.Replace === null;
    if (!absent && !Array.isArray(normalized.Replace)) throw new Error('Invalid replacement declarations');
    // Go 1.27.3 omits an empty Replace field; null and [] are equivalent here.
    // No other field is normalized: every remaining declaration must match exactly.
    if (absent || normalized.Replace.length === 0) delete normalized.Replace;
    return normalized;
  };
  if (JSON.stringify(normalizeEmptyReplace(actual)) !== JSON.stringify(normalizeEmptyReplace(baseline))) throw new Error('Baseline changed other module declarations');
}

export function verifyModuleGraphs(actual, baseline) {
  for (const graph of [actual, baseline]) {
    if (graph.some(m => !m.Path || m.Error || m.Replace) || new Set(graph.map(m => m.Path)).size !== graph.length) throw new Error('Invalid module graph');
  }
  const current = actual.find(m => m.Path === MODULE);
  const original = baseline.find(m => m.Path === MODULE);
  if (current?.Version !== VERSION || current.Replace
      || original?.Version !== VERSION || original.Replace) throw new Error('Torrent baseline identity mismatch');
  const normalized = graph => graph.map(m => ({ Path: m.Path, Version: m.Version ?? '',
    ...(m.Path !== MODULE && m.Replace ? { Replace: { Path: m.Replace.Path, Version: m.Replace.Version ?? '' } } : {}) }))
    .sort((a, b) => a.Path.localeCompare(b.Path));
  if (JSON.stringify(normalized(actual)) !== JSON.stringify(normalized(baseline))) throw new Error('Baseline changed the dependency graph');
  return hash(JSON.stringify(normalized(baseline)));
}

function validateUpstreamDeclarations(repo, environment) {
  baselineMod(readFileSync(join(repo, 'go.mod'), 'utf8'));
  const declarations = JSON.parse(go(['mod', 'edit', '-json'], repo, moduleEnvironment(undefined, environment)));
  verifyModuleDeclarations(declarations, declarations);
  const sumLine = `${MODULE} ${VERSION} ${MODULE_SUM}`;
  if (readFileSync(join(repo, 'go.sum'), 'utf8').split(/\r?\n/).filter(line => line === sumLine).length !== 1) throw new Error('Upstream torrent checksum declaration mismatch');
  return { module: MODULE, version: VERSION, source: 'unmodified-upstream', moduleSum: MODULE_SUM,
    sourceManifestSha256: SOURCE_MANIFEST_SHA256 };
}

function upstreamDownload(repo, env) {
  const item = JSON.parse(go(['mod', 'download', '-json', MODULE + '@' + VERSION], repo, env));
  if (item.Path !== MODULE || item.Version !== VERSION || item.Sum !== MODULE_SUM || !item.Dir || !item.Zip) throw new Error('Upstream torrent archive identity mismatch');
  return item;
}

export function verifyUpstream(repo, { offline = false, environment = process.env } = {}) {
  const provenance = validateUpstreamDeclarations(repo, environment);
  const env = moduleEnvironment(undefined, offline ? { ...environment, GOPROXY: 'off', GOSUMDB: 'off' } : environment);
  const download = upstreamDownload(repo, env);
  go(['mod', 'verify'], repo, env);
  return { provenance: { ...provenance, archiveSha256: hash(readFileSync(download.Zip)) }, directory: download.Dir };
}

// Governance preflight is offline and fails if the pinned archive has not yet
// been fetched or the local extracted module has been modified.
export function validateBackport(repo, environment = process.env) {
  return verifyUpstream(repo, { offline: true, environment }).provenance;
}

export function moduleEnvironment(modfile, environment = process.env) {
  if (/(?:^|\s)["']?-mod(?:file)?(?:=|\s|$)/.test(environment.GOFLAGS ?? '')) throw new Error('Conflicting GOFLAGS module override');
  if (modfile && /["\r\n]/.test(modfile)) throw new Error('Unsupported module-file path');
  // Go's quoted.Split only recognizes a quote at the start of the entire argument.
  return { ...environment, GOWORK: 'off', GOFLAGS: [environment.GOFLAGS ?? '', '-mod=readonly',
    ...(modfile ? [`"-modfile=${modfile.replaceAll('\\', '/')}"`] : [])].join(' ').trim() };
}

function go(args, cwd, env) {
  return execFileSync('go', args, { cwd, env, encoding: 'utf8', timeout: 180_000, maxBuffer: 16 * 1024 * 1024, windowsHide: true });
}

function prepare(repo, directory, basename) {
  if (!lstatSync(directory).isDirectory() || lstatSync(directory).isSymbolicLink() || readdirSync(directory).length) throw new Error('Baseline directory must be empty and private');
  const { provenance } = verifyUpstream(repo);
  const sourceMod = readFileSync(join(repo, 'go.mod'), 'utf8');
  const sourceSum = readFileSync(join(repo, 'go.sum'));
  const modfile = join(directory, basename + '.mod');
  const sumfile = join(directory, basename + '.sum');
  writeFileSync(modfile, baselineMod(sourceMod), { flag: 'wx', mode: 0o600 });
  writeFileSync(sumfile, sourceSum, { flag: 'wx', mode: 0o600 });
  const initial = { sourceMod: hash(sourceMod), sourceSum: hash(sourceSum), baselineMod: hash(readFileSync(modfile)), baselineSum: hash(readFileSync(sumfile)) };
  const actualEnvironment = moduleEnvironment();
  const baselineEnvironment = moduleEnvironment(modfile);
  const actualMod = JSON.parse(go(['mod', 'edit', '-json'], repo, actualEnvironment));
  const originalMod = JSON.parse(go(['mod', 'edit', '-json', '-modfile=' + modfile], repo, actualEnvironment));
  verifyModuleDeclarations(actualMod, originalMod);
  const actualGraph = go(['list', '-m', '-json', 'all'], repo, actualEnvironment);
  const originalGraph = go(['list', '-m', '-json', 'all'], repo, baselineEnvironment);
  const graphSha256 = verifyModuleGraphs(parseModuleJSON(actualGraph), parseModuleJSON(originalGraph));
  const unchanged = () => {
    const current = { sourceMod: hash(readFileSync(join(repo, 'go.mod'))), sourceSum: hash(readFileSync(join(repo, 'go.sum'))), baselineMod: hash(readFileSync(modfile)), baselineSum: hash(readFileSync(sumfile)) };
    if (JSON.stringify(initial) !== JSON.stringify(current)) throw new Error('Scanner changed the module manifests');
  };
  unchanged();
  console.log(JSON.stringify({ torrentBaseline: 'qualified-upstream-version-coverage', module: MODULE, version: VERSION,
    graphSha256, sourceManifestSha256: provenance.sourceManifestSha256, ...initial }));
  return { environment: baselineEnvironment, unchanged, actualGraph, originalGraph, graphSha256, initial, modfile };
}

function buildTargetHashes(repo) {
  // Git checkouts on Windows may use CRLF; bind the exact UTF-8 source with LF line endings.
  return Object.fromEntries(BUILD_FILES.map(file => [file, hash(readFileSync(join(repo, file), 'utf8').replaceAll('\r\n', '\n'))]));
}

function evidence(label, raw, exitCode = 0) {
  console.log(JSON.stringify({ rawEvidence: label, sha256: hash(raw), bytes: Buffer.byteLength(raw), exitCode, raw }));
}

function packageOutput(repo, args, env, label) {
  const result = spawnSync('go', args, { cwd: repo, env, encoding: 'utf8', timeout: 180_000, maxBuffer: 64 * 1024 * 1024, windowsHide: true });
  evidence(label + '.stdout', result.stdout ?? '', result.status);
  evidence(label + '.stderr', result.stderr ?? '', result.status);
  if (result.error || result.signal || result.status !== 0) throw new Error('Production dependency inspection failed: ' + label);
  return result.stdout;
}

function scanInventories(repo, directory) {
  const policy = JSON.parse(readFileSync(join(repo, 'scripts/torrent-backport-scan-scope.json'), 'utf8'));
  const baseline = prepare(repo, directory, 'baseline');
  evidence('selected.actual', baseline.actualGraph);
  evidence('selected.original', baseline.originalGraph);
  const actual = parseModuleJSON(baseline.actualGraph), original = parseModuleJSON(baseline.originalGraph);
  if (actual.filter(m => m.Main && m.Path === MAIN_MODULE).length !== 1 || actual.some(m => m.Replace)) throw new Error('Unexpected scanner graph replacement or main module');
  const selected = original.filter(m => !m.Main).map(({ Path, Version }) => ({ Path, Version })).sort((a, b) => ascii(a.Path, b.Path));
  const environment = { ...process.env, GOFLAGS: '', GOWORK: 'off', GOTOOLCHAIN: 'local', CGO_ENABLED: '0', GOAMD64: 'v1', GOARM64: 'v8.0', GOEXPERIMENT: '' };
  if (go(['env', 'GOVERSION'], repo, environment).trim() !== GO_VERSION) throw new Error('Production inventory requires the fixed Go toolchain');
  const rows = [], union = new Map();
  for (const target of productionTargets) {
    const env = { ...environment, GOOS: target.goos, GOARCH: target.goarch };
    const args = ['list', '-deps', '-json=ImportPath,Name,Standard,Module,Dir,GoFiles,Error,DepsErrors,Incomplete', ...target.targets];
    const current = productionClosure(parseModuleJSON(packageOutput(repo, args, moduleEnvironment(undefined, env), `${target.goos}-${target.goarch}.actual`)), target, actual);
    const conservative = productionClosure(parseModuleJSON(packageOutput(repo, args, moduleEnvironment(baseline.modfile, env), `${target.goos}-${target.goarch}.original`)), target, original);
    if (JSON.stringify(current) !== JSON.stringify(conservative)) throw new Error('Original-version production closure differs from actual source');
    rows.push(current.row);
    for (const m of current.modules) union.set(m.Path, m);
  }
  const production = [...union.values()].sort((a, b) => ascii(a.Path, b.Path));
  if (!production.some(m => m.Path === MODULE && m.Version === VERSION)) throw new Error('Production inventory lost the torrent identity');
  const standaloneDir = verifyUpstream(repo).directory;
  const standaloneMod = JSON.parse(go(['mod', 'edit', '-json', '-modfile=' + join(standaloneDir, 'go.mod')], repo, moduleEnvironment()));
  if (standaloneMod.Module?.Path !== MODULE || standaloneMod.Replace?.length) throw new Error('Unexpected standalone module declarations');
  const standalone = standaloneMod.Require.map(({ Path, Version }) => ({ Path, Version })).sort((a, b) => ascii(a.Path, b.Path));
  const sources = firstPartySources(repo);
  const sourceFiles = new Set(sources.files.map(f => f.path));
  for (const p of rows.flatMap(r => r.packages).filter(p => p.module === MAIN_MODULE)) {
    if (p.goFiles.some(file => !sourceFiles.has(p.sourceDirectory + '/' + file))) throw new Error('Production GoFiles are missing from the first-party source inventory');
  }
  const identity = { sourceManifestSha256: SOURCE_MANIFEST_SHA256, selectedGraphSha256: baseline.graphSha256,
    buildTargetFilesSha256: buildTargetHashes(repo), closureSha256: hash(JSON.stringify(rows)), firstPartySourceSha256: sources.sha256 };
  validateScanScope(policy, identity);
  const inventories = { selected, production, standalone };
  for (const scope of ['selected', 'production', 'standalone']) {
    const destination = join(directory, scope); mkdirSync(destination, { mode: 0o700 });
    if (scope === 'standalone') {
      for (const file of ['go.mod', 'go.sum', 'Dockerfile']) writeFileSync(join(destination, file), readFileSync(join(standaloneDir, file)), { flag: 'wx', mode: 0o600 });
    } else writeFileSync(join(destination, 'go.mod'), scannerManifest(inventories[scope], scope), { flag: 'wx', mode: 0o600 });
  }
  baseline.unchanged();
  const record = { schemaVersion: 1, identity, inventories, rows, firstPartySources: sources.files, sourceManifests: { mod: baseline.initial.sourceMod, sum: baseline.initial.sourceSum },
    policySha256: hash(readFileSync(join(repo, 'scripts/torrent-backport-scan-scope.json'))),
    manifests: Object.fromEntries(Object.keys(inventories).map(scope => [scope, hash(readFileSync(join(directory, scope, 'go.mod')))])) };
  const raw = JSON.stringify(record);
  writeFileSync(join(directory, 'inventory.json'), raw, { flag: 'wx', mode: 0o600 });
  evidence('qualified-scan-inventory', raw);
}

function checkReport(repo, directory, scope, reportPath, exitCode, stderrPath) {
  const record = JSON.parse(readFileSync(join(directory, 'inventory.json'), 'utf8'));
  const policyRaw = readFileSync(join(repo, 'scripts/torrent-backport-scan-scope.json'));
  const policy = JSON.parse(policyRaw);
  verifyUpstream(repo);
  if (record.schemaVersion !== 1 || hash(policyRaw) !== record.policySha256
      || hash(readFileSync(join(repo, 'go.mod'), 'utf8')) !== record.sourceManifests.mod
      || hash(readFileSync(join(repo, 'go.sum'))) !== record.sourceManifests.sum
      || !record.inventories[scope] || hash(readFileSync(join(directory, scope, 'go.mod'))) !== record.manifests[scope]) throw new Error('Scan inputs changed');
  validateScanScope(policy, { ...record.identity, buildTargetFilesSha256: buildTargetHashes(repo), firstPartySourceSha256: firstPartySources(repo).sha256 });
  const stderr = readFileSync(stderrPath, 'utf8');
  if (/(?:^|\s)(?:FATAL|ERROR)(?:\s|$)|"level"\s*:\s*"(?:fatal|error)"/im.test(stderr)) throw new Error('Scanner reported an operational error');
  const result = verifyScanReport(JSON.parse(readFileSync(reportPath, 'utf8')), record.inventories[scope], Number(exitCode), scope, policy, record.inventories.production, record.inventories.selected);
  console.log(JSON.stringify({ torrentScan: result, policySha256: record.policySha256, identity: record.identity }));
}

function run(command, args, cwd, env) {
  const result = spawnSync(command, args, { cwd, env, stdio: 'inherit', timeout: 15 * 60 * 1000, windowsHide: true });
  if (result.error || result.signal || result.status !== 0) throw new Error('Baseline command failed: ' + command + ' exit=' + result.status + ' ' + (result.error?.message ?? result.signal ?? ''));
}

export function main(argv) {
  if (argv[0] === 'scan-inventories' && argv.length === 2) {
    scanInventories(repoRoot, resolve(argv[1]));
    return;
  }
  if (argv[0] === 'check-report' && argv.length === 6) {
    checkReport(repoRoot, resolve(argv[1]), argv[2], resolve(argv[3]), argv[4], resolve(argv[5]));
    return;
  }
  if (argv[0] === 'manifest' && argv.length === 2) {
    const directory = resolve(argv[1]);
    // Go expects go.mod/go.sum; the helper's alternate basename is used for govulncheck.
    prepare(repoRoot, directory, 'go');
    return;
  }
  if (argv[0] !== 'govuln' || argv.length !== 1) throw new Error('usage: torrent-backport-baseline.mjs govuln|manifest DIRECTORY|scan-inventories DIRECTORY|check-report DIRECTORY SCOPE REPORT EXIT STDERR');
  const directory = mkdtempSync(join(tmpdir(), 'kpanel-torrent-baseline-'));
  try {
    const baseline = prepare(repoRoot, directory, 'baseline');
    const makefile = readFileSync(join(repoRoot, 'Makefile'), 'utf8');
    const pins = [...new Set([...makefile.matchAll(/govulncheck@(v\d+\.\d+\.\d+)/g)].map(m => m[1]))];
    if (pins.length !== 1) throw new Error('Expected one fixed govulncheck version in Makefile');
    const bin = join(directory, 'bin');
    mkdirSync(bin, { mode: 0o700 });
    run('go', ['install', 'golang.org/x/vuln/cmd/govulncheck@' + pins[0]], repoRoot, { ...process.env, GOWORK: 'off', GOFLAGS: '', GOBIN: bin });
    const scanner = join(bin, 'govulncheck' + (process.platform === 'win32' ? '.exe' : ''));
    console.log('torrent_baseline_govuln=start tool=' + pins[0] + ' tool_sha256=' + hash(readFileSync(scanner)));
    run(scanner, ['./...'], repoRoot, baseline.environment);
    baseline.unchanged();
    console.log('torrent_baseline_govuln=pass coverage=verified-upstream-version');
  } finally {
    // The path is created in this invocation, outside the checkout; never accept a caller path here.
    if (resolve(directory).startsWith(resolve(tmpdir()) + (process.platform === 'win32' ? '\\' : '/'))) rmSync(directory, { recursive: true, force: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { main(process.argv.slice(2)); }
  catch (error) { process.stderr.write('torrent_baseline=fail ' + error.message + '\n'); process.exitCode = 1; }
}
