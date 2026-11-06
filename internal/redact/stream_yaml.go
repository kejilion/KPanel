package redact

import (
	"regexp"
	"strings"
)

var (
	streamYAMLDataStart  = regexp.MustCompile(`(?i)^([ ]*(?:-[ ]+)*)(?:data|stringdata|"data"|"stringdata"|'data'|'stringdata')[ ]*:[ ]*(.*)$`)
	streamYAMLHeader     = regexp.MustCompile(`(?i)^[ ]*(?:apiVersion|kind|metadata|name|type|immutable)[ ]*:[ ]*(?:[A-Za-z0-9_.-]+)?[ ]*(?:#.*)?$`)
	streamYAMLSecretKind = regexp.MustCompile(`(?i)^[ ]*kind[ ]*:[ ]*(?:secret|secretlist)[ ]*(?:#.*)?$`)
)

// streamYAMLState conservatively masks an entire simple YAML data subtree.
// Unknown syntax is rejected rather than guessing whether a scalar is closed.
type streamYAMLState struct {
	active         bool
	indent         int
	quote          byte
	flow           []byte
	secretDocument bool
}

// consume returns true when the entire record must be masked. It may leave a
// data subtree only at a top-level recognized metadata field or document
// separator with no outstanding quote or flow container.
func (s *streamYAMLState) consume(line string) (bool, bool, error) {
	// Format characters are ignored only in the classifier's view. The source
	// record itself is never rewritten or returned while this state masks it.
	body := removeFormatCharacters(strings.TrimSuffix(line, "\n"))
	if !s.active {
		if strings.TrimSpace(body) == "---" && s.secretDocument {
			s.secretDocument = false
			return true, false, nil
		}
		if streamYAMLSecretKind.MatchString(body) {
			s.secretDocument = true
			return true, true, nil
		}
		match := streamYAMLDataStart.FindStringSubmatch(body)
		if match != nil {
			s.active, s.indent = true, len(match[1])
			if err := s.observe(match[2]); err != nil {
				return false, false, err
			}
			return true, true, nil
		}
		if s.secretDocument && streamYAMLHeader.MatchString(body) {
			return true, false, nil
		}
		if s.secretDocument {
			return false, false, ErrStreamAmbiguous
		}
		return false, false, nil
	}
	trimmed := strings.TrimLeft(body, " ")
	indent := len(body) - len(trimmed)
	if s.quote == 0 && len(s.flow) == 0 && indent <= s.indent && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "- ") {
		if trimmed == "---" {
			s.active = false
			s.secretDocument = false
			return true, false, nil
		}
		if streamYAMLHeader.MatchString(body) {
			s.active = false
			if streamYAMLSecretKind.MatchString(body) {
				s.secretDocument = true
				return true, true, nil
			}
			return true, false, nil
		}
		return false, false, ErrStreamAmbiguous
	}
	if err := s.observe(body); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func (s *streamYAMLState) observe(value string) error {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if s.quote == '"' {
			if b == '\\' {
				if i+1 < len(value) {
					i++
				}
				continue
			}
			if b == '"' {
				s.quote = 0
			}
			continue
		}
		if s.quote == '\'' {
			if b == '\'' {
				if i+1 < len(value) && value[i+1] == '\'' {
					i++
					continue
				}
				s.quote = 0
			}
			continue
		}
		if b == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
			break
		}
		if (b == '\'' || b == '"') && (i == 0 || strings.ContainsRune(" \t:-,[{?", rune(value[i-1]))) {
			s.quote = b
			continue
		}
		switch b {
		case '[', '{':
			if len(s.flow) >= 32 {
				return ErrStreamLimit
			}
			s.flow = append(s.flow, b)
		case ']', '}':
			if len(s.flow) == 0 {
				return ErrStreamAmbiguous
			}
			want := byte('[')
			if b == '}' {
				want = '{'
			}
			if s.flow[len(s.flow)-1] != want {
				return ErrStreamAmbiguous
			}
			s.flow = s.flow[:len(s.flow)-1]
		}
	}
	return nil
}
