#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-source}"
trivy_image="${TRIVY_IMAGE:-aquasec/trivy@sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969}"
cache_dir="${TRIVY_CACHE_DIR:-${TMPDIR:-/tmp}/kpanel-trivy-cache}"

command -v docker >/dev/null 2>&1 || {
  echo "Docker is required for the pinned Trivy security scan." >&2
  exit 1
}
mkdir -p "$cache_dir"

proxy_args=()
network_args=()
for proxy_name in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
  if [[ -n "${!proxy_name:-}" ]]; then
    proxy_args+=(--env "$proxy_name")
  fi
done
if [[ -n "${HTTP_PROXY:-}${HTTPS_PROXY:-}${http_proxy:-}${https_proxy:-}" ]]; then
  # Trivy runs as a nested container. Host networking is required for the
  # loopback proxy endpoints commonly supplied by WSL, and --env NAME keeps
  # the values out of command lines and persisted evidence.
  network_args+=(--network host)
fi

case "$mode" in
  source)
    docker run --rm \
      "${network_args[@]}" \
      "${proxy_args[@]}" \
      -v "$repo_root:/src:ro" \
      -v "$cache_dir:/root/.cache/trivy" \
      "$trivy_image" \
      fs \
      --scanners vuln,secret,misconfig \
      --severity HIGH,CRITICAL \
      --exit-code 1 \
      --ignorefile /src/.trivyignore.yaml \
      --skip-files /src/go.sum \
      /src
    ;;
  image)
    image_ref="${2:-}"
    [[ -n "$image_ref" ]] || {
      echo "Usage: $0 image IMAGE_REFERENCE" >&2
      exit 2
    }
    docker image inspect "$image_ref" >/dev/null
    docker run --rm \
      "${network_args[@]}" \
      "${proxy_args[@]}" \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v "$cache_dir:/root/.cache/trivy" \
      "$trivy_image" \
      image \
      --scanners vuln,secret \
      --severity HIGH,CRITICAL \
      --exit-code 1 \
      "$image_ref"
    ;;
  *)
    echo "Unknown scan mode: $mode (expected source or image)" >&2
    exit 2
    ;;
esac
