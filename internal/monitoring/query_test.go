package monitoring

import "testing"

func TestHistoryQuerySharesAllLocalRangesAndRejectsMalformedInput(t *testing.T) {
	for _, value := range []string{"", "1h", "6h", "24h", "7d", "30d", "3m", "6m", "12m"} {
		query, err := ParseQuery("range=" + value)
		if err != nil {
			t.Fatal(value, err)
		}
		decoded, err := ParseQuery(query.Encode())
		if err != nil || decoded != query {
			t.Fatal("query roundtrip failed")
		}
	}
	for _, value := range []string{"range=1h&range=7d", "range=%GG", "range=13m", "range=1h;command=ls", "command=ls", "start=", "start=2026-09-11T00:00:00Z", "range=1h&start=2026-09-11T00:00:00Z&end=2026-09-12T00:00:00Z", "start=2026-09-12T00:00:00Z&end=2026-09-11T00:00:00Z"} {
		if _, err := ParseQuery(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
