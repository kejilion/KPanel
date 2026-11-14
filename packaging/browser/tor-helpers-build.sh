#!/bin/sh
set -eu
export GOTOOLCHAIN=local CGO_ENABLED=0 GOMAXPROCS=2
mkdir -p /out/provenance/licenses
cp /build/locks/manifest.json /out/provenance/manifest.json
for helper in conjure lyrebird; do
    cp "/build/locks/$helper/go.mod" "/build/src/$helper/go.mod"
    cp "/build/locks/$helper/go.sum" "/build/src/$helper/go.sum"
    cd "/build/src/$helper"
    go mod download
    go mod verify
    cmp go.mod "/build/locks/$helper/go.mod"
    cmp go.sum "/build/locks/$helper/go.sum"
    go list -mod=readonly -m -json all > "/out/provenance/$helper-modules.json"
    if [ "$helper" = conjure ]; then
        go build -p 1 -mod=readonly -trimpath -buildvcs=false -ldflags='-s -w' -o /out/conjure-client ./client
    else
        go build -p 1 -mod=readonly -trimpath -buildvcs=false -ldflags='-s -w -X main.lyrebirdVersion=0.10.0-kpanel.1' -o /out/lyrebird ./cmd/lyrebird
    fi
    mkdir -p "/out/provenance/licenses/$helper"
    find . -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) -exec cp '{}' "/out/provenance/licenses/$helper/" \;
done
# Preserve dependency license texts next to the exact module inventories.
cd /go/pkg/mod
find . -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) -exec cp --parents '{}' /out/provenance/licenses/ \;
go install -p 1 golang.org/x/vuln/cmd/govulncheck@v1.6.0
# Text mode returns a failure when vulnerable symbols are found; JSON mode does not.
/go/bin/govulncheck -mode=binary /out/conjure-client
/go/bin/govulncheck -mode=binary /out/lyrebird
sha256sum /out/conjure-client /out/lyrebird > /out/provenance/SHA256SUMS
