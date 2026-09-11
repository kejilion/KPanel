package cluster

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

// Check container/point cardinality before allocating typed metric structs.
// A byte limit alone would allow millions of tiny objects to expand in memory.
func decodeHistoryResponse(reader io.Reader, query monitoring.Query) (contract.MonitoringHistory, error) {
	return decodeHistoryPayload(reader, query, false)
}

func decodeHistoryPayload(reader io.Reader, query monitoring.Query, compressed bool) (contract.MonitoringHistory, error) {
	var result contract.MonitoringHistory
	content, err := monitoring.ReadHistoryPayload(reader, compressed)
	if err != nil {
		return result, ErrHistoryUnavailable
	}
	if err := scanHistoryJSON(content); err != nil {
		return result, ErrHistoryUnavailable
	}
	if json.Unmarshal(content, &result) != nil || !validHistoryResponse(result, query) {
		return contract.MonitoringHistory{}, ErrHistoryUnavailable
	}
	return result, nil
}

// Let encoding/json validate syntax first. This second, bounded walk checks
// shape without materializing millions of interface tokens and object maps.
// Ordinary keys/strings borrow the input; only escaped strings need decoding.
func scanHistoryJSON(content []byte) error {
	if !json.Valid(content) {
		return ErrHistoryUnavailable
	}
	scanner := historyJSONScanner{content: content}
	return scanner.value(0, 720)
}

type historyJSONScanner struct {
	content []byte
	offset  int
}

func (s *historyJSONScanner) space() {
	for s.offset < len(s.content) {
		switch s.content[s.offset] {
		case ' ', '\t', '\n', '\r':
			s.offset++
		default:
			return
		}
	}
}

// Called only on syntax-validated JSON at an opening quote.
func (s *historyJSONScanner) stringValue() ([]byte, error) {
	start := s.offset
	s.offset++
	escaped := false
	for s.content[s.offset] != '"' {
		if s.content[s.offset] == '\\' {
			escaped = true
			s.offset++
		}
		s.offset++
	}
	s.offset++
	value := s.content[start+1 : s.offset-1]
	// A JSON escape uses at most six source bytes per decoded byte.
	if len(value) > 6*4096 {
		return nil, ErrHistoryUnavailable
	}
	if escaped || !utf8.Valid(value) {
		var decoded string
		if err := json.Unmarshal(s.content[start:s.offset], &decoded); err != nil {
			return nil, err
		}
		value = []byte(decoded)
	}
	if len(value) > 4096 {
		return nil, ErrHistoryUnavailable
	}
	return value, nil
}

func (s *historyJSONScanner) value(depth, arrayLimit int) error {
	if depth > 8 {
		return ErrHistoryUnavailable
	}
	s.space()
	switch s.content[s.offset] {
	case '"':
		_, err := s.stringValue()
		return err
	case '[':
		s.offset++
		s.space()
		for count := 0; s.content[s.offset] != ']'; count++ {
			if count >= arrayLimit {
				return ErrHistoryUnavailable
			}
			if err := s.value(depth+1, 720); err != nil {
				return err
			}
			s.space()
			if s.content[s.offset] == ',' {
				s.offset++
				s.space()
			}
		}
		s.offset++
	case '{':
		s.offset++
		s.space()
		var seen [80][]byte
		for count := 0; s.content[s.offset] != '}'; count++ {
			key, err := s.stringValue()
			if err != nil || len(key) > 128 || count >= len(seen) {
				return ErrHistoryUnavailable
			}
			// Reject Unicode lookalikes before case-folded cardinality checks.
			for _, character := range key {
				if character > 127 {
					return ErrHistoryUnavailable
				}
			}
			for _, previous := range seen[:count] {
				if bytes.EqualFold(key, previous) {
					return ErrHistoryUnavailable
				}
			}
			seen[count] = key
			limit := 720
			if depth == 0 && bytes.EqualFold(key, []byte("containers")) {
				limit = 32
			}
			if depth == 0 && bytes.EqualFold(key, []byte("operatorLatency")) {
				limit = 9
			}
			s.space()
			s.offset++ // colon, guaranteed by json.Valid
			if err := s.value(depth+1, limit); err != nil {
				return err
			}
			s.space()
			if s.content[s.offset] == ',' {
				s.offset++
				s.space()
			}
		}
		s.offset++
	default:
		// Number, true, false or null; syntax is already validated.
		start := s.offset
		for s.offset < len(s.content) {
			switch s.content[s.offset] {
			case ',', '}', ']', ' ', '\t', '\n', '\r':
				goto primitiveEnd
			}
			s.offset++
		}
	primitiveEnd:
		if s.content[start] == '-' || (s.content[start] >= '0' && s.content[start] <= '9') {
			// Match Decoder.Token's finite float requirement, including unknown
			// fields that the subsequent typed unmarshal would otherwise skip.
			if _, err := strconv.ParseFloat(string(s.content[start:s.offset]), 64); err != nil {
				return ErrHistoryUnavailable
			}
		}
	}
	return nil
}
