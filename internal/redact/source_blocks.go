package redact

import "strings"

// sourceBlockHeaders recognizes a complete, conservative subset of block
// structured text without relying on a path, extension or tool identity.
// Ambiguous or incomplete input continues through the strict JSON projection.
// Only headers are returned: every other line is still projected normally.
func sourceBlockHeaders(lines []string) map[int]bool {
	headers := make(map[int]bool)
	depth := 0
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasSuffix(line, "{") {
			header := strings.TrimSpace(strings.TrimSuffix(line, "{"))
			if header == "" || strings.ContainsAny(header, "{}\"'\\[]=") || !sourceHeaderStart(header[0]) {
				return nil
			}
			depth++
			headers[i] = true
			continue
		}
		if line == "}" || line == "};" {
			depth--
			if depth < 0 {
				return nil
			}
			continue
		}
		fields := strings.FieldsFunc(line, func(r rune) bool { return r == ':' || r == '=' || r == ' ' || r == '\t' })
		if len(fields) > 0 && (strings.EqualFold(fields[0], "key") || strings.EqualFold(fields[0], "auth")) {
			return nil
		}
		if depth == 0 || strings.ContainsAny(line, "{}\\") || strings.ContainsAny(line[:1], "\"'[{\\") || !(strings.HasSuffix(line, ";") || strings.HasSuffix(line, ")")) {
			return nil
		}
	}
	if depth != 0 || len(headers) == 0 {
		return nil
	}
	return headers
}

func sourceHeaderStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '$'
}
