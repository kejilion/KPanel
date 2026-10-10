#!/usr/bin/env bash
# Run prebuilt, identically instrumented test binaries. Build outside the runtime
# cgroup; this script deliberately never downloads tools or changes host qdiscs.
set -euo pipefail
if [[ $# != 4 ]]; then
  echo "Usage: $0 BASELINE_BINARY CANDIDATE_BINARY OUTPUT_DIR PINNED_IMAGE_ID" >&2
  exit 2
fi
baseline="$(realpath "$1")"
candidate="$(realpath "$2")"
mkdir -p "$3"
output="$(realpath "$3")"
image="$4"
[[ "$image" =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'A pinned Docker image ID is required' >&2; exit 2; }
[[ -x "$baseline" && -x "$candidate" ]] || exit 2
samples="${TRANSFER_SAMPLES:-5}"
[[ "$samples" =~ ^[1-9][0-9]*$ ]] && (( samples <= 30 )) || exit 2
read -r -a cases <<< "${TRANSFER_CASES:-fast source-limited balanced no-range request-delay-20ms request-delay-100ms small-files two-tasks large}"
read -r -a variants <<< "${TRANSFER_VARIANTS:-baseline pipeline segmented}"
for case_name in "${cases[@]}"; do
  case "$case_name" in
    fast|source-limited|slow-source|balanced|no-range|request-delay-20ms|request-delay-100ms|small-files|two-tasks|large) ;;
    *) echo "Unknown benchmark case: $case_name" >&2; exit 2 ;;
  esac
done
[[ ! -e "$output/samples.jsonl" ]] || { echo 'Use a new output directory for each run' >&2; exit 2; }
sha256sum "$baseline" "$candidate" > "$output/binaries.sha256"
uname -a > "$output/host.txt"
docker image inspect "$image" --format '{{.Id}}' > "$output/image.txt"
printf '%s\n' '1 CPU; 256 MiB memory; 256 MiB memory+swap; 128 PIDs; 16 MiB tmpfs; network none; fixtures use loopback; real bind-mounted filesystem; shared warm page cache' > "$output/limits.txt"
printf 'samples=%s cases=%s variants=%s\n' "$samples" "${cases[*]}" "${variants[*]}" > "$output/parameters.txt"
run_one() {
  local case_name="$1" variant="$2" sample="$3" binary mode scratch log name code
  binary="$candidate"; mode=ordinary
  case "$variant" in baseline) binary="$baseline";; baseline-off) binary="$baseline"; mode=off;; candidate-off) mode=off;; adaptive) mode=adaptive;; pipeline) ;; segmented|segmented-2) mode="$variant";; *) exit 2;; esac
  scratch="$(mktemp -d "$output/transfer-scratch.XXXXXX")"
  name="kpanel-transfer-bench-${$}-$RANDOM"
  log="$output/${case_name}-${variant}-${sample}.log"
  set +e
  docker run --name "$name" --network none --read-only --cpus 1 --memory 256m --memory-swap 256m --pids-limit 128 \
    --cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,size=16m \
    --mount "type=bind,src=$binary,dst=/bench,readonly" --mount "type=bind,src=$scratch,dst=/scratch" \
    -e TMPDIR=/scratch -e GOMAXPROCS=1 -e GOMEMLIMIT=192MiB \
    -e KPANEL_TRANSFER_BENCHMARK=1 -e "KPANEL_TRANSFER_CASE=$case_name" -e "KPANEL_TRANSFER_MODE=$mode" \
    -e "KPANEL_TRANSFER_LABEL=$variant" -e "KPANEL_TRANSFER_SAMPLE=$sample" \
    "$image" /bench -test.run '^TestFileTransferPerformance$' -test.v -test.timeout 180s > "$log" 2>&1
  code=$?
  set -e
  docker inspect "$name" --format '{{json .State}}' > "$log.state.json"
  docker rm "$name" >/dev/null
  # Only remove the exact task-owned scratch created above, inside this output.
  [[ "$scratch" == "$output"/transfer-scratch.* ]] && rm -rf -- "$scratch"
  if (( code != 0 )); then cat "$log"; return "$code"; fi
  local -a rows=()
  mapfile -t rows < <(sed -n 's/^TRANSFER_BENCH //p' "$log")
  if (( ${#rows[@]} != 1 )); then echo "Missing or ambiguous measurement: $log" >&2; return 1; fi
  if [[ "$sample" != warmup ]]; then
    printf '%s\n' "${rows[0]}" >> "$output/samples.jsonl"
  fi
  printf '%s %s %s complete\n' "$case_name" "$variant" "$sample"
}
for case_name in "${cases[@]}"; do
  for variant in "${variants[@]}"; do run_one "$case_name" "$variant" warmup; done
  for ((sample=1; sample<=samples; sample++)); do
    # Rotate order to reduce temporal/cache drift rather than run all A then B.
    for ((index=0; index<${#variants[@]}; index++)); do
      run_one "$case_name" "${variants[$(((sample+index-1)%${#variants[@]}))]}" "$sample"
    done
  done
done
