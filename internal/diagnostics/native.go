package diagnostics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	nativeProvider             = "native"
	nativeCategoryID           = "core"
	nativeComprehensiveCheckID = "native-comprehensive"
	nativeCPUCheckID           = "native-cpu"
	nativeMemoryCheckID        = "native-memory"
	nativeDiskCheckID          = "native-disk"
	nativeRouteCheckID         = "native-route"
	nativeLatencyCheckID       = "native-latency"
	nativeSpeedCheckID         = "native-speed"
	nativeIPQualityCheckID     = "native-ip-quality"

	nativeCPUWindow          = 1200 * time.Millisecond
	nativeMemoryWindow       = 1200 * time.Millisecond
	nativeDiskBytes          = int64(64 << 20)
	nativeMemoryBytes        = 32 << 20
	nativeSpeedDownloadBytes = int64(8 << 20)
	nativeSpeedUploadBytes   = int64(2 << 20)
	nativeIPingEndpoint      = "https://api.iping.cc/v1/query"
	nativeIPingPageEndpoint  = "https://www.iping.cc/ip/"
	nativeIPingTimeout       = 5 * time.Second
	nativeIPingPageMaxBytes  = int64(512 << 10)
)

var nativeProbeOrder = []string{
	nativeCPUCheckID,
	nativeMemoryCheckID,
	nativeDiskCheckID,
	nativeRouteCheckID,
	nativeLatencyCheckID,
	nativeSpeedCheckID,
	nativeIPQualityCheckID,
}

var nativeHTTPClient = &http.Client{Timeout: 20 * time.Second, CheckRedirect: nativeProbeRedirect}

// Probes target fixed public HTTPS endpoints. Like every other outbound
// client, they follow only a short HTTPS-only redirect chain instead of the
// standard library's ten hops with scheme downgrades allowed.
func nativeProbeRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return errors.New("diagnostic probe redirect limit reached")
	}
	if request.URL.Scheme != "https" {
		return errors.New("diagnostic probe redirect left HTTPS")
	}
	return nil
}

var nativeIPingPageRiskPattern = regexp.MustCompile(`(?i)([0-9]{1,3}(?:\.[0-9]+)?)\s*%`)

type nativeProbeResult struct {
	Dimension string
	Metrics   []DiagnosticSummaryMetric
	Lines     []string
}

type nativeHTTPTarget struct {
	Name string
	URL  string
}

type ipingResponse struct {
	Code int       `json:"code"`
	Data ipingData `json:"data"`
	Msg  string    `json:"msg"`
}

type ipingData struct {
	ISP       string `json:"isp"`
	IsProxy   any    `json:"is_proxy"`
	UsageType string `json:"usage_type"`
	Type      string `json:"type"`
	RiskScore any    `json:"risk_score"`
	RiskTag   string `json:"risk_tag"`
	ASN       string `json:"asn"`
	ASOwner   string `json:"as_owner"`
}

var nativeLatencyTargets = []nativeHTTPTarget{
	{Name: "Cloudflare edge", URL: "https://www.cloudflare.com/cdn-cgi/trace"},
	{Name: "Google 204", URL: "https://www.google.com/generate_204"},
	{Name: "Apple success", URL: "https://www.apple.com/library/test/success.html"},
}

func nativeCatalog() Catalog {
	return Catalog{
		Categories: []Category{{ID: nativeCategoryID, Name: "KPanel 核心体检"}},
		Items: []Check{
			{
				ID: nativeComprehensiveCheckID, Category: nativeCategoryID,
				Name:        "KPanel 核心综合体检",
				Description: "由 KPanel Agent 在服务器本机执行性能探针，并从服务器出口完成路由、延迟、测速和 IP 基础质量检测。",
				Provider:    nativeProvider, EstimatedMinutes: 3, Impact: "network",
			},
			{
				ID: nativeCPUCheckID, Category: nativeCategoryID,
				Name:        "CPU 原生跑分",
				Description: "使用固定时长的本地整数运算，回显实际运算吞吐与 CPU 基础信息。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "light",
			},
			{
				ID: nativeMemoryCheckID, Category: nativeCategoryID,
				Name:        "内存原生跑分",
				Description: "使用受控内存块进行复制吞吐测试，避免依赖 sysbench 等外部工具。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "light",
			},
			{
				ID: nativeDiskCheckID, Category: nativeCategoryID,
				Name:        "硬盘原生跑分",
				Description: "在体检临时目录写入、同步并读取受控数据，回显顺序读写速度。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "intensive",
			},
			{
				ID: nativeRouteCheckID, Category: nativeCategoryID,
				Name:        "出口路由基础检测",
				Description: "通过固定探测点回显出口 ASN、Cloudflare 边缘节点和基础连通信息。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "network",
			},
			{
				ID: nativeLatencyCheckID, Category: nativeCategoryID,
				Name:        "延迟原生检测",
				Description: "由服务器向多个固定 HTTPS 探测点发起请求，回显平均延迟、抖动和失败率。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "network",
			},
			{
				ID: nativeSpeedCheckID, Category: nativeCategoryID,
				Name:        "网速原生测速",
				Description: "由服务器出口发起受控体积的下载与上传请求，回显本次实测上下行吞吐。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "network",
			},
			{
				ID: nativeIPQualityCheckID, Category: nativeCategoryID,
				Name:        "IP 基础质量检测",
				Description: "检测服务器出口公网 IP、ASN、地区、IPv4/IPv6 和反向解析，并按可用性补充 IPING 风险信息。",
				Provider:    nativeProvider, EstimatedMinutes: 1, Impact: "network",
			},
		},
	}
}

func mergeCatalogs(primary, secondary Catalog) Catalog {
	result := Catalog{
		Categories: append([]Category{}, primary.Categories...),
		Items:      append([]Check{}, primary.Items...),
	}
	categorySeen := make(map[string]bool, len(result.Categories))
	itemSeen := make(map[string]bool, len(result.Items))
	for _, category := range result.Categories {
		categorySeen[category.ID] = true
	}
	for _, item := range result.Items {
		itemSeen[item.ID] = true
	}
	for _, category := range secondary.Categories {
		if categorySeen[category.ID] {
			continue
		}
		categorySeen[category.ID] = true
		result.Categories = append(result.Categories, category)
	}
	for _, item := range secondary.Items {
		if itemSeen[item.ID] {
			continue
		}
		itemSeen[item.ID] = true
		if item.Provider == "" {
			item.Provider = "script"
		}
		result.Items = append(result.Items, item)
	}
	return result
}

func isNativeCheck(checkID, provider string) bool {
	return provider == nativeProvider || strings.HasPrefix(checkID, "native-")
}

func nativeCheckName(checkID string) string {
	for _, item := range nativeCatalog().Items {
		if item.ID == checkID {
			return item.Name
		}
	}
	return checkID
}

func (s *Service) runNativeJob(ctx context.Context, item record) error {
	workspace := filepath.Join(s.stateDir, item.ID+".work")
	if filepath.Dir(workspace) != s.stateDir {
		return s.fail(item, "workspace_invalid", errors.New("invalid diagnostic workspace"))
	}
	if err := os.Mkdir(workspace, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return s.fail(item, "workspace_unavailable", err)
	}
	defer os.RemoveAll(workspace)

	logFile, err := os.OpenFile(s.logPath(item.ID), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return s.fail(item, "log_unavailable", err)
	}
	defer logFile.Close()
	writer := &limitedWriter{target: logFile, remaining: maxLogBytes}
	_, _ = fmt.Fprintf(writer, "KPanel 原生体检：%s\n检测引擎：KPanel Native Diagnostics v1\n\n", item.CheckName)

	started := s.now().UTC()
	item.Provider = nativeProvider
	item.Interactive = false
	item.InputOpen = false
	item.Status = "running"
	item.Stage = "native_prepare"
	item.Progress = 5
	item.Message = "KPanel 原生体检已开始，正在准备本机探针"
	item.StartedAt = &started
	item.FinishedAt = nil
	if err := s.persistNativeJob(item); err != nil {
		return err
	}

	probeIDs := []string{item.CheckID}
	if item.CheckID == nativeComprehensiveCheckID {
		probeIDs = nativeProbeOrder
	}
	results := make([]nativeProbeResult, 0, len(probeIDs))
	failed := make([]string, 0)
	for index, probeID := range probeIDs {
		if err := ctx.Err(); err != nil {
			failed = append(failed, "任务被取消")
			break
		}
		item.Stage = strings.TrimPrefix(probeID, "native-")
		item.Progress = 10 + index*80/len(probeIDs)
		item.Message = fmt.Sprintf("正在进行 %s", nativeCheckName(probeID))
		if err := s.persistNativeJob(item); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(writer, "[%d/%d] %s\n", index+1, len(probeIDs), nativeCheckName(probeID))

		result, probeErr := runNativeProbe(ctx, probeID, workspace)
		if probeErr != nil {
			failed = append(failed, nativeCheckName(probeID)+"："+safeMessage(probeErr))
			_, _ = fmt.Fprintf(writer, "  未完成：%s\n\n", safeMessage(probeErr))
		} else {
			results = append(results, result)
			for _, line := range result.Lines {
				_, _ = fmt.Fprintf(writer, "  %s\n", line)
			}
			_, _ = io.WriteString(writer, "\n")
		}
		_ = logFile.Sync()
		item.Progress = 10 + (index+1)*85/len(probeIDs)
		if probeErr == nil {
			item.Message = fmt.Sprintf("%s 已完成，继续下一项", nativeCheckName(probeID))
		}
		if err := s.persistNativeJob(item); err != nil {
			return err
		}
	}

	item.Summary = nativeSummary(results)
	finished := s.now().UTC()
	item.Progress = 100
	item.FinishedAt = &finished
	item.InputOpen = false
	if ctx.Err() != nil {
		item.Status = "failed"
		item.Stage = "canceled"
		item.Message = "原生体检已取消，已完成的结果仍保留在任务记录中"
	} else if len(results) == 0 {
		item.Status = "failed"
		item.Stage = "failed"
		item.Message = "原生体检未采集到有效结果，请查看日志"
	} else {
		item.Status = "succeeded"
		if len(failed) > 0 {
			item.Stage = "partial"
			item.Message = fmt.Sprintf("原生体检完成，%d 项未完成；已完成结果已汇总", len(failed))
		} else {
			item.Stage = "completed"
			item.Message = "原生体检完成，真实结果已汇总"
		}
	}
	if len(failed) > 0 {
		_, _ = fmt.Fprintf(writer, "未完成项目：%s\n", strings.Join(failed, "；"))
	}
	_ = logFile.Sync()
	if err := s.persistNativeJob(item); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(results) == 0 {
		return errors.New("native diagnostics produced no result")
	}
	return nil
}

func (s *Service) persistNativeJob(item record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(item)
}

func nativeSummary(results []nativeProbeResult) *DiagnosticSummary {
	builder := newSummaryBuilder("kpanel-native-v1")
	for _, result := range results {
		for _, metric := range result.Metrics {
			builder.add(result.Dimension, metric.Key, metric.Value)
		}
	}
	return builder.build()
}

func runNativeProbe(ctx context.Context, checkID, workspace string) (nativeProbeResult, error) {
	switch checkID {
	case nativeCPUCheckID:
		return runNativeCPU(ctx)
	case nativeMemoryCheckID:
		return runNativeMemory(ctx)
	case nativeDiskCheckID:
		return runNativeDisk(ctx, workspace)
	case nativeRouteCheckID:
		return runNativeRoute(ctx)
	case nativeLatencyCheckID:
		return runNativeLatency(ctx)
	case nativeSpeedCheckID:
		return runNativeSpeed(ctx)
	case nativeIPQualityCheckID:
		return runNativeIPQuality(ctx)
	default:
		return nativeProbeResult{}, fmt.Errorf("unsupported native check: %s", checkID)
	}
}

func runNativeCPU(ctx context.Context) (nativeProbeResult, error) {
	model, cores := nativeCPUInfo()
	start := time.Now()
	deadline := start.Add(nativeCPUWindow)
	var value uint64 = 0x9e3779b97f4a7c15
	var operations uint64
	for time.Now().Before(deadline) {
		for index := 0; index < 1<<20; index++ {
			value = value*6364136223846793005 + uint64(index) + 1442695040888963407
			value ^= value >> 17
			operations++
			if index%8192 == 0 {
				if err := ctx.Err(); err != nil {
					return nativeProbeResult{}, err
				}
			}
		}
	}
	elapsed := time.Since(start)
	if operations == 0 || elapsed <= 0 {
		return nativeProbeResult{}, errors.New("CPU benchmark did not run")
	}
	opsPerSecond := float64(operations) / elapsed.Seconds()
	return nativeProbeResult{
		Dimension: "performance",
		Metrics: []DiagnosticSummaryMetric{
			{Key: "cpu_model", Value: model},
			{Key: "cpu_cores", Value: strconv.Itoa(cores)},
			{Key: "cpu_score", Value: fmt.Sprintf("%.0f KPS", opsPerSecond/1000)},
		},
		Lines: []string{
			"CPU：" + model,
			fmt.Sprintf("核心：%d · 原生运算吞吐：%.0f K ops/s", cores, opsPerSecond/1000),
		},
	}, nil
}

func runNativeMemory(ctx context.Context) (nativeProbeResult, error) {
	source := make([]byte, nativeMemoryBytes)
	destination := make([]byte, nativeMemoryBytes)
	for index := range source {
		source[index] = byte(index)
	}
	start := time.Now()
	deadline := start.Add(nativeMemoryWindow)
	var bytesMoved uint64
	for time.Now().Before(deadline) {
		copy(destination, source)
		copy(source, destination)
		bytesMoved += uint64(len(source) * 2)
		if err := ctx.Err(); err != nil {
			return nativeProbeResult{}, err
		}
	}
	elapsed := time.Since(start)
	if bytesMoved == 0 || elapsed <= 0 {
		return nativeProbeResult{}, errors.New("memory benchmark did not run")
	}
	throughput := float64(bytesMoved) / elapsed.Seconds()
	return nativeProbeResult{
		Dimension: "performance",
		Metrics:   []DiagnosticSummaryMetric{{Key: "memory_score", Value: formatNativeRate(throughput)}},
		Lines:     []string{fmt.Sprintf("内存复制吞吐：%s · 测试块：%s", formatNativeRate(throughput), formatNativeBytes(nativeMemoryBytes))},
	}, nil
}

func runNativeDisk(ctx context.Context, workspace string) (nativeProbeResult, error) {
	path := filepath.Join(workspace, "kpanel-native-disk.bin")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return nativeProbeResult{}, err
	}
	defer os.Remove(path)
	defer file.Close()
	block := bytes.Repeat([]byte{0x5a}, 1<<20)
	var written int64
	startWrite := time.Now()
	for written < nativeDiskBytes {
		if err := ctx.Err(); err != nil {
			return nativeProbeResult{}, err
		}
		remaining := nativeDiskBytes - written
		chunk := block
		if remaining < int64(len(chunk)) {
			chunk = chunk[:remaining]
		}
		count, writeErr := file.Write(chunk)
		written += int64(count)
		if writeErr != nil {
			return nativeProbeResult{}, writeErr
		}
		if count == 0 {
			return nativeProbeResult{}, errors.New("disk write returned zero bytes")
		}
	}
	if err := file.Sync(); err != nil {
		return nativeProbeResult{}, err
	}
	writeElapsed := time.Since(startWrite)
	if err := file.Close(); err != nil {
		return nativeProbeResult{}, err
	}

	readFile, err := os.Open(path)
	if err != nil {
		return nativeProbeResult{}, err
	}
	startRead := time.Now()
	readBytes, err := io.Copy(io.Discard, readFile)
	closeErr := readFile.Close()
	if err != nil {
		return nativeProbeResult{}, err
	}
	if closeErr != nil {
		return nativeProbeResult{}, closeErr
	}
	readElapsed := time.Since(startRead)
	if readBytes == 0 || writeElapsed <= 0 || readElapsed <= 0 {
		return nativeProbeResult{}, errors.New("disk benchmark did not run")
	}
	writeRate := float64(written) / writeElapsed.Seconds()
	readRate := float64(readBytes) / readElapsed.Seconds()
	return nativeProbeResult{
		Dimension: "performance",
		Metrics: []DiagnosticSummaryMetric{
			{Key: "disk_write", Value: formatNativeRate(writeRate)},
			{Key: "disk_read", Value: formatNativeRate(readRate)},
		},
		Lines: []string{
			fmt.Sprintf("顺序写入：%s", formatNativeRate(writeRate)),
			fmt.Sprintf("顺序读取：%s · 测试量：%s", formatNativeRate(readRate), formatNativeBytes(readBytes)),
		},
	}, nil
}

func runNativeRoute(ctx context.Context) (nativeProbeResult, error) {
	fields, elapsed, err := nativeTrace(ctx)
	if err != nil {
		return nativeProbeResult{}, err
	}
	asn := fields["asn"]
	colo := fields["colo"]
	location := fields["loc"]
	path := strings.Trim(strings.Join([]string{asn, colo}, " · "), " ·")
	if path == "" {
		path = "出口路径信息未返回"
	}
	return nativeProbeResult{
		Dimension: "route",
		Metrics: []DiagnosticSummaryMetric{
			{Key: "path", Value: path},
			{Key: "edge", Value: colo},
			{Key: "location", Value: location},
			{Key: "average", Value: formatNativeMilliseconds(elapsed)},
		},
		Lines: []string{fmt.Sprintf("出口路径：%s · 位置：%s · 探测响应：%s", path, location, formatNativeMilliseconds(elapsed))},
	}, nil
}

func runNativeLatency(ctx context.Context) (nativeProbeResult, error) {
	values := make([]float64, 0, len(nativeLatencyTargets)*3)
	failed := 0
	lines := make([]string, 0, len(nativeLatencyTargets))
	for _, target := range nativeLatencyTargets {
		targetValues := make([]float64, 0, 3)
		for attempt := 0; attempt < 3; attempt++ {
			start := time.Now()
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
			if requestErr != nil {
				failed++
				continue
			}
			response, doErr := nativeHTTPClient.Do(request)
			if doErr != nil {
				failed++
				continue
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
			if response.StatusCode >= http.StatusBadRequest {
				failed++
				continue
			}
			milliseconds := float64(time.Since(start).Microseconds()) / 1000
			targetValues = append(targetValues, milliseconds)
			values = append(values, milliseconds)
		}
		if len(targetValues) == 0 {
			lines = append(lines, target.Name+"：失败")
		} else {
			lines = append(lines, fmt.Sprintf("%s：平均 %.2f ms", target.Name, nativeAverage(targetValues)))
		}
	}
	if len(values) == 0 {
		return nativeProbeResult{}, errors.New("all latency targets failed")
	}
	minimum, maximum := values[0], values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	return nativeProbeResult{
		Dimension: "latency",
		Metrics: []DiagnosticSummaryMetric{
			{Key: "average", Value: formatNativeMilliseconds(nativeAverage(values))},
			{Key: "min", Value: formatNativeMilliseconds(minimum)},
			{Key: "max", Value: formatNativeMilliseconds(maximum)},
			{Key: "jitter", Value: formatNativeMilliseconds(maximum - minimum)},
			{Key: "loss", Value: fmt.Sprintf("%.1f%%", float64(failed)/float64(failed+len(values))*100)},
		},
		Lines: lines,
	}, nil
}

func runNativeSpeed(ctx context.Context) (nativeProbeResult, error) {
	downURL := fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", nativeSpeedDownloadBytes)
	downRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, downURL, nil)
	if err != nil {
		return nativeProbeResult{}, err
	}
	downStart := time.Now()
	downResponse, err := nativeHTTPClient.Do(downRequest)
	if err != nil {
		return nativeProbeResult{}, fmt.Errorf("下载测速失败: %w", err)
	}
	downBytes, copyErr := io.Copy(io.Discard, io.LimitReader(downResponse.Body, nativeSpeedDownloadBytes))
	_ = downResponse.Body.Close()
	if copyErr != nil {
		return nativeProbeResult{}, copyErr
	}
	if downResponse.StatusCode >= http.StatusBadRequest || downBytes == 0 {
		return nativeProbeResult{}, fmt.Errorf("下载测速返回 HTTP %d", downResponse.StatusCode)
	}

	uploadPayload := bytes.NewReader(bytes.Repeat([]byte{0x4b}, int(nativeSpeedUploadBytes)))
	upRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://speed.cloudflare.com/__up", uploadPayload)
	if err != nil {
		return nativeProbeResult{}, err
	}
	upRequest.Header.Set("Content-Type", "application/octet-stream")
	upStart := time.Now()
	upResponse, upErr := nativeHTTPClient.Do(upRequest)
	var upRate string
	if upErr == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(upResponse.Body, 64<<10))
		_ = upResponse.Body.Close()
		if upResponse.StatusCode < http.StatusBadRequest {
			upRate = formatNativeRate(float64(nativeSpeedUploadBytes) / time.Since(upStart).Seconds())
		}
	}

	downRate := formatNativeRate(float64(downBytes) / time.Since(downStart).Seconds())
	metrics := []DiagnosticSummaryMetric{{Key: "download", Value: downRate}}
	lines := []string{fmt.Sprintf("下载：%s · 实测 %s", downRate, formatNativeBytes(downBytes))}
	if upRate != "" {
		metrics = append(metrics, DiagnosticSummaryMetric{Key: "upload", Value: upRate})
		lines = append(lines, fmt.Sprintf("上传：%s · 测试量 %s", upRate, formatNativeBytes(nativeSpeedUploadBytes)))
	} else {
		lines = append(lines, "上传：本次探测点未返回有效结果，已保留下载结果")
	}
	return nativeProbeResult{Dimension: "speed", Metrics: metrics, Lines: lines}, nil
}

func runNativeIPQuality(ctx context.Context) (nativeProbeResult, error) {
	fields, elapsed, err := nativeTrace(ctx)
	if err != nil {
		return nativeProbeResult{}, err
	}
	publicIP := fields["ip"]
	if publicIP == "" {
		return nativeProbeResult{}, errors.New("trace response did not include public IP")
	}
	ipv4 := nativeDialStatus(ctx, "tcp", "1.1.1.1:443")
	ipv6 := nativeDialStatus(ctx, "tcp", "[2606:4700:4700::1111]:443")
	reverse := "未获取"
	lookupContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	names, lookupErr := net.DefaultResolver.LookupAddr(lookupContext, publicIP)
	cancel()
	if lookupErr == nil && len(names) > 0 {
		reverse = strings.TrimSuffix(names[0], ".")
	}
	statuses := []string{"IPv4 " + nativeStatusText(ipv4), "IPv6 " + nativeStatusText(ipv6)}
	asn := fields["asn"]
	location := fields["loc"]
	colo := fields["colo"]
	metrics := []DiagnosticSummaryMetric{
		{Key: "public_ip", Value: publicIP},
		{Key: "asn", Value: asn},
		{Key: "country", Value: location},
		{Key: "colo", Value: colo},
		{Key: "reverse_dns", Value: reverse},
		{Key: "ipv4_ipv6", Value: strings.Join(statuses, " · ")},
		{Key: "quality", Value: "基础信息已采集；IPING 风险信息待查询"},
	}
	lines := []string{
		fmt.Sprintf("公网 IP：%s · ASN：%s · 地区：%s", publicIP, asn, location),
		fmt.Sprintf("IPv4/IPv6：%s · 反向解析：%s", strings.Join(statuses, " · "), reverse),
		fmt.Sprintf("边缘节点：%s · 探测响应：%s", colo, formatNativeMilliseconds(elapsed)),
	}
	parsedPublicIP := net.ParseIP(publicIP)
	if parsedPublicIP == nil || parsedPublicIP.To4() == nil {
		lines = append(lines, "IPING：当前出口不是 IPv4，按接口能力跳过风险与运营商查询")
		return nativeProbeResult{Dimension: "ip", Metrics: metrics, Lines: lines}, nil
	}
	data, queryErr := queryIPing(ctx, publicIP)
	if queryErr != nil {
		lines = append(lines, "IPING：查询未完成，已保留 KPanel 原生 IP 数据 · "+safeMessage(queryErr))
		return nativeProbeResult{Dimension: "ip", Metrics: metrics, Lines: lines}, nil
	}
	metrics = append(metrics, ipingMetrics(data)...)
	metrics = replaceMetric(metrics, "quality", "已接入 IPING 风险与运营商数据")
	lines = append(lines, formatIPingLine(data))
	return nativeProbeResult{
		Dimension: "ip",
		Metrics:   metrics,
		Lines:     lines,
	}, nil
}

func queryIPing(ctx context.Context, publicIP string) (ipingData, error) {
	return queryIPingEndpoints(ctx, publicIP, nativeIPingEndpoint, nativeIPingPageEndpoint)
}

func queryIPingEndpoints(ctx context.Context, publicIP, apiEndpoint, pageEndpoint string) (ipingData, error) {
	type apiResult struct {
		data ipingData
		err  error
	}
	type pageResult struct {
		score float64
		err   error
	}

	apiResults := make(chan apiResult, 1)
	pageResults := make(chan pageResult, 1)
	go func() {
		data, err := queryIPingEndpoint(ctx, publicIP, apiEndpoint)
		apiResults <- apiResult{data: data, err: err}
	}()
	go func() {
		score, err := queryIPingPageRiskScore(ctx, publicIP, pageEndpoint)
		pageResults <- pageResult{score: score, err: err}
	}()

	api := <-apiResults
	page := <-pageResults
	if page.err == nil {
		api.data.RiskScore = page.score
		return api.data, nil
	}
	return api.data, api.err
}

func queryIPingEndpoint(ctx context.Context, publicIP, endpoint string) (ipingData, error) {
	parsed := net.ParseIP(publicIP)
	if parsed == nil || parsed.To4() == nil {
		return ipingData{}, errors.New("IPING only supports IPv4")
	}
	query := url.Values{}
	query.Set("ip", publicIP)
	query.Set("language", "en")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return ipingData{}, err
	}
	request.Header.Set("Accept", "application/json")
	requestContext, cancel := context.WithTimeout(ctx, nativeIPingTimeout)
	defer cancel()
	request = request.WithContext(requestContext)
	response, err := nativeHTTPClient.Do(request)
	if err != nil {
		return ipingData{}, fmt.Errorf("IPING request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return ipingData{}, fmt.Errorf("IPING returned HTTP %d", response.StatusCode)
	}
	var payload ipingResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return ipingData{}, fmt.Errorf("decode IPING response: %w", err)
	}
	if payload.Code != http.StatusOK {
		message := strings.TrimSpace(payload.Msg)
		if message == "" {
			message = "接口返回非 success"
		}
		return ipingData{}, fmt.Errorf("IPING code %d: %s", payload.Code, message)
	}
	return payload.Data, nil
}

func queryIPingPageRiskScore(ctx context.Context, publicIP, endpoint string) (float64, error) {
	parsed := net.ParseIP(publicIP)
	if parsed == nil || parsed.To4() == nil {
		return 0, errors.New("IPING only supports IPv4")
	}
	requestContext, cancel := context.WithTimeout(ctx, nativeIPingTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		strings.TrimRight(endpoint, "/")+"/"+url.PathEscape(publicIP),
		nil,
	)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Accept-Language", "en")
	response, err := nativeHTTPClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("IPING page request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("IPING page returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nativeIPingPageMaxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("read IPING page: %w", err)
	}
	if int64(len(body)) > nativeIPingPageMaxBytes {
		return 0, errors.New("IPING page response exceeded 512 KiB")
	}
	if score, ok := parseIPingPageRiskScore(string(body)); ok {
		return score, nil
	}
	return 0, errors.New("IPING page did not include a risk score")
}

func parseIPingPageRiskScore(document string) (float64, bool) {
	normalized := strings.ToLower(document)
	for _, marker := range []string{"ip threat level", "risk rating", "风险等级", "風險等級"} {
		start := strings.Index(normalized, marker)
		if start < 0 {
			continue
		}
		end := min(len(document), start+2048)
		match := nativeIPingPageRiskPattern.FindStringSubmatch(document[start:end])
		if len(match) != 2 {
			continue
		}
		if score, ok := ipingRiskScore(match[1]); ok {
			return score, true
		}
	}
	return 0, false
}

func ipingMetrics(data ipingData) []DiagnosticSummaryMetric {
	metrics := make([]DiagnosticSummaryMetric, 0, 8)
	add := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			metrics = append(metrics, DiagnosticSummaryMetric{Key: key, Value: value})
		}
	}
	add("isp", data.ISP)
	add("asn", data.ASN)
	add("as_owner", data.ASOwner)
	add("usage_type", data.UsageType)
	add("ip_type", data.Type)
	if score, ok := ipingRiskScore(data.RiskScore); ok {
		add("risk_score", formatSummaryNumber(score))
		add("risk_level", ipingRiskLevelForScore(score))
	}
	add("risk_tag", data.RiskTag)
	if proxy, ok := jsonBool(data.IsProxy); ok {
		if proxy {
			add("is_proxy", "是")
		} else {
			add("is_proxy", "否")
		}
	} else {
		add("is_proxy", jsonString(data.IsProxy))
	}
	return metrics
}

func ipingRiskScore(value any) (float64, bool) {
	score, err := strconv.ParseFloat(strings.TrimSpace(jsonString(value)), 64)
	if err != nil || score < 0 || score > 100 {
		return 0, false
	}
	return score, true
}

func ipingRiskLevelForScore(score float64) string {
	switch {
	case score >= 80:
		return "高风险"
	case score >= 30:
		return "中风险"
	default:
		return "低风险"
	}
}

func formatIPingLine(data ipingData) string {
	parts := []string{"IPING"}
	if data.ISP != "" {
		parts = append(parts, "ISP："+data.ISP)
	}
	if data.ASN != "" {
		parts = append(parts, data.ASN)
	}
	if score, ok := ipingRiskScore(data.RiskScore); ok {
		parts = append(parts, "风险分："+formatSummaryNumber(score)+"%", ipingRiskLevelForScore(score))
	}
	return strings.Join(parts, " · ")
}

func replaceMetric(metrics []DiagnosticSummaryMetric, key, value string) []DiagnosticSummaryMetric {
	for index := range metrics {
		if metrics[index].Key == key {
			metrics[index].Value = value
			return metrics
		}
	}
	return append(metrics, DiagnosticSummaryMetric{Key: key, Value: value})
}

func nativeTrace(ctx context.Context) (map[string]string, float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return nil, 0, err
	}
	start := time.Now()
	response, err := nativeHTTPClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return nil, 0, fmt.Errorf("trace response returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, 0, err
	}
	fields := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		parts := strings.SplitN(strings.TrimSpace(scanner.Text()), "=", 2)
		if len(parts) == 2 && parts[0] != "" {
			fields[parts[0]] = strings.TrimSpace(parts[1])
		}
	}
	return fields, float64(time.Since(start).Microseconds()) / 1000, nil
}

func nativeDialStatus(parent context.Context, network, address string) bool {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func nativeCPUInfo() (string, int) {
	model := runtime.GOARCH + " CPU"
	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Hardware") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
					model = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}
	return model, cores
}

func nativeAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func formatNativeRate(value float64) string {
	units := []string{"B/s", "KiB/s", "MiB/s", "GiB/s"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func formatNativeBytes(value interface{}) string {
	var bytesValue float64
	switch typed := value.(type) {
	case int:
		bytesValue = float64(typed)
	case int64:
		bytesValue = float64(typed)
	default:
		return "未知"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	unit := 0
	for bytesValue >= 1024 && unit < len(units)-1 {
		bytesValue /= 1024
		unit++
	}
	return fmt.Sprintf("%.2f %s", bytesValue, units[unit])
}

func formatNativeMilliseconds(value float64) string {
	return fmt.Sprintf("%.2f ms", value)
}

func nativeStatusText(connected bool) string {
	if connected {
		return "已连接"
	}
	return "不可用"
}
