#!/usr/bin/env node

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const defaultPolicyPath = resolve(repoRoot, 'environment-policy.json');

export const supportedPurposes = new Set([
  'candidate-validation',
  'browser-validation',
  'performance-validation',
  'failure-injection',
  'staging-deploy',
  'production-deploy',
  'production-safety-check',
]);

export function loadPolicy(policyPath = defaultPolicyPath) {
  return JSON.parse(readFileSync(policyPath, 'utf8'));
}

export function validatePolicy(policy) {
  const failures = [];
  if (policy?.schemaVersion !== 1) failures.push('schemaVersion must be 1');
  if (!policy?.environments || typeof policy.environments !== 'object') {
    return [...failures, 'environments must be an object'];
  }

  const identifiers = new Map();
  for (const [name, environment] of Object.entries(policy.environments)) {
    if (!['validation', 'production', 'hybrid'].includes(environment?.role)) {
      failures.push(`${name}: role must be validation, production, or hybrid`);
    }
    if (!Array.isArray(environment?.aliases)) failures.push(`${name}: aliases must be an array`);
    if (environment?.disabled !== undefined && typeof environment.disabled !== 'boolean') {
      failures.push(`${name}: disabled must be a boolean`);
    }
    if (!Array.isArray(environment?.allowedPurposes)) {
      failures.push(`${name}: allowedPurposes must be an array`);
    } else if (environment.disabled && environment.allowedPurposes.length > 0) {
      failures.push(`${name}: disabled environments must not allow any purpose`);
    } else if (!environment.disabled && environment.allowedPurposes.length === 0) {
      failures.push(`${name}: enabled environments must allow at least one purpose`);
    }
    for (const purpose of environment?.allowedPurposes ?? []) {
      if (!supportedPurposes.has(purpose)) failures.push(`${name}: unsupported purpose ${purpose}`);
      if (environment.role === 'production' && !['production-deploy', 'production-safety-check'].includes(purpose)) {
        failures.push(`${name}: production environments must not allow ${purpose}`);
      }
      if (environment.role === 'validation' && ['production-deploy', 'production-safety-check'].includes(purpose)) {
        failures.push(`${name}: validation environments must not allow ${purpose}`);
      }
    }
    for (const identifier of [name, ...(environment?.aliases ?? [])]) {
      const normalized = String(identifier).trim().toLowerCase();
      if (!normalized) {
        failures.push(`${name}: empty environment identifier`);
      } else if (identifiers.has(normalized)) {
        failures.push(`${name}: identifier ${identifier} duplicates ${identifiers.get(normalized)}`);
      } else {
        identifiers.set(normalized, name);
      }
    }
  }

  const prod108 = policy.environments['prod-108'];
  if (!prod108 || prod108.role !== 'production') failures.push('prod-108 must be registered as production');
  if (prod108?.disabled !== true) failures.push('prod-108 must remain disabled');
  if (prod108?.allowedPurposes?.length !== 0) failures.push('prod-108 must not allow any purpose');
  return failures;
}

export function resolveEnvironment(policy, identifier) {
  const normalized = String(identifier ?? '').trim().toLowerCase();
  for (const [name, environment] of Object.entries(policy.environments ?? {})) {
    const candidates = [name, ...(environment.aliases ?? [])].map((value) => String(value).toLowerCase());
    if (candidates.includes(normalized)) return { name, ...environment };
  }
  return null;
}

export function checkEnvironment(policy, identifier, purpose) {
  const failures = validatePolicy(policy);
  if (failures.length > 0) throw new Error(`invalid environment policy: ${failures.join('; ')}`);
  if (!supportedPurposes.has(purpose)) throw new Error(`unsupported purpose: ${purpose}`);
  const environment = resolveEnvironment(policy, identifier);
  if (!environment) {
    throw new Error(`environment is not registered: ${identifier}; register dedicated test hosts before use`);
  }
  if (environment.disabled) {
    throw new Error(`${environment.name} is disabled for all KPanel operations`);
  }
  if (!environment.allowedPurposes.includes(purpose)) {
    throw new Error(`${environment.name} (${environment.role}) does not allow ${purpose}`);
  }
  return environment;
}

function parseArgs(argv) {
  const options = { policyPath: defaultPolicyPath, validateOnly: false };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === '--validate-only') options.validateOnly = true;
    else if (arg === '--policy') options.policyPath = resolve(argv[++index]);
    else if (arg === '--environment') options.environment = argv[++index];
    else if (arg === '--purpose') options.purpose = argv[++index];
    else throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

export function main(argv = process.argv.slice(2)) {
  const options = parseArgs(argv);
  const policy = loadPolicy(options.policyPath);
  const failures = validatePolicy(policy);
  if (failures.length > 0) throw new Error(failures.join('\n'));
  if (options.validateOnly) {
    process.stdout.write('Environment policy validation passed.\n');
    return;
  }
  if (!options.environment || !options.purpose) {
    throw new Error('--environment and --purpose are required');
  }
  const environment = checkEnvironment(policy, options.environment, options.purpose);
  process.stdout.write(`environment_policy=pass environment=${environment.name} role=${environment.role} purpose=${options.purpose}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    process.stderr.write(`Environment policy check failed: ${error.message}\n`);
    process.exit(1);
  }
}
