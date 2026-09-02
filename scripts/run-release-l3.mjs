#!/usr/bin/env node

import { createHash } from 'node:crypto';
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { isAbsolute, join, resolve, sep } from 'node:path';
import { spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';

import { COVERAGE_BASELINE } from './check-release-acceptance-coverage.mjs';

const scriptRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const stableTagPattern = /^v(\d+)\.(\d+)\.(\d+)$/;

function usage(message) {
  if (message) process.stderr.write(`Release L3 orchestration failed: ${message}\n`);
  process.stderr.write(
    'usage: node scripts/run-release-l3.mjs --candidate SHA --base-tag vX.Y.Z ' +
      '--runner-image IMAGE --run-id ID --artifact-dir ABSOLUTE_PATH ' +
      '[--repo PATH] [--target ENVIRONMENT] [--prepare-only]\n',
  );
  process.exit(2);
}

function parseArgs(argv) {
  const options = { repo: process.cwd(), prepareOnly: false };
  const valueOptions = new Set([
    '--repo',
    '--candidate',
    '--base-tag',
    '--runner-image',
    '--run-id',
    '--artifact-dir',
    '--target',
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

  for (const key of ['candidate', 'baseTag', 'runnerImage', 'runId', 'artifactDir']) {
    if (!options[key]) usage(`--${key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)} is required`);
  }
  if (!options.prepareOnly && !options.target) usage('--target is required unless --prepare-only is used');
  return options;
}

function cleanGitEnvironment(environment = process.env) {
  const result = { ...environment };
  for (const key of [
    'GIT_DIR',
    'GIT_WORK_TREE',
    'GIT_INDEX_FILE',
    'GIT_COMMON_DIR',
    'GIT_OBJECT_DIRECTORY',
    'GIT_ALTERNATE_OBJECT_DIRECTORIES',
    'GIT_PREFIX',
  ]) {
    delete result[key];
  }
  return result;
}

function run(command, args, { cwd, inherit = false, environment = process.env } = {}) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    env: environment,
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

function runGit(repo, args) {
  return run('git', ['-C', repo, ...args], {
    environment: cleanGitEnvironment(),
  });
}

function gitSucceeds(repo, args) {
  const result = spawnSync('git', ['-C', repo, ...args], {
    encoding: 'utf8',
    env: cleanGitEnvironment(),
    shell: false,
    stdio: 'pipe',
  });
  if (result.error) throw result.error;
  return result.status === 0;
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

function parseBusinessBaseline(repo) {
  const path = resolve(repo, 'docs', 'product-quality-review-current.md');
  const content = readFileSync(path, 'utf8');
  const commit = content.match(/^- 基线提交：`([0-9a-f]{40,64})`\s*$/m)?.[1];
  const tag = content.match(/^- 基线版本：`(v\d+\.\d+\.\d+)`\s*$/m)?.[1];
  if (!commit || !tag) throw new Error('current business context is missing a valid baseline commit or tag');
  return { commit, tag };
}

function compareTags(left, right) {
  const leftParts = stableTagPattern.exec(left).slice(1).map(Number);
  const rightParts = stableTagPattern.exec(right).slice(1).map(Number);
  for (let index = 0; index < 3; index += 1) {
    if (leftParts[index] !== rightParts[index]) return leftParts[index] - rightParts[index];
  }
  return 0;
}

function remoteStableTags(repo) {
  const output = runGit(repo, ['ls-remote', '--tags', 'origin', 'refs/tags/v*']);
  const entries = new Map();
  for (const line of output.split(/\r?\n/).filter(Boolean)) {
    const [object, rawRef] = line.split(/\s+/);
    const peeled = rawRef.endsWith('^{}');
    const ref = peeled ? rawRef.slice(0, -3) : rawRef;
    const tag = ref.replace('refs/tags/', '');
    if (!stableTagPattern.test(tag)) continue;
    const current = entries.get(tag) ?? {};
    entries.set(tag, peeled ? { ...current, commit: object } : { ...current, object });
  }
  return [...entries.entries()].map(([tag, value]) => ({ tag, commit: value.commit ?? value.object }));
}

function exactRemoteMain(repo, candidate) {
  const output = runGit(repo, ['ls-remote', '--heads', 'origin', 'refs/heads/main']);
  const match = output.match(/^([0-9a-f]{40,64})\s+refs\/heads\/main$/i);
  if (!match) throw new Error('origin/main is missing or ambiguous');
  const remoteMain = match[1].toLowerCase();
  const trackedMain = runGit(repo, ['rev-parse', '--verify', 'refs/remotes/origin/main']).toLowerCase();
  if (trackedMain !== remoteMain) {
    throw new Error('local origin/main is stale; fetch that exact ref with --no-tags before retrying');
  }
  if (!gitSucceeds(repo, ['merge-base', '--is-ancestor', remoteMain, candidate])) {
    throw new Error('current origin/main is not an ancestor of the candidate');
  }
  return remoteMain;
}

function requiredStableTags(repo, candidate, baseTag) {
  const required = new Map();
  const remoteTags = remoteStableTags(repo);
  for (const entry of remoteTags) {
    if (!gitSucceeds(repo, ['cat-file', '-e', `${entry.commit}^{commit}`])) continue;
    if (!gitSucceeds(repo, ['merge-base', '--is-ancestor', entry.commit, candidate])) continue;
    if (compareTags(entry.tag, COVERAGE_BASELINE) < 0) continue;
    required.set(entry.tag, entry.commit);
  }

  const remoteBase = remoteTags.find((entry) => entry.tag === baseTag);
  if (!remoteBase) throw new Error(`base tag ${baseTag} is missing from origin`);
  required.set(baseTag, remoteBase.commit);

  const tags = [...required.entries()].sort(([left], [right]) => compareTags(left, right));
  for (const [tag, remoteCommit] of tags) {
    let localCommit;
    try {
      localCommit = runGit(repo, ['rev-parse', '--verify', `refs/tags/${tag}^{commit}`]);
    } catch {
      throw new Error(
        `required tag ${tag} is missing locally; fetch that exact tag with --no-tags before retrying`,
      );
    }
    if (localCommit !== remoteCommit) {
      throw new Error(`local tag ${tag} does not match origin; use a clean release clone instead of overwriting it`);
    }
  }
  return tags.map(([tag]) => tag);
}

function validateInputs(options) {
  if (!/^[0-9a-f]{40,64}$/i.test(options.candidate)) throw new Error('candidate must be a full Git object ID');
  if (!stableTagPattern.test(options.baseTag)) throw new Error('base tag must be a stable vX.Y.Z tag');
  if (!/^[A-Za-z0-9][A-Za-z0-9._/@:+-]{0,254}$/.test(options.runnerImage)) {
    throw new Error('runner image contains unsupported characters');
  }
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(options.runId)) {
    throw new Error('run ID must contain only letters, numbers, dot, underscore, or hyphen');
  }
  if (options.target && !/^[A-Za-z0-9][A-Za-z0-9._@-]{0,127}$/.test(options.target)) {
    throw new Error('target contains unsupported characters');
  }
  if (!isAbsolute(options.artifactDir)) throw new Error('artifact directory must be absolute');
}

function prepare(options) {
  validateInputs(options);
  const repo = resolve(options.repo);
  const root = resolve(runGit(repo, ['rev-parse', '--show-toplevel']));
  if (canonicalPath(root) !== canonicalPath(repo)) {
    throw new Error(`--repo must be the candidate repository root: ${root}`);
  }
  if (runGit(repo, ['rev-parse', 'HEAD']) !== options.candidate) {
    throw new Error('candidate does not match HEAD');
  }
  if (runGit(repo, ['status', '--short', '--untracked-files=all']) !== '') {
    throw new Error('candidate worktree must be clean');
  }
  const baseMainCommit = exactRemoteMain(repo, options.candidate);
  if (!gitSucceeds(repo, ['merge-base', '--is-ancestor', options.baseTag, options.candidate])) {
    throw new Error('base tag is not an ancestor of the candidate');
  }

  const businessBaseline = parseBusinessBaseline(repo);
  if (!gitSucceeds(repo, ['merge-base', '--is-ancestor', businessBaseline.commit, options.candidate])) {
    throw new Error('business context baseline is not an ancestor of the candidate');
  }
  const baselineTagCommit = runGit(repo, ['rev-parse', '--verify', `${businessBaseline.tag}^{commit}`]);
  if (!gitSucceeds(repo, ['merge-base', '--is-ancestor', baselineTagCommit, businessBaseline.commit])) {
    throw new Error('business context baseline tag is not reachable from its recorded commit');
  }

  const requiredTags = requiredStableTags(repo, options.candidate, options.baseTag);
  const artifactDir = resolve(options.artifactDir);
  const comparableRepo = comparablePath(repo);
  const comparableArtifact = comparablePath(artifactDir);
  if (comparableArtifact === comparableRepo || comparableArtifact.startsWith(comparableRepo + sep)) {
    throw new Error('artifact directory must stay outside the candidate repository');
  }
  if (existsSync(artifactDir)) throw new Error('artifact directory already exists; retries require a new run ID and path');
  mkdirSync(artifactDir, { recursive: true, mode: 0o700 });

  const bundleName = `kpanel-${options.runId}.bundle`;
  const bundlePath = join(artifactDir, bundleName);
  const tagRefs = requiredTags.map((tag) => `refs/tags/${tag}`);
  runGit(repo, ['bundle', 'create', bundlePath, 'HEAD', ...tagRefs]);
  runGit(repo, ['bundle', 'verify', bundlePath]);

  const verifyRoot = mkdtempSync(join(tmpdir(), 'kpanel-release-l3-'));
  try {
    const verifyRepo = join(verifyRoot, 'repo');
    run('git', ['clone', '--no-checkout', bundlePath, verifyRepo], {
      environment: cleanGitEnvironment(),
    });
    runGit(verifyRepo, ['checkout', '--detach', options.candidate]);
    if (runGit(verifyRepo, ['rev-parse', 'HEAD']) !== options.candidate) {
      throw new Error('offline bundle clone did not reproduce the candidate');
    }
    for (const tag of requiredTags) runGit(verifyRepo, ['show-ref', '--verify', `refs/tags/${tag}`]);
    run(process.execPath, [resolve(verifyRepo, 'scripts', 'check-business-context-freshness.mjs')], {
      cwd: verifyRepo,
      environment: cleanGitEnvironment(),
    });
  } finally {
    const safePrefix = resolve(tmpdir()) + (process.platform === 'win32' ? '\\' : '/');
    if (!resolve(verifyRoot).startsWith(safePrefix)) throw new Error('refusing to remove unexpected verification path');
    rmSync(verifyRoot, { recursive: true, force: true });
  }

  const remoteScriptSource = resolve(repo, 'scripts', 'run-release-l3-remote.sh');
  if (!existsSync(remoteScriptSource)) throw new Error('tracked remote L3 entrypoint is missing');
  const remoteScriptPath = join(artifactDir, 'run-release-l3-remote.sh');
  writeFileSync(remoteScriptPath, readFileSync(remoteScriptSource));

  const planPath = join(artifactDir, 'plan.env');
  const plan = [
    'SCHEMA_VERSION=1',
    `RUN_ID=${options.runId}`,
    `EXPECTED_COMMIT=${options.candidate.toLowerCase()}`,
    `BASE_MAIN_COMMIT=${baseMainCommit}`,
    `EXPECTED_BASE_TAG=${options.baseTag}`,
    `BUSINESS_BASELINE_COMMIT=${businessBaseline.commit.toLowerCase()}`,
    `BUSINESS_BASELINE_TAG=${businessBaseline.tag}`,
    `RUNNER_IMAGE=${options.runnerImage}`,
    `BUNDLE_FILE=${bundleName}`,
    `BUNDLE_SHA256=${sha256(bundlePath)}`,
    `REMOTE_SCRIPT_SHA256=${sha256(remoteScriptPath)}`,
    `REQUIRED_TAGS=${requiredTags.join(',')}`,
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
        runId: options.runId,
        candidate: options.candidate.toLowerCase(),
        baseMainCommit,
        baseTag: options.baseTag,
        businessBaseline,
        requiredTags,
        runnerImage: options.runnerImage,
        origin: runGit(repo, ['remote', 'get-url', 'origin']),
        files: {
          bundle: { name: bundleName, sha256: sha256(bundlePath) },
          plan: { name: 'plan.env', sha256: sha256(planPath) },
          remoteScript: { name: 'run-release-l3-remote.sh', sha256: sha256(remoteScriptPath) },
        },
      },
      null,
      2,
    ) + '\n',
    { encoding: 'utf8', mode: 0o600 },
  );

  return { artifactDir, bundlePath, planPath, remoteScriptPath, manifestPath, requiredTags };
}

function uploadAndRun(options, prepared) {
  run(process.execPath, [resolve(scriptRoot, 'scripts', 'check-environment-policy.mjs'), '--environment', options.target, '--purpose', 'candidate-validation'], {
    cwd: scriptRoot,
    inherit: true,
  });

  const inbox = `/root/kpanel-release-inbox/${options.runId}`;
  run('ssh', [options.target, 'test', '!', '-e', inbox], { inherit: true });
  run('ssh', [options.target, 'install', '-d', '-m', '700', '--', inbox], { inherit: true });
  for (const path of [prepared.bundlePath, prepared.planPath, prepared.remoteScriptPath, prepared.manifestPath]) {
    run('scp', [path, `${options.target}:${inbox}/`], { inherit: true });
  }
  run('ssh', [options.target, 'bash', `${inbox}/run-release-l3-remote.sh`, `${inbox}/plan.env`], {
    inherit: true,
  });
}

try {
  const options = parseArgs(process.argv.slice(2));
  const prepared = prepare(options);
  process.stdout.write(
    `release_l3_prepare=pass run_id=${options.runId} candidate=${options.candidate.toLowerCase()} ` +
      `tags=${prepared.requiredTags.length} manifest=${prepared.manifestPath}\n`,
  );
  if (!options.prepareOnly) {
    uploadAndRun(options, prepared);
    process.stdout.write(`release_l3_remote=pass run_id=${options.runId} target=${options.target}\n`);
  }
} catch (error) {
  process.stderr.write(`Release L3 orchestration failed: ${error.message}\n`);
  process.exitCode = 1;
}
