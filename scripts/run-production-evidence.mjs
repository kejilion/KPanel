#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import { isAbsolute, join, resolve, sep } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const phases = new Set(['preflight', 'backup', 'postdeploy']);
const versionPattern = /^\d+\.\d+\.\d+$/;

function usage(message) {
  if (message) process.stderr.write(`Production evidence orchestration failed: ${message}\n`);
  process.stderr.write(
    'usage: node scripts/run-production-evidence.mjs --phase preflight|backup|postdeploy ' +
      '--run-id ID --expected-version X.Y.Z --artifact-dir ABSOLUTE_PATH ' +
      '[--repo PATH] [--target arena-154] [--expected-revision SHA] ' +
      '[--expected-image-digest sha256:HEX] [--baseline-run-id ID] [--prepare-only]\n',
  );
  process.exit(2);
}

function parseArgs(argv) {
  const options = { repo: process.cwd(), target: 'arena-154', prepareOnly: false };
  const valueOptions = new Set([
    '--repo',
    '--target',
    '--phase',
    '--run-id',
    '--expected-version',
    '--expected-revision',
    '--expected-image-digest',
    '--baseline-run-id',
    '--artifact-dir',
  ]);
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === '--prepare-only') {
      options.prepareOnly = true;
      continue;
    }
    if (!valueOptions.has(argument)) usage(`unknown option ${argument}`);
    const value = argv[index + 1];
    if (!value || value.startsWith('--')) usage(`${argument} requires a value`);
    const key = argument.slice(2).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
    options[key] = value;
    index += 1;
  }
  for (const key of ['phase', 'runId', 'expectedVersion', 'artifactDir']) {
    if (!options[key]) usage(`--${key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)} is required`);
  }
  return options;
}

function run(command, args, { cwd, inherit = false } = {}) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    shell: false,
    stdio: inherit ? 'inherit' : 'pipe',
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const details = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`${command} ${args.join(' ')} exited ${result.status}${details ? `\n${details}` : ''}`);
  }
  return (result.stdout ?? '').trim();
}

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

function canonicalPath(path) {
  const value = realpathSync.native(resolve(path));
  return process.platform === 'win32' ? value.toLowerCase() : value;
}

function comparablePath(path) {
  const value = resolve(path);
  return process.platform === 'win32' ? value.toLowerCase() : value;
}

function validate(options) {
  if (!phases.has(options.phase)) throw new Error('phase must be preflight, backup, or postdeploy');
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(options.runId)) {
    throw new Error('run ID contains unsupported characters');
  }
  if (!versionPattern.test(options.expectedVersion)) throw new Error('expected version must be X.Y.Z');
  if (options.target !== 'arena-154') throw new Error('KPanel production evidence target must be arena-154');
  if (!isAbsolute(options.artifactDir)) throw new Error('artifact directory must be absolute');
  if (options.phase !== 'postdeploy') {
    const inapplicableFields = ['expectedRevision', 'expectedImageDigest'];
    if (options.phase === 'preflight') inapplicableFields.push('baselineRunId');
    for (const key of inapplicableFields) {
      // The remote plan uses '-' for fields that do not apply to this phase.
      if (options[key] !== undefined && options[key] !== '-') {
        const flag = key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
        throw new Error(`${options.phase} does not accept --${flag}`);
      }
    }
  }
  if (options.phase === 'postdeploy') {
    if (!/^[0-9a-f]{40,64}$/i.test(options.expectedRevision ?? '')) {
      throw new Error('postdeploy requires a full expected revision');
    }
    if (!/^sha256:[0-9a-f]{64}$/.test(options.expectedImageDigest ?? '')) {
      throw new Error('postdeploy requires an immutable expected image digest');
    }
    if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(options.baselineRunId ?? '')) {
      throw new Error('postdeploy requires a valid baseline run ID');
    }
  }
  if (options.phase === 'backup' && !/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(options.baselineRunId ?? '')) {
    throw new Error('backup requires a valid baseline run ID');
  }
}

function prepare(options) {
  validate(options);
  const repo = resolve(options.repo);
  const root = resolve(run('git', ['-C', repo, 'rev-parse', '--show-toplevel']));
  if (canonicalPath(root) !== canonicalPath(repo)) throw new Error('--repo must be the repository root');
  const artifactDir = resolve(options.artifactDir);
  const comparableRepo = comparablePath(repo);
  const comparableArtifact = comparablePath(artifactDir);
  if (comparableArtifact === comparableRepo || comparableArtifact.startsWith(comparableRepo + sep)) {
    throw new Error('artifact directory must stay outside the repository');
  }
  if (existsSync(artifactDir)) throw new Error('artifact directory already exists; use a new run ID and path');
  mkdirSync(artifactDir, { recursive: true, mode: 0o700 });

  const source = resolve(repo, 'scripts', 'run-production-evidence-remote.sh');
  if (!existsSync(source)) throw new Error('tracked remote production evidence entrypoint is missing');
  const remoteScriptPath = join(artifactDir, 'run-production-evidence-remote.sh');
  writeFileSync(remoteScriptPath, readFileSync(source));

  const planPath = join(artifactDir, 'plan.env');
  const plan = [
    'SCHEMA_VERSION=1',
    `PHASE=${options.phase}`,
    `RUN_ID=${options.runId}`,
    `EXPECTED_VERSION=${options.expectedVersion}`,
    `EXPECTED_REVISION=${options.expectedRevision ?? '-'}`,
    `EXPECTED_IMAGE_DIGEST=${options.expectedImageDigest ?? '-'}`,
    `BASELINE_RUN_ID=${options.baselineRunId ?? '-'}`,
    `REMOTE_SCRIPT_SHA256=${sha256(remoteScriptPath)}`,
    '',
  ].join('\n');
  writeFileSync(planPath, plan, { encoding: 'utf8', mode: 0o600 });

  const manifestPath = join(artifactDir, 'manifest.json');
  writeFileSync(
    manifestPath,
    JSON.stringify(
      {
        schemaVersion: 1,
        generatedAt: new Date().toISOString(),
        target: options.target,
        phase: options.phase,
        runId: options.runId,
        expectedVersion: options.expectedVersion,
        expectedRevision: options.expectedRevision ?? null,
        expectedImageDigest: options.expectedImageDigest ?? null,
        baselineRunId: options.baselineRunId ?? null,
        files: {
          plan: { name: 'plan.env', sha256: sha256(planPath) },
          remoteScript: { name: 'run-production-evidence-remote.sh', sha256: sha256(remoteScriptPath) },
        },
      },
      null,
      2,
    ) + '\n',
    { encoding: 'utf8', mode: 0o600 },
  );
  return { artifactDir, planPath, remoteScriptPath, manifestPath };
}

function uploadAndRun(options, prepared) {
  const purpose = options.phase === 'backup' ? 'production-deploy' : 'production-safety-check';
  run(process.execPath, [resolve(repoRoot, 'scripts', 'check-environment-policy.mjs'), '--environment', options.target, '--purpose', purpose], {
    cwd: repoRoot,
    inherit: true,
  });
  const inbox = `/root/kpanel-production-inbox/${options.runId}-${options.phase}`;
  run('ssh', [options.target, 'test', '!', '-e', inbox], { inherit: true });
  run('ssh', [options.target, 'install', '-d', '-m', '700', '--', inbox], { inherit: true });
  for (const path of [prepared.planPath, prepared.remoteScriptPath, prepared.manifestPath]) {
    run('scp', [path, `${options.target}:${inbox}/`], { inherit: true });
  }
  run('ssh', [options.target, 'bash', `${inbox}/run-production-evidence-remote.sh`, `${inbox}/plan.env`], {
    inherit: true,
  });
  const remoteEvidence = `/root/kpanel-release-evidence/${options.runId}/production-${options.phase}`;
  run('scp', ['-r', `${options.target}:${remoteEvidence}`, join(prepared.artifactDir, 'remote-evidence')], {
    inherit: true,
  });
}

try {
  const options = parseArgs(process.argv.slice(2));
  const prepared = prepare(options);
  process.stdout.write(
    `production_evidence_prepare=pass run_id=${options.runId} phase=${options.phase} manifest=${prepared.manifestPath}\n`,
  );
  if (!options.prepareOnly) {
    uploadAndRun(options, prepared);
    process.stdout.write(
      `production_evidence_remote=pass run_id=${options.runId} phase=${options.phase} target=${options.target}\n`,
    );
  }
} catch (error) {
  process.stderr.write(`Production evidence orchestration failed: ${error.message}\n`);
  process.exitCode = 1;
}
