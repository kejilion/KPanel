import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

// PROJECT_RULES 5.4 audit-disclosure boundary: the machine floor in
// check-governance-consistency.mjs must exist, match the documented artifact
// classes, and keep the fix-evidence predicate product-source scoped. This
// suite pins the structure; end-to-end git semantics are exercised by the
// two-branch scenario documented in the 5.4 adoption candidate.

const source = readFileSync(
  resolve(import.meta.dirname, '..', 'check-governance-consistency.mjs'),
  'utf8',
);

test('audit-disclosure check exists and is invoked', () => {
  assert.match(source, /function checkAuditArtifactDisclosure\(\)/);
  assert.match(source, /checkAuditArtifactDisclosure\(\);/);
});

test('sensitive artifact patterns cover findings detail and findings json', () => {
  assert.match(source, /FINDINGS-DETAIL\.md/);
  assert.match(source, /findings\.json/);
});

test('ledger and metadata paths are explicitly not flagged as sensitive', () => {
  assert.ok(source.includes('coverage-ledger'), 'ledger must be in the insensitive list');
  assert.ok(source.includes('run-metadata'), 'metadata must be in the insensitive list');
  assert.ok(source.includes('architecture'), 'architecture must be in the insensitive list');
  assert.ok(source.includes('NEEDS-VALIDATION'), 'needs-validation must be in the insensitive list');
});

test('empty findings arrays do not carry attack detail', () => {
  assert.match(source, /carriesConfirmed/);
  // The conservative path on unparseable findings must remain flagged.
  const conservativeIndex = source.indexOf('// unparseable findings: be conservative');
  assert.ok(conservativeIndex > 0, 'conservative unparseable-findings handling must be documented');
});

test('fix evidence requires product-source commits', () => {
  assert.ok(
    source.includes('/^(internal|cmd|web)\\//'),
    'product-source fix predicate must remain in the disclosure check',
  );
});

test('absent origin/main skips the machine floor but keeps the rule textual', () => {
  assert.match(source, /No origin\/main ref/);
});
