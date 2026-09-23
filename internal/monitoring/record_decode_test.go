package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHistoryCanceledScanReleasesQuerySlots(t *testing.T) {
	for _, rangeValue := range []string{"30d", "12m"} {
		t.Run(rangeValue, func(t *testing.T) {
			now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
			service, err := New(Config{
				StateDir: t.TempDir(), System: fakeSystemSource{summary: testSummary(0, 0)},
				Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			record := maximumRawRecord(now)
			if err := service.appendRecord(record); err != nil {
				t.Fatal(err)
			}
			if err := service.appendHourlyRecord(record); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := service.History(ctx, rangeValue); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled query returned %v", err)
			}
			if history, err := service.History(context.Background(), rangeValue); err != nil || len(history.Host) != 1 {
				t.Fatalf("query after cancellation failed: points=%d err=%v", len(history.Host), err)
			}
		})
	}
}

func diskDecoderSeeds() [][]byte {
	seeds := []string{
		`{}`, `null`, `[]`, `true`, `1`, `"record"`, ``, `{} {}`,
		`{"v":1,"at":"2026-09-23T12:00:00Z","h":{"cp":12.5,"nr":18446744073709551615}}`,
		`{"v":1,"V":2,"h":{"cp":1},"h":{"cn":2}}`,
		`{"\u0076":1,"h":{"cp":1e2},"c":[{"i":"a","n":"中文\\\"\ud83d\ude00"}]}`,
		`{"v":1,"h":null,"c":[null,{}],"ol":[null,{}]}`,
		`{"c":[{"n":"first"}],"c":[{"i":"second"}]}`,
		`{"c":[],"ol":[]}`, `{"c":null,"ol":null}`,
		`{"c":[{"i":"partial","cp":true}]}`, `{"c":[{}],"ol":[{}]}`,
		`{"at":"2026-09-23T20:00:00.123456789+08:00"}`,
		`{"at":"invalid"}`, `{"v":1.1}`, `{"h":{"nr":-1}}`,
		`{"h":{"nr":18446744073709551616}}`, `{"h":{"cp":1e309}}`,
		`{"h":{"nr":36000000000000000000}}`, `{"ct":25000000000000000000}`,
		`{"at":"2026-09-23T12:00:00,1Z"}`,
		`{"h":{"cp":"1"}}`, `{"h":{"cp":NaN}}`, `{"unknown":1e309}`,
		`{"c":[{"n":"\ud800"}]}`, `{"c":[{"n":"\udfff"}]}`,
		`{"unknown":` + strings.Repeat("[", 10001) + "0" + strings.Repeat("]", 10001) + "}",
		"{\"c\":[{\"n\":\"\xff\xfe\"}]}",
	}
	data, err := json.Marshal(maximumRawRecord(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		panic(err)
	}
	result := [][]byte{data}
	for _, seed := range seeds {
		result = append(result, []byte(seed))
	}
	return result
}

func compareDiskDecoder(t testing.TB, decoder *diskRecordDecoder, content []byte) {
	t.Helper()
	var want diskRecord
	wantErr := json.Unmarshal(content, &want)
	got, gotErr := decoder.decode(content)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("acceptance differs: standard=%v reused=%v", wantErr, gotErr)
	}
	if wantErr != nil {
		return
	}
	// Empty/missing slices are both omitted by the disk format and are consumed
	// identically by query/rollup callers. Compare every actual point and field.
	if len(got.Containers) == 0 {
		got.Containers = nil
	}
	if len(want.Containers) == 0 {
		want.Containers = nil
	}
	if len(got.OperatorLatency) == 0 {
		got.OperatorLatency = nil
	}
	if len(want.OperatorLatency) == 0 {
		want.OperatorLatency = nil
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("values differ for %q\nwant=%+v\ngot=%+v", content, want, got)
	}
}

func TestDiskRecordDecoderDoesNotLeakPreviousRows(t *testing.T) {
	seeds := diskDecoderSeeds()
	for _, first := range seeds {
		for _, next := range seeds {
			var decoder diskRecordDecoder
			compareDiskDecoder(t, &decoder, seeds[0])
			compareDiskDecoder(t, &decoder, first)
			compareDiskDecoder(t, &decoder, next)
		}
	}
}

func TestDiskRecordDecoderRetainsOnlyBoundedScratch(t *testing.T) {
	var decoder diskRecordDecoder
	compareDiskDecoder(t, &decoder, diskDecoderSeeds()[0])
	if len(decoder.containers) == 0 || len(decoder.latency) == 0 {
		t.Fatal("normal records should reuse their bounded arrays")
	}
	oversized := diskRecord{
		Containers:      make([]diskContainerPoint, maxScannedSeries*4),
		OperatorLatency: make([]diskOperatorLatencyPoint, MaxChecks*4),
	}
	data, err := json.Marshal(oversized)
	if err != nil {
		t.Fatal(err)
	}
	compareDiskDecoder(t, &decoder, data)
	if cap(decoder.containers) > maxScannedSeries || cap(decoder.latency) > MaxChecks {
		t.Fatal("oversized record enlarged retained scratch")
	}
	compareDiskDecoder(t, &decoder, []byte(`{"c":[{},null],"ol":[null,{}]}`))
}

func TestDiskRecordDecoderCopiedMetadataSurvivesReuse(t *testing.T) {
	data := []byte(`{"c":[{"i":"old-id","n":"old-name","m":"old-image","cp":12.5}]}`)
	var decoder diskRecordDecoder
	first, err := decoder.decode(data)
	if err != nil {
		t.Fatal(err)
	}
	retained := first.Containers[0]
	for i := range data {
		data[i] = 'x'
	}
	compareDiskDecoder(t, &decoder, []byte(`{"c":[{"i":"new-id","n":"new-name","cp":99}]}`))
	if retained.ID != "old-id" || retained.Name != "old-name" || retained.Image != "old-image" || retained.CPUPercent != 12.5 {
		t.Fatal("copied series metadata aliases decoder/scanner scratch")
	}
}

func FuzzDiskRecordDecoderReuse(f *testing.F) {
	seeds := diskDecoderSeeds()
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) >= maxHistoryLineBytes {
			t.Skip()
		}
		var decoder diskRecordDecoder
		compareDiskDecoder(t, &decoder, seeds[0])
		compareDiskDecoder(t, &decoder, data)
		compareDiskDecoder(t, &decoder, []byte(`{"c":[null,{}],"ol":[{},null]}`))
		compareDiskDecoder(t, &decoder, seeds[0])
	})
}
