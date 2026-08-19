#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

node scripts/check-governance-consistency.mjs
node scripts/check-business-context-freshness.mjs
node scripts/check-environment-policy.mjs --validate-only
node --test \
  scripts/tests/check-environment-policy.test.mjs \
  scripts/tests/background-browser-test.test.mjs \
  scripts/tests/local-feature-preview.test.mjs \
  scripts/tests/release-gate-runner.test.mjs \
  scripts/tests/verify-change-forced-level.test.mjs \
  scripts/tests/business-context-freshness.test.mjs \
  scripts/tests/report-release-metrics.test.mjs \
  scripts/tests/report-dependency-freshness.test.mjs
node scripts/report-dependency-freshness.mjs --validate-only

echo "Governance verification passed."
