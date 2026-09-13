#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

usage() {
  echo "usage: $0 /root/kpanel-production-inbox/RUN_ID-PHASE/plan.env" >&2
  exit 2
}

[ "$#" -eq 1 ] || usage
plan=$1
[[ "$plan" =~ ^/root/kpanel-production-inbox/[A-Za-z0-9][A-Za-z0-9._-]{2,80}-(preflight|backup|postdeploy)/plan\.env$ ]] || usage
inbox=$(dirname "$plan")

SCHEMA_VERSION=
PHASE=
RUN_ID=
EXPECTED_VERSION=
EXPECTED_REVISION=
EXPECTED_IMAGE_DIGEST=
BASELINE_RUN_ID=
REMOTE_SCRIPT_SHA256=
declare -A seen_plan_keys=()

while IFS='=' read -r key value; do
  [ -n "$key" ] || continue
  case "$key" in
    SCHEMA_VERSION|PHASE|RUN_ID|EXPECTED_VERSION|EXPECTED_REVISION|EXPECTED_IMAGE_DIGEST|BASELINE_RUN_ID|REMOTE_SCRIPT_SHA256)
      [ -z "${seen_plan_keys[$key]+present}" ] || {
        echo "duplicate production plan key: $key" >&2
        exit 2
      }
      seen_plan_keys[$key]=1
      printf -v "$key" '%s' "$value"
      ;;
    *)
      echo "unknown production plan key: $key" >&2
      exit 2
      ;;
  esac
done < "$plan"

[ "$SCHEMA_VERSION" = 1 ] || usage
[[ "$PHASE" =~ ^(preflight|backup|postdeploy)$ ]] || usage
[[ "$RUN_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$ ]] || usage
[[ "$EXPECTED_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
[[ "$REMOTE_SCRIPT_SHA256" =~ ^[0-9a-f]{64}$ ]] || usage
if [ "$PHASE" = postdeploy ]; then
  [[ "$EXPECTED_REVISION" =~ ^[0-9a-fA-F]{40,64}$ ]] || usage
  [[ "$EXPECTED_IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || usage
  [[ "$BASELINE_RUN_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$ ]] || usage
elif [ "$PHASE" = backup ]; then
  [ "$EXPECTED_REVISION" = - ] && [ "$EXPECTED_IMAGE_DIGEST" = - ] || usage
  [[ "$BASELINE_RUN_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{2,80}$ ]] || usage
else
  [ "$EXPECTED_REVISION" = - ] && [ "$EXPECTED_IMAGE_DIGEST" = - ] && [ "$BASELINE_RUN_ID" = - ] || usage
fi

[ "$(uname -s)" = Linux ] || { echo "production evidence entrypoint requires Linux" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || { echo "production evidence entrypoint requires root" >&2; exit 1; }
for command in awk bash curl date df diff docker findmnt grep journalctl python3 realpath seq sha256sum sort systemctl tar xargs zstd; do
  command -v "$command" >/dev/null || { echo "required command is missing: $command" >&2; exit 1; }
done
[ -S /var/run/docker.sock ] || { echo "Docker socket is required" >&2; exit 1; }

script_path="$inbox/run-production-evidence-remote.sh"
[ -f "$script_path" ] && [ -f "$inbox/manifest.json" ] || { echo "production evidence inputs are incomplete" >&2; exit 1; }
[ "$(sha256sum "$script_path" | awk '{print $1}')" = "$REMOTE_SCRIPT_SHA256" ] || {
  echo "production evidence entrypoint checksum mismatch" >&2
  exit 1
}

evidence_root="/root/kpanel-release-evidence/$RUN_ID"
evidence="$evidence_root/production-$PHASE"
[ "$(realpath -m "$evidence_root")" = "/root/kpanel-release-evidence/$RUN_ID" ] || exit 2
[ "$(realpath -m "$evidence")" = "/root/kpanel-release-evidence/$RUN_ID/production-$PHASE" ] || exit 2
[ ! -e "$evidence" ] || { echo "production evidence phase already exists; use a new run ID" >&2; exit 1; }
mkdir -p -m 700 "$evidence"

status=failed
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
finish() {
  rc=$?
  finished_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  {
    echo "run_id=$RUN_ID"
    echo "phase=$PHASE"
    echo "expected_version=$EXPECTED_VERSION"
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

cp "$plan" "$script_path" "$inbox/manifest.json" "$evidence/"
sha256sum "$evidence/plan.env" "$evidence/run-production-evidence-remote.sh" "$evidence/manifest.json" > "$evidence/input.sha256"

compose=/home/docker/kpanel/docker-compose.yml
env_file=/home/docker/kpanel/.env
data_dir=/home/docker/kpanel/data
service_unit=/etc/systemd/system/kejilion-agent.service
app_root=/home/docker/kpanel
container=kejilion-panel

snapshot() {
  local output_dir=$1
  install -d -m 700 "$output_dir"
  local health_ready=false
  local health_error="$output_dir/health.error"
  for _ in $(seq 1 10); do
    if curl -fsS http://127.0.0.1:8080/api/v1/health > "$output_dir/health.json" 2> "$health_error"; then
      health_ready=true
      break
    fi
    sleep 1
  done
  if [ "$health_ready" != true ]; then
    cat "$health_error" >&2
    return 1
  fi
  rm -f "$health_error"
  python3 - "$output_dir/health.json" <<'PY'
import json, pathlib, sys
p = pathlib.Path(sys.argv[1])
value = json.loads(p.read_text(encoding="utf-8"))
assert isinstance(value, dict)
assert value.get("status") == "ok"
assert isinstance(value.get("version"), str) and value["version"]
PY
  docker inspect "$container" > "$output_dir/container-inspect.json"
  docker image inspect "$(docker inspect "$container" --format '{{.Image}}')" > "$output_dir/image-inspect.json"
  systemctl show kejilion-agent \
    --property=LoadState,ActiveState,SubState,UnitFileState,NeedDaemonReload > "$output_dir/agent-state.txt"
  systemctl is-active --quiet kejilion-agent
  [ "$(docker inspect "$container" --format '{{.State.Status}}')" = running ]
  [ "$(docker inspect "$container" --format '{{.State.Health.Status}}')" = healthy ]
  [ "$(docker inspect "$container" --format '{{.RestartCount}}')" = 0 ]
  [ "$(docker inspect "$container" --format '{{.State.OOMKilled}}')" = false ]
  findmnt -T "$app_root" > "$output_dir/findmnt.txt"
  df -h "$app_root" > "$output_dir/df.txt"
  docker stats --no-stream --format '{{json .}}' "$container" > "$output_dir/resources.json"
  sha256sum "$compose" "$env_file" /home/docker/kpanel/agent.env > "$output_dir/protected.sha256"
  find "$data_dir" -xdev -type f -printf '%P\t%s\n' | LC_ALL=C sort > "$output_dir/data-inventory.tsv"
  python3 - "$data_dir" > "$output_dir/sqlite-check.txt" <<'PY'
import pathlib, sqlite3, sys
root = pathlib.Path(sys.argv[1])
for path in sorted(root.rglob("*.db")):
    if path.stat().st_size == 0:
        print(f"{path.relative_to(root)}\tempty")
        continue
    with sqlite3.connect(f"file:{path}?mode=ro", uri=True) as conn:
        result = conn.execute("pragma quick_check").fetchone()[0]
    if result != "ok":
        raise SystemExit(f"sqlite quick_check failed: {path}: {result}")
    print(f"{path.relative_to(root)}\tok")
PY
}

production_ready() {
  curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null 2>&1 &&
    systemctl is-active --quiet kejilion-agent &&
    [ "$(docker inspect "$container" --format '{{.State.Status}}')" = running ] &&
    [ "$(docker inspect "$container" --format '{{.State.Health.Status}}')" = healthy ]
}

case "$PHASE" in
  preflight)
    for path in "$compose" "$env_file" "$data_dir" "$service_unit" /home/docker/kpanel/bin/kejilion.sh; do
      [ -e "$path" ] || { echo "required production path is missing: $path" >&2; exit 1; }
    done
    snapshot "$evidence/snapshot"
    [ "$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$evidence/snapshot/health.json")" = "$EXPECTED_VERSION" ]
    ;;
  backup)
    baseline="/root/kpanel-release-evidence/$BASELINE_RUN_ID/production-preflight/snapshot"
    [ -f "$baseline/protected.sha256" ] || { echo "baseline preflight evidence is missing" >&2; exit 1; }
    available_kib=$(df -Pk /root/kpanel-backups | awk 'NR == 2 {print $4}')
    [ "$available_kib" -ge 1048576 ] || { echo "less than 1 GiB is available for the production backup" >&2; exit 1; }
    backup="/root/kpanel-backups/pre-v${EXPECTED_VERSION}-$(date -u +%Y%m%dT%H%M%SZ)"
    [ "$(realpath -m "$backup")" = "/root/kpanel-backups/$(basename "$backup")" ] || exit 2
    [ ! -e "$backup" ] || { echo "backup path already exists" >&2; exit 1; }
    install -d -m 700 "$backup"
    docker inspect "$container" > "$backup/panel-inspect.json"
    image_id=$(docker inspect "$container" --format '{{.Image}}')
    restore_services() {
      docker compose -f "$compose" up -d >/dev/null 2>&1 || true
      systemctl start kejilion-agent >/dev/null 2>&1 || true
    }
    trap 'restore_services; finish' EXIT
    systemctl stop kejilion-agent
    docker compose -f "$compose" stop
    tar --zstd -cpf "$backup/kpanel.tar.zst" \
      --exclude='./run' --exclude='./backups' -C "$app_root" .
    docker image save "$image_id" | zstd -T0 -q -o "$backup/old-image.tar.zst"
    cp "$service_unit" "$backup/kejilion-agent.service"
    if [ -f /root/apps/kpanel.conf ]; then cp /root/apps/kpanel.conf "$backup/kpanel.conf"; fi
    tar --zstd -tf "$backup/kpanel.tar.zst" >/dev/null
    zstd -dc "$backup/old-image.tar.zst" | tar -tf - >/dev/null
    zstd -dc "$backup/old-image.tar.zst" | docker image load > "$backup/image-load-verify.txt"
    find "$backup" -maxdepth 1 -type f ! -name SHA256SUMS -print0 |
      LC_ALL=C sort -z | xargs -0 sha256sum > "$backup/SHA256SUMS"
    (cd "$backup" && sha256sum -c SHA256SUMS)
    restore_services
    trap finish EXIT
    for _ in $(seq 1 30); do
      if production_ready; then break; fi
      sleep 1
    done
    snapshot "$evidence/snapshot"
    diff -u "$baseline/protected.sha256" "$evidence/snapshot/protected.sha256" > "$evidence/protected.diff"
    baseline_version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$baseline/health.json")
    restored_version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$evidence/snapshot/health.json")
    [ "$restored_version" = "$baseline_version" ]
    printf '%s\n' "$backup" > "$evidence/backup-path.txt"
    cp "$backup/SHA256SUMS" "$evidence/backup-SHA256SUMS"
    ;;
  postdeploy)
    baseline="/root/kpanel-release-evidence/$BASELINE_RUN_ID/production-preflight/snapshot"
    [ -f "$baseline/protected.sha256" ] || { echo "baseline preflight evidence is missing" >&2; exit 1; }
    snapshot "$evidence/snapshot"
    actual_version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$evidence/snapshot/health.json")
    [ "$actual_version" = "$EXPECTED_VERSION" ]
    actual_revision=$(docker inspect "$container" --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}')
    actual_image_version=$(docker inspect "$container" --format '{{ index .Config.Labels "org.opencontainers.image.version" }}')
    [ "$actual_revision" = "$EXPECTED_REVISION" ]
    [ "$actual_image_version" = "$EXPECTED_VERSION" ]
    docker image inspect "$(docker inspect "$container" --format '{{.Image}}')" --format '{{json .RepoDigests}}' |
      grep -Fq "@$EXPECTED_IMAGE_DIGEST"
    diff -u "$baseline/protected.sha256" "$evidence/snapshot/protected.sha256" > "$evidence/protected.diff"
    journalctl -u kejilion-agent --since '-10 minutes' --no-pager > "$evidence/agent-journal.txt"
    docker logs --since 10m "$container" > "$evidence/panel.log" 2>&1
    if grep -Eiq 'panic|fatal|out of memory|oom killed' "$evidence/agent-journal.txt" "$evidence/panel.log"; then
      echo "fatal production log signature detected" >&2
      exit 1
    fi
    ;;
esac

status=passed
echo "production_evidence_gate=pass run_id=$RUN_ID phase=$PHASE version=$EXPECTED_VERSION"
