package monitoring

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	defaultOperatorLatencyEvery = 5 * time.Minute
	operatorProbeTimeout        = 1500 * time.Millisecond
	operatorProbeWorkers        = 3
)

type OperatorLatencyProber interface {
	Probe(context.Context, string) (time.Duration, error)
}

type operatorLatencyProbe func(context.Context, string) (time.Duration, error)

type networkLatencyProber struct {
	probes []operatorLatencyProbe
	lookup func(context.Context, string) ([]net.IPAddr, error)
}

type dnsLatencyProbe struct {
	dialer net.Dialer
	nextID atomic.Uint32
}

type icmpLatencyProbe struct {
	nextSequence atomic.Uint32
}

// NewOperatorLatencyProber measures a bounded round trip to configured IP or DNS
// Ping targets. ICMP uses Linux ping sockets, while TCP/53 and UDP DNS provide
// fallbacks for networks that filter one of the protocols. No CAP_NET_RAW is
// added to the Agent capability boundary.
func NewOperatorLatencyProber() OperatorLatencyProber {
	dialer := net.Dialer{Timeout: operatorProbeTimeout}
	dns := &dnsLatencyProbe{dialer: dialer}
	echo := &icmpLatencyProbe{}
	return &networkLatencyProber{lookup: net.DefaultResolver.LookupIPAddr, probes: []operatorLatencyProbe{
		echo.Probe,
		func(ctx context.Context, address string) (time.Duration, error) {
			startedAt := time.Now()
			connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, "53"))
			latency := time.Since(startedAt)
			if err != nil {
				return 0, err
			}
			_ = connection.Close()
			return latency, nil
		},
		dns.Probe,
	}}
}

func (prober *networkLatencyProber) Probe(ctx context.Context, address string) (time.Duration, error) {
	host, valid := checkHost(address)
	if prober == nil || !valid || len(prober.probes) == 0 {
		return 0, errors.New("operator latency target is invalid")
	}
	probeContext, cancel := context.WithTimeout(ctx, operatorProbeTimeout)
	defer cancel()
	addresses := []string{host}
	if _, err := netip.ParseAddr(host); err != nil {
		lookup := prober.lookup
		if lookup == nil {
			lookup = net.DefaultResolver.LookupIPAddr
		}
		resolved, err := lookup(probeContext, host)
		if err != nil {
			return 0, err
		}
		// At most one IP per family bounds DNS fan-out; racing both families
		// prevents a blackholed AAAA from hiding a working A, or vice versa.
		addresses = nil
		seen := [2]bool{}
		for _, item := range resolved {
			if item.IP.To16() == nil {
				continue
			}
			family := 0
			if item.IP.To4() == nil {
				family = 1
			}
			if !seen[family] {
				addresses = append(addresses, item.String())
				seen[family] = true
			}
			if seen[0] && seen[1] {
				break
			}
		}
		if len(addresses) == 0 {
			return 0, errors.New("operator latency target has no IP addresses")
		}
	}
	type result struct {
		latency time.Duration
		err     error
	}
	count := len(addresses) * len(prober.probes)
	results := make(chan result, count)
	for _, target := range addresses {
		for _, probe := range prober.probes {
			go func(run operatorLatencyProbe, target string) {
				latency, err := run(probeContext, target)
				results <- result{latency: latency, err: err}
			}(probe, target)
		}
	}
	errorsSeen := make([]error, 0, count)
	for range count {
		select {
		case item := <-results:
			if item.err == nil {
				cancel()
				return item.latency, nil
			}
			errorsSeen = append(errorsSeen, item.err)
		case <-probeContext.Done():
			return 0, probeContext.Err()
		}
	}
	return 0, errors.Join(errorsSeen...)
}

func (prober *dnsLatencyProbe) Probe(ctx context.Context, address string) (time.Duration, error) {
	if _, err := netip.ParseAddr(address); prober == nil || err != nil {
		return 0, errors.New("DNS latency target is invalid")
	}
	connection, err := prober.dialer.DialContext(ctx, "udp", net.JoinHostPort(address, "53"))
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	deadline := time.Now().Add(operatorProbeTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return 0, err
	}
	requestID := uint16(prober.nextID.Add(1))
	request := dnsRootNSQuery(requestID)
	startedAt := time.Now()
	if _, err := connection.Write(request); err != nil {
		return 0, err
	}
	response := make([]byte, 512)
	read, err := connection.Read(response)
	latency := time.Since(startedAt)
	if err != nil {
		return 0, err
	}
	if read < 12 || binary.BigEndian.Uint16(response[:2]) != requestID || response[2]&0x80 == 0 {
		return 0, errors.New("DNS latency response is invalid")
	}
	return latency, nil
}

func (prober *icmpLatencyProbe) Probe(ctx context.Context, address string) (time.Duration, error) {
	if latency, err, handled := platformICMPProbe(ctx, address); handled {
		return latency, err
	}
	if prober == nil {
		return 0, errors.New("ICMP latency probe is unavailable")
	}
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return 0, errors.New("ICMP latency target is invalid")
	}
	destination := net.IP(ip.AsSlice())
	network, bind, protocol := "udp4", "0.0.0.0", 1
	var requestType, replyType icmp.Type = ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply
	if destination.To4() == nil {
		network, bind, protocol = "udp6", "::", 58
		requestType, replyType = ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply
	}
	connection, err := icmp.ListenPacket(network, bind)
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	deadline := time.Now().Add(operatorProbeTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return 0, err
	}
	sequence := int(prober.nextSequence.Add(1) & 0xffff)
	request, err := (&icmp.Message{
		Type: requestType,
		Code: 0,
		Body: &icmp.Echo{ID: sequence, Seq: sequence, Data: []byte("kpanel")},
	}).Marshal(nil)
	if err != nil {
		return 0, err
	}
	startedAt := time.Now()
	if _, err := connection.WriteTo(request, &net.UDPAddr{IP: destination, Zone: ip.Zone()}); err != nil {
		return 0, err
	}
	response := make([]byte, 128)
	read, peer, err := connection.ReadFrom(response)
	latency := time.Since(startedAt)
	if err != nil {
		return 0, err
	}
	peerAddress, ok := peer.(*net.UDPAddr)
	if !ok || !peerAddress.IP.Equal(destination) {
		return 0, errors.New("ICMP reply address mismatch")
	}
	message, err := icmp.ParseMessage(protocol, response[:read])
	if err != nil || message.Type != replyType || message.Code != 0 {
		return 0, errors.New("ICMP latency response is invalid")
	}
	echo, ok := message.Body.(*icmp.Echo)
	if !ok || echo.Seq != sequence {
		return 0, errors.New("ICMP latency response does not match request")
	}
	return latency, nil
}

func dnsRootNSQuery(id uint16) []byte {
	query := make([]byte, 17)
	binary.BigEndian.PutUint16(query[0:2], id)
	query[2] = 0x01                             // recursion desired
	query[5] = 0x01                             // one question
	query[12] = 0x00                            // root name
	binary.BigEndian.PutUint16(query[13:15], 2) // NS
	binary.BigEndian.PutUint16(query[15:17], 1) // IN
	return query
}

type operatorLatencyResult struct {
	target       Check
	milliseconds float64
	reachable    bool
	unknown      bool
	errorCode    string
}

func collectOperatorLatency(
	ctx context.Context,
	prober OperatorLatencyProber,
	targets []Check,
) []operatorLatencyResult {
	if prober == nil || len(targets) == 0 {
		return nil
	}
	results := make([]operatorLatencyResult, len(targets))
	jobs := make(chan int, len(targets))
	var workers sync.WaitGroup
	for worker := 0; worker < min(operatorProbeWorkers, len(targets)); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				target := targets[index]
				result := operatorLatencyResult{target: target}
				latency, err := probeCheck(ctx, prober, target)
				if err == nil {
					result.reachable = true
					result.milliseconds = float64(latency) / float64(time.Millisecond)
				} else if ctx.Err() != nil {
					result.unknown = true
					result.errorCode = "cancelled"
				} else {
					result.errorCode = checkErrorCode(err)
				}
				results[index] = result
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return results
}

func probeCheck(ctx context.Context, ping OperatorLatencyProber, target Check) (time.Duration, error) {
	probeContext, cancel := context.WithTimeout(ctx, operatorProbeTimeout)
	defer cancel()
	switch target.Kind {
	case "ping":
		return ping.Probe(probeContext, target.Target)
	case "tcp":
		host, port, err := net.SplitHostPort(target.Target)
		host, valid := checkHost(host)
		if err != nil || !valid {
			return 0, errors.New("TCP latency target is invalid")
		}
		startedAt := time.Now()
		connection, err := (&net.Dialer{Timeout: operatorProbeTimeout}).DialContext(probeContext, "tcp", net.JoinHostPort(host, port))
		latency := time.Since(startedAt)
		if err != nil {
			return 0, err
		}
		_ = connection.Close()
		return latency, nil
	case "http":
		transport := &http.Transport{
			Proxy:               nil,
			DialContext:         (&net.Dialer{Timeout: operatorProbeTimeout}).DialContext,
			TLSHandshakeTimeout: operatorProbeTimeout,
			DisableKeepAlives:   true,
		}
		client := &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				// Measure the selected endpoint's first response. A remote
				// endpoint cannot authorize requests to another destination.
				return http.ErrUseLastResponse
			},
		}
		request, err := http.NewRequestWithContext(probeContext, http.MethodGet, target.Target, nil)
		if err != nil {
			return 0, err
		}
		request.Header.Set("User-Agent", "KPanel-Monitor/1")
		startedAt := time.Now()
		response, err := client.Do(request)
		latency := time.Since(startedAt)
		if err != nil {
			return 0, err
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 400 {
			return 0, checkHTTPStatusError(response.StatusCode)
		}
		return latency, nil
	default:
		return 0, errors.New("monitoring check kind is invalid")
	}
}

type checkHTTPStatusError int

func (e checkHTTPStatusError) Error() string { return fmt.Sprintf("HTTP status %d", int(e)) }

func checkErrorCode(err error) string {
	var status checkHTTPStatusError
	var dns *net.DNSError
	var certificate *tls.CertificateVerificationError
	var network net.Error
	switch {
	case errors.As(err, &status):
		return "http_status"
	case errors.As(err, &certificate):
		return "tls"
	case errors.As(err, &dns):
		return "dns"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &network):
		if network.Timeout() {
			return "timeout"
		}
		return "connection"
	default:
		return "probe_failed"
	}
}
