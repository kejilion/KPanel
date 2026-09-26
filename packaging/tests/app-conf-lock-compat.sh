#!/usr/bin/env bash
set -euo pipefail

project_dir="${1:-.}"
app_conf="${2:-${project_dir}/packaging/kejilion-app/kpanel.conf}"

run_lock_compat_test() {
    # The app definition is sourced inside a kejilion.sh function in production.
    docker_app_plus() { :; }
    # shellcheck source=/dev/null
    . "${app_conf}"

    local busy=0
    local attempts=0
    flock() {
        [ "$1" = -n ] || return 2
        attempts=$((attempts + 1))
        [ "$busy" = 0 ]
    }
    sleep() {
        [ "$1" = 1 ] || return 2
        busy=0
    }

    kpanel_wait_for_lifecycle_lock 9 0
    [ "$attempts" = 1 ]

    busy=1
    attempts=0
    if kpanel_wait_for_lifecycle_lock 9 0; then
        echo "busy lifecycle lock was accepted" >&2
        return 1
    fi
    [ "$attempts" = 1 ]

    busy=1
    attempts=0
    kpanel_wait_for_lifecycle_lock 9 1
    [ "$attempts" = 2 ]
}

run_lock_compat_test
printf '%s\n' 'app_conf_lock_compat=pass'
