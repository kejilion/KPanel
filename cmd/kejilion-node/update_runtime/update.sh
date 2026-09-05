#!/bin/bash
set -euo pipefail

mode="${1:-update}"
case "$mode" in
	install|update) ;;
	*) echo "unsupported update mode" >&2; exit 2 ;;
esac
case "$(uname -m)" in
	x86_64|amd64) arch="amd64" ;;
	aarch64|arm64) arch="arm64" ;;
	*) echo "unsupported CPU architecture" >&2; exit 1 ;;
esac

home_dir="/usr/local/lib/kejilion-node"
binary_name="kejilion-node-linux-${arch}"
binary_path="${home_dir}/kejilion-node"
base_url="https://github.com/kejilion/KPanel/releases/latest/download"

# Unlike a mkdir lock, the kernel releases this lock even after SIGKILL.
exec 9>"${home_dir}/update.lock"
if ! flock -n 9; then
	echo "another KPanel lightweight node update is running" >&2
	exit 1
fi
# During the one-time handoff, an old updater still holds its mkdir lock and
# continues executing the old script. Do not race its binary/rollback writes.
if [ -d /run/lock/kejilion-node-update.lock ] && [ ! -f "${home_dir}/legacy-update.pid" ]; then
	echo "legacy KPanel update lock exists; finish or inspect the previous update first" >&2
	exit 1
fi
if [ -f "${home_dir}/legacy-update.pid" ]; then
	read -r legacy_pid legacy_start <"${home_dir}/legacy-update.pid"
	if [[ "$legacy_pid" =~ ^[1-9][0-9]*$ ]] && [[ "$legacy_start" =~ ^[0-9]+$ ]] &&
		[ "$(awk '{ sub(/^.*\) /, ""); print $20 }' "/proc/${legacy_pid}/stat" 2>/dev/null || true)" = "$legacy_start" ]; then
		echo "previous KPanel lightweight node updater is still finishing" >&2
		exit 1
	fi
fi
temporary_dir=""
cleanup() {
	[ -z "$temporary_dir" ] || rm -rf -- "$temporary_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
temporary_dir="$(mktemp -d /tmp/kejilion-node-update.XXXXXX)"

curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location --silent --show-error \
	--connect-timeout 15 --max-time 60 --retry 3 --retry-delay 5 --retry-max-time 240 \
	--max-filesize 65536 --dump-header "${temporary_dir}/headers" \
	-o "${temporary_dir}/SHA256SUMS" "${base_url}/SHA256SUMS"
expected="$(awk -v name="$binary_name" '$2 == name { print $1 }' "${temporary_dir}/SHA256SUMS")"
printf '%s' "$expected" | grep -Eq '^[0-9a-f]{64}$' || {
	echo "release checksum is unavailable" >&2
	exit 1
}
# The first redirect binds the manifest to a release; the following CDN redirect
# must never be used as a base URL or mixed with a later value of latest.
release_url="$(awk 'tolower($1) == "location:" { sub(/\r$/, "", $2); print $2 }' "${temporary_dir}/headers" |
	grep -E '^https://github.com/kejilion/KPanel/releases/download/v[0-9]+\.[0-9]+\.[0-9]+/SHA256SUMS$' | tail -n 1)"
[ -n "$release_url" ] || { echo "release manifest redirect is invalid" >&2; exit 1; }
release_base="${release_url%/SHA256SUMS}"

file_service="kejilion-node-file.service"
file_service_path="/etc/systemd/system/${file_service}"
file_service_unit_changed=false

ensure_file_service_unit() {
	if [ -e "$file_service_path" ] || [ -L "$file_service_path" ]; then
		[ -f "$file_service_path" ] && [ ! -L "$file_service_path" ] || return 1
		[ "$(stat -c '%u' "$file_service_path")" = "0" ] || return 1
		[ $(( 8#$(stat -c '%a' "$file_service_path") & 8#022 )) -eq 0 ] || return 1
	fi
	local template="${temporary_dir}/file.service" legacy_template="${temporary_dir}/file.legacy.service" unit_temporary
	cat >"$template" <<'KPANEL_NODE_FILE_SERVICE'
[Unit]
Description=KPanel Lightweight Node File Manager
After=network-online.target
Wants=network-online.target
ConditionPathExists=/etc/kejilion-node/node.json

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/
ExecStart=/usr/local/lib/kejilion-node/kejilion-node file-broker --config /etc/kejilion-node/node.json --terminal-config /etc/kejilion-node/terminal.json
Restart=on-failure
RestartSec=15s
NoNewPrivileges=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictRealtime=true
RestrictNamespaces=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SystemCallArchitectures=native
UMask=0077

[Install]
WantedBy=multi-user.target
KPANEL_NODE_FILE_SERVICE
	if [ -f "$file_service_path" ]; then
		cmp -s "$file_service_path" "$template" && return 0
		# Repair only the exact installer-owned legacy template. Preserve custom
		# units and drop-ins; never erase an administrator's service policy.
		# The original managed unit also required terminal.json before file-broker
		# could start. Compare that complete historical variant, not arbitrary edits.
		sed -e '/^ConditionPathExists=/aConditionPathExists=/etc/kejilion-node/terminal.json' \
			-e 's/^RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6$/RestrictAddressFamilies=AF_UNIX/' "$template" >"$legacy_template"
		if ! cmp -s "$file_service_path" "$legacy_template" && ! sed 's/^RestrictAddressFamilies=AF_UNIX$/RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6/' "$file_service_path" | cmp -s - "$template"; then
			echo "KPanel file service has custom settings; retaining the existing unit" >&2
			return 0
		fi
	fi
	unit_temporary="$(mktemp "${file_service_path}.XXXXXX")" || return 1
	if ! install -o root -g root -m 0644 "$template" "$unit_temporary" || ! mv -f -- "$unit_temporary" "$file_service_path"; then
		rm -f -- "$unit_temporary"
		return 1
	fi
	file_service_unit_changed=true
	systemctl daemon-reload
}

service_running_current() {
	local service="$1" pid
	systemctl is-active --quiet "$service" || return 1
	pid="$(systemctl show "$service" --property=MainPID --value)" || return 1
	[[ "$pid" =~ ^[1-9][0-9]*$ ]] && [ "/proc/${pid}/exe" -ef "$binary_path" ]
}
wait_for_service() {
	local service="$1" attempt
	for attempt in {1..20}; do
		if service_running_current "$service"; then
			sleep 0.25
			service_running_current "$service" && return 0
		fi
		sleep 0.25
	done
	return 1
}

repair_config_access() {
	local directory="/etc/kejilion-node" config="/etc/kejilion-node/node.json" gid owner group mode
	[ -e "$config" ] || return 0
	[ -d "$directory" ] && [ ! -L "$directory" ] && [ -f "$config" ] && [ ! -L "$config" ] || return 1
	gid="$(id -g kejilion-node)" || return 1
	[ "$(id -gn kejilion-node)" = "kejilion-node" ] || return 1
	[ "$(stat -c '%u:%g:%a' "$directory")" = "0:${gid}:750" ] || return 1
	read -r owner group mode < <(stat -c '%u %g %a' "$config")
	[ "$owner" = "0" ] && { [ "$group" = "0" ] || [ "$group" = "$gid" ]; } || return 1
	case "$mode" in 600|640) ;; *) return 1 ;; esac
	# Restore only the installer-owned telemetry credential, never terminal keys
	# or a caller-supplied path. Older file brokers accidentally made it 0600.
	chown "root:${gid}" "$config" && chmod 0640 "$config"
}
restart_required=false
if [ "$mode" = "update" ] && systemctl cat kejilion-node.service >/dev/null 2>&1; then
	restart_required=true
fi

restart_optional_services() {
	if ! ensure_file_service_unit; then
		echo "KPanel lightweight node updated; file service unit is unavailable" >&2
	fi
	systemctl enable "$file_service" >/dev/null 2>&1 || true
	# Telemetry is the core update contract. Optional brokers can be unavailable
	# on older centers; their failure must not roll back a healthy reporting node.
	local service
	for service in kejilion-node-terminal.service kejilion-node-ssh-login.service kejilion-node-file.service; do
		if systemctl cat "$service" >/dev/null 2>&1; then
			if service_running_current "$service" && { [ "$service" != "$file_service" ] || [ "$file_service_unit_changed" != true ]; }; then continue; fi
			if ! systemctl restart "$service" || ! wait_for_service "$service"; then
				echo "KPanel lightweight node updated; optional service unavailable: ${service}" >&2
			fi
		fi
	done
}
restart_services() {
	repair_config_access || return 1
	systemctl restart kejilion-node.service || return 1
	wait_for_service kejilion-node.service || return 1
	restart_optional_services
}

if [ -f "$binary_path" ] && [ "$(sha256sum "$binary_path" | awk '{print $1}')" = "$expected" ]; then
	if [ "$restart_required" = "true" ] && ! service_running_current kejilion-node.service; then
		restart_services || { echo "installed node is current but restart failed" >&2; exit 1; }
	elif [ "$restart_required" = "true" ]; then
		repair_config_access || { echo "node configuration access is invalid" >&2; exit 1; }
		restart_optional_services
	fi
	echo "KPanel lightweight node is already up to date."
	exit 0
fi

curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location --silent --show-error \
	--connect-timeout 15 --max-time 180 --retry 3 --retry-delay 5 --retry-max-time 600 \
	--max-filesize 134217728 \
	-o "${temporary_dir}/${binary_name}" "${release_base}/${binary_name}"
actual="$(sha256sum "${temporary_dir}/${binary_name}" | awk '{print $1}')"
[ "$actual" = "$expected" ] || {
	echo "release checksum verification failed" >&2
	exit 1
}
chmod 0755 "${temporary_dir}/${binary_name}"
version_output="$("${temporary_dir}/${binary_name}" version)"
printf '%s\n' "$version_output" | grep -Eq '^[^[:space:]]+ light-v1$' || {
	echo "release binary protocol is invalid" >&2
	exit 1
}

install -o root -g root -m 0755 "${temporary_dir}/${binary_name}" "${binary_path}.new"
had_previous=false
if [ -f "$binary_path" ]; then
	cp -p -- "$binary_path" "${binary_path}.previous"
	had_previous=true
fi
mv -f -- "${binary_path}.new" "$binary_path"

if [ "$restart_required" = "true" ] && ! restart_services; then
	if [ "$had_previous" = "true" ] && [ -f "${binary_path}.previous" ]; then
		mv -f -- "${binary_path}.previous" "$binary_path"
		if ! restart_services; then
			echo "KPanel lightweight node rollback restored the binary but service recovery failed." >&2
			exit 1
		fi
		echo "KPanel lightweight node update failed and was rolled back." >&2
	else
		echo "KPanel lightweight node update failed; no previous binary is available." >&2
	fi
	exit 1
fi
rm -f -- "${binary_path}.previous"
echo "KPanel lightweight node update completed: ${version_output}"
