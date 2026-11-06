package redact

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// These are display-projection budgets, independent of execution arguments.
	// The work budget includes decoded and re-encoded strings, so repeated JSON
	// encoding cannot restart the limits or cause unbounded intermediate output.
	maxProjectionInputBytes      = 16 << 20
	maxProjectionWorkBytes       = 64 << 20
	maxProjectionEntries         = 1 << 18
	maxProjectionDepth           = 32
	projectionLimitReplacement   = "[REDACTED: projection limit]"
	projectionInvalidReplacement = "[REDACTED: invalid structured data]"
)

var (
	errInvalidJSON     = errors.New("invalid JSON data")
	errProjectionLimit = errors.New("redaction projection limit")
)

type projectionBudget struct{ bytes, entries int }

func (b *projectionBudget) consume(bytes, entries int) error {
	if bytes > maxProjectionWorkBytes-b.bytes || entries > maxProjectionEntries-b.entries {
		return errProjectionLimit
	}
	b.bytes += bytes
	b.entries += entries
	return nil
}

// JSON projects a fresh decoded tree for display only. Every entry point uses
// the same bounded normal form: decode and classify fields, normalize terminal
// text, redact ordered physical leaves once, then reconstruct the original
// containers and encoded strings. The source and execution arguments are never
// mutated. Number tokens retain their exact decimal representation.
func JSON(raw []byte) ([]byte, error) {
	if len(raw) > maxProjectionInputBytes {
		return nil, errProjectionLimit
	}
	p := newProjection()
	value, err := p.decodeJSON(raw, 0)
	if err != nil {
		return nil, err
	}
	value, err = p.jsonValue(value, 0)
	if err != nil {
		return nil, err
	}
	if err := p.redact(); err != nil {
		return nil, err
	}
	safe, _, err := p.render(value)
	if err != nil {
		return nil, err
	}
	return p.marshal(safe)
}

// A projection owns every leaf and its location. Encoded JSON and JSONL are
// expanded before the shared PEM/configuration state machine sees any lines;
// they never start another redactor with a fresh continuation state.
type projection struct {
	literals           []string
	literalForms       []string
	literalFormsReady  bool
	keyDepth           int
	budget             projectionBudget
	lines              []semanticLine
	channel            int
	indent             int
	layer              int
	source             int
	sources            []lexicalSource
	parents            []int
	channels           map[channelKey]int
	yamlSecretChannels map[int]bool
	safe               []string
	suppressed         []bool
}

type semanticLine struct {
	value    string
	identity string
	channel  int
	indent   int
	source   int
}

// A source is the lexical text at one representation layer, before decoding a
// JSON wrapper. Configuration quotes/dedent belong to this source, not to a
// descendant's decoded characters. Sources are ordered parents before children;
// hiding a source hides its descendants even when its closing quote ends state.
type lexicalSource struct {
	value   string
	channel int
	layer   int
	indent  int
	parent  int
	leaf    bool
	child   bool
	hidden  bool
	// A fragment split must not separate a sensitive lexical value/identity.
	sensitive bool
	fragment  bool
}
type channelKey struct {
	parent int
	name   string
}

type textLeaf int
type maskedValue struct{ first, last int }
type scalarLeaf struct {
	value any
	line  int
}
type encodedJSON struct {
	value  any
	source int
}
type textSequence struct{ values []any }

func newProjection(literals ...string) *projection {
	return &projection{literals: literals, channels: make(map[channelKey]int), yamlSecretChannels: make(map[int]bool), parents: []int{-1}, source: -1}
}

// A complete known credential can itself be structured JSON. Its canonical
// representation still contains secret material even after key ordering or
// HTML escaping changes. Reuse the same bounded decoder and pre-marshal budget;
// this does not infer unknown credentials or restart a projection budget.
func (p *projection) prepareLiteralForms() error {
	if p.literalFormsReady {
		return nil
	}
	p.literalFormsReady = true
	normalized := make([]string, 0, len(p.literals))
	for _, literal := range p.literals {
		if err := p.budget.consume(len(literal), 1); err != nil {
			return err
		}
		normalized = append(normalized, normalizeRecordValue(literal))
	}
	p.literals = normalized
	for _, literal := range p.literals {
		if !looksLikeJSON(strings.TrimSpace(literal)) {
			continue
		}
		if len(literal) > maxProjectionInputBytes {
			return errProjectionLimit
		}
		value, err := p.decodeJSON([]byte(literal), 0)
		if errors.Is(err, errProjectionLimit) {
			return err
		}
		if err != nil {
			continue
		}
		identity, err := p.identityJSON(value, 0)
		if err != nil {
			return err
		}
		raw, err := p.marshal(identity)
		if err != nil {
			return err
		}
		p.literalForms = append(p.literalForms, string(raw))
	}
	return nil
}

// Object fields are independent text channels. Array indices and encoded
// string layers do not change a channel, allowing chronological JSONL records
// to continue the same payload without unrelated metadata closing a scalar.
func (p *projection) fieldChannel(name string) (int, error) {
	key := channelKey{p.channel, name}
	if id, ok := p.channels[key]; ok {
		return id, nil
	}
	if err := p.budget.consume(0, 1); err != nil {
		return 0, err
	}
	id := len(p.channels) + 1
	p.channels[key] = id
	p.parents = append(p.parents, p.channel)
	return id, nil
}

// decodeJSON reads tokens rather than decoding an unrestricted tree first.
// Depth and entry limits therefore apply before container allocation. Only
// decoder-produced, acyclic trees enter jsonValue; encoded strings share the
// same depth/work budget instead of resetting it at each recursion boundary.
func (p *projection) decodeJSON(raw []byte, depth int) (any, error) {
	if err := p.budget.consume(len(raw), 0); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := p.decodeValue(decoder, depth)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errInvalidJSON
	}
	return value, nil
}

func (p *projection) decodeValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxProjectionDepth {
		return nil, errProjectionLimit
	}
	if err := p.budget.consume(0, 1); err != nil {
		return nil, err
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, errInvalidJSON
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, errInvalidJSON
			}
			name, ok := key.(string)
			if !ok {
				return nil, errInvalidJSON
			}
			item, err := p.decodeValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = item
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
			return nil, errInvalidJSON
		}
		return object, nil
	case json.Delim('['):
		array := make([]any, 0)
		for decoder.More() {
			item, err := p.decodeValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, item)
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
			return nil, errInvalidJSON
		}
		return array, nil
	default:
		if _, delimiter := token.(json.Delim); delimiter {
			return nil, errInvalidJSON
		}
		return token, nil
	}
}

func normalizedJSONKey(key string) string {
	key = terminalEscape.ReplaceAllString(key, "")
	key = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, key)
	return simpleFoldForSecrets(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
}

func secretNormalizedJSONKey(key string) bool {
	if key == "auth" || key == "auths" || key == "key" || key == "tlskey" || key == "stringdata" {
		return true
	}
	for _, part := range []string{"password", "passwd", "pwd", "token", "secret", "apikey", "accesskey", "authorization", "cookie", "credential", "privatekey", "密码", "口令", "令牌", "密钥", "私钥"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func (p *projection) jsonValue(value any, depth int) (projected any, err error) {
	if depth > maxProjectionDepth {
		return nil, errProjectionLimit
	}
	if err := p.prepareLiteralForms(); err != nil {
		return nil, err
	}
	if len(p.literalForms) > 0 {
		// Match identity before field heuristics alter the representation. Still
		// visit its original leaves so private-block state and source provenance
		// continue across adjacent records.
		identity, err := p.identityJSON(value, depth)
		if err != nil {
			return nil, err
		}
		raw, err := p.marshal(identity)
		if err != nil {
			return nil, err
		}
		for _, literal := range p.literalForms {
			if string(raw) == literal {
				first := len(p.lines)
				defer func() {
					if err == nil {
						projected = maskedValue{first, len(p.lines)}
					}
				}()
				break
			}
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return p.scalar(value, "{}")
		}
		// Kubernetes Secret data contains arbitrary user-chosen keys whose
		// values are merely base64-encoded credentials. Inspect the entire map
		// before visiting its sorted fields so field order cannot bypass masking.
		kubernetesSecret, kubernetesSecretList := false, false
		for key, item := range typed {
			if normalizedJSONKey(key) == "kind" {
				if kind, ok := item.(string); ok {
					kind = removeFormatCharacters(strings.TrimSpace(kind))
					kubernetesSecret = kubernetesSecret || strings.EqualFold(kind, "secret")
					kubernetesSecretList = kubernetesSecretList || strings.EqualFold(kind, "secretlist")
				}
			}
		}
		keys := make([]string, 0, len(typed))
		names := make(map[string]string, len(typed))
		named := false
		for key, item := range typed {
			name := normalizedJSONKey(key)
			names[key] = name
			if name == "name" {
				_, isString := item.(string)
				named = named || isString
			}
			keys = append(keys, key)
		}
		// JSON objects have no ordering; preserve the existing deterministic order.
		// Arrays, physical lines, and input records retain their original order.
		sort.Strings(keys)
		for _, key := range keys {
			channel, err := p.fieldChannel(names[key])
			if err != nil {
				return nil, err
			}
			parent := p.channel
			p.channel = channel
			first := len(p.lines)
			item, err := p.jsonValue(typed[key], depth+1)
			p.channel = parent
			if err != nil {
				return nil, err
			}
			if secretNormalizedJSONKey(names[key]) || (kubernetesSecret && names[key] == "data") || (kubernetesSecretList && names[key] == "items") || (named && names[key] == "value") {
				// Hiding a value must not discard source markers or its hidden
				// state. Otherwise a credential field makes a private record look
				// partly visible and leaves a sibling's private key name intact.
				typed[key] = maskedValue{first, len(p.lines)}
			} else {
				typed[key] = item
			}
		}
		return typed, nil
	case []any:
		if len(typed) == 0 {
			return p.scalar(value, "[]")
		}
		for index, item := range typed {
			safe, err := p.jsonValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			typed[index] = safe
		}
		return typed, nil
	case string:
		return p.text(typed, depth)
	default:
		// Numeric/boolean/null values in a structured continuation still belong
		// to the parent scalar. Give them a semantic position so they cannot
		// evade an active configuration or PEM boundary by having no text leaf.
		var text string
		switch scalar := value.(type) {
		case nil:
			text = "null"
		case bool:
			if scalar {
				text = "true"
			} else {
				text = "false"
			}
		case json.Number:
			text = string(scalar)
		default:
			return nil, errInvalidJSON
		}
		return p.scalar(value, text)
	}
}

func (p *projection) scalar(value any, text string) (any, error) {
	leaf, err := p.leaf(text)
	if err != nil {
		return nil, err
	}
	return scalarLeaf{value, int(leaf.(textLeaf))}, nil
}

func looksLikeJSON(value string) bool {
	value = strings.TrimLeftFunc(value, unicode.IsSpace)
	if len(value) == 0 {
		return false
	}
	if value[0] == '{' || value[0] == '"' {
		return true
	}
	if value[0] != '[' {
		return false
	}
	// The longest recognized log level is eight bytes. Searching the entire
	// remaining input at every '[' makes ordinary output quadratic.
	if end := strings.IndexByte(value[:min(len(value), 10)], ']'); end > 1 {
		switch strings.ToLower(value[1:end]) {
		case "notice", "trace", "debug", "info", "warn", "warning", "error", "err", "critical", "crit", "fatal", "alert", "emerg", "panic":
			return false
		}
	}
	// Bracketed log prefixes such as [INFO] are ordinary text. An array starts
	// with a JSON value token; malformed array-shaped data is fail closed.
	tail := strings.TrimLeftFunc(value[1:], unicode.IsSpace)
	return len(tail) == 0 || strings.ContainsRune("{[\"]-0123456789ntf", rune(tail[0]))
}

func (p *projection) text(value string, depth int) (any, error) {
	if depth > maxProjectionDepth {
		return nil, errProjectionLimit
	}
	if err := p.budget.consume(len(value), 1); err != nil {
		return nil, err
	}
	// Decode valid wrappers before terminal normalization, so their encoded
	// descendants are not normalized repeatedly at every surrounding layer.
	if looksLikeJSON(value) {
		safe, err := p.encodedText(value, depth)
		if err == nil || errors.Is(err, errProjectionLimit) {
			return safe, err
		}
		p.markPossibleKubernetesYAML(value)
		normalized := normalizeRecordValue(value)
		if normalized == value {
			return p.unstructuredText(value, depth, true)
		}
		return p.normalizedText(normalized, depth)
	}
	p.markPossibleKubernetesYAML(value)
	return p.normalizedText(normalizeRecordValue(value), depth)
}

func (p *projection) markPossibleKubernetesYAML(value string) {
	if kubernetesYAMLSecretKind.MatchString(removeFormatCharacters(normalizeRecordValue(value))) {
		// A YAML document may be split across adjacent records in this channel.
		p.yamlSecretChannels[p.channel] = true
	}
}

func (p *projection) encodedText(value string, depth int) (any, error) {
	parentIndent := p.indent
	p.indent += len(value) - len(strings.TrimLeft(value, " \t"))
	defer func() { p.indent = parentIndent }()
	parsed, err := p.decodeJSON([]byte(value), depth+1)
	if err != nil {
		return nil, err
	}
	// Register only successfully decoded wrappers: failed parse attempts must
	// not create lexical events or affect configuration continuation.
	parentSource, parentLayer := p.source, p.layer
	if err := p.budget.consume(len(value), 1); err != nil {
		return nil, err
	}
	p.source = len(p.sources)
	p.sources = append(p.sources, lexicalSource{value: normalizeRecordValue(value), channel: p.channel, layer: p.layer, indent: p.indent, parent: parentSource})
	p.layer++
	defer func() { p.source, p.layer = parentSource, parentLayer }()
	safe, err := p.jsonValue(parsed, depth+1)
	if err != nil {
		return nil, err
	}
	return encodedJSON{safe, p.source}, nil
}

func (p *projection) normalizedText(value string, depth int) (any, error) {
	candidate := looksLikeJSON(value)
	if candidate {
		safe, err := p.encodedText(value, depth)
		if err == nil || errors.Is(err, errProjectionLimit) {
			return safe, err
		}
	}
	return p.unstructuredText(value, depth, candidate)
}

func (p *projection) unstructuredText(value string, depth int, invalidJSON bool) (any, error) {
	if strings.Contains(value, "\n") {
		// A text stream may be JSONL or mix JSON with ordinary log lines. Split
		// once, without re-normalizing the parent text or restarting state.
		count := strings.Count(value, "\n") + 1
		if err := p.budget.consume(0, count); err != nil {
			return nil, err
		}
		parts := strings.Split(value, "\n")
		headers := sourceBlockHeaders(parts)
		sequence := textSequence{values: make([]any, 0, len(parts))}
		for index, part := range parts {
			var item any
			var err error
			if headers[index] {
				item, err = p.leaf(part)
			} else {
				item, err = p.normalizedText(part, depth)
			}
			if err != nil {
				return nil, err
			}
			sequence.values = append(sequence.values, item)
		}
		return sequence, nil
	}
	if invalidJSON {
		// A standalone quote can close a multiline YAML/env scalar. Keep it as a
		// physical line for the shared state machine instead of treating it as a
		// malformed encoded document. Comments after a YAML close are also valid.
		trimmed := strings.TrimSpace(value)
		if trimmed != "\"" && !strings.HasPrefix(trimmed, "\" #") {
			return nil, errInvalidJSON
		}
	}
	if fragments, found, err := p.embeddedText(value, depth); found || err != nil {
		return fragments, err
	}
	return p.leaf(value)
}

func (p *projection) leaf(value string) (any, error) {
	if err := p.budget.consume(0, 1); err != nil {
		return nil, err
	}
	first := len(p.lines)
	indent := p.indent + len(value) - len(strings.TrimLeft(value, " \t"))
	source := len(p.sources)
	p.sources = append(p.sources, lexicalSource{value: value, channel: p.channel, layer: p.layer, indent: indent, parent: p.source, leaf: true})
	p.lines = append(p.lines, semanticLine{value, value, p.channel, indent, source})
	return textLeaf(first), nil
}

func (p *projection) redact() error {
	p.markPossibleKubernetesYAMLRecords()
	if err := p.projectIdentities(); err != nil {
		return err
	}
	logical, logicalHidden, err := p.logicalRecords()
	if err != nil {
		return err
	}
	for _, source := range p.sources {
		if source.parent >= 0 && p.sources[source.parent].channel == source.channel {
			p.sources[source.parent].child = true
		}
	}
	p.safe, p.suppressed = redactPhysicalLines(p.lines, p.parents, p.sources, logical, logicalHidden)
	p.redactKubernetesYAML()
	return nil
}

func (p *projection) marshal(value any) ([]byte, error) {
	if err := p.reserveJSON(value); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errInvalidJSON
	}
	return raw, nil
}

// Reserve encoding work before json.Marshal grows its output buffer. HTML and
// control escaping can expand a decoded string sixfold without growing input.
func (p *projection) reserveJSON(value any) error {
	charge := func(n int) error { return p.budget.consume(n, 0) }
	stringSize := func(value string) int {
		size := 2
		for i := 0; i < len(value); {
			b := value[i]
			switch {
			case b == '"' || b == '\\' || b == '\b' || b == '\f' || b == '\n' || b == '\r' || b == '\t':
				size += 2
				i++
			case b < 0x20 || b == '<' || b == '>' || b == '&':
				size += 6
				i++
			case b < utf8.RuneSelf:
				size++
				i++
			default:
				r, n := utf8.DecodeRuneInString(value[i:])
				if r == '\u2028' || r == '\u2029' || r == utf8.RuneError && n == 1 {
					size += 6
				} else {
					size += n
				}
				i += n
			}
		}
		return size
	}
	switch v := value.(type) {
	case nil:
		return charge(4)
	case bool:
		if v {
			return charge(4)
		}
		return charge(5)
	case json.Number:
		return charge(len(v))
	case string:
		return charge(stringSize(v))
	case []any:
		if err := charge(2 + max(0, len(v)-1)); err != nil {
			return err
		}
		for _, item := range v {
			if err := p.reserveJSON(item); err != nil {
				return err
			}
		}
	case map[string]any:
		if err := charge(2 + max(0, len(v)-1)); err != nil {
			return err
		}
		for name, item := range v {
			if err := charge(stringSize(name) + 1); err != nil {
				return err
			}
			if err := p.reserveJSON(item); err != nil {
				return err
			}
		}
	default:
		return errInvalidJSON
	}
	return nil
}

// render preserves JSON value types, array positions, encoded-string layers,
// and record positions. Suppressed JSON leaves get a placeholder; Lines can
// still drop a record whose entire semantic text was suppressed.
func (p *projection) render(value any) (any, bool, error) {
	switch typed := value.(type) {
	case maskedValue:
		hidden := false
		for i := typed.first; i < typed.last; i++ {
			hidden = hidden || p.suppressed[i]
		}
		return "[REDACTED]", hidden, nil
	case textLeaf:
		index := int(typed)
		if p.suppressed[index] {
			return "[REDACTED]", true, nil
		}
		return p.safe[index], false, nil
	case scalarLeaf:
		if p.suppressed[typed.line] {
			return "[REDACTED]", true, nil
		}
		safe := p.safe[typed.line]
		if safe != p.lines[typed.line].value {
			return safe, p.lines[typed.line].identity == p.lines[typed.line].value, nil
		}
		return typed.value, false, nil
	case encodedJSON:
		safe, suppressed, err := p.render(typed.value)
		if err != nil {
			return nil, false, err
		}
		if p.sources[typed.source].hidden || suppressed {
			return "[REDACTED]", true, nil
		}
		raw, err := p.marshal(safe)
		if err != nil {
			return nil, false, err
		}
		text := string(raw)
		return text, suppressed, nil
	case textSequence:
		parts := make([]string, 0, len(typed.values))
		for _, item := range typed.values {
			safe, suppressed, err := p.render(item)
			if err != nil {
				return nil, false, err
			}
			if !suppressed {
				parts = append(parts, safe.(string))
			}
		}
		return strings.Join(parts, "\n"), len(parts) == 0, nil
	case fragmentSequence:
		if p.sources[typed.source].hidden {
			return "[REDACTED]", true, nil
		}
		var out strings.Builder
		visible := false
		for _, item := range typed.values {
			safe, hidden, err := p.render(item)
			if err != nil {
				return nil, false, err
			}
			if !hidden {
				visible = true
				out.WriteString(safe.(string))
			}
		}
		return out.String(), !visible, nil
	case map[string]any:
		suppressed := len(typed) > 0
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			safe, hidden, err := p.render(item)
			if err != nil {
				return nil, false, err
			}
			name, err := p.keyText(key)
			if err != nil {
				return nil, false, err
			}
			result[name] = safe
			suppressed = suppressed && hidden
		}
		if suppressed {
			// A fully hidden record must not retain arbitrary keys from inside
			// a private/configuration body while only masking their values.
			return "[REDACTED]", true, nil
		}
		return result, suppressed, nil
	case []any:
		suppressed := len(typed) > 0
		for i, item := range typed {
			safe, hidden, err := p.render(item)
			if err != nil {
				return nil, false, err
			}
			typed[i] = safe
			suppressed = suppressed && hidden
		}
		return typed, suppressed, nil
	default:
		return value, false, nil
	}
}
