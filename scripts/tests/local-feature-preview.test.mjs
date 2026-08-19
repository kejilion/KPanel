import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  choosePort,
  parseArgs,
  validateLoopbackTarget,
  validateKnownOptions,
  validateStartOptions,
} from '../local-feature-preview.mjs';

test('argument parsing keeps the preview contract explicit', () => {
  assert.deepEqual(
    parseArgs(['start', '--scope', 'docker-compose', '--mode', 'mock', '--grade', 'draft']),
    {
      command: 'start',
      options: { scope: 'docker-compose', mode: 'mock', grade: 'draft' },
    },
  );
});

test('argument parsing rejects duplicate and unknown options', () => {
  assert.throws(() => parseArgs(['start', '--scope', 'a', '--scope', 'b']), /duplicate option/);
  assert.throws(() => validateKnownOptions('start', { scope: 'a', token: 'secret' }), /unknown option/);
});

test('integration preview only accepts secret-free loopback targets', () => {
  assert.equal(validateLoopbackTarget('http://127.0.0.1:8866/api'), 'http://127.0.0.1:8866');
  assert.equal(validateLoopbackTarget('http://localhost:8080'), 'http://localhost:8080');
  assert.throws(() => validateLoopbackTarget('https://example.com'), /only accepts loopback/);
  assert.throws(() => validateLoopbackTarget('http://user:secret@127.0.0.1:8080'), /must not contain credentials/);
});

test('acceptance preview requires a clean checkpoint', () => {
  assert.throws(
    () => validateStartOptions({ scope: 'docker-compose', mode: 'mock', grade: 'acceptance' }, { clean: false }),
    /requires a clean checkpoint/,
  );
  assert.deepEqual(
    validateStartOptions({ scope: 'docker-compose', mode: 'mock', grade: 'draft' }, { clean: false }),
    { scope: 'docker-compose', mode: 'mock', grade: 'draft' },
  );
});

test('mock and integration options cannot be silently mixed', () => {
  assert.throws(
    () => validateStartOptions({ scope: 'docker-compose', mode: 'mock', 'api-target': 'http://127.0.0.1:8080' }, { clean: true }),
    /only valid in integration mode/,
  );
  assert.throws(
    () => validateStartOptions({ scope: 'docker-compose', mode: 'integration' }, { clean: true }),
    /requires --api-target/,
  );
  assert.throws(
    () => validateStartOptions({ scope: 'docker-compose', mode: 'mock', 'change-origin': 'yes' }, { clean: true }),
    /must be true or false/,
  );
});

test('automatic port selection returns distinct available ports', async () => {
  const first = await choosePort(undefined, 49100);
  const second = await choosePort(undefined, 49100, new Set([first]));
  assert.notEqual(first, second);
});
