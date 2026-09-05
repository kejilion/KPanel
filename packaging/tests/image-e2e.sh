#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
EXPECTED_VERSION=${KPANEL_EXPECTED_VERSION:-$(tr -d '\r\n' <"$SCRIPT_DIR/../../VERSION")}
IMAGE=${1:?usage: image-e2e.sh IMAGE [PORT]}
PORT=${2:-18080}
SUFFIX=$$
CONTAINER="kpanel-image-e2e-$SUFFIX"
NETWORK="kpanel-image-e2e-$SUFFIX"
TEST_DIR=$(mktemp -d "/tmp/kpanel-image-e2e.$SUFFIX.XXXXXX")

cleanup() {
	docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	docker network rm "$NETWORK" >/dev/null 2>&1 || true
	case "$TEST_DIR" in
		/tmp/kpanel-image-e2e.*)
			rm -rf -- "$TEST_DIR"
			;;
	esac
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$TEST_DIR/data" "$TEST_DIR/run"
printf '%064d\n' 0 >"$TEST_DIR/agent.token"
chown -R 65532:65532 "$TEST_DIR/data"
chmod 700 "$TEST_DIR/data"
chmod 755 "$TEST_DIR/run"
chmod 444 "$TEST_DIR/agent.token"

docker network create "$NETWORK" >/dev/null
NETWORK_SUBNET=$(docker network inspect \
	--format '{{(index .IPAM.Config 0).Subnet}}' "$NETWORK")

docker run -d \
	--name "$CONTAINER" \
	--network "$NETWORK" \
	--read-only \
	--cap-drop ALL \
	--security-opt no-new-privileges:true \
	--tmpfs /tmp:size=16m,mode=1777 \
	-p "127.0.0.1:$PORT:8080" \
	-e KEJILION_PANEL_PUBLIC_URL="http://127.0.0.1:$PORT" \
	-e KEJILION_PANEL_SECURE_COOKIE=false \
	-e KEJILION_PANEL_TRUSTED_PROXY_CIDRS="127.0.0.0/8,::1/128,$NETWORK_SUBNET" \
	-v "$TEST_DIR/data:/var/lib/kejilion-panel" \
	-v "$TEST_DIR/run:/run/kejilion-panel:ro" \
	-v "$TEST_DIR/agent.token:/run/secrets/agent-token:ro" \
	"$IMAGE" >/dev/null

attempt=0
until curl --noproxy '*' -fsS "http://127.0.0.1:$PORT/api/v1/health" >"$TEST_DIR/health.json" 2>/dev/null; do
	attempt=$((attempt + 1))
	[ "$attempt" -lt 30 ] || {
		docker logs "$CONTAINER"
		exit 1
	}
	sleep 1
done
grep -F "\"version\":\"$EXPECTED_VERSION\"" "$TEST_DIR/health.json" >/dev/null

test "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' \
	-H "Host: 127.0.0.1:$PORT" "http://127.0.0.1:$PORT/")" = 200
test "$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' \
	-H 'Host: panel.e2e.invalid' \
	-H 'X-Forwarded-Proto: https' \
	"http://127.0.0.1:$PORT/")" = 200

# HTML alone does not exercise public files copied from a restrictive checkout.
# Read real image bytes through the unprivileged Panel, not docker cp as root.
for asset in desktop-icons/files-kpanel-flat-v1.webp wallpapers/kpanel-desktop.webp; do
	curl --noproxy '*' -fsS --max-time 15 "http://127.0.0.1:$PORT/$asset" >"$TEST_DIR/public-asset"
	cmp "$SCRIPT_DIR/../../web/public/$asset" "$TEST_DIR/public-asset"
done

BOOTSTRAP_TOKEN=$(tr -d '\r\n' <"$TEST_DIR/data/bootstrap.token")
curl --noproxy '*' -sS -D "$TEST_DIR/bootstrap.headers" -o "$TEST_DIR/bootstrap.json" \
	-H 'Host: panel.e2e.invalid' \
	-H 'X-Forwarded-Proto: https' \
	-H 'Origin: https://panel.e2e.invalid' \
	-H 'Content-Type: application/json' \
	--data "{\"token\":\"$BOOTSTRAP_TOKEN\",\"username\":\"admin\",\"password\":\"e2e-strong-password\"}" \
	"http://127.0.0.1:$PORT/api/v1/auth/bootstrap"
grep -F 'HTTP/1.1 201 Created' "$TEST_DIR/bootstrap.headers" >/dev/null || {
	cat "$TEST_DIR/bootstrap.headers" >&2
	head -c 2048 "$TEST_DIR/bootstrap.json" >&2
	exit 1
}
test "$(grep -ic '^Set-Cookie: .*; Secure;' "$TEST_DIR/bootstrap.headers")" = 2

test "$(docker inspect --format '{{len .NetworkSettings.Networks}}' "$CONTAINER")" = 1
attempt=0
until [ "$(docker inspect --format '{{.State.Health.Status}}' "$CONTAINER")" = healthy ]; do
	attempt=$((attempt + 1))
	[ "$attempt" -lt 45 ] || exit 1
	sleep 1
done

printf '%s\n' "image_e2e=pass"
