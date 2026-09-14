#!/usr/bin/env bash
set -euo pipefail

[ "${KPANEL_APP_CONF_TEST_ROOTFS:-}" = 1 ] && [ -f /.dockerenv ] || {
	echo "refusing to run outside the disposable app-conf test container" >&2
	exit 1
}

PROJECT_DIR=${1:-/src}
TEST_DIR=$(mktemp -d /tmp/kpanel-app-openrc-test.XXXXXX)
OPENRC_LOG=$TEST_DIR/openrc.log
GROUP_LOG=$TEST_DIR/group.log
MANUAL_SERVICE=$TEST_DIR/manual-kejilion-agent

cleanup() {
	rm -f /etc/init.d/kejilion-agent
	rm -rf /home/docker/kpanel /run/openrc
	case "$TEST_DIR" in
		/tmp/kpanel-app-openrc-test.*) rm -rf -- "$TEST_DIR" ;;
	esac
}
trap cleanup EXIT HUP INT TERM

mkdir -p /home/docker/kpanel/bin /home/docker/kpanel/run \
	/etc/init.d /etc/runlevels/default /run/openrc
mkdir -p "$TEST_DIR/bin"
cat >"$TEST_DIR/bin/addgroup" <<'MOCK_ADDGROUP'
#!/bin/sh
printf 'addgroup %s\n' "$*" >>"${KPANEL_GROUP_TEST_LOG:?}"
MOCK_ADDGROUP
cat >"$TEST_DIR/bin/delgroup" <<'MOCK_DELGROUP'
#!/bin/sh
printf 'delgroup %s\n' "$*" >>"${KPANEL_GROUP_TEST_LOG:?}"
MOCK_DELGROUP
chmod 0755 "$TEST_DIR/bin/addgroup" "$TEST_DIR/bin/delgroup"
for tool in rc-service rc-update start-stop-daemon supervise-daemon; do
	rm -f "/sbin/$tool"
done
cat >/sbin/rc-service <<'MOCK_RC_SERVICE'
#!/bin/sh
printf 'rc-service %s\n' "$*" >>"${KPANEL_OPENRC_TEST_LOG:?}"
MOCK_RC_SERVICE
cat >/sbin/rc-update <<'MOCK_RC_UPDATE'
#!/bin/sh
printf 'rc-update %s\n' "$*" >>"${KPANEL_OPENRC_TEST_LOG:?}"
MOCK_RC_UPDATE
for tool in start-stop-daemon supervise-daemon; do
	printf '%s\n' '#!/bin/sh' 'exit 0' >"/sbin/$tool"
done
chmod 0755 /sbin/rc-service /sbin/rc-update /sbin/start-stop-daemon /sbin/supervise-daemon
export KPANEL_OPENRC_TEST_LOG=$OPENRC_LOG
export KPANEL_GROUP_TEST_LOG=$GROUP_LOG
export PATH="$TEST_DIR/bin:$PATH"

docker_app_plus() {
	:
}

run_contract() {
	# The app definition uses local metadata declarations and is sourced by the
	# kejilion.sh dispatcher inside a function in production.
	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"

	kpanel_detect_init_system
	[ "$KPANEL_INIT_SYSTEM" = openrc ]
	kpanel_system_group_add kejilion-test
	kpanel_system_group_delete kejilion-test
	kpanel_write_agent_service
	test -x /home/docker/kpanel/kejilion-agent.openrc
	grep -Fx '#!/sbin/openrc-run' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'supervisor="supervise-daemon"' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'command_user="root:kejilion-panel"' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'stopgroup=true' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'no_new_privs=true' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'output_logger="logger -t kejilion-agent"' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'error_logger="logger -t kejilion-agent"' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	! grep -F 'output_log=' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	grep -Fx 'export KEJILION_DOCKER_ALLOW_SOCKET_ACTIVATION="false"' /home/docker/kpanel/kejilion-agent.openrc >/dev/null
	test "$(stat -c '%a' /home/docker/kpanel/agent.env)" = 600

	kpanel_agent_service_link
	kpanel_agent_service_managed
	kpanel_agent_service_reload
	kpanel_agent_service_enable_start
	kpanel_agent_service_start
	kpanel_agent_service_stop
	kpanel_agent_service_disable_stop
	kpanel_agent_service_unlink
	test ! -e /etc/init.d/kejilion-agent

	cat >"$MANUAL_SERVICE" <<'MANUAL_OPENRC'
#!/sbin/openrc-run
command=/bin/false
MANUAL_OPENRC
	chmod 0755 "$MANUAL_SERVICE"
	ln -s "$MANUAL_SERVICE" /etc/init.d/kejilion-agent
	kpanel_agent_service_unlink
	test "$(readlink -f /etc/init.d/kejilion-agent)" = "$MANUAL_SERVICE"
}

run_contract
cat >"$TEST_DIR/expected.log" <<'EXPECTED_OPENRC'
rc-update add kejilion-agent default
rc-service kejilion-agent start
rc-service kejilion-agent start
rc-service kejilion-agent stop
rc-service kejilion-agent stop
rc-update del kejilion-agent default
EXPECTED_OPENRC
cmp "$TEST_DIR/expected.log" "$OPENRC_LOG"
cat >"$TEST_DIR/expected-group.log" <<'EXPECTED_GROUP'
addgroup -S kejilion-test
delgroup kejilion-test
EXPECTED_GROUP
cmp "$TEST_DIR/expected-group.log" "$GROUP_LOG"

printf '%s\n' 'app_conf_openrc_lifecycle=pass'
