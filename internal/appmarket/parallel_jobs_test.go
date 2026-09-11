package appmarket

import (
	"context"
	"errors"
	"fmt"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func parallelTestService(t testing.TB) *Service {
	t.Helper()
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.configureJobs(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), &fakeJobRunner{unitState: "active"}); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "kejilion.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\nKPANEL_APP_CONCURRENCY_PROTOCOL_VERSION=\"1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
	service.scriptInteractiveFinder = func() (string, error) { return script, nil }
	service.scriptInteractiveManageFinder = service.scriptInteractiveFinder
	service.listeningPorts = func(context.Context) (map[uint16][]string, error) { return map[uint16][]string{}, nil }
	return service
}

func TestDifferentApplicationShellsStartConcurrently(t *testing.T) {
	service := parallelTestService(t)
	ids := []string{"builtin-28", "builtin-64", "builtin-76", "builtin-4"}
	start := make(chan struct{})
	results := make(chan error, len(ids))
	var workers sync.WaitGroup
	for index, id := range ids {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := service.StartInstall(context.Background(), id, InstallInput{HostPort: uint16(18080 + index)})
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			t.Logf("start result: %v", err)
		}
	}
	t.Logf("independent applications accepted=%d attempted=%d", accepted, len(ids))
	if accepted != len(ids) {
		t.Fatalf("accepted %d independent applications, want %d", accepted, len(ids))
	}
}

func BenchmarkApplicationJobLogTail(b *testing.B) {
	service := parallelTestService(b)
	id := strings.Repeat("a", 32)
	line := strings.Repeat("x", 126) + "\n"
	if err := os.WriteFile(service.jobs.logPath(id), []byte(strings.Repeat(line, (8<<20)/len(line))), 0600); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if lines := service.jobs.logTail(id, 120); len(lines) != 120 {
			b.Fatal(fmt.Sprintf("tail lines=%d", len(lines)))
		}
	}
}

func TestParallelAdmissionResourceReservations(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*appJobRecord)
		want   error
	}{
		{"same app", func(r *appJobRecord) { r.AppID = "builtin-28" }, ErrTaskConflict},
		{"selector alias", func(r *appJobRecord) { r.Selector = "28" }, ErrTaskConflict},
		{"container alias", func(r *appJobRecord) { r.ExpectedContainerID = strings.Repeat("c", 64) }, ErrTaskConflict},
		{"shared directory", func(r *appJobRecord) { r.ResourceKeys = []string{"data:webtop"} }, ErrTaskConflict},
		{"reserved installed port", func(r *appJobRecord) { r.ResourceKeys = []string{"port:18080"} }, ErrPortConflict},
		{"old active worker", func(r *appJobRecord) { r.ParallelSafe = false }, ErrTaskConflict},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s := parallelTestService(t)
			existing := appJobRecord{AppJob: AppJob{ID: strings.Repeat("a", 32), AppID: "other", Status: "running"}, ParallelSafe: true, Selector: "other", ResourceKeys: []string{"other"}}
			scenario.mutate(&existing)
			if err := s.jobs.put(existing); err != nil {
				t.Fatal(err)
			}
			candidate := appJobRecord{AppJob: AppJob{AppID: "builtin-28"}, ParallelSafe: true, Selector: "28", HostPort: 18080, ExpectedContainerID: strings.Repeat("c", 64), ResourceKeys: []string{"port:18080", "data:webtop"}}
			if err := s.jobs.canStart(candidate); !errors.Is(err, scenario.want) {
				t.Fatalf("got %v, want %v", err, scenario.want)
			}
		})
	}
}

func TestParallelAdmissionAtomicLimits(t *testing.T) {
	for _, samePort := range []bool{false, true} {
		t.Run(fmt.Sprint(samePort), func(t *testing.T) {
			s := parallelTestService(t)
			ids := []string{"builtin-28", "builtin-64", "builtin-76", "builtin-4", "builtin-13", "builtin-24"}
			start := make(chan struct{})
			results := make(chan error, len(ids))
			for index, id := range ids {
				go func() {
					<-start
					port := uint16(18080 + index)
					if samePort {
						port = 18080
					}
					_, err := s.StartInstall(context.Background(), id, InstallInput{HostPort: port})
					results <- err
				}()
			}
			close(start)
			accepted := 0
			for range ids {
				err := <-results
				if err == nil {
					accepted++
				} else if !errors.Is(err, ErrTaskLimit) && !errors.Is(err, ErrPortConflict) {
					t.Errorf("unexpected admission error: %v", err)
				}
			}
			want := 4
			if samePort {
				want = 1
			}
			if accepted != want {
				t.Fatalf("accepted=%d want=%d", accepted, want)
			}
		})
	}
}

func TestParallelReservationsSurviveRestartAndCancellation(t *testing.T) {
	s := parallelTestService(t)
	a, err := s.StartInstall(context.Background(), "builtin-28", InstallInput{HostPort: 18080})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.StartInstall(context.Background(), "builtin-64", InstallInput{HostPort: 18081})
	if err != nil {
		t.Fatal(err)
	}
	state, executable := s.jobs.stateDir, s.jobExecutable
	if err := s.configureJobs(state, executable, &fakeJobRunner{unitState: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelAppJob(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartInstall(context.Background(), "builtin-76", InstallInput{HostPort: 18080}); !errors.Is(err, ErrPortConflict) {
		t.Fatalf("cancelling task released port too early: %v", err)
	}
	other, err := s.jobs.read(b.ID)
	if err != nil || other.Stage == "cancelling" {
		t.Fatalf("other task affected: %#v %v", other, err)
	}
	first, _ := s.jobs.read(a.ID)
	s.finishCancelledJob(first)
	if _, err := s.StartInstall(context.Background(), "builtin-76", InstallInput{HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
}

func TestParallelOldScriptRemainsExclusive(t *testing.T) {
	s := parallelTestService(t)
	path, _ := s.scriptInteractiveFinder()
	if err := os.WriteFile(path, []byte("#!/bin/bash\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartInstall(context.Background(), "builtin-28", InstallInput{HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartInstall(context.Background(), "builtin-64", InstallInput{HostPort: 18081}); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("old script allowed parallel: %v", err)
	}
}

func TestParallelResourceKeysIncludeAllRuntimePorts(t *testing.T) {
	s := parallelTestService(t)
	item, err := s.Find(context.Background(), "builtin-28")
	if err != nil {
		t.Fatal(err)
	}
	item.Runtime.Ports = []contract.PortBinding{{PublicPort: 18080}, {PublicPort: 18081, Type: "udp"}}
	keys := strings.Join(s.appJobResourceKeys(item), ",")
	if !strings.Contains(keys, "port:18080") || !strings.Contains(keys, "port:18081") {
		t.Fatalf("missing port reservations: %s", keys)
	}
}

func TestParallelInstallReservesFixedAdditionalPortsBeforeContainerExists(t *testing.T) {
	for _, sample := range []struct {
		app                 string
		primary, additional uint16
	}{
		{"builtin-4", 18081, 80}, {"builtin-4", 18081, 443},
		{"builtin-8", 18081, 56881}, {"builtin-17", 18081, 53},
		{"builtin-69", 18081, 22022}, {"builtin-70", 18081, 11451},
		{"builtin-86", 18081, 7359}, {"builtin-88", 18081, 1935},
		{"builtin-100", 18081, 22000},
	} {
		t.Run(fmt.Sprintf("%s-%d", sample.app, sample.additional), func(t *testing.T) {
			s := parallelTestService(t)
			if _, err := s.StartInstall(context.Background(), sample.app, InstallInput{HostPort: sample.primary}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartInstall(context.Background(), "builtin-28", InstallInput{HostPort: sample.additional}); !errors.Is(err, ErrPortConflict) {
				t.Fatalf("unreserved extra port %d: %v", sample.additional, err)
			}
		})
	}
}

func TestAppJobPollOnlyReconcilesRequestedUnit(t *testing.T) {
	s := parallelTestService(t)
	for index, id := range []string{"builtin-28", "builtin-64", "builtin-76", "builtin-4"} {
		job, err := s.StartInstall(context.Background(), id, InstallInput{HostPort: uint16(18080 + index)})
		if err != nil {
			t.Fatal(err)
		}
		record, _ := s.jobs.read(job.ID)
		record.CreatedAt = time.Now().Add(-time.Hour)
		if err := s.jobs.put(record); err != nil {
			t.Fatal(err)
		}
	}
	runner := s.jobRunner.(*fakeJobRunner)
	runner.calls = nil
	if _, err := s.AppJob(s.jobs.list()[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("one status poll spawned %d unit queries", len(runner.calls))
	}
}
