import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { runPlan } from '../run-source-checks.mjs';
import { resolveBash } from '../run-repo-bash.mjs';

const repo = resolve(import.meta.dirname, '../..');
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'kpanel-source-plan-'));
  return { root, run: (lanes, options = {}) => runPlan({ lanes, cwd: root, evidenceDir: join(root, 'evidence'), ...options }),
    cleanup: () => rmSync(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }) };
}
const lane = (id, code) => ({ id, argv: [process.execPath, '-e', code] });

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

test('timeout kills the owned descendant process tree', { timeout: 10_000 }, async () => {
  const f = fixture();
  try {
    const heartbeat = join(f.root, 'heartbeat');
    const descendant = 'const fs=require("node:fs");setInterval(()=>fs.appendFileSync(process.argv[1],"x"),30)';
    const report = await f.run([lane('web', 'require("node:child_process").spawn(process.execPath,["-e",' + JSON.stringify(descendant) + ',' + JSON.stringify(heartbeat) + '],{stdio:"inherit"});setInterval(()=>{},1000)')], { timeoutMs: 500 });
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
    writeFileSync(envFile, 'npm() { printf "npm %s\\n" "$*" >>"$CHECK_COMMANDS"; [ "${FAIL_TYPECHECK:-}" != 1 ] || [ "$*" != "run typecheck" ]; }; go() { printf "go %s\\n" "$*" >>"$CHECK_COMMANDS"; }; export -f npm go\n');
    const log = join(f.root, 'commands');
    const envPath = envFile.replace(/^([A-Za-z]):/, (_, drive) => '/' + drive.toLowerCase()).replaceAll('\\', '/');
    const run = (id, extra = {}) => spawnSync(resolveBash(), ['scripts/verify-source-lane.sh', id], { cwd: f.root, encoding: 'utf8', env: { ...process.env, BASH_ENV: envPath, CHECK_COMMANDS: log, ...extra } });
    for (const id of ['web', 'go', 'deploy']) assert.equal(run(id).status, 0);
    assert.deepEqual(readFileSync(log, 'utf8').trim().split('\n'), ['npm ci', 'npm run typecheck', 'npm test', 'npm run build', 'go test ./...', 'go test -race ./internal/panel ./internal/auth ./internal/dockerx', 'go vet ./...', 'deploy-check']);
    writeFileSync(log, '');
    assert.notEqual(run('web', { FAIL_TYPECHECK: '1' }).status, 0);
    assert.equal(readFileSync(log, 'utf8').trim(), 'npm ci\nnpm run typecheck');
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
