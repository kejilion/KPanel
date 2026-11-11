package browsercomponent

import _ "embed"

// These build inputs are byte-checked against the manual deployment assets.
// Keep their provenance with packaging/browser rather than a second generator.
//
//go:embed assets/managed-compose.yml
var composeTemplate []byte

//go:embed assets/browser-seccomp.json
var seccompProfile []byte

const componentImage = "docker.io/kjlion/kpanel-browser@sha256:9a5cd79bef4262cf675331a8ce07e9e5df9121d26fb3721440c6256ad444ea03"
