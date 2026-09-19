#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';

// This entry only retires one stable release candidate. Authorization and public
// artifact verification belong to its caller; preview trains never enter here.
export function archiveReleaseCandidate({ repo = '.', tag, releaseSha, apply = false }) {
  if (!/^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$/.test(tag ?? '')) {
    throw new Error('A canonical stable release tag is required');
  }
  if (!/^[0-9a-f]{40}$/.test(releaseSha ?? '')) throw new Error('An exact release SHA is required');
  const candidate = `refs/heads/release/${tag}-candidate`;
  const archive = `refs/heads/archive/release/${tag}-candidate`;
  const tagRef = `refs/tags/${tag}`;
  const env = { ...process.env, GIT_TERMINAL_PROMPT: '0' };
  // Actions checkout deliberately does not persist credentials. Scope its
  // temporary token to GitHub HTTPS in this subprocess environment only.
  if (env.GITHUB_ACTIONS === 'true' && env.GITHUB_TOKEN) {
    const index = Number(env.GIT_CONFIG_COUNT || 0);
    if (!Number.isSafeInteger(index) || index < 0) throw new Error('Invalid Git config count');
    env.GIT_CONFIG_COUNT = String(index + 1);
    env[`GIT_CONFIG_KEY_${index}`] = 'http.https://github.com/.extraheader';
    env[`GIT_CONFIG_VALUE_${index}`] = `AUTHORIZATION: basic ${Buffer.from(`x-access-token:${env.GITHUB_TOKEN}`).toString('base64')}`;
  }
  delete env.GITHUB_TOKEN;
  function git(args, allowed = [0]) {
    const result = spawnSync('git', args, { cwd: repo, env, encoding: 'utf8', timeout: 60000, maxBuffer: 4 * 1024 * 1024 });
    if (result.error || !allowed.includes(result.status)) {
      throw new Error(`git ${args[0]} failed; candidate archival is unverified (exit ${result.status})`);
    }
    return { output: result.stdout.trim(), status: result.status };
  }
  function remoteRefs() {
    const output = git(['ls-remote', 'origin', candidate, archive, tagRef, `${tagRef}^{}`]).output;
    return new Map(output ? output.split('\n').map(line => {
      const [sha, ref] = line.trim().split(/\s+/);
      return [ref, sha];
    }) : []);
  }
  function checkTag(refs) {
    if ((refs.get(`${tagRef}^{}`) || refs.get(tagRef)) !== releaseSha) {
      throw new Error('Remote release tag does not match the exact release SHA');
    }
  }
  const refs = remoteRefs(); // Errors are never interpreted as missing branches.
  checkTag(refs);
  const candidateSha = refs.get(candidate);
  const archiveSha = refs.get(archive);
  if (!candidateSha && !archiveSha) throw new Error('Candidate absent without an archive; historical recovery evidence is required');
  if (candidateSha && archiveSha && candidateSha !== archiveSha) throw new Error('Archive collision; preserve both branches');
  const savedSha = candidateSha || archiveSha;
  const shallow = git(['rev-parse', '--is-shallow-repository']).output === 'true';
  git(['fetch', '--no-tags', ...(shallow ? ['--unshallow'] : []), 'origin', tagRef]);
  if (git(['rev-parse', 'FETCH_HEAD^{commit}']).output !== releaseSha) throw new Error('Release tag moved during verification');
  git(['fetch', '--no-tags', 'origin', savedSha]);
  if (git(['merge-base', '--is-ancestor', savedSha, releaseSha], [0, 1]).status !== 0) {
    throw new Error('Candidate is not contained in the stable release tag');
  }
  if (!candidateSha) return { status: 'already-archived', candidate, archive, sha: savedSha, tag, releaseSha };
  if (!apply) return { status: 'ready', candidate, archive, sha: savedSha, tag, releaseSha };
  // One transaction, with expected old values on BOTH refs. A concurrent writer,
  // archive collision, rejected deletion or unsupported atomic push preserves all.
  git(['push', '--atomic',
    `--force-with-lease=${candidate}:${candidateSha}`,
    `--force-with-lease=${archive}:${archiveSha || ''}`,
    'origin', `${savedSha}:${archive}`, `:${candidate}`]);
  const after = remoteRefs();
  checkTag(after);
  if (after.has(candidate) || after.get(archive) !== savedSha) throw new Error('Remote archival verification failed');
  return { status: 'archived', candidate, archive, sha: savedSha, tag, releaseSha };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const { values } = parseArgs({ options: {
      repo: { type: 'string', default: '.' },
      tag: { type: 'string' },
      'release-sha': { type: 'string' },
      apply: { type: 'boolean', default: false },
    } });
    console.log(JSON.stringify(archiveReleaseCandidate({ repo: values.repo, tag: values.tag, releaseSha: values['release-sha'], apply: values.apply })));
  } catch (error) {
    console.error(`candidate_archive=fail ${error.message}`);
    process.exitCode = 1;
  }
}
