#!/usr/bin/env bash
set -Eeuo pipefail
test "${GITHUB_ACTIONS:-}" = true && test "${RUNNER_ENVIRONMENT:-}" = github-hosted
: "${RUNNER_TEMP:?}" "${EXPECTED_VERSION:?}" "${EXPECTED_SHA:?}" "${IMAGE_DIGEST:?}"
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
collect_permissions() {
  for path in "$RUNNER_TEMP"/kpanel-{real,browser}-*/results "$PUBLIC_EVIDENCE"; do
    test ! -d "$path" || sudo chown -R "$(id -u):$(id -g)" "$path"
  done
}
trap collect_permissions EXIT
export RELEASE_TEST_RUNNER="kpanel-release-test:${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
export RELEASE_BROWSER_RUNTIME="$RUNNER_TEMP/kpanel-browser-runtime"
# Pass only the frozen acceptance inputs, even when sudo keeps no environment.
validation_env=(
  'PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
  "RUNNER_TEMP=${RUNNER_TEMP:?}" "EXPECTED_VERSION=${EXPECTED_VERSION:?}"
  "EXPECTED_SHA=${EXPECTED_SHA:?}" "IMAGE_DIGEST=${IMAGE_DIGEST:?}"
  "RELEASE_TEST_RUNNER=${RELEASE_TEST_RUNNER:?}" "RELEASE_BROWSER_RUNTIME=${RELEASE_BROWSER_RUNTIME:?}"
  "GITHUB_ACTIONS=${GITHUB_ACTIONS:?}" "RUNNER_ENVIRONMENT=${RUNNER_ENVIRONMENT:?}"
  "PREVIOUS_PREVIEW_VERSION=${PREVIOUS_PREVIEW_VERSION:?}" "PREVIOUS_PREVIEW_IMAGE=${PREVIOUS_PREVIEW_IMAGE:?}"
  "EXPECTED_NOTES=${EXPECTED_NOTES:?}" "EXPECTED_UPGRADE_NOTES=${EXPECTED_UPGRADE_NOTES:?}"
  "EXPECTED_STABLE_VERSION=${EXPECTED_STABLE_VERSION:?}"
)
mkdir "$RELEASE_BROWSER_RUNTIME"
# Full package version and matching browser image are fixed together. The registry
# integrity in package-lock.json is retained in the evidence, with no global install.
cp "$repo/packaging/release-browser/package"*.json "$RELEASE_BROWSER_RUNTIME/"
npm ci --prefix "$RELEASE_BROWSER_RUNTIME" --ignore-scripts --no-audit --no-fund
npm audit --prefix "$RELEASE_BROWSER_RUNTIME" --audit-level=high
chmod -R a+rX "$RELEASE_BROWSER_RUNTIME"
docker build -f "$repo/packaging/release-runner/Dockerfile" -t "$RELEASE_TEST_RUNNER" "$repo"
docker image inspect "$RELEASE_TEST_RUNNER" > "$PUBLIC_EVIDENCE/test-runner.json"
image="docker.io/kjlion/kejilion-panel@$IMAGE_DIGEST"
sudo env -i "${validation_env[@]}" timeout --kill-after=30s 15m bash "$repo/packaging/tests/release-browser/real-feature-public.sh" "$image" public "$RUNNER_TEMP/kpanel-real-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT" "$IMAGE_DIGEST"
for mode in published upgrade; do
  sudo env -i "${validation_env[@]}" timeout --kill-after=30s 15m bash "$repo/packaging/tests/release-browser/public-browser-complete.sh" "$image" "$mode" "$RUNNER_TEMP/kpanel-browser-$mode-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT" "$IMAGE_DIGEST"
done
printf 'Browser and real-feature validation passed for %s at %s\n' "$EXPECTED_VERSION" "$EXPECTED_SHA" >> "$GITHUB_STEP_SUMMARY"
