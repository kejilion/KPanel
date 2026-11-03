#!/usr/bin/env bash
set -euo pipefail

# Destructive lifecycle tests belong only on disposable, canonical hosted runners.
[[ "${GITHUB_ACTIONS:-}" = true && "${RUNNER_ENVIRONMENT:-}" = github-hosted &&
   "${GITHUB_REPOSITORY:-}" = kejilion/KPanel && "$(id -u)" = 0 ]] || exit 2
[[ "${GITHUB_RUN_ID:-}" =~ ^[0-9]+$ && "${GITHUB_RUN_ATTEMPT:-}" =~ ^[0-9]+$ &&
   "${GITHUB_SHA:-}" =~ ^[a-f0-9]{40}$ && "${RUNNER_TEMP:-}" = /* ]] || exit 2
[[ "$(cat /proc/1/comm)" = systemd ]] || exit 2

stable=docker.io/kjlion/kejilion-panel@sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4
preview=docker.io/kjlion/kejilion-panel@sha256:1e936f24e2ebf65e827f14d814759e8b500fcbacf4c437bb19c4e1a559eb7696
work="$RUNNER_TEMP/kpanel-native-host-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
app=/home/docker/kpanel

# Separate processes source the unmodified, image-extracted official lifecycle.
if [[ "${1:-}" = --invoke ]]; then
  [[ $# = 3 ]] || exit 2
  action="$2"; lifecycle="$3"
  case "$lifecycle" in "$work/stable.conf"|"$work/preview.conf"|"$app/bin/kpanel.conf") ;; *) exit 2 ;; esac
  invoke() {
    docker_app_plus() { :; } # Menu registration is unrelated to the lifecycle.
    check_docker_app_ip() { :; } # Avoid printing access credentials to the job log.
    local ipv4_address=127.0.0.1
    source "$lifecycle"
    docker_port=18080
    case "$action" in
      install) docker_app_install ;;
      update) KJ_KPANEL_AUTOMATIC=1 KJ_KPANEL_TARGET_VERSION=1.29.0-rc.7 KJ_KPANEL_TARGET_IMAGE="$preview" docker_app_update ;;
      recover) KJ_KPANEL_FORCE_ROLLBACK=1 kpanel_recover_automatic_update ;;
      uninstall) docker_app_uninstall ;;
      *) return 2 ;;
    esac
  }
  invoke
  exit
fi
[[ $# = 0 && "$(git rev-parse HEAD)" = "$GITHUB_SHA" ]] || exit 2
for path in "$app" /root/kejilion.sh /usr/local/bin/k /usr/bin/k /etc/systemd/system/kejilion-agent.service; do
  [[ ! -e "$path" && ! -L "$path" ]] || { echo "Refusing existing host state: $path" >&2; exit 2; }
done
[[ -z "$(docker ps -aq --filter name='^/kejilion-panel$')" ]] || exit 2
[[ ! -e "$work" ]] || exit 2
mkdir -m 711 "$work"; mkdir -m 755 "$work/evidence"
self="$(realpath "$0")"
checks=0; update_pid=''; owned=false
qualified_parents=(); original_parent_modes=()
finish() {
  local result=$? cleanup=passed
  trap - EXIT TERM INT
  if [[ -n "$update_pid" ]]; then kill -KILL -- "-$update_pid" 2>/dev/null || true; wait "$update_pid" 2>/dev/null || true; fi
  systemctl status kejilion-agent --no-pager >"$work/evidence/service-final.log" 2>&1 || true
  stat -c '%n %u:%g %a %h %F' / /home /home/docker "$app" "$app/bin" \
    "$app/.managed-by-kejilion-app" "$app/bin/kpanel.conf" "$app/bin/kejilion.sh" \
    /root /usr /usr/bin /usr/local /usr/local/bin \
    /run /run/lock /run/lock/kejilion-kpanel-lifecycle.lock \
    >"$work/evidence/migration-path-metadata.log" 2>&1 || true
  journalctl -u kejilion-agent -n 100 --no-pager >"$work/evidence/agent-journal.log" 2>&1 || true
  if [[ "$owned" = true && -d "$app" ]]; then
    bash "$self" --invoke uninstall "$work/preview.conf" >"$work/evidence/cleanup.log" 2>&1 || cleanup=failed
    [[ ! -e "$app" && -z "$(docker ps -aq --filter name='^/kejilion-panel$')" ]] || cleanup=failed
  fi
  for ((index=0;index<${#qualified_parents[@]};index++)); do
    chmod "${original_parent_modes[index]}" "${qualified_parents[index]}" || cleanup=failed
  done
  [[ "$cleanup" = passed ]] || result=1
  printf '{"candidate":"%s","runId":"%s","attempt":"%s","exitCode":%s,"checks":%s,"cleanup":"%s","mocked":false,"productVersion":"1.29.0-rc.7","stableDigest":"%s","productDigest":"%s"}\n' \
    "$GITHUB_SHA" "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT" "$result" "$checks" "$cleanup" "${stable##*@}" "${preview##*@}" >"$work/evidence/result.json"
  chmod -R a+rX "$work/evidence"
  exit "$result"
}
trap finish EXIT
trap 'exit 124' TERM INT
pass() { checks=$((checks + 1)); printf '%s\n' "$1=pass" >>"$work/evidence/checks.log"; }

docker version >"$work/evidence/docker-version.log"
systemd --version >"$work/evidence/systemd-version.log"
umask >"$work/evidence/caller-umask.log"
uname -a >"$work/evidence/kernel.log"
# Hosted images can provide group-writable installation/tool directories.
# Qualify every fixed ancestor required by the unchanged migration contract;
# never change ownership, traverse a link or broaden the production guard.
[[ -e /home/docker || -L /home/docker ]] || mkdir -m 755 /home/docker
for parent in / /root /usr /usr/bin /usr/local /usr/local/bin /home /home/docker; do
  [[ -d "$parent" && ! -L "$parent" && "$(stat -c '%u' "$parent")" = 0 ]]
  stat -c '%n %u:%g %a %F' "$parent" >>"$work/evidence/parent-before.log"
  qualified_parents+=("$parent")
  original_parent_modes+=("$(stat -c '%a' "$parent")")
  chmod go-w "$parent"
  stat -c '%n %u:%g %a %F' "$parent" >>"$work/evidence/parent-qualified.log"
done
for channel in stable preview; do
  image="$stable"; [[ "$channel" = stable ]] || image="$preview"
  docker pull "$image" >"$work/evidence/pull-$channel.log" 2>&1
  container="$(docker create "$image")"
  docker cp "$container:/release/kpanel.conf" "$work/$channel.conf"
  docker cp "$container:/release/kejilion.sh" "$work/$channel.sh"
  docker rm "$container" >/dev/null
done
cmp "$work/preview.conf" packaging/kejilion-app/kpanel.conf
git diff --exit-code v1.29.0-rc.7 -- packaging/kejilion-app/kpanel.conf cmd/kejilion-agent internal/selfupdate >"$work/evidence/product-source-diff.log"
pass exact_published_lifecycle_and_migration_source
docker pull docker.io/kjlion/kejilion-panel:latest >"$work/evidence/pull-latest.log" 2>&1
[[ "$(docker image inspect --format '{{.Id}}' docker.io/kjlion/kejilion-panel:latest)" = "$(docker image inspect --format '{{.Id}}' "$stable")" ]]
pass unchanged_stable_install_source
cp "$work/stable.sh" /root/kejilion.sh
sed -i 's/^permission_granted="false"$/permission_granted="true"/;s/^ENABLE_STATS="true"$/ENABLE_STATS="false"/;s/^canshu="default"$/canshu="CN"/' /root/kejilion.sh
chmod 700 /root/kejilion.sh
owned=true
bash "$self" --invoke install "$work/stable.conf" >"$work/evidence/install.log" 2>&1
systemctl is-active --quiet kejilion-agent
[[ "$("$app/bin/kejilion-agent" version | awk '{print $1}')" = 1.28.0 ]]
pass official_stable_install_and_formal_systemd_unit
systemctl stop kejilion-panel-update.timer
printf 'panel-native-data\n' >"$app/data/panel/native-test-marker"
chown 65532:65532 "$app/data/panel/native-test-marker"
printf 'agent-native-data\n' >"$app/data/agent/native-test-marker"
sha256sum /root/kejilion.sh "$app/bin/kejilion.sh" "$app/data/panel/native-test-marker" "$app/data/agent/native-test-marker" >"$work/before.sha256"

# Kill the actual old updater after it swaps binaries but before transaction commit.
# Real systemd, Docker, filesystem and recovery code remain unmodified.
setsid bash "$self" --invoke update "$work/stable.conf" >"$work/evidence/interrupted-upgrade.log" 2>&1 &
update_pid=$!
caught=false
for ((attempt=0;attempt<2400;attempt++)); do
  if [[ "$(cat "$app/update-state/transaction/phase" 2>/dev/null || true)" = verifying ]]; then
    kill -KILL -- "-$update_pid"
    wait "$update_pid" 2>/dev/null || true
    update_pid=''; caught=true; break
  fi
  kill -0 "$update_pid" 2>/dev/null || break
  sleep 0.05
done
[[ "$caught" = true ]]
pass interrupted_real_update_before_commit
# Alter only test-owned marker files while both real writers are stopped. A
# checksum match after recovery must come from the cold backup, not retention.
systemctl stop kejilion-agent
docker stop kejilion-panel >"$work/evidence/stop-interrupted-panel.log"
printf 'damaged-after-interruption\n' >"$app/data/panel/native-test-marker"
printf 'damaged-after-interruption\n' >"$app/data/agent/native-test-marker"
bash "$self" --invoke recover "$work/stable.conf" >"$work/evidence/recovery.log" 2>&1
[[ ! -e "$app/update-state/transaction" ]]
sha256sum -c "$work/before.sha256" >"$work/evidence/restored-data-and-script.log"
systemctl is-active --quiet kejilion-agent
[[ "$("$app/bin/kejilion-agent" version | awk '{print $1}')" = 1.28.0 ]]
pass cold_backup_script_and_service_recovery
bash "$self" --invoke update "$work/stable.conf" >"$work/evidence/upgrade.log" 2>&1
for ((attempt=0;attempt<120;attempt++)); do
  if [[ -e /usr/local/bin/k ]] && cmp -s /root/kejilion.sh /usr/local/bin/k && grep -Fx 'canshu="CN"' /usr/local/bin/k >/dev/null; then break; fi
  sleep 1
done
[[ ! -e "$app/update-state/transaction" && ! -e "$app/run/update-freeze" ]]
[[ "$("$app/bin/kejilion-agent" version | awk '{print $1}')" = 1.29.0-rc.7 ]]
cmp /root/kejilion.sh /usr/local/bin/k
[[ "$(readlink /usr/bin/k)" = /usr/local/bin/k ]]
grep -Fx 'canshu="CN"' /usr/local/bin/k >/dev/null
grep -Fx 'ENABLE_STATS="false"' /usr/local/bin/k >/dev/null
pass committed_upgrade_and_shared_script_preferences
systemctl is-active --quiet kejilion-agent
docker exec kejilion-panel /paneld agent-healthcheck
[[ "$(docker inspect --format '{{.Image}}' kejilion-panel)" = "$(docker image inspect --format '{{.Id}}' "$preview")" ]]
grep -Fx 'panel-native-data' "$app/data/panel/native-test-marker" >/dev/null
grep -Fx 'agent-native-data' "$app/data/agent/native-test-marker" >/dev/null
pass final_real_panel_agent_and_retained_data
printf '%s\n' native_host_upgrade=pass
