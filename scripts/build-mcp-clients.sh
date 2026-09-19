#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
output="${1:-dist/mcp-clients}"
version="$(tr -d '\r\n' < VERSION)"
mkdir -p "$output"
for target_os in linux darwin windows; do
  for target_arch in amd64 arm64; do
    suffix=""
    [[ "$target_os" != windows ]] || suffix=".exe"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath \
      -ldflags "-s -w -X github.com/kejilion/kejilion-panel/internal/version.Version=$version" \
      -o "$output/kpanel-mcp-$target_os-$target_arch$suffix" ./cmd/kpanel-mcp
  done
done
