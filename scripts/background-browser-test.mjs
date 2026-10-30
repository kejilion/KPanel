#!/usr/bin/env node

import { createHash } from 'node:crypto';
import {
  closeSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from 'node:fs';
import { dirname, resolve } from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { checkEnvironment, loadPolicy } from './check-environment-policy.mjs';

const scriptPath = fileURLToPath(import.meta.url);
const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));

function parseOptions(argv) {
  const options = {};
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (!arg.startsWith('--')) throw new Error(`unexpected argument: ${arg}`);
    const key = arg.slice(2).replaceAll('-', '_');
    const value = argv[++index];
    if (value === undefined) throw new Error(`missing value for ${arg}`);
    options[key] = value;
  }
  return options;
}

export function atomicWrite(path, value, {
  platform = process.platform,
  replace = renameSync,
  pause = (ms) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms),
} = {}) {
  mkdirSync(dirname(path), { recursive: true });
  const temporary = `${path}.${process.pid}.tmp`;
  writeFileSync(temporary, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
  // Windows readers can briefly prevent replacement. Keep the old complete state until
  // rename succeeds; never unlink it or fall back to an in-place write. At most 200 ms.
  for (let attempt = 0; ; attempt += 1) {
    try {
      replace(temporary, path);
      return;
    } catch (error) {
      if (platform !== 'win32' || !['EPERM', 'EACCES', 'EBUSY'].includes(error.code) || attempt >= 4) throw error;
      pause(20 * (attempt + 1));
    }
  }
}

function readJson(path) {
  return JSON.parse(readFileSync(path, 'utf8'));
}

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

function validateSpec(spec) {
  if (!spec || typeof spec !== 'object') throw new Error('command spec must be an object');
  if (typeof spec.command !== 'string' || spec.command.length === 0) throw new Error('spec.command is required');
  if (!Array.isArray(spec.args) || spec.args.some((arg) => typeof arg !== 'string')) {
    throw new Error('spec.args must be an array of strings');
  }
  if (spec.cwd !== undefined && typeof spec.cwd !== 'string') throw new Error('spec.cwd must be a string');
  if (spec.env !== undefined && (typeof spec.env !== 'object' || Array.isArray(spec.env))) {
    throw new Error('spec.env must be an object');
  }
  for (const [key, value] of Object.entries(spec.env ?? {})) {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) || typeof value !== 'string') {
      throw new Error('spec.env must contain string environment variables with safe names');
    }
    if (/(?:TOKEN|SECRET|PASSWORD|COOKIE|PRIVATE_KEY|CREDENTIAL)/i.test(key)) {
      throw new Error(`spec.env must not persist sensitive variable ${key}; inherit it from the runner environment`);
    }
  }
}

function statePath(evidenceDir) {
  return resolve(evidenceDir, 'state.json');
}

function start(options) {
  for (const required of ['environment', 'candidate_ref', 'spec', 'evidence_dir', 'timeout_seconds']) {
    if (!options[required]) throw new Error(`--${required.replaceAll('_', '-')} is required`);
  }
  const timeoutSeconds = Number(options.timeout_seconds);
  if (!Number.isInteger(timeoutSeconds) || timeoutSeconds < 1 || timeoutSeconds > 86400) {
    throw new Error('--timeout-seconds must be an integer between 1 and 86400');
  }
  if (!/^(?:[0-9a-f]{40}|sha256:[0-9a-f]{64})$/i.test(options.candidate_ref)) {
    throw new Error('--candidate-ref must be a full Git commit or sha256 digest');
  }
  const policyPath = resolve(options.policy ?? resolve(repoRoot, 'environment-policy.json'));
  const environment = checkEnvironment(loadPolicy(policyPath), options.environment, 'browser-validation');
  const inputSpecPath = resolve(options.spec);
  const spec = readJson(inputSpecPath);
  validateSpec(spec);

  const evidenceDir = resolve(options.evidence_dir);
  const statusFile = statePath(evidenceDir);
  if (existsSync(statusFile)) throw new Error(`background job already exists: ${statusFile}`);
  mkdirSync(evidenceDir, { recursive: true });
  const specPath = resolve(evidenceDir, 'command-spec.json');
  copyFileSync(inputSpecPath, specPath);
  const configPath = resolve(evidenceDir, 'job-config.json');
  const config = {
    schemaVersion: 1,
    environment: environment.name,
    role: environment.role,
    timeoutSeconds,
    candidateRef: options.candidate_ref,
    specPath,
    specSha256: sha256(specPath),
    evidenceDir,
    createdAt: new Date().toISOString(),
  };
  atomicWrite(configPath, config);
  atomicWrite(statusFile, {
    schemaVersion: 1,
    status: 'queued',
    environment: environment.name,
    candidateRef: config.candidateRef,
    specSha256: config.specSha256,
    createdAt: config.createdAt,
  });

  const child = spawn(process.execPath, [scriptPath, 'worker', '--config', configPath], {
    detached: true,
    stdio: 'ignore',
    windowsHide: true,
  });
  child.unref();
  process.stdout.write(`${JSON.stringify({
    jobId: `${environment.name}-${child.pid}`,
    pid: child.pid,
    statusFile,
    evidenceDir,
  })}\n`);
}

async function worker(options) {
  if (!options.config) throw new Error('--config is required');
  let config;
  let statusFile;
  let logFd;
  try {
    config = readJson(resolve(options.config));
    statusFile = statePath(config.evidenceDir);
    const spec = readJson(config.specPath);
    validateSpec(spec);
    if (sha256(config.specPath) !== config.specSha256) throw new Error('command spec changed after launch');

    const logPath = resolve(config.evidenceDir, 'browser-test.log');
    logFd = openSync(logPath, 'a');
    const startedAt = new Date().toISOString();
    atomicWrite(statusFile, {
      schemaVersion: 1,
      status: 'running',
      environment: config.environment,
      candidateRef: config.candidateRef,
      workerPid: process.pid,
      startedAt,
      timeoutSeconds: config.timeoutSeconds,
      specSha256: config.specSha256,
      logPath,
    });

    const child = spawn(spec.command, spec.args, {
      cwd: resolve(spec.cwd ?? repoRoot),
      env: { ...process.env, ...(spec.env ?? {}) },
      detached: process.platform !== 'win32',
      shell: false,
      stdio: ['ignore', logFd, logFd],
      windowsHide: true,
    });
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      if (process.platform === 'win32') child.kill();
      else {
        try { process.kill(-child.pid, 'SIGTERM'); } catch { child.kill(); }
      }
    }, config.timeoutSeconds * 1000);
    const result = await new Promise((resolveResult) => {
      child.once('error', (error) => resolveResult({ exitCode: null, signal: null, error: error.message }));
      child.once('exit', (exitCode, signal) => resolveResult({ exitCode, signal, error: null }));
    });
    clearTimeout(timer);
    closeSync(logFd);
    logFd = undefined;
    const finishedAt = new Date().toISOString();
    const status = timedOut ? 'timed_out' : result.exitCode === 0 ? 'passed' : 'failed';
    atomicWrite(statusFile, {
      schemaVersion: 1,
      status,
      environment: config.environment,
      candidateRef: config.candidateRef,
      workerPid: process.pid,
      commandPid: child.pid,
      startedAt,
      finishedAt,
      timeoutSeconds: config.timeoutSeconds,
      exitCode: result.exitCode,
      signal: result.signal,
      error: result.error,
      specSha256: config.specSha256,
      logPath,
    });
  } catch (error) {
    if (logFd !== undefined) closeSync(logFd);
    if (statusFile && config) {
      atomicWrite(statusFile, {
        schemaVersion: 1,
        status: 'failed',
        environment: config.environment,
        candidateRef: config.candidateRef,
        workerPid: process.pid,
        finishedAt: new Date().toISOString(),
        exitCode: null,
        error: error.message,
        specSha256: config.specSha256,
      });
    }
    throw error;
  }
}

function status(options) {
  if (!options.evidence_dir) throw new Error('--evidence-dir is required');
  const value = readJson(statePath(resolve(options.evidence_dir)));
  process.stdout.write(`${JSON.stringify(value, null, 2)}\n`);
}

export async function main(argv = process.argv.slice(2)) {
  const command = argv[0];
  const options = parseOptions(argv.slice(1));
  if (command === 'start') return start(options);
  if (command === 'worker') return worker(options);
  if (command === 'status') return status(options);
  throw new Error('usage: background-browser-test.mjs <start|status> [options]');
}

if (process.argv[1] && resolve(process.argv[1]) === scriptPath) {
  main().catch((error) => {
    process.stderr.write(`Background browser test failed: ${error.message}\n`);
    process.exit(1);
  });
}
