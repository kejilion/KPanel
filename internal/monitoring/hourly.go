package monitoring

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxHistoricalContainerMetadataBytes = 64

type hourlyContainerAccumulator struct {
	point    diskContainerPoint
	seenAt   time.Time
	cpuSum   float64
	cpuCount int
}

type hourlyPreviousContainer struct {
	point diskContainerPoint
	at    time.Time
}

type hourlyAccumulator struct {
	hour               time.Time
	record             diskRecord
	hostCPUSum         float64
	hostCPUCount       int
	previousHost       diskHostPoint
	previousHostAt     time.Time
	previousHostExists bool
	containers         map[string]hourlyContainerAccumulator
	previousContainers map[string]hourlyPreviousContainer
	latency            map[string]diskOperatorLatencyPoint
	latencyOrder       []string
}

func emptyHourlyAccumulator(hour time.Time) *hourlyAccumulator {
	return &hourlyAccumulator{
		hour:               hour.UTC().Truncate(time.Hour),
		containers:         make(map[string]hourlyContainerAccumulator, maxScannedSeries),
		previousContainers: make(map[string]hourlyPreviousContainer, maxScannedSeries),
		latency:            make(map[string]diskOperatorLatencyPoint, MaxChecks),
		latencyOrder:       make([]string, 0, MaxChecks),
	}
}

func newHourlyAccumulator(record diskRecord) *hourlyAccumulator {
	accumulator := emptyHourlyAccumulator(record.CollectedAt)
	accumulator.add(record)
	return accumulator
}

func (a *hourlyAccumulator) next(record diskRecord) *hourlyAccumulator {
	next := emptyHourlyAccumulator(record.CollectedAt)
	next.previousHost = a.previousHost
	next.previousHostAt = a.previousHostAt
	next.previousHostExists = a.previousHostExists
	for id, previous := range a.previousContainers {
		next.previousContainers[id] = previous
	}
	next.add(record)
	return next
}

func (a *hourlyAccumulator) add(record diskRecord) {
	if record.CollectedAt.UTC().Truncate(time.Hour) != a.hour {
		return
	}
	a.hostCPUSum += record.Host.CPUPercent
	a.hostCPUCount++
	record.Host = a.hostPointWithRates(record.Host, record.CollectedAt)
	if a.record.Version == 0 {
		a.record = record
		a.record.Containers = nil
		a.record.OperatorLatency = nil
	} else {
		mergeHourlyHost(&a.record.Host, record.Host)
		a.record.CollectedAt = record.CollectedAt
	}
	if record.ContainerSampled {
		a.record.ContainerSampled = true
		a.record.DockerAvailable = record.DockerAvailable
		a.record.ContainerTotal = record.ContainerTotal
		a.record.ContainerFailed = record.ContainerFailed
		a.record.ContainerTruncated = record.ContainerTruncated
		for _, container := range record.Containers {
			container = a.containerPointWithRates(container, record.CollectedAt)
			current, exists := a.containers[container.ID]
			if !exists && len(a.containers) >= maxScannedSeries {
				a.evictOldestContainer()
			}
			if exists {
				mergeHourlyContainer(&current.point, container)
			} else {
				current.point = container
			}
			current.cpuSum += container.CPUPercent
			current.cpuCount++
			current.seenAt = record.CollectedAt
			a.containers[container.ID] = current
		}
	}
	for _, latency := range record.OperatorLatency {
		current, exists := a.latency[latency.ID]
		success, failure := latency.SuccessCount, latency.FailureCount
		if success == 0 && failure == 0 {
			if latency.Reachable {
				success = 1
			} else {
				failure = 1
			}
		}
		if !exists {
			a.latencyOrder = append(a.latencyOrder, latency.ID)
			latency.SuccessCount = success
			latency.FailureCount = failure
			a.latency[latency.ID] = latency
			continue
		}
		current.SuccessCount += success
		current.FailureCount += failure
		if latency.Reachable &&
			(!current.Reachable || latency.LatencyMilliseconds > current.LatencyMilliseconds) {
			current.Reachable = true
			current.LatencyMilliseconds = latency.LatencyMilliseconds
		}
		a.latency[latency.ID] = current
	}
}

func (a *hourlyAccumulator) hostPointWithRates(
	point diskHostPoint,
	at time.Time,
) diskHostPoint {
	if a.previousHostExists {
		seconds := at.Sub(a.previousHostAt).Seconds()
		point.NetworkRxPeakRate = counterRate(a.previousHost.NetworkRxBytes, point.NetworkRxBytes, seconds)
		point.NetworkTxPeakRate = counterRate(a.previousHost.NetworkTxBytes, point.NetworkTxBytes, seconds)
		if a.previousHost.DiskIOAvailable && point.DiskIOAvailable {
			point.DiskReadPeakRate = counterRate(a.previousHost.DiskReadBytes, point.DiskReadBytes, seconds)
			point.DiskWritePeakRate = counterRate(a.previousHost.DiskWriteBytes, point.DiskWriteBytes, seconds)
		}
	}
	a.previousHost = point
	a.previousHostAt = at
	a.previousHostExists = true
	return point
}

func (a *hourlyAccumulator) containerPointWithRates(
	point diskContainerPoint,
	at time.Time,
) diskContainerPoint {
	if previous, exists := a.previousContainers[point.ID]; exists {
		seconds := at.Sub(previous.at).Seconds()
		point.NetworkRxPeakRate = counterRate(previous.point.NetworkRxBytes, point.NetworkRxBytes, seconds)
		point.NetworkTxPeakRate = counterRate(previous.point.NetworkTxBytes, point.NetworkTxBytes, seconds)
		point.BlockReadPeakRate = counterRate(previous.point.BlockReadBytes, point.BlockReadBytes, seconds)
		point.BlockWritePeakRate = counterRate(previous.point.BlockWriteBytes, point.BlockWriteBytes, seconds)
	}
	a.previousContainers[point.ID] = hourlyPreviousContainer{point: point, at: at}
	return point
}

func (a *hourlyAccumulator) evictOldestContainer() {
	var oldestID string
	var oldestAt time.Time
	for id, container := range a.containers {
		if oldestID == "" || container.seenAt.Before(oldestAt) ||
			(container.seenAt.Equal(oldestAt) && id < oldestID) {
			oldestID = id
			oldestAt = container.seenAt
		}
	}
	delete(a.containers, oldestID)
	delete(a.previousContainers, oldestID)
}

func (a *hourlyAccumulator) finalized() diskRecord {
	record := a.record
	if a.hostCPUCount > 0 {
		record.Host.CPUAveragePercent = a.hostCPUSum / float64(a.hostCPUCount)
		record.Host.CPUSampleCount = a.hostCPUCount
	}
	containers := make([]hourlyContainerAccumulator, 0, len(a.containers))
	for _, container := range a.containers {
		containers = append(containers, container)
	}
	sort.Slice(containers, func(i, j int) bool {
		if !containers[i].seenAt.Equal(containers[j].seenAt) {
			return containers[i].seenAt.After(containers[j].seenAt)
		}
		return containers[i].point.ID < containers[j].point.ID
	})
	if len(containers) > defaultMaxContainers {
		record.ContainerTruncated += len(containers) - defaultMaxContainers
		containers = containers[:defaultMaxContainers]
	}
	record.Containers = make([]diskContainerPoint, 0, len(containers))
	for _, container := range containers {
		point := container.point
		if container.cpuCount > 0 {
			point.CPUAveragePercent = container.cpuSum / float64(container.cpuCount)
			point.CPUSampleCount = container.cpuCount
		}
		record.Containers = append(record.Containers, point)
	}
	record.OperatorLatency = make([]diskOperatorLatencyPoint, 0, len(a.latency))
	for _, id := range a.latencyOrder {
		record.OperatorLatency = append(record.OperatorLatency, a.latency[id])
	}
	return record
}

func mergeHourlyHost(current *diskHostPoint, next diskHostPoint) {
	if next.CPUPercent > current.CPUPercent {
		current.CPUPercent = next.CPUPercent
	}
	if ratio(next.MemoryUsedBytes, next.MemoryTotalBytes) >
		ratio(current.MemoryUsedBytes, current.MemoryTotalBytes) {
		current.MemoryUsedBytes = next.MemoryUsedBytes
		current.MemoryTotalBytes = next.MemoryTotalBytes
	}
	if ratio(next.SwapUsedBytes, next.SwapTotalBytes) >
		ratio(current.SwapUsedBytes, current.SwapTotalBytes) {
		current.SwapUsedBytes = next.SwapUsedBytes
		current.SwapTotalBytes = next.SwapTotalBytes
	}
	if next.DiskPercent > current.DiskPercent {
		current.DiskUsedBytes = next.DiskUsedBytes
		current.DiskTotalBytes = next.DiskTotalBytes
		current.DiskPercent = next.DiskPercent
	}
	if next.NetworkRxPeakRate > current.NetworkRxPeakRate {
		current.NetworkRxPeakRate = next.NetworkRxPeakRate
	}
	if next.NetworkTxPeakRate > current.NetworkTxPeakRate {
		current.NetworkTxPeakRate = next.NetworkTxPeakRate
	}
	if next.DiskReadPeakRate > current.DiskReadPeakRate {
		current.DiskReadPeakRate = next.DiskReadPeakRate
	}
	if next.DiskWritePeakRate > current.DiskWritePeakRate {
		current.DiskWritePeakRate = next.DiskWritePeakRate
	}
	current.CPUCores = next.CPUCores
	if next.LoadOne > current.LoadOne {
		current.LoadOne = next.LoadOne
	}
	if next.LoadFive > current.LoadFive {
		current.LoadFive = next.LoadFive
	}
	if next.LoadFifteen > current.LoadFifteen {
		current.LoadFifteen = next.LoadFifteen
	}
	current.DiskIOAvailable = next.DiskIOAvailable
	current.DiskReadBytes = next.DiskReadBytes
	current.DiskWriteBytes = next.DiskWriteBytes
	current.NetworkRxBytes = next.NetworkRxBytes
	current.NetworkTxBytes = next.NetworkTxBytes
	if next.TCPConnections > current.TCPConnections {
		current.TCPConnections = next.TCPConnections
	}
	if next.UDPConnections > current.UDPConnections {
		current.UDPConnections = next.UDPConnections
	}
}

func mergeHourlyContainer(current *diskContainerPoint, next diskContainerPoint) {
	if next.CPUPercent > current.CPUPercent {
		current.CPUPercent = next.CPUPercent
	}
	if next.MemoryPercent > current.MemoryPercent {
		current.MemoryBytes = next.MemoryBytes
		current.MemoryLimitBytes = next.MemoryLimitBytes
		current.MemoryPercent = next.MemoryPercent
	}
	if next.NetworkRxPeakRate > current.NetworkRxPeakRate {
		current.NetworkRxPeakRate = next.NetworkRxPeakRate
	}
	if next.NetworkTxPeakRate > current.NetworkTxPeakRate {
		current.NetworkTxPeakRate = next.NetworkTxPeakRate
	}
	if next.BlockReadPeakRate > current.BlockReadPeakRate {
		current.BlockReadPeakRate = next.BlockReadPeakRate
	}
	if next.BlockWritePeakRate > current.BlockWritePeakRate {
		current.BlockWritePeakRate = next.BlockWritePeakRate
	}
	current.Name = next.Name
	current.Image = next.Image
	current.NetworkRxBytes = next.NetworkRxBytes
	current.NetworkTxBytes = next.NetworkTxBytes
	current.BlockReadBytes = next.BlockReadBytes
	current.BlockWriteBytes = next.BlockWriteBytes
	if next.PIDs > current.PIDs {
		current.PIDs = next.PIDs
	}
}

func (s *Service) updateHourly(record diskRecord) error {
	s.rollupMu.Lock()
	defer s.rollupMu.Unlock()
	hour := record.CollectedAt.UTC().Truncate(time.Hour)
	if s.hourly == nil {
		s.hourly = newHourlyAccumulator(record)
		return nil
	}
	if hour.Equal(s.hourly.hour) {
		s.hourly.add(record)
		return nil
	}
	if hour.Before(s.hourly.hour) {
		return nil
	}
	finalized := s.hourly.finalized()
	appendErr := s.appendHourlyRecord(finalized)
	// Advance even when this hour cannot be persisted. Otherwise one full or
	// temporarily unwritable shard pins the accumulator to the failed hour and
	// prevents every later rollup from recovering.
	s.hourly = s.hourly.next(record)
	if appendErr != nil {
		return appendErr
	}
	bytes, limitReached, err := s.pruneHourly(record.CollectedAt)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.status.RollupStorageBytes = bytes
	s.status.RollupStorageLimitReached = limitReached
	rolledAt := finalized.CollectedAt
	s.status.LastRollupAt = &rolledAt
	s.mu.Unlock()
	return nil
}

func (s *Service) currentHourlyRecord() (diskRecord, bool) {
	s.rollupMu.Lock()
	defer s.rollupMu.Unlock()
	if s.hourly == nil {
		return diskRecord{}, false
	}
	return s.hourly.finalized(), true
}

func (s *Service) restoreCurrentHour(now time.Time) error {
	start := now.UTC().Truncate(time.Hour)
	var accumulator *hourlyAccumulator
	_, _, err := s.scanRecords(context.Background(), start, now.UTC(), func(record diskRecord) {
		if accumulator == nil {
			accumulator = newHourlyAccumulator(record)
		} else {
			accumulator.add(record)
		}
	})
	if err != nil {
		return err
	}
	s.hourly = accumulator
	return nil
}

func (s *Service) appendHourlyRecord(record diskRecord) error {
	record = compactHourlyRecord(record)
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode hourly monitoring rollup: %w", err)
	}
	if len(data)+1 > maxHistoryLineBytes {
		return errors.New("hourly monitoring rollup exceeds the per-record limit")
	}
	path := filepath.Join(s.stateDir, "hourly-"+record.CollectedAt.Format("2006-01")+".jsonl")
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("hourly monitoring shard is not a regular file")
		}
		if info.Size()+int64(len(data)+1) > maxRollupShardBytes {
			return errors.New("hourly monitoring shard storage limit reached")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect hourly monitoring shard: %w", statErr)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open hourly monitoring shard: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect hourly monitoring shard: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append hourly monitoring rollup: %w", err)
	}
	return nil
}

func compactHourlyRecord(record diskRecord) diskRecord {
	containers := record.Containers
	if len(containers) > defaultMaxContainers {
		record.ContainerTruncated += len(containers) - defaultMaxContainers
		containers = containers[:defaultMaxContainers]
	}
	record.Containers = make([]diskContainerPoint, len(containers))
	copy(record.Containers, containers)
	for index := range record.Containers {
		record.Containers[index].Name = boundedText(
			record.Containers[index].Name,
			maxHistoricalContainerMetadataBytes,
		)
		// Image names are repeated metadata and can dominate a year of hourly
		// rollups. The active in-memory hour still supplies the current image.
		record.Containers[index].Image = ""
	}
	return record
}

func compactRawRecord(record diskRecord) diskRecord {
	record.Containers = append([]diskContainerPoint(nil), record.Containers...)
	for index := range record.Containers {
		record.Containers[index].Name = boundedText(
			record.Containers[index].Name,
			maxHistoricalContainerMetadataBytes,
		)
		record.Containers[index].Image = boundedText(
			record.Containers[index].Image,
			maxHistoricalContainerMetadataBytes,
		)
	}
	return record
}

func (s *Service) pruneHourly(now time.Time) (int64, bool, error) {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		return 0, false, fmt.Errorf("read hourly monitoring directory: %w", err)
	}
	type shard struct {
		name  string
		month time.Time
		size  int64
	}
	cutoff := midnightUTC(now.AddDate(0, 0, -s.rollupRetentionDays))
	shards := make([]shard, 0, 13)
	var total int64
	for _, entry := range entries {
		match := hourlyFilePattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			continue
		}
		month, parseErr := time.Parse("2006-01", match[1])
		if parseErr != nil {
			continue
		}
		path := filepath.Join(s.stateDir, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !month.AddDate(0, 1, 0).After(cutoff) {
			if removeErr := os.Remove(path); removeErr != nil {
				return total, false, fmt.Errorf("remove expired hourly monitoring shard: %w", removeErr)
			}
			continue
		}
		shards = append(shards, shard{name: entry.Name(), month: month, size: info.Size()})
		total += info.Size()
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].month.Before(shards[j].month) })
	current := "hourly-" + now.Format("2006-01") + ".jsonl"
	for _, item := range shards {
		if total <= s.maxRollupStorageBytes {
			break
		}
		if item.name == current {
			continue
		}
		if err := os.Remove(filepath.Join(s.stateDir, item.name)); err != nil {
			return total, true, fmt.Errorf("remove excess hourly monitoring shard: %w", err)
		}
		total -= item.size
	}
	return total, total >= s.maxRollupStorageBytes, nil
}

func (s *Service) scanHourlyRecords(
	ctx context.Context,
	start time.Time,
	end time.Time,
	consume func(diskRecord),
) (int64, int, error) {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		return 0, 0, fmt.Errorf("read hourly monitoring shards: %w", err)
	}
	type namedShard struct {
		name  string
		month time.Time
		size  int64
	}
	shards := make([]namedShard, 0, 13)
	for _, entry := range entries {
		match := hourlyFilePattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			continue
		}
		month, parseErr := time.Parse("2006-01", match[1])
		if parseErr != nil || month.After(end) || !month.AddDate(0, 1, 0).After(start) {
			continue
		}
		path := filepath.Join(s.stateDir, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		shards = append(shards, namedShard{name: entry.Name(), month: month, size: info.Size()})
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].month.Before(shards[j].month) })
	var scanned int64
	skipped := 0
	for _, shard := range shards {
		if err := ctx.Err(); err != nil {
			return scanned, skipped, err
		}
		if scanned+shard.size > s.maxRollupStorageBytes {
			break
		}
		file, openErr := os.Open(filepath.Join(s.stateDir, shard.name))
		if openErr != nil {
			return scanned, skipped, fmt.Errorf("open hourly monitoring shard: %w", openErr)
		}
		scanner := bufio.NewScanner(io.LimitReader(file, shard.size))
		scanner.Buffer(make([]byte, 64<<10), maxHistoryLineBytes)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				_ = file.Close()
				return scanned, skipped, err
			}
			var record diskRecord
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil ||
				record.Version != recordVersion {
				skipped++
				continue
			}
			if record.CollectedAt.Before(start) || record.CollectedAt.After(end) {
				continue
			}
			consume(record)
		}
		if scanner.Err() != nil {
			skipped++
		}
		if closeErr := file.Close(); closeErr != nil {
			return scanned, skipped, fmt.Errorf("close hourly monitoring shard: %w", closeErr)
		}
		scanned += shard.size
	}
	return scanned, skipped, nil
}
