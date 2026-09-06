#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
RELEASE_VERSION=$(tr -d '\r\n' <"$PROJECT_DIR/VERSION")
export KP_AGENT_VERSION="${KP_AGENT_VERSION:-$RELEASE_VERSION}"
FAKE_BIN=$SCRIPT_DIR/fake-bin
TEST_DIR=$(mktemp -d /tmp/kejilion-panel-install-test.XXXXXX)
TEST_BIN=$TEST_DIR/fake-bin
mkdir -p "$TEST_BIN"
for command_name in curl groupadd openssl; do
	printf '%s\n' \
		'#!/bin/sh' \
		"echo \"install-safety: unexpected $command_name invocation\" >&2" \
		'exit 97' >"$TEST_BIN/$command_name"
	chmod 0700 "$TEST_BIN/$command_name"
done
REAL_SHA256SUM=$(command -v sha256sum)
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "${1:-}" = "--check" ] && [ "${2:-}" = "--status" ]; then' \
	'  shift 2' \
	"  exec \"$REAL_SHA256SUM\" --check --status \"\$@\"" \
	'fi' \
	"exec \"$REAL_SHA256SUM\" \"\$@\"" >"$TEST_BIN/sha256sum"
chmod 0700 "$TEST_BIN/sha256sum"

cleanup() {
	case "$TEST_DIR" in
		/tmp/kejilion-panel-install-test.*)
			rm -rf -- "$TEST_DIR"
			;;
		*)
			echo "refusing to remove unexpected test directory: $TEST_DIR" >&2
			;;
	esac
}
trap cleanup EXIT HUP INT TERM

DOCKER_LOG=$TEST_DIR/docker.log
: >"$DOCKER_LOG"
AGENT_BINARY=$FAKE_BIN/agent
AGENT_SHA=$(sha256sum "$AGENT_BINARY" | awk '{print $1}')
IMAGE=docker.io/example/kejilion-panel@sha256:0000000000000000000000000000000000000000000000000000000000000000

run_installer() {
	PATH="$TEST_BIN:$FAKE_BIN:$PATH" \
	KP_DOCKER_LOG="$DOCKER_LOG" \
	KP_AGENT_VERSION="${KP_AGENT_VERSION:-$RELEASE_VERSION}" \
	sh "$PROJECT_DIR/deploy/install.sh" \
		--agent-binary "$AGENT_BINARY" \
		--agent-sha256 "$AGENT_SHA" \
		--image "$IMAGE" \
		--public-url https://panel.example.com \
		--network-subnet 172.29.255.240/28 \
		--dry-run
}

run_installer >"$TEST_DIR/success.out"
grep -F 'Docker daemon was not queried and no host state was changed.' "$TEST_DIR/success.out" >/dev/null
grep -F 'Private Panel endpoint: http://172.29.255.242:8080' "$TEST_DIR/success.out" >/dev/null
test "$(wc -l <"$DOCKER_LOG" | tr -d ' ')" = 1
grep -Fx -- '--host unix:///var/run/docker.sock compose version' "$DOCKER_LOG" >/dev/null

# Exercise the real checksum implementation selected by this environment.
if (AGENT_SHA=0000000000000000000000000000000000000000000000000000000000000000; run_installer) >"$TEST_DIR/checksum-mismatch.out" 2>&1; then
	echo "installer accepted an incorrect Agent checksum" >&2
	exit 1
fi
grep -F 'agent binary SHA-256 mismatch' "$TEST_DIR/checksum-mismatch.out" >/dev/null
test "$(wc -l <"$DOCKER_LOG" | tr -d ' ')" = 2

if (AGENT_SHA=invalid-checksum; run_installer) >"$TEST_DIR/checksum-invalid.out" 2>&1; then
	echo "installer accepted a malformed Agent checksum" >&2
	exit 1
fi
grep -F 'invalid agent SHA-256' "$TEST_DIR/checksum-invalid.out" >/dev/null

if (AGENT_BINARY="$TEST_DIR/missing-agent"; run_installer) >"$TEST_DIR/checksum-missing.out" 2>&1; then
	echo "installer accepted a missing Agent binary" >&2
	exit 1
fi
grep -F 'agent binary not found' "$TEST_DIR/checksum-missing.out" >/dev/null
test "$(wc -l <"$DOCKER_LOG" | tr -d ' ')" = 2

BEFORE_LINES=$(wc -l <"$DOCKER_LOG" | tr -d ' ')
if DOCKER_HOST=tcp://198.51.100.1:2375 run_installer >"$TEST_DIR/remote.out" 2>&1; then
	echo "installer accepted DOCKER_HOST during dry-run" >&2
	exit 1
fi
test "$(wc -l <"$DOCKER_LOG" | tr -d ' ')" = "$BEFORE_LINES"
grep -F 'DOCKER_HOST and DOCKER_CONTEXT must be unset' "$TEST_DIR/remote.out" >/dev/null

if KP_AGENT_VERSION=0.0.1 run_installer >"$TEST_DIR/version.out" 2>&1; then
	echo "installer accepted a mismatched Agent version" >&2
	exit 1
fi
grep -F "does not match $RELEASE_VERSION v1alpha1" "$TEST_DIR/version.out" >/dev/null

if PATH="$TEST_BIN:$FAKE_BIN:$PATH" KP_DOCKER_LOG="$DOCKER_LOG" \
	sh "$PROJECT_DIR/deploy/install.sh" \
		--agent-binary "$AGENT_BINARY" \
		--agent-sha256 "$AGENT_SHA" \
		--image "$IMAGE" \
		--public-url https://panel.example.com \
		--network-subnet 172.29.255.241/28 \
		--dry-run >"$TEST_DIR/subnet.out" 2>&1; then
	echo "installer accepted a misaligned subnet" >&2
	exit 1
fi
grep -F 'aligned RFC1918 IPv4 /28' "$TEST_DIR/subnet.out" >/dev/null

if PATH="$TEST_BIN:$FAKE_BIN:$PATH" KP_DOCKER_LOG="$DOCKER_LOG" KP_ROUTE_CONFLICT=1 \
	sh "$PROJECT_DIR/deploy/install.sh" \
		--agent-binary "$AGENT_BINARY" \
		--agent-sha256 "$AGENT_SHA" \
		--image "$IMAGE" \
		--public-url https://panel.example.com \
		--network-subnet 172.29.255.240/28 \
		--dry-run >"$TEST_DIR/route.out" 2>&1; then
	echo "installer accepted an overlapping route" >&2
	exit 1
fi
grep -F 'overlaps an existing host route' "$TEST_DIR/route.out" >/dev/null

if KP_GROUP_MEMBER=1 run_installer >"$TEST_DIR/install-group.out" 2>&1; then
	echo "installer accepted an Agent group with supplemental members" >&2
	exit 1
fi
grep -F 'group has supplemental members' "$TEST_DIR/install-group.out" >/dev/null

if KP_GROUP_EXISTS=1 KP_PRIMARY_USER=1 \
	run_installer >"$TEST_DIR/install-primary-group.out" 2>&1; then
	echo "installer accepted an Agent group used as a primary group" >&2
	exit 1
fi
grep -F 'gid is a primary group for host users' "$TEST_DIR/install-primary-group.out" >/dev/null

if KP_GROUP_EXISTS=1 KP_PASSWD_FAIL=1 \
	run_installer >"$TEST_DIR/install-passwd.out" 2>&1; then
	echo "installer accepted incomplete host user enumeration" >&2
	exit 1
fi
grep -F 'cannot enumerate host users' "$TEST_DIR/install-passwd.out" >/dev/null

if KP_UNIT_LOAD_STATE=loaded \
	run_installer >"$TEST_DIR/install-unit.out" 2>&1; then
	echo "installer accepted an already loaded Agent unit" >&2
	exit 1
fi
grep -F 'existing or loaded kejilion-agent.service' "$TEST_DIR/install-unit.out" >/dev/null

KP_WEB_ROOT_FAIL=1 \
	run_installer >"$TEST_DIR/install-web-root-missing.out" 2>&1
grep -F 'website management disabled' "$TEST_DIR/install-web-root-missing.out" >/dev/null
grep -F 'Docker daemon was not queried and no host state was changed.' \
	"$TEST_DIR/install-web-root-missing.out" >/dev/null

KP_WEB_ROOT_KIND='regular file' \
	run_installer >"$TEST_DIR/install-web-root-file.out" 2>&1
grep -F 'website management disabled' "$TEST_DIR/install-web-root-file.out" >/dev/null

if KP_DOCKER_SOCKET_FAIL=1 \
	run_installer >"$TEST_DIR/install-docker-socket-missing.out" 2>&1; then
	echo "installer accepted a missing local Docker socket" >&2
	exit 1
fi
grep -F 'local Docker Unix socket is unavailable' "$TEST_DIR/install-docker-socket-missing.out" >/dev/null

if KP_DOCKER_SOCKET_KIND='regular file' \
	run_installer >"$TEST_DIR/install-docker-socket-file.out" 2>&1; then
	echo "installer accepted a non-socket local Docker endpoint" >&2
	exit 1
fi
grep -F 'local Docker endpoint is not a Unix socket' "$TEST_DIR/install-docker-socket-file.out" >/dev/null

grep -F 'trap cleanup EXIT' "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F "trap 'exit 129' HUP" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F "trap 'exit 130' INT" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F "trap 'exit 143' TERM" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F '/run/kejilion-panel \' "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F '/run/kejilion-panel \' "$PROJECT_DIR/deploy/preflight.sh" >/dev/null
grep -F 'CRITICAL: automatic failure cleanup could not be fully verified.' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -Fx 'ProtectProc=default' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
grep -Fx 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_NET_ADMIN CAP_SYS_RESOURCE CAP_DAC_OVERRIDE CAP_CHOWN CAP_LINUX_IMMUTABLE CAP_SYS_PTRACE' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
grep -Fx 'AmbientCapabilities=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_NET_ADMIN CAP_SYS_RESOURCE CAP_DAC_OVERRIDE CAP_CHOWN CAP_LINUX_IMMUTABLE CAP_SYS_PTRACE' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
grep -Fx 'RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
if grep -Eq '^ProtectProc=(invisible|ptraceable|noaccess)$' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service"; then
	echo "Agent unit hides the dockerd process required by the socket activation guard" >&2
	exit 1
fi
if grep -Eq '^StateDirectory=kejilion-panel(/.*)?$' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service"; then
	echo "Agent unit can take ownership of the Panel container data tree" >&2
	exit 1
fi
grep -Fx 'ReadOnlyPaths=/var/lib/kejilion-panel/panel' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
grep -Fx 'ProtectHome=false' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
grep -Fx 'ProtectSystem=false' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null
if grep -Fx 'ProtectSystem=strict' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null; then
	echo "Agent unit remounts the file manager root read-only" >&2
	exit 1
fi
if grep -Fx 'ProtectHome=read-only' \
	"$PROJECT_DIR/deploy/systemd/kejilion-agent.service" >/dev/null; then
	echo "Agent unit makes the file manager root read-only" >&2
	exit 1
fi
if grep -q '^ReadWritePaths=' "$PROJECT_DIR/deploy/systemd/kejilion-agent.service"; then
	echo "Agent unit relies on ineffective root write exceptions" >&2
	exit 1
fi
grep -F 'SYSTEM_STATE_DIR=/var/lib/kejilion-panel/system' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'WORDPRESS_STATE_DIR=/var/lib/kejilion-panel/wordpress-jobs' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'APP_STATE_DIR=/var/lib/kejilion-panel/app-jobs' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'DIAGNOSTIC_STATE_DIR=/var/lib/kejilion-panel/diagnostic-jobs' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'SITE_ICON_STATE_DIR=/var/lib/kejilion-panel/site-icons' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'MONITORING_STATE_DIR=/var/lib/kejilion-panel/monitoring' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'install -d -o root -g root -m 0700 "$SITE_ICON_STATE_DIR"' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'install -d -o root -g root -m 0700 "$MONITORING_STATE_DIR"' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'expected 65532:65532:700' "$PROJECT_DIR/deploy/install.sh" >/dev/null
test "$(grep -c '^assert_panel_data_dir \"after ' "$PROJECT_DIR/deploy/install.sh")" = 3
AGENT_DATA_GATE_LINE=$(grep -n '^assert_panel_data_dir "after Agent start"$' \
	"$PROJECT_DIR/deploy/install.sh" | cut -d: -f1)
PANEL_START_ATTEMPT_LINE=$(grep -n '^PANEL_START_ATTEMPTED=true$' \
	"$PROJECT_DIR/deploy/install.sh" | cut -d: -f1)
[ "$AGENT_DATA_GATE_LINE" -lt "$PANEL_START_ATTEMPT_LINE" ] || {
	echo "Panel can start before the post-Agent data ownership gate" >&2
	exit 1
}
grep -F 'internal: true' "$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
test "$(grep -c 'internal: true' "$PROJECT_DIR/deploy/compose/compose.yml")" = 1
grep -F 'panel-egress:' "$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
grep -F 'name: kejilion-panel-egress' "$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
grep -F 'ipv4_address: ${KEJILION_PANEL_IPV4:' \
	"$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
grep -F 'gateway: ${KEJILION_PANEL_GATEWAY:' \
	"$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
if grep -Eq '^[[:space:]]*ports:' "$PROJECT_DIR/deploy/compose/compose.yml"; then
	echo "internal Panel network still declares an unreachable host port" >&2
	exit 1
fi
grep -F 'ports:' "$PROJECT_DIR/deploy/compose/direct-port.yml" >/dev/null
grep -F 'panel-egress: {}' "$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
grep -F 'name: kejilion-panel-egress' "$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
grep -F 'host.docker.internal:host-gateway' "$PROJECT_DIR/deploy/compose/compose.yml" >/dev/null
if grep -F 'internal: false' "$PROJECT_DIR/deploy/compose/direct-port.yml" >/dev/null; then
	echo "direct-port overlay weakens the internal Panel network" >&2
	exit 1
fi
grep -F "{{len .HostConfig.PortBindings}}" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F "{{(index .IPAM.Config 0).Subnet}}" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F "'{{.Internal}}' kejilion-panel-egress" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F '"kejilion-panel-egress"}}{{.IPAddress}}' "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F 'KEJILION_PANEL_CLUSTER_PRIVATE_CIDRS=' "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F "{{.State.Health.Status}}" "$PROJECT_DIR/deploy/install.sh" >/dev/null
grep -F '"http://$PANEL_IPV4:8080/api/v1/health"' \
	"$PROJECT_DIR/deploy/install.sh" >/dev/null
if grep -R -F '127.0.0.1:18443' \
	"$PROJECT_DIR/deploy/install.sh" \
	"$PROJECT_DIR/deploy/preflight.sh" \
	"$PROJECT_DIR/deploy/compose" >/dev/null; then
	echo "deployment still depends on the unreachable loopback publication" >&2
	exit 1
fi

: >"$DOCKER_LOG"
PATH="$TEST_BIN:$FAKE_BIN:$PATH" KP_DOCKER_LOG="$DOCKER_LOG" KP_WEB_ROOT_FAIL=1 \
	sh "$PROJECT_DIR/deploy/preflight.sh" \
		--public-url https://panel.example.com \
		--network-subnet 172.29.255.240/28 \
		>"$TEST_DIR/preflight.out" 2>&1 || true
test "$(wc -l <"$DOCKER_LOG" | tr -d ' ')" = 1
grep -Fx -- '--host unix:///var/run/docker.sock compose version' "$DOCKER_LOG" >/dev/null
grep -F 'private Panel endpoint is reserved: http://172.29.255.242:8080' \
	"$TEST_DIR/preflight.out" >/dev/null
grep -F 'website management will remain disabled until /home/web is initialized' \
	"$TEST_DIR/preflight.out" >/dev/null

: >"$DOCKER_LOG"
PATH="$TEST_BIN:$FAKE_BIN:$PATH" KP_DOCKER_LOG="$DOCKER_LOG" KP_GROUP_MEMBER=1 \
	sh "$PROJECT_DIR/deploy/preflight.sh" \
		--public-url https://panel.example.com \
		--network-subnet 172.29.255.240/28 \
		>"$TEST_DIR/group.out" 2>&1 || true
grep -F 'kejilion-panel group has supplemental members' "$TEST_DIR/group.out" >/dev/null

echo "install_safety=pass"
