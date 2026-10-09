#!/usr/bin/env bash
# Compare the same BT/Panel/Agent binary with adaptation disabled and enabled.
# HTTP comparison uses file-transfer-benchmark.sh with baseline-off/adaptive.
set -euo pipefail
if [[ $# != 3 ]]; then
  echo "Usage: $0 PANEL_TEST_BINARY NEW_OUTPUT_DIR PINNED_IMAGE_ID" >&2
  exit 2
fi
binary="$(realpath "$1")"
mkdir -p "$2"
output="$(realpath "$2")"
image="$3"
[[ -x "$binary" && "$image" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
[[ ! -e "$output/samples.jsonl" ]] || exit 2
samples="${BT_SAMPLES:-3}"
[[ "$samples" =~ ^[1-9][0-9]*$ ]] && (( samples <= 10 )) || exit 2
read -r -a cases <<< "${BT_CASES:-swarm scarce}"
for name in "${cases[@]}"; do [[ "$name" == swarm || "$name" == scarce ]] || exit 2; done
sha256sum "$binary" > "$output/binary.sha256"
printf '%s\n' '1 CPU; 256 MiB memory and memory+swap; 128 PIDs; 16 MiB tmpfs; network none; combined seeders/Panel/Agent; real filesystem' > "$output/limits.txt"
printf 'samples=%s cases=%s\n' "$samples" "${cases[*]}" > "$output/parameters.txt"
run_one() {
  local scenario="$1" mode="$2" sample="$3" scratch name log code
  scratch="$(mktemp -d "$output/bt-scratch.XXXXXX")"
  name="kpanel-bt-bench-${$}-$RANDOM"
  log="$output/${scenario}-${mode}-${sample}.log"
  set +e
  docker run --name "$name" --network none --read-only --cpus 1 --memory 256m --memory-swap 256m --pids-limit 128 \
    --cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,size=16m \
    --mount "type=bind,src=$binary,dst=/bench,readonly" --mount "type=bind,src=$scratch,dst=/scratch" \
    -e TMPDIR=/scratch -e GOMAXPROCS=1 -e GOMEMLIMIT=192MiB \
    -e KPANEL_BT_BENCHMARK=1 -e "KPANEL_BT_CASE=$scenario" -e "KPANEL_BT_MODE=$mode" \
    "$image" /bench -test.run '^TestTorrentTransferPerformance$' -test.v -test.timeout 180s > "$log" 2>&1
  code=$?
  set -e
  docker inspect "$name" --format '{{json .State}}' > "$log.state.json"
  docker rm "$name" >/dev/null
  [[ "$scratch" == "$output"/bt-scratch.* ]] && rm -rf -- "$scratch"
  if (( code != 0 )); then cat "$log"; return "$code"; fi
  local -a rows=()
  mapfile -t rows < <(sed -n 's/^BT_BENCH //p' "$log")
  (( ${#rows[@]} == 1 )) || { echo "Missing benchmark result: $log" >&2; return 1; }
  if [[ "$sample" != warmup ]]; then printf '%s\n' "${rows[0]}" >> "$output/samples.jsonl"; fi
  printf '%s %s %s complete\n' "$scenario" "$mode" "$sample"
}
for scenario in "${cases[@]}"; do
  run_one "$scenario" off warmup
  run_one "$scenario" auto warmup
  for ((sample=1;sample<=samples;sample++)); do
    if (( sample % 2 )); then modes=(off auto); else modes=(auto off); fi
    for mode in "${modes[@]}"; do run_one "$scenario" "$mode" "$sample"; done
  done
done
