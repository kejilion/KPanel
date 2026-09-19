package redact

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const privateKeyReplacement = "[REDACTED PRIVATE KEY]"

var (
	jsonSecretAssignment = regexp.MustCompile(`(?i)("(?:[^"\\]|\\.)*(?:password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|authorization|cookie|credential|private[_-]?key)(?:[^"\\]|\\.)*"\s*:\s*)("(?:\\.|[^"\\])*"|[^,\s}\]]+)`)
	secretAssignment     = regexp.MustCompile(`(?i)(^|[^?&A-Za-z0-9_.-])([A-Za-z0-9_.-]*(?:password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|authorization|cookie|credential|private[_-]?key)[A-Za-z0-9_.-]*)(\s*[:=]\s*)("(?:\\.|[^"\\])*"|'[^']*'|[^\s]+)`)
	secretFlag           = regexp.MustCompile(`(?i)(--?[A-Za-z0-9_.-]*(?:password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|authorization|cookie|credential|private[_-]?key)[A-Za-z0-9_.-]*\s+)("(?:\\.|[^"\\])*"|'[^']*'|[^\s]+)`)
	authorizationHeader  = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization)(\s*[:=]\s*)("(?:\\.|[^"\\])*"|'[^']*'|[^\r\n]+)`)
	authorizationFlag    = regexp.MustCompile(`(?i)(--?(?:proxy-)?authorization\s+)("(?:\\.|[^"\\])*"|'[^']*'|[^\r\n]+)`)
	cookieHeader         = regexp.MustCompile(`(?i)\b(cookie|set-cookie)(\s*:\s*)[^\r\n]+`)
	bearerSecret         = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	basicSecret          = regexp.MustCompile(`(?i)\bBasic\s+[A-Za-z0-9_+/=-]{8,}`)
	openAISecret         = regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9._~+/=-]{4,}`)
	urlCredentials       = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s@]+@`)
	urlSensitiveQuery    = regexp.MustCompile(`(?i)([?&](?:access[_-]?token|refresh[_-]?token|id[_-]?token|token|api[_-]?key|key|secret|signature|sig|password|passwd|pwd|credential|x-amz-(?:signature|credential|security-token))=)[^&#\s]*`)
	privateKeyBegin      = regexp.MustCompile(`(?i)-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----`)
	privateKeyEnd        = regexp.MustCompile(`(?i)-----END (?:[A-Z0-9]+ )*PRIVATE KEY-----`)
)

func Text(value string) string {
	result := Records([]string{value})
	if len(result) == 0 {
		return ""
	}
	return result[0]
}

// Records redacts chronological records while preserving their positions. If
// a bounded window begins inside a private-key block, an otherwise orphaned
// END marker causes the uncertain prefix to be hidden conservatively.
func Records(values []string) []string {
	result, _ := redactRecords(values)
	return result
}

func Lines(data []byte, limit int, maxLineBytes int) []string {
	rawLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	redactedLines, suppressed := redactRecords(rawLines)
	result := make([]string, 0, len(rawLines))
	for index, redacted := range redactedLines {
		if suppressed[index] {
			continue
		}
		if maxLineBytes > 0 && len(redacted) > maxLineBytes {
			redacted = truncateUTF8(redacted, maxLineBytes) + "…"
		}
		result = append(result, redacted)
	}
	if limit >= 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

func redactRecords(values []string) ([]string, []bool) {
	type physicalLine struct {
		record int
		value  string
	}
	lines := make([]physicalLine, 0, len(values))
	for record, value := range values {
		value = strings.ToValidUTF8(value, "�")
		value = strings.ReplaceAll(value, "\r\n", "\n")
		value = strings.ReplaceAll(value, "\r", "\n")
		for _, line := range strings.Split(value, "\n") {
			lines = append(lines, physicalLine{record: record, value: line})
		}
	}

	lineValues := make([]string, len(lines))
	for index := range lines {
		lineValues[index] = lines[index].value
	}
	redactedLines, suppressedLines := redactPhysicalLines(lineValues)
	recordLines := make([][]string, len(values))
	visible := make([]bool, len(values))
	for index, line := range lines {
		if suppressedLines[index] {
			continue
		}
		recordLines[line.record] = append(recordLines[line.record], redactedLines[index])
		visible[line.record] = true
	}
	result := make([]string, len(values))
	suppressed := make([]bool, len(values))
	for index := range values {
		result[index] = strings.Join(recordLines[index], "\n")
		suppressed[index] = !visible[index]
	}
	return result, suppressed
}

func redactPhysicalLines(values []string) ([]string, []bool) {
	result := make([]string, len(values))
	suppressed := make([]bool, len(values))
	inPrivateKey := false
	uncertainStart := 0
	for index, value := range values {
		firstIsEnd, lastIsBegin, hasMarker := privateKeyMarkerOrder(value)
		if inPrivateKey {
			suppressed[index] = true
			if hasMarker {
				inPrivateKey = lastIsBegin
				if !inPrivateKey {
					uncertainStart = index + 1
				}
			}
			continue
		}
		if firstIsEnd {
			result[uncertainStart] = privateKeyReplacement
			for uncertainIndex := uncertainStart + 1; uncertainIndex <= index; uncertainIndex++ {
				result[uncertainIndex] = ""
				suppressed[uncertainIndex] = true
			}
			inPrivateKey = lastIsBegin
			if !inPrivateKey {
				uncertainStart = index + 1
			}
			continue
		}
		if hasMarker {
			result[index] = privateKeyReplacement
			inPrivateKey = lastIsBegin
			if !inPrivateKey {
				uncertainStart = index + 1
			}
			continue
		}
		result[index] = redactLine(value)
	}
	return result, suppressed
}

func privateKeyMarkerOrder(value string) (bool, bool, bool) {
	begins := privateKeyBegin.FindAllStringIndex(value, -1)
	ends := privateKeyEnd.FindAllStringIndex(value, -1)
	if len(begins) == 0 && len(ends) == 0 {
		return false, false, false
	}
	firstIsEnd := len(ends) > 0 && (len(begins) == 0 || ends[0][0] < begins[0][0])
	lastIsBegin := len(begins) > 0 && (len(ends) == 0 || begins[len(begins)-1][0] > ends[len(ends)-1][0])
	return firstIsEnd, lastIsBegin, true
}

func redactLine(value string) string {
	value = authorizationHeader.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = authorizationFlag.ReplaceAllString(value, "${1}[REDACTED]")
	value = cookieHeader.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = urlSensitiveQuery.ReplaceAllString(value, "${1}[REDACTED]")
	value = jsonSecretAssignment.ReplaceAllString(value, `${1}"[REDACTED]"`)
	value = secretAssignment.ReplaceAllString(value, "${1}${2}${3}[REDACTED]")
	value = secretFlag.ReplaceAllString(value, "${1}[REDACTED]")
	value = bearerSecret.ReplaceAllString(value, "Bearer [REDACTED]")
	value = basicSecret.ReplaceAllString(value, "Basic [REDACTED]")
	value = openAISecret.ReplaceAllString(value, "[REDACTED]")
	value = urlCredentials.ReplaceAllString(value, "${1}[REDACTED]@")
	return strings.Map(func(character rune) rune {
		if character == '\t' || !unicode.IsControl(character) {
			return character
		}
		return -1
	}, value)
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}
