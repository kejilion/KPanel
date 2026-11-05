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
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			if strings.Contains(line, "\\") || sensitiveSourceFields(line) {
				return nil
			}
			continue
		}
		if strings.HasSuffix(line, "{") {
			header := strings.TrimSpace(strings.TrimSuffix(line, "{"))
			if header == "" || strings.ContainsAny(header, "{}\"'[]=") || !sourceHeaderStart(header[0]) || unsafeSourceEscapes(header) || sensitiveSourceFields(header) {
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
		if sensitiveSourceFields(line) || strings.Contains(line, "\\") || strings.Count(line, "{") != strings.Count(line, "}") || strings.ContainsAny(line[:1], "\"'[{\\") || !(strings.HasSuffix(line, ";") || strings.HasSuffix(line, ")") || strings.HasSuffix(line, "}")) {
			return nil
		}
	}
	if depth != 0 || len(headers) == 0 {
		return nil
	}
	return headers
}

func sensitiveSourceFields(value string) bool {
	line := strings.ToLower(strings.TrimSpace(value))
	allowServerTokens := line == "server_tokens on;" || line == "server_tokens off;" || line == "server_tokens build;"
	count := 0
	for len(value) > 0 {
		value = strings.TrimLeft(value, ":= \t")
		if value == "" {
			break
		}
		count++
		if count > maxProjectionEntries {
			return true
		}
		end := strings.IndexAny(value, ":= \t")
		if end < 0 {
			end = len(value)
		}
		name := normalizedJSONKey(value[:end])
		// This Nginx display directive is a boolean/version setting, not a
		// credential. All its other fields are still inspected independently.
		if !(allowServerTokens && name == "servertokens") && secretNormalizedJSONKey(name) {
			return true
		}
		value = value[end:]
	}
	return false
}

// Regex-style escapes of punctuation are common in source block headers. An
// escaped letter or digit could conceal a credential name and stays strict.
func unsafeSourceEscapes(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			continue
		}
		i++
		if i == len(value) || !strings.ContainsRune("./$^+*?()|-", rune(value[i])) {
			return true
		}
	}
	return false
}

func sourceHeaderStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '$'
}
