import { spawnSync } from 'node:child_process';
import { appendFileSync } from 'node:fs';

const sha = process.env.CANDIDATE;
if (process.env.GITHUB_ACTIONS !== 'true' || process.env.RUNNER_ENVIRONMENT !== 'github-hosted'
    || process.env.GITHUB_REPOSITORY !== 'kejilion/KPanel' || !/^[0-9a-f]{40}$/.test(sha ?? '')) {
  throw new Error('Canonical GitHub-hosted workflow and exact candidate SHA required');
}
const execute = (program, args, extra = {}) => {
  const result = spawnSync(program, args, { encoding: 'utf8', shell: false, timeout: 60_000, ...extra });
  // Never log authentication arguments, environment or remote diagnostics.
  if (result.error || result.status !== 0) throw new Error(`${program} failed (exit ${result.status})`);
  return result.stdout.trim();
};
const git = (...args) => execute('git', args);
git('fetch', 'origin', '--no-tags', '+refs/heads/main:refs/remotes/origin/main');
git('merge-base', '--is-ancestor', 'origin/main', sha);
const version = git('show', `${sha}:VERSION`).trim();
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-rc\.([1-9]\d*)$/.test(version)
    || version.split(/\D+/).some(n => Number(n) > 999999)) throw new Error('Valid preview version required');
const tag = `v${version}`, branch = `release/v${version.split('-rc.')[0]}-candidate`;
const remote = git('ls-remote', 'origin', `refs/heads/${branch}`, `refs/tags/${tag}`);
if (remote.split('\n').some(line => line.endsWith(`refs/tags/${tag}`))) throw new Error('Version tag already exists; immutable versions cannot be retried');
if (remote !== `${sha}\trefs/heads/${branch}`) throw new Error('Candidate branch does not match the frozen SHA');
// The workflow implementation must already be trusted on main: a dispatch never executes
// release automation supplied only by an unreviewed candidate.
const automation = ref => git('ls-tree', '-r', ref, '--', '.github/workflows', 'scripts', 'Makefile',
  'packaging/tests', 'packaging/release-browser', 'packaging/release-runner')
  .split('\n').filter(line => !line.endsWith('\tscripts/torrent-backport-scan-scope.json')
    && !line.endsWith('\tscripts/mock-app-market-api.mjs')).join('\n');
// Source-version qualification intentionally changes with every release version;
// all executable release helpers, including newly added files, must already be trusted.
// The local mock API is test infrastructure, not release automation.
if (automation('origin/main') !== automation(sha)) throw new Error('Release automation must be integrated on main before dispatch');
if (process.argv[2] === 'check') {
  if (process.env.GITHUB_REF !== 'refs/heads/main') throw new Error('Dispatch Publish preview from main only');
  const mainVersion = git('show', 'origin/main:VERSION').trim();
  const parseVersion = value => {
    const match = value.match(/^(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/);
    if (!match) throw new Error('Main version identity is invalid');
    return [Number(match[1]), Number(match[2]), Number(match[3]), match[4] ? Number(match[4]) : Infinity];
  };
  const next = parseVersion(version), current = parseVersion(mainVersion);
  const firstDifference = next.findIndex((number, i) => number !== current[i]);
  if (firstDifference === -1 || next[firstDifference] < current[firstDifference]) throw new Error('Preview version must advance the current main version');
  const runs = JSON.parse(execute('gh', ['api', `repos/kejilion/KPanel/actions/workflows/ci.yml/runs?event=push&head_sha=${sha}&per_page=100`]));
  if (!runs.workflow_runs.some(run => run.head_sha === sha && run.head_branch === branch && run.status === 'completed' && run.conclusion === 'success')) {
    throw new Error('Frozen candidate must first pass its own push CI');
  }
  appendFileSync(process.env.GITHUB_OUTPUT, `candidate=${sha}\ntag=${tag}\nbranch=${branch}\nbase=${git('rev-parse', 'origin/main')}\n`);
} else if (process.argv[2] === 'promote') {
  if (process.env.RELEASE_TAG !== tag || process.env.RELEASE_BRANCH !== branch) throw new Error('Frozen release identity changed');
  const before = git('rev-parse', 'origin/main');
  if (before !== process.env.RELEASE_BASE) throw new Error('Main moved since candidate qualification; stop and refreeze');
  git('config', 'user.name', 'github-actions[bot]');
  git('config', 'user.email', '41898282+github-actions[bot]@users.noreply.github.com');
  git('tag', '-a', tag, sha, '-m', `KPanel ${version}; GitHub run ${process.env.GITHUB_RUN_ID}`);
  // GITHUB_TOKEN pushes do not recursively trigger CI/Release. The caller explicitly
  // invokes both reusable workflows, so failures cannot silently end the chain.
  const auth = Buffer.from(`x-access-token:${process.env.GH_TOKEN}`).toString('base64');
  const env = { ...process.env, GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'http.https://github.com/.extraheader', GIT_CONFIG_VALUE_0: `AUTHORIZATION: basic ${auth}` };
  execute('git', ['push', '--atomic', `--force-with-lease=refs/heads/main:${before}`,
    'https://github.com/kejilion/KPanel.git', `${sha}:refs/heads/main`, `refs/tags/${tag}`], { env });
  if (git('ls-remote', 'origin', 'refs/heads/main') !== `${sha}\trefs/heads/main`) throw new Error('Main publication readback failed');
  if (!git('ls-remote', 'origin', `refs/tags/${tag}^{}`).startsWith(`${sha}\t`)) throw new Error('Tag publication readback failed');
} else throw new Error('Expected check or promote');
