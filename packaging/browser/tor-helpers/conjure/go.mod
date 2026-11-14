module gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/conjure

go 1.26.0

require (
	github.com/pires/go-proxyproto v0.8.0
	github.com/refraction-networking/conjure v0.9.1
	github.com/refraction-networking/gotapdance v1.7.10
	github.com/refraction-networking/utls v1.8.2
	gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/goptlib v1.6.0
	gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/ptutil v0.0.0-20250130151315-efaf4e0ec0d3
	gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake/v2 v2.11.0
)

require (
	filippo.io/bigmod v0.0.3 // indirect
	filippo.io/keygen v0.0.0-20240718133620-7f162efbbd87 // indirect
	github.com/andybalholm/brotli v1.3.0 // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/libp2p/go-reuseport v0.4.0 // indirect
	github.com/mroth/weightedrand v1.0.0 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pion/dtls/v2 v2.2.12 // indirect
	github.com/pion/logging v0.2.3 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.37 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.10 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/refraction-networking/ed25519 v0.1.2 // indirect
	github.com/refraction-networking/obfs4 v0.1.2 // indirect
	github.com/sergeyfrolov/bsbuffer v0.0.0-20180903213811-94e85abb8507 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)

replace (
	github.com/pion/dtls/v2 => github.com/mingyech/dtls/v2 v2.0.0
	github.com/refraction-networking/conjure v0.7.11 => github.com/refraction-networking/conjure v0.7.12-0.20250507182851-8676ab6282b8
)
