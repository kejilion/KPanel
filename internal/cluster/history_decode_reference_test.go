package cluster

import (
	"encoding/json"
	"strings"
)

// Original token-based preflight, retained only as a differential fuzz oracle.
func scanHistoryJSONReference(decoder *json.Decoder, depth, arrayLimit int) error {
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
				if err := scanHistoryJSONReference(decoder, depth+1, 720); err != nil {
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
				if err := scanHistoryJSONReference(decoder, depth+1, limit); err != nil {
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
