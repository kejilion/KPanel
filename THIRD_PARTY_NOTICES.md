# Third-Party Notices

KPanel is licensed under `AGPL-3.0-only`. This notice does not replace or
modify the licenses of third-party components distributed with or used by
KPanel.

## Bundled runtime components

| Component | Version or revision | License | License text |
| --- | --- | --- | --- |
| `kejilion.sh` | `f031d1206224de3743845d2fc81c4801ecda32f4` | Apache-2.0 | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) |
| `github.com/coder/websocket` | `v1.8.15` | ISC | [`LICENSES/coder-websocket-ISC.txt`](LICENSES/coder-websocket-ISC.txt) |
| `github.com/flynn/noise` | `v1.1.0` | BSD-3-Clause | [`LICENSES/flynn-noise-BSD-3-Clause.txt`](LICENSES/flynn-noise-BSD-3-Clause.txt) |
| `golang.org/x/crypto` | `v0.54.0` | BSD-3-Clause | [`LICENSES/golang-x-crypto-BSD-3-Clause.txt`](LICENSES/golang-x-crypto-BSD-3-Clause.txt) |
| `golang.org/x/image` | `v0.45.0` | BSD-3-Clause | [`LICENSES/golang-x-image-BSD-3-Clause.txt`](LICENSES/golang-x-image-BSD-3-Clause.txt) |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause | [`LICENSES/golang-x-sys-BSD-3-Clause.txt`](LICENSES/golang-x-sys-BSD-3-Clause.txt) |
| `@lucide/vue` | `1.26.0` | ISC and bundled icon notices | [`LICENSES/lucide-ISC.txt`](LICENSES/lucide-ISC.txt) |
| `@xterm/addon-fit` | `0.11.0` | MIT | [`LICENSES/xterm-addon-fit-MIT.txt`](LICENSES/xterm-addon-fit-MIT.txt) |
| `@xterm/addon-web-links` | `0.12.0` | MIT | [`LICENSES/xterm-addon-web-links-MIT.txt`](LICENSES/xterm-addon-web-links-MIT.txt) |
| `@xterm/xterm` | `6.0.0` | MIT | [`LICENSES/xterm-MIT.txt`](LICENSES/xterm-MIT.txt) |
| `circle-flags` | `2.8.3` | MIT | [`LICENSES/circle-flags-MIT.txt`](LICENSES/circle-flags-MIT.txt) |
| `@mercuryworkshop/scramjet` | `2.0.67-alpha.2` | AGPL-3.0-only | [`LICENSES/mercuryworkshop-scramjet-AGPL-3.0.txt`](LICENSES/mercuryworkshop-scramjet-AGPL-3.0.txt) |
| `@mercuryworkshop/scramjet-controller` | `0.0.14` | AGPL-3.0-only | [`LICENSES/mercuryworkshop-scramjet-AGPL-3.0.txt`](LICENSES/mercuryworkshop-scramjet-AGPL-3.0.txt) |
| `simple-icons` | `16.27.1` | CC0-1.0; referenced brands retain their trademark rights | [`LICENSES/simple-icons-CC0-1.0.txt`](LICENSES/simple-icons-CC0-1.0.txt) |
| `vue` | `3.5.40` | MIT | [`LICENSES/vue-MIT.txt`](LICENSES/vue-MIT.txt) |
| `vue-router` | `4.6.4` | MIT | [`LICENSES/vue-router-MIT.txt`](LICENSES/vue-router-MIT.txt) |

Transitive dependency versions and license identifiers are recorded in
`go.sum` and `web/package-lock.json`. Release container images additionally
publish an SBOM. Each component remains governed by its own license.

Third-party project names and logos are used only for identification. Their
appearance in KPanel does not imply sponsorship or endorsement.

## AGPL-3.0 network source offer

`@mercuryworkshop/scramjet` and `@mercuryworkshop/scramjet-controller` (used
unmodified, as compiled distribution bundles, by the desktop "browser"
feature — see `web/public/scramjet`, `web/public/controller`, and
`web/public/browser-app`) are licensed AGPL-3.0-only. KPanel itself is
AGPL-3.0-only, so combining them raises no additional relicensing
obligation, but AGPL §13 still requires that anyone interacting with this
feature over a network be offered the corresponding source. That
requirement is satisfied by the unmodified upstream source at
<https://github.com/MercuryWorkshop/scramjet> (packages `scramjet` and
`packages/controller` at the versions above) together with KPanel's own
public source repository, which carries the integration code in this
document's directories.
