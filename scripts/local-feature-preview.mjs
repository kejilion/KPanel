#!/usr/bin/env node

import { createHash, randomBytes } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import {
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { createServer } from 'node:net';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptPath = fileURLToPath(import.meta.url);
const defaultRepoRoot = resolve(dirname(scriptPath), '..');
const validScope = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const validModes = new Set(['mock', 'integration']);
const validGrades = new Set(['draft', 'acceptance']);

export function parseArgs(argv) {
  const [command, ...tokens] = argv;
  const options = {};
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (!token.startsWith('--')) throw new Error(`unexpected argument: ${token}`);
    const key = token.slice(2);
    const value = tokens[index + 1];
    if (!value || value.startsWith('--')) throw new Error(`missing value for --${key}`);
    if (Object.hasOwn(options, key)) throw new Error(`duplicate option: --${key}`);
    options[key] = value;
    index += 1;
  }
  return { command, options };
}

function git(repoRoot, args, { allowFailure = false, trim = true } = {}) {
  const result = spawnSync('git', args, { cwd: repoRoot, encoding: 'utf8' });
  if (!allowFailure && result.status !== 0) {
    throw new Error((result.stderr || result.stdout || `git ${args.join(' ')} failed`).trim());
  }
  const stdout = result.stdout || '';
  return trim ? stdout.trim() : stdout.replace(/\r?\n$/, '');
}

export function sourceIdentity(repoRoot) {
  const commit = git(repoRoot, ['rev-parse', 'HEAD']);
  const branch = git(repoRoot, ['branch', '--show-current']) || '(detached)';
  const statusRaw = git(repoRoot, ['status', '--porcelain=v1', '--untracked-files=all'], { trim: false });
  const status = statusRaw ? statusRaw.split(/\r?\n/) : [];
  const hash = createHash('sha256');
  hash.update(commit);
  hash.update('\0');
  hash.update(statusRaw);
  hash.update('\0');
  hash.update(git(repoRoot, ['diff', '--binary', 'HEAD'], { trim: false }));
  hash.update('\0');
  hash.update(git(repoRoot, ['diff', '--cached', '--binary', 'HEAD'], { trim: false }));
  const untracked = git(repoRoot, ['ls-files', '--others', '--exclude-standard', '-z'], { trim: false });
  for (const relativePath of untracked.split('\0').filter(Boolean).sort()) {
    const absolutePath = resolve(repoRoot, relativePath);
    hash.update('\0' + relativePath + '\0');
    const size = statSync(absolutePath).size;
    if (size <= 10 * 1024 * 1024) hash.update(readFileSync(absolutePath));
    else hash.update(`oversize:${size}`);
  }
  return {
    branch,
    commit,
    clean: status.length === 0,
    status,
    workingTreeFingerprint: hash.digest('hex'),
  };
}

export function validateLoopbackTarget(rawTarget) {
  let target;
  try {
    target = new URL(rawTarget);
  } catch {
    throw new Error(`invalid --api-target URL: ${rawTarget}`);
  }
  if (!['http:', 'https:'].includes(target.protocol)) {
    throw new Error('--api-target must use http or https');
  }
  if (target.username || target.password || target.search || target.hash) {
    throw new Error('--api-target must not contain credentials, query parameters, or fragments');
  }
  if (!['127.0.0.1', 'localhost', '[::1]'].includes(target.hostname)) {
    throw new Error('local preview only accepts loopback API targets; use an approved real-host workflow for remote systems');
  }
  return target.origin;
}

function parsePort(raw, label) {
  if (raw === undefined || raw === 'auto') return undefined;
  const port = Number.parseInt(raw, 10);
  if (!Number.isInteger(port) || String(port) !== raw || port < 1024 || port > 65535) {
    throw new Error(`${label} must be "auto" or an integer between 1024 and 65535`);
  }
  return port;
}

function canListen(port) {
  return new Promise((resolvePromise) => {
    const server = createServer();
    server.unref();
    server.once('error', () => resolvePromise(false));
    server.listen({ host: '127.0.0.1', port, exclusive: true }, () => {
      server.close(() => resolvePromise(true));
    });
  });
}

export async function choosePort(preferred, start, excluded = new Set()) {
  if (preferred !== undefined) {
    if (excluded.has(preferred) || !(await canListen(preferred))) {
      throw new Error(`port ${preferred} is already in use`);
    }
    return preferred;
  }
  for (let port = start; port < start + 200; port += 1) {
    if (!excluded.has(port) && await canListen(port)) return port;
  }
  throw new Error(`no free loopback port found in ${start}-${start + 199}`);
}

function isProcessAlive(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function processCommandLine(pid) {
  if (process.platform === 'win32') {
    const command = `(Get-CimInstance Win32_Process -Filter 'ProcessId=${pid}').CommandLine`;
    return spawnSync('powershell.exe', ['-NoProfile', '-Command', command], { encoding: 'utf8' }).stdout.trim();
  }
  const procPath = `/proc/${pid}/cmdline`;
  if (existsSync(procPath)) return readFileSync(procPath, 'utf8').replaceAll('\0', ' ');
  return spawnSync('ps', ['-p', String(pid), '-o', 'command='], { encoding: 'utf8' }).stdout.trim();
}

function stopOwnedProcess(processInfo, previewId) {
  if (!processInfo || !isProcessAlive(processInfo.pid)) return 'not-running';
  const commandLine = processCommandLine(processInfo.pid);
  if (!commandLine.includes(previewId)) {
    throw new Error(`refusing to stop PID ${processInfo.pid}: ownership marker is missing`);
  }
  if (process.platform === 'win32') {
    const result = spawnSync('taskkill.exe', ['/PID', String(processInfo.pid), '/T', '/F'], { encoding: 'utf8' });
    if (result.status !== 0 && isProcessAlive(processInfo.pid)) {
      throw new Error((result.stderr || result.stdout || `failed to stop PID ${processInfo.pid}`).trim());
    }
  } else {
    try {
      process.kill(-processInfo.pid, 'SIGTERM');
    } catch {
      process.kill(processInfo.pid, 'SIGTERM');
    }
  }
  return 'stopped';
}

async function waitForUrl(url, { requireOk = true, timeoutMs = 30_000 } = {}) {
  const deadline = Date.now() + timeoutMs;
  let lastError = 'not ready';
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1_500) });
      if (!requireOk || response.ok) return { ok: true, status: response.status };
      lastError = `HTTP ${response.status}`;
    } catch (error) {
      lastError = error.message;
    }
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 250));
  }
  return { ok: false, error: lastError };
}

function spawnLogged(command, args, { cwd, env, logPath }) {
  const logFd = openSync(logPath, 'a');
  try {
    const child = spawn(command, args, {
      cwd,
      env,
      detached: true,
      windowsHide: true,
      stdio: ['ignore', logFd, logFd],
    });
    child.unref();
    return child;
  } finally {
    closeSync(logFd);
  }
}

function writeManifest(path, manifest) {
  writeFileSync(path, JSON.stringify(manifest, null, 2) + '\n', 'utf8');
}

function readManifest(evidenceDir) {
  const manifestPath = join(evidenceDir, 'manifest.json');
  if (!existsSync(manifestPath)) throw new Error(`preview manifest not found: ${manifestPath}`);
  return { manifestPath, manifest: JSON.parse(readFileSync(manifestPath, 'utf8')) };
}

function resolveEvidenceDir(repoRoot, rawPath, scope) {
  if (rawPath) return isAbsolute(rawPath) ? resolve(rawPath) : resolve(repoRoot, rawPath);
  const timestamp = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
  return resolve(repoRoot, '.codex-tmp', 'previews', `${scope}-${timestamp}`);
}

export function validateStartOptions(options, identity) {
  const scope = options.scope;
  const mode = options.mode || 'mock';
  const grade = options.grade || 'draft';
  if (!scope || !validScope.test(scope)) throw new Error('--scope must be a lowercase hyphenated identifier');
  if (!validModes.has(mode)) throw new Error('--mode must be mock or integration');
  if (!validGrades.has(grade)) throw new Error('--grade must be draft or acceptance');
  if (grade === 'acceptance' && !identity.clean) {
    throw new Error('acceptance preview requires a clean checkpoint; use --grade draft for an uncommitted worktree');
  }
  if (mode === 'mock' && options['api-target']) throw new Error('--api-target is only valid in integration mode');
  if (mode === 'integration' && !options['api-target']) throw new Error('integration mode requires --api-target');
  if (options['change-origin'] && !['true', 'false'].includes(options['change-origin'])) {
    throw new Error('--change-origin must be true or false');
  }
  return { scope, mode, grade };
}

export function validateKnownOptions(command, options) {
  const allowed = command === 'start'
    ? new Set(['project-dir', 'scope', 'mode', 'grade', 'web-port', 'api-port', 'api-target', 'evidence-dir', 'change-origin'])
    : new Set(['evidence-dir']);
  for (const key of Object.keys(options)) {
    if (!allowed.has(key)) throw new Error(`unknown option for ${command}: --${key}`);
  }
}

async function startPreview(options) {
  const repoRoot = resolve(options['project-dir'] || defaultRepoRoot);
  const actualRoot = git(repoRoot, ['rev-parse', '--show-toplevel']);
  const identity = sourceIdentity(actualRoot);
  const { scope, mode, grade } = validateStartOptions(options, identity);
  const evidenceDir = resolveEvidenceDir(actualRoot, options['evidence-dir'], scope);
  const evidenceRelativePath = relative(actualRoot, evidenceDir).replaceAll('\\', '/');
  if (!evidenceRelativePath.startsWith('../') && evidenceRelativePath !== '.codex-tmp' && !evidenceRelativePath.startsWith('.codex-tmp/')) {
    throw new Error('evidence inside the repository must stay under .codex-tmp');
  }
  const manifestPath = join(evidenceDir, 'manifest.json');
  if (existsSync(manifestPath)) {
    const existing = readManifest(evidenceDir).manifest;
    const active = Object.values(existing.processes || {}).some((item) => isProcessAlive(item?.pid));
    if (active) throw new Error(`an active preview already owns ${evidenceDir}`);
    throw new Error(`evidence directory already exists; choose a new --evidence-dir: ${evidenceDir}`);
  }
  mkdirSync(evidenceDir, { recursive: true });

  const previewId = `${scope}-${Date.now()}-${randomBytes(3).toString('hex')}`;
  const requestedWebPort = parsePort(options['web-port'], '--web-port');
  const requestedApiPort = parsePort(options['api-port'], '--api-port');
  const webPort = await choosePort(requestedWebPort, 4173);
  let apiTarget;
  let apiPort;
  if (mode === 'mock') {
    apiPort = await choosePort(requestedApiPort, 8080, new Set([webPort]));
    apiTarget = `http://127.0.0.1:${apiPort}`;
  } else {
    if (requestedApiPort !== undefined) throw new Error('--api-port is only valid in mock mode');
    apiTarget = validateLoopbackTarget(options['api-target']);
  }

  const webDir = join(actualRoot, 'web');
  const vitePath = join(webDir, 'node_modules', 'vite', 'bin', 'vite.js');
  if (!existsSync(vitePath)) throw new Error(`Vite is not installed in this worktree; run npm ci in ${webDir}`);

  const manifest = {
    schemaVersion: 1,
    id: previewId,
    state: 'starting',
    scope,
    mode,
    grade,
    evidenceLevel: mode === 'mock' ? 'mock-ui' : 'local-integration',
    source: { repository: actualRoot, ...identity },
    urls: { preview: `http://127.0.0.1:${webPort}`, apiTarget },
    processes: {},
    evidenceDir,
    startedAt: new Date().toISOString(),
    limitations: mode === 'mock'
      ? ['模拟数据：仅证明界面、交互和错误反馈，不证明真实 Panel/Agent、宿主机或 Docker 行为。']
      : ['本地集成：只证明所连接的本地候选 API；不替代隔离真机、公开产物或生产部署安全核对。'],
  };
  writeManifest(manifestPath, manifest);

  try {
    if (mode === 'mock') {
      const mockLog = join(evidenceDir, 'mock-api.log');
      const mock = spawnLogged(process.execPath, [join(actualRoot, 'scripts', 'mock-app-market-api.mjs'), '--preview-id', previewId], {
        cwd: actualRoot,
        env: { ...process.env, KPANEL_MOCK_API_PORT: String(apiPort) },
        logPath: mockLog,
      });
      manifest.processes.mockApi = { pid: mock.pid, log: mockLog };
      writeManifest(manifestPath, manifest);
      const apiReady = await waitForUrl(`${apiTarget}/api/v1/agent/health`);
      if (!apiReady.ok) throw new Error(`mock API did not become ready: ${apiReady.error}`);
    } else {
      const apiReady = await waitForUrl(apiTarget, { requireOk: false, timeoutMs: 5_000 });
      if (!apiReady.ok) throw new Error(`local integration API is unreachable: ${apiReady.error}`);
    }

    const webLog = join(evidenceDir, 'web.log');
    const web = spawnLogged(process.execPath, [
      vitePath,
      '--host', '127.0.0.1',
      '--port', String(webPort),
      '--strictPort',
      '--mode', `kpanel-preview-${previewId}`,
    ], {
      cwd: webDir,
      env: {
        ...process.env,
        VITE_DEV_API_TARGET: apiTarget,
        VITE_DEV_API_CHANGE_ORIGIN: options['change-origin'] === 'true' ? 'true' : 'false',
      },
      logPath: webLog,
    });
    manifest.processes.web = { pid: web.pid, log: webLog };
    writeManifest(manifestPath, manifest);
    const webReady = await waitForUrl(manifest.urls.preview);
    if (!webReady.ok) throw new Error(`web preview did not become ready: ${webReady.error}`);

    manifest.state = 'ready';
    manifest.readyAt = new Date().toISOString();
    writeManifest(manifestPath, manifest);
    process.stdout.write(JSON.stringify({
      state: manifest.state,
      id: previewId,
      mode,
      grade,
      source: manifest.source,
      previewUrl: manifest.urls.preview,
      evidenceDir,
      manifestPath,
      stopCommand: `node scripts/local-feature-preview.mjs stop --evidence-dir "${evidenceDir}"`,
    }, null, 2) + '\n');
  } catch (error) {
    manifest.state = 'failed';
    manifest.failure = error.message;
    manifest.finishedAt = new Date().toISOString();
    for (const processInfo of Object.values(manifest.processes)) {
      try { stopOwnedProcess(processInfo, previewId); } catch {}
    }
    writeManifest(manifestPath, manifest);
    throw error;
  }
}

async function previewStatus(options) {
  if (!options['evidence-dir']) throw new Error('status requires --evidence-dir');
  const evidenceDir = resolve(options['evidence-dir']);
  const { manifestPath, manifest } = readManifest(evidenceDir);
  const processes = Object.fromEntries(Object.entries(manifest.processes || {}).map(([name, value]) => [
    name,
    { ...value, alive: isProcessAlive(value.pid) },
  ]));
  const preview = await waitForUrl(manifest.urls.preview, { timeoutMs: 1_500 });
  process.stdout.write(JSON.stringify({
    id: manifest.id,
    recordedState: manifest.state,
    liveState: processes.web?.alive && preview.ok ? 'ready' : 'not-ready',
    preview,
    processes,
    source: manifest.source,
    mode: manifest.mode,
    grade: manifest.grade,
    manifestPath,
  }, null, 2) + '\n');
}

function stopPreview(options) {
  if (!options['evidence-dir']) throw new Error('stop requires --evidence-dir');
  const evidenceDir = resolve(options['evidence-dir']);
  const { manifestPath, manifest } = readManifest(evidenceDir);
  const stopped = {};
  for (const [name, processInfo] of Object.entries(manifest.processes || {})) {
    stopped[name] = stopOwnedProcess(processInfo, manifest.id);
  }
  manifest.state = 'stopped';
  manifest.stoppedAt = new Date().toISOString();
  writeManifest(manifestPath, manifest);
  process.stdout.write(JSON.stringify({ id: manifest.id, state: manifest.state, stopped, manifestPath }, null, 2) + '\n');
}

function usage() {
  return `usage:
  local-feature-preview.mjs start --scope <slug> [--mode mock|integration] [--grade draft|acceptance]
    [--project-dir <path>] [--web-port auto|port] [--api-port auto|port]
    [--api-target http://127.0.0.1:port] [--evidence-dir <path>] [--change-origin true|false]
  local-feature-preview.mjs status --evidence-dir <path>
  local-feature-preview.mjs stop --evidence-dir <path>\n`;
}

async function main() {
  const { command, options } = parseArgs(process.argv.slice(2));
  if (!['start', 'status', 'stop'].includes(command)) throw new Error(usage());
  validateKnownOptions(command, options);
  if (command === 'start') return startPreview(options);
  if (command === 'status') return previewStatus(options);
  if (command === 'stop') return stopPreview(options);
}

if (process.argv[1] && resolve(process.argv[1]) === scriptPath) {
  main().catch((error) => {
    process.stderr.write(`Local feature preview failed: ${error.message}\n`);
    process.exitCode = 1;
  });
}
