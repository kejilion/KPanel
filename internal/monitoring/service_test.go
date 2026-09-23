package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

type fakeSystemSource struct {
	summary contract.SystemSummary
	err     error
}

func (source fakeSystemSource) CollectRuntime(context.Context) (contract.SystemSummary, error) {
	return source.summary, source.err
}

type fakeDockerSource struct {
	batch dockerx.ContainerMetricBatch
	err   error
	calls int
}

type fakeOperatorLatencyProber struct {
	mu        sync.Mutex
	active    int
	maxActive int
	calls     []string
	failures  map[string]bool
}

func (prober *fakeOperatorLatencyProber) Probe(
	ctx context.Context,
	address string,
) (time.Duration, error) {
	prober.mu.Lock()
	prober.active++
	if prober.active > prober.maxActive {
		prober.maxActive = prober.active
	}
	prober.calls = append(prober.calls, address)
	prober.mu.Unlock()
	select {
	case <-time.After(time.Millisecond):
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	prober.mu.Lock()
	prober.active--
	fails := prober.failures[address]
	prober.mu.Unlock()
	if fails {
		return 0, errors.New("unreachable")
	}
	return time.Duration(len(address)) * time.Millisecond, nil
}

func (source *fakeDockerSource) RunningContainerStats(
	context.Context,
	int,
	int,
) (dockerx.ContainerMetricBatch, error) {
	source.calls++
	return source.batch, source.err
}

func testSummary(rx, tx uint64) contract.SystemSummary {
	return contract.SystemSummary{
		CPU:  contract.CPUSummary{Cores: 4, UsagePercent: 25},
		Load: contract.LoadSummary{One: 0.5, Five: 0.4, Fifteen: 0.3},
		Memory: contract.MemorySummary{
			UsedBytes: 2 << 30, TotalBytes: 8 << 30,
			SwapUsedBytes: 128 << 20, SwapTotalBytes: 1 << 30,
		},
		Disks: []contract.DiskSummary{{
			MountPoint: "/", UsedBytes: 20 << 30, TotalBytes: 100 << 30, UsagePercent: 20,
		}},
		DiskIO: contract.DiskIOSummary{
			Available: true, ReadBytes: rx * 2, WriteBytes: tx * 2,
		},
		Network: contract.NetworkSummary{
			ReceivedBytes: rx, SentBytes: tx, TCPConnections: 12, UDPConnections: 3,
		},
	}
}

func TestSampleAndHistoryPersistBoundedHostAndContainerMetrics(t *testing.T) {
	current := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	system := &fakeSystemSource{summary: testSummary(1_000, 2_000)}
	docker := &fakeDockerSource{batch: dockerx.ContainerMetricBatch{
		Total: 1,
		Items: []dockerx.ContainerMetricSample{{
			Name: "web", Image: "nginx:latest",
			ContainerStats: dockerx.ContainerStats{
				ContainerID: strings.Repeat("a", 64), CPUPercent: 12,
				MemoryBytes: 128 << 20, MemoryLimit: 1 << 30, MemoryPercent: 12.5,
				NetworkRx: 100, NetworkTx: 200, BlockRead: 1_000, BlockWrite: 2_000, PIDs: 5,
			},
		}},
	}}
	service, err := New(Config{
		StateDir: t.TempDir(), System: system, Docker: docker,
		Now:               func() time.Time { return current },
		ContainerInterval: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	system.summary = testSummary(1_600, 2_900)
	docker.batch.Items[0].NetworkRx = 400
	docker.batch.Items[0].NetworkTx = 500
	docker.batch.Items[0].BlockRead = 1_600
	docker.batch.Items[0].BlockWrite = 3_200
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}

	history, err := service.History(context.Background(), "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Host) != 2 || history.Host[1].NetworkRxRate != 10 ||
		history.Host[1].NetworkTxRate != 15 || history.Host[1].DiskReadRate != 20 ||
		history.Host[1].DiskWriteRate != 30 {
		t.Fatalf("unexpected host history: %#v", history.Host)
	}
	if len(history.Containers) != 1 || len(history.Containers[0].Points) != 2 ||
		history.Containers[0].Points[1].NetworkRxRate != 5 ||
		history.Containers[0].Points[1].BlockReadRate != 10 ||
		history.Containers[0].Points[1].BlockWriteRate != 20 {
		t.Fatalf("unexpected container history: %#v", history.Containers)
	}
	if history.Storage.StorageBytes <= 0 || history.Storage.LastContainerRecorded != 1 ||
		history.Storage.RetentionDays != defaultRetentionDays ||
		history.Storage.MaxStorageBytes != defaultMaxStorageBytes ||
		docker.calls != 2 {
		t.Fatalf("unexpected storage status: %#v calls=%d", history.Storage, docker.calls)
	}
	info, err := os.Stat(filepath.Join(service.stateDir, "metrics-2026-07-31.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("history shard permissions = %o", info.Mode().Perm())
	}
}

func TestContainerHistoryCPUUsesIntervalCounters(t *testing.T) {
	current := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	system := &fakeSystemSource{summary: testSummary(1_000, 2_000)}
	docker := &fakeDockerSource{batch: dockerx.ContainerMetricBatch{
		Total: 1,
		Items: []dockerx.ContainerMetricSample{{
			Name: "web", Image: "nginx:latest",
			ContainerStats: dockerx.ContainerStats{
				ContainerID: strings.Repeat("a", 64), CPUPercent: 99,
				CPUTotalUsage: 1_000, SystemCPUUsage: 10_000,
				CPUOnlineCPUs: 2, CPUCountersAvailable: true,
			},
		}},
	}}
	service, err := New(Config{
		StateDir: t.TempDir(), System: system, Docker: docker,
		Now: func() time.Time { return current }, ContainerInterval: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	docker.batch.Items[0].CPUTotalUsage = 1_250
	docker.batch.Items[0].SystemCPUUsage = 11_000
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}

	history, err := service.History(context.Background(), "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Containers) != 1 || len(history.Containers[0].Points) != 2 {
		t.Fatalf("unexpected container history: %#v", history.Containers)
	}
	points := history.Containers[0].Points
	if points[0].CPUPercent != 0 || points[1].CPUPercent != 50 {
		t.Fatalf("container CPU history = %#v, want first baseline 0 then interval 50%%", points)
	}
}

func TestContainerHistoryCPUResetsInvalidCounters(t *testing.T) {
	previous := containerCPUCounter{
		totalUsage: 2_000, systemUsage: 20_000, onlineCPUs: 2, available: true,
	}
	multiCore := containerCPUCounter{
		totalUsage: 3_000, systemUsage: 21_000, onlineCPUs: 4, available: true,
	}
	if got := containerCPUPercent(previous, multiCore); got != 400 {
		t.Fatalf("multi-core CPU = %f, want Docker-compatible 400%%", got)
	}
	reset := containerCPUCounter{
		totalUsage: 100, systemUsage: 21_000, onlineCPUs: 2, available: true,
	}
	if got := containerCPUPercent(previous, reset); got != 0 {
		t.Fatalf("CPU after counter reset = %f, want 0", got)
	}
	if got := containerCPUPercent(previous, containerCPUCounter{}); got != 0 {
		t.Fatalf("CPU without counters = %f, want 0", got)
	}
}

func TestContainerHistoryCPUResetsBaselineAfterCollectionFailure(t *testing.T) {
	service := &Service{containerCPU: make(map[string]containerCPUCounter)}
	id := strings.Repeat("b", 64)
	first := dockerx.ContainerMetricSample{ContainerStats: dockerx.ContainerStats{
		ContainerID: id, CPUTotalUsage: 1_000, SystemCPUUsage: 10_000,
		CPUOnlineCPUs: 2, CPUCountersAvailable: true,
	}}
	second := first
	second.CPUTotalUsage = 1_250
	second.SystemCPUUsage = 11_000
	if got := service.containerCPUPercentages([]dockerx.ContainerMetricSample{first})[0]; got != 0 {
		t.Fatalf("first CPU sample = %f, want 0", got)
	}
	service.resetContainerCPU()
	if got := service.containerCPUPercentages([]dockerx.ContainerMetricSample{second})[0]; got != 0 {
		t.Fatalf("CPU after failed collection reset = %f, want 0", got)
	}
}

func TestOperatorLatencyCollectionIsFixedBoundedAndPersisted(t *testing.T) {
	current := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	targets := DefaultChecks()
	failedAddress := targets[2].Target
	prober := &fakeOperatorLatencyProber{failures: map[string]bool{failedAddress: true}}
	service, err := New(Config{
		StateDir: t.TempDir(), System: fakeSystemSource{summary: testSummary(1, 2)},
		Now: func() time.Time { return current }, OperatorLatency: prober,
		OperatorLatencyInterval: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := service.Status()
	if !status.OperatorLatencyAvailable || status.OperatorLatencyIntervalSeconds != 60 ||
		status.LastOperatorLatencySuccessful != len(targets)-1 ||
		status.LastOperatorLatencyFailed != 1 {
		t.Fatalf("unexpected operator latency status: %#v", status)
	}
	prober.mu.Lock()
	callCount, maxActive := len(prober.calls), prober.maxActive
	prober.mu.Unlock()
	if callCount != len(targets) || maxActive > operatorProbeWorkers {
		t.Fatalf("operator probe calls=%d max active=%d", callCount, maxActive)
	}
	history, err := service.History(context.Background(), "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.OperatorLatency) != len(targets) {
		t.Fatalf("operator series count=%d", len(history.OperatorLatency))
	}
	for index, series := range history.OperatorLatency {
		target := targets[index]
		if series.ID != target.ID || series.Operator != target.Operator ||
			series.Region != target.Region || series.Address != target.Target || len(series.Points) != 1 {
			t.Fatalf("unexpected operator series at %d: %#v", index, series)
		}
		if target.Target == failedAddress {
			if series.Points[0].LatencyMilliseconds != nil {
				t.Fatalf("failed probe was recorded as latency: %#v", series.Points[0])
			}
		} else if series.Points[0].LatencyMilliseconds == nil {
			t.Fatalf("successful probe was recorded as missing: %#v", series.Points[0])
		}
	}
}

func TestOperatorLatencyCatalogIsNineFixedIPv4Targets(t *testing.T) {
	targets := DefaultChecks()
	if len(targets) != 9 {
		t.Fatalf("operator target count=%d", len(targets))
	}
	seen := make(map[string]struct{}, len(targets))
	counts := make(map[string]int)
	for _, target := range targets {
		if target.ID != fmt.Sprintf("%s-%s", target.Operator, target.Region) {
			t.Fatalf("target id mismatch: %#v", target)
		}
		if ip := net.ParseIP(target.Target); ip == nil || ip.To4() == nil {
			t.Fatalf("target is not fixed IPv4: %#v", target)
		}
		if _, exists := seen[target.Target]; exists {
			t.Fatalf("duplicate target address: %s", target.Target)
		}
		seen[target.Target] = struct{}{}
		counts[target.Operator]++
	}
	for _, operator := range []string{"telecom", "unicom", "mobile"} {
		if counts[operator] != 3 {
			t.Fatalf("operator %s target count=%d", operator, counts[operator])
		}
	}
}

func TestDNSRootNSQueryUsesBoundedValidatedPacket(t *testing.T) {
	query := dnsRootNSQuery(0x1234)
	if len(query) != 17 || query[0] != 0x12 || query[1] != 0x34 ||
		query[2] != 0x01 || query[5] != 0x01 || query[12] != 0 ||
		query[13] != 0 || query[14] != 2 || query[15] != 0 || query[16] != 1 {
		t.Fatalf("unexpected DNS query: %v", query)
	}
}

func TestNetworkLatencyProbeUsesFirstSuccessfulProtocol(t *testing.T) {
	prober := &networkLatencyProber{probes: []operatorLatencyProbe{
		func(context.Context, string) (time.Duration, error) {
			return 0, errors.New("filtered")
		},
		func(context.Context, string) (time.Duration, error) {
			return 42 * time.Millisecond, nil
		},
	}}
	latency, err := prober.Probe(context.Background(), "192.0.2.1")
	if err != nil || latency != 42*time.Millisecond {
		t.Fatalf("latency=%s err=%v", latency, err)
	}
}

func TestNetworkLatencyProbeRejectsInvalidTargetsAndAllFailures(t *testing.T) {
	prober := &networkLatencyProber{probes: []operatorLatencyProbe{
		func(context.Context, string) (time.Duration, error) {
			return 0, errors.New("filtered")
		},
	}}
	if _, err := prober.Probe(context.Background(), "example.com"); err == nil {
		t.Fatal("invalid target was accepted")
	}
	if _, err := prober.Probe(context.Background(), "192.0.2.1"); err == nil {
		t.Fatal("failed protocols were reported as a latency")
	}
}

func TestSortContainerSeriesPrioritizesFreshMemoryThenCPU(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	series := []contract.MonitoringContainerSeries{
		{ContainerID: "empty", Name: "empty"},
		{
			ContainerID: "same-zeta", Name: "zeta",
			Points: []contract.MonitoringContainerPoint{{
				CollectedAt: now, MemoryBytes: 3 << 30, CPUPercent: 5,
			}},
		},
		{
			ContainerID: "same-alpha", Name: "alpha",
			Points: []contract.MonitoringContainerPoint{{
				CollectedAt: now, MemoryBytes: 3 << 30, CPUPercent: 5,
			}},
		},
		{
			ContainerID: "older", Name: "older",
			Points: []contract.MonitoringContainerPoint{{
				CollectedAt: now.Add(-5 * time.Minute), MemoryBytes: 8 << 30, CPUPercent: 100,
			}},
		},
		{
			ContainerID: "lower-memory", Name: "lower-memory",
			Points: []contract.MonitoringContainerPoint{{
				CollectedAt: now, MemoryBytes: 1 << 30, CPUPercent: 90,
			}},
		},
		{
			ContainerID: "lower-cpu", Name: "lower-cpu",
			Points: []contract.MonitoringContainerPoint{{
				CollectedAt: now, MemoryBytes: 2 << 30, CPUPercent: 20,
			}},
		},
		{
			ContainerID: "higher-cpu", Name: "higher-cpu",
			Points: []contract.MonitoringContainerPoint{{
				CollectedAt: now, MemoryBytes: 2 << 30, CPUPercent: 40,
			}},
		},
	}

	sortContainerSeries(series)
	want := []string{"same-alpha", "same-zeta", "higher-cpu", "lower-cpu", "lower-memory", "older", "empty"}
	for index, containerID := range want {
		if series[index].ContainerID != containerID {
			t.Fatalf("container order at %d = %q, want %q: %#v", index, series[index].ContainerID, containerID, series)
		}
	}
}

func TestHistorySkipsCorruptRecordsAndRejectsUnknownRange(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	service, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "metrics-2026-07-31.jsonl"),
		[]byte("{not-json}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	history, err := service.History(context.Background(), "24h")
	if err != nil {
		t.Fatal(err)
	}
	if history.SkippedLines != 1 || len(history.Host) != 0 {
		t.Fatalf("corrupt history result = %#v", history)
	}
	if _, err := service.History(context.Background(), "31d"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("unknown range error = %v", err)
	}
}

func TestLongHistoryUsesHourlyRollupsWithBoundedBuckets(t *testing.T) {
	now := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	current := now.Add(-time.Minute)
	service, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(1_000, 2_000)},
		Now:      func() time.Time { return current },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	current = now
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "hourly-2026-08.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 0 {
		t.Fatalf("hourly rollup was not persisted: size=%d", info.Size())
	}

	tests := []struct {
		rangeValue string
		duration   time.Duration
		bucket     time.Duration
	}{
		{rangeValue: "3m", duration: 90 * 24 * time.Hour, bucket: 6 * time.Hour},
		{rangeValue: "6m", duration: 180 * 24 * time.Hour, bucket: 12 * time.Hour},
		{rangeValue: "12m", duration: 365 * 24 * time.Hour, bucket: 24 * time.Hour},
	}
	for _, test := range tests {
		spec, parseErr := parseRange(test.rangeValue)
		if parseErr != nil || spec.duration != test.duration || spec.bucket != test.bucket ||
			spec.querySlots != 2 || !spec.hourly {
			t.Fatalf("unexpected %s range: %#v err=%v", test.rangeValue, spec, parseErr)
		}
		history, historyErr := service.History(context.Background(), test.rangeValue)
		if historyErr != nil {
			t.Fatal(historyErr)
		}
		if len(history.Host) != 1 || !history.Host[0].CollectedAt.Equal(current) ||
			history.ScannedBytes != info.Size() ||
			history.BucketSeconds != int(test.bucket.Seconds()) {
			t.Fatalf("unexpected %s history: %#v", test.rangeValue, history)
		}
	}
}

func TestHistoryBetweenValidatesBoundsAndUsesBoundedBuckets(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: t.TempDir(),
		System:   fakeSystemSource{summary: testSummary(1_000, 2_000)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}

	invalid := [][2]time.Time{
		{now.Add(-time.Hour), now.Add(-time.Hour)},
		{now, now.Add(time.Minute)},
		{now.Add(-7 * time.Hour), now},
		{now.Add(-31 * 24 * time.Hour), now.Add(-31*24*time.Hour + time.Hour)},
	}
	for _, window := range invalid {
		if _, historyErr := service.HistoryBetween(
			context.Background(), "6h", window[0], window[1],
		); !errors.Is(historyErr, ErrInvalidWindow) {
			t.Fatalf("invalid window %s..%s error = %v", window[0], window[1], historyErr)
		}
	}

	tests := []struct {
		duration time.Duration
		hourly   bool
		want     time.Duration
	}{
		{duration: time.Hour, want: time.Minute},
		{duration: 24 * time.Hour, want: 5 * time.Minute},
		{duration: 30 * 24 * time.Hour, want: 2 * time.Hour},
		{duration: 24 * time.Hour, hourly: true, want: time.Hour},
		{duration: 180 * 24 * time.Hour, hourly: true, want: 12 * time.Hour},
	}
	for _, test := range tests {
		if got := dynamicHistoryBucket(test.duration, test.hourly); got != test.want {
			t.Fatalf("bucket duration=%s hourly=%v got=%s want=%s", test.duration, test.hourly, got, test.want)
		}
	}
}

func TestHistoryBetweenChoosesRawOrHourlyBySelectionAge(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: t.TempDir(),
		System:   fakeSystemSource{summary: testSummary(1_000, 2_000)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	recentAt := now.Add(-2 * time.Hour)
	oldAt := now.Add(-60 * 24 * time.Hour)
	if err := service.appendRecord(maximumRollupRecord(recentAt)); err != nil {
		t.Fatal(err)
	}
	if err := service.appendHourlyRecord(maximumRollupRecord(oldAt)); err != nil {
		t.Fatal(err)
	}

	recent, err := service.HistoryBetween(
		context.Background(), "12m", recentAt.Add(-time.Hour), recentAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Host) != 1 || !recent.Host[0].CollectedAt.Equal(recentAt) ||
		recent.BucketSeconds != int(time.Minute.Seconds()) {
		t.Fatalf("recent custom history = %#v", recent)
	}

	old, err := service.HistoryBetween(
		context.Background(), "12m", oldAt.Add(-time.Hour), oldAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Host) != 1 || !old.Host[0].CollectedAt.Equal(oldAt) ||
		old.BucketSeconds != int(time.Hour.Seconds()) {
		t.Fatalf("old custom history = %#v", old)
	}
}

func TestHourlyRollupRestoresCurrentHourAfterRestart(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 8, 5, 0, 10, 0, 0, time.UTC)
	system := &fakeSystemSource{summary: testSummary(1_000, 2_000)}
	service, err := New(Config{
		StateDir: dir, System: system, Now: func() time.Time { return current },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}

	current = current.Add(10 * time.Minute)
	restarted, err := New(Config{
		StateDir: dir, System: system, Now: func() time.Time { return current },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	current = time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	if err := restarted.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, err := restarted.History(context.Background(), "3m")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Host) != 1 || !history.Host[0].CollectedAt.Equal(current) {
		t.Fatalf("restart did not restore and close current hour: %#v", history.Host)
	}
	var persisted []diskRecord
	if _, _, err := restarted.scanHourlyRecords(
		context.Background(), current.Add(-time.Hour), current,
		func(record diskRecord) {
			record.Containers = append([]diskContainerPoint(nil), record.Containers...)
			record.OperatorLatency = append([]diskOperatorLatencyPoint(nil), record.OperatorLatency...)
			persisted = append(persisted, record)
		},
	); err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 ||
		!persisted[0].CollectedAt.Equal(time.Date(2026, 8, 5, 0, 20, 0, 0, time.UTC)) {
		t.Fatalf("restart did not persist restored partial hour: %#v", persisted)
	}
}

func TestLongHistorySkipsCorruptRollupsAndPrunesExpiredMonths(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	old := filepath.Join(dir, "hourly-2025-07.jsonl")
	boundary := filepath.Join(dir, "hourly-2025-08.jsonl")
	current := filepath.Join(dir, "hourly-2026-08.jsonl")
	for _, path := range []string{old, boundary} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(current, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired hourly shard remains: %v", err)
	}
	if _, err := os.Stat(boundary); err != nil {
		t.Fatalf("boundary hourly shard was removed: %v", err)
	}
	history, err := service.History(context.Background(), "12m")
	if err != nil {
		t.Fatal(err)
	}
	if history.SkippedLines != 2 || len(history.Host) != 0 {
		t.Fatalf("corrupt hourly history result = %#v", history)
	}
}

func TestLongHistoryPrioritizesRecentlyActiveContainersAcrossChurn(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: t.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	const containerCount = maxScannedSeries + 8
	for index := 0; index < containerCount; index++ {
		at := now.Add(-time.Duration(containerCount-index) * time.Hour)
		record := diskRecord{
			Version: recordVersion, CollectedAt: at,
			Host:             hostPoint(testSummary(uint64(index), uint64(index)), at),
			ContainerSampled: true, DockerAvailable: true, ContainerTotal: 1,
			Containers: []diskContainerPoint{{
				ID: fmt.Sprintf("%064x", index), Name: fmt.Sprintf("container-%02d", index),
				Image: "example:test", MemoryBytes: uint64(index + 1), MemoryLimitBytes: 1 << 30,
			}},
		}
		if err := service.appendHourlyRecord(record); err != nil {
			t.Fatal(err)
		}
	}
	history, err := service.History(context.Background(), "3m")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Containers) != maxHistorySeries {
		t.Fatalf("container series=%d, want %d", len(history.Containers), maxHistorySeries)
	}
	newestID := fmt.Sprintf("%064x", containerCount-1)
	foundNewest := false
	for _, series := range history.Containers {
		if series.ContainerID == newestID {
			foundNewest = true
		}
	}
	if !foundNewest {
		t.Fatalf("newest container %q was crowded out by stale series", newestID)
	}
}

func TestMaximumHourlyRollupFitsAnnualStorageBudget(t *testing.T) {
	at := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	compacted := compactHourlyRecord(maximumRollupRecord(at))
	data, err := json.Marshal(compacted)
	if err != nil {
		t.Fatal(err)
	}
	// Monthly shards retain the boundary month, so physical storage can include
	// up to 31 additional days even though queries remain limited to 365 days.
	projected := int64(len(data)+1) * 24 * (defaultRollupDays + 31)
	t.Logf("maximum hourly rollup=%d bytes/record annual=%d bytes", len(data), projected)
	if projected >= defaultMaxRollupBytes {
		t.Fatalf("maximum annual rollup projection=%d, budget=%d", projected, defaultMaxRollupBytes)
	}
	for _, container := range compacted.Containers {
		if len(container.Name) > maxHistoricalContainerMetadataBytes || container.Image != "" {
			t.Fatalf("unbounded hourly container metadata: %#v", container)
		}
	}
}

func TestHourlyRollupKeepsPeaksAndLatestCounters(t *testing.T) {
	at := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	first := maximumRollupRecord(at)
	first.Host.NetworkRxPeakRate = 0
	first.Containers[0].NetworkRxPeakRate = 0
	first.Host.CPUPercent = 90
	first.Host.MemoryUsedBytes = 6
	first.Host.MemoryTotalBytes = 10
	first.Host.NetworkRxBytes = 100
	first.Containers[0].CPUPercent = 80
	first.Containers[0].MemoryPercent = 70
	first.Containers[0].NetworkRxBytes = 100
	middle := maximumRollupRecord(at.Add(time.Minute))
	middle.Host.NetworkRxPeakRate = 0
	middle.Containers[0].NetworkRxPeakRate = 0
	middle.Host.CPUPercent = 50
	middle.Host.MemoryUsedBytes = 4
	middle.Host.MemoryTotalBytes = 10
	middle.Host.NetworkRxBytes = 400
	middle.Containers[0].CPUPercent = 50
	middle.Containers[0].MemoryPercent = 50
	middle.Containers[0].NetworkRxBytes = 400
	last := maximumRollupRecord(at.Add(59 * time.Minute))
	last.Host.NetworkRxPeakRate = 0
	last.Containers[0].NetworkRxPeakRate = 0
	last.Host.CPUPercent = 10
	last.Host.MemoryUsedBytes = 2
	last.Host.MemoryTotalBytes = 10
	last.Host.NetworkRxBytes = 500
	last.Containers[0].CPUPercent = 20
	last.Containers[0].MemoryPercent = 30
	last.Containers[0].NetworkRxBytes = 500
	first.OperatorLatency[0].Reachable = false
	first.OperatorLatency[0].SuccessCount = 0
	first.OperatorLatency[0].FailureCount = 0
	middle.OperatorLatency[0].Reachable = true
	middle.OperatorLatency[0].LatencyMilliseconds = 30
	middle.OperatorLatency[0].SuccessCount = 0
	middle.OperatorLatency[0].FailureCount = 0
	last.OperatorLatency[0].Reachable = false
	last.OperatorLatency[0].SuccessCount = 0
	last.OperatorLatency[0].FailureCount = 0

	accumulator := newHourlyAccumulator(first)
	accumulator.add(middle)
	accumulator.add(last)
	got := accumulator.finalized()
	if got.Host.CPUPercent != 90 || got.Host.CPUAveragePercent != 50 ||
		got.Host.CPUSampleCount != 3 || got.Host.MemoryUsedBytes != 6 ||
		got.Host.NetworkRxBytes != 500 || got.Host.NetworkRxPeakRate != 5 {
		t.Fatalf("unexpected hourly host aggregation: %#v", got.Host)
	}
	if len(got.Containers) != defaultMaxContainers || got.Containers[0].CPUPercent != 80 ||
		got.Containers[0].CPUAveragePercent != 50 || got.Containers[0].CPUSampleCount != 3 ||
		got.Containers[0].MemoryPercent != 70 || got.Containers[0].NetworkRxBytes != 500 ||
		got.Containers[0].NetworkRxPeakRate != 5 {
		t.Fatalf("unexpected hourly container aggregation: %#v", got.Containers)
	}
	if got.OperatorLatency[0].SuccessCount != 1 || got.OperatorLatency[0].FailureCount != 2 ||
		!got.OperatorLatency[0].Reachable || got.OperatorLatency[0].LatencyMilliseconds != 30 {
		t.Fatalf("unexpected hourly latency aggregation: %#v", got.OperatorLatency[0])
	}
}

func TestLongBucketKeepsWeightedCPUAveragePeaksAndAvailability(t *testing.T) {
	at := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	host := appendHostBucket(nil, contract.MonitoringHostPoint{
		CollectedAt: at, CPUPercent: 90, CPUAveragePercent: 40, CPUSampleCount: 60,
		LoadOne: 8, TCPConnections: 100, NetworkRxRate: 5,
	}, 6*time.Hour)
	host = appendHostBucket(host, contract.MonitoringHostPoint{
		CollectedAt: at.Add(time.Hour), CPUPercent: 70, CPUAveragePercent: 20, CPUSampleCount: 30,
		LoadOne: 2, TCPConnections: 20, NetworkRxRate: 3,
	}, 6*time.Hour)
	if len(host) != 1 || host[0].CPUPercent != 90 || host[0].CPUAveragePercent != 100.0/3.0 ||
		host[0].CPUSampleCount != 90 || host[0].LoadOne != 8 || host[0].TCPConnections != 100 ||
		host[0].NetworkRxRate != 5 {
		t.Fatalf("unexpected long host bucket: %#v", host)
	}

	container := appendContainerBucket(nil, contract.MonitoringContainerPoint{
		CollectedAt: at, CPUPercent: 80, CPUAveragePercent: 30, CPUSampleCount: 12, PIDs: 40,
	}, 6*time.Hour)
	container = appendContainerBucket(container, contract.MonitoringContainerPoint{
		CollectedAt: at.Add(time.Hour), CPUPercent: 60, CPUAveragePercent: 10, CPUSampleCount: 6, PIDs: 10,
	}, 6*time.Hour)
	if len(container) != 1 || container[0].CPUAveragePercent != 70.0/3.0 ||
		container[0].CPUSampleCount != 18 || container[0].PIDs != 40 {
		t.Fatalf("unexpected long container bucket: %#v", container)
	}

	latencyA := 20.0
	latencyB := 30.0
	latency := appendOperatorLatencyBucket(nil, contract.MonitoringOperatorLatencyPoint{
		CollectedAt: at, LatencyMilliseconds: &latencyA, SuccessCount: 10, FailureCount: 2,
	}, 6*time.Hour)
	latency = appendOperatorLatencyBucket(latency, contract.MonitoringOperatorLatencyPoint{
		CollectedAt: at.Add(time.Hour), LatencyMilliseconds: &latencyB, SuccessCount: 8, FailureCount: 4,
	}, 6*time.Hour)
	if len(latency) != 1 || latency[0].LatencyMilliseconds == nil ||
		*latency[0].LatencyMilliseconds != 30 || latency[0].SuccessCount != 18 ||
		latency[0].FailureCount != 6 {
		t.Fatalf("unexpected long latency bucket: %#v", latency)
	}
}

func TestPruneKeepsThirtyDaysAndRemovesExpiredShard(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	retainedShard := filepath.Join(dir, "metrics-2026-07-01.jsonl")
	oldShard := filepath.Join(dir, "metrics-2026-06-30.jsonl")
	unrelated := filepath.Join(dir, "keep.txt")
	for _, path := range []string{retainedShard, oldShard} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldShard); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired shard remains: %v", err)
	}
	if _, err := os.Stat(retainedShard); err != nil {
		t.Fatalf("shard inside 30-day retention was removed: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestAppendRejectsSymlinkShard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "metrics-2026-07-31.jsonl")); err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink shard error = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "protected" {
		t.Fatalf("symlink target changed: %q, %v", data, err)
	}
}

func TestAppendHourlyRejectsSymlinkShard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "hourly-2026-08.jsonl")); err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(0, 0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.appendHourlyRecord(maximumRollupRecord(
		time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	)); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("hourly symlink error = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "protected" {
		t.Fatalf("hourly symlink target changed: %q, %v", data, err)
	}
}

func TestRollupConfigurationRejectsUnboundedRetentionAndStorage(t *testing.T) {
	base := Config{
		StateDir: t.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
	}
	tooLong := base
	tooLong.RollupRetentionDays = defaultRollupDays + 1
	if _, err := New(tooLong); err == nil {
		t.Fatal("unbounded rollup retention was accepted")
	}
	tooLarge := base
	tooLarge.MaxRollupStorageBytes = defaultMaxRollupBytes + 1
	if _, err := New(tooLarge); err == nil {
		t.Fatal("unbounded rollup storage was accepted")
	}
}

func TestHourlyRollupFailureDoesNotBlockRawSampling(t *testing.T) {
	current := time.Date(2026, 8, 5, 0, 59, 0, 0, time.UTC)
	dir := t.TempDir()
	service, err := New(Config{
		StateDir: dir,
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return current },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	hourlyPath := filepath.Join(dir, "hourly-2026-08.jsonl")
	file, err := os.OpenFile(hourlyPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxRollupShardBytes); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	if err := service.Sample(context.Background()); err != nil {
		t.Fatalf("rollup failure blocked raw sample: %v", err)
	}
	status := service.Status()
	if !strings.Contains(status.LastError, "hourly monitoring shard storage limit reached") {
		t.Fatalf("rollup failure was not reported: %#v", status)
	}
	data, err := os.ReadFile(filepath.Join(dir, "metrics-2026-08-05.jsonl"))
	if err != nil || bytes.Count(data, []byte{'\n'}) != 2 {
		t.Fatalf("raw samples after rollup failure = %d err=%v", bytes.Count(data, []byte{'\n'}), err)
	}
	if currentRecord, ok := service.currentHourlyRecord(); !ok ||
		!currentRecord.CollectedAt.Equal(current) {
		t.Fatalf("hourly accumulator did not advance after failure: %#v, %v", currentRecord, ok)
	}
	if err := os.Remove(hourlyPath); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Hour)
	if err := service.Sample(context.Background()); err != nil {
		t.Fatalf("rollup did not recover after storage became writable: %v", err)
	}
	rollupData, err := os.ReadFile(hourlyPath)
	if err != nil {
		t.Fatal(err)
	}
	var recovered diskRecord
	if err := json.Unmarshal(bytes.TrimSpace(rollupData), &recovered); err != nil {
		t.Fatal(err)
	}
	if !recovered.CollectedAt.Equal(current.Add(-time.Hour)) {
		t.Fatalf("recovered rollup hour = %s", recovered.CollectedAt)
	}
}

func TestCompactRecordFitsDailyStorageBudgetAtMaximumContainerCount(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	hostOnly := diskRecord{
		Version: recordVersion, CollectedAt: now, Host: hostPoint(testSummary(1, 2), now),
	}
	full := hostOnly
	full.ContainerSampled = true
	full.DockerAvailable = true
	full.ContainerTotal = defaultMaxContainers
	for _, target := range DefaultChecks() {
		full.OperatorLatency = append(full.OperatorLatency, diskOperatorLatencyPoint{
			ID: target.ID, LatencyMilliseconds: 999.9, Reachable: true,
		})
	}
	for index := 0; index < defaultMaxContainers; index++ {
		full.Containers = append(full.Containers, diskContainerPoint{
			ID: strings.Repeat("a", 64), Name: strings.Repeat("n", 128),
			Image:      strings.Repeat("i", 256),
			CPUPercent: 100, MemoryBytes: 1 << 30, MemoryLimitBytes: 2 << 30,
			MemoryPercent: 50, NetworkRxBytes: 1 << 40, NetworkTxBytes: 1 << 40,
			BlockReadBytes: 1 << 40, BlockWriteBytes: 1 << 40, PIDs: 999,
		})
	}
	full = compactRawRecord(full)
	hostData, err := json.Marshal(hostOnly)
	if err != nil {
		t.Fatal(err)
	}
	fullData, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	projectedDailyBytes := int64(len(hostData)+1)*1152 + int64(len(fullData)+1)*288
	t.Logf(
		"maximum sampling projection=%d bytes/day host=%d bytes full=%d bytes",
		projectedDailyBytes, len(hostData), len(fullData),
	)
	if projectedDailyBytes >= defaultMaxDailyBytes {
		t.Fatalf(
			"maximum sampling projection = %d bytes (host=%d full=%d), budget=%d",
			projectedDailyBytes, len(hostData), len(fullData), defaultMaxDailyBytes,
		)
	}
	projectedRetentionBytes := projectedDailyBytes * (defaultRetentionDays + 1)
	if projectedRetentionBytes >= defaultMaxStorageBytes {
		t.Fatalf(
			"maximum retention projection = %d bytes, budget=%d",
			projectedRetentionBytes, defaultMaxStorageBytes,
		)
	}
	for _, container := range full.Containers {
		if len(container.Name) > maxHistoricalContainerMetadataBytes ||
			len(container.Image) > maxHistoricalContainerMetadataBytes {
			t.Fatalf("unbounded raw container metadata: %#v", container)
		}
	}
}

func TestBucketAggregationKeepsResourcePeaksAndLatestCounters(t *testing.T) {
	at := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	host := appendHostBucket(nil, contract.MonitoringHostPoint{
		CollectedAt: at, MemoryUsedBytes: 4, MemoryTotalBytes: 10,
		SwapUsedBytes: 1, SwapTotalBytes: 10,
		DiskUsedBytes: 4, DiskTotalBytes: 10, DiskPercent: 40,
		NetworkRxBytes: 100, DiskReadRate: 2, DiskWriteRate: 3,
	}, time.Minute)
	host = appendHostBucket(host, contract.MonitoringHostPoint{
		CollectedAt: at.Add(30 * time.Second), MemoryUsedBytes: 6, MemoryTotalBytes: 20,
		SwapUsedBytes: 8, SwapTotalBytes: 10,
		DiskUsedBytes: 9, DiskTotalBytes: 10, DiskPercent: 90,
		NetworkRxBytes: 200, DiskReadRate: 8, DiskWriteRate: 9,
	}, time.Minute)
	if host[0].MemoryUsedBytes != 4 || host[0].SwapUsedBytes != 8 ||
		host[0].DiskUsedBytes != 9 || host[0].NetworkRxBytes != 200 ||
		host[0].DiskReadRate != 8 || host[0].DiskWriteRate != 9 {
		t.Fatalf("host bucket did not preserve peaks and latest counter: %#v", host[0])
	}

	container := appendContainerBucket(nil, contract.MonitoringContainerPoint{
		CollectedAt: at, MemoryBytes: 4, MemoryLimitBytes: 10, MemoryPercent: 40,
		NetworkRxBytes: 100, BlockReadRate: 2, BlockWriteRate: 3,
	}, time.Minute)
	container = appendContainerBucket(container, contract.MonitoringContainerPoint{
		CollectedAt: at.Add(30 * time.Second), MemoryBytes: 6, MemoryLimitBytes: 20,
		MemoryPercent: 30, NetworkRxBytes: 200, BlockReadRate: 8, BlockWriteRate: 9,
	}, time.Minute)
	if container[0].MemoryBytes != 4 || container[0].MemoryPercent != 40 ||
		container[0].NetworkRxBytes != 200 || container[0].BlockReadRate != 8 ||
		container[0].BlockWriteRate != 9 {
		t.Fatalf("container bucket did not preserve peak and latest counter: %#v", container[0])
	}

	latencyA := 12.5
	latencyB := 30.25
	latency := appendOperatorLatencyBucket(nil, contract.MonitoringOperatorLatencyPoint{
		CollectedAt: at, LatencyMilliseconds: &latencyA,
	}, time.Minute)
	latency = appendOperatorLatencyBucket(latency, contract.MonitoringOperatorLatencyPoint{
		CollectedAt: at.Add(20 * time.Second), LatencyMilliseconds: nil,
	}, time.Minute)
	latency = appendOperatorLatencyBucket(latency, contract.MonitoringOperatorLatencyPoint{
		CollectedAt: at.Add(40 * time.Second), LatencyMilliseconds: &latencyB,
	}, time.Minute)
	if len(latency) != 1 || latency[0].LatencyMilliseconds == nil ||
		*latency[0].LatencyMilliseconds != latencyB ||
		!latency[0].CollectedAt.Equal(at.Add(40*time.Second)) {
		t.Fatalf("latency bucket did not preserve successful peak: %#v", latency)
	}
}

func TestMaximumThirtyDayResponseFitsPanelAgentLimit(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	const pointCount = 30*12 + 1
	history := contract.MonitoringHistory{
		Range: "30d", StartedAt: now.Add(-30 * 24 * time.Hour), EndedAt: now,
		BucketSeconds:   int((2 * time.Hour).Seconds()),
		Host:            make([]contract.MonitoringHostPoint, pointCount),
		Containers:      make([]contract.MonitoringContainerSeries, maxHistorySeries),
		OperatorLatency: operatorLatencyCatalog(),
	}
	for seriesIndex := range history.OperatorLatency {
		series := &history.OperatorLatency[seriesIndex]
		series.Points = make([]contract.MonitoringOperatorLatencyPoint, pointCount)
		for pointIndex := range series.Points {
			value := 999.9
			series.Points[pointIndex] = contract.MonitoringOperatorLatencyPoint{
				CollectedAt:         now.Add(-time.Duration(pointIndex) * 2 * time.Hour),
				LatencyMilliseconds: &value,
				SuccessCount:        24,
				FailureCount:        24,
			}
		}
	}
	for index := range history.Host {
		history.Host[index] = contract.MonitoringHostPoint{
			CollectedAt: now.Add(-time.Duration(index) * 2 * time.Hour),
			CPUPercent:  100, CPUAveragePercent: 99.9, CPUSampleCount: 120,
			CPUCores: 64, MemoryUsedBytes: 1 << 40,
			MemoryTotalBytes: 2 << 40, DiskUsedBytes: 1 << 40, DiskTotalBytes: 2 << 40,
			NetworkRxBytes: 1 << 50, NetworkTxBytes: 1 << 50,
			NetworkRxRate: 1 << 30, NetworkTxRate: 1 << 30,
		}
	}
	for seriesIndex := range history.Containers {
		series := &history.Containers[seriesIndex]
		series.ContainerID = strings.Repeat("a", 64)
		series.Name = "container-name-1234567890"
		series.Image = "registry.example.test/namespace/application:latest"
		series.Points = make([]contract.MonitoringContainerPoint, pointCount)
		for pointIndex := range series.Points {
			series.Points[pointIndex] = contract.MonitoringContainerPoint{
				CollectedAt: now.Add(-time.Duration(pointIndex) * 2 * time.Hour),
				CPUPercent:  100, CPUAveragePercent: 99.9, CPUSampleCount: 24,
				MemoryBytes: 1 << 40, MemoryLimitBytes: 2 << 40,
				MemoryPercent: 50, NetworkRxBytes: 1 << 50, NetworkTxBytes: 1 << 50,
				NetworkRxRate: 1 << 30, NetworkTxRate: 1 << 30,
				BlockReadBytes: 1 << 50, BlockWriteBytes: 1 << 50, PIDs: 999,
			}
		}
	}
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	const panelAgentResponseLimit = 8 << 20
	t.Logf("maximum thirty-day response=%d bytes", len(data))
	if len(data) >= panelAgentResponseLimit {
		t.Fatalf("maximum history response = %d bytes, limit=%d", len(data), panelAgentResponseLimit)
	}
}

func TestMaximumTwelveMonthResponseFitsPanelAgentLimit(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	const pointCount = 365 + 1
	history := maximumHistoryFixture(now, "12m", 24*time.Hour, pointCount)
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	const panelAgentResponseLimit = 8 << 20
	t.Logf("maximum twelve-month response=%d bytes", len(data))
	if len(data) >= panelAgentResponseLimit {
		t.Fatalf("maximum history response = %d bytes, limit=%d", len(data), panelAgentResponseLimit)
	}
}

func maximumHistoryFixture(
	now time.Time,
	rangeValue string,
	bucket time.Duration,
	pointCount int,
) contract.MonitoringHistory {
	history := contract.MonitoringHistory{
		Range: rangeValue, StartedAt: now.Add(-time.Duration(pointCount-1) * bucket), EndedAt: now,
		BucketSeconds:   int(bucket.Seconds()),
		Host:            make([]contract.MonitoringHostPoint, pointCount),
		Containers:      make([]contract.MonitoringContainerSeries, maxHistorySeries),
		OperatorLatency: operatorLatencyCatalog(),
	}
	for seriesIndex := range history.OperatorLatency {
		series := &history.OperatorLatency[seriesIndex]
		series.Points = make([]contract.MonitoringOperatorLatencyPoint, pointCount)
		for pointIndex := range series.Points {
			value := 999.9
			series.Points[pointIndex] = contract.MonitoringOperatorLatencyPoint{
				CollectedAt:         now.Add(-time.Duration(pointIndex) * bucket),
				LatencyMilliseconds: &value,
				SuccessCount:        120,
				FailureCount:        120,
			}
		}
	}
	for index := range history.Host {
		history.Host[index] = contract.MonitoringHostPoint{
			CollectedAt: now.Add(-time.Duration(index) * bucket),
			CPUPercent:  100, CPUAveragePercent: 99.9, CPUSampleCount: 1440,
			CPUCores: 64, MemoryUsedBytes: 1 << 40,
			MemoryTotalBytes: 2 << 40, DiskUsedBytes: 1 << 40, DiskTotalBytes: 2 << 40,
			NetworkRxBytes: 1 << 50, NetworkTxBytes: 1 << 50,
			NetworkRxRate: 1 << 30, NetworkTxRate: 1 << 30,
		}
	}
	for seriesIndex := range history.Containers {
		series := &history.Containers[seriesIndex]
		series.ContainerID = fmt.Sprintf("%064x", seriesIndex)
		series.Name = "container-name-1234567890"
		series.Image = "registry.example.test/namespace/application:latest"
		series.Points = make([]contract.MonitoringContainerPoint, pointCount)
		for pointIndex := range series.Points {
			series.Points[pointIndex] = contract.MonitoringContainerPoint{
				CollectedAt: now.Add(-time.Duration(pointIndex) * bucket),
				CPUPercent:  100, CPUAveragePercent: 99.9, CPUSampleCount: 288,
				MemoryBytes: 1 << 40, MemoryLimitBytes: 2 << 40,
				MemoryPercent: 50, NetworkRxBytes: 1 << 50, NetworkTxBytes: 1 << 50,
				NetworkRxRate: 1 << 30, NetworkTxRate: 1 << 30,
				BlockReadBytes: 1 << 50, BlockWriteBytes: 1 << 50, PIDs: 999,
			}
		}
	}
	return history
}

func TestLongHistoryUsesExclusiveQueryCapacity(t *testing.T) {
	service, err := New(Config{
		StateDir: t.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rangeValue := range []string{"30d", "3m", "6m", "12m"} {
		service.querySlots <- struct{}{}
		if _, err := service.History(context.Background(), rangeValue); !errors.Is(err, ErrBusy) {
			t.Fatalf("%s query with occupied capacity error = %v, want busy", rangeValue, err)
		}
		if len(service.querySlots) != 1 {
			t.Fatalf("failed %s query leaked capacity: %d", rangeValue, len(service.querySlots))
		}
		<-service.querySlots
	}
	service.querySlots <- struct{}{}
	if _, err := service.History(context.Background(), "7d"); err != nil {
		t.Fatalf("short query did not use remaining capacity: %v", err)
	}
	<-service.querySlots
}

func TestThirtyDayRangeUsesBoundedTwoHourBuckets(t *testing.T) {
	spec, err := parseRange("30d")
	if err != nil {
		t.Fatal(err)
	}
	if spec.duration != 30*24*time.Hour || spec.bucket != 2*time.Hour || spec.querySlots != 2 {
		t.Fatalf("unexpected 30-day range: %#v", spec)
	}
}

func BenchmarkHistoryThirtyDays(b *testing.B) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: b.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		b.Fatal(err)
	}
	for index := 0; index < 30*24*60; index++ {
		at := now.Add(-time.Duration(index) * time.Minute)
		record := diskRecord{Version: recordVersion, CollectedAt: at, Host: hostPoint(testSummary(uint64(index), uint64(index)), at)}
		if err := service.appendRecord(record); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := service.History(context.Background(), "30d"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistoryTwelveMonthsThirtyTwoContainers(b *testing.B) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: b.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		b.Fatal(err)
	}
	for index := 365 * 24; index >= 0; index-- {
		at := now.Add(-time.Duration(index) * time.Hour)
		record := maximumRollupRecord(at)
		if err := service.appendHourlyRecord(record); err != nil {
			b.Fatal(err)
		}
	}
	var responseBytes int
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		history, historyErr := service.History(context.Background(), "12m")
		if historyErr != nil {
			b.Fatal(historyErr)
		}
		data, marshalErr := json.Marshal(history)
		if marshalErr != nil {
			b.Fatal(marshalErr)
		}
		responseBytes = len(data)
	}
	b.StopTimer()
	b.ReportMetric(float64(responseBytes), "response-B")
}

func BenchmarkHistoryWindowMatrix(b *testing.B) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: b.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return now },
	})
	if err != nil {
		b.Fatal(err)
	}
	for index := 30 * 24 * 60; index >= 0; index-- {
		at := now.Add(-time.Duration(index) * time.Minute)
		if err := service.appendRecord(maximumRawRecord(at)); err != nil {
			b.Fatalf("prepare raw history at %s: %v", at, err)
		}
	}
	for index := 365 * 24; index >= 0; index-- {
		at := now.Add(-time.Duration(index) * time.Hour)
		if err := service.appendHourlyRecord(maximumRollupRecord(at)); err != nil {
			b.Fatalf("prepare hourly history at %s: %v", at, err)
		}
	}

	for _, rangeValue := range []string{"1h", "6h", "24h", "7d", "30d", "3m", "6m", "12m"} {
		b.Run(rangeValue, func(b *testing.B) {
			var responseBytes int
			var history contract.MonitoringHistory
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				var historyErr error
				history, historyErr = service.History(context.Background(), rangeValue)
				if historyErr != nil {
					b.Fatal(historyErr)
				}
				data, marshalErr := json.Marshal(history)
				if marshalErr != nil {
					b.Fatal(marshalErr)
				}
				responseBytes = len(data)
			}
			containerPoints := 0
			for _, series := range history.Containers {
				containerPoints += len(series.Points)
			}
			b.ReportMetric(float64(responseBytes), "response-B")
			b.ReportMetric(float64(history.ScannedBytes), "scanned-B")
			b.ReportMetric(float64(len(history.Host)), "host-points")
			b.ReportMetric(float64(containerPoints), "container-points")
		})
	}

	customWindows := []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{name: "zoom-recent-6h", start: now.Add(-6 * time.Hour), end: now},
		{name: "zoom-old-24h", start: now.Add(-60 * 24 * time.Hour), end: now.Add(-59 * 24 * time.Hour)},
		{name: "zoom-old-7d", start: now.Add(-180 * 24 * time.Hour), end: now.Add(-173 * 24 * time.Hour)},
	}
	for _, window := range customWindows {
		b.Run(window.name, func(b *testing.B) {
			var responseBytes int
			var history contract.MonitoringHistory
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				var historyErr error
				history, historyErr = service.HistoryBetween(
					context.Background(), "12m", window.start, window.end,
				)
				if historyErr != nil {
					b.Fatal(historyErr)
				}
				data, marshalErr := json.Marshal(history)
				if marshalErr != nil {
					b.Fatal(marshalErr)
				}
				responseBytes = len(data)
			}
			containerPoints := 0
			for _, series := range history.Containers {
				containerPoints += len(series.Points)
			}
			b.ReportMetric(float64(responseBytes), "response-B")
			b.ReportMetric(float64(history.ScannedBytes), "scanned-B")
			b.ReportMetric(float64(len(history.Host)), "host-points")
			b.ReportMetric(float64(containerPoints), "container-points")
		})
	}
}

func BenchmarkHourlyRollupUpdateThirtyTwoContainers(b *testing.B) {
	start := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: b.TempDir(),
		System:   fakeSystemSource{summary: testSummary(0, 0)},
		Now:      func() time.Time { return start },
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		record := maximumRollupRecord(start.Add(time.Duration(index) * time.Minute))
		if err := service.updateHourly(record); err != nil {
			b.Fatal(err)
		}
	}
}

func maximumRollupRecord(at time.Time) diskRecord {
	record := diskRecord{
		Version: recordVersion, CollectedAt: at,
		Host:             hostPoint(testSummary(uint64(at.Unix()), uint64(at.Unix())), at),
		ContainerSampled: true, DockerAvailable: true, ContainerTotal: defaultMaxContainers,
	}
	record.Host.CPUAveragePercent = 99.9
	record.Host.CPUSampleCount = 60
	record.Host.NetworkRxPeakRate = 1 << 30
	record.Host.NetworkTxPeakRate = 1 << 30
	record.Host.DiskReadPeakRate = 1 << 30
	record.Host.DiskWritePeakRate = 1 << 30
	for _, target := range DefaultChecks() {
		record.OperatorLatency = append(record.OperatorLatency, diskOperatorLatencyPoint{
			ID: target.ID, LatencyMilliseconds: 999.9, Reachable: true,
			SuccessCount: 6, FailureCount: 6,
		})
	}
	for index := 0; index < defaultMaxContainers; index++ {
		record.Containers = append(record.Containers, diskContainerPoint{
			ID: fmt.Sprintf("%064x", index), Name: strings.Repeat("n", 128),
			Image:      strings.Repeat("i", 256),
			CPUPercent: 100, CPUAveragePercent: 99.9, CPUSampleCount: 12,
			MemoryBytes: 1 << 30, MemoryLimitBytes: 2 << 30,
			MemoryPercent: 50, NetworkRxBytes: uint64(at.Unix()) + uint64(index),
			NetworkTxBytes: uint64(at.Unix()) + uint64(index), BlockReadBytes: 1 << 40,
			BlockWriteBytes: 1 << 40, PIDs: 999,
			NetworkRxPeakRate: 1 << 30, NetworkTxPeakRate: 1 << 30,
			BlockReadPeakRate: 1 << 30, BlockWritePeakRate: 1 << 30,
		})
	}
	return record
}

func maximumRawRecord(at time.Time) diskRecord {
	record := maximumRollupRecord(at)
	record.Host.CPUAveragePercent = 0
	record.Host.CPUSampleCount = 0
	record.Host.NetworkRxPeakRate = 0
	record.Host.NetworkTxPeakRate = 0
	record.Host.DiskReadPeakRate = 0
	record.Host.DiskWritePeakRate = 0
	for index := range record.Containers {
		container := &record.Containers[index]
		container.CPUAveragePercent = 0
		container.CPUSampleCount = 0
		container.NetworkRxPeakRate = 0
		container.NetworkTxPeakRate = 0
		container.BlockReadPeakRate = 0
		container.BlockWritePeakRate = 0
	}
	for index := range record.OperatorLatency {
		record.OperatorLatency[index].SuccessCount = 0
		record.OperatorLatency[index].FailureCount = 0
	}
	if at.Minute()%5 != 0 {
		record.ContainerSampled = false
		record.DockerAvailable = false
		record.ContainerTotal = 0
		record.Containers = nil
		record.OperatorLatency = nil
	}
	return record
}
