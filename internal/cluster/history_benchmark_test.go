package cluster

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

// Vary measurements so payload compression is not measured on identical samples.
func historyPerformancePayload(b testing.TB) []byte {
	b.Helper()
	value := completeHistoryFixture(time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC), 360)
	random := rand.New(rand.NewPCG(123, 456))
	for i := range value.Host {
		value.Host[i].CPUPercent = random.Float64() * 100
		value.Host[i].MemoryUsedBytes += random.Uint64N(1 << 29)
		value.Host[i].NetworkRxRate = random.Float64() * 10000000
	}
	for i := range value.Containers {
		for j := range value.Containers[i].Points {
			p := &value.Containers[i].Points[j]
			p.CPUPercent = random.Float64() * 100
			p.MemoryBytes += random.Uint64N(1 << 27)
			p.NetworkRxRate = random.Float64() * 10000000
			p.BlockReadRate = random.Float64() * 1000000
		}
	}
	for i := range value.OperatorLatency {
		for j := range value.OperatorLatency[i].Points {
			latency := random.Float64() * 300
			value.OperatorLatency[i].Points[j].LatencyMilliseconds = &latency
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func BenchmarkHistoryPayload(b *testing.B) {
	data := historyPerformancePayload(b)
	var compressed bytes.Buffer
	writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	_, _ = writer.Write(data)
	_ = writer.Close()
	b.Logf("varied 32-container 6h history: plain=%d gzip=%d reduction=%.1f%%", len(data), compressed.Len(), 100*(1-float64(compressed.Len())/float64(len(data))))
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := decodeHistoryResponse(bytes.NewReader(data), monitoring.Query{Range: "6h"}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("gzip-fast", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(data)), "plain-bytes")
		b.ReportMetric(float64(compressed.Len()), "gzip-bytes")
		for b.Loop() {
			var output bytes.Buffer
			writer, _ := gzip.NewWriterLevel(&output, gzip.BestSpeed)
			_, _ = writer.Write(data)
			_ = writer.Close()
		}
	})
}
