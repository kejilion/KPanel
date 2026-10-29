package browsercomponent

import _ "embed"

// These build inputs are byte-checked against the manual deployment assets.
// Keep their provenance with packaging/browser rather than a second generator.
//
//go:embed assets/managed-compose.yml
var composeTemplate []byte

//go:embed assets/browser-seccomp.json
var seccompProfile []byte

const componentImage = "docker.io/kjlion/kpanel-browser@sha256:37c0eb1c4a0e6408fe395e699687d405fe78d052669c32625e09a17a7e3165af"
