#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2329
set -eu

[ "${KPANEL_APP_CONF_TEST_ROOTFS:-}" = 1 ] && [ -f /.dockerenv ] || {
	echo "refusing to run outside the disposable app-conf test container" >&2
	exit 1
}

PROJECT_DIR=${1:-/src}
bash "$PROJECT_DIR/packaging/tests/app-conf-acl.sh" "$PROJECT_DIR"
RELEASE_VERSION=$(tr -d '\r\n' <"$PROJECT_DIR/VERSION")
export KPANEL_RELEASE_VERSION=$RELEASE_VERSION
export KPANEL_PROJECT_DIR=$PROJECT_DIR
export KPANEL_REAL_SHA256=$(command -v sha256sum)
export KPANEL_REAL_AWK=$(command -v awk)
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
	rm -f /root/kejilion.sh /usr/local/bin/k /usr/bin/k
}
trap cleanup EXIT HUP INT TERM

cat >"$FAKE_BIN/awk" <<'EOF'
#!/bin/sh
if [ "$#" = 2 ] && [ "$2" = /proc/self/mountinfo ] && [ -n "${KPANEL_MOCK_MOUNTINFO:-}" ]; then
    exec "$KPANEL_REAL_AWK" "$1" "$KPANEL_MOCK_MOUNTINFO"
fi
exec "$KPANEL_REAL_AWK" "$@"
EOF
chmod 755 "$FAKE_BIN/awk"

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
	"exec kejilion-panel")
		[ "$3 ${4:-}" = '/paneld agent-healthcheck' ] || exit 2
		[ "${KPANEL_MOCK_PANEL_AGENT_FAIL:-0}" != 1 ]
		exit
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
			*RepoDigests*)
				if [ "${KPANEL_MOCK_REAL_IMAGE_IDS:-0}" = 1 ]; then
					printf '%s\n' 'kjlion/kejilion-panel@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
				else
					printf '%s\n' 'kjlion/kejilion-panel@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
				fi
				;;
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
				if [ "${KPANEL_MOCK_RETARGET_AFTER_GATE:-0}" = 1 ]; then
					printf '%s\n' 'sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc' >"$state/latest-id"
				fi
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
				if [ "$(cat /home/docker/kpanel/update-state/transaction/phase 2>/dev/null)" = verifying ]; then
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
case "${1:-}" in
	-n) [ "${KPANEL_MOCK_FLOCK_BUSY:-0}" != 1 ] ;;
	-u) exit 0 ;;
	*) echo "unsupported flock option: $*" >&2; exit 2 ;;
esac
EOF

cat >"$FAKE_BIN/journalctl" <<'EOF'
#!/bin/sh
exit 0
EOF

cat >"$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
destination=
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) destination=$2; shift ;;
        https://*) url=$1 ;;
    esac
    shift
done
[ -n "$destination" ] || exit 2
case "$url" in
    https://raw.githubusercontent.com/kejilion/sh/main/kejilion.sh|https://gh.kejilion.pro/raw.githubusercontent.com/kejilion/sh/main/kejilion.sh) ;;
    *) exit 2 ;;
esac
[ -z "${KPANEL_MOCK_DOWNLOAD_LOG:-}" ] || printf '%s\n' "$url" >>"$KPANEL_MOCK_DOWNLOAD_LOG"
case "${KPANEL_MOCK_DOWNLOAD:-ok}" in
    fail) printf 'partial' >"$destination"; exit 22 ;;
    empty) : >"$destination"; exit 0 ;;
    html) printf '<html>error</html>' >"$destination"; exit 0 ;;
    fallback) [ "$url" != https://raw.githubusercontent.com/kejilion/sh/main/kejilion.sh ] || exit 28 ;;
esac
cat >"$destination" <<'SCRIPT'
#!/usr/bin/env bash
sh_v="4.5.99"
canshu="default"
permission_granted="false"
ENABLE_STATS="true"
kpanel_protocol_active() {
    :
}
KJ_DNS_NONINTERACTIVE=1
KJ_APP_NONINTERACTIVE=1
KJ_WEB_NONINTERACTIVE=1
KJ_WEB_INTERACTIVE=1
KJ_TEST_NONINTERACTIVE=1
# latest-download-fixture
SCRIPT
if [ "${KPANEL_MOCK_DOWNLOAD:-}" = syntax ]; then printf '\nif\n' >>"$destination"; fi
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

run_license_inheritance() {
	local destination="$TEST_DIR/license-destination.sh"
	local original="$TEST_DIR/license-original.sh"
	local output="$TEST_DIR/license-output.txt"
	cat >"$original" <<'EOF'
canshu="default"
permission_granted="false"
ENABLE_STATS="true"
EOF
	docker_app_plus() { :; }
	# shellcheck source=/dev/null
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	# A script's existence alone is not evidence of consent; piped yes is not consent either.
	cp "$original" "$destination"
	cp "$original" /root/kejilion.sh
	chmod 700 /root/kejilion.sh
	if kpanel_inherit_script_preferences "$destination" < <(printf 'y\n') >"$output"; then
		echo "noninteractive installation accepted an unconfirmed license" >&2
		return 1
	fi
	cmp "$original" "$destination"
	grep -F '请在交互终端重新安装' "$output" >/dev/null
	sed -i 's/permission_granted="false"/permission_granted="true"/; s/ENABLE_STATS="true"/ENABLE_STATS="false"/; s/canshu="default"/canshu="CN"/' /root/kejilion.sh
	# Explicit consent must not inherit preferences from an untrusted script.
	chmod 777 /root/kejilion.sh
	# Simulate only the TTY predicate; use the real read builtin and file writes.
	interactive_license() (
		[() {
			if [[ "$*" == '-t 0 ]' ]]; then return 0; fi
			if [[ "$*" == '! -t 0 ]' ]]; then return 1; fi
			builtin [ "$@"
		}
		kpanel_inherit_script_preferences "$destination"
	)
	local answer=""
	for answer in n '' yes y Y; do
		cp "$original" "$destination"
		if printf '%s\n' "$answer" | interactive_license >"$output"; then
			printf 'accepted\n' >"$TEST_DIR/license-result"
		else
			printf 'rejected\n' >"$TEST_DIR/license-result"
		fi
		case "$answer" in
			y|Y)
				grep -Fx accepted "$TEST_DIR/license-result" >/dev/null
				grep -Fx 'permission_granted="true"' "$destination" >/dev/null
				grep -Fx 'ENABLE_STATS="true"' "$destination" >/dev/null
				grep -Fx 'canshu="default"' "$destination" >/dev/null
				;;
			*)
				grep -Fx rejected "$TEST_DIR/license-result" >/dev/null
				cmp "$original" "$destination"
				;;
		esac
	done
	cp "$original" "$destination"
	if interactive_license </dev/null >"$output"; then
		echo "end of input accepted an unconfirmed license" >&2
		return 1
	fi
	cmp "$original" "$destination"
	# Existing accepted preferences remain usable for unattended updates.
	chmod 700 /root/kejilion.sh
	cp "$original" "$destination"
	kpanel_inherit_script_preferences "$destination" </dev/null
	grep -Fx 'permission_granted="true"' "$destination" >/dev/null
	grep -Fx 'ENABLE_STATS="false"' "$destination" >/dev/null
	grep -Fx 'canshu="CN"' "$destination" >/dev/null
	rm /root/kejilion.sh
	printf '%s\n' "app_conf_license_inheritance=pass"
}

run_shared_script_bootstrap() {
	local bootstrap="$TEST_DIR/shared-bootstrap"
	printf '#!/bin/bash\n# verified image bootstrap\n' > "$bootstrap"
	mkdir -p /home/docker/kpanel/bin
	test ! -e /usr/local/bin/k
	kpanel_prepare_shared_script "$bootstrap"
	cmp "$bootstrap" /usr/local/bin/k
	cmp "$bootstrap" /root/kejilion.sh
	test "$(readlink /usr/bin/k)" = /usr/local/bin/k
	test "$(stat -c '%a' /usr/local/bin/k)" = 755
	kpanel_restore_shared_script
	test ! -e /root/kejilion.sh
	test ! -e /usr/local/bin/k
	test ! -L /usr/bin/k
	kpanel_clear_update_files
	printf 'old-root-script\n' >/root/kejilion.sh
	chmod 700 /root/kejilion.sh
	printf 'old-host-k\n' >/usr/local/bin/k
	chmod 755 /usr/local/bin/k
	printf 'old-usr-bin-k\n' >/usr/bin/k
	chmod 755 /usr/bin/k
	cp /usr/local/bin/k "$TEST_DIR/shared-current"
	cp /root/kejilion.sh "$TEST_DIR/root-current"
	kpanel_prepare_shared_script "$bootstrap"
	cmp "$bootstrap" /root/kejilion.sh
	cmp "$bootstrap" /usr/local/bin/k
	test "$(readlink /usr/bin/k)" = /usr/local/bin/k
	kpanel_restore_shared_script
	cmp "$TEST_DIR/shared-current" /usr/local/bin/k
	cmp "$TEST_DIR/root-current" /root/kejilion.sh
	test "$(stat -c '%a' /root/kejilion.sh)" = 700
	grep -Fx old-usr-bin-k /usr/bin/k >/dev/null
	kpanel_clear_update_files
	# Simulate a failure and a process death after replacing only the first file.
	local failure
	for failure in return kill; do
		if (
			mv() {
				if [ "${!#}" = /usr/local/bin/k ]; then
					[ "$failure" != kill ] || kill -KILL "$BASHPID"
					return 1
				fi
				command mv "$@"
			}
			kpanel_prepare_shared_script "$bootstrap"
		); then return 1; fi
		cmp "$bootstrap" /root/kejilion.sh
		kpanel_restore_shared_script
		cmp "$TEST_DIR/root-current" /root/kejilion.sh
		cmp "$TEST_DIR/shared-current" /usr/local/bin/k
		grep -Fx old-usr-bin-k /usr/bin/k >/dev/null
		kpanel_clear_update_files
	done
	chmod 777 /usr/local/bin/k
	if kpanel_prepare_shared_script "$bootstrap"; then return 1; fi
	chmod 755 /usr/local/bin/k
	rm /usr/local/bin/k
	ln -s "$TEST_DIR/shared-current" /usr/local/bin/k
	if kpanel_prepare_shared_script "$bootstrap"; then return 1; fi
	cmp "$TEST_DIR/shared-current" /usr/local/bin/k
	rm /usr/local/bin/k /usr/bin/k /root/kejilion.sh
	rmdir /home/docker/kpanel/bin
	printf '%s\n' 'shared_script_sync_and_rollback=pass'
}

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
	grep -Fx '首次初始化 Token： test-bootstrap-token' \
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
	grep -Fx 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_NET_ADMIN CAP_SYS_RESOURCE CAP_DAC_OVERRIDE CAP_CHOWN CAP_LINUX_IMMUTABLE CAP_SYS_PTRACE CAP_KILL' \
		/home/docker/kpanel/kejilion-agent.service >/dev/null
	grep -Fx 'AmbientCapabilities=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_NET_ADMIN CAP_SYS_RESOURCE CAP_DAC_OVERRIDE CAP_CHOWN CAP_LINUX_IMMUTABLE CAP_SYS_PTRACE CAP_KILL' \
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
	assert_expected_systemd_units
	test ! -e /etc/systemd/system/multi-user.target.wants/kejilion-agent.service
	# Migrate legacy links, including enable links canonicalized through /home/docker.
	local unit_name=""
	mkdir -p /etc/systemd/system/multi-user.target.wants /etc/systemd/system/timers.target.wants
	for unit_name in kejilion-agent.service kejilion-panel-update.service kejilion-panel-update.timer; do
		rm "/etc/systemd/system/$unit_name"
		ln -s "$(readlink -f "/home/docker/kpanel/$unit_name")" "/etc/systemd/system/$unit_name"
	done
	ln -s "$(readlink -f /home/docker/kpanel/kejilion-agent.service)" \
		/etc/systemd/system/multi-user.target.wants/kejilion-agent.service
	ln -s "$(readlink -f /home/docker/kpanel/kejilion-panel-update.timer)" \
		/etc/systemd/system/timers.target.wants/kejilion-panel-update.timer
	# Model a newer SSH-managed script and a legacy private copy. Updates and
	# A successful update must replace SSH-updated bytes with the paired release.
	printf '\n# SSH-updated shared script\n' >> /usr/local/bin/k
	local shared_digest="$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)"
	local original_links=""
	original_links="$(systemd_link_snapshot)"
	test -x /usr/local/bin/k
	test ! -e /home/docker/kpanel/bin/kejilion.sh
	test "$(stat -c '%a' /usr/local/bin/k)" = 755
	grep -Fx 'permission_granted="true"' /usr/local/bin/k >/dev/null
	grep -Fx 'ENABLE_STATS="false"' /usr/local/bin/k >/dev/null
	grep -Fx 'canshu="CN"' /usr/local/bin/k >/dev/null
	cp /usr/local/bin/k /home/docker/kpanel/bin/kejilion.sh

	if [ "${KPANEL_REQUIRE_LEGACY_MIGRATION:-0}" = 1 ] &&
		[ "${KPANEL_LEGACY_MIGRATION_TEST_COMPLETED:-0}" != 1 ]; then
		# Run the immutable old updater, not merely a new updater with an old
		# unit file. The target lifecycle is installed only when it finishes.
		(
			# shellcheck source=/dev/null
			. "$PROJECT_DIR/dist/test-fixtures/rc3-kpanel.conf"
			KPANEL_MOCK_REAL_IMAGE_IDS=1 docker_app_update >"$TEST_DIR/legacy-first-upgrade.txt"
		)
		test "$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)" = "$shared_digest"
		test -f /home/docker/kpanel/bin/kejilion.sh
		! cmp -s /home/docker/kpanel/bin/kejilion.sh /usr/local/bin/k
		cmp -s /home/docker/kpanel/bin/kpanel.conf "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
		# Exercise the real new Agent migration entry with a fixed clean PATH;
		# mock commands remain confined to the surrounding lifecycle fixture.
		[ -e /bin/bash ] || ln -s /usr/local/bin/bash /bin/bash
		"$PROJECT_DIR/dist/test-fixtures/migration-agent" host-script-migrate
		test "$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)" != "$shared_digest"
		cmp -s /root/kejilion.sh /usr/local/bin/k
		test "$(readlink /usr/bin/k)" = /usr/local/bin/k
		grep -Fx 'canshu="CN"' /usr/local/bin/k >/dev/null
		grep -Fx 'ENABLE_STATS="false"' /usr/local/bin/k >/dev/null
		grep -Fx '# KPanel shared script compatibility wrapper: v1' /home/docker/kpanel/bin/kejilion.sh >/dev/null
		test "$(stat -c '%u:%a' /home/docker/kpanel/bin/kejilion.sh)" = 0:700
		# The following ordinary update must also work and retire the wrapper.
		docker_app_update >"$TEST_DIR/after-legacy-migration.txt"
		test ! -e /home/docker/kpanel/bin/kejilion.sh
		grep -F -- '--script /usr/local/bin/k' /home/docker/kpanel/kejilion-panel-update.service >/dev/null
		printf '%s\n' 'legacy_first_upgrade_shared_script=pass'
		KPANEL_LEGACY_MIGRATION_TEST_COMPLETED=1
		# Restore the following matrix's newer-SSH/legacy-private precondition.
		printf '\n# SSH-updated shared script\n' >> /usr/local/bin/k
		cp /usr/local/bin/k /home/docker/kpanel/bin/kejilion.sh
		shared_digest="$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)"
		original_links="$(systemd_link_snapshot)"
		rm -f "$MOCK_STATE/running-id" "$MOCK_STATE/latest-id" "$MOCK_STATE/target-id" \
			"$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag"
	fi

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
	# A refreshed script must migrate the unit generated by older releases.
	sed -i 's/ CAP_KILL//g' /home/docker/kpanel/kejilion-agent.service
	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag"
	KPANEL_MOCK_REAL_IMAGE_IDS=1 KPANEL_MOCK_RETARGET_AFTER_GATE=1 docker_app_update \
		>"$TEST_DIR/retargeted-latest-output.txt"
	test "$(grep -Ec '^(CapabilityBoundingSet|AmbientCapabilities)=.* CAP_KILL$' \
		/home/docker/kpanel/kejilion-agent.service)" -eq 2
	assert_expected_systemd_units
	if [ "${KPANEL_TEST_UNIT_LAYOUT:-linked}" = copied ]; then
		test "$(readlink /etc/systemd/system/multi-user.target.wants/kejilion-agent.service)" = \
			/etc/systemd/system/kejilion-agent.service
		test "$(readlink /etc/systemd/system/timers.target.wants/kejilion-panel-update.timer)" = \
			/etc/systemd/system/kejilion-panel-update.timer
		# A missing data tree must not hide definitions for separately mounted data.
		mv /home/docker/kpanel /home/docker/kpanel-hidden
		test -f /etc/systemd/system/multi-user.target.wants/kejilion-agent.service
		test -f /etc/systemd/system/timers.target.wants/kejilion-panel-update.timer
		mv /home/docker/kpanel-hidden /home/docker/kpanel
	else
		# Root-mount users retain both unit and enable links byte-for-byte/inode.
		test "$(systemd_link_snapshot)" = "$original_links"
	fi
	grep -Fx '    image: docker.io/kjlion/kejilion-panel@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' \
		/home/docker/kpanel/docker-compose.yml >/dev/null
	test "$(cat "$MOCK_STATE/running-id")" = \
		'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
	test "$(cat "$MOCK_STATE/latest-id")" = \
		'sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc'
	rm -f "$MOCK_STATE/running-id" "$MOCK_STATE/latest-id" "$MOCK_STATE/target-id"
	docker_app_update >"$TEST_DIR/after-retarget-update-output.txt"
	grep -Fx '    image: docker.io/kjlion/kejilion-panel@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
		/home/docker/kpanel/docker-compose.yml >/dev/null

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
	test "$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)" != "$shared_digest"
	shared_digest="$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)"
	cmp /root/kejilion.sh /usr/local/bin/k
	test "$(readlink /usr/bin/k)" = /usr/local/bin/k
	test ! -e /home/docker/kpanel/bin/kejilion.sh
	test "$(/home/docker/kpanel/bin/kejilion-agent version)" = "$RELEASE_VERSION v1alpha1"
	grep -Fx 'permission_granted="true"' /usr/local/bin/k >/dev/null
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
	test "$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)" = "$shared_digest"
	test ! -e /home/docker/kpanel/bin/kejilion.sh
	grep -Fx 'KPANEL_SECURE_COOKIE=true' /home/docker/kpanel/.env >/dev/null
	grep -Fx 'KPANEL_PUBLIC_URL=https://panel.example.com' /home/docker/kpanel/.env >/dev/null
	cp -p /home/docker/kpanel/.env "$TEST_DIR/https-env"

	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag" "$MOCK_STATE/image-rm"
	sed -i 's/Description=KPanel Host Agent/Description=KPanel previous Agent/' \
		/home/docker/kpanel/kejilion-agent.service
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
	assert_expected_systemd_units
	grep -Fx 'Description=KPanel previous Agent' /etc/systemd/system/kejilion-agent.service >/dev/null

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
	grep -F 'image: docker.io/kjlion/kejilion-panel@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
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
	printf '\n# SSH version before interrupted update\n' >> /usr/local/bin/k
	cp -p /usr/local/bin/k /root/kejilion.sh
	shared_digest="$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)"
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
	test "$("$KPANEL_REAL_SHA256" /usr/local/bin/k | cut -d ' ' -f1)" = "$shared_digest"
	cmp /root/kejilion.sh /usr/local/bin/k
	test "$(readlink /usr/bin/k)" = /usr/local/bin/k
	grep -Fx 'original-panel-data' \
		/home/docker/kpanel/data/panel/rollback-marker >/dev/null
	grep -Fx 'original-agent-data' \
		/home/docker/kpanel/data/agent/rollback-marker >/dev/null
	assert_expected_systemd_units
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
	grep -F 'image: docker.io/kjlion/kejilion-panel@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
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

	# A capability-negative lifecycle models releases that only recognize links.
	local legacy_lifecycle="$TEST_DIR/legacy-kpanel.conf"
	sed '/^# KPanel systemd unit copies: v1$/d' "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf" >"$legacy_lifecycle"
	KPANEL_MOCK_LIFECYCLE_SOURCE="$legacy_lifecycle" docker_app_update >/dev/null
	assert_legacy_systemd_units
	rm -f "$MOCK_STATE/rollback-tagged" "$MOCK_STATE/image-tag"
	if KPANEL_MOCK_UPDATE_HEALTH_FAIL=1 docker_app_update >"$TEST_DIR/legacy-rollback-output.txt" 2>&1; then
		echo "failed upgrade from a legacy lifecycle unexpectedly succeeded" >&2
		return 1
	fi
	assert_legacy_systemd_units
	# Upgrading again migrates all three definitions to system-owned copies.
	docker_app_update >/dev/null
	assert_expected_systemd_units

	docker_app_uninstall
	[ ! -e /home/docker/kpanel ]
	for unit_name in kejilion-agent.service kejilion-panel-update.service kejilion-panel-update.timer; do
		test ! -e "/etc/systemd/system/$unit_name"
	done
	rm -f /etc/systemd/system/multi-user.target.wants/kejilion-agent.service \
		/etc/systemd/system/timers.target.wants/kejilion-panel-update.timer
}

assert_legacy_systemd_units() {
	local unit_name=""
	for unit_name in kejilion-agent.service kejilion-panel-update.service kejilion-panel-update.timer; do
		test -L "/etc/systemd/system/$unit_name"
		test "$(readlink -f "/etc/systemd/system/$unit_name")" = \
			"$(readlink -f "/home/docker/kpanel/$unit_name")"
	done
}

run_legacy_image_install() (
	local ipv4_address="198.51.100.25"
	local legacy_lifecycle="$TEST_DIR/legacy-install-kpanel.conf"
	docker_app_plus() { :; }
	check_docker_app_ip() { :; }
	# A new fetched installer may still install an older stable image.
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	docker_port=18080
	sed '/^# KPanel systemd unit copies: v1$/d' "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf" >"$legacy_lifecycle"
	KPANEL_MOCK_LIFECYCLE_SOURCE="$legacy_lifecycle" docker_app_install >/dev/null
	assert_legacy_systemd_units
	docker_app_update >/dev/null
	assert_expected_systemd_units
	docker_app_uninstall >/dev/null
)

assert_expected_systemd_units() {
	if [ "${KPANEL_TEST_UNIT_LAYOUT:-linked}" = linked ]; then
		assert_legacy_systemd_units
		if [ -n "${original_links:-}" ]; then
			test "$(systemd_link_snapshot)" = "$original_links"
		fi
		return
	fi
	local unit_name=""
	for unit_name in kejilion-agent.service kejilion-panel-update.service kejilion-panel-update.timer; do
		local target="/etc/systemd/system/$unit_name"
		test -f "$target" && test ! -L "$target"
		test "$(stat -c '%u:%g:%a' "$target")" = 0:0:644
		grep -Fx "# KPanel managed unit: /home/docker/kpanel/$unit_name" "$target" >/dev/null
		if [[ "$unit_name" = *.service ]]; then
			grep -Fx "RequiresMountsFor=\"$(readlink -f /home/docker/kpanel)\"" "$target" >/dev/null
			tail -n +4 "$target" | cmp -s - "/home/docker/kpanel/$unit_name"
		else
			tail -n +2 "$target" | cmp -s - "/home/docker/kpanel/$unit_name"
		fi
	done
}

run_mount_detection() (
	docker_app_plus() { :; }
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	# Exercise the real proc table before using deterministic mount fixtures.
	kpanel_path_on_root_mount /
	local KPANEL_MOCK_MOUNTINFO="$TEST_DIR/detection-mountinfo"
	export KPANEL_MOCK_MOUNTINFO
	printf '%s\n' \
		'1 0 8:1 / / rw - ext4 /dev/root rw' \
		'2 1 8:2 / /data rw - ext4 /dev/data rw' \
		'3 1 8:1 /storage /bound rw - ext4 /dev/root rw' \
		'4 1 8:1 /subvol /subvolume rw - btrfs /dev/root rw' \
		'5 1 8:2 / /data\040with\040space\134leaf%vol rw - ext4 /dev/data rw' \
		'6 1 8:2 / /tabs\011data rw - ext4 /dev/data rw' \
		>"$KPANEL_MOCK_MOUNTINFO"
	kpanel_path_on_root_mount /home/docker/kpanel
	kpanel_path_on_root_mount /database/kpanel
	local path="" status=0
	for path in /data/kpanel /bound/kpanel /subvolume/kpanel \
		'/data with space\leaf%vol/kpanel' $'/tabs\tdata/kpanel'; do
		status=0
		kpanel_path_on_root_mount "$path" || status=$?
		test "$status" = 1
	done
	: >"$KPANEL_MOCK_MOUNTINFO"
	status=0
	kpanel_path_on_root_mount /home/docker/kpanel || status=$?
	test "$status" = 2
	# Unknown detection must not migrate an existing root-mount installation.
	mkdir -p /home/docker/kpanel
	printf '[Unit]\nDescription=fixture\n' >/home/docker/kpanel/kejilion-agent.service
	ln -s /home/docker/kpanel/kejilion-agent.service /etc/systemd/system/kejilion-agent.service
	local inode="$(stat -c '%i' /etc/systemd/system/kejilion-agent.service)"
	kpanel_link_managed_unit kejilion-agent.service /home/docker/kpanel/kejilion-agent.service
	test "$(stat -c '%i' /etc/systemd/system/kejilion-agent.service)" = "$inode"
	assert_root_link() {
		test "$(readlink /etc/systemd/system/kejilion-agent.service)" = /home/docker/kpanel/kejilion-agent.service
	}
	assert_root_link
	rm /etc/systemd/system/kejilion-agent.service
	rm -rf /home/docker/kpanel
	printf '%s\n' 'mount_detection=pass'
)

systemd_link_snapshot() {
	local path=""
	for path in /etc/systemd/system/kejilion-agent.service \
		/etc/systemd/system/kejilion-panel-update.service \
		/etc/systemd/system/kejilion-panel-update.timer \
		/etc/systemd/system/multi-user.target.wants/kejilion-agent.service \
		/etc/systemd/system/timers.target.wants/kejilion-panel-update.timer; do
		printf '%s:%s:%s\n' "$path" "$(stat -c '%i' "$path")" "$(readlink "$path")"
	done
}

run_symlinked_docker_root_lifecycle() {
	local physical_docker_root="$TEST_DIR/physical docker root"

	MOCK_STATE="$TEST_DIR/symlinked-root-state"
	mkdir -p "$MOCK_STATE"
	export KPANEL_MOCK_STATE="$MOCK_STATE"
	rmdir /home/docker
	mkdir -p "$physical_docker_root"
	ln -s "$physical_docker_root" /home/docker
	test "$(readlink -f /home/docker)" = "$physical_docker_root"
	# A symlink whose destination remains on / must not trigger migration.
	run_lifecycle
	# Model a separately mounted bind/subvolume on the same device as /.
	local KPANEL_MOCK_MOUNTINFO="$TEST_DIR/separate-mountinfo"
	local KPANEL_TEST_UNIT_LAYOUT=copied
	local encoded="${physical_docker_root// /\\040}"
	printf '1 0 8:1 / / rw - ext4 /dev/root rw\n2 1 8:1 / %s rw - ext4 /dev/root rw\n' \
		"$encoded" >"$KPANEL_MOCK_MOUNTINFO"
	export KPANEL_MOCK_MOUNTINFO KPANEL_TEST_UNIT_LAYOUT
	MOCK_STATE="$TEST_DIR/separate-mount-state"
	mkdir -p "$MOCK_STATE"
	export KPANEL_MOCK_STATE="$MOCK_STATE"
	run_lifecycle
}

run_failed_install() {
	local ipv4_address="198.51.100.25"
	printf '\n# existing SSH version before failed install\n' >> /usr/local/bin/k
	cp -p /usr/local/bin/k "$TEST_DIR/before-failed-install-k"
	cp -p /root/kejilion.sh "$TEST_DIR/before-failed-install-root"

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
	cmp "$TEST_DIR/before-failed-install-k" /usr/local/bin/k
	cmp "$TEST_DIR/before-failed-install-root" /root/kejilion.sh
	test "$(readlink /usr/bin/k)" = /usr/local/bin/k
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
	# Neither installation nor cleanup may claim an unmarked regular unit.
	printf '%s\n' '# administrator service' >/etc/systemd/system/kejilion-agent.service
	: >/home/docker/kpanel/kejilion-agent.service
	if kpanel_link_managed_unit kejilion-agent.service /home/docker/kpanel/kejilion-agent.service; then
		echo "unmanaged regular unit was overwritten" >&2
		return 1
	fi
	kpanel_remove_managed_unit_link kejilion-agent.service /home/docker/kpanel/kejilion-agent.service
	grep -Fx '# administrator service' /etc/systemd/system/kejilion-agent.service >/dev/null
	rm /etc/systemd/system/kejilion-agent.service
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

run_latest_script_download() {
    local candidate_agent="$TEST_DIR/latest-agent"
    local candidate_script="$TEST_DIR/latest-script"
    local current="$TEST_DIR/current-script"
    local mode
    docker_app_plus() { :; }
    . "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
    printf '#!/bin/bash\ncanshu="V6"\npermission_granted="true"\nENABLE_STATS="false"\n# existing host\n' >"$current"
    cp "$current" /usr/local/bin/k
    cp "$current" /root/kejilion.sh
    chmod 700 /usr/local/bin/k /root/kejilion.sh
    for mode in ok fallback; do
        : >"$TEST_DIR/latest-download.log"
        KPANEL_MOCK_DOWNLOAD="$mode" KPANEL_MOCK_DOWNLOAD_LOG="$TEST_DIR/latest-download.log" \
            kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"
        grep -Fx '# latest-download-fixture' "$candidate_script"
        grep -Fx 'canshu="V6"' "$candidate_script"
        grep -Fx 'permission_granted="true"' "$candidate_script"
        grep -Fx 'ENABLE_STATS="false"' "$candidate_script"
        test "$(stat -c %a "$candidate_script")" = 700
        cmp "$current" /usr/local/bin/k
        cmp "$current" /root/kejilion.sh
        if [ "$mode" = fallback ]; then
            test "$(wc -l <"$TEST_DIR/latest-download.log")" = 2
        else test "$(wc -l <"$TEST_DIR/latest-download.log")" = 1; fi
    done
    for mode in fail empty html syntax; do
        if KPANEL_MOCK_DOWNLOAD="$mode" kpanel_extract_release mock-image "$candidate_agent" "$candidate_script"; then
            echo "invalid latest script accepted: $mode" >&2
            return 1
        fi
        test ! -e "$candidate_agent"
        test ! -e "$candidate_script"
        test -z "$(find "$TEST_DIR" -name 'latest-script.latest.*' -print)"
        cmp "$current" /usr/local/bin/k
        cmp "$current" /root/kejilion.sh
    done
    rm /usr/local/bin/k /root/kejilion.sh
    printf '%s\n' 'app_conf_latest_script_download=pass'
}

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
bash "$PROJECT_DIR/packaging/tests/app-conf-lock-compat.sh" "$PROJECT_DIR"
run_license_inheritance
run_shared_script_bootstrap
run_latest_script_download
# Exercise the successful interrupted-update branch with a legacy private copy.
# Dependencies are stubbed in a subshell; cleanup remains the real lifecycle code.
run_shared_script_interrupted_success() (
	. "$PROJECT_DIR/packaging/kejilion-app/kpanel.conf"
	export KJ_KPANEL_LIFECYCLE_LOCK_HELD=1
	mkdir -p /home/docker/kpanel/update-state/transaction /home/docker/kpanel/bin
	printf '%s\n' 'shared-script' >/usr/local/bin/k
	printf '%s\n' 'legacy-script' >/home/docker/kpanel/bin/kejilion.sh
	kpanel_detect_init_system() { :; }
	kpanel_validate_automatic_storage() { :; }
	kpanel_transaction_value() {
		case "$1" in
			phase) echo verifying ;;
			*-preexisted) echo false ;;
			*) echo fixture ;;
		esac
	}
	kpanel_automatic_target_ready() { :; }
	kpanel_complete_automatic_transaction() { :; }
	kpanel_install_pending_lifecycle() { :; }
	kpanel_clear_update_files() { :; }
	kpanel_recover_automatic_update
	test ! -e /home/docker/kpanel/bin/kejilion.sh
	grep -Fx shared-script /usr/local/bin/k >/dev/null
	rm /usr/local/bin/k
	rmdir /home/docker/kpanel/bin /home/docker/kpanel/update-state/transaction /home/docker/kpanel/update-state
	printf '%s\n' 'shared_script_interrupted_success=pass'
)
run_shared_script_interrupted_success
run_mount_detection
run_lifecycle
run_symlinked_docker_root_lifecycle
run_legacy_image_install
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
if [ -L /home/docker ] && [ "$(readlink /home/docker)" = "$TEST_DIR/physical docker root" ]; then
	rm /home/docker
	mkdir /home/docker
fi
cleanup
bash "$PROJECT_DIR/packaging/tests/app-conf-update-backups.sh" "$PROJECT_DIR"
