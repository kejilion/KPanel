package monitoring

import (
	"net/url"
	"time"
)

// Query is the fixed, read-only history request shared by all node types.
type Query struct {
	Range string    `json:"range"`
	Start time.Time `json:"start,omitempty"`
	End   time.Time `json:"end,omitempty"`
	Gzip  bool      `json:"gzip,omitempty"`
}

const MaxHistoryResponseBytes int64 = 16 << 20

func ParseQuery(raw string) (Query, error) {
	if len(raw) > 512 {
		return Query{}, ErrInvalidWindow
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return Query{}, ErrInvalidWindow
	}
	for key, items := range values {
		if len(items) != 1 || (key != "range" && key != "start" && key != "end" && key != "gzip") {
			return Query{}, ErrInvalidWindow
		}
	}
	q := Query{Range: values.Get("range")}
	if values.Has("gzip") {
		if values.Get("gzip") != "1" {
			return Query{}, ErrInvalidWindow
		}
		q.Gzip = true
	}
	if values.Has("start") || values.Has("end") {
		q.Start, err = time.Parse(time.RFC3339Nano, values.Get("start"))
		if err != nil {
			return Query{}, ErrInvalidWindow
		}
		q.End, err = time.Parse(time.RFC3339Nano, values.Get("end"))
		if err != nil {
			return Query{}, ErrInvalidWindow
		}
	}
	return q, q.Validate()
}

func (q Query) Validate() error {
	spec, err := parseRange(q.Range)
	if err != nil {
		return err
	}
	if q.Start.IsZero() && q.End.IsZero() {
		return nil
	}
	if q.Start.IsZero() || q.End.IsZero() || !q.Start.Before(q.End) || q.End.Sub(q.Start) > spec.duration {
		return ErrInvalidWindow
	}
	return nil
}

func (q Query) Encode() string {
	values := url.Values{"range": {q.Range}}
	if q.Gzip {
		values.Set("gzip", "1")
	}
	if !q.Start.IsZero() {
		values.Set("start", q.Start.UTC().Format(time.RFC3339Nano))
		values.Set("end", q.End.UTC().Format(time.RFC3339Nano))
	}
	return values.Encode()
}
