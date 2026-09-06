package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
)

type summaryStatusRunner struct {
	started    chan string
	release    chan struct{}
	failF2B    bool
	timeoutF2B bool
}

func (r *summaryStatusRunner) LookPath(name string) (string, error) { return name, nil }

func (r *summaryStatusRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name != "env" || len(args) != 7 || args[2] != "LANG=C.UTF-8" || args[6] != "status" {
		return nil, errors.New("unexpected status protocol")
	}
	action := args[5]
	r.started <- action
	if r.timeoutF2B && action == "f2b" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if action == "f2b" {
		if r.failF2B {
			return nil, errors.New("SSH status unavailable")
		}
		return []byte("KPANEL_F2B_STATUS {\"installed\":true,\"running\":true,\"enabled\":true,\"autostart\":true,\"jail\":\"sshd\",\"banned\":2}\n"), nil
	}
	if action == "bbrv3" {
		return []byte("KPANEL_BBRV3_STATUS {\"supported\":true,\"installed\":false,\"active\":false,\"architecture\":\"x86_64\",\"os\":\"debian\",\"codename\":\"trixie\",\"runningKernel\":\"6.12\",\"installedKernel\":\"\",\"congestionControl\":\"cubic\",\"defaultQDisc\":\"fq_codel\",\"rebootRequired\":false,\"reason\":\"\"}\n"), nil
	}
	return nil, errors.New("unexpected action")
}

func TestSystemSummaryIndependentStatusReadsOverlapAndPreserveFailure(t *testing.T) {
	for _, failF2B := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "partial_failure"}[failF2B], func(t *testing.T) {
			server := testServer(t)
			server.system.CPUSampleInterval = 0
			server.system.PublicNetworkLookupEnabled = false
			runner := &summaryStatusRunner{started: make(chan string, 2), release: make(chan struct{}), failF2B: failF2B}
			finder := func() (string, error) { return "/trusted/kejilion.sh", nil }
			server.systemManager = systemmanage.NewManager(systemmanage.Config{
				EtcRoot: t.TempDir(), StateDir: t.TempDir(), Runner: runner, F2BScript: finder, BBRv3Script: finder,
			})
			request := httptest.NewRequest(http.MethodGet, "/v1/system/summary", nil)
			request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { server.ServeHTTP(response, request); close(done) }()
			seen := map[string]bool{}
			for i := 0; i < 2; i++ {
				select {
				case action := <-runner.started:
					seen[action] = true
				case <-time.After(time.Second):
					close(runner.release)
					<-done
					t.Fatal("independent status reads ran serially")
				}
			}
			close(runner.release)
			<-done
			if !seen["f2b"] || !seen["bbrv3"] || response.Code != http.StatusOK {
				t.Fatalf("status reads = %#v, HTTP %d", seen, response.Code)
			}
			var summary contract.SystemSummary
			if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
				t.Fatal(err)
			}
			if !summary.Management.BBRv3.Available || summary.Management.BBRv3.OS != "debian" {
				t.Fatalf("independent BBRv3 result lost: %#v", summary.Management.BBRv3)
			}
			if summary.Management.SSH.Defense.Available == failF2B {
				t.Fatalf("SSH failure misreported: %#v", summary.Management.SSH.Defense)
			}
		})
	}
}

func TestSystemSummaryCancellationStopsBothStatusReads(t *testing.T) {
	server := testServer(t)
	server.system.CPUSampleInterval = 0
	server.system.PublicNetworkLookupEnabled = false
	runner := &summaryStatusRunner{started: make(chan string, 2), release: make(chan struct{})}
	finder := func() (string, error) { return "/trusted/kejilion.sh", nil }
	server.systemManager = systemmanage.NewManager(systemmanage.Config{EtcRoot: t.TempDir(), StateDir: t.TempDir(), Runner: runner, F2BScript: finder, BBRv3Script: finder})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/system/summary", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	done := make(chan struct{})
	go func() { server.ServeHTTP(httptest.NewRecorder(), request); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-runner.started:
		case <-time.After(time.Second):
			t.Fatal("status read did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled summary left a status read running")
	}
}

func TestSystemSummaryFinishesCollectionBeforeStatusReadsAndKeepsPartialTimeout(t *testing.T) {
	server := testServer(t)
	server.system.CPUSampleInterval = 0
	collecting := make(chan struct{})
	finishCollection := make(chan struct{})
	server.system.PublicNetworkLookupEnabled = false
	server.system.Now = func() time.Time {
		close(collecting)
		<-finishCollection
		return time.Now()
	}
	runner := &summaryStatusRunner{started: make(chan string, 2), release: make(chan struct{}), timeoutF2B: true}
	close(runner.release)
	finder := func() (string, error) { return "/trusted/kejilion.sh", nil }
	server.systemManager = systemmanage.NewManager(systemmanage.Config{EtcRoot: t.TempDir(), StateDir: t.TempDir(), Runner: runner, F2BScript: finder, BBRv3Script: finder})
	request := httptest.NewRequest(http.MethodGet, "/v1/system/summary", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { server.ServeHTTP(response, request); close(done) }()
	<-collecting
	select {
	case <-runner.started:
		close(finishCollection)
		<-done
		t.Fatal("status subprocess started before collection finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(finishCollection)
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("status timeout did not bound summary")
	}
	var summary contract.SystemSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || summary.Management.SSH.Defense.Available || !summary.Management.BBRv3.Available {
		t.Fatalf("status timeout lost partial result: HTTP %d, %#v", response.Code, summary.Management)
	}
}
