package selfupdate

import (
	"strconv"
	"strings"
)

type parsedVersion struct {
	core [3]uint64
	rc   *uint64
}

func normalizeImageDigest(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return ""
	}
	for _, character := range strings.TrimPrefix(value, "sha256:") {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return ""
		}
	}
	return value
}

func normalizeCurrentVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "-dev")
	return normalizeReleaseVersion(value)
}

func normalizeStableVersion(value string) string {
	normalized, parsed, ok := parseReleaseVersion(value)
	if !ok || parsed.rc != nil {
		return ""
	}
	return normalized
}

func normalizePreviewVersion(value string) string {
	normalized, parsed, ok := parseReleaseVersion(value)
	if !ok || parsed.rc == nil {
		return ""
	}
	return normalized
}

func normalizeReleaseVersion(value string) string {
	normalized, _, ok := parseReleaseVersion(value)
	if !ok {
		return ""
	}
	return normalized
}

func parseReleaseVersion(value string) (string, parsedVersion, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	coreValue := value
	var releaseCandidate *uint64
	if separator := strings.IndexByte(value, '-'); separator >= 0 {
		coreValue = value[:separator]
		suffix := value[separator+1:]
		if !strings.HasPrefix(suffix, "rc.") {
			return "", parsedVersion{}, false
		}
		numberValue := strings.TrimPrefix(suffix, "rc.")
		number, ok := parseVersionNumber(numberValue, false)
		if !ok || number == 0 {
			return "", parsedVersion{}, false
		}
		releaseCandidate = &number
	}
	parts := strings.Split(coreValue, ".")
	if len(parts) != 3 {
		return "", parsedVersion{}, false
	}
	parsed := parsedVersion{rc: releaseCandidate}
	for index, part := range parts {
		number, ok := parseVersionNumber(part, true)
		if !ok {
			return "", parsedVersion{}, false
		}
		parsed.core[index] = number
	}
	normalized := strings.Join(parts, ".")
	if releaseCandidate != nil {
		normalized += "-rc." + strconv.FormatUint(*releaseCandidate, 10)
	}
	return normalized, parsed, true
}

func parseVersionNumber(value string, allowZero bool) (uint64, bool) {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	number, err := strconv.ParseUint(value, 10, 31)
	if err != nil || number > 999999 || (!allowZero && number == 0) {
		return 0, false
	}
	return number, true
}

func compareVersions(left, right string) int {
	_, leftVersion, leftOK := parseReleaseVersion(left)
	_, rightVersion, rightOK := parseReleaseVersion(right)
	if !leftOK && !rightOK {
		return 0
	}
	if !leftOK {
		return -1
	}
	if !rightOK {
		return 1
	}
	for index := range leftVersion.core {
		if leftVersion.core[index] < rightVersion.core[index] {
			return -1
		}
		if leftVersion.core[index] > rightVersion.core[index] {
			return 1
		}
	}
	if leftVersion.rc == nil && rightVersion.rc != nil {
		return 1
	}
	if leftVersion.rc != nil && rightVersion.rc == nil {
		return -1
	}
	if leftVersion.rc != nil && rightVersion.rc != nil {
		if *leftVersion.rc < *rightVersion.rc {
			return -1
		}
		if *leftVersion.rc > *rightVersion.rc {
			return 1
		}
	}
	return 0
}
