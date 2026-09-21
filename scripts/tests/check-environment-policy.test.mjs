import assert from 'node:assert/strict';
import test from 'node:test';

import {
  checkEnvironment,
  loadPolicy,
  validatePolicy,
} from '../check-environment-policy.mjs';

const policy = loadPolicy();

test('environment policy is structurally valid', () => {
  assert.deepEqual(validatePolicy(policy), []);
});

test('arena-154 accepts validation and production deployment', () => {
  assert.equal(checkEnvironment(policy, 'arena-154', 'browser-validation').role, 'hybrid');
  assert.equal(checkEnvironment(policy, '154', 'candidate-validation').name, 'arena-154');
  assert.equal(checkEnvironment(policy, 'arena-154', 'production-deploy').name, 'arena-154');
  assert.equal(checkEnvironment(policy, '154', 'production-safety-check').role, 'hybrid');
});

test('local-wsl-dr is candidate-only and uses a fixed root WSL transport', () => {
  const environment = checkEnvironment(policy, 'local-wsl-dr', 'candidate-validation');
  assert.deepEqual(environment.transport, { kind: 'wsl', distribution: 'Ubuntu', user: 'root' });
  for (const purpose of [
    'browser-validation',
    'performance-validation',
    'failure-injection',
    'staging-deploy',
    'production-deploy',
    'production-safety-check',
  ]) {
    assert.throws(() => checkEnvironment(policy, 'local-wsl-dr', purpose), /does not allow/);
  }
});

test('prod-108 rejects every KPanel purpose', () => {
  for (const purpose of [
    'candidate-validation',
    'browser-validation',
    'performance-validation',
    'failure-injection',
    'staging-deploy',
    'production-deploy',
    'production-safety-check',
  ]) {
    assert.throws(() => checkEnvironment(policy, 'prod-108', purpose), /disabled for all KPanel operations/);
    assert.throws(() => checkEnvironment(policy, '108', purpose), /disabled for all KPanel operations/);
  }
});

test('unregistered hosts fail closed', () => {
  assert.throws(
    () => checkEnvironment(policy, 'temporary-host', 'browser-validation'),
    /environment is not registered/,
  );
});

test('disabled environments cannot be configured with an allowed purpose', () => {
  const invalid = structuredClone(policy);
  invalid.environments['prod-108'].allowedPurposes.push('browser-validation');
  assert.ok(validatePolicy(invalid).some((failure) => failure.includes('disabled environments must not allow')));
});

test('enabled environments require a closed transport configuration', () => {
  const missing = structuredClone(policy);
  delete missing.environments['arena-154'].transport;
  assert.ok(validatePolicy(missing).some((failure) => failure.includes('must declare a transport')));

  const extra = structuredClone(policy);
  extra.environments['local-wsl-dr'].transport.shell = 'bash';
  assert.ok(validatePolicy(extra).some((failure) => failure.includes('unsupported wsl transport field')));
});
