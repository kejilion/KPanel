package browsercomponent

import _ "embed"

// These build inputs are byte-checked against the manual deployment assets.
// Keep their provenance with packaging/browser rather than a second generator.
//
//go:embed assets/managed-compose.yml
var composeTemplate []byte

//go:embed assets/browser-seccomp.json
var seccompProfile []byte

const componentImage = "docker.io/kjlion/kpanel-browser@sha256:0660b4608e9219e8d03c4d72dc8ba3e9c64f6310df36c766933373855f918839"
