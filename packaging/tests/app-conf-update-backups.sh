#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2329
set -eu

# This harness refuses to run outside a disposable rootfs and supplies Docker/init mocks.
export KPANEL_APP_CONF_TEST_LIBRARY=1
# shellcheck source=packaging/tests/app-conf-lifecycle.sh
. "${1:-/src}/packaging/tests/app-conf-lifecycle.sh" "${1:-/src}"

real_tar=$(command -v tar)
export KPANEL_REAL_TAR=$real_tar
cat >"$FAKE_BIN/tar" <<'EOF'
#!/bin/sh
case "$*" in
  *-cpf*)
    test -f "$KPANEL_MOCK_STATE/agent-stopped" && test -f "$KPANEL_MOCK_STATE/panel-stopped" || exit 93
    [ "${KPANEL_MOCK_TAR_FAIL:-0}" != 1 ] || exit 1
    ;;
esac
exec "$KPANEL_REAL_TAR" "$@"
EOF
real_df=$(command -v df)
export KPANEL_REAL_DF=$real_df
cat >"$FAKE_BIN/df" <<'EOF'
#!/bin/sh
if [ "${KPANEL_MOCK_SPACE_FAIL:-0}" = 1 ]; then
  printf 'Filesystem 1024-blocks Used Available Capacity Mounted\nmock 100 99 1 99%% /\n'
else
  exec "$KPANEL_REAL_DF" "$@"
fi
EOF
chmod 755 "$FAKE_BIN/tar" "$FAKE_BIN/df"
printf '#!/usr/bin/env bash\npermission_granted="true"\ncanshu="default"\nENABLE_STATS="false"\n' >/root/kejilion.sh
chmod 700 /root/kejilion.sh

run_backup_parity() {
  local init=$1 ipv4_address=198.51.100.25
  local failure old_image='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
  local archive transaction_id
  docker_app_plus() { :; }
  check_docker_app_ip() { :; }
  # shellcheck source=/dev/null
  . "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
  docker_port=18080
  docker_app_install >/dev/null

  if [ "$init" = openrc ]; then
    mkdir -p /run/openrc /etc/init.d
    rm -rf /run/systemd/system
    cat >/sbin/rc-service <<'EOF'
#!/bin/sh
case "$2" in
  stop)
    [ "${KPANEL_MOCK_AGENT_STOP_FAIL:-0}" != 1 ] || exit 1
    : >"$KPANEL_MOCK_STATE/agent-stopped" ;;
  start) rm -f "$KPANEL_MOCK_STATE/agent-stopped" ;;
esac
EOF
    for tool in rc-update start-stop-daemon supervise-daemon; do
      printf '#!/bin/sh\nexit 0\n' >"/sbin/$tool"
      chmod 755 "/sbin/$tool"
    done
    chmod 755 /sbin/rc-service
    kpanel_detect_init_system
    kpanel_write_agent_service
    kpanel_agent_service_link
    rm -f /home/docker/kpanel/kejilion-panel-update.service /home/docker/kpanel/kejilion-panel-update.timer
  fi
  export KPANEL_MOCK_REAL_IMAGE_IDS=1
  printf '%s\n' "$old_image" >"$MOCK_STATE/running-id"
  mkdir -p /home/docker/kpanel/data/panel/cluster-light-secrets /home/docker/kpanel/data/agent/nested
  printf 'node-secret\n' >/home/docker/kpanel/data/panel/cluster-light-secrets/host-one.lightkey
  chmod 600 /home/docker/kpanel/data/panel/cluster-light-secrets/host-one.lightkey
  printf 'sqlite-wal-bytes\n' >/home/docker/kpanel/data/agent/nested/store.db-wal
  printf 'original-panel-data\n' >/home/docker/kpanel/data/panel/rollback-marker
  printf 'original-agent-data\n' >/home/docker/kpanel/data/agent/rollback-marker
  cp -p /home/docker/kpanel/.env "$TEST_DIR/original-env"
  cp -p /home/docker/kpanel/kejilion-agent.service "$TEST_DIR/original-service"
  [ "$init" != openrc ] || cp -p /home/docker/kpanel/kejilion-agent.openrc "$TEST_DIR/original-openrc"
  # Older installed/target helpers cannot parse the new manual transaction.
  # A fresh process must keep using the executing lifecycle until recovery ends.
  printf 'local app_id="kpanel"\nkpanel_recover_automatic_update() { return 89; }\n' >"$TEST_DIR/legacy-lifecycle"
  cp "$TEST_DIR/legacy-lifecycle" /home/docker/kpanel/bin/kpanel.conf
  chmod 600 /home/docker/kpanel/bin/kpanel.conf
  export KPANEL_MOCK_LIFECYCLE_SOURCE="$TEST_DIR/legacy-lifecycle"
  recover_fresh() {
    bash -c 'set -eu; docker_app_plus() { :; }; entry() { . /home/docker/kpanel/bin/kpanel.conf; kpanel_recover_automatic_update; }; entry'
  }

  for failure in SPACE TAR CHECKSUM AGENT_STOP PANEL_STOP; do
    export "KPANEL_MOCK_${failure}_FAIL=1"
    if docker_app_update >"$TEST_DIR/$init-$failure.log" 2>&1; then
      echo "update accepted $init $failure failure" >&2; return 1
    fi
    unset "KPANEL_MOCK_${failure}_FAIL"
    # Stop failures remain attached until a safe retry can stop both writers.
    if [ -d /home/docker/kpanel/update-state/transaction ]; then
      recover_fresh >/dev/null
    fi
    test ! -e /home/docker/kpanel/update-state/transaction
    test ! -e /home/docker/kpanel/run/update-freeze
    test "$(cat "$MOCK_STATE/latest-id")" = "$old_image"
    test "$(cat "$MOCK_STATE/running-id")" = "$old_image"
    cmp "$TEST_DIR/original-env" /home/docker/kpanel/.env
    grep -Fx original-panel-data /home/docker/kpanel/data/panel/rollback-marker
  done

  rm -f "$MOCK_STATE/automatic-data-mutated" "$MOCK_STATE/automatic-target-started"
  if KPANEL_MOCK_UPDATE_HEALTH_FAIL=1 KPANEL_MOCK_MUTATE_DATA_ON_UP=1 docker_app_update >"$TEST_DIR/$init-health.log" 2>&1; then
    echo 'unhealthy manual update succeeded' >&2; return 1
  fi
  test -f "$MOCK_STATE/automatic-data-mutated"
  test ! -e /home/docker/kpanel/run/update-freeze
  grep -Fx original-panel-data /home/docker/kpanel/data/panel/rollback-marker
  grep -Fx original-agent-data /home/docker/kpanel/data/agent/rollback-marker
  grep -Fx node-secret /home/docker/kpanel/data/panel/cluster-light-secrets/host-one.lightkey
  grep -Fx sqlite-wal-bytes /home/docker/kpanel/data/agent/nested/store.db-wal
  test "$(stat -c %a /home/docker/kpanel/data/panel/cluster-light-secrets/host-one.lightkey)" = 600
  cmp "$TEST_DIR/original-env" /home/docker/kpanel/.env
  cmp "$TEST_DIR/original-service" /home/docker/kpanel/kejilion-agent.service
  [ "$init" != openrc ] || cmp "$TEST_DIR/original-openrc" /home/docker/kpanel/kejilion-agent.openrc
  test "$(cat "$MOCK_STATE/running-id")" = "$old_image"

  rm -f "$MOCK_STATE/automatic-data-mutated" "$MOCK_STATE/automatic-crashed"
  if (KPANEL_MOCK_MUTATE_DATA_ON_UP=1 KPANEL_MOCK_CRASH_AFTER_TARGET_UP=1 docker_app_update); then
    echo 'interrupted manual update succeeded' >&2; return 1
  fi
  test -d /home/docker/kpanel/update-state/transaction
  test -f /home/docker/kpanel/run/update-freeze
  transaction_id=$(kpanel_transaction_value id)
  archive="/home/docker/kpanel/update-state/backups/$transaction_id/data.tar"
  cp "$archive" "$TEST_DIR/good-data.tar"
  printf corrupt >>"$archive"
  if KJ_KPANEL_FORCE_ROLLBACK=1 recover_fresh; then
    echo 'corrupt snapshot restored' >&2; return 1
  fi
  test -d /home/docker/kpanel/update-state/transaction
  grep -Fx mutated-panel-data /home/docker/kpanel/data/panel/rollback-marker
  cp "$TEST_DIR/good-data.tar" "$archive"
  # Runtime restoration must be retryable even after a failed snapshot check.
  mv /home/docker/kpanel/agent.env.rollback "$TEST_DIR/missing-rollback"
  if KJ_KPANEL_FORCE_ROLLBACK=1 kpanel_recover_automatic_update; then
    echo 'missing runtime rollback accepted' >&2; return 1
  fi
  mv "$TEST_DIR/missing-rollback" /home/docker/kpanel/agent.env.rollback
  if KPANEL_MOCK_PANEL_STOP_FAIL=1 KJ_KPANEL_FORCE_ROLLBACK=1 kpanel_recover_automatic_update; then
    echo 'restored while Panel still writing' >&2; return 1
  fi
  grep -Fx mutated-panel-data /home/docker/kpanel/data/panel/rollback-marker
  # Simulate a second interruption after the old Panel directory has moved.
  # Also cover a transaction written by releases without target-image-id.
  rm /home/docker/kpanel/update-state/transaction/target-image-id
  if (
    mv() {
      command mv "$@" || return
      if [ "$1" = /home/docker/kpanel/data/panel ]; then
        kill -KILL "$BASHPID"
      fi
    }
    KJ_KPANEL_FORCE_ROLLBACK=1 kpanel_recover_automatic_update
  ); then
    echo 'interrupted data restore completed unexpectedly' >&2; return 1
  fi
  test -d /home/docker/kpanel/update-state/transaction
  test "$(kpanel_transaction_value phase)" = restoring-data
  KJ_KPANEL_FORCE_ROLLBACK=1 recover_fresh
  test ! -e /home/docker/kpanel/update-state/transaction
  test ! -e /home/docker/kpanel/run/update-freeze
  cmp "$TEST_DIR/legacy-lifecycle" /home/docker/kpanel/bin/kpanel.conf
  grep -Fx original-panel-data /home/docker/kpanel/data/panel/rollback-marker
  grep -Fx original-agent-data /home/docker/kpanel/data/agent/rollback-marker
  test "$(cat "$MOCK_STATE/running-id")" = "$old_image"

  # The RC transaction used by preview updates must also be recoverable.
  if [ "$init" = systemd ]; then
    for failure in SPACE TAR CHECKSUM AGENT_STOP PANEL_STOP; do
      export "KPANEL_MOCK_${failure}_FAIL=1"
      if KJ_KPANEL_AUTOMATIC=1 KJ_KPANEL_TARGET_VERSION="$RELEASE_VERSION" \
        KJ_KPANEL_TARGET_IMAGE=docker.io/kjlion/kejilion-panel@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc \
        docker_app_update; then
        echo "automatic update accepted $failure failure" >&2; return 1
      fi
      unset "KPANEL_MOCK_${failure}_FAIL"
      if [ -d /home/docker/kpanel/update-state/transaction ]; then
        kpanel_recover_automatic_update
      fi
      test ! -e /home/docker/kpanel/update-state/transaction
      test "$(cat "$MOCK_STATE/running-id")" = "$old_image"
    done
    if KJ_KPANEL_AUTOMATIC=1 KJ_KPANEL_TARGET_VERSION=10.0.0-rc.2 \
      KJ_KPANEL_TARGET_IMAGE=docker.io/kjlion/kejilion-panel@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc \
      KPANEL_MOCK_IMAGE_VERSION=10.0.0-rc.2 KPANEL_MOCK_RELEASE_FILE_VERSION=10.0.0-rc.2 \
      KPANEL_MOCK_AGENT_VERSION=10.0.0-rc.2 KPANEL_MOCK_RUNNING_VERSION=10.0.0-rc.2 \
      KPANEL_MOCK_UPDATE_HEALTH_FAIL=1 docker_app_update; then
      echo 'failed preview update succeeded' >&2; return 1
    fi
    test ! -e /home/docker/kpanel/update-state/transaction
    grep -Fx original-agent-data /home/docker/kpanel/data/agent/rollback-marker
  fi
  docker_app_update
  test ! -e /home/docker/kpanel/update-state/transaction
  test ! -e /home/docker/kpanel/run/update-freeze
  test "$(find /home/docker/kpanel/update-state/backups -name data.tar | wc -l)" -eq 2
  for archive in /home/docker/kpanel/update-state/backups/*/data.tar; do
    sha256sum -c "${archive%/*}/data.sha256"
    tar -tf "$archive" | grep -Fx data/panel/cluster-light-secrets/host-one.lightkey
    tar -tf "$archive" | grep -Fx data/agent/nested/store.db-wal
  done
  unset KPANEL_MOCK_REAL_IMAGE_IDS
  unset KPANEL_MOCK_LIFECYCLE_SOURCE
  docker_app_uninstall >/dev/null
  rm -f "$MOCK_STATE"/*
}

run_backup_parity systemd
run_backup_parity openrc
printf '%s\n' 'app_conf_update_backup_parity=pass'
