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
	if got.Network.ReceivedBytes != 2000 || got.Network.SentBytes != 5000 {
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

func TestReadDisksReturnsStableEmptyCollection(t *testing.T) {
	procRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procRoot, "self", "mounts"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	disks := (&Collector{ProcRoot: procRoot}).readDisks()
	if disks == nil || len(disks) != 0 {
		t.Fatalf("empty disks = %#v, want non-nil empty collection", disks)
	}
}

func TestDiskSummariesWritableOverlayRootUsesUpperCapacityOnce(t *testing.T) {
	for _, upperMount := range []string{"/overlay", "/mnt/extroot"} {
		t.Run(upperMount, func(t *testing.T) {
			mounts := "/dev/root /rom squashfs ro 0 0\n" +
				"/dev/sda2 " + upperMount + " ext4 rw 0 0\n" +
				"overlayfs:/overlay / overlay rw,lowerdir=/,upperdir=" + upperMount + "/upper,workdir=" + upperMount + "/work 0 0\n" +
				"tmpfs /tmp tmpfs rw 0 0\n" +
				"overlay /mnt/container overlay rw,upperdir=/tmp/container 0 0\n" +
				"/dev/sdb1 /mnt/data ext4 rw 0 0\n"
			calls := make(map[string]int)
			disks := diskSummaries(mounts, func(path string) (uint64, uint64, float64, bool) {
				calls[path]++
				if path == "/" {
					return 1000, 250, 25, true
				}
				return 2000, 1000, 50, true
			})
			if len(disks) != 2 || disks[0].MountPoint != "/" || disks[0].FileSystem != "overlay" ||
				disks[0].TotalBytes != 1000 || disks[0].UsedBytes != 250 || disks[1].MountPoint != "/mnt/data" {
				t.Fatalf("disk summaries = %#v", disks)
			}
			if len(calls) != 2 || calls["/"] != 1 || calls["/mnt/data"] != 1 {
				t.Fatalf("unexpected capacity probes: %#v", calls)
			}
		})
	}
}

func TestDiskSummariesKeepsVirtualAndReadOnlyRootsExcluded(t *testing.T) {
	for _, root := range []string{
		"overlay / overlay ro,lowerdir=/rom 0 0",
		"overlay / overlay ro,upperdir=/overlay/upper 0 0",
		"overlay / overlay rw,upperdir=relative 0 0",
		"overlay / overlay rw,upperdir=/ 0 0",
		"/dev/root / squashfs ro 0 0",
		"tmpfs / tmpfs rw 0 0",
	} {
		calls := 0
		disks := diskSummaries(root+"\n/dev/sda1 /mnt/data ext4 rw 0 0\n", func(path string) (uint64, uint64, float64, bool) {
			calls++
			return 100, 10, 10, true
		})
		if len(disks) != 1 || disks[0].MountPoint != "/mnt/data" || calls != 1 {
			t.Fatalf("%q yielded %#v (%d probes)", root, disks, calls)
		}
	}
	disks := diskSummaries("/dev/sda1 / ext4 rw 0 0\n", func(string) (uint64, uint64, float64, bool) {
		return 100, 10, 10, true
	})
	if len(disks) != 1 || disks[0].MountPoint != "/" {
		t.Fatalf("ordinary root changed: %#v", disks)
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

func TestReadNetworkPrefersDefaultRouteInterfaces(t *testing.T) {
	procRoot := t.TempDir()
	netRoot := filepath.Join(procRoot, "net")
	if err := os.MkdirAll(netRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dev := "Inter-| Receive | Transmit\n" +
		" lo: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n" +
		" eth0: 1000 0 0 0 0 0 0 0 2000 0 0 0 0 0 0 0\n" +
		" docker0: 3000 0 0 0 0 0 0 0 4000 0 0 0 0 0 0 0\n" +
		" wg0: 5000 0 0 0 0 0 0 0 6000 0 0 0 0 0 0 0\n"
	route := "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"eth0 00000000 0100000A 0003 0 0 100 00000000 0 0 0\n" +
		"docker0 000011AC 00000000 0001 0 0 0 0000FFFF 0 0 0\n"
	for name, data := range map[string]string{"dev": dev, "route": route} {
		if err := os.WriteFile(filepath.Join(netRoot, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var got contract.NetworkSummary
	if err := (&Collector{ProcRoot: procRoot}).readNetwork(&got); err != nil {
		t.Fatalf("readNetwork() error = %v", err)
	}
	if got.ReceivedBytes != 1000 || got.SentBytes != 2000 {
		t.Fatalf("readNetwork() = %#v, want only default-route interface counters", got)
	}
}

func TestReadNetworkFallbackExcludesLoopbackAndVirtualInterfaces(t *testing.T) {
	procRoot := t.TempDir()
	netRoot := filepath.Join(procRoot, "net")
	if err := os.MkdirAll(netRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dev := "Inter-| Receive | Transmit\n" +
		" lo: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n" +
		" ens3: 1000 0 0 0 0 0 0 0 2000 0 0 0 0 0 0 0\n" +
		" docker0: 3000 0 0 0 0 0 0 0 4000 0 0 0 0 0 0 0\n" +
		" veth1234: 5000 0 0 0 0 0 0 0 6000 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(netRoot, "dev"), []byte(dev), 0o600); err != nil {
		t.Fatal(err)
	}

	var got contract.NetworkSummary
	if err := (&Collector{ProcRoot: procRoot}).readNetwork(&got); err != nil {
		t.Fatalf("readNetwork() error = %v", err)
	}
	if got.ReceivedBytes != 1000 || got.SentBytes != 2000 {
		t.Fatalf("readNetwork() = %#v, want only non-virtual interface counters", got)
	}
}

func TestDiskCapacityExcludesReservedBlocksFromUsedBytes(t *testing.T) {
	total, used, usagePercent, ok := diskCapacity(1000, 200, 150, 4096)
	if !ok || total != 1000*4096 || used != 800*4096 || usagePercent != 84.21 {
		t.Fatalf(
			"diskCapacity() = total %d, used %d, percent %v, ok %v",
			total, used, usagePercent, ok,
		)
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
