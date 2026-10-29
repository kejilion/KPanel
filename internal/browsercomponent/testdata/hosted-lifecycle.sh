#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${GITHUB_ACTIONS:-} == true && ${RUNNER_ENVIRONMENT:-} == github-hosted && -n ${RUNNER_TEMP:-} ]]
repo=$(cd "$(dirname "$0")/../../.." && pwd -P)
evidence="$repo/internal/browsercomponent/testdata"
network=kejilion-panel-browser-control
egress=kpanel-browser_browser-egress
if docker container inspect kpanel-browser >/dev/null 2>&1 || docker container inspect kejilion-panel >/dev/null 2>&1 || docker network inspect "$network" >/dev/null 2>&1 || docker network inspect "$egress" >/dev/null 2>&1; then
  echo 'preflight failed: fixed browser resources already exist' >&2; exit 1
fi
runtime=$(mktemp -d "$RUNNER_TEMP/kpanel-component-test.XXXXXXXX")
mkdir -m 0700 "$runtime/state"
umask 0077
label="hosted-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
panel_id=''
cleanup() {
  status=$?
  trap - EXIT
  actual=$(docker network inspect --format '{{index .Labels "com.docker.compose.project"}}' "$network" 2>/dev/null || true)
  if [[ "$actual" == kpanel && -n "$panel_id" ]]; then
    if docker container inspect kpanel-browser >/dev/null 2>&1; then
      attached=$(docker inspect --format '{{with index .NetworkSettings.Networks "kejilion-panel-browser-control"}}{{.NetworkID}}{{end}}' kpanel-browser)
      owned=$(docker network inspect --format '{{.Id}}' "$network")
      if [[ "$attached" == "$owned" ]]; then docker rm -f kpanel-browser >/dev/null || status=1; else status=1; fi
    fi
    docker network disconnect "$network" "$panel_id" >/dev/null || status=1
    docker network rm "$network" >/dev/null || status=1
  fi
  if [[ -n "$panel_id" ]]; then docker rm -f "$panel_id" >/dev/null || status=1; fi
  if docker network inspect "$egress" >/dev/null 2>&1; then
    project=$(docker network inspect --format '{{index .Labels "com.docker.compose.project"}}' "$egress")
    if [[ "$project" == kpanel-browser ]]; then docker network rm "$egress" >/dev/null || status=1; else status=1; fi
  fi
  case "$runtime" in "$RUNNER_TEMP"/kpanel-component-test.*) rm -rf -- "$runtime" ;; *) status=1 ;; esac
  exit "$status"
}
trap cleanup EXIT
mkdir -m 0755 "$runtime/runtime-parent"
cd "$repo"
go build -buildvcs=false -o "$runtime/agent" ./cmd/kejilion-agent
go build -buildvcs=false -o "$runtime/reconcile" "$evidence/reconcile.go"
CGO_ENABLED=0 go build -buildvcs=false -o "$runtime/probe" "$evidence/live-browser-probe/main.go"
chmod 0555 "$runtime/probe"
docker pull docker.io/kjlion/kpanel-browser@sha256:37c0eb1c4a0e6408fe395e699687d405fe78d052669c32625e09a17a7e3165af >/dev/null
start_panel() { docker run -d --name kejilion-panel --label com.docker.compose.project=kpanel \
  --label com.docker.compose.service=panel --label "kpanel.test=$label" \
  --mount "type=bind,src=$runtime/runtime-parent,dst=/run/kejilion-panel,readonly" \
  --mount "type=bind,src=$runtime/probe,dst=/test/probe,readonly" \
  --entrypoint /bin/sleep docker.io/kjlion/kpanel-browser@sha256:37c0eb1c4a0e6408fe395e699687d405fe78d052669c32625e09a17a7e3165af 3600; }
cat > "$runtime/panel.yml" <<EOF
services:
  panel:
    image: docker.io/kjlion/kpanel-browser@sha256:37c0eb1c4a0e6408fe395e699687d405fe78d052669c32625e09a17a7e3165af
    container_name: kejilion-panel
    entrypoint: ["/bin/sleep", "3600"]
    volumes:
      - "$runtime/runtime-parent:/run/kejilion-panel:ro"
      - "$runtime/probe:/test/probe:ro"
    networks: [browser-control]
networks:
  browser-control:
    name: kejilion-panel-browser-control
    internal: true
EOF
docker compose -p kpanel -f "$runtime/panel.yml" up -d >/dev/null
panel_id=$(docker inspect --format '{{.Id}}' kejilion-panel)
for action in install stop start restart uninstall install uninstall; do
  id=$(node "$evidence/live-component-task.mjs" "$action" "$runtime/state")
  "$runtime/agent" browser-component-run --state-dir "$runtime/state" --runtime-dir "$runtime/runtime-parent/browser" --docker-socket /var/run/docker.sock --job "$id"
  node "$evidence/live-component-task.mjs" result "$runtime/state"
  if [[ "$action" == install ]]; then
    docker inspect --format 'limits memory={{.HostConfig.Memory}} pids={{.HostConfig.PidsLimit}} readonly={{.HostConfig.ReadonlyRootfs}} ports={{json .HostConfig.PortBindings}}' kpanel-browser
    [[ $(docker inspect --format '{{.HostConfig.Memory}}' kpanel-browser) == 1610612736 ]]
    [[ $(docker inspect --format '{{.HostConfig.PidsLimit}}' kpanel-browser) == 512 ]]
    [[ $(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' kpanel-browser) == true ]]
    [[ $(docker inspect --format '{{json .HostConfig.PortBindings}}' kpanel-browser) == '{}' ]]
    docker exec --user 65532:65532 "$panel_id" /test/probe browser /run/kejilion-panel/browser/browser-token
    if [[ "$action" == install && -z ${recreated:-} ]]; then
      docker rm -f "$panel_id" >/dev/null
      panel_id=$(start_panel)
      "$runtime/reconcile" "$runtime/state" "$runtime/runtime-parent/browser"
      docker exec --user 65532:65532 "$panel_id" /test/probe browser /run/kejilion-panel/browser/browser-token
      recreated=1
    fi
  fi
done
echo 'component_lifecycle=pass actions=7 navigation=pass cleanup=pending'
