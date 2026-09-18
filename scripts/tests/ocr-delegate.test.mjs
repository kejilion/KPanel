import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

import { ocrArguments, ocrEnvironment, parseVersion, pinnedComponent, toolDirectory } from '../ocr-delegate.mjs';

test('only LLM-free delegation subcommands are forwarded', () => {
  assert.deepEqual(ocrArguments(['preview', '--from', 'abc^', '--to', 'def', '--format', 'json']),
    ['delegate', 'preview', '--from', 'abc^', '--to', 'def', '--format', 'json']);
  assert.deepEqual(ocrArguments(['rule', '--format', 'json', 'a.go']), ['delegate', 'rule', '--format', 'json', 'a.go']);
  assert.deepEqual(ocrArguments(['rules-check', 'a.go']), ['rules', 'check', 'a.go']);
  for (const command of ['review', 'scan', 'config', 'llm', undefined]) {
    assert.throws(() => ocrArguments(command === undefined ? [] : [command]), /forbidden|unsupported/);
  }
});

test('selection policy cannot be swapped per run', () => {
  assert.throws(() => ocrArguments(['preview', '--rule', 'other.json']), /forbidden/);
  assert.throws(() => ocrArguments(['preview', '--rule=other.json']), /forbidden/);
  assert.throws(() => ocrArguments(['preview', '--repo', '../other']), /forbidden/);
  assert.throws(() => ocrArguments(['preview', '--exclude', '**/*.test.ts']), /forbidden/);
  assert.throws(() => ocrArguments(['preview', '--exclude=**/*.test.ts']), /forbidden/);
  assert.throws(() => ocrArguments(['preview', 'stray.go']), /unexpected argument/);
  assert.throws(() => ocrArguments(['rule', '-b', 'context', 'a.go']), /forbidden/);
});

test('OCR runs with an isolated home and without OCR/OTEL overrides', () => {
  const env = ocrEnvironment({ PATH: 'p', OCR_ENABLE_TELEMETRY: '1', OTEL_EXPORTER_OTLP_ENDPOINT: 'x:4317', OCR_LLM_URL: 'u', HOME: '/root' }, '/iso');
  assert.deepEqual(env, { PATH: 'p', HOME: '/iso', USERPROFILE: '/iso', OCR_NO_UPDATE: '1' });
});

test('pin comes from dependency-policy.json and installs outside the repository', () => {
  const policy = JSON.parse(readFileSync(resolve(process.cwd(), 'dependency-policy.json'), 'utf8'));
  const { pinnedVersion } = pinnedComponent(policy);
  assert.match(pinnedVersion, /^\d+\.\d+\.\d+$/);
  assert.throws(() => pinnedComponent({ groups: [] }), /must pin/);
  assert.equal(parseVersion('open-code-review v1.12.5 (189be5b02) windows/amd64'), '1.12.5');
  assert.equal(parseVersion('garbage'), null);
  assert.equal(toolDirectory('1.12.5', { KPANEL_OCR_TOOL_DIR: '/opt/ocr' }), resolve('/opt/ocr'));
  assert.ok(toolDirectory('1.12.5', {}).endsWith('ocr-1.12.5'));
  assert.ok(!toolDirectory('1.12.5', {}).startsWith(resolve(process.cwd())));
});
