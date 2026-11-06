package monitoring

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestCheckAddressFormatsAndPersistence(t *testing.T) {
	valid := map[string][]string{
		"ping": {"1.1.1.1", "2606:4700:4700::1111", "example.com", "localhost", "例子.测试", "example.com.", "fe80::1%eth0", "my_service"},
		"tcp":  {"1.1.1.1:443", "[2606:4700:4700::1111]:443", "example.com:443", "localhost:80", "例子.测试:443", "[fe80::1%eth0]:443", "my_service:80"},
		"http": {"http://1.1.1.1/", "http://[::1]:8080/health", "https://example.com/health", "http://localhost/", "http://例子.测试/", "http://[fe80::1%25eth0]/", "http://my_service/"},
	}
	invalid := map[string][]string{
		"ping": {"999.1.1.1", "127.1", "[::1]", "2001:::1", "example.com:80", "https://example.com", "a/b", "-bad.example", "a..b", "a;b", strings.Repeat("a", 64) + ".com"},
		"tcp":  {"::1:443", "example.com:0", "example.com:65536", "example.com"},
		"http": {"https://user:secret@example.com/", "ftp://example.com/", "not-a-url"},
	}
	store, err := openCheckStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for kind, targets := range valid {
		for _, target := range targets {
			t.Run(kind+"/"+target, func(t *testing.T) {
				saved, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: store.snapshot().ResourceVersion, Items: []Check{{ID: "address-test", Kind: kind, Name: "Address", Target: target}}})
				if err != nil {
					t.Fatal(err)
				}
				reopened, err := openCheckStore(filepath.Dir(store.path))
				if err != nil || len(reopened.snapshot().Items) != 1 || reopened.snapshot().Items[0].Target != target || saved.Items[0].Target != target {
					t.Fatalf("target did not survive restart unchanged: %v %v", reopened, err)
				}
			})
		}
	}
	for kind, targets := range invalid {
		for _, target := range targets {
			t.Run("invalid/"+kind+"/"+target, func(t *testing.T) {
				before := store.snapshot()
				_, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: before.ResourceVersion, Items: []Check{{ID: "address-test", Kind: kind, Name: "Address", Target: target}}})
				if validationCheckField(err) != "items.target" || store.snapshot().ResourceVersion != before.ResourceVersion {
					t.Fatalf("invalid target changed config: %q err=%v", target, err)
				}
			})
		}
	}
}

func TestPingDNSFanoutAndFamilyFallback(t *testing.T) {
	for _, successful := range []string{"192.0.2.1", "2001:db8::1"} {
		t.Run(successful, func(t *testing.T) {
			called := make(chan string, 10)
			release := make(chan struct{})
			prober := &networkLatencyProber{
				lookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
					if host != "example.test" {
						t.Errorf("lookup host=%q", host)
					}
					addresses := make([]net.IPAddr, 100)
					for index := range addresses {
						addresses[index].IP = net.ParseIP("2001:db8::1")
					}
					addresses[99].IP = net.ParseIP("192.0.2.1")
					return addresses, nil
				},
				probes: []operatorLatencyProbe{func(ctx context.Context, address string) (time.Duration, error) {
					called <- address
					select {
					case <-release:
					case <-ctx.Done():
						return 0, ctx.Err()
					}
					if address == successful {
						return 7 * time.Millisecond, nil
					}
					<-ctx.Done()
					return 0, ctx.Err()
				}},
			}
			done := make(chan error, 1)
			go func() { _, err := prober.Probe(context.Background(), "example.test"); done <- err }()
			for range 2 {
				select {
				case <-called:
				case <-time.After(2 * time.Second):
					t.Fatal("both address families were not probed")
				}
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if len(called) != 0 {
				t.Fatal("DNS answers caused unbounded probe fan-out")
			}
		})
	}
}

func TestPingResolutionSharesCancellationBudget(t *testing.T) {
	var probes atomic.Int32
	prober := &networkLatencyProber{
		lookup: func(ctx context.Context, _ string) ([]net.IPAddr, error) { <-ctx.Done(); return nil, ctx.Err() },
		probes: []operatorLatencyProbe{func(context.Context, string) (time.Duration, error) { probes.Add(1); return 0, nil }},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := prober.Probe(ctx, "example.test"); !errors.Is(err, context.DeadlineExceeded) || probes.Load() != 0 {
		t.Fatalf("DNS cancellation: %v probes=%d", err, probes.Load())
	}
	for _, addresses := range [][]net.IPAddr{nil, {{IP: net.ParseIP("::1")}}, {{IP: net.ParseIP("127.0.0.1")}}} {
		prober.lookup = func(context.Context, string) ([]net.IPAddr, error) { return addresses, nil }
		_, err := prober.Probe(context.Background(), "example.test")
		if (len(addresses) == 0) != (err != nil) {
			t.Fatalf("single-stack/empty resolution: %v %v", addresses, err)
		}
	}
}

func TestTCPAndHTTPAddressFamilies(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(address, func(t *testing.T) {
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			server.Listener.Close()
			server.Listener = listener
			server.Start()
			defer server.Close()
			host, _, _ := net.SplitHostPort(listener.Addr().String())
			resolver := addressTestResolver(t, net.ParseIP(host))
			previous := net.DefaultResolver
			net.DefaultResolver = resolver
			defer func() { net.DefaultResolver = previous }()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			for _, target := range []string{listener.Addr().String(), net.JoinHostPort("address.example.test", port)} {
				for _, kind := range []string{"tcp", "http"} {
					input := target
					if kind == "http" {
						input = "http://" + target + "/health"
					}
					if _, err := probeCheck(context.Background(), fakeCheckPing{}, Check{Kind: kind, Target: input}); err != nil {
						t.Fatalf("%s %s: %v", kind, input, err)
					}
				}
			}
		})
	}
}

// A local DNS responder makes IPv4-only/AAAA-only tests independent of the
// host's generated /etc/hosts. This package's tests do not run in parallel.
func addressTestResolver(t *testing.T, ip net.IP) *net.Resolver {
	t.Helper()
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 1500)
		for {
			count, peer, err := connection.ReadFrom(buffer)
			if err != nil {
				return
			}
			var request dnsmessage.Message
			if request.Unpack(buffer[:count]) != nil {
				continue
			}
			response := dnsmessage.Message{Header: dnsmessage.Header{ID: request.ID, Response: true, RecursionAvailable: true}, Questions: request.Questions}
			for _, question := range request.Questions {
				header := dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET, Type: question.Type, TTL: 1}
				if question.Type == dnsmessage.TypeA && ip.To4() != nil {
					var address [4]byte
					copy(address[:], ip.To4())
					response.Answers = append(response.Answers, dnsmessage.Resource{Header: header, Body: &dnsmessage.AResource{A: address}})
				} else if question.Type == dnsmessage.TypeAAAA && ip.To4() == nil {
					var address [16]byte
					copy(address[:], ip.To16())
					response.Answers = append(response.Answers, dnsmessage.Resource{Header: header, Body: &dnsmessage.AAAAResource{AAAA: address}})
				}
			}
			if packet, err := response.Pack(); err == nil {
				connection.WriteTo(packet, peer)
			}
		}
	}()
	t.Cleanup(func() { connection.Close(); <-done })
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", connection.LocalAddr().String())
	}}
}

func TestICMPLoopbackAddressFamilies(t *testing.T) {
	if runtime.GOOS != "windows" && os.Getenv("KPANEL_TEST_ICMP") != "1" {
		t.Skip("Linux ping socket smoke requires an isolated network namespace with ping_group_range enabled")
	}
	for _, target := range []string{"127.0.0.1", "::1"} {
		if _, err := (&icmpLatencyProbe{}).Probe(context.Background(), target); err != nil {
			t.Fatalf("native ICMP %s: %v", target, err)
		}
	}
	if _, err := NewOperatorLatencyProber().Probe(context.Background(), "localhost"); err != nil {
		t.Fatalf("native DNS Ping localhost: %v", err)
	}
}
