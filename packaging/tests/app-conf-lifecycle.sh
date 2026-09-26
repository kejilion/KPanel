#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2329
set -eu

[ "${KPANEL_APP_CONF_TEST_ROOTFS:-}" = 1 ] && [ -f /.dockerenv ] || {
	echo "refusing to run outside the disposable app-conf test container" >&2
	exit 1
}

PROJECT_DIR=${1:-/src}
RELEASE_VERSION=$(tr -d '\r\n' <"$PROJECT_DIR/VERSION")
export KPANEL_RELEASE_VERSION=$RELEASE_VERSION
export KPANEL_PROJECT_DIR=$PROJECT_DIR
export KPANEL_REAL_SHA256=$(command -v sha256sum)
TEST_DIR=$(mktemp -d /tmp/kpanel-app-conf-test.XXXXXX)
FAKE_BIN="$TEST_DIR/bin"
MOCK_STATE="$TEST_DIR/state"
mkdir -p "$FAKE_BIN" "$MOCK_STATE" /run/systemd/system /etc/systemd/system /home/docker

cleanup() {
	case "$TEST_DIR" in
		/tmp/kpanel-app-conf-test.*)
			rm -rf -- "$TEST_DIR"
			;;
	esac
	rm -rf -- /home/docker/kpanel
	rm -f /bin/systemctl
	rm -f /root/kejilion.sh
}
trap cleanup EXIT HUP INT TERM

cat >"$FAKE_BIN/docker" <<'EOF'
#!/bin/sh
set -eu
state=${KPANEL_MOCK_STATE:-}
require_state() {
	[ -n "$state" ] || {
		echo "KPANEL_MOCK_STATE is required for lifecycle mutations" >&2
		exit 2
	}
}
case "$1 ${2:-}" in
	"compose version")
		exit 0
		;;
	"pull "*)
		if [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ]; then
			printf '%s\n' 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' >"$state/target-id"
			[ "$2" != docker.io/kjlion/kejilion-panel:latest ] || cp "$state/target-id" "$state/latest-id"
			rm -f "$state/rollback-tagged" "$state/automatic-restored"
		fi
		printf '%s\n' "${2:-}" |
			grep -Eq '^docker\.io/kjlion/kejilion-panel:(latest|[0-9]+\.[0-9]+\.[0-9]+)$|^docker\.io/kjlion/kejilion-panel@sha256:[0-9a-f]{64}$'
		exit
		;;
	"ps -a")
		exit 0
		;;
	"network ls")
		exit 0
		;;
	"network inspect")
		require_state
		if [ "${3:-}" = "--format" ]; then
			case "${5:-}" in
				kejilion-panel-internal) printf '%s\n' '172.30.0.0/16' ;;
				kejilion-panel-egress) printf '%s\n' '172.31.0.1' ;;
				*) exit 2 ;;
			esac
			exit 0
		fi
		[ -f "$state/network" ]
		exit
		;;
	"create --name")
		require_state
		: >"$state/release-container"
		printf '%s\n' mock-release-container
		exit 0
		;;
	"container create")
		require_state
		: >"$state/network-preflight"
		printf '%s\n' mock-network-preflight
		exit 0
		;;
	"container start")
		require_state
		[ -f "$state/network-preflight" ]
		if [ "${KPANEL_MOCK_PORT_PUBLISH_FAIL:-0}" = 1 ]; then
			echo "simulated missing DOCKER chain" >&2
			exit 1
		fi
		exit 0
		;;
	"container rm")
		require_state
		rm -f "$state/network-preflight"
		exit 0
		;;
	"cp "*)
		destination=$3
		case "$2" in
			*:/release/kpanel.conf)
				cp "${KPANEL_MOCK_LIFECYCLE_SOURCE:-${KPANEL_PROJECT_DIR:?}/packaging/kejilion-app/kpanel.conf}" \
					"$destination"
				exit 0
				;;
			*:/release/VERSION)
				printf '%s\n' \
					"${KPANEL_MOCK_RELEASE_FILE_VERSION:-${KPANEL_RELEASE_VERSION:?}}" \
					>"$destination"
				exit 0
				;;
			*:/release/kejilion.sh)
				cat >"$destination" <<'SCRIPT'
#!/usr/bin/env bash
canshu="default"
permission_granted="false"
ENABLE_STATS="true"
KJ_DNS_NONINTERACTIVE=1
KJ_APP_NONINTERACTIVE=1
KJ_WEB_NONINTERACTIVE=1
KJ_WEB_INTERACTIVE=1
KJ_TEST_NONINTERACTIVE=1
SCRIPT
				chmod 700 "$destination"
				exit 0
				;;
		esac
		mock_agent_version="${KPANEL_MOCK_AGENT_VERSION:-${KPANEL_RELEASE_VERSION:?}}"
		cat >"$destination" <<'AGENT'
#!/bin/sh
case "${1:-}" in
	version)
		printf '%s v1alpha1\n' '__KPANEL_MOCK_AGENT_VERSION__'
		;;
	healthcheck)
		[ -f "${KEJILION_AGENT_TOKEN_FILE:?}" ]
		[ "$(stat -c '%a' "$KEJILION_AGENT_TOKEN_FILE")" = 640 ]
		[ "$(tr -d '\r\n' <"$KEJILION_AGENT_TOKEN_FILE" | wc -c)" = 64 ]
		;;
	*) exit 0 ;;
esac
AGENT
		sed -i "s/__KPANEL_MOCK_AGENT_VERSION__/${mock_agent_version}/" "$destination"
		chmod 755 "$destination"
		exit 0
		;;
	"image inspect")
		case "$4" in
			*"{{.Id}}"*)
				if [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ] && [ "${5#*@}" != "$5" ]; then
					cat "$state/target-id"
					exit 0
				fi
				if [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ] && [ -f "$state/latest-id" ]; then
					cat "$state/latest-id"
					exit 0
				fi
				printf '%s\n' \
					"${KPANEL_MOCK_TARGET_IMAGE_ID:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
				;;
			*org.opencontainers.image.version*)
				printf '%s\n' \
					"${KPANEL_MOCK_IMAGE_VERSION:-${KPANEL_RELEASE_VERSION:?}}"
				;;
			*io.kejilion.kpanel.update-freeze*)
				printf '%s\n' "${KPANEL_MOCK_UPDATE_FREEZE_CAPABILITY-1}"
				;;
			*org.opencontainers.image.revision*)
				printf '%s\n' \
					"${KPANEL_MOCK_IMAGE_REVISION:-2222222222222222222222222222222222222222}"
				;;
			*io.kejilion.script.revision*)
				printf '%s\n' \
					"${KPANEL_MOCK_SCRIPT_REVISION:-4444444444444444444444444444444444444444}"
				;;
			*io.kejilion.script.sha256*)
				printf '%s\n' \
					"${KPANEL_MOCK_SCRIPT_SHA256:-1111111111111111111111111111111111111111111111111111111111111111}"
				;;
			*) exit 2 ;;
		esac
		exit 0
		;;
	"image tag")
		require_state
		printf '%s|%s\n' "$3" "$4" >"$state/image-tag"
		: >"$state/rollback-tagged"
		[ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" != 1 ] || printf '%s\n' "$3" >"$state/latest-id"
		exit 0
		;;
	"image ls")
		case " $* " in
			*" --all "*|*" -a "*) ;;
			*) exit 0 ;;
		esac
		printf '%s\n' "${KPANEL_MOCK_OLD_IMAGE_IDS:-}"
		exit 0
		;;
	"image rm")
		require_state
		printf '%s\n' "$3" >>"$state/image-rm"
		exit 0
		;;
	"ps -aq")
		case "${4:-}" in
			"ancestor=${KPANEL_MOCK_IN_USE_IMAGE_ID:-unused}")
				printf '%s\n' mock-container
				;;
		esac
		exit 0
		;;
	"rm "*)
		exit 0
		;;
	"inspect --format")
		case "$3" in
			*"{{.Image}}"*)
				if [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ] && [ -f "$state/running-id" ]; then
					cat "$state/running-id"
					exit 0
				fi
				printf '%s\n' \
				'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' ;;
			*PortBindings*) printf '%s\n' "${KPANEL_MOCK_CURRENT_PORT:-18080}" ;;
			*NetworkSettings*) printf '%s\n' 1 ;;
			*org.opencontainers.image.version*)
				printf '%s\n' \
					"${KPANEL_MOCK_RUNNING_VERSION:-${KPANEL_MOCK_IMAGE_VERSION:-${KPANEL_RELEASE_VERSION:?}}}"
				;;
			*)
				if [ "${KPANEL_MOCK_HEALTH_FAIL:-0}" = 1 ] ||
					{ [ "${KPANEL_MOCK_UPDATE_HEALTH_FAIL:-0}" = 1 ] &&
						[ ! -f "$state/rollback-tagged" ] &&
						[ ! -f "$state/automatic-restored" ]; }; then
					printf '%s\n' unhealthy
				else
					printf '%s\n' healthy
				fi
				;;
		esac
		exit 0
		;;
	"compose --env-file")
		require_state
		case "${4:-}" in
			create) : >"$state/network" ;;
			up)
				rm -f "$state/panel-stopped"
				if [ "$(cat /home/docker/kpanel/update-state/transaction/phase 2>/dev/null)" = verifying ] &&
					[ ! -f /home/docker/kpanel/run/update-freeze ]; then
					echo 'target Panel started without update freeze' >&2
					exit 94
				fi
				if [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ] && [ -f "$state/latest-id" ]; then
					if grep -F 'image: docker.io/kjlion/kejilion-panel@' /home/docker/kpanel/docker-compose.yml >/dev/null; then
						cp "$state/target-id" "$state/running-id"
					else
						cp "$state/latest-id" "$state/running-id"
					fi
				fi
				if grep -Eq '^    image: docker\.io/kjlion/kejilion-panel@sha256:[0-9a-f]{64}$' \
					/home/docker/kpanel/docker-compose.yml ||
					{ [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ] &&
					  [ "$(cat /home/docker/kpanel/update-state/transaction/phase 2>/dev/null)" = verifying ]; }; then
					: >"$state/automatic-target-started"
					if [ "${KPANEL_MOCK_MUTATE_DATA_ON_UP:-0}" = 1 ] &&
						[ ! -f "$state/automatic-data-mutated" ]; then
						printf '%s\n' 'mutated-panel-data' \
							>/home/docker/kpanel/data/panel/rollback-marker
						printf '%s\n' 'mutated-agent-data' \
							>/home/docker/kpanel/data/agent/rollback-marker
						: >"$state/automatic-data-mutated"
					fi
					if [ "${KPANEL_MOCK_CRASH_AFTER_TARGET_UP:-0}" = 1 ] &&
						[ ! -f "$state/automatic-crashed" ]; then
						: >"$state/automatic-crashed"
						kill -KILL "$PPID"
						exit 137
					fi
				elif [ -f "$state/automatic-target-started" ]; then
					: >"$state/automatic-restored"
				fi
				if [ "$(cat /home/docker/kpanel/update-state/transaction/phase 2>/dev/null)" = rollback-ready ] &&
					[ "${KPANEL_MOCK_CRASH_AFTER_ROLLBACK_UP:-0}" = 1 ] &&
					[ ! -f "$state/rollback-crashed" ]; then
					printf '%s\n' 'changed-after-old-runtime-start' \
						>/home/docker/kpanel/data/panel/cluster-light-secrets/host-one.lightkey
					: >"$state/rollback-crashed"
					kill -KILL "$PPID"
					exit 137
				fi
				if [ "${KPANEL_MOCK_BOOTSTRAP_MISSING:-0}" != 1 ]; then
					mkdir -p /home/docker/kpanel/data/panel
					printf '%s\n' 'test-bootstrap-token' \
						>/home/docker/kpanel/data/panel/bootstrap.token
					chmod 600 /home/docker/kpanel/data/panel/bootstrap.token
				fi
				;;
			stop)
				[ "${KPANEL_MOCK_PANEL_STOP_FAIL:-0}" != 1 ] || exit 1
				: >"$state/panel-stopped"
				;;
			down) rm -f "$state/network" ;;
			*) exit 2 ;;
		esac
		exit 0
		;;
esac
echo "unexpected docker invocation: $*" >&2
exit 2
EOF

cat >"$FAKE_BIN/systemctl" <<'EOF'
#!/bin/sh
printf '%s|%s\n' "$#" "$*" >>"${KPANEL_MOCK_SYSTEMCTL_LOG:?}"
if [ "$1" = "--version" ]; then
	printf '%s\n' 'systemd 255 (mock)'
	exit 0
fi
case "$1" in
	stop)
		[ "${KPANEL_MOCK_AGENT_STOP_FAIL:-0}" != 1 ] || exit 1
		: >"${KPANEL_MOCK_STATE}/agent-stopped"
		exit 0
		;;
	start) rm -f "${KPANEL_MOCK_STATE}/agent-stopped"; exit 0 ;;
	link)
		ln -sf "$2" "/etc/systemd/system/$(basename "$2")"
		exit 0
		;;
	daemon-reload|enable|start|stop|disable) exit 0 ;;
esac
echo "unexpected systemctl invocation: $*" >&2
exit 2
EOF

cat >"$FAKE_BIN/getent" <<'EOF'
#!/bin/sh
[ "$1" = group ] && [ "$2" = kejilion-panel ] || exit 1
printf '%s\n' 'kejilion-panel:x:987:'
EOF

cat >"$FAKE_BIN/groupadd" <<'EOF'
#!/bin/sh
exit 0
EOF

cat >"$FAKE_BIN/groupdel" <<'EOF'
#!/bin/sh
exit 0
EOF

cat >"$FAKE_BIN/sleep" <<'EOF'
#!/bin/sh
exit 0
EOF

cat >"$FAKE_BIN/flock" <<'EOF'
#!/bin/sh
exit 0
EOF

cat >"$FAKE_BIN/journalctl" <<'EOF'
#!/bin/sh
exit 0
EOF

cat >"$FAKE_BIN/sha256sum" <<'EOF'
#!/bin/sh
case "$*" in
	*data.tar*|*data.sha256*)
		[ "${KPANEL_MOCK_CHECKSUM_FAIL:-0}" != 1 ] || exit 1
		exec "${KPANEL_REAL_SHA256:?}" "$@"
		;;
esac
printf '%s  %s\n' \
	"${KPANEL_MOCK_SCRIPT_SHA256_ACTUAL:-1111111111111111111111111111111111111111111111111111111111111111}" \
	"$1"
EOF
chmod 755 "$FAKE_BIN"/*
[ ! -e /bin/systemctl ] || {
	echo "disposable app-conf test image unexpectedly provides /bin/systemctl" >&2
	exit 1
}
ln -s "$FAKE_BIN/systemctl" /bin/systemctl

run_lifecycle() {
	local ipv4_address="198.51.100.25"
	local interrupted_digest="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	local failed_digest="sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	local mismatched_digest="sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	local preview_digest="sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	cat >/root/kejilion.sh <<'EOF'
#!/usr/bin/env bash
canshu="CN"
permission_granted="true"
ENABLE_STATS="false"
EOF
	chmod 700 /root/kejilion.sh

	systemctl() {
		local COMMAND="$1"
		local SERVICE_NAME="${2:-}"
		/bin/systemctl "$COMMAND" "$SERVICE_NAME"
	}
	docker_app_plus() {
		:
	}
	check_docker_app_ip() {
		:
	}

	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	docker_port="18080"
	docker_app_install >"$TEST_DIR/install-output.txt"
	grep -Fx '首次初始化 Token：test-bootstrap-token' \
		"$TEST_DIR/install-output.txt" >/dev/null
	grep -Fx '请复制此 Token 完成管理员账户初始化；初始化成功后 Token 自动失效。' \
		"$TEST_DIR/install-output.txt" >/dev/null
	if grep -F '首次初始化 Token 文件：' "$TEST_DIR/install-output.txt" >/dev/null; then
		echo "install output still asks the user to read the token file" >&2
		return 1
	fi
	grep -F "image: docker.io/kjlion/kejilion-panel:latest" \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	grep -F -- '- "18080:8080"' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx 'KPANEL_PUBLIC_URL=http://198.51.100.25:18080' \
		/home/docker/kpanel/.env >/dev/null
	grep -Fx 'KPANEL_SECURE_COOKIE=false' \
		/home/docker/kpanel/.env >/dev/null
	grep -Fx 'KPANEL_ALLOW_IP_HOSTS=true' \
		/home/docker/kpanel/.env >/dev/null
	grep -F 'KEJILION_PANEL_SECURE_COOKIE: ${KPANEL_SECURE_COOKIE:-false}' \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	grep -F 'KEJILION_PANEL_ALLOW_IP_HOSTS: ${KPANEL_ALLOW_IP_HOSTS:-true}' \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx 'KPANEL_TRUSTED_PROXY_CIDRS=127.0.0.0/8,::1/128,172.30.0.0/16,172.31.0.1/32' \
		/home/docker/kpanel/.env >/dev/null
	grep -F 'KEJILION_PANEL_CLUSTER_PRIVATE_CIDRS: ${KPANEL_CLUSTER_PRIVATE_CIDRS:-}' \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	test "$(grep -c '^    networks:$' /home/docker/kpanel/docker-compose.yml)" = 1
	grep -Fx '      - kpanel-internal' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx '      - kpanel-egress' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx '    internal: true' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx '    name: kejilion-panel-internal' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx '    name: kejilion-panel-egress' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -F 'host.docker.internal:host-gateway' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -F 'ExecStart=/home/docker/kpanel/bin/kejilion-agent' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -Fx 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_NET_ADMIN CAP_SYS_RESOURCE CAP_DAC_OVERRIDE CAP_CHOWN CAP_LINUX_IMMUTABLE CAP_SYS_PTRACE' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -Fx 'AmbientCapabilities=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_NET_ADMIN CAP_SYS_RESOURCE CAP_DAC_OVERRIDE CAP_CHOWN CAP_LINUX_IMMUTABLE CAP_SYS_PTRACE' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -Fx 'RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -Fx 'ProtectHome=false' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -Fx 'ProtectSystem=false' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -F 'kpanel_report_failed_install' \
		"$PROJECT_DIR/packaging/kejilion-app/kpanel.conf" >/dev/null
	grep -F 'docker compose --env-file .env up -d --force-recreate --remove-orphans' \
		"$PROJECT_DIR/packaging/kejilion-app/kpanel.conf" >/dev/null
	if grep -F '请运行：systemctl status kejilion-agent' \
		"$PROJECT_DIR/packaging/kejilion-app/kpanel.conf" >/dev/null; then
		echo "KPanel installer still points users at a unit removed by cleanup" >&2
		exit 1
	fi
	if grep -q '^ReadWritePaths=' /home/docker/kpanel/kejilion-agent.service; then
		echo "KPanel app unit relies on ineffective root write exceptions" >&2
		exit 1
	fi
	grep -Fx 'ReadOnlyPaths=/home/docker/kpanel/data/panel' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	test -f /home/docker/kpanel/secrets/agent.token
	test "$(stat -c '%a' /home/docker/kpanel/secrets/agent.token)" = 640
	test "$(stat -c '%u:%g' /home/docker/kpanel/secrets/agent.token)" = 0:987
	test "$(tr -d '\r\n' </home/docker/kpanel/secrets/agent.token | wc -c)" = 64
	test -f /home/docker/kpanel/.managed-by-kejilion-app
	test -f /home/docker/kpanel/bin/kpanel.conf
	cmp -s "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf" \
		/home/docker/kpanel/bin/kpanel.conf
	test "$(stat -c '%a' /home/docker/kpanel/bin/kpanel.conf)" = 600
	test "$(stat -c '%a' /home/docker/kpanel/update-state)" = 700
	test "$(stat -c '%u:%g' /home/docker/kpanel/update-state)" = 0:0
	grep -Fx 'KEJILION_AGENT_SELF_UPDATE_STATE_DIR=/home/docker/kpanel/update-state' \
		/home/docker/kpanel/agent.env >/dev/null
	grep -F 'ExecStart=/home/docker/kpanel/bin/kejilion-agent self-update-run' \
		/home/docker/kpanel/kejilion-panel-update.service >/dev/null
	grep -Fx 'OnBootSec=15min' /home/docker/kpanel/kejilion-panel-update.timer >/dev/null
	grep -Fx 'OnCalendar=*-*-* 04:00:00' /home/docker/kpanel/kejilion-panel-update.timer >/dev/null
	grep -Fx 'RandomizedDelaySec=30min' /home/docker/kpanel/kejilion-panel-update.timer >/dev/null
	test "$(readlink -f /etc/systemd/system/kejilion-panel-update.service)" = \
		"$(readlink -f /home/docker/kpanel/kejilion-panel-update.service)"
	test "$(readlink -f /etc/systemd/system/kejilion-panel-update.timer)" = \
		"$(readlink -f /home/docker/kpanel/kejilion-panel-update.timer)"
	test -x /home/docker/kpanel/bin/kejilion.sh
	test "$(stat -c '%a' /home/docker/kpanel/bin/kejilion.sh)" = 700
	grep -Fx 'permission_granted="true"' /home/docker/kpanel/bin/kejilion.sh >/dev/null
	grep -Fx 'ENABLE_STATS="false"' /home/docker/kpanel/bin/kejilion.sh >/dev/null
	grep -Fx 'canshu="CN"' /home/docker/kpanel/bin/kejilion.sh >/dev/null

	local systemctl_lines_before=""
	systemctl_lines_before="$(wc -l <"$KPANEL_MOCK_SYSTEMCTL_LOG")"
	if KPANEL_MOCK_PORT_PUBLISH_FAIL=1 docker_app_update \
		>"$TEST_DIR/network-preflight-output.txt" 2>&1; then
		echo "KPanel update ignored a failed Docker port-publish preflight" >&2
		return 1
	fi
	grep -F 'Docker 端口映射预检失败' \
		"$TEST_DIR/network-preflight-output.txt" >/dev/null
	test "$(wc -l <"$KPANEL_MOCK_SYSTEMCTL_LOG")" = "$systemctl_lines_before"
	test ! -e "$MOCK_STATE/network-preflight"
	test ! -e "$MOCK_STATE/rollback-tagged"
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"
	if KPANEL_MOCK_UPDATE_FREEZE_CAPABILITY=0 docker_app_update \
		>"$TEST_DIR/freeze-capability-output.txt" 2>&1; then
		echo "KPanel update accepted a target without write-freeze support" >&2
		return 1
	fi
	grep -F '目标镜像未声明更新期写入冻结能力' \
		"$TEST_DIR/freeze-capability-output.txt" >/dev/null
	test "$(wc -l <"$KPANEL_MOCK_SYSTEMCTL_LOG")" = "$systemctl_lines_before"
	test ! -e /home/docker/kpanel/update-state/transaction
	test ! -e /home/docker/kpanel/run/update-freeze
	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag"

	rm -rf /home/docker/kpanel/update-state
	rm -f "$MOCK_STATE/image-rm"
	local current_image_id='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
	local removable_image_id='sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
	local retained_image_id='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'
	KPANEL_MOCK_OLD_IMAGE_IDS="$(printf '%s\n%s\n%s' \
		"$current_image_id" "$removable_image_id" "$retained_image_id")" \
		KPANEL_MOCK_IN_USE_IMAGE_ID="$retained_image_id" \
		docker_app_update >"$TEST_DIR/image-cleanup-output.txt"
	test "$(stat -c '%a' /home/docker/kpanel/update-state)" = 700
	test "$(stat -c '%u:%g' /home/docker/kpanel/update-state)" = 0:0
	test "$(stat -c '%a' /home/docker/kpanel/update-state/backups)" = 700
	test "$(cat "$MOCK_STATE/image-rm")" = "$removable_image_id"
	if grep -Ei 'image|镜像|cleanup|清理' "$TEST_DIR/image-cleanup-output.txt" >/dev/null; then
		echo "successful KPanel update exposed old-image cleanup output" >&2
		return 1
	fi
	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag"
	if KPANEL_MOCK_AGENT_VERSION=9.9.9 docker_app_update; then
		echo "KPanel update accepted a mismatched Agent" >&2
		return 1
	fi
	grep -Fx \
		'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|docker.io/kjlion/kejilion-panel:latest' \
		"$MOCK_STATE/image-tag" >/dev/null
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"

	sed -i 's#^KPANEL_TRUSTED_PROXY_CIDRS=.*#KPANEL_TRUSTED_PROXY_CIDRS=127.0.0.0/8,::1/128,172.20.0.0/16#' \
		/home/docker/kpanel/.env
	sed -i '/^KPANEL_SECURE_COOKIE=/d' /home/docker/kpanel/.env
	docker_port="8080"
	docker_app_update
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"
	grep -Fx 'permission_granted="true"' /home/docker/kpanel/bin/kejilion.sh >/dev/null
	grep -F -- '- "18080:8080"' /home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx 'KPANEL_SECURE_COOKIE=false' \
		/home/docker/kpanel/.env >/dev/null
	grep -Fx 'KPANEL_TRUSTED_PROXY_CIDRS=127.0.0.0/8,::1/128,172.30.0.0/16,172.31.0.1/32' \
		/home/docker/kpanel/.env >/dev/null
	test ! -e /home/docker/kpanel/.env.rollback

	sed -i 's#^KPANEL_PUBLIC_URL=.*#KPANEL_PUBLIC_URL=https://panel.example.com#' \
		/home/docker/kpanel/.env
	sed -i '/^KPANEL_SECURE_COOKIE=/d' /home/docker/kpanel/.env
	docker_app_update
	grep -Fx 'KPANEL_SECURE_COOKIE=true' /home/docker/kpanel/.env >/dev/null
	grep -Fx 'KPANEL_PUBLIC_URL=https://panel.example.com' /home/docker/kpanel/.env >/dev/null
	cp -p /home/docker/kpanel/.env "$TEST_DIR/https-env"

	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag" "$MOCK_STATE/image-rm"
	if KPANEL_MOCK_UPDATE_HEALTH_FAIL=1 \
		KPANEL_MOCK_OLD_IMAGE_IDS='sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd' \
		docker_app_update; then
		echo "failed KPanel update unexpectedly succeeded" >&2
		return 1
	fi
	grep -Fx \
		'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|docker.io/kjlion/kejilion-panel:latest' \
		"$MOCK_STATE/image-tag" >/dev/null
	grep -F -- '- "18080:8080"' /home/docker/kpanel/docker-compose.yml >/dev/null
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"
	cmp -s "$TEST_DIR/https-env" /home/docker/kpanel/.env
	grep -Fx 'KPANEL_SECURE_COOKIE=true' /home/docker/kpanel/.env >/dev/null
	test ! -e /home/docker/kpanel/.env.rollback
	test ! -e "$MOCK_STATE/image-rm"

	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag" \
		"$MOCK_STATE/automatic-target-started" "$MOCK_STATE/automatic-restored"
	if KJ_KPANEL_AUTOMATIC=1 \
		KJ_KPANEL_TARGET_VERSION=9.9.7 \
		KJ_KPANEL_TARGET_IMAGE="docker.io/kjlion/kejilion-panel@${mismatched_digest}" \
		KPANEL_MOCK_IMAGE_VERSION=9.9.7 \
		KPANEL_MOCK_RELEASE_FILE_VERSION=9.9.7 \
		KPANEL_MOCK_AGENT_VERSION=9.9.7 \
		KPANEL_MOCK_RUNNING_VERSION=9.9.7 \
		KPANEL_MOCK_TARGET_IMAGE_ID=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
		docker_app_update >"$TEST_DIR/automatic-image-mismatch-output.txt" 2>&1; then
		echo "automatic KPanel update accepted the wrong running image" >&2
		return 1
	fi
	grep -F 'image: docker.io/kjlion/kejilion-panel:latest' \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	test ! -e /home/docker/kpanel/update-state/transaction
	test ! -e "$MOCK_STATE/rollback-tagged"
	test ! -e "$MOCK_STATE/image-tag"
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"

	printf '%s\n' 'original-panel-data' \
		>/home/docker/kpanel/data/panel/rollback-marker
	printf '%s\n' 'original-agent-data' \
		>/home/docker/kpanel/data/agent/rollback-marker
	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag" \
		"$MOCK_STATE/automatic-target-started" "$MOCK_STATE/automatic-restored" \
		"$MOCK_STATE/automatic-data-mutated" "$MOCK_STATE/automatic-crashed"
	if (
		KJ_KPANEL_AUTOMATIC=1 \
		KJ_KPANEL_TARGET_VERSION=9.9.8 \
		KJ_KPANEL_TARGET_IMAGE="docker.io/kjlion/kejilion-panel@${interrupted_digest}" \
		KPANEL_MOCK_IMAGE_VERSION=9.9.8 \
		KPANEL_MOCK_RELEASE_FILE_VERSION=9.9.8 \
		KPANEL_MOCK_AGENT_VERSION=9.9.8 \
		KPANEL_MOCK_RUNNING_VERSION=9.9.8 \
		KPANEL_MOCK_MUTATE_DATA_ON_UP=1 \
		KPANEL_MOCK_CRASH_AFTER_TARGET_UP=1 \
			docker_app_update
	); then
		echo "interrupted automatic KPanel update unexpectedly completed" >&2
		return 1
	fi
	test -d /home/docker/kpanel/update-state/transaction
	grep -Fx 'mutated-panel-data' \
		/home/docker/kpanel/data/panel/rollback-marker >/dev/null
	kpanel_recover_automatic_update >"$TEST_DIR/interrupted-recovery-output.txt"
	grep -Fx 'original-panel-data' \
		/home/docker/kpanel/data/panel/rollback-marker >/dev/null
	grep -Fx 'original-agent-data' \
		/home/docker/kpanel/data/agent/rollback-marker >/dev/null
	test ! -e /home/docker/kpanel/update-state/transaction
	test ! -e "$MOCK_STATE/rollback-tagged"
	test ! -e "$MOCK_STATE/image-tag"

	printf '%s\n' 'original-panel-data' \
		>/home/docker/kpanel/data/panel/rollback-marker
	printf '%s\n' 'original-agent-data' \
		>/home/docker/kpanel/data/agent/rollback-marker
	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag" \
		"$MOCK_STATE/automatic-target-started" "$MOCK_STATE/automatic-restored" \
		"$MOCK_STATE/automatic-data-mutated" "$MOCK_STATE/automatic-crashed"
	if KJ_KPANEL_AUTOMATIC=1 \
		KJ_KPANEL_TARGET_VERSION=9.9.9 \
		KJ_KPANEL_TARGET_IMAGE="docker.io/kjlion/kejilion-panel@${failed_digest}" \
		KPANEL_MOCK_IMAGE_VERSION=9.9.9 \
		KPANEL_MOCK_RELEASE_FILE_VERSION=9.9.9 \
		KPANEL_MOCK_AGENT_VERSION=9.9.9 \
		KPANEL_MOCK_RUNNING_VERSION=9.9.9 \
		KPANEL_MOCK_UPDATE_HEALTH_FAIL=1 \
		KPANEL_MOCK_MUTATE_DATA_ON_UP=1 \
		docker_app_update >"$TEST_DIR/automatic-rollback-output.txt" 2>&1; then
		echo "failed automatic KPanel update unexpectedly succeeded" >&2
		return 1
	fi
	grep -Fx 'original-panel-data' \
		/home/docker/kpanel/data/panel/rollback-marker >/dev/null
	grep -Fx 'original-agent-data' \
		/home/docker/kpanel/data/agent/rollback-marker >/dev/null
	grep -F 'image: docker.io/kjlion/kejilion-panel:latest' \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	test ! -e /home/docker/kpanel/update-state/transaction
	test -n "$(find /home/docker/kpanel/update-state/backups -mindepth 1 -maxdepth 1 -type d -print -quit)"
	test ! -e "$MOCK_STATE/rollback-tagged"
	test ! -e "$MOCK_STATE/image-tag"
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"

	rm -f "$MOCK_STATE/automatic-target-started" "$MOCK_STATE/automatic-restored" \
		"$MOCK_STATE/automatic-data-mutated" "$MOCK_STATE/automatic-crashed"
	KJ_KPANEL_AUTOMATIC=1 \
		KJ_KPANEL_TARGET_VERSION=10.0.0-rc.2 \
		KJ_KPANEL_TARGET_IMAGE="docker.io/kjlion/kejilion-panel@${preview_digest}" \
		KPANEL_MOCK_IMAGE_VERSION=10.0.0-rc.2 \
		KPANEL_MOCK_RELEASE_FILE_VERSION=10.0.0-rc.2 \
		KPANEL_MOCK_AGENT_VERSION=10.0.0-rc.2 \
		KPANEL_MOCK_RUNNING_VERSION=10.0.0-rc.2 \
		docker_app_update >"$TEST_DIR/preview-update-output.txt"
	grep -F "image: docker.io/kjlion/kejilion-panel@${preview_digest}" \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	grep -Fx 'KPanel 自动更新完成 / Automatic Update Complete' \
		"$TEST_DIR/preview-update-output.txt" >/dev/null
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "10.0.0-rc.2 v1alpha1"

	docker_app_uninstall
	[ ! -e /home/docker/kpanel ]
}

run_symlinked_docker_root_lifecycle() {
	local physical_docker_root="$TEST_DIR/physical-docker-root"

	MOCK_STATE="$TEST_DIR/symlinked-root-state"
	mkdir -p "$MOCK_STATE"
	export KPANEL_MOCK_STATE="$MOCK_STATE"
	rmdir /home/docker
	mkdir -p "$physical_docker_root"
	ln -s "$physical_docker_root" /home/docker
	test "$(readlink -f /home/docker)" = "$physical_docker_root"
	run_lifecycle
}

run_failed_install() {
	local ipv4_address="198.51.100.25"

	systemctl() {
		local COMMAND="$1"
		local SERVICE_NAME="${2:-}"
		/bin/systemctl "$COMMAND" "$SERVICE_NAME"
	}
	docker_app_plus() {
		:
	}
	check_docker_app_ip() {
		:
	}

	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	docker_port="18080"
	if KPANEL_MOCK_HEALTH_FAIL=1 docker_app_install; then
		echo "failed install unexpectedly succeeded" >&2
		return 1
	fi
	[ ! -e /home/docker/kpanel ]
	[ ! -e "$MOCK_STATE/network" ]
}

run_missing_bootstrap_token() {
	local ipv4_address="198.51.100.25"

	systemctl() {
		local COMMAND="$1"
		local SERVICE_NAME="${2:-}"
		/bin/systemctl "$COMMAND" "$SERVICE_NAME"
	}
	docker_app_plus() {
		:
	}
	check_docker_app_ip() {
		:
	}

	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	docker_port="18080"
	if KPANEL_MOCK_BOOTSTRAP_MISSING=1 docker_app_install \
		>"$TEST_DIR/missing-bootstrap-output.txt"; then
		echo "install without a bootstrap token unexpectedly succeeded" >&2
		return 1
	fi
	grep -Fx 'KPanel 初始化凭证读取失败，安装已停止。' \
		"$TEST_DIR/missing-bootstrap-output.txt" >/dev/null
	[ ! -e /home/docker/kpanel ]
	[ ! -e "$MOCK_STATE/network" ]
}

run_unmanaged_guard() {
	local manual_unit="/tmp/manual-kejilion-agent.service"

	systemctl() {
		local COMMAND="$1"
		local SERVICE_NAME="${2:-}"
		/bin/systemctl "$COMMAND" "$SERVICE_NAME"
	}
	docker_app_plus() {
		:
	}
	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	mkdir -p /home/docker/kpanel
	: >/home/docker/kpanel/.managed-by-kejilion-app
	: >/home/docker/kpanel/docker-compose.yml
	: >"$manual_unit"
	ln -sf "$manual_unit" /etc/systemd/system/kejilion-agent.service

	if docker_app_update || docker_app_uninstall; then
		echo "unmanaged KPanel instance was accepted" >&2
		return 1
	fi
	test -f /home/docker/kpanel/docker-compose.yml
	test "$(readlink -f /etc/systemd/system/kejilion-agent.service)" = "$manual_unit"
	rm -f /etc/systemd/system/kejilion-agent.service "$manual_unit"
	rm -rf /home/docker/kpanel
}

run_partial_uninstall() (
	docker_app_plus() { :; }
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	mkdir -p /home/docker/kpanel/data
	: >/home/docker/kpanel/.managed-by-kejilion-app
	printf '%s\n' preserved >/home/docker/kpanel/data/record
	# Missing Compose and service files must not prevent clearing owned leftovers.
	docker_app_uninstall
	test ! -e /home/docker/kpanel

	mkdir -p /home/docker/kpanel/data
	: >/home/docker/kpanel/.managed-by-kejilion-app
	: >/home/docker/kpanel/docker-compose.yml
	: >/home/docker/kpanel/.env
	printf '%s\n' preserved >/home/docker/kpanel/data/record
	docker() { return 1; }
	if docker_app_uninstall; then
		echo "failed Docker cleanup removed installation data" >&2
		exit 1
	fi
	grep -Fx preserved /home/docker/kpanel/data/record
	# Without Compose, an existing container must also preserve the data.
	rm /home/docker/kpanel/docker-compose.yml
	docker() { printf '%s\n' kejilion-panel; }
	if docker_app_uninstall; then
		echo "incomplete configuration with a live container was removed" >&2
		exit 1
	fi
	grep -Fx preserved /home/docker/kpanel/data/record
	local mock_network_state='other-project|0'
	local network_removed=false
	docker() {
		case "$1 ${2:-}" in
			'ps -a') return 0 ;;
			'network ls') echo kejilion-panel-internal ;;
			'network inspect') echo "$mock_network_state" ;;
			'network rm') network_removed=true ;;
			*) return 1 ;;
		esac
	}
	if docker_app_uninstall; then exit 1; fi
	test "$network_removed" = false
	grep -Fx preserved /home/docker/kpanel/data/record
	mock_network_state='kpanel|1'
	if docker_app_uninstall; then exit 1; fi
	test "$network_removed" = false
	mock_network_state='kpanel|0'
	docker_app_uninstall
	test "$network_removed" = true
	test ! -e /home/docker/kpanel
)

run_release_contract_guards() {
	local candidate_agent="$TEST_DIR/release-contract-agent"
	local candidate_script="$TEST_DIR/release-contract-script"

	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"

	if grep -Eq '"[0-9]+\.[0-9]+\.[0-9]+ v1alpha1"' \
		"$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"; then
		echo "KPanel app config still hard-codes an Agent release version" >&2
		return 1
	fi
	if grep -Eq 'local script_sha256="[0-9a-f]{64}"' \
		"$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"; then
		echo "KPanel app config still hard-codes a script release digest" >&2
		return 1
	fi

	if (
		export KPANEL_MOCK_IMAGE_VERSION=9.9.9
		kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"
	); then
		echo "release VERSION mismatch was accepted" >&2
		return 1
	fi
	if (
		export KPANEL_MOCK_AGENT_VERSION=9.9.9
		kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"
	); then
		echo "Agent version mismatch was accepted" >&2
		return 1
	fi
	if (
		export KPANEL_MOCK_SCRIPT_SHA256=3333333333333333333333333333333333333333333333333333333333333333
		kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"
	); then
		echo "script digest mismatch was accepted" >&2
		return 1
	fi
	if (
		export KPANEL_MOCK_IMAGE_REVISION=invalid
		kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"
	); then
		echo "invalid image revision was accepted" >&2
		return 1
	fi
	if (
		export KPANEL_MOCK_SCRIPT_REVISION=invalid
		kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"
	); then
		echo "invalid script revision was accepted" >&2
		return 1
	fi
	[ ! -e "$candidate_agent" ]
	[ ! -e "$candidate_script" ]
}

export PATH="$FAKE_BIN:$PATH"
export KPANEL_MOCK_STATE="$MOCK_STATE"
export KPANEL_MOCK_SYSTEMCTL_LOG="$TEST_DIR/systemctl.log"
[ "${KPANEL_APP_CONF_TEST_LIBRARY:-0}" != 1 ] || return 0
# A leftover empty directory must not block the complete install/uninstall cycle.
mkdir -p /home/docker/kpanel
run_lifecycle
run_symlinked_docker_root_lifecycle
grep -Fx '1|daemon-reload' "$KPANEL_MOCK_SYSTEMCTL_LOG" >/dev/null
grep -Fx '3|enable --now kejilion-agent.service' "$KPANEL_MOCK_SYSTEMCTL_LOG" >/dev/null
grep -Fx '3|enable --now kejilion-panel-update.timer' "$KPANEL_MOCK_SYSTEMCTL_LOG" >/dev/null
grep -Fx '3|disable --now kejilion-agent.service' "$KPANEL_MOCK_SYSTEMCTL_LOG" >/dev/null
grep -Fx '3|disable --now kejilion-panel-update.timer' "$KPANEL_MOCK_SYSTEMCTL_LOG" >/dev/null
if grep -F 'daemon-reload ' "$KPANEL_MOCK_SYSTEMCTL_LOG" >/dev/null; then
	echo "daemon-reload received the empty service argument from kejilion.sh's systemctl wrapper" >&2
	exit 1
fi
run_failed_install
run_missing_bootstrap_token
run_unmanaged_guard
run_partial_uninstall
run_release_contract_guards
printf '%s\n' "app_conf_lifecycle=pass"
# Run the failure matrix under the same disposable-rootfs gate in CI and releases.
if [ -L /home/docker ] && [ "$(readlink /home/docker)" = "$TEST_DIR/physical-docker-root" ]; then
	rm /home/docker
	mkdir /home/docker
fi
cleanup
bash "$PROJECT_DIR/packaging/tests/app-conf-update-backups.sh" "$PROJECT_DIR"
