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
    # Keep every upstream source file, including manifests and development recipes,
    # in the secret scan. Scanner-type routing below must never hide secrets.
    docker run --rm \
      "${network_args[@]}" \
      "${proxy_args[@]}" \
      -v "$repo_root:/src:ro" \
      -v "$cache_dir:/root/.cache/trivy" \
      "$trivy_image" \
      fs \
      --scanners secret \
      --severity HIGH,CRITICAL \
      --exit-code 1 \
      /src
    docker run --rm \
      "${network_args[@]}" \
      "${proxy_args[@]}" \
      -v "$repo_root:/src:ro" \
      -v "$cache_dir:/root/.cache/trivy" \
      "$trivy_image" \
      fs \
      --scanners vuln,misconfig \
      --severity HIGH,CRITICAL \
      --exit-code 1 \
      --ignorefile /src/.trivyignore.yaml \
      --skip-files /src/go.sum \
      --skip-files /src/third_party/torrent/go.mod \
      --skip-files /src/third_party/torrent/go.sum \
      --skip-files /src/third_party/torrent/Dockerfile \
      /src
    # Check the exact upstream module, production closure and selected graph.
    # The release Runner and host Docker daemon share this checkout at the same path.
    # Runner-private /tmp cannot be used as a nested Docker bind source.
    baseline_root="$repo_root/.codex-tmp"
    if [[ -L "$baseline_root" || ( -e "$baseline_root" && ! -d "$baseline_root" ) ]]; then
      echo 'Refusing an unsafe baseline staging directory.' >&2
      exit 1
    fi
    [[ -d "$baseline_root" ]] || mkdir -m 700 -- "$baseline_root"
    baseline_dir="$(mktemp -d "$baseline_root/torrent-baseline.XXXXXXXX")"
    trap 'rm -rf -- "$baseline_dir"' EXIT
    node "$repo_root/scripts/torrent-backport-baseline.mjs" scan-inventories "$baseline_dir"
    for scope in production selected standalone; do
      scanners=vuln
      [[ "$scope" != standalone ]] || scanners=vuln,misconfig
      report="$baseline_dir/$scope.report.json"
      stderr="$baseline_dir/$scope.stderr.log"
      # Capture the real nonzero status for complete finding reports. The checker
      # rejects operational failures and every finding outside the bounded review.
      if docker run --rm \
        "${network_args[@]}" \
        "${proxy_args[@]}" \
        -v "$baseline_dir/$scope:/inventory:ro" \
        -v "$cache_dir:/root/.cache/trivy" \
        "$trivy_image" \
        fs \
        --scanners "$scanners" \
        --severity HIGH,CRITICAL \
        --exit-code 1 \
        --format json \
        --list-all-pkgs \
        /inventory >"$report" 2>"$stderr"; then
        scanner_status=0
      else
        scanner_status=$?
      fi
      # Canonical stdout/stderr evidence receives originals before the private
      # staging trap runs, including on a later validation failure.
      printf '\ntrivy_raw_begin scope=%s stream=stdout exit=%s\n' "$scope" "$scanner_status"
      cat -- "$report"
      printf '\ntrivy_raw_end scope=%s stream=stdout\n' "$scope"
      printf '\ntrivy_raw_begin scope=%s stream=stderr exit=%s\n' "$scope" "$scanner_status" >&2
      cat -- "$stderr" >&2
      printf '\ntrivy_raw_end scope=%s stream=stderr\n' "$scope" >&2
      node "$repo_root/scripts/torrent-backport-baseline.mjs" check-report "$baseline_dir" "$scope" "$report" "$scanner_status" "$stderr"
    done
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
