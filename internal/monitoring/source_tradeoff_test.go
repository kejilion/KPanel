package monitoring

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// This is an experiment, not a supported query mode: current hourly rollups
// change partial-hour boundaries, container timestamps and retained metadata.
// Both sources are generated from identical raw samples before timing begins.
func BenchmarkHistoryThirtyDaySourceTradeoff(b *testing.B) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: b.TempDir(), System: fakeSystemSource{summary: testSummary(0, 0)},
		Now: func() time.Time { return now },
	})
	if err != nil {
		b.Fatal(err)
	}
	for index := 30 * 24 * 60; index >= 0; index-- {
		record := maximumRawRecord(now.Add(-time.Duration(index) * time.Minute))
		if err := service.appendRecord(record); err != nil {
			b.Fatal(err)
		}
		if err := service.updateHourly(record); err != nil {
			b.Fatal(err)
		}
	}
	spec, err := parseRange("30d")
	if err != nil {
		b.Fatal(err)
	}
	for _, candidate := range []struct {
		name   string
		hourly bool
	}{
		{"raw-current-semantics", false},
		{"hourly-changed-semantics", true},
	} {
		b.Run(candidate.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				history, err := service.queryHistory(context.Background(), spec, now.Add(-spec.duration), now, spec.bucket, candidate.hourly)
				if err != nil {
					b.Fatal(err)
				}
				data, err := json.Marshal(history)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(history.ScannedBytes), "scanned-B")
				b.ReportMetric(float64(len(data)), "response-B")
				b.ReportMetric(float64(len(history.Host)), "host-points")
			}
		})
	}
}
