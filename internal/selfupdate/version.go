package selfupdate

import (
	"strconv"
	"strings"
)

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
	return normalizeStableVersion(value)
}

func normalizeStableVersion(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return ""
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return ""
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return ""
			}
		}
		if number, err := strconv.ParseUint(part, 10, 31); err != nil || number > 999999 {
			return ""
		}
	}
	return strings.Join(parts, ".")
}

func compareVersions(left, right string) int {
	left = normalizeStableVersion(left)
	right = normalizeStableVersion(right)
	if left == "" && right == "" {
		return 0
	}
	if left == "" {
		return -1
	}
	if right == "" {
		return 1
	}
	l := strings.Split(left, ".")
	r := strings.Split(right, ".")
	for index := range l {
		ln, _ := strconv.Atoi(l[index])
		rn, _ := strconv.Atoi(r[index])
		if ln < rn {
			return -1
		}
		if ln > rn {
			return 1
		}
	}
	return 0
}
