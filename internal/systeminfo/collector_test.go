package systeminfo

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestPrepareDefaultsIsSafeForConcurrentCollectors(t *testing.T) {
	collector := &Collector{}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			collector.prepareDefaults()
		}()
	}
	group.Wait()
	if collector.ProcRoot != "/proc" || collector.SysRoot != "/sys" || collector.Now == nil {
		t.Fatalf("collector defaults were not initialized: %#v", collector)
	}
}

func TestCollectorReadsLinuxFixtures(t *testing.T) {
	root := filepath.Join("testdata", "root")
	collector := &Collector{
		ProcRoot: filepath.Join(root, "proc"),
		EtcRoot:  filepath.Join(root, "etc"),
		Now:      func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	got, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if got.OS != "Fixture Linux 1" || got.OSID != "fixture" ||
		len(got.OSLike) != 2 || got.OSLike[0] != "debian" ||
		got.Kernel != "6.8.0-fixture" {
		t.Fatalf("unexpected OS data: %#v", got)
	}
	if got.CPU.Cores != 2 || got.CPU.UsagePercent != 0 {
		t.Fatalf("unexpected CPU: %#v", got.CPU)
	}
	if got.CPU.Model != "Fixture CPU" || got.CPU.FrequencyMHz != 2400 {
		t.Fatalf("unexpected CPU identity: %#v", got.CPU)
	}
	if got.Load.One != 0.1 || got.Load.Five != 0.2 || got.Load.Fifteen != 0.3 {
		t.Fatalf("unexpected load averages: %#v", got.Load)
	}
	if got.Memory.TotalBytes != 8*1024*1024 || got.Memory.UsedBytes != 6*1024*1024 {
		t.Fatalf("unexpected memory: %#v", got.Memory)
	}
	if got.Network.ReceivedBytes != 3000 || got.Network.SentBytes != 7000 {
		t.Fatalf("unexpected network: %#v", got.Network)
	}
	if got.Network.TCPConnections != 1 {
		t.Fatalf("unexpected connection counts: %#v", got.Network)
	}
	if !got.DiskIO.Available || got.DiskIO.ReadBytes != 1536*512 ||
		got.DiskIO.WriteBytes != 3072*512 {
		t.Fatalf("unexpected disk I/O: %#v", got.DiskIO)
	}
	if got.UptimeSeconds != 12345 {
		t.Fatalf("unexpected uptime: %d", got.UptimeSeconds)
	}
	if len(got.Management.SSH.Ports) != 1 || got.Management.SSH.Ports[0] != 2222 {
		t.Fatalf("unexpected SSH configuration: %#v", got.Management.SSH)
	}
	if len(got.Management.DNS.Servers) != 2 || got.Management.DNS.Servers[0] != "1.1.1.1" {
		t.Fatalf("unexpected DNS configuration: %#v", got.Management.DNS)
	}
	if got.Management.Timezone != "Asia/Shanghai" || got.Management.IPPreference != "ipv4" {
		t.Fatalf("unexpected regional configuration: %#v", got.Management)
	}
	if got.Management.PackageManager != "apt" ||
		len(got.Management.PackageSources) != 1 ||
		got.Management.PackageSources[0] != "mirrors.example.test" {
		t.Fatalf("unexpected package sources: %#v", got.Management)
	}
	if got.Management.Swap.ActiveDevices != 1 ||
		!got.Management.Swap.FileExists ||
		!got.Management.Swap.FileActive ||
		got.Management.Swap.FileSizeBytes == 0 ||
		got.Management.Swap.OtherActiveDevices != 0 {
		t.Fatalf("unexpected swap state: %#v", got.Management.Swap)
	}
	if !got.Management.KernelOptimization.Enabled ||
		got.Management.KernelOptimization.Profile != "网站优化模式" {
		t.Fatalf("unexpected kernel optimization: %#v", got.Management.KernelOptimization)
	}
	if !got.Management.BBR.Enabled || !got.Management.BBR.Supported ||
		got.Management.BBR.DefaultQDisc != "fq" {
		t.Fatalf("unexpected BBR state: %#v", got.Management.BBR)
	}
}

func TestCollectRuntimeSkipsNetworkIdentityAndManagementProbes(t *testing.T) {
	root := filepath.Join("testdata", "root")
	lookupCalls := 0
	collector := &Collector{
		ProcRoot:                   filepath.Join(root, "proc"),
		EtcRoot:                    filepath.Join(root, "etc"),
		Now:                        func() time.Time { return time.Unix(1_700_000_000, 0) },
		PublicNetworkLookupEnabled: true,
		PublicNetworkLookup: func(context.Context) (contract.PublicNetworkSummary, error) {
			lookupCalls++
			return contract.PublicNetworkSummary{IPv4: "192.0.2.1"}, nil
		},
	}
	got, err := collector.CollectRuntime(context.Background())
	if err != nil {
		t.Fatalf("CollectRuntime() error = %v", err)
	}
	if lookupCalls != 0 || got.PublicNetwork.IPv4 != "" {
		t.Fatalf("runtime collection performed public lookup: calls=%d result=%#v", lookupCalls, got.PublicNetwork)
	}
	if len(got.Management.SSH.Ports) != 0 || got.Management.Timezone != "" ||
		got.Management.PackageManager != "" {
		t.Fatalf("runtime collection included management data: %#v", got.Management)
	}
	if got.CPU.Cores != 2 || got.Memory.TotalBytes == 0 || got.Network.ReceivedBytes == 0 {
		t.Fatalf("runtime collection missed local metrics: %#v", got)
	}
}

func TestReadHostsAndCronConfiguration(t *testing.T) {
	root := t.TempDir()
	etcRoot := filepath.Join(root, "etc")
	varRoot := filepath.Join(root, "var")
	if err := os.MkdirAll(filepath.Join(varRoot, "spool", "cron", "crontabs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(etcRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(etcRoot, "hosts"), []byte("127.0.0.1 localhost\ninvalid ignored\n192.0.2.10 example.com # note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(varRoot, "spool", "cron", "crontabs", "root"), []byte("# managed\n0 2 * * * k update\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := &Collector{EtcRoot: etcRoot, VarRoot: varRoot}
	if got := collector.readHostsConfiguration(); len(got) != 2 || got[1] != "192.0.2.10 example.com" {
		t.Fatalf("unexpected hosts: %#v", got)
	}
	if got := collector.readCronConfiguration(); len(got) != 1 || got[0] != "0 2 * * * k update" {
		t.Fatalf("unexpected cron: %#v", got)
	}
}

func TestReadSwapConfigurationSeparatesKejilionLegacyAndExternalSwap(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	primaryPath := filepath.Join(root, "swapfile")
	legacyPath := filepath.Join(root, "legacy-swapfile")
	if err := os.MkdirAll(procRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, size := range map[string]int64{
		primaryPath: 1024 * 1024 * 1024,
		legacyPath:  2 * 1024 * 1024 * 1024,
	} {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(size); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	swaps := "Filename Type Size Used Priority\n" +
		primaryPath + " file 1048572 128 -2\n" +
		primaryPath + " file 2097148 0 -3\n" +
		"/dev/vda2 partition 524284 32 -4\n"
	if err := os.WriteFile(filepath.Join(procRoot, "swaps"), []byte(swaps), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := &Collector{
		ProcRoot: procRoot, SwapPath: primaryPath, LegacySwapPath: legacyPath,
	}

	got := collector.readSwapConfiguration()
	if got.Path != primaryPath || !got.FileExists || !got.FileActive ||
		!got.LegacyExists || !got.LegacyActive ||
		got.ActiveDevices != 3 || got.OtherActiveDevices != 1 ||
		got.FileUsedBytes != 128*1024 ||
		got.OtherSwapTotalBytes != 524284*1024 {
		t.Fatalf("unexpected swap configuration: %#v", got)
	}
}

func TestCPUUsagePercentUsesIntervalDelta(t *testing.T) {
	before := cpuTimes{total: 1_000, idle: 800}
	after := cpuTimes{total: 1_200, idle: 900}
	if got := cpuUsagePercent(before, after); got != 50 {
		t.Fatalf("cpuUsagePercent() = %v, want 50", got)
	}
}

func TestReadDiskIOIncludesWholeDisksWithoutPartitionDoubleCounting(t *testing.T) {
	procRoot := t.TempDir()
	data := " 252 0 vda 1 0 100 0 2 0 200 0 0 0 0 0 0 0\n" +
		" 252 1 vda1 1 0 90 0 2 0 180 0 0 0 0 0 0 0\n" +
		" 259 0 nvme0n1 1 0 300 0 2 0 400 0 0 0 0 0 0 0\n" +
		" 259 1 nvme0n1p1 1 0 250 0 2 0 350 0 0 0 0 0 0 0\n" +
		" 7 0 loop0 1 0 999 0 2 0 999 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(procRoot, "diskstats"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got := (&Collector{ProcRoot: procRoot}).readDiskIO()
	if !got.Available || got.ReadBytes != 400*512 || got.WriteBytes != 600*512 {
		t.Fatalf("readDiskIO() = %#v", got)
	}
}

func BenchmarkReadDiskIO(b *testing.B) {
	collector := &Collector{ProcRoot: filepath.Join("testdata", "root", "proc")}
	b.ReportAllocs()
	for b.Loop() {
		if !collector.readDiskIO().Available {
			b.Fatal("fixture disk I/O unavailable")
		}
	}
}

func TestPhysicalDiskDeviceRejectsPartitionsAndVirtualLayers(t *testing.T) {
	tests := map[string]bool{
		"sda": true, "vda": true, "xvdb": true, "nvme0n1": true, "mmcblk0": true,
		"sda1": false, "nvme0n1p1": false, "mmcblk0p1": false,
		"loop0": false, "dm-0": false, "md0": false,
	}
	for name, expected := range tests {
		if got := physicalDiskDevice(name); got != expected {
			t.Errorf("physicalDiskDevice(%q) = %v, want %v", name, got, expected)
		}
	}
}

func TestSectorsToBytesSaturatesInsteadOfWrapping(t *testing.T) {
	if got := sectorsToBytes(^uint64(0)); got != ^uint64(0) {
		t.Fatalf("sectorsToBytes(max) = %d, want saturation", got)
	}
}

func TestReadPackageSourcesRecognizesMainstreamManagers(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		manager string
		host    string
	}{
		{
			name: "dnf", path: "yum.repos.d/baseos.repo",
			content: "[baseos]\nbaseurl=https://dl.rockylinux.org/pub/rocky/9/BaseOS/x86_64/os/\n",
			manager: "rpm", host: "dl.rockylinux.org",
		},
		{
			name: "pacman", path: "pacman.d/mirrorlist",
			content: "Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch\n",
			manager: "pacman", host: "geo.mirror.pkgbuild.com",
		},
		{
			name: "zypper", path: "zypp/repos.d/repo-oss.repo",
			content: "[repo-oss]\nbaseurl=https://download.opensuse.org/distribution/leap/15.6/repo/oss/\n",
			manager: "zypper", host: "download.opensuse.org",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcRoot := t.TempDir()
			path := filepath.Join(etcRoot, filepath.FromSlash(test.path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			manager, sources := (&Collector{EtcRoot: etcRoot}).readPackageSources()
			if manager != test.manager || len(sources) != 1 || sources[0] != test.host {
				t.Fatalf("readPackageSources() = manager %q sources %#v", manager, sources)
			}
		})
	}
}
