#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# Both draft and final publication use the runtime parser on the rendered artifact.
bash scripts/render-release-notes.sh "$@"
KPANEL_RELEASE_NOTES_FILE="$4" KPANEL_RELEASE_NOTES_VERSION="$1" go test ./internal/selfupdate \
  -run '^TestRenderedReleaseNotesForPublication$' -count=1
