import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import {
  evaluateFreshness,
  extractBusinessContextMetadata,
  gitEnvironment,
  nearestStableVersion,
  releaseCountAfter,
} from '../check-business-context-freshness.mjs';

function git(repo, ...args) {
  const env = { ...process.env };
  delete env.GIT_DIR;
  delete env.GIT_WORK_TREE;
  delete env.GIT_INDEX_FILE;
  return execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8', env }).trim();
}

function makeCommit(repo, message) {
  git(repo, 'commit', '--allow-empty', '-m', message);
  return git(repo, 'rev-parse', 'HEAD');
}

test('extractBusinessContextMetadata reads the canonical machine fields', () => {
  const metadata = extractBusinessContextMetadata([
    '- 复核日期：`2026-08-15`',
    '- 基线提交：`565e476623159247ec3ebb6967ab0d6753f165d1`',
    '- 基线版本：`v0.73.2`',
  ].join('\n'));
  assert.deepEqual(metadata, {
    reviewedAt: '2026-08-15',
    baselineCommit: '565e476623159247ec3ebb6967ab0d6753f165d1',
    baselineVersion: 'v0.73.2',
  });
});

test('freshness remains cheap until a meaningful change-volume threshold is reached', () => {
  assert.deepEqual(evaluateFreshness({ commitCount: 49, releaseCount: 7 }), { stale: false, reasons: [] });
  assert.equal(evaluateFreshness({ commitCount: 50, releaseCount: 0 }).stale, true);
  assert.equal(evaluateFreshness({ commitCount: 0, releaseCount: 8 }).stale, false);
  assert.equal(evaluateFreshness({ commitCount: 20, releaseCount: 8 }).stale, true);
});

test('business context Git reads ignore all caller repository overrides', () => {
  const foreign = {
    GIT_DIR: 'foreign/.git',
    GIT_WORK_TREE: process.cwd(),
    GIT_INDEX_FILE: 'foreign/.git/index',
    GIT_COMMON_DIR: 'foreign/.git',
    GIT_OBJECT_DIRECTORY: 'foreign/.git/objects',
    GIT_ALTERNATE_OBJECT_DIRECTORIES: 'foreign/alternate',
    PATH: process.env.PATH,
  };
  const isolated = gitEnvironment(process.cwd(), foreign);
  assert.notEqual(isolated.GIT_DIR, foreign.GIT_DIR);
  if (isolated.GIT_DIR) {
    assert.equal(isolated.GIT_WORK_TREE?.replaceAll('\\', '/'), process.cwd().replaceAll('\\', '/'));
  } else {
    assert.equal(isolated.GIT_WORK_TREE, undefined);
  }
  assert.equal(isolated.GIT_INDEX_FILE, undefined);
  assert.equal(isolated.GIT_COMMON_DIR, undefined);
  assert.equal(isolated.GIT_OBJECT_DIRECTORY, undefined);
  assert.equal(isolated.GIT_ALTERNATE_OBJECT_DIRECTORIES, undefined);
  assert.equal(isolated.PATH, process.env.PATH);
});

test('release counting and baseline version ignore prerelease and non-version tags', () => {
  const repo = mkdtempSync(join(tmpdir(), 'kpanel-business-context-'));
  try {
    git(repo, 'init', '--quiet');
    git(repo, 'config', 'user.name', 'KPanel Test');
    git(repo, 'config', 'user.email', 'kpanel-test@example.invalid');
    const baselineTagCommit = makeCommit(repo, 'stable release');
    git(repo, 'tag', 'v1.2.3', baselineTagCommit);
    const baseline = makeCommit(repo, 'acceptance record');
    git(repo, 'tag', 'backup-2026', baseline);
    git(repo, 'tag', 'v9.0.0-rc.1', baseline);
    const releaseCommit = makeCommit(repo, 'next release');
    git(repo, 'tag', '-a', 'v1.2.4', '-m', 'stable annotated release', releaseCommit);
    for (const tag of ['v1.2.5-rc.1', 'v1.2.5+build.1', 'vnightly', 'vfoo', 'v2.0.0-alpha', 'v2026-nightly', 'v1', 'v1.2']) {
      git(repo, 'tag', tag, releaseCommit);
    }
    const target = makeCommit(repo, 'post release');

    assert.equal(nearestStableVersion(repo, baseline), 'v1.2.3');
    assert.equal(releaseCountAfter(repo, baseline, target), 1);
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
});
