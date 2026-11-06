package redact

import (
	"errors"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

func decodeYAMLQuotedKey(raw string) (string, bool) {
	if len(raw) > 64<<10 {
		return "", false
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil || len(document.Content) != 1 {
		return "", false
	}
	scalar := document.Content[0]
	if scalar.Kind != yaml.ScalarNode || scalar.Tag != "!!str" {
		return "", false
	}
	return scalar.Value, true
}

func (p *projection) markPossibleKubernetesYAMLRecords() {
	channels := make(map[int]*strings.Builder)
	for _, line := range p.lines {
		builder := channels[line.channel]
		if builder == nil {
			builder = &strings.Builder{}
			channels[line.channel] = builder
		}
		builder.WriteString(line.value)
		builder.WriteByte('\n')
	}
	for channel, value := range channels {
		if kubernetesYAMLSecretKind.MatchString(value.String()) {
			p.yamlSecretChannels[channel] = true
		}
	}
}

// Only release lines when a small, unambiguous YAML document proves their
// boundaries. Unsupported encodings fall back to masking the whole channel.
func (p *projection) redactKubernetesYAML() {
	groups := make(map[int][]int)
	for i, line := range p.lines {
		for channel := line.channel; channel >= 0; channel = p.parents[channel] {
			if p.yamlSecretChannels[channel] {
				groups[channel] = append(groups[channel], i)
			}
		}
	}
	for channel, indexes := range groups {
		values := make([]string, 0, len(indexes))
		unambiguous := true
		for _, index := range indexes {
			line := p.lines[index]
			if line.channel != channel || p.sources[line.source].fragment {
				unambiguous = false
			}
			values = append(values, line.value)
		}
		mask, ok := yamlSecretLineMask(values)
		if !unambiguous || !ok {
			for _, index := range indexes {
				p.safe[index] = "[REDACTED KUBERNETES SECRET]"
			}
			continue
		}
		for i, index := range indexes {
			if mask[i] {
				p.safe[index] = "[REDACTED KUBERNETES SECRET]"
			}
		}
	}
}

func yamlSecretLineMask(lines []string) ([]bool, bool) {
	if len(lines) == 0 || len(lines) > 4096 {
		return nil, false
	}
	size, sequenceMarkers := 0, 0
	for _, line := range lines {
		size += len(line) + 1
		trimmed := strings.TrimLeft(line, " ")
		if size > 128<<10 || len(line)-len(trimmed) > 128 || strings.ContainsAny(line, "[]{}&*") || strings.HasPrefix(trimmed, "? ") {
			return nil, false
		}
		for strings.HasPrefix(trimmed, "- ") {
			sequenceMarkers++
			if sequenceMarkers > 256 {
				return nil, false
			}
			trimmed = strings.TrimLeft(trimmed[1:], " ")
		}
	}
	decoder := yaml.NewDecoder(strings.NewReader(strings.Join(lines, "\n")))
	documents := []*yaml.Node{}
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(documents) >= 256 {
			return nil, false
		}
		documents = append(documents, &document)
	}
	if len(documents) == 0 {
		return nil, false
	}
	mask := make([]bool, len(lines))
	for i, document := range documents {
		end := len(lines) + 1
		if i+1 < len(documents) {
			end = documents[i+1].Line
		}
		if !maskYAMLNode(document, end, mask, 0) {
			return nil, false
		}
	}
	return mask, true
}

func maskYAMLNode(node *yaml.Node, end int, mask []bool, depth int) bool {
	if depth > 128 || node == nil || node.Anchor != "" || node.Kind == yaml.AliasNode || node.Line < 1 || node.Line > len(mask) || end < node.Line || end > len(mask)+1 {
		return false
	}
	if node.Kind == yaml.ScalarNode && kubernetesYAMLSecretKind.MatchString(node.Value) {
		return false
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return false
		}
		secret, secretList := false, false
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Anchor != "" {
				return false
			}
			if normalizedJSONKey(key.Value) == "kind" {
				if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Anchor != "" {
					return false
				}
				kind := strings.TrimSpace(value.Value)
				secret = secret || strings.EqualFold(kind, "secret")
				secretList = secretList || strings.EqualFold(kind, "secretlist")
			}
		}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			next := end
			if i+2 < len(node.Content) {
				next = node.Content[i+2].Line
			}
			if next <= key.Line {
				return false
			}
			name := normalizedJSONKey(key.Value)
			if secret && (name == "data" || name == "stringdata") || secretList && name == "items" {
				for line := key.Line; line < next; line++ {
					mask[line-1] = true
				}
				continue
			}
			if !maskYAMLNode(value, next, mask, depth+1) {
				return false
			}
		}
		return true
	}
	for i, child := range node.Content {
		next := end
		if i+1 < len(node.Content) {
			next = node.Content[i+1].Line
		}
		if !maskYAMLNode(child, next, mask, depth+1) {
			return false
		}
	}
	return true
}
