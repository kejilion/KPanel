#!/bin/bash
# Shared by the installer and its generated updater. Keep the inode outside
# the removable installation; unlinking a held flock creates two lock owners.
kpanel_node_release_lock() {
	[ "${kpanel_legacy_lock_owned:-false}" != true ] || rmdir -- /run/lock/kejilion-node-update.lock 2>/dev/null || true
}
kpanel_node_acquire_lock() {
	local lifecycle_lock=/run/kejilion-node-lifecycle.lock
	local home=/usr/local/lib/kejilion-node legacy_lock=/run/lock/kejilion-node-update.lock
	local legacy_marker="${home}/legacy-update.pid" legacy_pid legacy_start process argument script_file
	kpanel_legacy_lock_owned=false
	[ ! -L "$lifecycle_lock" ] && { [ ! -e "$lifecycle_lock" ] || [ -f "$lifecycle_lock" ]; } || return 1
	[ ! -e "$lifecycle_lock" ] || [ "$(stat -c '%u' "$lifecycle_lock")" = 0 ] || return 1
	# The installer passes the same open-file description to its child updater.
	# Do not trust an environment flag as proof of lock ownership.
	if [ /proc/self/fd/8 -ef "$lifecycle_lock" ] && flock -n 8; then
		return 0
	fi
	local previous_umask="$(umask)"
	umask 077
	exec 8>>"$lifecycle_lock" || { umask "$previous_umask"; return 1; }
	umask "$previous_umask"
	if ! flock -n 8; then
		echo "another KPanel lightweight node lifecycle operation is running; retry when it finishes" >&2
		return 1
	fi
	chmod 0600 "$lifecycle_lock" || return 1
	# Bridge the previous flock updater before inspecting its PID marker. Keep
	# this descriptor open through enrollment, activation and uninstall as well.
	if [ -d "$home" ]; then
		[ ! -L "$home" ] && [ ! -L "${home}/update.lock" ] || return 1
		exec 9>>"${home}/update.lock" || return 1
		if ! flock -n 9; then
			echo "another KPanel lightweight node update is running" >&2
			return 1
		fi
	fi
	if [ -e "$legacy_marker" ] || [ -L "$legacy_marker" ]; then
		if [ -L "$legacy_marker" ] || [ ! -f "$legacy_marker" ] ||
			! read -r legacy_pid legacy_start <"$legacy_marker" ||
			! [[ "$legacy_pid" =~ ^[1-9][0-9]*$ && "$legacy_start" =~ ^[0-9]+$ ]]; then
			echo "KPanel legacy updater identity is invalid; inspect ${legacy_marker} first" >&2
			return 1
		fi
		if [ "$(awk '{ sub(/^.*\) /, ""); print $20 }' "/proc/${legacy_pid}/stat" 2>/dev/null || true)" = "$legacy_start" ]; then
			echo "previous KPanel lightweight node updater is still finishing" >&2
			return 1
		fi
	fi
	if ! mkdir -- "$legacy_lock" 2>/dev/null; then
		[ ! -L "$legacy_lock" ] && [ -d "$legacy_lock" ] && [ -r /proc/self/stat ] || return 1
		[ "$(stat -c '%u' "$legacy_lock")" = 0 ] || return 1
		for process in /proc/[0-9]*; do
			[ "${process##*/}" != "$BASHPID" ] || continue
			script_file="$(readlink "${process}/fd/255" 2>/dev/null || true)"
			if [ "${script_file% (deleted)}" = "${home}/update.sh" ]; then
				echo "legacy KPanel update lock is still in use" >&2; return 1
			fi
			if [ ! -r "${process}/cmdline" ]; then
				[ ! -d "$process" ] && continue
				return 1
			fi
			while IFS= read -r -d '' argument; do
				if [ "$argument" = "${home}/update.sh" ]; then
					echo "legacy KPanel update lock is still in use" >&2; return 1
				fi
			done <"${process}/cmdline" || return 1
		done
		# Never recursively remove unknown contents or a competing old owner.
		rmdir -- "$legacy_lock" && mkdir -- "$legacy_lock" || return 1
		echo "Recovered an inactive KPanel lightweight node update lock."
	fi
	kpanel_legacy_lock_owned=true
	trap kpanel_node_release_lock EXIT
	trap 'echo "KPanel lightweight node update interrupted; run the command again to retry." >&2; exit 130' INT
	trap 'exit 143' HUP TERM
	rm -f -- "$legacy_marker"
}
# KPANEL_NODE_RUNTIME_GENERATION=2
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
# Resolve releases at the origin: script-delivery proxies can rewrite literal
# GitHub URLs and hide the redirects that bind the checksum to one release.
github_host="github.com"
base_url="https://${github_host}/kejilion/KPanel/releases/latest/download"
temporary_dir=""

kpanel_node_acquire_lock || exit 1
cleanup() {
	[ -z "$temporary_dir" ] || rm -rf -- "$temporary_dir"
	kpanel_node_release_lock
}
trap cleanup EXIT
trap 'echo "KPanel lightweight node update interrupted; run the command again to retry." >&2; exit 130' INT
trap 'exit 143' HUP TERM

# Old release assets only migrate in kejilion-node-update.*. The new staging
# namespace prevents their `version` probe from restoring an obsolete updater.
temporary_dir="$(mktemp -d /tmp/kejilion-node-release.XXXXXX)"

curl_progress=(--silent --show-error)
[ ! -t 2 ] || curl_progress=(--progress-bar --show-error)
echo "Checking KPanel lightweight node release..."
if ! curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location "${curl_progress[@]}" \
	--connect-timeout 15 --max-time 60 --retry 3 --retry-delay 5 --retry-max-time 240 \
	--max-filesize 65536 --dump-header "${temporary_dir}/headers" \
	-o "${temporary_dir}/SHA256SUMS" "${base_url}/SHA256SUMS"; then
	echo "KPanel release check failed; check access to github.com and retry." >&2
	exit 1
fi
expected="$(awk -v name="$binary_name" '$2 == name { print $1 }' "${temporary_dir}/SHA256SUMS")"
printf '%s' "$expected" | grep -Eq '^[0-9a-f]{64}$' || {
	echo "release checksum is unavailable" >&2
	exit 1
}
# The first redirect binds the manifest to a release; the following CDN redirect
# must never be used as a base URL or mixed with a later value of latest.
release_url="$(awk 'tolower($1) == "location:" { sub(/\r$/, "", $2); print $2 }' "${temporary_dir}/headers" |
	grep -E '^https://github[.]com/kejilion/KPanel/releases/download/v[0-9]+\.[0-9]+\.[0-9]+/SHA256SUMS$' | tail -n 1 || true)"
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

echo "Downloading KPanel lightweight node..."
if ! curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location "${curl_progress[@]}" \
	--connect-timeout 15 --max-time 180 --retry 3 --retry-delay 5 --retry-max-time 600 \
	--max-filesize 134217728 \
	-o "${temporary_dir}/${binary_name}" "${release_base}/${binary_name}"; then
	echo "KPanel node download failed; check access to GitHub release downloads and retry." >&2
	exit 1
fi
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
