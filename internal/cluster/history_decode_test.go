package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

func historyPreflightSeeds() [][]byte {
	seeds := [][]byte{
		[]byte(`{}`), []byte(`null`), []byte(`1e999`), []byte(`{} {}`),
		[]byte(`{"a":0,"A":1}`), []byte(`{"a":0,"\u0061":1}`),
		[]byte(`{"a":"\"\\\uD800","b":[true,false,null,-1.2e3]}`),
		[]byte(`{"container\u017f":[]}`), []byte(`{"a":"` + strings.Repeat("x", 4097) + `"}`),
		[]byte(`{"a":"` + strings.Repeat(`\u0061`, 4096) + `"}`),
		[]byte(`{"a":"` + strings.Repeat(`\u0061`, 4097) + `"}`),
		[]byte(`{"a":"` + string([]byte{0xff, 0xc0}) + `"}`),
	}
	for _, key := range []string{"host", "containers", "CONTAINERS", `\u0063ontainers`, "operatorLatency"} {
		for _, count := range []int{9, 10, 32, 33, 720, 721} {
			seeds = append(seeds, []byte(`{"`+key+`":[`+strings.Repeat(`{},`, count-1)+`{}]}`))
		}
	}
	for _, count := range []int{8, 9, 80, 81} {
		seeds = append(seeds, []byte(strings.Repeat("[", count)+"0"+strings.Repeat("]", count)))
		var fields []string
		for i := range count {
			fields = append(fields, fmt.Sprintf(`"key%d":%d`, i, i))
		}
		seeds = append(seeds, []byte("{"+strings.Join(fields, ",")+"}"))
	}
	return seeds
}

func compareHistoryPreflight(t testing.TB, data []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	want := scanHistoryJSONReference(decoder, 0, 720) == nil
	if _, err := decoder.Token(); err != io.EOF {
		want = false
	}
	if got := scanHistoryJSON(data) == nil; got != want {
		t.Fatalf("preflight mismatch: got=%v want=%v data=%.200q", got, want, data)
	}
}

func TestClusterHistoryPreflightCompatibility(t *testing.T) {
	for _, data := range historyPreflightSeeds() {
		compareHistoryPreflight(t, data)
	}
	compareHistoryPreflight(t, historyPerformancePayload(t))
}

func FuzzHistoryPreflight(f *testing.F) {
	for _, data := range historyPreflightSeeds() {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		compareHistoryPreflight(t, data)
	})
}
