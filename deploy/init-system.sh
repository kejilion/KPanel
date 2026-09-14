#!/bin/sh

# Shared service-manager adapter for deployment scripts. The caller decides
# whether a failed probe is fatal; these helpers never start Docker implicitly.
kpanel_detect_init_system() {
	kpanel_systemd_runtime_dir=${KPANEL_SYSTEMD_RUNTIME_DIR:-/run/systemd/system}
	kpanel_openrc_runtime_dir=${KPANEL_OPENRC_RUNTIME_DIR:-/run/openrc}
	if command -v systemctl >/dev/null 2>&1 &&
		command -v systemd-analyze >/dev/null 2>&1 &&
		[ -d "$kpanel_systemd_runtime_dir" ]; then
		KPANEL_INIT_SYSTEM=systemd
		return 0
	fi
	if [ -d "$kpanel_openrc_runtime_dir" ] && command -v rc-service >/dev/null 2>&1 &&
		command -v rc-update >/dev/null 2>&1 &&
		command -v start-stop-daemon >/dev/null 2>&1 &&
		command -v supervise-daemon >/dev/null 2>&1 &&
		command -v logger >/dev/null 2>&1; then
		KPANEL_INIT_SYSTEM=openrc
		return 0
	fi
	# A live init marker is authoritative. Missing tools must fail closed rather
	# than selecting commands left behind by a different service manager.
	if [ -d "$kpanel_systemd_runtime_dir" ] || [ -d "$kpanel_openrc_runtime_dir" ]; then
		KPANEL_INIT_SYSTEM=
		return 1
	fi
	# Permit read-only preflight/dry-run validation in a build container where
	# systemd is installed but is not PID 1. Real installation still fails on
	# the first service-state query unless the manager is actually running.
	if command -v systemctl >/dev/null 2>&1 &&
		command -v systemd-analyze >/dev/null 2>&1 &&
		systemctl --version >/dev/null 2>&1; then
		KPANEL_INIT_SYSTEM=systemd
		return 0
	fi
	KPANEL_INIT_SYSTEM=
	return 1
}

kpanel_service_name() {
	case "$KPANEL_INIT_SYSTEM" in
		openrc) printf '%s\n' "${1%.service}" ;;
		*) printf '%s\n' "$1" ;;
	esac
}

kpanel_service_active() {
	service_name=$(kpanel_service_name "$1") || return 1
	case "$KPANEL_INIT_SYSTEM" in
		systemd) systemctl is-active --quiet "$service_name" ;;
		openrc) rc-service "$service_name" status >/dev/null 2>&1 ;;
		*) return 1 ;;
	esac
}

kpanel_service_start() {
	service_name=$(kpanel_service_name "$1") || return 1
	case "$KPANEL_INIT_SYSTEM" in
		systemd) systemctl restart "$service_name" ;;
		openrc) rc-service "$service_name" start ;;
		*) return 1 ;;
	esac
}

kpanel_service_stop() {
	service_name=$(kpanel_service_name "$1") || return 1
	case "$KPANEL_INIT_SYSTEM" in
		systemd) systemctl stop "$service_name" ;;
		openrc) rc-service "$service_name" stop ;;
		*) return 1 ;;
	esac
}

kpanel_service_enable() {
	service_name=$(kpanel_service_name "$1") || return 1
	case "$KPANEL_INIT_SYSTEM" in
		systemd) systemctl enable "$service_name" ;;
		openrc) rc-update add "$service_name" default ;;
		*) return 1 ;;
	esac
}

kpanel_service_disable() {
	service_name=$(kpanel_service_name "$1") || return 1
	case "$KPANEL_INIT_SYSTEM" in
		systemd) systemctl disable "$service_name" ;;
		openrc) rc-update del "$service_name" default ;;
		*) return 1 ;;
	esac
}

kpanel_reload_service_manager() {
	case "$KPANEL_INIT_SYSTEM" in
		systemd) systemctl daemon-reload ;;
		openrc) return 0 ;;
		*) return 1 ;;
	esac
}
