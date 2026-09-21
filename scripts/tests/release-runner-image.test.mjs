import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

const dockerfile = readFileSync(resolve(import.meta.dirname, '..', '..', 'packaging', 'release-runner', 'Dockerfile'), 'utf8');

test('release runner pins bases and direct packages and keeps npm executable', () => {
  assert.match(dockerfile, /^FROM node:24\.20\.0-alpine@sha256:[0-9a-f]{64} AS node-runtime$/m);
  assert.match(dockerfile, /^FROM golang:1\.26\.7-alpine@sha256:[0-9a-f]{64}$/m);
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
