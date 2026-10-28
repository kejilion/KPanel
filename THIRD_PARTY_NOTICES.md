# Third-Party Notices

KPanel is licensed under `AGPL-3.0-only`. This notice does not replace or
modify the licenses of third-party components distributed with or used by
KPanel.

## Bundled runtime components

| Component | Version or revision | License | License text |
| --- | --- | --- | --- |
| [DB-IP IP to Country Lite](https://db-ip.com/db/download/ip-to-country-lite) | October 2026 (unmodified) | CC-BY-4.0 | [`LICENSES/CC-BY-4.0.txt`](LICENSES/CC-BY-4.0.txt) |
| `github.com/containrrr/shoutrrr` | `v0.8.0` | MIT | [`LICENSES/shoutrrr-MIT.txt`](LICENSES/shoutrrr-MIT.txt) |
| `kejilion.sh` | `f031d1206224de3743845d2fc81c4801ecda32f4` | Apache-2.0 | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) |
| `github.com/flynn/noise` | `v1.1.0` | BSD-3-Clause | [`LICENSES/flynn-noise-BSD-3-Clause.txt`](LICENSES/flynn-noise-BSD-3-Clause.txt) |
| `github.com/minio/minio-go/v7` | `v7.3.0` | Apache-2.0 | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) |
| `github.com/coder/websocket` | `v1.8.15` | ISC | [`LICENSES/coder-websocket-ISC.txt`](LICENSES/coder-websocket-ISC.txt) |
| `github.com/chromedp/chromedp` (optional browser service) | `v0.21.0` | MIT | [`LICENSES/chromedp-MIT.txt`](LICENSES/chromedp-MIT.txt) |
| `github.com/chromedp/cdproto` | `v0.157.8` | MIT | [`LICENSES/cdproto-MIT.txt`](LICENSES/cdproto-MIT.txt) |
| [TigerVNC](https://github.com/TigerVNC/tigervnc) (optional browser image) | Alpine package version recorded in `/usr/share/kpanel-browser-packages.txt` | GPL-2.0-or-later and component notices | [`LICENSES/GPL-2.0.txt`](LICENSES/GPL-2.0.txt); [source and build recipe](https://git.alpinelinux.org/aports/tree/community/tigervnc?h=3.24-stable) |
| [Openbox](http://openbox.org/) (optional browser image) | Alpine package version recorded in `/usr/share/kpanel-browser-packages.txt` | GPL-2.0-or-later | [`LICENSES/GPL-2.0.txt`](LICENSES/GPL-2.0.txt); [source and build recipe](https://git.alpinelinux.org/aports/tree/community/openbox?h=3.24-stable) |
| `github.com/go-json-experiment/json` | `44df1a37e875` | BSD-3-Clause | [`LICENSES/go-json-experiment-BSD-3-Clause.txt`](LICENSES/go-json-experiment-BSD-3-Clause.txt) |
| `github.com/anacrolix/dht/v2` | `v2.23.0` | MPL-2.0 | [`LICENSES/anacrolix-dht-MPL-2.0.txt`](LICENSES/anacrolix-dht-MPL-2.0.txt) |
| `github.com/anacrolix/log` | `v0.17.1-0.20251118025802-918f1157b7bb` | MPL-2.0 | [`LICENSES/anacrolix-log-MPL-2.0.txt`](LICENSES/anacrolix-log-MPL-2.0.txt) |
| `github.com/anacrolix/torrent` | `v1.61.0` with upstream message-writer fixes ([source and modifications](third_party/torrent/KPANEL_BACKPORT.md)) | MPL-2.0 | [`LICENSES/anacrolix-torrent-MPL-2.0.txt`](LICENSES/anacrolix-torrent-MPL-2.0.txt) |
| `golang.org/x/crypto` | `v0.54.0` | BSD-3-Clause | [`LICENSES/golang-x-crypto-BSD-3-Clause.txt`](LICENSES/golang-x-crypto-BSD-3-Clause.txt) |
| `golang.org/x/image` | `v0.45.0` | BSD-3-Clause | [`LICENSES/golang-x-image-BSD-3-Clause.txt`](LICENSES/golang-x-image-BSD-3-Clause.txt) |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause | [`LICENSES/golang-x-sys-BSD-3-Clause.txt`](LICENSES/golang-x-sys-BSD-3-Clause.txt) |
| `golang.org/x/time` | `v0.15.0` | BSD-3-Clause | [`LICENSES/golang-x-time-BSD-3-Clause.txt`](LICENSES/golang-x-time-BSD-3-Clause.txt) |
| `@lucide/vue` | `1.26.0` | ISC and bundled icon notices | [`LICENSES/lucide-ISC.txt`](LICENSES/lucide-ISC.txt) |
| `@xterm/addon-fit` | `0.11.0` | MIT | [`LICENSES/xterm-addon-fit-MIT.txt`](LICENSES/xterm-addon-fit-MIT.txt) |
| `@xterm/addon-web-links` | `0.12.0` | MIT | [`LICENSES/xterm-addon-web-links-MIT.txt`](LICENSES/xterm-addon-web-links-MIT.txt) |
| `@xterm/xterm` | `6.0.0` | MIT | [`LICENSES/xterm-MIT.txt`](LICENSES/xterm-MIT.txt) |
| `circle-flags` | `2.8.3` | MIT | [`LICENSES/circle-flags-MIT.txt`](LICENSES/circle-flags-MIT.txt) |
| `simple-icons` | `16.27.1` | CC0-1.0; referenced brands retain their trademark rights | [`LICENSES/simple-icons-CC0-1.0.txt`](LICENSES/simple-icons-CC0-1.0.txt) |
| `@novnc/novnc` | `1.7.0` (unmodified; lazy console and browser client) | MPL-2.0 with bundled BSD notices | [MPL-2.0](LICENSES/noVNC-MPL-2.0.txt), [BSD](LICENSES/noVNC-BSD-3-Clause.txt), [pako](LICENSES/noVNC-pako.txt) |
| `vue` | `3.5.40` | MIT | [`LICENSES/vue-MIT.txt`](LICENSES/vue-MIT.txt) |
| `vue-router` | `4.6.4` | MIT | [`LICENSES/vue-router-MIT.txt`](LICENSES/vue-router-MIT.txt) |


IP Geolocation by [DB-IP](https://db-ip.com), published by DB-IP / Eris Networks S.A.S.
The bundled country data is redistributed unmodified under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/), separately from KPanel's
code license, without warranty. Its fixed release, official download and checksums
are recorded in [`internal/sitetraffic/geodata/NOTICE.md`](internal/sitetraffic/geodata/NOTICE.md).
The site monitoring page retains a DB-IP attribution link. Lite data has reduced
coverage and accuracy.

Transitive dependency versions are recorded in `go.sum` and
`web/package-lock.json`; these lock files are not a complete license inventory.
Release container images additionally publish an SBOM. Each component remains
governed by its own license.

Third-party project names and logos are used only for identification. Their
appearance in KPanel does not imply sponsorship or endorsement.
