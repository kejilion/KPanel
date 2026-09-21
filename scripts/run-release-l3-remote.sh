#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

usage() {
  echo "usage: $0 /root/kpanel-release-inbox/RUN_ID/plan.env" >&2
  exit 2
}

[ "$#" -eq 1 ] || usage
plan=$1
[[ "$plan" =~ ^/root/kpanel-release-inbox/[A-Za-z0-9][A-Za-z0-9._-]{2,80}/plan\.env$ ]] || usage
inbox=$(dirname "$plan")

SCHEMA_VERSION=
RUN_ID=
EXPECTED_COMMIT=
BASE_MAIN_COMMIT=
EXPECTED_BASE_TAG=
BUSINESS_BASELINE_COMMIT=
BUSINESS_BASELINE_TAG=
RUNNER_IMAGE=
EXPECTED_RUNNER_ID=
RUNNER_ARCHIVE_FILE=
RUNNER_ARCHIVE_SHA256=
BUNDLE_FILE=
BUNDLE_SHA256=
REMOTE_SCRIPT_SHA256=
REQUIRED_TAGS=
declare -A seen_plan_keys=()

while IFS='=' read -r key value; do
  [ -n "$key" ] || continue
  case "$key" in
    SCHEMA_VERSION|RUN_ID|EXPECTED_COMMIT|BASE_MAIN_COMMIT|EXPECTED_BASE_TAG|BUSINESS_BASELINE_COMMIT|\
    BUSINESS_BASELINE_TAG|RUNNER_IMAGE|EXPECTED_RUNNER_ID|RUNNER_ARCHIVE_FILE|RUNNER_ARCHIVE_SHA256|\
    BUNDLE_FILE|BUNDLE_SHA256|REMOTE_SCRIPT_SHA256|REQUIRED_TAGS)
      [ -z "${seen_plan_keys[$key]+present}" ] || {
        echo "duplicate release plan key: $key" >&2
        exit 2
      }
      seen_plan_keys[$key]=1
      printf -v "$key" '%s' "$value"
      ;;
    *)
      echo "unknown release plan key: $key" >&2
      exit 2
      ;;
  esac
done < "$plan"

[ "$SCHEMA_VERSION" = 2 ] || usage
[[ "$RUN_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$ ]] || usage
[[ "$EXPECTED_COMMIT" =~ ^[0-9a-fA-F]{40,64}$ ]] || usage
[[ "$BASE_MAIN_COMMIT" =~ ^[0-9a-fA-F]{40,64}$ ]] || usage
[[ "$EXPECTED_BASE_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
[[ "$BUSINESS_BASELINE_COMMIT" =~ ^[0-9a-fA-F]{40,64}$ ]] || usage
[[ "$BUSINESS_BASELINE_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
[[ "$RUNNER_IMAGE" =~ ^[A-Za-z0-9][A-Za-z0-9._/@:+-]{0,254}$ ]] || usage
[[ "$EXPECTED_RUNNER_ID" =~ ^sha256:[0-9a-f]{64}$ ]] || usage
if [ -n "$RUNNER_ARCHIVE_FILE" ] || [ -n "$RUNNER_ARCHIVE_SHA256" ]; then
  [[ "$RUNNER_ARCHIVE_FILE" =~ ^kpanel-runner-[A-Za-z0-9._-]+\.tar$ ]] || usage
  [[ "$RUNNER_ARCHIVE_SHA256" =~ ^[0-9a-f]{64}$ ]] || usage
fi
[[ "$BUNDLE_FILE" =~ ^kpanel-[A-Za-z0-9._-]+\.bundle$ ]] || usage
[[ "$BUNDLE_SHA256" =~ ^[0-9a-f]{64}$ ]] || usage
[[ "$REMOTE_SCRIPT_SHA256" =~ ^[0-9a-f]{64}$ ]] || usage
[[ "$REQUIRED_TAGS" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(,v[0-9]+\.[0-9]+\.[0-9]+)*$ ]] || usage

[ "$(uname -s)" = Linux ] || {
  echo "remote L3 entrypoint requires Linux" >&2
  exit 1
}
for command in git docker sha256sum tee; do
  command -v "$command" >/dev/null || {
    echo "required command is missing: $command" >&2
    exit 1
  }
done
[ -S /var/run/docker.sock ] || {
  echo "Docker socket is required" >&2
  exit 1
}

script_path="$inbox/run-release-l3-remote.sh"
bundle="$inbox/$BUNDLE_FILE"
[ -f "$script_path" ] && [ -f "$bundle" ] || {
  echo "release inputs are incomplete" >&2
  exit 1
}
[ "$(sha256sum "$script_path" | awk '{print $1}')" = "$REMOTE_SCRIPT_SHA256" ] || {
  echo "remote entrypoint checksum mismatch" >&2
  exit 1
}
[ "$(sha256sum "$bundle" | awk '{print $1}')" = "$BUNDLE_SHA256" ] || {
  echo "release bundle checksum mismatch" >&2
  exit 1
}
if [ -n "$RUNNER_ARCHIVE_FILE" ]; then
  runner_archive="$inbox/$RUNNER_ARCHIVE_FILE"
  [ -f "$runner_archive" ] || {
    echo "runner archive is missing" >&2
    exit 1
  }
  [ "$(sha256sum "$runner_archive" | awk '{print $1}')" = "$RUNNER_ARCHIVE_SHA256" ] || {
    echo "runner archive checksum mismatch" >&2
    exit 1
  }
fi

work="/root/kpanel-release-work/$RUN_ID"
evidence="/root/kpanel-release-evidence/$RUN_ID"
[ "$(realpath -m "$work")" = "/root/kpanel-release-work/$RUN_ID" ] || exit 2
[ "$(realpath -m "$evidence")" = "/root/kpanel-release-evidence/$RUN_ID" ] || exit 2
[ ! -e "$work" ] && [ ! -e "$evidence" ] || {
  echo "run ID already has work or evidence; use a new run ID for every attempt" >&2
  exit 1
}
mkdir -p -m 700 "$work" "$evidence"

status=failed
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
finish() {
  rc=$?
  finished_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  {
    echo "run_id=$RUN_ID"
    echo "candidate=$EXPECTED_COMMIT"
    echo "base_main_commit=$BASE_MAIN_COMMIT"
    echo "base_tag=$EXPECTED_BASE_TAG"
    echo "status=$status"
    echo "exit_code=$rc"
    echo "started_at=$started_at"
    echo "finished_at=$finished_at"
  } > "$evidence/status.txt.tmp"
  mv "$evidence/status.txt.tmp" "$evidence/status.txt"
  find "$evidence" -maxdepth 1 -type f ! -name evidence.sha256 -print0 |
    sort -z | xargs -0 sha256sum > "$evidence/evidence.sha256" || true
}
trap finish EXIT

cp "$plan" "$script_path" "$bundle" "$inbox/manifest.json" "$evidence/"
sha256sum "$evidence/plan.env" "$evidence/run-release-l3-remote.sh" "$evidence/$BUNDLE_FILE" \
  "$evidence/manifest.json" > "$evidence/input.sha256"
if [ -n "$RUNNER_ARCHIVE_FILE" ]; then
  sha256sum "$runner_archive" >> "$evidence/input.sha256"
fi

verify_repo="$work/bundle-verify.git"
git init --bare "$verify_repo" > "$evidence/bundle-init.log"
git -C "$verify_repo" bundle verify "$bundle" > "$evidence/bundle-verify.log" 2>&1

candidate_repo="$work/candidate"
git clone --no-checkout "$bundle" "$candidate_repo" > "$evidence/bundle-clone.log" 2>&1
git -C "$candidate_repo" checkout --detach "$EXPECTED_COMMIT" >> "$evidence/bundle-clone.log" 2>&1
[ "$(git -C "$candidate_repo" rev-parse HEAD)" = "$EXPECTED_COMMIT" ]
[ -z "$(git -C "$candidate_repo" status --short --untracked-files=all)" ]
git -C "$candidate_repo" merge-base --is-ancestor "$EXPECTED_BASE_TAG" "$EXPECTED_COMMIT"
git -C "$candidate_repo" merge-base --is-ancestor "$BASE_MAIN_COMMIT" "$EXPECTED_COMMIT"
git -C "$candidate_repo" merge-base --is-ancestor "$BUSINESS_BASELINE_COMMIT" "$EXPECTED_COMMIT"
git -C "$candidate_repo" show-ref --verify "refs/tags/$BUSINESS_BASELINE_TAG" > /dev/null

IFS=',' read -r -a required_tags <<< "$REQUIRED_TAGS"
for tag in "${required_tags[@]}"; do
  git -C "$candidate_repo" show-ref --verify "refs/tags/$tag" > /dev/null
done

if [ -n "$RUNNER_ARCHIVE_FILE" ]; then
  docker load --input "$runner_archive" > "$evidence/runner-load.log" 2>&1
fi
if ! runner_id=$(docker image inspect "$RUNNER_IMAGE" --format '{{.Id}}' 2> "$evidence/runner-inspect.log"); then
  {
    echo "runner_image=$RUNNER_IMAGE"
    echo "expected_runner_id=$EXPECTED_RUNNER_ID"
    echo "runner_id=unavailable"
  } > "$evidence/runner.txt"
  echo "runner image is unavailable" >&2
  exit 1
fi
[[ "$runner_id" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  echo "runner image did not resolve to an immutable image ID" >&2
  exit 1
}
{
  echo "runner_image=$RUNNER_IMAGE"
  echo "expected_runner_id=$EXPECTED_RUNNER_ID"
  echo "runner_id=$runner_id"
} > "$evidence/runner.txt"
[ "$runner_id" = "$EXPECTED_RUNNER_ID" ] || {
  echo "runner image ID mismatch: expected $EXPECTED_RUNNER_ID, got $runner_id" >&2
  exit 1
}

set +e
(
  cd "$candidate_repo"
  bash scripts/run-release-gate.sh "$EXPECTED_COMMIT" "$EXPECTED_BASE_TAG" "$runner_id"
) 2>&1 | tee "$evidence/l3-verify-release.log"
gate_status=${PIPESTATUS[0]}
set -e
[ "$gate_status" -eq 0 ] || exit "$gate_status"

status=passed
echo "release_l3_gate=pass run_id=$RUN_ID candidate=$EXPECTED_COMMIT runner=$runner_id"
