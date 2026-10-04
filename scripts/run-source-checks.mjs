#!/usr/bin/env node

// All source checks remain mandatory. Independent fixed lanes overlap; publication never happens here.
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, existsSync, mkdirSync, mkdtempSync, openSync, readFileSync, realpathSync, writeSync } from 'node:fs';
import { availableParallelism, tmpdir } from 'node:os';
import { isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { cleanShellEnvironment, resolveBash } from './run-repo-bash.mjs';

const MAX_LOG_BYTES = 32 * 1024 * 1024;
const MAX_TIMEOUT_MS = 45 * 60 * 1000;

function terminate(child) {
  if (!child.pid) return;
  if (process.platform === 'win32') {
    spawnSync('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
  } else {
    try { process.kill(-child.pid, 'SIGKILL'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
  }
}

export async function runPlan({ lanes, cwd, evidenceDir, jobs = 2, timeoutMs = MAX_TIMEOUT_MS, environment = process.env,
  identity = {}, verifyIdentity = () => true }) {
  if (![1, 2].includes(jobs)) throw new Error('jobs must be 1 or 2');
  if (!Number.isInteger(timeoutMs) || timeoutMs < 100 || timeoutMs > MAX_TIMEOUT_MS) throw new Error('invalid timeout');
  if (!Array.isArray(lanes) || lanes.length < 1 || lanes.length > 3) throw new Error('invalid lane count');
  const ids = new Set();
  for (const lane of lanes) {
    if (!/^[a-z][a-z0-9-]{0,30}$/.test(lane.id) || ids.has(lane.id)
        || !Array.isArray(lane.argv) || lane.argv.length < 1 || lane.argv.some((x) => typeof x !== 'string' || !x || x.includes('\0'))) {
      throw new Error('invalid or duplicate lane');
    }
    ids.add(lane.id);
  }
  mkdirSync(evidenceDir, { recursive: true });
  // Reserve the terminal receipt before launching anything; a reused directory cannot overwrite evidence.
  const receiptFd = openSync(join(evidenceDir, 'result.json'), 'wx', 0o600);
  const started = Date.now();
  const results = [];
  const active = new Set();
  let next = 0;
  let stopped = false;
  const cancel = () => { stopped = true; for (const entry of active) entry.cancel('cancelled'); };
  process.on('SIGINT', cancel);
  process.on('SIGTERM', cancel);

  async function laneRun(lane) {
    const start = Date.now();
    const log = join(evidenceDir, lane.id + '.log');
    let fd;
    try { fd = openSync(log, 'wx', 0o600); } catch (error) {
      return { id: lane.id, status: 'failed', exitCode: null, error: error.message, durationMs: Date.now() - start };
    }
    return new Promise((resolveResult) => {
      let reason = null;
      let bytes = 0;
      const digest = createHash('sha256');
      let complete = false;
      const child = spawn(lane.argv[0], lane.argv.slice(1), {
        cwd, env: environment, shell: false, detached: process.platform !== 'win32', windowsHide: true,
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      const entry = { cancel: (value) => { reason ??= value; terminate(child); } };
      active.add(entry);
      const timer = setTimeout(() => entry.cancel('timeout'), timeoutMs);
      const output = (chunk) => {
        if (reason || complete) return;
        bytes += chunk.length;
        if (bytes > MAX_LOG_BYTES) { entry.cancel('output-limit'); return; }
        try { writeSync(fd, chunk); digest.update(chunk); } catch { entry.cancel('evidence-write-failed'); }
      };
      child.stdout.on('data', output);
      child.stderr.on('data', output);
      const finish = (code, signal, error) => {
        if (complete) return;
        complete = true;
        clearTimeout(timer);
        active.delete(entry);
        closeSync(fd);
        resolveResult({ id: lane.id, status: reason ?? (code === 0 && !error && !signal ? 'passed' : 'failed'),
          exitCode: code, signal: signal ?? null, ...(error ? { error: error.message } : {}),
          startedAt: new Date(start).toISOString(), finishedAt: new Date().toISOString(), durationMs: Date.now() - start, log, sha256: digest.digest('hex') });
      };
      child.on('error', (error) => finish(null, null, error));
      child.on('close', (code, signal) => finish(code, signal));
    });
  }

  try {
    async function worker() {
      while (!stopped && next < lanes.length) {
        const lane = lanes[next++];
        const result = await laneRun(lane);
        results.push(result);
        if (result.status !== 'passed') cancel();
      }
    }
    await Promise.all(Array.from({ length: Math.min(jobs, lanes.length) }, () => worker()));
    for (const lane of lanes.slice(next)) results.push({ id: lane.id, status: 'not-run', exitCode: null });
    let identityUnchanged = false;
    try { identityUnchanged = Boolean(await verifyIdentity()); } catch { /* Refuse success when Git cannot prove identity. */ }
    for (const result of results.filter((entry) => entry.status === 'passed')) {
      try {
        if (createHash('sha256').update(readFileSync(result.log)).digest('hex') !== result.sha256) result.status = 'evidence-damaged';
      } catch { result.status = 'evidence-missing'; }
    }
    const report = { schemaVersion: 1, ...identity, identityUnchanged,
      status: !stopped && identityUnchanged && results.length === lanes.length && results.every((x) => x.status === 'passed') ? 'passed' : 'failed',
      jobs, startedAt: new Date(started).toISOString(), finishedAt: new Date().toISOString(), durationMs: Date.now() - started,
      lanes: lanes.map((lane) => results.find((x) => x.id === lane.id)) };
    writeSync(receiptFd, JSON.stringify(report, null, 2) + '\n');
    return report;
  } finally {
    process.removeListener('SIGINT', cancel);
    process.removeListener('SIGTERM', cancel);
    closeSync(receiptFd);
  }
}

export async function main(argv) {
  const options = { repo: process.cwd(), jobs: 2, timeoutMs: MAX_TIMEOUT_MS };
  const keys = new Map([['--repo', 'repo'], ['--jobs', 'jobs'], ['--timeout-ms', 'timeoutMs'], ['--evidence-dir', 'evidenceDir']]);
  const seen = new Set();
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--help') { process.stdout.write('usage: run-source-checks.mjs [--repo PATH] [--jobs 1|2] [--timeout-ms N] [--evidence-dir OUTSIDE_REPO]\n'); return 0; }
    const key = keys.get(argv[i]);
    if (!key || seen.has(key) || !argv[i + 1] || argv[i + 1].startsWith('--')) throw new Error('invalid option: ' + argv[i]);
    seen.add(key); options[key] = argv[++i];
  }
  if (![1, 2].includes(Number(options.jobs))) throw new Error('jobs must be 1 or 2');
  if (!Number.isInteger(Number(options.timeoutMs)) || Number(options.timeoutMs) < 100 || Number(options.timeoutMs) > MAX_TIMEOUT_MS) throw new Error('invalid timeout');
  const repo = realpathSync.native(resolve(options.repo));
  const environment = cleanShellEnvironment();
  for (const key of Object.keys(environment)) if (/^(GITHUB_TOKEN|GOVERNANCE_CI_TOKEN|DOCKERHUB_TOKEN)$/i.test(key)) delete environment[key];
  environment.GOMAXPROCS ??= String(Math.max(1, Math.floor(availableParallelism() / 2)));
  const git = (...args) => execFileSync('git', ['-C', repo, ...args], { env: environment, encoding: 'utf8', timeout: 20_000 }).trim();
  const root = realpathSync.native(git('rev-parse', '--show-toplevel'));
  if (process.platform === 'win32' ? root.toLowerCase() !== repo.toLowerCase() : root !== repo) throw new Error('repo must be the worktree root');
  const candidate = git('rev-parse', 'HEAD');
  const tree = git('rev-parse', 'HEAD^{tree}');
  const clean = () => !git('diff', '--name-only', 'HEAD', '--') && !git('ls-files', '--others', '--exclude-standard')
    .split(/\r?\n/).filter((path) => path && path !== 'release/DRAFT_NOTES.md').length;
  if (!clean()) throw new Error('source must be clean (only generated release/DRAFT_NOTES.md is exempt)');
  const laneScript = join(repo, 'scripts', 'verify-source-lane.sh');
  if (!existsSync(laneScript)) throw new Error('fixed source lane script is missing');
  const bash = resolveBash();
  const qualified = spawnSync(bash, [laneScript, 'preflight'], { cwd: repo, env: environment, encoding: 'utf8', timeout: 30_000 });
  if (qualified.status !== 0 || qualified.error) throw new Error('source qualification failed: ' + (qualified.stderr || qualified.error?.message));
  process.stdout.write(qualified.stdout);
  const evidenceDir = options.evidenceDir ? resolve(options.evidenceDir) : mkdtempSync(join(tmpdir(), 'kpanel-source-checks-'));
  mkdirSync(evidenceDir, { recursive: true });
  const evidenceRelative = relative(repo, realpathSync.native(evidenceDir));
  if (!evidenceRelative || (!evidenceRelative.startsWith('..' + (process.platform === 'win32' ? '\\' : '/')) && !isAbsolute(evidenceRelative))) {
    throw new Error('evidence directory must be outside the repository');
  }
  process.stdout.write('source_checks=start candidate=' + candidate + ' jobs=' + options.jobs + ' evidence=' + evidenceDir + '\n');
  const report = await runPlan({ lanes: ['web', 'go', 'deploy'].map((id) => ({ id, argv: [bash, laneScript, id] })),
    cwd: repo, evidenceDir, jobs: Number(options.jobs), timeoutMs: Number(options.timeoutMs), environment,
    identity: { candidate, tree, nodeVersion: process.version, platform: process.platform, tools: qualified.stdout.trim(), timeoutMs: Number(options.timeoutMs), gomaxprocs: environment.GOMAXPROCS },
    verifyIdentity: () => git('rev-parse', 'HEAD') === candidate && git('rev-parse', 'HEAD^{tree}') === tree && clean() });
  for (const lane of report.lanes) {
    process.stdout.write('source_lane=' + lane.id + ' status=' + lane.status + ' duration_ms=' + (lane.durationMs ?? 0) + '\n');
    // L3 transports stdout as the authoritative artifact; retain every lane's original bytes there too.
    if (lane.log) process.stdout.write(readFileSync(lane.log));
  }
  process.stdout.write('source_checks=' + (report.status === 'passed' ? 'pass' : 'fail') + ' candidate=' + candidate
    + ' duration_ms=' + report.durationMs + ' identity_unchanged=' + report.identityUnchanged + '\n');
  return report.status === 'passed' ? 0 : 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.exitCode = await main(process.argv.slice(2)); }
  catch (error) { process.stderr.write('source_checks=fail ' + error.message + '\n'); process.exitCode = 1; }
}
