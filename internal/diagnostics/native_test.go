package diagnostics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper testRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestNativeCatalogIsIndependentFromScripts(t *testing.T) {
	catalog := nativeCatalog()
	if len(catalog.Categories) != 1 || catalog.Categories[0].ID != nativeCategoryID {
		t.Fatalf("native categories = %#v", catalog.Categories)
	}
	if len(catalog.Items) < 7 {
		t.Fatalf("native items = %#v", catalog.Items)
	}
	for _, item := range catalog.Items {
		if item.Provider != nativeProvider || !strings.HasPrefix(item.ID, "native-") {
			t.Fatalf("native item is not marked native: %#v", item)
		}
	}
}

func TestMergeCatalogKeepsNativeChecksBeforeOptionalScripts(t *testing.T) {
	merged := mergeCatalogs(nativeCatalog(), Catalog{
		Categories: []Category{{ID: "network", Name: "Network"}},
		Items:      []Check{{ID: "mtr", Category: "network", Name: "MTR", Provider: ""}},
	})
	if len(merged.Items) != len(nativeCatalog().Items)+1 {
		t.Fatalf("merged items = %#v", merged.Items)
	}
	if merged.Items[0].ID != nativeComprehensiveCheckID || merged.Items[len(merged.Items)-1].Provider != "script" {
		t.Fatalf("merged order/provider = %#v", merged.Items)
	}
}

func TestNativeSummaryOnlyContainsProbeMetrics(t *testing.T) {
	summary := nativeSummary([]nativeProbeResult{
		{Dimension: "performance", Metrics: []DiagnosticSummaryMetric{
			{Key: "cpu_score", Value: "123 KPS"},
		}},
		{Dimension: "ip", Metrics: []DiagnosticSummaryMetric{
			{Key: "public_ip", Value: "203.0.113.10"},
			{Key: "quality", Value: "基础信息已采集；未接入第三方信誉库"},
		}},
	})
	if summary == nil || summary.Parser != "kpanel-native-v1" {
		t.Fatalf("native summary = %#v", summary)
	}
	if got := summaryMetricValue(summary, "performance", "cpu_score"); got != "123 KPS" {
		t.Fatalf("cpu score = %q", got)
	}
	if got := summaryMetricValue(summary, "ip", "public_ip"); got != "203.0.113.10" {
		t.Fatalf("public ip = %q", got)
	}
}

func TestIPingQueryMapsOnlySelectedMetrics(t *testing.T) {
	previousHTTPClient := nativeHTTPClient
	t.Cleanup(func() {
		nativeHTTPClient = previousHTTPClient
	})

	var requestedIP string
	var requestedLanguage string
	nativeHTTPClient = &http.Client{Timeout: 20 * time.Second, Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
		requestedIP = request.URL.Query().Get("ip")
		requestedLanguage = request.URL.Query().Get("language")
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":200,"data":{"ip":"203.0.113.10","isp":"Example ISP","is_proxy":"true","usage_type":"IDC","type":"native","risk_score":97,"risk_tag":"proxy","asn":"AS64500","as_owner":"Example Network","company":"must-not-map","country":"JP"},"msg":"success"}`)),
			Request:    request,
		}, nil
	})}

	data, err := queryIPingEndpoint(context.Background(), "203.0.113.10", "http://iping.test/v1/query")
	if err != nil {
		t.Fatalf("queryIPingEndpoint() error = %v", err)
	}
	if requestedIP != "203.0.113.10" || requestedLanguage != "en" {
		t.Fatalf("IPING query = ip %q, language %q", requestedIP, requestedLanguage)
	}

	metrics := ipingMetrics(data)
	values := make(map[string]string, len(metrics))
	for _, metric := range metrics {
		values[metric.Key] = metric.Value
	}
	want := map[string]string{
		"isp":        "Example ISP",
		"is_proxy":   "是",
		"usage_type": "IDC",
		"ip_type":    "native",
		"risk_score": "97",
		"risk_level": "高风险",
		"risk_tag":   "proxy",
		"asn":        "AS64500",
		"as_owner":   "Example Network",
	}
	for key, expected := range want {
		if values[key] != expected {
			t.Fatalf("metric %s = %q, want %q", key, values[key], expected)
		}
	}
	for _, key := range []string{"company", "country", "ip"} {
		if _, ok := values[key]; ok {
			t.Fatalf("unselected metric %s was mapped: %#v", key, values)
		}
	}
}

func TestIPingQueryRejectsIPv6(t *testing.T) {
	if _, err := queryIPingEndpoint(context.Background(), "2001:db8::1", "http://127.0.0.1"); err == nil {
		t.Fatal("queryIPingEndpoint() accepted IPv6")
	}
}

func TestIPingPageRiskScoreOverridesConflictingAPIValue(t *testing.T) {
	previousHTTPClient := nativeHTTPClient
	t.Cleanup(func() {
		nativeHTTPClient = previousHTTPClient
	})

	var requestedPagePath string
	nativeHTTPClient = &http.Client{Timeout: 20 * time.Second, Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{"code":200,"data":{"risk_score":0,"isp":"Example ISP"},"msg":"success"}`
		contentType := "application/json"
		if request.URL.Host == "www.iping.test" {
			body = `<span>IP Threat Level:</span><div class="right"><div class="tag green"><div>7%</div>No Risk</div></div>`
			contentType = "text/html"
			requestedPagePath = request.URL.Path
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}

	data, err := queryIPingEndpoints(
		context.Background(),
		"158.179.20.115",
		"https://api.iping.test/v1/query",
		"https://www.iping.test/ip/",
	)
	if err != nil {
		t.Fatalf("queryIPingEndpoints() error = %v", err)
	}
	if requestedPagePath != "/ip/158.179.20.115" {
		t.Fatalf("IPING page path = %q", requestedPagePath)
	}
	if score, ok := ipingRiskScore(data.RiskScore); !ok || score != 7 {
		t.Fatalf("IPING risk score = %#v, want 7", data.RiskScore)
	}
	if data.ISP != "Example ISP" {
		t.Fatalf("IPING API metadata was not retained: %#v", data)
	}
}

func TestIPingPageFailureKeepsAPIRiskScore(t *testing.T) {
	previousHTTPClient := nativeHTTPClient
	t.Cleanup(func() {
		nativeHTTPClient = previousHTTPClient
	})

	nativeHTTPClient = &http.Client{Timeout: 20 * time.Second, Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{"code":200,"data":{"risk_score":23},"msg":"success"}`
		contentType := "application/json"
		if request.URL.Host == "www.iping.test" {
			body = `<html><body>risk temporarily unavailable</body></html>`
			contentType = "text/html"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}

	data, err := queryIPingEndpoints(
		context.Background(),
		"203.0.113.10",
		"https://api.iping.test/v1/query",
		"https://www.iping.test/ip/",
	)
	if err != nil {
		t.Fatalf("queryIPingEndpoints() error = %v", err)
	}
	if score, ok := ipingRiskScore(data.RiskScore); !ok || score != 23 {
		t.Fatalf("IPING fallback risk score = %#v, want 23", data.RiskScore)
	}
}

func TestParseIPingPageRiskScoreRejectsUnrelatedPercentages(t *testing.T) {
	if score, ok := parseIPingPageRiskScore(`<style>body { width: 100% }</style><p>No risk result</p>`); ok {
		t.Fatalf("unrelated percentage parsed as risk score %v", score)
	}
	if score, ok := parseIPingPageRiskScore(`<span>风险等级：</span><div><b>7%</b> 无风险</div>`); !ok || score != 7 {
		t.Fatalf("localized IPING page risk score = %v, %v", score, ok)
	}
}

func TestNativeLocalProbesReturnMeasuredMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	workspace := t.TempDir()
	for _, checkID := range []string{nativeCPUCheckID, nativeMemoryCheckID, nativeDiskCheckID} {
		result, err := runNativeProbe(ctx, checkID, workspace)
		if err != nil {
			t.Fatalf("runNativeProbe(%s) error = %v", checkID, err)
		}
		if result.Dimension != "performance" || len(result.Metrics) == 0 || len(result.Lines) == 0 {
			t.Fatalf("runNativeProbe(%s) = %#v", checkID, result)
		}
	}
}

func TestNativeProbeRedirectStaysShortAndHTTPS(t *testing.T) {
	next := func(raw string) *http.Request {
		request, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return request
	}
	hop := next("https://example.com/")
	if err := nativeProbeRedirect(next("https://example.com/a"), []*http.Request{hop}); err != nil {
		t.Fatalf("single HTTPS redirect rejected: %v", err)
	}
	if err := nativeProbeRedirect(next("http://example.com/a"), []*http.Request{hop}); err == nil {
		t.Fatal("HTTPS downgrade was followed")
	}
	if err := nativeProbeRedirect(next("https://example.com/d"), []*http.Request{hop, hop, hop}); err == nil {
		t.Fatal("fourth redirect was followed")
	}
}
