#!/usr/bin/env bash
set -euo pipefail

# Fixed lanes only. run-source-checks.mjs owns qualification, bounded scheduling and evidence.
case "${1:-}" in
  web)
    cd web
    npm ci
    npm run typecheck
    npm test
    npm run build
    ;;
  go)
    go test ./...
    go test -race ./internal/panel ./internal/auth ./internal/dockerx
    go vet ./...
    ;;
  deploy)
    bash scripts/verify-deploy.sh
    ;;
  preflight)
    missing=()
    for tool in git node npm go bash; do
      command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
    done
    for input in go.mod web/package.json web/package-lock.json scripts/verify-deploy.sh; do
      [[ -f "$input" ]] || missing+=("$input")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
      printf 'source_preflight=fail missing=%s\n' "${missing[*]}" >&2
      exit 1
    fi
    git --version
    node --version
    npm --version
    go version
    bash --version | sed -n '1p'
    ;;
  *)
    echo 'usage: verify-source-lane.sh web|go|deploy|preflight' >&2
    exit 2
    ;;
esac
