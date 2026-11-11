#!/usr/bin/env bash
set -Eeuo pipefail
test "${GITHUB_ACTIONS:-}" = true
test "${RUNNER_ENVIRONMENT:-}" = github-hosted
test "${GITHUB_REPOSITORY:-}" = kejilion/KPanel
test "${GITHUB_REF:-}" = refs/heads/main
: "${RUNNER_TEMP:?}" "${GITHUB_WORKSPACE:?}" "${EXPECTED_CANDIDATE:?}" "${GITHUB_SHA:?}"
[[ "$GITHUB_RUN_ID" =~ ^[1-9][0-9]*$ && "$GITHUB_RUN_ATTEMPT" =~ ^[1-9][0-9]*$ ]]
[[ "$EXPECTED_CANDIDATE" =~ ^[a-f0-9]{40}$ ]]
test "$(git rev-parse HEAD)" = "$EXPECTED_CANDIDATE"
test -z "$(git status --porcelain)"
test -f internal/virtualmachine/jobs.go
key="$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
proof="$RUNNER_TEMP/kpanel-native-$key"
test ! -e "$proof"
mkdir -m 755 "$proof" "$proof/evidence" "$proof/build" "$proof/http"
evidence="$proof/evidence"
controller="$GITHUB_WORKSPACE/controller"
image='node:24.22.1-alpine@sha256:6a1c190e17215d844926b0677f4bf87273ff7d5e54db06d3b5b5863ab0b09a3d'
network="kpanel-native-$key"; server="kpanel-native-http-$key"
install_dir="/usr/local/libexec/kpanel-release-qualification-$key"
state_dir="/var/lib/kpanel-release-qualification-$key"
name="kpanel-qa-$key"
cleanup() {
  docker logs "$server" > "$evidence/docker-http.log" 2>&1 || true
  docker inspect "$server" > "$evidence/docker-http-inspect.json" 2>&1 || true
  docker rm -f "$server" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  # Root filesystem changes belong only to this ephemeral hosted runner. Native
  # test cleanup stops its exact task units/domain; failed evidence is retained.
  printf 'Attempted cleanup of exact owned Docker resources; command failures may remain. Hosted runner disposal is the final resource boundary.\n' > "$evidence/cleanup.txt"
}
trap cleanup EXIT
exec > >(tee "$evidence/native-host.log") 2>&1
test -c /dev/kvm
test "$(go env GOVERSION)" = go1.27.4
test "$(ps -p 1 -o comm=)" = systemd
git ls-files '*.go' | LC_ALL=C sort | while IFS= read -r file; do sha256sum "$file"; done > "$evidence/go-sources-before.sha256"
git rev-parse 'HEAD^{tree}' > "$evidence/source-tree.txt"
go version > "$evidence/go-version.txt"
test "$(id -u)" -ne 0
go test -run '^TestWorkerRejectsNonRootOwnedState$' -v ./internal/virtualmachine > "$evidence/nonroot-worker-test.log"
grep -F -- '--- PASS: TestWorkerRejectsNonRootOwnedState' "$evidence/nonroot-worker-test.log"
cat /etc/os-release > "$evidence/os-release.txt"
sha256sum "$controller/.github/workflows/virtual-machine-native-verify.yml" "$controller/packaging/tests/virtualmachine/native-host.sh" "$controller/packaging/tests/virtualmachine/native-host_test.go.tmpl" > "$evidence/controller-sources.sha256"
cp "$controller/packaging/tests/virtualmachine/native-host_test.go.tmpl" "$proof/build/native-host_test.go"
node --input-type=module - "$PWD" "$proof" <<'NODE'
import { writeFileSync } from 'node:fs';import { join } from 'node:path';
const [repo,proof]=process.argv.slice(2);
writeFileSync(join(proof,'build/overlay.json'),JSON.stringify({Replace:{[join(repo,'internal/virtualmachine/native_release_qualification_test.go')]:join(proof,'build/native-host_test.go')}}));
NODE
go build -trimpath -buildvcs=false -o "$proof/build/kejilion-agent" ./cmd/kejilion-agent
go test -c -trimpath -buildvcs=false -overlay "$proof/build/overlay.json" -o "$proof/build/native-test" ./internal/virtualmachine
sha256sum "$proof/build/kejilion-agent" "$proof/build/native-test" "$proof/build/native-host_test.go" > "$evidence/build.sha256"
sudo test ! -e "$install_dir"
sudo test ! -e "$state_dir"
sudo install -d -m 755 "$install_dir"
sudo install -m 755 "$proof/build/kejilion-agent" "$install_dir/kejilion-agent"
sudo install -m 755 "$proof/build/native-test" "$install_dir/native-test"
sudo install -d -m 700 "$state_dir"
docker pull "$image"
docker network create "$network" > "$evidence/docker-network-id.txt"
printf 'KPanel-native-%s\n' "$key" > "$proof/http/index.html"
docker run -d --name "$server" --network "$network" --read-only --cap-drop ALL --security-opt no-new-privileges --memory 64m --cpus .25 --pids-limit 32 -p 127.0.0.1::8080 -v "$proof/http:/web:ro" --entrypoint node "$image" --max-old-space-size=16 -e 'require("node:http").createServer((req,res)=>res.end(require("node:fs").readFileSync("/web/index.html"))).listen(8080,"0.0.0.0")'
port=$(docker port "$server" 8080/tcp | sed -n 's/^127\.0\.0\.1://p')
[[ "$port" =~ ^[0-9]{1,5}$ ]]
url="http://127.0.0.1:$port/"
# A bounded readiness window covers container startup without weakening the
# exact response or later post-installation coexistence assertions.
curl -fsS --max-time 2 --retry 10 --retry-all-errors --retry-delay 1 --retry-max-time 20 "$url" | grep -Fx "KPanel-native-$key"
peer_probe() {
  docker run --rm --network "$network" --read-only --cap-drop ALL --security-opt no-new-privileges --memory 64m --cpus .25 --pids-limit 32 --entrypoint node "$image" --max-old-space-size=16 -e 'require("node:http").get(process.argv[1],r=>{if(r.statusCode!==200)process.exitCode=1;r.pipe(process.stdout)}).on("error",()=>process.exit(1)).setTimeout(10000,function(){this.destroy(Error("timeout"))})' "http://$server:8080/" | grep -Fx "KPanel-native-$key"
}
peer_probe
sudo iptables -S FORWARD | head -n 1 > "$evidence/forward-before.txt"
sudo iptables-save > "$evidence/iptables-before.txt"
docker network inspect "$network" bridge > "$evidence/docker-networks-before.json"
# The root-only fake-runner unit fixture remains separate from real native
# integration proof; both execute with their required ownership boundary.
sudo env GITHUB_ACTIONS=true RUNNER_ENVIRONMENT=github-hosted GITHUB_REPOSITORY=kejilion/KPanel "KPANEL_NATIVE_KEY=$key" "KPANEL_NATIVE_NAME=$name" "KPANEL_NATIVE_STATE=$state_dir" "KPANEL_NATIVE_AGENT=$install_dir/kejilion-agent" "KPANEL_NATIVE_EVIDENCE=$evidence" "KPANEL_NATIVE_DOCKER_URL=$url" "KPANEL_NATIVE_SHA=$EXPECTED_CANDIDATE" "$install_dir/native-test" -test.v -test.run '^(TestNativeReleaseQualification|TestWorkerRunsOnceAndPersistsFailure)$' -test.timeout 45m 2>&1 | tee "$evidence/native-tests.log"
grep -F -- '--- PASS: TestWorkerRunsOnceAndPersistsFailure' "$evidence/native-tests.log"
grep -F -- '--- PASS: TestNativeReleaseQualification' "$evidence/native-tests.log"
curl -fsS --max-time 10 "$url" | grep -Fx "KPanel-native-$key"
peer_probe
sudo iptables -S FORWARD | head -n 1 > "$evidence/forward-after.txt"
diff -u "$evidence/forward-before.txt" "$evidence/forward-after.txt"
sudo iptables-save > "$evidence/iptables-after.txt"
sudo nft list ruleset > "$evidence/nft-after.txt"
grep -qi libvirt "$evidence/iptables-after.txt" "$evidence/nft-after.txt"
grep -q DOCKER "$evidence/iptables-after.txt"
docker network inspect "$network" bridge > "$evidence/docker-networks-after.json"
dpkg-query -W 'libvirt*' 'qemu*' 'dnsmasq*' 'cloud-image-utils' > "$evidence/packages.txt"
sudo virsh --connect qemu:///system net-dumpxml default > "$evidence/default-network.xml"
network_source=/usr/share/libvirt/networks/default.xml
if ! test -f "$network_source"; then network_source=/usr/share/libvirt/networks/default.xml.in; fi
test -f "$network_source"
dpkg-query -S "$network_source" > "$evidence/default-network-package.txt"
sha256sum "$network_source" > "$evidence/default-network-source.sha256"
apt-cache policy libvirt-daemon-system libvirt-daemon-config-network > "$evidence/libvirt-package-policy.txt"
git ls-files '*.go' | LC_ALL=C sort | while IFS= read -r file; do sha256sum "$file"; done > "$evidence/go-sources-after.sha256"
diff -u "$evidence/go-sources-before.sha256" "$evidence/go-sources-after.sha256"
test "$(git rev-parse HEAD)" = "$EXPECTED_CANDIDATE"
test -z "$(git status --porcelain)"
node --input-type=module - "$evidence" "$EXPECTED_CANDIDATE" "$GITHUB_SHA" <<'NODE'
import{readFileSync,writeFileSync}from'node:fs';import{join}from'node:path';
const [dir,candidate,controller]=process.argv.slice(2);const native=JSON.parse(readFileSync(join(dir,'native-result.json')));
if(native.status!=='passed'||native.candidate!==candidate)throw Error('Native evidence did not pass');
writeFileSync(join(dir,'receipt.json'),JSON.stringify({status:'passed',candidate,controller,environment:'github-hosted',native,DockerHostAndPeerBeforeAfter:true,forwardPolicyUnchanged:true,defaultNetworkPackageRecorded:true,originalGoSourcesUnchanged:true,productionConnected:false,finishedAt:new Date().toISOString()},null,2)+'\n');
NODE
