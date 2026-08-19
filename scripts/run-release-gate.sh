#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  echo "usage: $0 EXPECTED_COMMIT EXPECTED_BASE_TAG RUNNER_IMAGE" >&2
  exit 2
}

[ "$#" -eq 3 ] || usage
expected_commit=$1
expected_base_tag=$2
runner_image=$3

[[ "$expected_commit" =~ ^[0-9a-fA-F]{40,64}$ ]] || usage
[[ "$expected_base_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
[ "$(uname -s)" = Linux ] || {
  echo "release gate runner requires Linux" >&2
  exit 1
}

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repo_root"

actual_root=$(git rev-parse --show-toplevel)
actual_root=$(cd "$actual_root" && pwd -P)
[ "$actual_root" = "$repo_root" ] || {
  echo "release gate must run from the candidate repository" >&2
  exit 1
}
[ "$(git rev-parse HEAD)" = "$expected_commit" ] || {
  echo "candidate HEAD does not match EXPECTED_COMMIT" >&2
  exit 1
}
[ -z "$(git status --short --untracked-files=all)" ] || {
  echo "candidate worktree must be clean" >&2
  exit 1
}
git show-ref --verify --quiet "refs/tags/$expected_base_tag" || {
  echo "stable base tag is missing from the candidate repository" >&2
  exit 1
}
git merge-base --is-ancestor "$expected_base_tag" HEAD || {
  echo "stable base tag is not an ancestor of the candidate" >&2
  exit 1
}
[ -S /var/run/docker.sock ] || {
  echo "Docker socket is required for nested release gates" >&2
  exit 1
}

runner_id=$(docker image inspect "$runner_image" --format '{{.Id}}')
echo "release_gate_preflight=pass commit=$expected_commit base=$expected_base_tag runner=$runner_id"

docker run --rm \
  --entrypoint sh \
  -e "KPANEL_EXPECTED_COMMIT=$expected_commit" \
  -e "KPANEL_EXPECTED_BASE_TAG=$expected_base_tag" \
  -e "VERIFY_BASE_REF=$expected_base_tag" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$repo_root:$repo_root" \
  -w "$repo_root" \
  "$runner_id" \
  -c '
    set -eu
    test "$(git rev-parse HEAD)" = "$KPANEL_EXPECTED_COMMIT"
    test -z "$(git status --short --untracked-files=all)"
    git show-ref --verify --quiet "refs/tags/$KPANEL_EXPECTED_BASE_TAG"
    git merge-base --is-ancestor "$KPANEL_EXPECTED_BASE_TAG" HEAD
    npm ci --prefix web
    make verify-release
    docker run --rm \
      -e KPANEL_APP_CONF_TEST_ROOTFS=1 \
      -v "$PWD:/src:ro" \
      bash:5.2.37-alpine3.22@sha256:3bee76a96d86d5d2d5efc7c1c570e5a7c95db22348a26944e0e546fa174e3324 \
      bash /src/packaging/tests/app-conf-lifecycle.sh /src
  '

echo "release_gate_runner=pass commit=$expected_commit"
