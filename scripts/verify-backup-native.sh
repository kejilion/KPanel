#!/usr/bin/env bash
# Independent Linux Docker integration fixture. No existing daemon is used.
# Inputs: KPB_NATIVE_SOURCE (read-only source), KPB_NATIVE_GO (Go root),
# KPB_NATIVE_MODCACHE (already-populated module cache). All writes go to mktemp.
set -euo pipefail
if [ "$(id -u)" != 0 ]; then echo "run as root for private mount/network namespaces" >&2; exit 2; fi
: "${KPB_NATIVE_SOURCE:?source directory required}"
: "${KPB_NATIVE_GO:?Go root required}"
: "${KPB_NATIVE_MODCACHE:?offline module cache required}"
for cmd in unshare nsenter mount ip docker dockerd containerd tar; do command -v "$cmd" >/dev/null; done
source_dir=$(realpath "$KPB_NATIVE_SOURCE")
go_dir=$(realpath "$KPB_NATIVE_GO")
mod_dir=$(realpath "$KPB_NATIVE_MODCACHE")
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
task=$(mktemp -d /var/tmp/kpanel-native-review.XXXXXX)
chmod 700 "$task"
mkdir -p "$task"/{runtime,modcache,gocache,repo/internal,fs/home,fs/opt,jobs,evidence,image-root}
cp "$source_dir/go.mod" "$source_dir/go.sum" "$task/repo/"
cp -a "$source_dir/internal/backup" "$source_dir/internal/hostbackup" "$task/repo/internal/"
cp "$script_dir/testdata/backup-native/main.go" "$task/fixture.go"
cp "$0" "$task/evidence/run-native-integration.sh"
export KPB_NATIVE_TASK="$task" KPB_NATIVE_GO="$go_dir" KPB_NATIVE_MODCACHE="$mod_dir"
echo "EVIDENCE_DIR=$task/evidence"
unshare --mount --net --fork bash -s <<'ISOLATED'
set -euo pipefail
task=$KPB_NATIVE_TASK
mount --make-rprivate /
mount --bind "$KPB_NATIVE_GO" "$task/runtime"
mount --bind "$KPB_NATIVE_MODCACHE" "$task/modcache"
mount -o remount,bind,ro "$task/modcache"
mount --bind "$task/fs/home" /home
mount --bind "$task/fs/opt" /opt
ip link set lo up
printf '{"Task":"%s","MountNS":"%s","NetNS":"%s"}\n' "$task" "$(readlink /proc/self/ns/mnt)" "$(readlink /proc/self/ns/net)" > "$task/isolation.json"
chmod 600 "$task/isolation.json"
export PATH="$task/runtime/bin:$PATH" GOMODCACHE="$task/modcache" GOCACHE="$task/gocache" GOPROXY=off GOSUMDB=off
CGO_ENABLED=0 go build -o "$task/image-root/fixture" "$task/fixture.go"
tar -C "$task/image-root" -cf "$task/image.tar" .
containerd --root "$task/containerd-root" --state "$task/containerd-state" --address "$task/containerd.sock" >"$task/evidence/containerd.log" 2>&1 &
ctd=$!
dockerd --host="unix://$task/docker.sock" --data-root=/opt/docker --exec-root="$task/docker-run" --pidfile="$task/docker.pid" --containerd="$task/containerd.sock" --containerd-namespace=kpb-native --containerd-plugins-namespace=kpb-native-plugins --storage-driver=vfs --iptables=false --ip6tables=false --ip-forward=false >"$task/evidence/dockerd.log" 2>&1 &
dock=$!
migrate=0
cleanup() {
 touch "$task/STOP-MIGRATE"
 if [ "$migrate" != 0 ]; then wait "$migrate" || true; fi
 kill "$dock" "$ctd" 2>/dev/null || true
 wait "$dock" "$ctd" 2>/dev/null || true
}
trap cleanup EXIT
ready=false
for n in $(seq 1 50); do
 if docker -H "unix://$task/docker.sock" info >"$task/evidence/docker-info.txt" 2>&1; then ready=true; break; fi
 sleep 1
done
$ready || { cat "$task/evidence/dockerd.log"; exit 1; }
docker -H "unix://$task/docker.sock" import --change 'ENTRYPOINT ["/fixture"]' --change 'CMD ["sleep"]' --change 'EXPOSE 1234' "$task/image.tar" kpb-native-fixture:v1 >"$task/evidence/image-import.txt"
docker -H "unix://$task/docker.sock" save -o "$task/image-save.tar" kpb-native-fixture:v1
unshare --net --fork bash -s <<'MIGRATION' &
set -euo pipefail
task=$KPB_NATIVE_TASK
ip link set lo up
containerd --root "$task/containerd-migrate-root" --state "$task/containerd-migrate-state" --address "$task/containerd-migrate.sock" >"$task/evidence/containerd-migrate.log" 2>&1 &
ctd=$!
dockerd --host="unix://$task/docker-migrate.sock" --data-root=/opt/docker-migrate --exec-root="$task/docker-migrate-run" --pidfile="$task/docker-migrate.pid" --containerd="$task/containerd-migrate.sock" --containerd-namespace=kpb-migrate --containerd-plugins-namespace=kpb-migrate-plugins --storage-driver=vfs --iptables=false --ip6tables=false --ip-forward=false >"$task/evidence/dockerd-migrate.log" 2>&1 &
dock=$!
trap 'kill "$dock" "$ctd" 2>/dev/null || true; wait "$dock" "$ctd" 2>/dev/null || true' EXIT
ready=false
for n in $(seq 1 50); do
 if docker -H "unix://$task/docker-migrate.sock" info >"$task/evidence/docker-migrate-info.txt" 2>&1; then ready=true; break; fi
 sleep 1
done
$ready || exit 1
docker -H "unix://$task/docker-migrate.sock" load -i "$task/image-save.tar" >"$task/evidence/image-migrate-load.txt"
touch "$task/MIGRATE-READY"
while [ ! -f "$task/STOP-MIGRATE" ]; do sleep 1; done
MIGRATION
migrate=$!
for n in $(seq 1 50); do
 if [ -f "$task/MIGRATE-READY" ]; then break; fi
 kill -0 "$migrate"
 sleep 1
done
test -f "$task/MIGRATE-READY"
cd "$task/repo"
uname -a >"$task/evidence/environment.txt"
go version >>"$task/evidence/environment.txt"
docker -H "unix://$task/docker.sock" version >>"$task/evidence/environment.txt"
sha256sum internal/hostbackup/*.go internal/backup/*.go >"$task/evidence/source-sha256.txt"
go test ./internal/hostbackup -run '^TestNative' -count=1 -v -timeout=1m >"$task/evidence/default-skip.log" 2>&1
KPB_NATIVE_INTEGRATION=1 go test ./internal/hostbackup -run '^TestNative' -count=1 -v -timeout=15m 2>&1 | tee "$task/evidence/native.log"
docker -H "unix://$task/docker.sock" ps -a >"$task/evidence/source-containers-after.txt"
docker -H "unix://$task/docker-migrate.sock" ps -a >"$task/evidence/target-containers-after.txt"
ISOLATED
echo "PASS artifacts retained at $task"
