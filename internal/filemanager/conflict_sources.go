package filemanager

import "github.com/kejilion/kejilion-panel/internal/contract"

func deduplicateConflictSources(input *contract.FileActionRequest) {
	if input.ConflictPolicies == nil || (input.Action != "copy" && input.Action != "move") {
		return
	}
	result := make([]string, 0, len(input.Sources))
	seen := make(map[string]bool)
	for _, source := range input.Sources {
		key := platformPathKey(source)
		if seen[key] {
			continue
		}
		seen[key] = true
		covered := false
		for _, parent := range input.Sources {
			if platformPathKey(parent) != key && isWithin(key, platformPathKey(parent)) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, source)
		}
	}
	input.Sources = result
}
