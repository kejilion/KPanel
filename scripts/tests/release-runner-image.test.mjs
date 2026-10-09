import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

const dockerfile = readFileSync(resolve(import.meta.dirname, '..', '..', 'packaging', 'release-runner', 'Dockerfile'), 'utf8');

test('release runner pins bases and direct packages and keeps npm executable', () => {
  assert.match(dockerfile, /^FROM node:24\.21\.0-alpine@sha256:[0-9a-f]{64} AS node-runtime$/m);
  assert.match(dockerfile, /^FROM golang:1\.27\.2-alpine@sha256:[0-9a-f]{64}$/m);
  for (const component of ['bash', 'build-base', 'ca-certificates', 'coreutils', 'docker-cli', 'docker-cli-buildx', 'git', 'make']) {
    assert.match(dockerfile, new RegExp(`\\s${component.replaceAll('-', '\\-')}=[^\\s\\\\]+`));
  }
  assert.match(dockerfile, /ln -s \.\.\/lib\/node_modules\/npm\/bin\/npm-cli\.js \/usr\/local\/bin\/npm/);
  assert.match(dockerfile, /npm --version/);
  assert.match(dockerfile, /npx --version/);
});

test('release runner never imports mutable host binaries or floating base tags', () => {
  assert.doesNotMatch(dockerfile, /^FROM\s+[^\n@]+$/m);
  assert.doesNotMatch(dockerfile, /^COPY\s+(?!--from=node-runtime)/m);
  assert.doesNotMatch(dockerfile, /curl|wget|latest/);
});

test('Go toolchain stays aligned across modules, images, Runner and CI', () => {
  const root = resolve(import.meta.dirname, '..', '..');
  const read = path => readFileSync(resolve(root, path), 'utf8');
  const goVersion = read('go.mod').match(/^go (\d+\.\d+\.\d+)$/m)?.[1];
  assert.equal(goVersion, '1.27.2');
  const goBase = dockerfile.match(/^FROM (golang:[^\s]+)$/m)?.[1];
  assert.ok(goBase?.startsWith(`golang:${goVersion}-alpine@sha256:`));
  assert.ok(read('Dockerfile').includes(`FROM --platform=$BUILDPLATFORM ${goBase} AS go-build`));
  assert.ok(dockerfile.includes(`org.opencontainers.image.base.golang="${goBase}"`));
  assert.ok(dockerfile.includes(`go version | grep -F 'go${goVersion}'`));
  for (const workflow of ['ci.yml', 'release.yml', 'dependency-freshness.yml']) {
    const versions = [...read(`.github/workflows/${workflow}`).matchAll(/go-version: "([^"]+)"/g)];
    assert.ok(versions.length > 0, `${workflow} must pin Go`);
    assert.ok(versions.every(([, version]) => version === goVersion), workflow);
  }
  assert.ok(read('.codex-workflows/kpanel-real-machine-app-lifecycle.workflow.yaml').includes(goBase));
  const siteWorkflow = read('.codex-workflows/kpanel-site-icon-cache-validation.workflow.yaml');
  const sitePins = [...siteWorkflow.matchAll(/golang:([^\s]+)/g)];
  assert.equal(sitePins.length, 3);
  assert.ok(sitePins.every(([, pin]) => pin === `${goVersion}-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`));
});

test('release runner accepts the optional build proxy only as a BuildKit secret', () => {
  assert.match(dockerfile, /^# syntax=docker\/dockerfile:\d+\.\d+\.\d+@sha256:[0-9a-f]{64}$/m);
  assert.match(dockerfile, /--mount=type=secret,id=https_proxy,required=false/);
  assert.match(dockerfile, /cat \/run\/secrets\/https_proxy/);
  assert.doesNotMatch(dockerfile, /^(?:ARG|ENV)\s+.*(?:https?_proxy|HTTPS?_PROXY)/m);
});
