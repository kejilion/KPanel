#!/usr/bin/env node

import { createHash } from 'node:crypto';
import {
  copyFileSync,
  existsSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { basename, dirname, isAbsolute, join, resolve, sep } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';

import { COVERAGE_BASELINE } from './check-release-acceptance-coverage.mjs';
import { checkEnvironment, loadPolicy } from './check-environment-policy.mjs';

const scriptRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const stableTagPattern = /^v(\d+)\.(\d+)\.(\d+)$/;
const canonicalOrigin = 'git@github.com:kejilion/KPanel.git';
const gitTimeout = 120_000;
const gitOutputLimit = 8 * 1024 * 1024;
const preparationTimeout = 600_000;
let gitDeadline = Infinity;
let gitExecutable;

function gitProgram() {
  if (process.platform !== 'win32') return 'git';
  if (gitExecutable) return gitExecutable;
  // Git for Windows' cmd/git.exe launcher can exit before its real child.
  // Own the actual Git process so taskkill /T still has a live tree root.
  const result = spawnSync('git', ['--exec-path'], {
    encoding: 'utf8', env: cleanGitEnvironment(), timeout: 10_000, maxBuffer: 16_384,
  });
  if (result.status !== 0) throw new Error('Git for Windows executable detection failed');
  const executable = resolve(dirname(dirname(result.stdout.trim())), 'bin', 'git.exe');
  if (!existsSync(executable)) throw new Error('Git for Windows executable was not found');
  gitExecutable = executable;
  return executable;
}

function usage(message) {
  if (message) process.stderr.write(`Release L3 orchestration failed: ${message}\n`);
  process.stderr.write(
    'usage: node scripts/run-release-l3.mjs --candidate SHA --base-tag vX.Y.Z ' +
      '--runner-image IMAGE --runner-id sha256:HEX --run-id ID --artifact-dir ABSOLUTE_PATH ' +
      '[--runner-archive ABSOLUTE_PATH --runner-archive-sha256 HEX] ' +
      '[--repo PATH] [--target ENVIRONMENT] [--prepare-only]\n' +
      '   or: node scripts/run-release-l3.mjs --execute-kit ABSOLUTE_PATH ' +
      '--kit-manifest-sha256 HEX --target ENVIRONMENT\n',
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
    '--runner-id',
    '--runner-archive',
    '--runner-archive-sha256',
    '--run-id',
    '--artifact-dir',
    '--target',
    '--execute-kit',
    '--kit-manifest-sha256',
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

  if (options.executeKit) {
    const incompatible = ['candidate', 'baseTag', 'runnerImage', 'runnerId', 'runId', 'artifactDir', 'runnerArchive', 'runnerArchiveSha256']
      .filter((key) => options[key]);
    if (options.prepareOnly || incompatible.length > 0) usage('--execute-kit only accepts --target');
    if (!isAbsolute(options.executeKit)) usage('--execute-kit must be an absolute path');
    if (!/^[0-9a-f]{64}$/.test(options.kitManifestSha256 ?? '')) {
      usage('--kit-manifest-sha256 must contain 64 lowercase hexadecimal characters');
    }
    if (!options.target) usage('--target is required with --execute-kit');
    return options;
  }
  if (options.kitManifestSha256) usage('--kit-manifest-sha256 requires --execute-kit');
  for (const key of ['candidate', 'baseTag', 'runnerImage', 'runnerId', 'runId', 'artifactDir']) {
    if (!options[key]) usage(`--${key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)} is required`);
  }
  if (Boolean(options.runnerArchive) !== Boolean(options.runnerArchiveSha256)) {
    usage('--runner-archive and --runner-archive-sha256 must be supplied together');
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

function run(command, args, { cwd, inherit = false, environment = process.env, timeout, input } = {}) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    env: environment,
    shell: false,
    stdio: inherit ? 'inherit' : ['pipe', 'pipe', 'pipe'],
    timeout,
    input,
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const details = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`${command} ${args.join(' ')} exited ${result.status}${details ? `\n${details}` : ''}`);
  }
  return (result.stdout ?? '').trim();
}

// Git can include SSH commands, key paths and proxy credentials in stderr.
// Never pass those diagnostics through the generic command/logging wrapper.
export async function gitResult(repo, args, environment = cleanGitEnvironment(), timeout = gitTimeout) {
  const remaining = gitDeadline - Date.now();
  if (remaining <= 0) throw new Error('source preparation time budget exceeded');
  return new Promise((resolveResult, reject) => {
    const child = spawn(gitProgram(), ['-C', repo, ...args], {
      env: environment, shell: false, stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true, detached: process.platform !== 'win32',
    });
    let failure;
    let size = 0;
    const stdout = [];
    const stderr = [];
    const stop = (code) => {
      if (failure) return;
      failure = code;
      if (!child.pid) return;
      // Stop the owned tree while its root still exists. Killing only Git first
      // leaves an SSH child holding Windows directories and Linux output pipes.
      if (process.platform === 'win32') {
        const killed = spawnSync('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], {
          windowsHide: true, stdio: 'ignore', timeout: 10_000,
        });
        if (killed.status !== 0) {
          failure = 'termination-failed';
          child.kill();
          child.stdout.destroy();
          child.stderr.destroy();
        }
      } else {
        try { process.kill(-child.pid, 'SIGKILL'); } catch (error) {
          if (error.code !== 'ESRCH') failure = 'termination-failed';
        }
      }
    };
    const interrupt = () => stop('interrupted');
    process.once('SIGINT', interrupt);
    process.once('SIGTERM', interrupt);
    const timer = setTimeout(() => stop('timeout'), Math.min(gitTimeout, remaining, timeout));
    for (const [stream, chunks] of [[child.stdout, stdout], [child.stderr, stderr]]) {
      stream.on('data', (chunk) => {
        size += chunk.length;
        if (size > gitOutputLimit) stop('output-limit');
        if (!failure) chunks.push(chunk);
      });
    }
    child.on('error', () => { failure = 'process-failed'; });
    child.on('close', (status, signal) => {
      clearTimeout(timer);
      process.removeListener('SIGINT', interrupt);
      process.removeListener('SIGTERM', interrupt);
      if (failure || signal) reject(new Error(`git ${args[0]} failed code=${failure || 'process-failed'}`));
      else resolveResult({ status, stdout: Buffer.concat(stdout).toString('utf8'), stderr: Buffer.concat(stderr).toString('utf8') });
    });
  });
}

async function runGit(repo, args, environment) {
  const result = await gitResult(repo, args, environment);
  if (result.status !== 0) throw new Error(`git ${args[0]} failed exit=${result.status}`);
  return result.stdout.trim();
}

async function gitSucceeds(repo, args) {
  const result = await gitResult(repo, args);
  return result.status === 0;
}

async function optionalConfig(repo, key) {
  const result = await gitResult(repo, ['config', '--get', key]);
  if (result.status === 1) return undefined;
  if (result.status !== 0) throw new Error('git config failed');
  return result.stdout.trim();
}

export async function transportEnvironment(repo, environment = process.env) {
  const result = cleanGitEnvironment(environment);
  // core.sshCommand overrides GIT_SSH, but GIT_SSH_COMMAND overrides both.
  if (result.GIT_SSH_COMMAND === undefined) {
    const command = await optionalConfig(repo, 'core.sshCommand');
    if (command !== undefined) result.GIT_SSH_COMMAND = command;
  }
  if (result.GIT_SSH_VARIANT === undefined) {
    const variant = await optionalConfig(repo, 'ssh.variant');
    if (variant !== undefined) result.GIT_SSH_VARIANT = variant;
  }
  return result;
}

function directoryIdentity(path) {
  const stat = lstatSync(path);
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error('unsafe temporary directory');
  return { dev: stat.dev, ino: stat.ino, real: canonicalPath(path) };
}

export function removeOwnedDirectory(path, identity) {
  const current = directoryIdentity(path);
  if (current.dev !== identity.dev || current.ino !== identity.ino || current.real !== identity.real) {
    throw new Error('temporary directory identity changed; preserved for recovery');
  }
  rmSync(path, { recursive: true, force: false });
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

async function parseBusinessBaseline(repo, candidate) {
  const content = await runGit(repo, ['show', `${candidate}:docs/product-quality-review-current.md`]);
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

async function remoteStableTags(repo, environment) {
  const output = await runGit(repo, ['ls-remote', '--tags', 'origin', 'refs/tags/v*'], environment);
  const entries = new Map();
  for (const line of output.split(/\r?\n/).filter(Boolean)) {
    const [object, rawRef] = line.split(/\s+/);
    if (!/^[0-9a-f]{40,64}$/.test(object) || !rawRef) throw new Error('invalid remote tag response');
    const peeled = rawRef.endsWith('^{}');
    const ref = peeled ? rawRef.slice(0, -3) : rawRef;
    const tag = ref.replace('refs/tags/', '');
    if (!stableTagPattern.test(tag)) continue;
    const current = entries.get(tag) ?? {};
    entries.set(tag, peeled ? { ...current, commit: object } : { ...current, object });
  }
  if (entries.size > 10_000) throw new Error('remote stable tag limit exceeded');
  return [...entries.entries()].map(([tag, value]) => {
    if (!value.object) throw new Error('remote tag object missing');
    return { tag, object: value.object, commit: value.commit ?? value.object };
  });
}

async function exactRemoteMain(repo, candidate, environment) {
  const output = await runGit(repo, ['ls-remote', '--heads', 'origin', 'refs/heads/main'], environment);
  const match = output.match(/^([0-9a-f]{40,64})\s+refs\/heads\/main$/i);
  if (!match) throw new Error('origin/main is missing or ambiguous');
  const remoteMain = match[1].toLowerCase();
  const trackedMain = (await runGit(repo, ['rev-parse', '--verify', 'refs/remotes/origin/main'])).toLowerCase();
  if (trackedMain !== remoteMain) {
    throw new Error('local origin/main is stale; fetch that exact ref with --no-tags before retrying');
  }
  if (!await gitSucceeds(repo, ['merge-base', '--is-ancestor', remoteMain, candidate])) {
    throw new Error('current origin/main is not an ancestor of the candidate');
  }
  return remoteMain;
}

function requiredStableTags(ancestors, remoteTags, baseTag, businessTag) {
  const required = new Map();
  for (const entry of remoteTags) {
    if (!ancestors.has(entry.commit)) continue;
    if (compareTags(entry.tag, COVERAGE_BASELINE) < 0) continue;
    required.set(entry.tag, entry);
  }
  for (const tag of [baseTag, businessTag]) {
    const entry = remoteTags.find((item) => item.tag === tag);
    if (!entry) throw new Error(`required tag ${tag} is missing from origin`);
    required.set(tag, entry);
  }
  return [...required.values()].sort((left, right) => compareTags(left.tag, right.tag));
}

function validateInputs(options) {
  if (!/^[0-9a-f]{40,64}$/i.test(options.candidate)) throw new Error('candidate must be a full Git object ID');
  if (!stableTagPattern.test(options.baseTag)) throw new Error('base tag must be a stable vX.Y.Z tag');
  if (!/^[A-Za-z0-9][A-Za-z0-9._/@:+-]{0,254}$/.test(options.runnerImage)) {
    throw new Error('runner image contains unsupported characters');
  }
  if (!/^sha256:[0-9a-f]{64}$/.test(options.runnerId)) {
    throw new Error('runner ID must be an immutable sha256 image ID');
  }
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(options.runId)) {
    throw new Error('run ID must contain only letters, numbers, dot, underscore, or hyphen');
  }
  if (options.target && !/^[A-Za-z0-9][A-Za-z0-9._@-]{0,127}$/.test(options.target)) {
    throw new Error('target contains unsupported characters');
  }
  if (!isAbsolute(options.artifactDir)) throw new Error('artifact directory must be absolute');
  if (options.runnerArchive) {
    if (!isAbsolute(options.runnerArchive)) throw new Error('runner archive path must be absolute');
    if (!/^[0-9a-f]{64}$/.test(options.runnerArchiveSha256)) {
      throw new Error('runner archive SHA-256 must contain 64 lowercase hexadecimal characters');
    }
    const archiveStat = lstatSync(options.runnerArchive);
    if (!archiveStat.isFile() || archiveStat.isSymbolicLink()) throw new Error('runner archive must be a regular file');
    if (sha256(options.runnerArchive) !== options.runnerArchiveSha256) throw new Error('runner archive checksum mismatch');
  }
}

async function checkSource(repo, candidate) {
  const root = resolve(await runGit(repo, ['rev-parse', '--show-toplevel']));
  if (canonicalPath(root) !== canonicalPath(repo)) {
    throw new Error('--repo must be the candidate repository root');
  }
  if (await runGit(repo, ['rev-parse', 'HEAD']) !== candidate) {
    throw new Error('candidate does not match HEAD');
  }
  if (await runGit(repo, ['status', '--short', '--untracked-files=all']) !== '') {
    throw new Error('candidate worktree must be clean');
  }
  if (await optionalConfig(repo, 'remote.origin.url') !== canonicalOrigin ||
      await runGit(repo, ['remote', 'get-url', 'origin']) !== canonicalOrigin) {
    throw new Error('origin must be the canonical KPanel SSH remote');
  }
}

async function prepare(options) {
  validateInputs(options);
  options.candidate = options.candidate.toLowerCase();
  const source = resolve(options.repo);
  const artifactDir = resolve(options.artifactDir);
  const comparableRepo = canonicalPath(source);
  // Resolve existing parents too: an artifact symlink must not point into source.
  let parent = artifactDir;
  while (!existsSync(parent)) parent = dirname(parent);
  const comparableArtifact = comparablePath(resolve(canonicalPath(parent), artifactDir.slice(parent.length).replace(/^[/\\]+/, '')));
  if (comparableArtifact === comparableRepo || comparableArtifact.startsWith(comparableRepo + sep)) {
    throw new Error('artifact directory must stay outside the candidate repository');
  }
  if (existsSync(artifactDir)) throw new Error('artifact directory already exists; retries require a new run ID and path');
  mkdirSync(artifactDir, { recursive: true, mode: 0o700 });
  const statePath = join(artifactDir, 'source-prepare.json');
  const started = Date.now();
  gitDeadline = started + preparationTimeout;
  const state = { schemaVersion: 1, runId: options.runId, candidate: options.candidate, status: 'running', phase: 'source-check', cleanup: 'not-created' };
  const record = () => writeFileSync(statePath, JSON.stringify(state, null, 2) + '\n', { mode: 0o600 });
  const phase = (name) => {
    state.phase = name;
    record();
    if (Date.now() - started >= preparationTimeout) throw new Error('source preparation time budget exceeded');
  };
  record();
  let ownedRoot;
  let identity;
  let prepared;
  let failure;
  try {
    await checkSource(source, options.candidate);
    const promisor = await gitResult(source, ['config', '--bool', '--get-regexp', '^remote\\..*\\.promisor$']);
    if (![0, 1].includes(promisor.status)) throw new Error('git config failed');
    if (await runGit(source, ['rev-parse', '--is-shallow-repository']) !== 'false' ||
        await optionalConfig(source, 'extensions.partialClone') !== undefined ||
        promisor.stdout.split(/\r?\n/).some((line) => line.endsWith(' true'))) {
      throw new Error('source must have complete non-shallow, non-partial candidate history');
    }
    // This proves the closure, rather than treating an absent remote tag object as
    // an error: a complete candidate cannot have that commit as an ancestor.
    await runGit(source, ['rev-list', '--objects', '--missing=error', options.candidate]);
    const ancestors = new Set((await runGit(source, ['rev-list', options.candidate])).split('\n'));
    const environment = await transportEnvironment(source);
    phase('remote-snapshot');
    const baseMainCommit = await exactRemoteMain(source, options.candidate, environment);
    const businessBaseline = await parseBusinessBaseline(source, options.candidate);
    const tags = requiredStableTags(ancestors, await remoteStableTags(source, environment), options.baseTag, businessBaseline.tag);
    state.sourceTagDifferences = [];
    for (const { tag, object } of tags) {
      const result = await gitResult(source, ['rev-parse', '--verify', `refs/tags/${tag}`]);
      if (result.status !== 0 || result.stdout.trim() !== object) state.sourceTagDifferences.push(tag);
    }
    phase('isolated-source');
    ownedRoot = mkdtempSync(join(tmpdir(), 'kpanel-release-source-'));
    identity = directoryIdentity(ownedRoot);
    state.cleanup = 'pending';
    state.temporaryDirectory = ownedRoot;
    record();
    const seed = join(ownedRoot, 'candidate.bundle');
    const repo = join(ownedRoot, 'repo');
    // HEAD only: no source tags, config, hooks or shared object alternates.
    await runGit(source, ['bundle', 'create', seed, 'HEAD']);
    await runGit(ownedRoot, ['clone', '--no-checkout', '--no-tags', seed, repo]);
    await runGit(repo, ['remote', 'set-url', 'origin', canonicalOrigin]);
    await runGit(repo, ['checkout', '--detach', options.candidate]);
    await checkSource(repo, options.candidate);
    await runGit(repo, ['fetch', '--no-tags', 'origin', 'refs/heads/main:refs/remotes/origin/main',
      ...tags.map(({ tag }) => `refs/tags/${tag}:refs/tags/${tag}`)], environment);
    if (await runGit(repo, ['rev-parse', 'refs/remotes/origin/main']) !== baseMainCommit) {
      throw new Error('origin/main moved during source preparation');
    }
    for (const { tag, object, commit } of tags) {
      if (await runGit(repo, ['rev-parse', `refs/tags/${tag}`]) !== object ||
          await runGit(repo, ['rev-parse', `refs/tags/${tag}^{commit}`]) !== commit) {
        throw new Error(`required tag ${tag} changed during source preparation`);
      }
    }
    if (!await gitSucceeds(repo, ['merge-base', '--is-ancestor', options.baseTag, options.candidate])) {
      throw new Error('base tag is not an ancestor of the candidate');
    }
    if (!await gitSucceeds(repo, ['merge-base', '--is-ancestor', businessBaseline.commit, options.candidate])) {
      throw new Error('business context baseline is not an ancestor of the candidate');
    }
    if (!await gitSucceeds(repo, ['merge-base', '--is-ancestor', businessBaseline.tag, businessBaseline.commit])) {
      throw new Error('business context baseline tag is not reachable from its recorded commit');
    }
    phase('bundle-verification');
    prepared = await buildKit(options, { repo, artifactDir, baseMainCommit, businessBaseline, requiredTags: tags.map(({ tag }) => tag), ownedRoot });
    phase('final-source-check');
    await checkSource(source, options.candidate);
    if (await exactRemoteMain(source, options.candidate, environment) !== baseMainCommit ||
        JSON.stringify(requiredStableTags(ancestors, await remoteStableTags(source, environment), options.baseTag, businessBaseline.tag)) !== JSON.stringify(tags)) {
      throw new Error('origin main or required tags changed during source preparation');
    }
    phase('cleanup');
  } catch (error) {
    failure = error;
  } finally {
    if (ownedRoot && identity) {
      try {
        removeOwnedDirectory(ownedRoot, identity);
        state.cleanup = 'removed';
      } catch {
        state.cleanup = 'preserved';
        failure = new Error('temporary source cleanup failed; preserved for recovery');
      }
    }
    state.status = failure ? 'failed' : 'pass';
    if (failure) state.error = failure.message;
    record();
    gitDeadline = Infinity;
  }
  if (failure) throw failure;
  return prepared;
}

async function buildKit(options, { repo, artifactDir, baseMainCommit, businessBaseline, requiredTags, ownedRoot }) {
  const bundleName = `kpanel-${options.runId}.bundle`;
  const bundlePath = join(artifactDir, bundleName);
  const tagRefs = requiredTags.map((tag) => `refs/tags/${tag}`);
  await runGit(repo, ['bundle', 'create', bundlePath, 'HEAD', ...tagRefs]);
  await runGit(repo, ['bundle', 'verify', bundlePath]);

  const verifyRepo = join(ownedRoot, 'offline');
  await runGit(ownedRoot, ['clone', '--no-checkout', bundlePath, verifyRepo]);
  await runGit(verifyRepo, ['checkout', '--detach', options.candidate]);
  if (await runGit(verifyRepo, ['rev-parse', 'HEAD']) !== options.candidate) {
    throw new Error('offline bundle clone did not reproduce the candidate');
  }
  for (const tag of requiredTags) await runGit(verifyRepo, ['show-ref', '--verify', `refs/tags/${tag}`]);
  run(process.execPath, [resolve(verifyRepo, 'scripts', 'check-business-context-freshness.mjs')], {
    cwd: verifyRepo,
    environment: cleanGitEnvironment(),
    timeout: Math.max(1, Math.min(gitTimeout, gitDeadline - Date.now())),
  });

  const remoteScriptSource = resolve(repo, 'scripts', 'run-release-l3-remote.sh');
  if (!existsSync(remoteScriptSource)) throw new Error('tracked remote L3 entrypoint is missing');
  const remoteScriptPath = join(artifactDir, 'run-release-l3-remote.sh');
  writeFileSync(remoteScriptPath, readFileSync(remoteScriptSource));

  let runnerArchivePath;
  let runnerArchiveName = '';
  if (options.runnerArchive) {
    runnerArchiveName = `kpanel-runner-${options.runId}.tar`;
    runnerArchivePath = join(artifactDir, runnerArchiveName);
    copyFileSync(options.runnerArchive, runnerArchivePath);
    if (sha256(runnerArchivePath) !== options.runnerArchiveSha256) {
      throw new Error('copied runner archive checksum mismatch');
    }
  }

  const planPath = join(artifactDir, 'plan.env');
  const plan = [
    'SCHEMA_VERSION=2',
    `RUN_ID=${options.runId}`,
    `EXPECTED_COMMIT=${options.candidate.toLowerCase()}`,
    `BASE_MAIN_COMMIT=${baseMainCommit}`,
    `EXPECTED_BASE_TAG=${options.baseTag}`,
    `BUSINESS_BASELINE_COMMIT=${businessBaseline.commit.toLowerCase()}`,
    `BUSINESS_BASELINE_TAG=${businessBaseline.tag}`,
    `RUNNER_IMAGE=${options.runnerImage}`,
    `EXPECTED_RUNNER_ID=${options.runnerId}`,
    `RUNNER_ARCHIVE_FILE=${runnerArchiveName}`,
    `RUNNER_ARCHIVE_SHA256=${options.runnerArchiveSha256 ?? ''}`,
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
        schemaVersion: 2,
        generatedAt: new Date().toISOString(),
        runId: options.runId,
        candidate: options.candidate.toLowerCase(),
        baseMainCommit,
        baseTag: options.baseTag,
        businessBaseline,
        requiredTags,
        runnerImage: options.runnerImage,
        expectedRunnerId: options.runnerId,
        runnerArchive: runnerArchivePath ? {
          name: runnerArchiveName,
          sha256: options.runnerArchiveSha256,
        } : null,
        origin: canonicalOrigin,
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

  return {
    artifactDir,
    bundlePath,
    planPath,
    remoteScriptPath,
    manifestPath,
    runnerArchivePath,
    requiredTags,
  };
}

export function loadPreparedKit(kitDirectory, expectedManifestSha256) {
  const artifactDir = resolve(kitDirectory);
  const stat = lstatSync(artifactDir);
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error('handoff kit must be a regular directory');
  const manifestPath = join(artifactDir, 'manifest.json');
  if (!/^[0-9a-f]{64}$/.test(expectedManifestSha256 ?? '') || sha256(manifestPath) !== expectedManifestSha256) {
    throw new Error('handoff manifest checksum mismatch');
  }
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
  if (manifest?.schemaVersion !== 2 || manifest.origin !== canonicalOrigin) {
    throw new Error('handoff manifest schema or origin is invalid');
  }
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(manifest.runId ?? '') ||
      !/^[0-9a-f]{40,64}$/.test(manifest.candidate ?? '') ||
      !/^sha256:[0-9a-f]{64}$/.test(manifest.expectedRunnerId ?? '')) {
    throw new Error('handoff manifest identity is invalid');
  }
  const file = (entry, expectedName) => {
    if (entry?.name !== expectedName || !/^[0-9a-f]{64}$/.test(entry?.sha256 ?? '')) {
      throw new Error(`handoff manifest file entry is invalid: ${expectedName}`);
    }
    const path = join(artifactDir, expectedName);
    const fileStat = lstatSync(path);
    if (!fileStat.isFile() || fileStat.isSymbolicLink() || sha256(path) !== entry.sha256) {
      throw new Error(`handoff kit checksum mismatch: ${expectedName}`);
    }
    return path;
  };
  const bundleName = manifest.files?.bundle?.name;
  if (!/^kpanel-[A-Za-z0-9._-]+\.bundle$/.test(bundleName ?? '')) {
    throw new Error('handoff bundle name is invalid');
  }
  const bundlePath = file(manifest.files.bundle, bundleName);
  const planPath = file(manifest.files?.plan, 'plan.env');
  const remoteScriptPath = file(manifest.files?.remoteScript, 'run-release-l3-remote.sh');
  let runnerArchivePath;
  if (manifest.runnerArchive !== null) {
    if (!/^kpanel-runner-[A-Za-z0-9._-]+\.tar$/.test(manifest.runnerArchive?.name ?? '')) {
      throw new Error('handoff runner archive name is invalid');
    }
    runnerArchivePath = file(manifest.runnerArchive, manifest.runnerArchive.name);
  }

  const planEntries = new Map();
  for (const line of readFileSync(planPath, 'utf8').split(/\r?\n/).filter(Boolean)) {
    const separator = line.indexOf('=');
    if (separator <= 0) throw new Error('handoff plan line is invalid');
    const key = line.slice(0, separator);
    if (planEntries.has(key)) throw new Error(`duplicate handoff plan key: ${key}`);
    planEntries.set(key, line.slice(separator + 1));
  }
  const allowedKeys = new Set([
    'SCHEMA_VERSION', 'RUN_ID', 'EXPECTED_COMMIT', 'BASE_MAIN_COMMIT', 'EXPECTED_BASE_TAG',
    'BUSINESS_BASELINE_COMMIT', 'BUSINESS_BASELINE_TAG', 'RUNNER_IMAGE', 'EXPECTED_RUNNER_ID',
    'RUNNER_ARCHIVE_FILE', 'RUNNER_ARCHIVE_SHA256', 'BUNDLE_FILE', 'BUNDLE_SHA256',
    'REMOTE_SCRIPT_SHA256', 'REQUIRED_TAGS',
  ]);
  for (const key of planEntries.keys()) if (!allowedKeys.has(key)) throw new Error(`unknown handoff plan key: ${key}`);
  if (planEntries.size !== allowedKeys.size || planEntries.get('SCHEMA_VERSION') !== '2' ||
      planEntries.get('RUN_ID') !== manifest.runId ||
      planEntries.get('EXPECTED_COMMIT') !== manifest.candidate ||
      planEntries.get('BASE_MAIN_COMMIT') !== manifest.baseMainCommit ||
      planEntries.get('EXPECTED_BASE_TAG') !== manifest.baseTag ||
      planEntries.get('BUSINESS_BASELINE_COMMIT') !== manifest.businessBaseline?.commit ||
      planEntries.get('BUSINESS_BASELINE_TAG') !== manifest.businessBaseline?.tag ||
      planEntries.get('RUNNER_IMAGE') !== manifest.runnerImage ||
      planEntries.get('EXPECTED_RUNNER_ID') !== manifest.expectedRunnerId ||
      planEntries.get('REQUIRED_TAGS') !== manifest.requiredTags?.join(',') ||
      planEntries.get('BUNDLE_FILE') !== bundleName ||
      planEntries.get('BUNDLE_SHA256') !== manifest.files.bundle.sha256 ||
      planEntries.get('REMOTE_SCRIPT_SHA256') !== manifest.files.remoteScript.sha256 ||
      planEntries.get('RUNNER_ARCHIVE_FILE') !== (manifest.runnerArchive?.name ?? '') ||
      planEntries.get('RUNNER_ARCHIVE_SHA256') !== (manifest.runnerArchive?.sha256 ?? '')) {
    throw new Error('handoff plan does not match its manifest');
  }
  return {
    artifactDir,
    bundlePath,
    planPath,
    remoteScriptPath,
    manifestPath,
    runnerArchivePath,
    requiredTags: manifest.requiredTags,
    runId: manifest.runId,
    candidate: manifest.candidate,
  };
}

function uploadAndRun(options, prepared) {
  const environment = checkEnvironment(loadPolicy(), options.target, 'candidate-validation');
  const uploadPaths = [prepared.bundlePath, prepared.planPath, prepared.remoteScriptPath, prepared.manifestPath];
  if (prepared.runnerArchivePath) uploadPaths.push(prepared.runnerArchivePath);

  const inbox = `/root/kpanel-release-inbox/${options.runId}`;
  if (environment.transport.kind === 'ssh') {
    const target = environment.transport.target;
    run('ssh', [target, 'test', '!', '-e', inbox], { inherit: true });
    run('ssh', [target, 'install', '-d', '-m', '700', '--', inbox], { inherit: true });
    for (const path of uploadPaths) run('scp', [path, `${target}:${inbox}/`], { inherit: true });
    run('ssh', [target, 'bash', `${inbox}/run-release-l3-remote.sh`, `${inbox}/plan.env`], { inherit: true });
    return;
  }
  if (environment.transport.kind !== 'wsl') throw new Error(`unsupported transport: ${environment.transport.kind}`);
  if (process.platform !== 'win32') throw new Error('local-wsl-dr transport requires a Windows control host');

  const prefix = ['-d', environment.transport.distribution, '-u', environment.transport.user, '--'];
  const wsl = (args, runOptions = {}) => run('wsl.exe', [...prefix, ...args], runOptions);
  const windowsPathToWsl = (path) => wsl(['wslpath', '-a', path.replaceAll('\\', '/')]);
  wsl(['test', '!', '-e', inbox]);
  wsl(['install', '-d', '-m', '700', '--', inbox]);
  for (const path of uploadPaths) {
    const source = windowsPathToWsl(path);
    wsl(['cp', '--', source, `${inbox}/${basename(path)}`]);
  }

  let remoteFailure;
  let evidenceFailure;
  try {
    wsl(['bash', `${inbox}/run-release-l3-remote.sh`, `${inbox}/plan.env`], { inherit: true });
  } catch (error) {
    remoteFailure = error;
  }
  try {
    const remoteEvidence = `/root/kpanel-release-evidence/${options.runId}`;
    const localEvidence = join(prepared.artifactDir, 'wsl-evidence');
    mkdirSync(localEvidence, { recursive: false, mode: 0o700 });
    const destination = windowsPathToWsl(localEvidence);
    const evidencePaths = wsl(['find', remoteEvidence, '-maxdepth', '1', '-type', 'f', '-print'])
      .split(/\r?\n/).filter(Boolean);
    for (const sourcePath of evidencePaths) {
      const expectedPrefix = `${remoteEvidence}/`;
      if (!sourcePath.startsWith(expectedPrefix)) throw new Error('unsafe WSL evidence path');
      const name = sourcePath.slice(expectedPrefix.length);
      if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(name)) throw new Error('unsafe WSL evidence filename');
      if (!/\.(?:log|txt|sha256)$/.test(name)) continue;
      const destinationPath = `${destination}/${name}`;
      const sourceHash = wsl(['sha256sum', '--', sourcePath]).split(/\s+/)[0];
      wsl(['cp', '--', sourcePath, destinationPath]);
      const destinationHash = wsl(['sha256sum', '--', destinationPath]).split(/\s+/)[0];
      if (!/^[0-9a-f]{64}$/.test(sourceHash) || sourceHash !== destinationHash) {
        throw new Error(`WSL evidence checksum mismatch: ${name}`);
      }
    }
  } catch (error) {
    evidenceFailure = error;
  }
  if (remoteFailure && evidenceFailure) {
    throw new Error(`${remoteFailure.message}; WSL evidence sync failed: ${evidenceFailure.message}`);
  }
  if (remoteFailure) throw remoteFailure;
  if (evidenceFailure) throw evidenceFailure;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) try {
  const options = parseArgs(process.argv.slice(2));
  if (options.executeKit) {
    const prepared = loadPreparedKit(options.executeKit, options.kitManifestSha256);
    options.runId = prepared.runId;
    uploadAndRun(options, prepared);
    process.stdout.write(`release_l3_handoff=pass run_id=${prepared.runId} candidate=${prepared.candidate} target=${options.target}\n`);
    process.exit(0);
  }
  const prepared = await prepare(options);
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
