package monitoring

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
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
}

type dnsLatencyProbe struct {
	dialer net.Dialer
	nextID atomic.Uint32
}

type icmpLatencyProbe struct {
	nextSequence atomic.Uint32
}

// NewOperatorLatencyProber measures a bounded round trip to configured IPv4
// Ping targets. ICMP uses Linux ping sockets, while TCP/53 and UDP DNS provide
// fallbacks for networks that filter one of the protocols. No CAP_NET_RAW is
// added to the Agent capability boundary.
func NewOperatorLatencyProber() OperatorLatencyProber {
	dialer := net.Dialer{Timeout: operatorProbeTimeout}
	dns := &dnsLatencyProbe{dialer: dialer}
	echo := &icmpLatencyProbe{}
	return &networkLatencyProber{probes: []operatorLatencyProbe{
		echo.Probe,
		func(ctx context.Context, address string) (time.Duration, error) {
			startedAt := time.Now()
			connection, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(address, "53"))
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
	if prober == nil || net.ParseIP(address) == nil || len(prober.probes) == 0 {
		return 0, errors.New("operator latency target is invalid")
	}
	probeContext, cancel := context.WithTimeout(ctx, operatorProbeTimeout)
	defer cancel()
	type result struct {
		latency time.Duration
		err     error
	}
	results := make(chan result, len(prober.probes))
	for _, probe := range prober.probes {
		go func(run operatorLatencyProbe) {
			latency, err := run(probeContext, address)
			results <- result{latency: latency, err: err}
		}(probe)
	}
	errorsSeen := make([]error, 0, len(prober.probes))
	for range prober.probes {
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
	if prober == nil || net.ParseIP(address) == nil {
		return 0, errors.New("DNS latency target is invalid")
	}
	connection, err := prober.dialer.DialContext(ctx, "udp4", net.JoinHostPort(address, "53"))
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
	if prober == nil {
		return 0, errors.New("ICMP latency probe is unavailable")
	}
	destination := net.ParseIP(address)
	if destination == nil || destination.To4() == nil {
		return 0, errors.New("ICMP latency target is invalid")
	}
	connection, err := icmp.ListenPacket("udp4", "0.0.0.0")
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
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: sequence, Seq: sequence, Data: []byte("kpanel")},
	}).Marshal(nil)
	if err != nil {
		return 0, err
	}
	startedAt := time.Now()
	if _, err := connection.WriteTo(request, &net.UDPAddr{IP: destination}); err != nil {
		return 0, err
	}
	response := make([]byte, 128)
	read, _, err := connection.ReadFrom(response)
	latency := time.Since(startedAt)
	if err != nil {
		return 0, err
	}
	message, err := icmp.ParseMessage(1, response[:read])
	if err != nil || message.Type != ipv4.ICMPTypeEchoReply {
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
		startedAt := time.Now()
		connection, err := (&net.Dialer{Timeout: operatorProbeTimeout}).DialContext(probeContext, "tcp", target.Target)
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
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("too many redirects")
				}
				if request.URL.User != nil || (request.URL.Scheme != "http" && request.URL.Scheme != "https") {
					return errors.New("unsafe HTTP redirect")
				}
				return nil
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
			return 0, fmt.Errorf("HTTP status %d", response.StatusCode)
		}
		return latency, nil
	default:
		return 0, errors.New("monitoring check kind is invalid")
	}
}
