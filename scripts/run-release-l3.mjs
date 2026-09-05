#!/usr/bin/env node

import { createHash } from 'node:crypto';
import {
  existsSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { dirname, isAbsolute, join, resolve, sep } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';

import { COVERAGE_BASELINE } from './check-release-acceptance-coverage.mjs';

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

function run(command, args, { cwd, inherit = false, environment = process.env, timeout } = {}) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    env: environment,
    shell: false,
    stdio: inherit ? 'inherit' : 'pipe',
    timeout,
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
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$/.test(options.runId)) {
    throw new Error('run ID must contain only letters, numbers, dot, underscore, or hyphen');
  }
  if (options.target && !/^[A-Za-z0-9][A-Za-z0-9._@-]{0,127}$/.test(options.target)) {
    throw new Error('target contains unsupported characters');
  }
  if (!isAbsolute(options.artifactDir)) throw new Error('artifact directory must be absolute');
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

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) try {
  const options = parseArgs(process.argv.slice(2));
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
