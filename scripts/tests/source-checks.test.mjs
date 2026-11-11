import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, unlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { runPlan } from '../run-source-checks.mjs';
import { resolveBash } from '../run-repo-bash.mjs';
import { baselineMod, firstPartySources, moduleEnvironment, parseModuleJSON, productionClosure, productionTargets, scannerManifest, validateBackport, validateScanScope, verifyModuleDeclarations, verifyModuleGraphs, verifyScanReport } from '../torrent-backport-baseline.mjs';

const repo = resolve(import.meta.dirname, '../..');
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-source-plan-'));
  return { root, run: (lanes, options = {}) => runPlan({ lanes, cwd: root, evidenceDir: join(root, 'evidence'), ...options }),
    cleanup: () => rmSync(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }) };
}
const lane = (id, code) => ({ id, argv: [process.execPath, '-e', code] });

const torrentVersion = 'v1.61.1-0.20260911233437-76452a2c8a2f';
test('torrent baseline requires the exact upstream version and rejects local replacements', () => {
  const source = `module example.test/panel\n\ngo 1.27.4\nrequire github.com/anacrolix/torrent ${torrentVersion}\n`;
  assert.equal(baselineMod(source), source);
  assert.throws(() => baselineMod(source.replace(torrentVersion, 'v1.61.0')), /exact unmodified/);
  assert.throws(() => baselineMod(source + `replace github.com/anacrolix/torrent ${torrentVersion} => ./third_party/torrent\n`), /exact unmodified/);
});

function moduleDeclarations() {
  // Shape observed from the fixed Go 1.27.4 `go mod edit -json`: empty Replace is omitted.
  const baseline = { Module: { Path: 'example.test/panel' }, Go: '1.27.4',
    Require: [{ Path: 'github.com/anacrolix/torrent', Version: torrentVersion }, { Path: 'example.test/other', Version: 'v1.0.0', Indirect: true }] };
  return { baseline, actual: structuredClone(baseline) };
}

test('torrent declaration comparison accepts only equivalent empty Replace representations', () => {
  const { actual, baseline } = moduleDeclarations();
  for (const empty of [baseline, { ...baseline, Replace: null }, { ...baseline, Replace: [] }]) {
    assert.doesNotThrow(() => verifyModuleDeclarations(actual, empty));
  }
  assert.equal(Object.hasOwn(baseline, 'Replace'), false, 'comparison must not mutate input');
  assert.equal(Object.hasOwn(actual, 'Replace'), false);
  for (const Replace of ['', {}, false, 0]) {
    assert.throws(() => verifyModuleDeclarations(actual, { ...baseline, Replace }), /Invalid replacement declarations/);
  }
});

test('torrent declaration comparison preserves every other field and replacement exactly', () => {
  const { actual, baseline } = moduleDeclarations();
  const otherReplacement = { Old: { Path: 'example.test/other', Version: 'v1.0.0' }, New: { Path: 'example.test/fork', Version: 'v1.0.1' } };
  assert.throws(() => verifyModuleDeclarations({ ...actual, Replace: [otherReplacement] }, baseline), /Unexpected torrent module declaration/);
  for (const key of ['Module', 'Go', 'Toolchain', 'Require', 'Exclude', 'Retract', 'Tool', 'Godebug', 'FutureField']) {
    const changed = { ...structuredClone(baseline), [key]: { changed: true } };
    assert.throws(() => verifyModuleDeclarations(actual, changed), /changed other module declarations/, key);
  }
  const changedPin = structuredClone(baseline); changedPin.Require[1].Version = 'v1.0.1';
  assert.throws(() => verifyModuleDeclarations(actual, changedPin), /changed other module declarations/);
  // Even another field's null/empty/omitted distinction remains strict.
  assert.throws(() => verifyModuleDeclarations({ ...actual, Exclude: [] }, baseline), /changed other module declarations/);
  assert.throws(() => verifyModuleDeclarations({ ...actual, Exclude: null }, { ...baseline, Exclude: [] }), /changed other module declarations/);
});

test('torrent declaration comparison rejects missing, duplicate and changed torrent identities', () => {
  const { actual, baseline } = moduleDeclarations();
  for (const Replace of [[{ Old: { Path: 'github.com/anacrolix/torrent', Version: torrentVersion }, New: { Path: './third_party/torrent' } }]]) {
    assert.throws(() => verifyModuleDeclarations({ ...actual, Replace }, baseline), /Unexpected torrent module declaration/);
  }
  for (const mutate of [
    value => { value.Require[0].Version = 'v1.62.0'; },
    value => { value.Require.shift(); },
    value => { value.Require.push(structuredClone(value.Require[0])); },
  ]) {
    const changed = structuredClone(actual); mutate(changed);
    assert.throws(() => verifyModuleDeclarations(changed, baseline), /Unexpected torrent module declaration/);
  }
  assert.doesNotThrow(() => verifyModuleDeclarations(actual, actual));
});

test('torrent graph qualification rejects missing identities, other dependency changes and malformed output', () => {
  const original = [{ Path: 'example.test/panel', Main: true }, { Path: 'github.com/anacrolix/torrent', Version: torrentVersion }, { Path: 'example.test/other', Version: 'v1.0.0' }];
  const actual = structuredClone(original);
  assert.match(verifyModuleGraphs(actual, original), /^[a-f0-9]{64}$/);
  assert.deepEqual(parseModuleJSON(original.map(x => JSON.stringify(x, null, 2)).join('\n')), original);
  assert.throws(() => verifyModuleGraphs(actual, []), /identity mismatch/);
  const changed = structuredClone(original); changed[2].Version = 'v1.0.1';
  assert.throws(() => verifyModuleGraphs(actual, changed), /changed the dependency graph/);
  changed[2] = { ...original[2], Replace: { Path: 'example.test/fork', Version: 'v1.0.0' } };
  assert.throws(() => verifyModuleGraphs(actual, changed), /Invalid module graph/);
  assert.throws(() => verifyModuleGraphs(actual, [...original, original[1]]), /Invalid module graph/);
  for (const malformed of ['', '{', '{}junk', '{"Path":"unfinished}', '}']) assert.throws(() => parseModuleJSON(malformed), /module graph/);
});

test('baseline Go flags quote the whole modfile argument and reject conflicting overrides', () => {
  const environment = moduleEnvironment('C:\\task directory\\baseline.mod', { GOFLAGS: '-tags=netgo' });
  assert.equal(environment.GOWORK, 'off');
  assert.equal(environment.GOFLAGS, '-tags=netgo -mod=readonly "-modfile=C:/task directory/baseline.mod"');
  for (const GOFLAGS of ['-mod=mod', '-modfile=other.mod', '"-modfile=with spaces.mod"', "'-mod=mod'"]) {
    assert.throws(() => moduleEnvironment('/tmp/baseline.mod', { GOFLAGS }), /Conflicting GOFLAGS/);
  }
  assert.throws(() => moduleEnvironment('/tmp/"bad/baseline.mod', {}), /Unsupported module-file path/);
});

test('torrent provenance and scanner inventory retain the upstream module identity', () => {
  const provenance = validateBackport(repo);
  assert.equal(provenance.source, 'unmodified-upstream');
  assert.equal(provenance.version, torrentVersion);
  assert.equal(provenance.sourceManifestSha256, createHash('sha256').update(`${provenance.module}@${provenance.version} ${provenance.moduleSum}`).digest('hex'));
  const makefile = readFileSync(join(repo, 'Makefile'), 'utf8');
  assert.match(makefile, /security-audit-go:\s+\$\(GO\) run golang\.org\/x\/vuln\/cmd\/govulncheck@v1\.6\.0 \.\/\.\.\.\s+node scripts\/torrent-backport-baseline\.mjs govuln/);
  for (const workflow of ['ci.yml', 'release.yml']) assert.match(readFileSync(join(repo, '.github/workflows', workflow), 'utf8'), /run: make security-audit-go/);
  const scan = readFileSync(join(repo, 'scripts/security-scan.sh'), 'utf8');
  assert.match(scan, /--scanners secret/);
  assert.match(scan, /--scanners vuln,misconfig/);
  assert.match(scan, /--format json \\\s+--list-all-pkgs/);
  assert.match(scan, /torrent-backport-baseline\.mjs" check-report/);
  assert.match(scan, /baseline_root="\$repo_root\/\.codex-tmp"/);
  assert.match(scan, /mktemp -d "\$baseline_root\/torrent-baseline\.XXXXXXXX"/);
});

test('production closure retains exact selected versions and rejects incomplete or missing entry points', () => {
  const main = { Path: 'github.com/kejilion/kejilion-panel', Main: true, Dir: '/repo' };
  const dependency = { Path: 'github.com/anacrolix/torrent', Version: torrentVersion };
  const selected = [main, dependency];
  const target = { goos: 'linux', goarch: 'amd64', targets: ['./cmd/paneld'] };
  const packages = [
    { ImportPath: main.Path + '/cmd/paneld', Name: 'main', Module: main, Dir: '/repo/cmd/paneld', GoFiles: ['main.go'] },
    { ImportPath: dependency.Path, Name: 'torrent', Module: dependency, GoFiles: ['torrent.go'] },
    { ImportPath: 'fmt', Name: 'fmt', Standard: true },
  ];
  const closure = productionClosure(packages, target, selected);
  assert.deepEqual(closure.modules, [{ Path: dependency.Path, Version: dependency.Version }]);
  assert.deepEqual(closure.row.packages[0], { path: 'fmt', module: 'stdlib', version: 'go1.27.4' });
  assert.deepEqual(productionClosure([...packages].reverse(), target, selected), closure);
  for (const bad of [[], packages.slice(1), [...packages, packages[0]], packages.map(p => ({ ...p, Incomplete: true })), packages.map(p => ({ ...p, DepsErrors: [{}] }))]) {
    assert.throws(() => productionClosure(bad, target, selected));
  }
  for (const change of [{ Version: 'v1.62.0' }, { Path: 'example.invalid/unselected' }, { Error: { Err: 'missing module' } }, { Replace: { Path: './elsewhere' } }]) {
    const changed = structuredClone(packages); changed[1].Module = { ...dependency, ...change };
    assert.throws(() => productionClosure(changed, target, selected), /outside the selected graph/);
  }
  const changed = structuredClone(packages); changed[0].Name = 'library';
  assert.throws(() => productionClosure(changed, target, selected), /entry point/);
  for (const patch of [{ GoFiles: undefined }, { GoFiles: ['generated.pb.go'] }, { GoFiles: ['../outside.go'] }, { Dir: '/repo/other' }, { Dir: '/outside/cmd/paneld' }]) {
    const changed = structuredClone(packages); Object.assign(changed[0], patch);
    assert.throws(() => productionClosure(changed, target, selected), /Generated-code|source directory/);
  }
});

const scopePolicy = () => JSON.parse(readFileSync(join(repo, 'scripts/torrent-backport-scan-scope.json'), 'utf8'));
const scopeIdentity = policy => Object.fromEntries(['sourceManifestSha256', 'selectedGraphSha256', 'buildTargetFilesSha256', 'closureSha256', 'firstPartySourceSha256'].map(key => [key, structuredClone(policy[key])]));

test('bounded scan scope expires, binds source and targets, and requires exact independently reviewed findings', () => {
  const policy = scopePolicy(), identity = scopeIdentity(policy), observed = new Date('2026-11-06T12:00:00Z');
  assert.doesNotThrow(() => validateScanScope(policy, identity, observed));
  for (const date of ['2026-11-05T23:59:59Z', '2026-11-20T00:00:00Z']) assert.throws(() => validateScanScope(policy, identity, new Date(date)), /window/);
  const stale = { ...policy, observedAt: '2026-10-23', expiresAt: '2026-11-06' };
  assert.throws(() => validateScanScope(stale, identity, new Date('2026-10-24T12:00:00Z')), /window/);
  for (const key of ['sourceManifestSha256', 'selectedGraphSha256', 'closureSha256', 'buildTargetFilesSha256', 'firstPartySourceSha256']) {
    assert.throws(() => validateScanScope(policy, { ...identity, [key]: 'changed' }, observed), /changed/);
  }
  for (const mutate of [p => { p.review.state = 'pending'; }, p => { p.review.evidenceSha256 = ''; }, p => { p.findings[0].id = '*'; }, p => { p.findings[1] = p.findings[0]; }, p => { p.findings.push(p.findings[0]); }]) {
    const changed = structuredClone(policy); mutate(changed);
    assert.throws(() => validateScanScope(changed, identity, observed));
  }
  assert.equal(policy.findings.filter(f => f.scope === 'selected').length, 10);
  assert.equal(policy.findings.length, 38);
  assert.equal(policy.findings.filter(f => f.scope === 'standalone').length, 28);
});

test('release inventory matrix and build-file hashes cover every supported release environment', () => {
  assert.deepEqual(productionTargets.map(t => `${t.goos}/${t.goarch}`), ['linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64', 'windows/amd64', 'windows/arm64']);
  for (const target of productionTargets) assert.deepEqual(target.targets, target.goos === 'linux'
    ? ['./cmd/paneld', './cmd/kejilion-agent', './cmd/kejilion-node', './cmd/kpctl', './cmd/kpanel-mcp'] : target.goos === 'windows' ? ['./cmd/kejilion-node', './cmd/kpanel-mcp'] : ['./cmd/kpanel-mcp']);
  const policy = scopePolicy();
  assert.equal(firstPartySources(repo).sha256, policy.firstPartySourceSha256, 'first-party source observation changed');
  assert.deepEqual(Object.keys(policy.buildTargetFilesSha256), ['Dockerfile', '.github/workflows/release.yml', 'scripts/build-mcp-clients.sh', 'Makefile']);
  for (const [path, expected] of Object.entries(policy.buildTargetFilesSha256)) {
    assert.equal(createHash('sha256').update(readFileSync(join(repo, path), 'utf8').replaceAll('\r\n', '\n')).digest('hex'), expected, 'build target contract changed: ' + path);
  }
});

test('first-party source observation binds content and rejects generated code and symlink roots', () => {
  const f = fixture();
  try {
    mkdirSync(join(f.root, 'cmd')); mkdirSync(join(f.root, 'internal'));
    writeFileSync(join(f.root, 'cmd/main.go'), 'package main\r\n');
    const observed = firstPartySources(f.root);
    writeFileSync(join(f.root, 'cmd/main.go'), 'package main\n');
    assert.deepEqual(firstPartySources(f.root), observed, 'line endings are normalized');
    writeFileSync(join(f.root, 'cmd/main.go'), 'package changed\n');
    assert.notEqual(firstPartySources(f.root).sha256, observed.sha256);
    for (const name of ['generated.pb.go', 'messages.proto']) {
      writeFileSync(join(f.root, 'internal', name), 'generated');
      assert.throws(() => firstPartySources(f.root), /Generated protobuf/);
      unlinkSync(join(f.root, 'internal', name));
    }
    rmSync(join(f.root, 'cmd'), { recursive: true });
    symlinkSync(join(f.root, 'internal'), join(f.root, 'cmd'), process.platform === 'win32' ? 'junction' : 'dir');
    assert.throws(() => firstPartySources(f.root), /real directory/);
  } finally { f.cleanup(); }
});

function scanReport(inventory, findings = []) {
  return { SchemaVersion: 2, ArtifactType: 'filesystem', ArtifactName: '/inventory', Results: [{ Target: 'go.mod', Class: 'lang-pkgs', Type: 'gomod', Packages: inventory.map(m => ({ Name: m.Path, Version: m.Version })), Vulnerabilities: findings }] };
}

test('scanner inventory requires every resolved module and genuine exit semantics', () => {
  const inventory = [{ Path: 'github.com/anacrolix/torrent', Version: 'v1.61.0' }, { Path: 'example.invalid/dependency', Version: 'v2.0.0' }];
  const verify = (report, exit = 0) => verifyScanReport(report, inventory, exit, 'production', scopePolicy(), inventory, inventory);
  assert.equal(verify(scanReport(inventory)).packageCount, 2);
  for (const change of [r => r.Results[0].Packages.pop(), r => r.Results[0].Packages.push(r.Results[0].Packages[0]), r => { r.Results[0].Packages[0].Version = 'v1.62.0'; }, r => { r.Results[0].Packages[0].Name = 'unknown'; }, r => { r.Results[0].Target = 'other/go.mod'; }, r => { r.Results[0].Error = 'DB unavailable'; }, r => { r.SchemaVersion = 3; }]) {
    const report = scanReport(inventory); change(report); assert.throws(() => verify(report));
  }
  for (const exit of [1, 2, 125, NaN]) assert.throws(() => verify(scanReport(inventory), exit));
  const finding = { VulnerabilityID: 'CVE-2026-12345', PkgName: inventory[0].Path, InstalledVersion: inventory[0].Version, Severity: 'HIGH' };
  assert.throws(() => verify(scanReport(inventory, [finding]), 0), /exit status/);
  assert.throws(() => verify(scanReport(inventory, [finding]), 1), /Production dependency/);
});

test('selected and standalone findings remain visible and cannot allow unknown IDs or production intersections', () => {
  const policy = scopePolicy();
  const entry = policy.findings.find(f => f.scope === 'selected');
  const inventory = [{ Path: entry.module, Version: entry.version }];
  const finding = { VulnerabilityID: entry.id, PkgName: entry.module, InstalledVersion: entry.version, Severity: entry.severity };
  const verify = (report, exit = 1, production = []) => verifyScanReport(report, inventory, exit, 'selected', policy, production, inventory);
  assert.equal(verify(scanReport(inventory, [finding])).findingsRetained[0].id, entry.id);
  assert.equal(verify(scanReport(inventory), 0).findingsRetained.length, 0, 'resolved database findings need not persist forever');
  assert.throws(() => verify(scanReport(inventory, [finding]), 1, inventory), /now in production/);
  assert.throws(() => verify(scanReport(inventory, [{ ...finding, VulnerabilityID: 'CVE-2026-999999' }])), /Unreviewed/);
  assert.throws(() => verify(scanReport(inventory, [finding, finding])), /Duplicate/);
  const old = policy.findings.find(f => f.scope === 'standalone' && f.disposition === 'superseded-by-selected-version');
  const oldInventory = [{ Path: old.module, Version: old.version }];
  const report = scanReport(oldInventory, [{ VulnerabilityID: old.id, PkgName: old.module, InstalledVersion: old.version, Severity: old.severity }]);
  report.Results.push({ Target: 'Dockerfile', Class: 'config', Type: 'dockerfile', Misconfigurations: [] });
  assert.throws(() => verifyScanReport(report, oldInventory, 1, 'standalone', policy, [], oldInventory), /not superseded/);
  assert.equal(verifyScanReport(report, oldInventory, 1, 'standalone', policy, [], [{ Path: old.module, Version: 'v99.0.0' }]).findingsRetained.length, 1);
  const xText = policy.findings.find(f => f.scope === 'standalone' && f.id === 'CVE-2026-56851');
  assert.equal(xText?.module, 'golang.org/x/text');
  assert.equal(xText?.version, 'v0.31.0');
  assert.equal(xText?.disposition, 'superseded-by-selected-version');
  assert.match(xText?.reason ?? '', /below v0\.41\.0; KPanel selected and production inventories use v0\.43\.0/);
  const xTextInventory = [{ Path: xText.module, Version: xText.version }];
  const xTextFinding = { VulnerabilityID: xText.id, PkgName: xText.module, InstalledVersion: xText.version, Severity: xText.severity };
  const xTextReport = scanReport(xTextInventory, [xTextFinding]);
  xTextReport.Results.push({ Target: 'Dockerfile', Class: 'config', Type: 'dockerfile', Misconfigurations: [] });
  assert.equal(verifyScanReport(xTextReport, xTextInventory, 1, 'standalone', policy, [], [{ Path: xText.module, Version: 'v0.43.0' }]).findingsRetained[0].id, xText.id);
  assert.throws(() => verifyScanReport(xTextReport, xTextInventory, 1, 'standalone', policy, [], xTextInventory), /not superseded/);
  const xNet = policy.findings.find(f => f.scope === 'standalone' && f.id === 'CVE-2026-78661');
  assert.equal(xNet?.module, 'golang.org/x/net');
  assert.equal(xNet?.version, 'v0.47.0');
  assert.equal(xNet?.disposition, 'superseded-by-selected-version');
  assert.match(xNet?.reason ?? '', /selected and production inventories use v0\.62\.0/);
  const xNetInventory = [{ Path: xNet.module, Version: xNet.version }];
  const xNetReport = scanReport(xNetInventory, [{ VulnerabilityID: xNet.id, PkgName: xNet.module, InstalledVersion: xNet.version, Severity: xNet.severity }]);
  xNetReport.Results.push({ Target: 'Dockerfile', Class: 'config', Type: 'dockerfile', Misconfigurations: [] });
  assert.equal(verifyScanReport(xNetReport, xNetInventory, 1, 'standalone', policy, [], [{ Path: xNet.module, Version: 'v0.62.0' }]).findingsRetained[0].id, xNet.id);
  assert.throws(() => verifyScanReport(xNetReport, xNetInventory, 1, 'standalone', policy, [], xNetInventory), /not superseded/);
  const selected = [{ Path: old.module, Version: 'v99.0.0' }];
  assert.throws(() => verifyScanReport({ ...report, Results: report.Results.slice(0, 1) }, oldInventory, 1, 'standalone', policy, [], selected), /incomplete/);
  const config = policy.findings.find(f => f.kind === 'misconfiguration');
  const configFinding = { ID: config.id, Severity: config.severity, Status: 'FAIL' };
  report.Results[1].Misconfigurations.push(configFinding);
  assert.equal(verifyScanReport(report, oldInventory, 1, 'standalone', policy, [], selected).findingsRetained.length, 2);
  configFinding.Status = 'PASS';
  assert.throws(() => verifyScanReport(report, oldInventory, 1, 'standalone', policy, [], selected), /configuration finding status/);
});

test('scanner-only manifests include all exact modules and reject malformed identities', () => {
  const modules = [{ Path: 'github.com/anacrolix/torrent', Version: 'v1.61.0' }, { Path: 'example.invalid/module/v2', Version: 'v2.0.0' }];
  const text = scannerManifest(modules, 'selected');
  assert.match(text, /github.com\/anacrolix\/torrent v1\.61\.0/);
  assert.match(text, /example\.invalid\/module\/v2 v2\.0\.0/);
  assert.doesNotMatch(text, /replace/);
  for (const bad of [[], [...modules, modules[0]], [{ Path: 'bad\npath', Version: 'v1.0.0' }], [{ Path: 'good/path', Version: 'latest' }]]) assert.throws(() => scannerManifest(bad, 'production'));
});

test('bounded parallel source checks retain every log and ordered terminal result', async () => {
  const f = fixture();
  try {
    const report = await f.run(['web', 'go', 'deploy'].map((id) => lane(id, 'console.log("begin");setTimeout(()=>console.log("end"),350)')));
    assert.equal(report.status, 'passed');
    assert.deepEqual(report.lanes.map((x) => x.id), ['web', 'go', 'deploy']);
    const [web, go, deploy] = report.lanes;
    assert.ok(Date.parse(go.startedAt) < Date.parse(web.finishedAt), 'independent checks must overlap');
    assert.ok(Date.parse(deploy.startedAt) >= Math.min(Date.parse(web.finishedAt), Date.parse(go.finishedAt)), 'third lane must wait for a slot');
    for (const result of report.lanes) assert.match(readFileSync(result.log, 'utf8'), /begin\r?\nend/);
    assert.equal(JSON.parse(readFileSync(join(f.root, 'evidence/result.json'))).status, 'passed');
    await assert.rejects(f.run([lane('web', 'process.exit(0)')]), /EEXIST/);
  } finally { f.cleanup(); }
});

test('failure cancels active work and never starts queued checks', async () => {
  const f = fixture();
  try {
    const report = await f.run([lane('web', 'setTimeout(()=>process.exit(7),150)'), lane('go', 'setTimeout(()=>{},10000)'), lane('deploy', 'console.log("must not run")')]);
    assert.equal(report.status, 'failed');
    assert.equal(report.lanes[0].exitCode, 7);
    assert.equal(report.lanes[1].status, 'cancelled');
    assert.equal(report.lanes[2].status, 'not-run');
    assert.ok(!existsSync(join(f.root, 'evidence/deploy.log')));
  } finally { f.cleanup(); }
});

test('timeout kills the owned descendant process tree', { timeout: 15_000 }, async () => {
  const f = fixture();
  try {
    const heartbeat = join(f.root, 'heartbeat');
    const descendant = 'const fs=require("node:fs");setInterval(()=>fs.appendFileSync(process.argv[1],"x"),30)';
    const report = await f.run([lane('web', 'require("node:child_process").spawn(process.execPath,["-e",' + JSON.stringify(descendant) + ',' + JSON.stringify(heartbeat) + '],{stdio:"inherit"});setInterval(()=>{},1000)')], { timeoutMs: 3_000 });
    assert.equal(report.status, 'failed');
    assert.equal(report.lanes[0].status, 'timeout');
    assert.ok(existsSync(heartbeat), 'descendant must actually run before cancellation');
    await sleep(100);
    const stopped = readFileSync(heartbeat, 'utf8');
    await sleep(150);
    assert.equal(readFileSync(heartbeat, 'utf8'), stopped, 'descendant must stop writing');
  } finally { f.cleanup(); }
});

test('signal cancellation, candidate change, damaged logs and output overflow cannot produce success', async () => {
  for (const mode of ['signal', 'identity', 'evidence', 'output']) {
    const f = fixture();
    try {
      const pending = f.run([lane('web', mode === 'output' ? 'process.stdout.write(Buffer.alloc(33*1024*1024))' : 'setTimeout(()=>{},200)')],
        { verifyIdentity: () => { if (mode === 'evidence') writeFileSync(join(f.root, 'evidence/web.log'), 'corrupted'); return mode !== 'identity'; } });
      if (mode === 'signal') { await sleep(80); process.emit('SIGTERM'); }
      const report = await pending;
      assert.equal(report.status, 'failed', mode);
      assert.equal(JSON.parse(readFileSync(join(f.root, 'evidence/result.json'))).status, 'failed', mode);
    } finally { f.cleanup(); }
  }
});

test('invalid plans and missing executables fail before success', async () => {
  const f = fixture();
  try {
    await assert.rejects(f.run([lane('web', 'process.exit(0)')], { jobs: 3 }), /jobs/);
    await assert.rejects(f.run([lane('web', 'process.exit(0)'), lane('web', 'process.exit(0)')]), /duplicate/);
    const report = await f.run([{ id: 'web', argv: [join(f.root, 'missing-executable')] }]);
    assert.equal(report.status, 'failed');
  } finally { f.cleanup(); }
});

test('fixed lanes preserve source command coverage and stop on an early failed command', () => {
  const f = fixture();
  try {
    mkdirSync(join(f.root, 'web'));
    mkdirSync(join(f.root, 'scripts'));
    cpSync(join(repo, 'scripts/verify-source-lane.sh'), join(f.root, 'scripts/verify-source-lane.sh'));
    writeFileSync(join(f.root, 'scripts/verify-deploy.sh'), 'printf "deploy-check\\n" >>"$CHECK_COMMANDS"\n');
    const envFile = join(f.root, 'env.sh');
    writeFileSync(envFile, 'npm() { printf "npm %s\\n" "$*" >>"$CHECK_COMMANDS"; [ "${FAIL_TYPECHECK:-}" != 1 ] || [ "$*" != "run typecheck" ]; }; go() { printf "go %s\\n" "$*" >>"$CHECK_COMMANDS"; if [[ "${FAIL_TORRENT:-}" == 1 && "$*" == *github.com/anacrolix/torrent* ]]; then return 1; fi; }; export -f npm go\n');
    const log = join(f.root, 'commands');
    const envPath = envFile.replace(/^([A-Za-z]):/, (_, drive) => '/' + drive.toLowerCase()).replaceAll('\\', '/');
    const run = (id, extra = {}) => spawnSync(resolveBash(), ['scripts/verify-source-lane.sh', id], { cwd: f.root, encoding: 'utf8', env: { ...process.env, BASH_ENV: envPath, CHECK_COMMANDS: log, ...extra } });
    for (const id of ['web', 'go', 'deploy']) assert.equal(run(id).status, 0);
    const torrentRegression = 'go test -race github.com/anacrolix/torrent -run ^TestMsgWriter(BroadcastDuringFillWakesWriter|KeepAliveTimerRearmed)$ -count=1 -timeout=1m';
    const bencodeRegression = 'go test -race github.com/anacrolix/torrent/bencode -run ^TestDecodeMaxDepth -count=1 -timeout=1m';
    assert.deepEqual(readFileSync(log, 'utf8').trim().split('\n'), ['npm ci', 'npm run typecheck', 'npm test', 'npm run build', 'go test ./...', 'go test -race ./internal/panel ./internal/auth ./internal/dockerx', torrentRegression, bencodeRegression, 'go vet ./...', 'deploy-check']);
    writeFileSync(log, '');
    assert.notEqual(run('web', { FAIL_TYPECHECK: '1' }).status, 0);
    assert.equal(readFileSync(log, 'utf8').trim(), 'npm ci\nnpm run typecheck');
    writeFileSync(log, '');
    assert.notEqual(run('go', { FAIL_TORRENT: '1' }).status, 0);
    assert.deepEqual(readFileSync(log, 'utf8').trim().split('\n'), ['go test ./...', 'go test -race ./internal/panel ./internal/auth ./internal/dockerx', torrentRegression]);
    assert.notEqual(run('arbitrary').status, 0);
    const missing = run('preflight');
    assert.notEqual(missing.status, 0);
    assert.match(missing.stderr, /go.mod.*web\/package.json.*web\/package-lock.json/);
    for (const [path, expected] of [['.github/workflows/release.yml', /node scripts\/run-source-checks\.mjs/], ['scripts/verify-change.sh', /node scripts\/run-source-checks\.mjs/]]) assert.match(readFileSync(join(repo, path), 'utf8'), expected);
  } finally { f.cleanup(); }
});

test('the CLI qualifies a real clean Git checkout and records actual identity changes as failure', () => {
  const f = fixture();
  try {
    const checkout = join(f.root, 'checkout');
    mkdirSync(join(checkout, 'web'), { recursive: true }); mkdirSync(join(checkout, 'scripts'));
    cpSync(join(repo, 'scripts/verify-source-lane.sh'), join(checkout, 'scripts/verify-source-lane.sh'));
    writeFileSync(join(checkout, 'scripts/verify-deploy.sh'), 'exit 0\n');
    for (const path of ['go.mod', 'web/package.json', 'web/package-lock.json', 'notes.md']) writeFileSync(join(checkout, path), 'fixture\n');
    const git = (...args) => execFileSync('git', ['-C', checkout, ...args], { stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8' }).trim();
    git('init', '--initial-branch=main'); git('config', 'user.name', 'KPanel Test'); git('config', 'user.email', 'kpanel@example.invalid'); git('add', '.'); git('commit', '-m', 'test: clean source');
    const candidate = git('rev-parse', 'HEAD');
    mkdirSync(join(checkout, 'release')); writeFileSync(join(checkout, 'release/DRAFT_NOTES.md'), 'generated notes\n');
    const envFile = join(f.root, 'env.sh');
    writeFileSync(envFile, 'npm() { echo "fixture npm $*"; }; go() { echo "fixture go $*"; if [ "${CHANGE_SOURCE:-}" = 1 ] && [ "$*" = "test ./..." ]; then printf changed >notes.md; fi; }; export -f npm go\n');
    const envPath = envFile.replace(/^([A-Za-z]):/, (_, drive) => '/' + drive.toLowerCase()).replaceAll('\\', '/');
    const run = (evidence, extra = {}) => spawnSync(process.execPath, [join(repo, 'scripts/run-source-checks.mjs'), '--repo', checkout, '--evidence-dir', evidence],
      { cwd: checkout, env: { ...process.env, BASH_ENV: envPath, ...extra }, encoding: 'utf8', timeout: 20_000 });
    const evidence = join(f.root, 'cli-pass');
    const passed = run(evidence);
    assert.equal(passed.status, 0, passed.stdout + passed.stderr);
    const receipt = JSON.parse(readFileSync(join(evidence, 'result.json')));
    assert.equal(receipt.candidate, candidate); assert.equal(receipt.identityUnchanged, true);
    assert.equal(run(evidence).status, 1, 'an original receipt cannot be overwritten');
    const failedEvidence = join(f.root, 'cli-changed');
    const changed = run(failedEvidence, { CHANGE_SOURCE: '1' });
    assert.equal(changed.status, 1, changed.stdout + changed.stderr);
    const failed = JSON.parse(readFileSync(join(failedEvidence, 'result.json')));
    assert.equal(failed.status, 'failed'); assert.equal(failed.identityUnchanged, false);
  } finally { f.cleanup(); }
});
