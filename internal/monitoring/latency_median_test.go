package monitoring

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func latencyRecord(at time.Time, latency float64, reachable bool) diskRecord {
	record := maximumRawRecord(at)
	for index := range record.OperatorLatency {
		record.OperatorLatency[index].MedianMilliseconds = nil
		record.OperatorLatency[index].MinimumMilliseconds = nil
	}
	record.OperatorLatency[0].Reachable = reachable
	record.OperatorLatency[0].LatencyMilliseconds = 0
	record.OperatorLatency[0].SuccessCount, record.OperatorLatency[0].FailureCount = 0, 1
	if reachable {
		record.OperatorLatency[0].LatencyMilliseconds = latency
		record.OperatorLatency[0].SuccessCount, record.OperatorLatency[0].FailureCount = 1, 0
	}
	return record
}

func TestHourlyRollupKeepsLatencyMedianAndMinimum(t *testing.T) {
	at := time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC)
	samples := []struct {
		latency   float64
		reachable bool
	}{{30, true}, {10, true}, {20, true}, {0, false}, {40, true}}
	accumulator := newHourlyAccumulator(latencyRecord(at, samples[0].latency, samples[0].reachable))
	for index, sample := range samples[1:] {
		accumulator.add(latencyRecord(at.Add(time.Duration(index+1)*5*time.Minute), sample.latency, sample.reachable))
	}
	got := accumulator.finalized().OperatorLatency[0]
	if got.LatencyMilliseconds != 40 || got.SuccessCount != 4 || got.FailureCount != 1 ||
		got.MedianMilliseconds == nil || *got.MedianMilliseconds != 25 ||
		got.MinimumMilliseconds == nil || *got.MinimumMilliseconds != 10 {
		t.Fatalf("hourly latency = %#v", got)
	}

	// The rollup survives the on-disk round trip; a line written before the
	// fields existed decodes without them instead of failing.
	encoded, err := json.Marshal(compactHourlyRecord(accumulator.finalized()))
	if err != nil {
		t.Fatal(err)
	}
	var decoder diskRecordDecoder
	decoded, err := decoder.decode(encoded)
	if err != nil || decoded.OperatorLatency[0].MedianMilliseconds == nil || *decoded.OperatorLatency[0].MedianMilliseconds != 25 {
		t.Fatalf("decoded rollup = %#v, %v", decoded.OperatorLatency, err)
	}
	legacy := strings.NewReplacer(`,"md":25`, "", `,"mn":10`, "").Replace(string(encoded))
	decoded, err = decoder.decode([]byte(legacy))
	if err != nil || decoded.OperatorLatency[0].MedianMilliseconds != nil || decoded.OperatorLatency[0].MinimumMilliseconds != nil {
		t.Fatalf("legacy rollup = %#v, %v", decoded.OperatorLatency, err)
	}
}

func TestHourlyRollupWithoutSuccessHasNoMedian(t *testing.T) {
	at := time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC)
	accumulator := newHourlyAccumulator(latencyRecord(at, 0, false))
	accumulator.add(latencyRecord(at.Add(5*time.Minute), 0, false))
	got := accumulator.finalized().OperatorLatency[0]
	if got.Reachable || got.MedianMilliseconds != nil || got.MinimumMilliseconds != nil || got.FailureCount != 2 {
		t.Fatalf("all-timeout hour = %#v", got)
	}
}

func TestMedianHelpers(t *testing.T) {
	cases := []struct {
		samples         []float64
		median, minimum float64
		ok              bool
	}{
		{nil, 0, 0, false},
		{[]float64{3, 1, 2}, 2, 1, true},
		{[]float64{4, 1, 3, 2}, 2.5, 1, true},
	}
	for _, item := range cases {
		original := append([]float64(nil), item.samples...)
		median, minimum, ok := medianAndMinimum(item.samples)
		if median != item.median || minimum != item.minimum || ok != item.ok {
			t.Fatalf("medianAndMinimum(%v) = %v, %v, %v", item.samples, median, minimum, ok)
		}
		for index := range original {
			if item.samples[index] != original[index] {
				t.Fatalf("medianAndMinimum reordered its input: %v", item.samples)
			}
		}
	}
	weighted := []struct {
		samples []latencySample
		want    float64
		ok      bool
	}{
		{nil, 0, false},
		{[]latencySample{{30, 1}, {10, 1}, {20, 1}}, 20, true},
		{[]latencySample{{10, 1}, {20, 1}}, 10, true},
		{[]latencySample{{100, 1}, {10, 9}}, 10, true},
		{[]latencySample{{10, 1}, {100, 9}}, 100, true},
	}
	for _, item := range weighted {
		got, ok := weightedMedian(item.samples)
		if got != item.want || ok != item.ok {
			t.Fatalf("weightedMedian(%v) = %v, %v", item.samples, got, ok)
		}
	}
}

func TestHistoryLatencyPointsCarryMedianBesidePeak(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: t.TempDir(), System: fakeSystemSource{summary: testSummary(0, 0)},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	start := now.Add(-2 * time.Hour)
	// Twenty-four 5-minute probes: 20 ms, with every sixth spiking to 500 ms.
	for index := 0; index < 24; index++ {
		latency := 20.0
		if index%6 == 5 {
			latency = 500
		}
		record := latencyRecord(start.Add(time.Duration(index)*5*time.Minute), latency, true)
		if err := service.appendRecord(record); err != nil {
			t.Fatal(err)
		}
		if err := service.updateHourly(record); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := parseRange("6h")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name   string
		hourly bool
		bucket time.Duration
	}{{"raw", false, 30 * time.Minute}, {"hourly", true, time.Hour}} {
		history, err := service.queryHistory(context.Background(), spec, start, now, mode.bucket, mode.hourly)
		if err != nil {
			t.Fatalf("%s query: %v", mode.name, err)
		}
		points := history.OperatorLatency[0].Points
		if len(points) < 2 {
			t.Fatalf("%s points = %#v", mode.name, points)
		}
		for _, point := range points {
			if point.LatencyMilliseconds == nil || *point.LatencyMilliseconds != 500 ||
				point.MedianMilliseconds == nil || *point.MedianMilliseconds != 20 ||
				point.MinimumMilliseconds == nil || *point.MinimumMilliseconds != 20 {
				t.Fatalf("%s point keeps only the spike: %#v", mode.name, point)
			}
		}
		encoded, err := json.Marshal(history.OperatorLatency[0].Points[0])
		if err != nil || !strings.Contains(string(encoded), `"medianMilliseconds":20`) || !strings.Contains(string(encoded), `"latencyMilliseconds":500`) {
			t.Fatalf("%s wire point = %s, %v", mode.name, encoded, err)
		}
	}
}

func TestHistoryLatencyFromLegacyRollupsKeepsPeakOnly(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	service, err := New(Config{
		StateDir: t.TempDir(), System: fakeSystemSource{summary: testSummary(0, 0)},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := latencyRecord(now.Add(-3*time.Hour).Truncate(time.Hour), 80, true)
	legacy.OperatorLatency[0].SuccessCount = 12
	if err := service.appendHourlyRecord(legacy); err != nil {
		t.Fatal(err)
	}
	spec, err := parseRange("6h")
	if err != nil {
		t.Fatal(err)
	}
	history, err := service.queryHistory(context.Background(), spec, now.Add(-6*time.Hour), now, time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	points := history.OperatorLatency[0].Points
	if len(points) != 1 || points[0].LatencyMilliseconds == nil || *points[0].LatencyMilliseconds != 80 ||
		points[0].MedianMilliseconds != nil || points[0].MinimumMilliseconds != nil {
		t.Fatalf("legacy rollup point = %#v", points)
	}
}
