package browsercomponent

import _ "embed"

// These build inputs are byte-checked against the manual deployment assets.
// Keep their provenance with packaging/browser rather than a second generator.
//
//go:embed assets/managed-compose.yml
var composeTemplate []byte

//go:embed assets/browser-seccomp.json
var seccompProfile []byte

//go:embed assets/tor-compose.yml
var torComposeTemplate []byte

//go:embed assets/tor.Dockerfile
var torDockerfile []byte

//go:embed assets/tor-start.sh
var torStart []byte

// Public Tor release signature and signing key; verified against the primary
// fingerprint in tor.Dockerfile, never credentials.
//
//go:embed assets/tor-browser.tar.xz.asc
var torSignature []byte

//go:embed assets/tor-signing-key.asc
var torSigningKey []byte

const componentTorImage = "kpanel-browser-tor:16.0.1"

const componentImage = "docker.io/kjlion/kpanel-browser@sha256:9a5cd79bef4262cf675331a8ce07e9e5df9121d26fb3721440c6256ad444ea03"
