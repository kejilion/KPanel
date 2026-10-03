//go:build windows

package systeminfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	windowsKernel      = windows.NewLazySystemDLL("kernel32.dll")
	getSystemTimes     = windowsKernel.NewProc("GetSystemTimes")
	getMemoryStatus    = windowsKernel.NewProc("GlobalMemoryStatusEx")
	getTickCount       = windowsKernel.NewProc("GetTickCount64")
	getProcessorGroups = windowsKernel.NewProc("GetActiveProcessorGroupCount")
	setThreadGroup     = windowsKernel.NewProc("SetThreadGroupAffinity")
)

type groupAffinity struct {
	Mask     uintptr
	Group    uint16
	Reserved [3]uint16
}

// GetSystemTimes describes the calling thread's processor group. Sample each
// group on one locked OS thread; restore its exact affinity on every exit.
func windowsCPUTimes() (cpuTimes, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	count, _, _ := getProcessorGroups.Call()
	if count == 0 || count > 64 {
		return cpuTimes{}, errors.New("invalid processor group count")
	}
	var result cpuTimes
	for group := uint16(0); group < uint16(count); group++ {
		active := windows.GetActiveProcessorCount(group)
		if active == 0 || active > uint32(unsafe.Sizeof(uintptr(0))*8) {
			return cpuTimes{}, errors.New("invalid active processor count")
		}
		affinity := groupAffinity{Mask: ^uintptr(0) >> (uint32(unsafe.Sizeof(uintptr(0))*8) - active), Group: group}
		var previous groupAffinity
		ok, _, err := setThreadGroup.Call(uintptr(windows.CurrentThread()), uintptr(unsafe.Pointer(&affinity)), uintptr(unsafe.Pointer(&previous)))
		if ok == 0 {
			return cpuTimes{}, fmt.Errorf("select processor group %d: %w", group, err)
		}
		var idle, kernel, user windows.Filetime
		ok, _, sampleErr := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
		restored, _, restoreErr := setThreadGroup.Call(uintptr(windows.CurrentThread()), uintptr(unsafe.Pointer(&previous)), 0)
		if restored == 0 {
			return cpuTimes{}, fmt.Errorf("restore processor affinity: %w", restoreErr)
		}
		if ok == 0 {
			return cpuTimes{}, fmt.Errorf("sample processor group: %w", sampleErr)
		}
		ticks := func(v windows.Filetime) uint64 { return uint64(v.HighDateTime)<<32 | uint64(v.LowDateTime) }
		result.idle += ticks(idle)
		result.total += ticks(kernel) + ticks(user)
	}
	return result, nil
}

func (c *Collector) collectPlatformRuntime(ctx context.Context) (contract.SystemSummary, error, bool) {
	// Explicit ProcRoot supports the existing deterministic Linux fixture tests.
	if c.ProcRoot != "/proc" {
		return contract.SystemSummary{}, nil, false
	}
	result := contract.SystemSummary{Platform: "windows", UnavailableMetrics: []string{"load", "swap", "diskIO", "networkConnections"}, Architecture: runtime.GOARCH, OSID: "windows", CollectedAt: c.Now().UTC()}
	var errs []error
	var err error
	result.Hostname, err = os.Hostname()
	if err != nil {
		errs = append(errs, fmt.Errorf("hostname: %w", err))
	}
	version := windows.RtlGetVersion()
	result.Kernel = fmt.Sprintf("%d.%d.%d", version.MajorVersion, version.MinorVersion, version.BuildNumber)
	result.OS = "Windows " + result.Kernel
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		if name, _, err := key.GetStringValue("ProductName"); err == nil {
			result.OS = name
		}
		_ = key.Close()
	}
	before, cpuErr := windowsCPUTimes()
	if cpuErr == nil {
		interval := c.CPUSampleInterval
		if interval <= 0 {
			interval = 150 * time.Millisecond
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, ctx.Err(), true
		case <-timer.C:
		}
		after, err := windowsCPUTimes()
		cpuErr = err
		if err == nil {
			result.CPU.UsagePercent = cpuUsagePercent(before, after)
		}
	}
	if cpuErr != nil {
		errs = append(errs, fmt.Errorf("cpu: %w", cpuErr))
	}
	result.CPU.Cores = int(windows.GetActiveProcessorCount(0xffff))
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		result.CPU.Model, _, _ = key.GetStringValue("ProcessorNameString")
		if mhz, _, err := key.GetIntegerValue("~MHz"); err == nil {
			result.CPU.FrequencyMHz = float64(mhz)
		}
		_ = key.Close()
	}
	var memory struct {
		Length, Load                                                                                  uint32
		TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtended uint64
	}
	memory.Length = uint32(unsafe.Sizeof(memory))
	if ok, _, err := getMemoryStatus.Call(uintptr(unsafe.Pointer(&memory))); ok == 0 {
		errs = append(errs, fmt.Errorf("memory: %w", err))
	} else {
		result.Memory.TotalBytes, result.Memory.AvailableBytes = memory.TotalPhys, memory.AvailPhys
		result.Memory.UsedBytes = memory.TotalPhys - memory.AvailPhys
		result.Memory.UsagePercent = roundPercent(float64(result.Memory.UsedBytes) * 100 / float64(memory.TotalPhys))
	}
	uptime, _, _ := getTickCount.Call()
	result.UptimeSeconds = uint64(uptime) / 1000
	if err := windowsNetwork(&result.Network); err != nil {
		errs = append(errs, fmt.Errorf("network: %w", err))
	}
	result.Disks, err = windowsDisks()
	if err != nil {
		errs = append(errs, err)
	}
	return result, errors.Join(errs...), true
}

func windowsNetwork(out *contract.NetworkSummary) error {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &table); err != nil {
		return err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	if table.NumEntries > 4096 {
		return errors.New("network interface limit exceeded")
	}
	routed := make(map[uint64]bool)
	var routes *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_UNSPEC, &routes); err != nil {
		return err
	}
	if routes.NumEntries > 65536 {
		windows.FreeMibTable(unsafe.Pointer(routes))
		return errors.New("route table limit exceeded")
	}
	for _, route := range routes.Rows() {
		if route.DestinationPrefix.PrefixLength == 0 {
			routed[route.InterfaceLuid] = true
		}
	}
	windows.FreeMibTable(unsafe.Pointer(routes))
	for _, row := range unsafe.Slice(&table.Table[0], int(table.NumEntries)) {
		// A cloud NIC is often virtual. Exclude only loopback/tunnel interfaces,
		// never adapter names, manufacturer or the HardwareInterface bit.
		if row.Type == 24 || row.OperStatus != 1 || len(routed) > 0 && !routed[row.InterfaceLuid] || len(routed) == 0 && row.Type == 131 {
			continue
		}
		out.ReceivedBytes += row.InOctets
		out.SentBytes += row.OutOctets
	}
	return nil
}

func windowsDisks() ([]contract.DiskSummary, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	var result []contract.DiskSummary
	var errs []error
	for n := uint32(0); n < 26; n++ {
		if mask&(1<<n) == 0 {
			continue
		}
		root := string(rune('A'+n)) + `:\`
		ptr, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(ptr) != windows.DRIVE_FIXED {
			continue
		}
		var total, free uint64
		if err := windows.GetDiskFreeSpaceEx(ptr, nil, &total, &free); err != nil {
			errs = append(errs, fmt.Errorf("disk %s: %w", root, err))
			continue
		}
		var fs [32]uint16
		if err := windows.GetVolumeInformation(ptr, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))); err != nil {
			errs = append(errs, fmt.Errorf("filesystem %s: %w", root, err))
			continue
		}
		if total == 0 || free > total {
			errs = append(errs, fmt.Errorf("disk %s: invalid counters", root))
			continue
		}
		result = append(result, contract.DiskSummary{Device: root, MountPoint: "/" + strings.TrimSuffix(root, `:\`), FileSystem: windows.UTF16ToString(fs[:]), TotalBytes: total, UsedBytes: total - free, UsagePercent: roundPercent(float64(total-free) * 100 / float64(total))})
	}
	if len(result) == 0 && len(errs) == 0 {
		errs = append(errs, errors.New("no readable fixed disks"))
	}
	return result, errors.Join(errs...)
}
