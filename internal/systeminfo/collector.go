package systeminfo

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

// Collector reads Linux kernel pseudo-files directly and can optionally perform
// one fixed, cached public-network identity lookup. It never invokes a shell.
type Collector struct {
	ProcRoot                   string
	SysRoot                    string
	EtcRoot                    string
	VarRoot                    string
	SwapPath                   string
	LegacySwapPath             string
	Now                        func() time.Time
	CPUSampleInterval          time.Duration
	PublicNetworkLookupEnabled bool
	PublicNetworkLookup        func(context.Context) (contract.PublicNetworkSummary, error)
	PublicNetworkCacheTTL      time.Duration

	defaultsOnce         sync.Once
	publicNetworkMu      sync.Mutex
	publicNetworkCache   contract.PublicNetworkSummary
	publicNetworkExpires time.Time
	publicNetworkLoading bool
	publicNetworkDone    chan struct{}
}

func NewCollector() *Collector {
	return &Collector{
		ProcRoot: "/proc", SysRoot: "/sys", EtcRoot: "/etc", VarRoot: "/var",
		SwapPath: "/swapfile", LegacySwapPath: "/var/lib/kejilion-panel/system/swapfile",
		Now:                        time.Now,
		CPUSampleInterval:          150 * time.Millisecond,
		PublicNetworkLookupEnabled: true,
		PublicNetworkLookup:        lookupPublicNetwork,
		PublicNetworkCacheTTL:      30 * time.Minute,
	}
}

func (c *Collector) prepareDefaults() {
	c.defaultsOnce.Do(func() {
		if c.ProcRoot == "" {
			c.ProcRoot = "/proc"
		}
		if c.EtcRoot == "" {
			c.EtcRoot = "/etc"
		}
		if c.SysRoot == "" {
			c.SysRoot = "/sys"
		}
		if c.VarRoot == "" {
			if filepath.Clean(c.EtcRoot) == "/etc" {
				c.VarRoot = "/var"
			} else {
				c.VarRoot = filepath.Join(filepath.Dir(c.EtcRoot), "var")
			}
		}
		if c.SwapPath == "" {
			c.SwapPath = "/swapfile"
		}
		if c.LegacySwapPath == "" {
			c.LegacySwapPath = "/var/lib/kejilion-panel/system/swapfile"
		}
		if c.Now == nil {
			c.Now = time.Now
		}
		if c.PublicNetworkCacheTTL <= 0 {
			c.PublicNetworkCacheTTL = 30 * time.Minute
		}
	})
}

func (c *Collector) collectRuntime(ctx context.Context) (contract.SystemSummary, error) {
	c.prepareDefaults()
	result := contract.SystemSummary{
		Architecture: runtime.GOARCH,
		CollectedAt:  c.Now().UTC(),
	}
	result.Hostname, _ = os.Hostname()
	result.OS, result.OSID, result.OSLike = c.readOSRelease()
	result.Kernel = strings.TrimSpace(c.readOptional("sys/kernel/osrelease"))

	var errs []error
	if err := c.readLoad(&result.Load); err != nil {
		errs = append(errs, err)
	}
	if err := c.readCPU(ctx, &result.CPU); err != nil {
		errs = append(errs, err)
	}
	if err := c.readMemory(&result.Memory); err != nil {
		errs = append(errs, err)
	}
	if err := c.readUptime(&result.UptimeSeconds); err != nil {
		errs = append(errs, err)
	}
	if err := c.readNetwork(&result.Network); err != nil {
		errs = append(errs, err)
	}
	result.DiskIO = c.readDiskIO()
	result.Disks = c.readDisks()
	return result, errors.Join(errs...)
}

// CollectRuntime reads only bounded local runtime metrics. It deliberately
// skips public-network lookup and management configuration probes so a
// background history sampler never creates network traffic or invokes host
// management tools.
func (c *Collector) CollectRuntime(ctx context.Context) (contract.SystemSummary, error) {
	return c.collectRuntime(ctx)
}

func (c *Collector) Collect(ctx context.Context) (contract.SystemSummary, error) {
	result, err := c.collectRuntime(ctx)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	if c.PublicNetworkLookupEnabled {
		result.PublicNetwork = c.readPublicNetwork(ctx)
	}
	c.readManagement(&result.Management)
	return result, errors.Join(errs...)
}

func (c *Collector) readOSRelease() (string, string, []string) {
	data, err := os.ReadFile(filepath.Join(c.EtcRoot, "os-release"))
	if err != nil {
		return runtime.GOOS, runtime.GOOS, nil
	}
	values := make(map[string]string)
	s := bufio.NewScanner(strings.NewReader(string(data)))
	for s.Scan() {
		k, v, ok := strings.Cut(s.Text(), "=")
		if !ok {
			continue
		}
		values[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	name := values["PRETTY_NAME"]
	if name == "" {
		name = strings.TrimSpace(values["NAME"] + " " + values["VERSION"])
	}
	id := strings.ToLower(strings.TrimSpace(values["ID"]))
	like := strings.Fields(strings.ToLower(values["ID_LIKE"]))
	return name, id, like
}

func (c *Collector) readLoad(out *contract.LoadSummary) error {
	fields := strings.Fields(c.readOptional("loadavg"))
	if len(fields) < 3 {
		return errors.New("read loadavg: invalid or unavailable")
	}
	var err error
	if out.One, err = strconv.ParseFloat(fields[0], 64); err != nil {
		return fmt.Errorf("read loadavg: %w", err)
	}
	out.Five, _ = strconv.ParseFloat(fields[1], 64)
	out.Fifteen, _ = strconv.ParseFloat(fields[2], 64)
	return nil
}

type cpuTimes struct {
	total uint64
	idle  uint64
}

func (c *Collector) readCPU(ctx context.Context, out *contract.CPUSummary) error {
	before, err := c.readCPUTimes()
	if err != nil {
		return err
	}
	if c.CPUSampleInterval > 0 {
		timer := time.NewTimer(c.CPUSampleInterval)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return fmt.Errorf("sample cpu: %w", ctx.Err())
		case <-timer.C:
		}
	}
	after, err := c.readCPUTimes()
	if err != nil {
		return err
	}
	out.UsagePercent = cpuUsagePercent(before, after)

	cpuInfo := c.readOptional("cpuinfo")
	for _, line := range strings.Split(cpuInfo, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "processor":
			out.Cores++
		case "model name", "Hardware":
			if out.Model == "" {
				out.Model = strings.TrimSpace(value)
			}
		case "cpu MHz":
			if out.FrequencyMHz == 0 {
				out.FrequencyMHz, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
			}
		}
	}
	if out.Cores == 0 {
		out.Cores = runtime.NumCPU()
	}
	if out.FrequencyMHz == 0 {
		frequency := strings.TrimSpace(readFileLimited(filepath.Join(
			c.SysRoot,
			"devices",
			"system",
			"cpu",
			"cpu0",
			"cpufreq",
			"scaling_cur_freq",
		)))
		kHz, _ := strconv.ParseFloat(frequency, 64)
		if kHz > 0 {
			out.FrequencyMHz = kHz / 1000
		}
	}
	return nil
}

func (c *Collector) readCPUTimes() (cpuTimes, error) {
	first, _, _ := strings.Cut(c.readOptional("stat"), "\n")
	fields := strings.Fields(first)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, errors.New("read cpu: invalid or unavailable /proc/stat")
	}
	var values []uint64
	for _, field := range fields[1:] {
		n, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, fmt.Errorf("read cpu: %w", err)
		}
		values = append(values, n)
	}
	var total uint64
	// Linux user/nice already include guest/guest_nice. Summing those final
	// fields again would double count virtual CPU time.
	for i, n := range values {
		if i >= 8 {
			break
		}
		total += n
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return cpuTimes{total: total, idle: idle}, nil
}

func cpuUsagePercent(before, after cpuTimes) float64 {
	if after.total <= before.total || after.idle < before.idle {
		return 0
	}
	totalDelta := after.total - before.total
	idleDelta := after.idle - before.idle
	if idleDelta >= totalDelta {
		return 0
	}
	return roundPercent(float64(totalDelta-idleDelta) * 100 / float64(totalDelta))
}

func (c *Collector) readMemory(out *contract.MemorySummary) error {
	data := c.readOptional("meminfo")
	if data == "" {
		return errors.New("read memory: unavailable /proc/meminfo")
	}
	values := make(map[string]uint64)
	for _, line := range strings.Split(data, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		n, _ := strconv.ParseUint(fields[0], 10, 64)
		values[key] = n * 1024
	}
	out.TotalBytes = values["MemTotal"]
	out.AvailableBytes = values["MemAvailable"]
	if out.AvailableBytes == 0 {
		out.AvailableBytes = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if out.TotalBytes >= out.AvailableBytes {
		out.UsedBytes = out.TotalBytes - out.AvailableBytes
	}
	if out.TotalBytes > 0 {
		out.UsagePercent = roundPercent(float64(out.UsedBytes) * 100 / float64(out.TotalBytes))
	}
	out.SwapTotalBytes = values["SwapTotal"]
	if out.SwapTotalBytes >= values["SwapFree"] {
		out.SwapUsedBytes = out.SwapTotalBytes - values["SwapFree"]
	}
	if out.TotalBytes == 0 {
		return errors.New("read memory: MemTotal missing")
	}
	return nil
}

func (c *Collector) readUptime(out *uint64) error {
	fields := strings.Fields(c.readOptional("uptime"))
	if len(fields) == 0 {
		return errors.New("read uptime: invalid or unavailable")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return fmt.Errorf("read uptime: %w", err)
	}
	if value > 0 {
		*out = uint64(value)
	}
	return nil
}

func (c *Collector) readNetwork(out *contract.NetworkSummary) error {
	data := c.readOptional("net/dev")
	if data == "" {
		return errors.New("read network: unavailable /proc/net/dev")
	}
	for _, line := range strings.Split(data, "\n") {
		_, values, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 16 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		out.ReceivedBytes += rx
		out.SentBytes += tx
	}
	out.TCPConnections = c.connectionCount("net/tcp") + c.connectionCount("net/tcp6")
	out.UDPConnections = c.connectionCount("net/udp") + c.connectionCount("net/udp6")
	return nil
}

func (c *Collector) readDiskIO() contract.DiskIOSummary {
	data := c.readOptional("diskstats")
	var result contract.DiskIOSummary
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 || !physicalDiskDevice(fields[2]) {
			continue
		}
		readSectors, readErr := strconv.ParseUint(fields[5], 10, 64)
		writeSectors, writeErr := strconv.ParseUint(fields[9], 10, 64)
		if readErr != nil || writeErr != nil {
			continue
		}
		result.Available = true
		result.ReadBytes += sectorsToBytes(readSectors)
		result.WriteBytes += sectorsToBytes(writeSectors)
	}
	return result
}

// physicalDiskDevice keeps aggregate I/O counters free of partition and
// device-mapper double counting. These names cover the physical and
// hypervisor-backed disks used by the supported Linux distributions.
func physicalDiskDevice(name string) bool {
	for _, prefix := range []string{"sd", "vd", "xvd", "hd", "dasd"} {
		if strings.HasPrefix(name, prefix) {
			return onlyASCIILetters(name[len(prefix):])
		}
	}
	if strings.HasPrefix(name, "nvme") {
		rest := name[len("nvme"):]
		controllerEnd := strings.IndexByte(rest, 'n')
		return controllerEnd > 0 && onlyASCIIDigits(rest[:controllerEnd]) &&
			onlyASCIIDigits(rest[controllerEnd+1:])
	}
	if strings.HasPrefix(name, "mmcblk") {
		return onlyASCIIDigits(name[len("mmcblk"):])
	}
	return false
}

func onlyASCIILetters(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < 'a' || char > 'z' {
			return false
		}
	}
	return true
}

func onlyASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func sectorsToBytes(sectors uint64) uint64 {
	const sectorSize = uint64(512)
	if sectors > ^uint64(0)/sectorSize {
		return ^uint64(0)
	}
	return sectors * sectorSize
}

func (c *Collector) connectionCount(name string) int {
	data := strings.TrimSpace(c.readOptional(name))
	if data == "" {
		return 0
	}
	lines := strings.Split(data, "\n")
	if len(lines) <= 1 {
		return 0
	}
	return len(lines) - 1
}

func (c *Collector) readDisks() []contract.DiskSummary {
	data := c.readOptional("self/mounts")
	seen := make(map[string]bool)
	var result []contract.DiskSummary
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPoint := unescapeMount(fields[1])
		if seen[mountPoint] || !meaningfulMount(mountPoint, fields[2]) {
			continue
		}
		seen[mountPoint] = true
		total, free, ok := diskUsage(mountPoint)
		if !ok || total == 0 {
			continue
		}
		used := total - free
		result = append(result, contract.DiskSummary{
			Device:       unescapeMount(fields[0]),
			MountPoint:   mountPoint,
			FileSystem:   fields[2],
			TotalBytes:   total,
			UsedBytes:    used,
			UsagePercent: roundPercent(float64(used) * 100 / float64(total)),
		})
	}
	return result
}

func (c *Collector) readOptional(name string) string {
	data, _ := os.ReadFile(filepath.Join(c.ProcRoot, filepath.FromSlash(name)))
	return string(data)
}

func roundPercent(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func unescapeMount(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`)
	return replacer.Replace(value)
}

func meaningfulMount(path, fs string) bool {
	switch fs {
	case "proc", "sysfs", "devtmpfs", "devpts", "tmpfs", "cgroup", "cgroup2",
		"overlay", "squashfs", "nsfs", "mqueue", "securityfs", "pstore", "tracefs",
		"debugfs", "configfs", "fusectl", "hugetlbfs", "rpc_pipefs":
		return false
	}
	return path == "/" || path == "/home" || strings.HasPrefix(path, "/mnt/") || strings.HasPrefix(path, "/data")
}
