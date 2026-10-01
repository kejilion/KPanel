import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

const dockerfile = readFileSync(resolve(import.meta.dirname, '..', '..', 'packaging', 'release-runner', 'Dockerfile'), 'utf8');

test('release runner pins bases and direct packages and keeps npm executable', () => {
  assert.match(dockerfile, /^FROM node:26\.10\.0-alpine@sha256:[0-9a-f]{64} AS node-runtime$/m);
  assert.match(dockerfile, /^FROM golang:1\.26\.7-alpine@sha256:[0-9a-f]{64}$/m);
  for (const component of ['bash', 'build-base', 'ca-certificates', 'coreutils', 'docker-cli', 'docker-cli-buildx', 'git', 'make']) {
    assert.match(dockerfile, new RegExp(`\\s${component.replaceAll('-', '\\-')}=[^\\s\\\\]+`));
  }
  assert.match(dockerfile, /ln -s \.\.\/lib\/node_modules\/npm\/bin\/npm-cli\.js \/usr\/local\/bin\/npm/);
  assert.match(dockerfile, /npm --version/);
  assert.match(dockerfile, /npx --version/);
});

test('Node 26 preparation keeps image, Runner, CI, engines and declarations aligned', () => {
  const root = resolve(import.meta.dirname, '..', '..');
  const read = path => readFileSync(resolve(root, path), 'utf8');
  const nodeBase = dockerfile.match(/^FROM (node:[^\s]+) AS node-runtime$/m)?.[1];
  assert.ok(nodeBase);
  assert.ok(read('Dockerfile').includes(`FROM --platform=$BUILDPLATFORM ${nodeBase} AS web-build`));
  assert.ok(dockerfile.includes(`org.opencontainers.image.base.node="${nodeBase}"`));
  assert.ok(dockerfile.includes("node --version | grep -Fx 'v26.10.0'"));
  assert.ok(dockerfile.includes("npm --version | grep -Fx '11.19.1'"));
  for (const workflow of ['ci.yml', 'release.yml', 'dependency-freshness.yml']) {
    const versions = [...read(`.github/workflows/${workflow}`).matchAll(/node-version: "([^"]+)"/g)];
    assert.ok(versions.length > 0, `${workflow} must pin Node`);
    assert.ok(versions.every(([, version]) => version === '26.10.0'), workflow);
  }
  const manifest = JSON.parse(read('web/package.json'));
  const lock = JSON.parse(read('web/package-lock.json'));
  assert.equal(manifest.engines.node, '>=26.10.0');
  assert.equal(manifest.devDependencies['@types/node'], '26.6.3');
  assert.equal(lock.packages[''].engines.node, manifest.engines.node);
  assert.equal(lock.packages['node_modules/@types/node'].version, '26.6.3');
  const policy = JSON.parse(read('dependency-policy.json'));
  assert.equal(policy.stableRelease.nodeChannel, 'latest-lts');
  const experiment = policy.exceptions.find(item => item.component === 'Node.js LTS');
  assert.equal(experiment?.currentVersion, '26.10.0');
  assert.match(experiment.reason, /isolated.*experiment/);
});

test('release runner never imports mutable host binaries or floating base tags', () => {
  assert.doesNotMatch(dockerfile, /^FROM\s+[^\n@]+$/m);
  assert.doesNotMatch(dockerfile, /^COPY\s+(?!--from=node-runtime)/m);
  assert.doesNotMatch(dockerfile, /curl|wget|latest/);
});
