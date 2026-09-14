#!/bin/sh
set -eu

LC_ALL=C
export LC_ALL

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/init-system.sh"

PUBLIC_URL=
NETWORK_SUBNET=172.29.255.240/28
FAILURES=0
WARNINGS=0

usage() {
	cat <<'EOF'
Usage:
  ./deploy/preflight.sh \
    --public-url https://panel.example.com \
    [--network-subnet 172.29.255.240/28]

This command is read-only. It does not connect to the Docker socket, start a
service, create a directory, or change kejilion.sh and /home/web.
EOF
}

ok() {
	printf '[OK]   %s\n' "$*"
}

warn() {
	WARNINGS=$((WARNINGS + 1))
	printf '[WARN] %s\n' "$*" >&2
}

fail() {
	FAILURES=$((FAILURES + 1))
	printf '[FAIL] %s\n' "$*" >&2
}

validate_private_subnet() {
	subnet=$1
	case "$subnet" in
		*/*)
			address=${subnet%/*}
			prefix=${subnet#*/}
			;;
		*)
			return 1
			;;
	esac
	[ "$prefix" = 28 ] || return 1
	printf '%s' "$address" |
		grep -Eq '^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$' ||
		return 1
	old_ifs=$IFS
	IFS=.
	set -- $address
	IFS=$old_ifs
	[ "$#" -eq 4 ] || return 1
	for octet in "$@"; do
		case "$octet" in
			0|[1-9]|[1-9][0-9]|[1-9][0-9][0-9]) ;;
			*) return 1 ;;
		esac
		[ "$octet" -le 255 ] || return 1
	done
	first=$1
	second=$2
	fourth=$4
	case "$first:$second" in
		10:*) ;;
		172:*)
			[ "$second" -ge 16 ] && [ "$second" -le 31 ] || return 1
			;;
		192:168) ;;
		*) return 1 ;;
	esac
	[ $((fourth % 16)) -eq 0 ] || return 1
}

derive_network_addresses() {
	address=${NETWORK_SUBNET%/*}
	prefix=${address%.*}
	base=${address##*.}
	PANEL_GATEWAY=$prefix.$((base + 1))
	PANEL_IPV4=$prefix.$((base + 2))
}

network_routes() {
	matched_routes=$(ip -4 route show match "$NETWORK_SUBNET" 2>/dev/null) || return 1
	rooted_routes=$(ip -4 route show root "$NETWORK_SUBNET" 2>/dev/null) || return 1
	printf '%s\n%s\n' "$matched_routes" "$rooted_routes" |
		while IFS= read -r route; do
			case "$route" in
				''|default\ *) ;;
				*) printf '%s\n' "$route" ;;
			esac
		done
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--public-url)
			[ "$#" -ge 2 ] || {
				fail "--public-url requires a value"
				break
			}
			PUBLIC_URL=$2
			shift 2
			;;
		--network-subnet)
			[ "$#" -ge 2 ] || {
				fail "--network-subnet requires a value"
				break
			}
			NETWORK_SUBNET=$2
			shift 2
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			fail "unknown argument: $1"
			shift
			;;
	esac
done

if [ "$(uname -s 2>/dev/null || true)" = Linux ]; then
	ok "Linux host detected"
else
	fail "production deployment requires Linux"
fi

if [ "$(id -u)" -eq 0 ]; then
	ok "running as root"
else
	warn "run the installer with sudo after this preflight"
fi

if printf '%s' "$PUBLIC_URL" |
	grep -Eq '^https://([A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?)(:[0-9]{1,5})?$'; then
	ok "public URL is an HTTPS origin: $PUBLIC_URL"
else
	fail "--public-url must be an HTTPS origin without a path, query, or fragment"
fi

if validate_private_subnet "$NETWORK_SUBNET"; then
	ok "private Docker subnet is valid: $NETWORK_SUBNET"
	derive_network_addresses
	ok "private Panel endpoint is reserved: http://$PANEL_IPV4:8080 (gateway $PANEL_GATEWAY)"
else
	fail "--network-subnet must be an aligned RFC1918 IPv4 /28"
fi

for command_name in \
	awk cat curl dirname docker getent grep id install ip mkdir mktemp \
	openssl rm rmdir sed sha256sum sleep stat tr; do
	if command -v "$command_name" >/dev/null 2>&1; then
		ok "command available: $command_name"
	else
		fail "required command not found: $command_name"
	fi
done

if command -v groupadd >/dev/null 2>&1; then
	ok "system group tool available: groupadd"
elif command -v addgroup >/dev/null 2>&1; then
	ok "system group tool available: addgroup"
else
	fail "required system group tool not found: groupadd or addgroup"
fi

if kpanel_detect_init_system; then
	ok "init system supported: $KPANEL_INIT_SYSTEM"
	case "$KPANEL_INIT_SYSTEM" in
		systemd)
			for command_name in systemctl systemd-analyze; do
				command -v "$command_name" >/dev/null 2>&1 && ok "command available: $command_name" ||
					fail "required command not found: $command_name"
			done
			;;
		openrc)
			for command_name in rc-service rc-update start-stop-daemon supervise-daemon; do
				command -v "$command_name" >/dev/null 2>&1 && ok "command available: $command_name" ||
					fail "required command not found: $command_name"
			done
			;;
	esac
else
	fail "systemd or OpenRC service management is required"
fi

if command -v docker >/dev/null 2>&1; then
	if docker --host unix:///var/run/docker.sock compose version >/dev/null 2>&1; then
		ok "Docker Compose v2 is available"
	else
		fail "Docker Compose v2 is required"
	fi
fi

# Deliberately do not call docker info or open docker.sock here: a read-only
# preflight must not socket-activate a stopped Docker daemon.
if [ -S /var/run/docker.sock ]; then
	ok "Docker Unix socket exists"
else
	fail "Docker Unix socket is absent"
fi
if [ -n "${KPANEL_INIT_SYSTEM:-}" ] && kpanel_service_active docker.service; then
	ok "Docker service is already active"
else
	fail "Docker service is not active; assess existing containers before starting it manually"
fi

if validate_private_subnet "$NETWORK_SUBNET" && command -v ip >/dev/null 2>&1; then
	if NETWORK_ROUTES=$(network_routes); then
		if [ -z "$NETWORK_ROUTES" ]; then
			ok "Docker subnet does not overlap an existing host route"
		else
			fail "Docker subnet overlaps an existing host route: $NETWORK_ROUTES"
		fi
	else
		fail "cannot inspect host routes for Docker subnet safety"
	fi
fi

WEB_ROOT_KIND=$(stat -L -c %F /home/web 2>/dev/null || true)
if [ "$WEB_ROOT_KIND" = "directory" ] && [ -r /home/web ]; then
	ok "Kejilion Web root is available: /home/web"
	for web_path in /home/web/conf.d /home/web/html /home/web/certs; do
		if [ -d "$web_path" ] && [ -r "$web_path" ]; then
			ok "Kejilion path is readable: $web_path"
		else
			warn "Kejilion path is unavailable: $web_path"
		fi
	done
else
	warn "Kejilion Web root is unavailable; website management will remain disabled until /home/web is initialized"
fi

for managed_path in \
	/etc/kejilion-panel \
	/opt/kejilion-panel \
	/var/lib/kejilion-panel \
	/run/kejilion-panel \
	/usr/local/libexec/kejilion-agent \
	/etc/init.d/kejilion-agent \
	/etc/conf.d/kejilion-agent \
	/etc/runlevels/default/kejilion-agent \
	/etc/systemd/system/kejilion-agent.service \
	/etc/systemd/system/kejilion-agent.service.d \
	/run/systemd/system/kejilion-agent.service \
	/run/systemd/system/kejilion-agent.service.d \
	/usr/lib/systemd/system/kejilion-agent.service \
	/usr/lib/systemd/system/kejilion-agent.service.d \
	/lib/systemd/system/kejilion-agent.service \
	/lib/systemd/system/kejilion-agent.service.d; do
	if [ -e "$managed_path" ] || [ -L "$managed_path" ]; then
		fail "existing Panel resource found; the v0.1 installer only supports a fresh install: $managed_path"
	fi
done

if [ "${KPANEL_INIT_SYSTEM:-}" = systemd ]; then
	if UNIT_LOAD_STATE=$(systemctl show \
		--property=LoadState --value kejilion-agent.service 2>/dev/null) &&
		UNIT_FRAGMENT_PATH=$(systemctl show \
			--property=FragmentPath --value kejilion-agent.service 2>/dev/null) &&
		UNIT_DROP_INS=$(systemctl show \
			--property=DropInPaths --value kejilion-agent.service 2>/dev/null); then
		if [ "$UNIT_LOAD_STATE" = "not-found" ] &&
			[ -z "$UNIT_FRAGMENT_PATH" ] &&
			[ -z "$UNIT_DROP_INS" ]; then
			ok "no existing or loaded kejilion-agent.service was found"
		else
			fail "an existing or loaded kejilion-agent.service was found"
		fi
	else
		fail "cannot query systemd for an existing kejilion-agent.service"
	fi
elif [ "${KPANEL_INIT_SYSTEM:-}" = openrc ]; then
	if [ ! -e /etc/init.d/kejilion-agent ] && [ ! -L /etc/init.d/kejilion-agent ]; then
		ok "no existing OpenRC kejilion-agent service was found"
	else
		fail "an existing OpenRC kejilion-agent service was found"
	fi
fi

if getent group kejilion-panel >/dev/null 2>&1; then
	PANEL_GROUP_ENTRY=$(getent group kejilion-panel)
	PANEL_GID=$(printf '%s\n' "$PANEL_GROUP_ENTRY" | awk -F: '$1 == "kejilion-panel" {print $3; exit}')
	PANEL_MEMBERS=$(printf '%s\n' "$PANEL_GROUP_ENTRY" | awk -F: '$1 == "kejilion-panel" {print $4; exit}')
	if [ -z "$PANEL_GID" ]; then
		fail "cannot resolve kejilion-panel gid"
	elif [ -n "$PANEL_MEMBERS" ]; then
		fail "kejilion-panel group has supplemental members"
	else
		if ! PASSWD_ENTRIES=$(getent passwd) || [ -z "$PASSWD_ENTRIES" ]; then
			fail "cannot enumerate host users for the Agent group boundary"
		else
			PRIMARY_GROUP_USERS=$(printf '%s\n' "$PASSWD_ENTRIES" |
				awk -F: -v gid="$PANEL_GID" '$4 == gid {print $1}')
			if [ -n "$PRIMARY_GROUP_USERS" ]; then
				fail "kejilion-panel gid is a primary group for host users: $PRIMARY_GROUP_USERS"
			else
				ok "existing kejilion-panel group has no host members"
			fi
		fi
	fi
fi

if [ "$FAILURES" -ne 0 ]; then
	printf '\nPreflight failed: %s failure(s), %s warning(s).\n' "$FAILURES" "$WARNINGS" >&2
	exit 1
fi

printf '\nPreflight passed with %s warning(s). No host state was changed.\n' "$WARNINGS"
