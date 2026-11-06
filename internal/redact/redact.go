package redact

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const privateKeyReplacement = "[REDACTED PRIVATE KEY]"

// These labels are emitted by the authoritative installation scripts as well
// as English configuration keys. All display formats share this classifier.
const secretNamePattern = `password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|authorization|cookie|credential|private[_-]?key|tls[._-]?key|密码|口令|令牌|密钥|私钥`

var (
	mysqlCommand               = regexp.MustCompile(`(?i:\b(?:mysql|mariadb|mysqldump|mariadb-dump|mysqladmin)\b)[^\r\n]*`)
	curlUserCommand            = regexp.MustCompile(`(?i:\bcurl\b)[^\r\n]*?(?:[ \t]-[a-zA-Z]*u(?:[ \t]|[^-\s])|[ \t]--user(?:[ \t]|=))[^\r\n]*`)
	dockerLoginPasswordCommand = regexp.MustCompile(`(?i:\bdocker\b)[^\r\n]*?(?i:\blogin\b)[^\r\n]*?(?:[ \t]-p(?:[ \t]|[^-\s])|[ \t]--password(?:[ \t]|=))[^\r\n]*`)
	yamlEscapedKeyAssignment   = regexp.MustCompile(`^([ \t]*(?:-[ \t]+)*)(?:(?:![^ \t\r\n]+|&[^ \t\r\n]+)[ \t]+)*("(?:[^"\\]|\\.)*")([ \t]*[:：][ \t]*)([^\r\n]*)$`)
	// A kind value or even its quoted key can use YAML escapes and folded lines.
	// Such inputs must reach the bounded parser (or its fail-closed fallback).
	kubernetesYAMLSecretKind    = regexp.MustCompile(`(?mi)(?:^[ \t]*(?:-[ \t]+|(?:![^ \t\r\n]+|&[^ \t\r\n]+)[ \t]+)*|[{,][ \t]*)(?:["']?kind["']?[ \t]*:[ \t]*(?:(?:![^ \t\r\n]+|&[^ \t\r\n]+)[ \t]+)*(?:secret(?:list)?(?:[ \t#},\]]|$)|["'|>*&!]|$)|\?(?:[ \t]|$)|"(?:\\\r?\n[ \t]*)*(?:k|\\u006b|\\U0000006b|\\x6b)(?:\\\r?\n[ \t]*)*(?:i|\\u0069|\\U00000069|\\x69)(?:\\\r?\n[ \t]*)*(?:n|\\u006e|\\U0000006e|\\x6e)(?:\\\r?\n[ \t]*)*(?:d|\\u0064|\\U00000064|\\x64)(?:\\\r?\n[ \t]*)*"[ \t]*:)`)
	jsonSecretAssignment        = regexp.MustCompile(`(?i)("(?:[^"\\]|\\.)*(?:` + secretNamePattern + `)(?:[^"\\]|\\.)*"\s*[:：]\s*)("(?:\\.|[^"\\])*"|[^,\s}\]]+)`)
	secretLineAssignment        = regexp.MustCompile(`(?i)^([ \t]*(?:-[ \t]+)*(?:export[ \t]+)?["']?(?:[\p{L}\p{N}_.-]*(?:` + secretNamePattern + `)[\p{L}\p{N}_.-]*|auths?)["']?[ \t]*[:：=][ \t]*)[^\r\n]*$`)
	secretAssignment            = regexp.MustCompile(`(?i)(^|[^?&%\p{L}\p{N}_.-])((?:[\p{L}\p{N}_.-]*(?:` + secretNamePattern + `)[\p{L}\p{N}_.-]*|auths?))(\s*[:：=]\s*)((?:"(?:\\.|[^"\\])*"?|'[^']*'?|\\.|\\$|[^\s'"\\])+)`)
	secretFlag                  = regexp.MustCompile(`(?i)(--?[\p{L}\p{N}_.-]*(?:` + secretNamePattern + `)[\p{L}\p{N}_.-]*\s+)((?:"(?:\\.|[^"\\])*"?|'[^']*'?|\\.|\\$|[^\s'"\\])+)`)
	mysqlPasswordAttachedOption = regexp.MustCompile(`(\s-p)((?:"(?:\\.|[^"\\])*"?|'[^']*'?|\\.|[^\s;&|'"\\])+)`)
	mysqlPasswordSpacedOption   = regexp.MustCompile(`(\s-p)(\s+)((?:"(?:\\.|[^"\\])*"?|'[^']*'?|\\.|[^\s;&|'"\\-])(?:"(?:\\.|[^"\\])*"?|'[^']*'?|\\.|[^\s;&|'"\\])*)`)
	authorizationHeader         = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization)(\s*[:=]\s*)("(?:\\.|[^"\\])*"|'[^']*'|[^\r\n]+)`)
	authorizationFlag           = regexp.MustCompile(`(?i)(--?(?:proxy-)?authorization\s+)("(?:\\.|[^"\\])*"|'[^']*'|[^\r\n]+)`)
	cookieHeader                = regexp.MustCompile(`(?i)\b(cookie|set-cookie)(\s*:\s*)[^\r\n]+`)
	bearerSecret                = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	basicSecret                 = regexp.MustCompile(`(?i)\bBasic\s+[A-Za-z0-9_+/=-]{4,}`)
	openAISecret                = regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9._~+/=-]{4,}`)
	urlCredentials              = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s@]+@`)
	urlQueryParameter           = regexp.MustCompile(`(?i)([?&]((?:[A-Za-z0-9_.-]|%[0-9a-f]{2})+)=)([^&#\s]*)`)
	sensitiveURLQueryName       = regexp.MustCompile(`(?i)(?:` + secretNamePattern + `|signature)`)
	terminalEscape              = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
	privateKeyBegin             = regexp.MustCompile(`(?i)-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`)
	privateKeyEnd               = regexp.MustCompile(`(?i)-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`)
)

func Text(value string) string {
	result := Records([]string{value})
	if len(result) == 0 {
		return ""
	}
	return result[0]
}

// TextWithSecrets uses the same semantic projection and budgets as Text. It
// adds exact credentials known to the caller without changing execution data.
func TextWithSecrets(value string, secrets ...string) string {
	total := 0
	for _, secret := range secrets {
		if len(secret) > maxProjectionInputBytes-total {
			return projectionLimitReplacement
		}
		total += len(secret)
	}
	if len(secrets) > 128 {
		return projectionLimitReplacement
	}
	result, _ := redactRecords([]string{value}, secrets...)
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
	var rawLines []string
	if len(data) > maxProjectionInputBytes || strings.Count(string(data), "\n") >= maxProjectionEntries {
		rawLines = []string{projectionLimitReplacement}
	} else {
		rawLines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
			rawLines = rawLines[:len(rawLines)-1]
		}
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

func redactRecords(values []string, literals ...string) ([]string, []bool) {
	p := newProjection(literals...)
	inputBytes := 0
	failClosed := func(err error) ([]string, []bool) {
		replacement := projectionLimitReplacement
		if errors.Is(err, errInvalidJSON) {
			replacement = projectionInvalidReplacement
		}
		result := make([]string, len(values))
		for i := range result {
			result[i] = replacement
		}
		return result, make([]bool, len(values))
	}
	if len(values) > maxProjectionEntries {
		return failClosed(errProjectionLimit)
	}
	for _, value := range values {
		if len(value) > maxProjectionInputBytes-inputBytes {
			return failClosed(errProjectionLimit)
		}
		inputBytes += len(value)
	}
	nodes := make([]any, 0, len(values))
	for _, value := range values {
		node, err := p.text(value, 0)
		if err != nil {
			return failClosed(err)
		}
		nodes = append(nodes, node)
	}
	if err := p.redact(); err != nil {
		return failClosed(err)
	}
	result := make([]string, len(values))
	suppressed := make([]bool, len(values))
	for i, node := range nodes {
		safe, hidden, err := p.render(node)
		if err != nil {
			return failClosed(err)
		}
		suppressed[i] = hidden
		if !hidden {
			result[i] = safe.(string)
		}
	}
	return result, suppressed
}

func normalizeRecordValue(value string) string {
	value = terminalEscape.ReplaceAllString(strings.ToValidUTF8(value, "�"), "")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, value)
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

type configurationContinuation struct {
	quote          byte
	yamlSingle     bool
	indent         int
	block          bool
	implicit       bool
	layer          int
	shell          bool
	lex            shellLexicalState
	inlineCompound bool
}

func (c configurationContinuation) active() bool {
	return c.quote != 0 || c.block || c.shell && c.lex.active()
}

func hasClosingConfigQuote(value string, quote byte, yamlSingle bool) bool {
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && !yamlSingle {
			i++
		} else if value[i] == quote {
			if yamlSingle && i+1 < len(value) && value[i+1] == quote {
				i++
				continue
			}
			return true
		}
	}
	return false
}

func (c *configurationContinuation) suppress(value string, indent int) bool {
	if c.shell && c.active() {
		c.lex.observe(value)
		return true
	}
	if c.quote != 0 {
		if hasClosingConfigQuote(value, c.quote, c.yamlSingle) {
			c.quote = 0
		}
		return true
	}
	if c.block {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || indent > c.indent {
			return true
		}
		// An implicit value may be an indentless block sequence; comments do
		// not terminate it. Explicit |/> scalars still end on any dedent.
		sequence := trimmed == "-" || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "-\t")
		if c.implicit && (strings.HasPrefix(trimmed, "#") || indent == c.indent && sequence) {
			return true
		}
		c.block = false
	}
	return false
}

func (c *configurationContinuation) begin(value string, indent int, inlineSensitive bool) {
	match := secretLineAssignment.FindStringSubmatchIndex(value)
	if len(match) < 4 {
		if inlineSensitive {
			var lex shellLexicalState
			lex.observe(value)
			if lex.active() || lex.compound {
				*c = configurationContinuation{lex: lex, shell: true, layer: c.layer, inlineCompound: lex.compound}
			}
		}
		return
	}
	prefix, tail := strings.TrimSpace(value[match[2]:match[3]]), strings.TrimSpace(value[match[3]:])
	*c = configurationContinuation{layer: c.layer}
	if strings.HasSuffix(prefix, "=") {
		// Keep whitespace after '=': it determines whether a following # is
		// part of the assignment word or starts an ordinary shell comment.
		separator := strings.LastIndexByte(value[:match[3]], '=')
		c.lex.frames = []shellFrame{{word: true}}
		c.lex.observe(value[separator+1:])
		c.shell = true
		return
	}
	// YAML node properties precede the scalar's own lexical syntax. They do
	// not turn a following block indicator or quote into a plain value.
	if strings.HasSuffix(prefix, ":") || strings.HasSuffix(prefix, "：") {
		for len(tail) > 0 && (tail[0] == '&' || tail[0] == '!') {
			end := strings.IndexAny(tail, " \t")
			if end < 0 {
				tail = ""
				break
			}
			tail = strings.TrimSpace(tail[end:])
		}
	}
	implicit := tail == "" || strings.HasPrefix(tail, "#")
	quoted := len(tail) > 0 && (tail[0] == '\'' || tail[0] == '"')
	if strings.HasSuffix(prefix, ":") && !quoted {
		// A sequence indicator occupies a column before the mapping key. Its
		// sibling mapping fields dedent to the key, not to the initial dash.
		leading := len(value) - len(strings.TrimLeft(value, " \t"))
		key := leading
		for key+1 < len(value) && value[key] == '-' && (value[key+1] == ' ' || value[key+1] == '\t') {
			key++
			for key < len(value) && (value[key] == ' ' || value[key] == '\t') {
				key++
			}
		}
		c.block, c.indent = true, indent+key-leading
		c.implicit = implicit
	} else if len(tail) > 0 && (tail[0] == '\'' || tail[0] == '"') && !hasClosingConfigQuote(tail[1:], tail[0], strings.HasSuffix(prefix, ":") && tail[0] == '\'') {
		c.quote = tail[0]
		c.yamlSingle = strings.HasSuffix(prefix, ":") && tail[0] == '\''
	}
}

// PEM masking is the union of chronological and same-channel state. Neither
// unrelated metadata nor a nested JSON field can terminate a payload's block.
// Each uncertain index is revisited at most once per state on an orphan END.
type privateContinuation struct {
	active    bool
	uncertain []int
}

func (c *privateContinuation) observe(index int, firstIsEnd, lastIsBegin, hasMarker bool, result []string, suppressed []bool) {
	if c.active {
		suppressed[index] = true
		if hasMarker {
			c.active = lastIsBegin
		}
		return
	}
	c.uncertain = append(c.uncertain, index)
	if firstIsEnd {
		result[c.uncertain[0]] = privateKeyReplacement
		for _, previous := range c.uncertain[1:] {
			suppressed[previous] = true
		}
	} else if hasMarker {
		result[index] = privateKeyReplacement
	}
	if hasMarker {
		c.active = lastIsBegin
		c.uncertain = c.uncertain[:0]
	}
}

func redactConfigurations(sources []lexicalSource, parents []int, privateHidden, inlineSensitive []bool, logical map[int]string, logicalHidden []bool) []bool {
	configurations := make(map[int]configurationContinuation)
	hidden := make([]bool, len(sources))
	for index, source := range sources {
		if source.parent >= 0 && hidden[source.parent] {
			hidden[index] = true
		}
		if source.parent >= 0 {
			parent := sources[source.parent]
			if parent.fragment && parent.channel == source.channel && parent.layer == source.layer {
				// Prefix/suffix are parts of the parent's physical record. Their
				// quotes cannot be observed twice or close its continuation early.
				sources[index].hidden = hidden[index]
				continue
			}
		}
		for channel := parents[source.channel]; channel >= 0; channel = parents[channel] {
			if ancestor := configurations[channel]; ancestor.active() {
				hidden[index] = true
			}
		}
		configuration := configurations[source.channel]
		// A structured wrapper is observed before its decoded descendants. Only
		// this representation's original syntax may close an existing quote;
		// decoded apostrophes in child JSON cannot close a parent YAML scalar.
		if source.layer == configuration.layer || source.layer < configuration.layer && !source.child {
			// Raw and encoded records may be mixed. If no same-channel decoded
			// child exists, this is the nearest available source view, and must
			// still be covered by an earlier encoded record's continuation.
			hidden[index] = configuration.suppress(source.value, source.indent) || hidden[index]
		} else if source.layer > configuration.layer && configuration.active() {
			// A wrapper that preceded the sensitive opener may contain several
			// later leaves; they inherit the active boundary without allowing a
			// deeper decoded quote to terminate its original source layer.
			hidden[index] = true
		}
		priorHidden := hidden[index]
		hidden[index] = hidden[index] || source.sensitive || logicalHidden[index]
		if !priorHidden && (source.leaf || source.fragment) && !privateHidden[index] {
			value := source.value
			if joined, ok := logical[index]; ok {
				value = joined
			}
			configuration.begin(normalizeEscapedYAMLKey(value), source.indent, inlineSensitive[index] || source.sensitive || logicalHidden[index])
			// A regex token boundary is insufficient for an inline compound
			// shell value, even if it closes within this physical record.
			hidden[index] = hidden[index] || configuration.inlineCompound
			configuration.layer = source.layer
		}
		if configuration.active() {
			configurations[source.channel] = configuration
		} else {
			delete(configurations, source.channel)
		}
		sources[index].hidden = hidden[index]
	}
	return hidden
}

func redactPhysicalLines(values []semanticLine, parents []int, sources []lexicalSource, logical map[int]string, logicalHidden []bool) ([]string, []bool) {
	result := make([]string, len(values))
	suppressed := make([]bool, len(values))
	privateHidden := make([]bool, len(sources))
	inlineSensitive := make([]bool, len(sources))
	privateChannels := make(map[int]*privateContinuation)
	var chronological privateContinuation
	for index, line := range values {
		value := line.value
		result[index] = redactLine(line.identity)
		inlineSensitive[line.source] = result[index] != value
		firstIsEnd, lastIsBegin, hasMarker := privateKeyMarkerOrder(value)
		chronological.observe(index, firstIsEnd, lastIsBegin, hasMarker, result, suppressed)
		private := privateChannels[line.channel]
		if private == nil {
			private = &privateContinuation{}
			privateChannels[line.channel] = private
		}
		private.observe(index, firstIsEnd, lastIsBegin, hasMarker, result, suppressed)
		for channel := line.channel; channel >= 0; channel = parents[channel] {
			// A decoded JSON child remains inside the physical parent scalar. Its
			// inherited indentation controls YAML dedent without altering its value.
			if ancestor := privateChannels[channel]; ancestor != nil && ancestor.active && channel != line.channel {
				suppressed[index] = true
			}
		}
	}
	for index, line := range values {
		privateHidden[line.source] = suppressed[index]
	}
	configurationHidden := redactConfigurations(sources, parents, privateHidden, inlineSensitive, logical, logicalHidden)
	for index, line := range values {
		suppressed[index] = suppressed[index] || configurationHidden[line.source]
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

// YAML double-quoted mapping keys can encode sensitive field names. Decode
// their spelling for the shared classifier and continuation state machine.
func normalizeEscapedYAMLKey(value string) string {
	parts := yamlEscapedKeyAssignment.FindStringSubmatch(value)
	if len(parts) != 5 || !strings.Contains(parts[2], `\`) {
		return value
	}
	key, ok := decodeYAMLQuotedKey(parts[2])
	if !ok {
		// Unsupported YAML escapes in a key are ambiguous; hide its value.
		key = "secret"
	}
	if !secretNormalizedJSONKey(normalizedJSONKey(key)) {
		return value
	}
	normalized := parts[1] + strconv.Quote(key) + parts[3] + parts[4]
	if secretLineAssignment.MatchString(normalized) {
		return normalized
	}
	return parts[1] + `"secret"` + parts[3] + parts[4]
}

func redactLine(value string) string {
	value = normalizeEscapedYAMLKey(value)
	// Short options are command-specific; redact the remainder of those
	// commands rather than trying to evaluate shell quoting or substitutions.
	value = curlUserCommand.ReplaceAllString(value, "[REDACTED COMMAND]")
	value = dockerLoginPasswordCommand.ReplaceAllString(value, "[REDACTED COMMAND]")
	value = authorizationHeader.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = authorizationFlag.ReplaceAllString(value, "${1}[REDACTED]")
	value = cookieHeader.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = urlQueryParameter.ReplaceAllStringFunc(value, func(parameter string) string {
		parts := urlQueryParameter.FindStringSubmatch(parameter)
		if len(parts) != 4 {
			return parameter
		}
		name, err := url.QueryUnescape(parts[2])
		if err != nil || !isSensitiveURLQueryName(name) {
			return parameter
		}
		return parts[1] + "[REDACTED]"
	})
	// Standalone configuration assignments (YAML/.env) may contain unquoted
	// spaces; shell-token matching alone would expose the rest of the value.
	value = secretLineAssignment.ReplaceAllString(value, "${1}[REDACTED]")
	value = jsonSecretAssignment.ReplaceAllString(value, `${1}"[REDACTED]"`)
	value = mysqlCommand.ReplaceAllStringFunc(value, func(command string) string {
		command = mysqlPasswordAttachedOption.ReplaceAllString(command, "${1}[REDACTED]")
		return mysqlPasswordSpacedOption.ReplaceAllString(command, "${1}${2}[REDACTED]")
	})
	value = secretAssignment.ReplaceAllString(value, "${1}${2}${3}[REDACTED]")
	value = secretFlag.ReplaceAllString(value, "${1}[REDACTED]")
	value = bearerSecret.ReplaceAllString(value, "Bearer [REDACTED]")
	value = basicSecret.ReplaceAllStringFunc(value, func(match string) string {
		parts := strings.Fields(match)
		if len(parts) != 2 {
			return match
		}
		if len(parts[1]) < 8 {
			decoded, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(parts[1])
			}
			if err != nil {
				decoded, err = base64.URLEncoding.DecodeString(parts[1])
			}
			if err != nil {
				decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
			}
			if err != nil || !bytes.ContainsRune(decoded, ':') {
				return match
			}
		}
		return "Basic [REDACTED]"
	})
	value = openAISecret.ReplaceAllString(value, "[REDACTED]")
	value = urlCredentials.ReplaceAllString(value, "${1}[REDACTED]@")
	return strings.Map(func(character rune) rune {
		if character == '\t' || !unicode.IsControl(character) {
			return character
		}
		return -1
	}, value)
}

func isSensitiveURLQueryName(name string) bool {
	if sensitiveURLQueryName.MatchString(name) {
		return true
	}
	// Split compound names such as access_token and accessToken so generic
	// words like key and sig only match when they are complete name parts.
	parts := make([]string, 0, 4)
	runes := []rune(name)
	start := 0
	for index, character := range runes {
		if character == '_' || character == '-' || character == '.' {
			if start < index {
				parts = append(parts, strings.ToLower(string(runes[start:index])))
			}
			start = index + 1
			continue
		}
		if index > start && unicode.IsUpper(character) && (unicode.IsLower(runes[index-1]) || unicode.IsDigit(runes[index-1])) {
			parts = append(parts, strings.ToLower(string(runes[start:index])))
			start = index
		}
	}
	if start < len(runes) {
		parts = append(parts, strings.ToLower(string(runes[start:])))
	}
	for _, part := range parts {
		switch part {
		case "password", "passwd", "pwd", "token", "secret", "authorization", "cookie", "credential", "signature", "key", "sig":
			return true
		case "signingkey":
			return true
		}
	}
	return false
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
