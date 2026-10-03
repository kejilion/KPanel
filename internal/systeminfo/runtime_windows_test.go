//go:build windows

package systeminfo

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestWindowsNativeRuntimeReportsRealCounters(t *testing.T) {
	c := NewCollector()
	c.PublicNetworkLookupEnabled = false
	c.CPUSampleInterval = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	summary, err := c.CollectRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Platform != "windows" || summary.OSID != "windows" || summary.Hostname == "" || summary.Kernel == "" {
		t.Fatalf("missing platform identity: %#v", summary)
	}
	if summary.CPU.Cores <= 0 || summary.Memory.TotalBytes == 0 || summary.UptimeSeconds == 0 || len(summary.Disks) == 0 {
		t.Fatalf("missing native metrics: %#v", summary)
	}
	if summary.Network.ReceivedBytes == 0 && summary.Network.SentBytes == 0 {
		t.Fatal("network table omitted traffic counters")
	}
	for _, metric := range []string{"load", "swap", "diskIO", "networkConnections"} {
		if !slices.Contains(summary.UnavailableMetrics, metric) {
			t.Fatalf("%s falsely reported as supported", metric)
		}
	}
	for _, disk := range summary.Disks {
		if disk.TotalBytes == 0 || len(disk.MountPoint) != 2 || disk.MountPoint[0] != '/' {
			t.Fatalf("invalid fixed volume %#v", disk)
		}
	}
}

func TestWindowsNativeRuntimeRespectsCancellation(t *testing.T) {
	c := NewCollector()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.CollectRuntime(ctx); err == nil {
		t.Fatal("cancelled sampling returned success")
	}
}
