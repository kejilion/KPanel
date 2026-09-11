package cluster

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

// Check container/point cardinality before allocating typed metric structs.
// A byte limit alone would allow millions of tiny objects to expand in memory.
func decodeHistoryResponse(reader io.Reader, query monitoring.Query) (contract.MonitoringHistory, error) {
	var result contract.MonitoringHistory
	content, err := io.ReadAll(io.LimitReader(reader, monitoring.MaxHistoryResponseBytes+1))
	if err != nil || int64(len(content)) > monitoring.MaxHistoryResponseBytes {
		return result, ErrHistoryUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := scanHistoryJSON(decoder, 0, 720); err != nil {
		return result, ErrHistoryUnavailable
	}
	if _, err := decoder.Token(); err != io.EOF {
		return result, ErrHistoryUnavailable
	}
	if json.Unmarshal(content, &result) != nil || !validHistoryResponse(result, query) {
		return contract.MonitoringHistory{}, ErrHistoryUnavailable
	}
	return result, nil
}

func scanHistoryJSON(decoder *json.Decoder, depth, arrayLimit int) error {
	if depth > 8 {
		return ErrHistoryUnavailable
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch value := token.(type) {
	case string:
		if len(value) > 4096 {
			return ErrHistoryUnavailable
		}
	case json.Delim:
		switch value {
		case '[':
			count := 0
			for decoder.More() {
				count++
				if count > arrayLimit {
					return ErrHistoryUnavailable
				}
				if err := scanHistoryJSON(decoder, depth+1, 720); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return ErrHistoryUnavailable
			}
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				// The contract uses ASCII keys. encoding/json also folds Unicode
				// lookalikes (e.g. long s), which must not bypass cardinality checks.
				for _, character := range key {
					if character > 127 {
						return ErrHistoryUnavailable
					}
				}
				key = strings.ToLower(key)
				if !ok || len(key) > 128 || len(seen) >= 80 || seen[key] {
					return ErrHistoryUnavailable
				}
				seen[key] = true
				limit := 720
				if depth == 0 && key == "containers" {
					limit = 32
				}
				if depth == 0 && key == "operatorlatency" {
					limit = 9
				}
				if err := scanHistoryJSON(decoder, depth+1, limit); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return ErrHistoryUnavailable
			}
		default:
			return ErrHistoryUnavailable
		}
	}
	return nil
}
