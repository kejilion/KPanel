#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

node scripts/check-governance-consistency.mjs
node scripts/check-business-context-freshness.mjs
node scripts/check-environment-policy.mjs --validate-only
node --test \
  scripts/tests/check-environment-policy.test.mjs \
  scripts/tests/governance-candidate-ci.test.mjs \
  scripts/tests/collaboration-state.test.mjs \
  scripts/tests/background-browser-test.test.mjs \
  scripts/tests/local-feature-preview.test.mjs \
  scripts/tests/release-gate-runner.test.mjs \
  scripts/tests/release-l3-orchestrator.test.mjs \
  scripts/tests/production-evidence-orchestrator.test.mjs \
  scripts/tests/run-repo-bash.test.mjs \
  scripts/tests/verify-change-forced-level.test.mjs \
  scripts/tests/business-context-freshness.test.mjs \
  scripts/tests/report-release-metrics.test.mjs \
  scripts/tests/report-dependency-freshness.test.mjs \
  scripts/tests/release-acceptance-coverage.test.mjs
node scripts/report-dependency-freshness.mjs --validate-only
node scripts/check-release-acceptance-coverage.mjs

echo "Governance verification passed."
