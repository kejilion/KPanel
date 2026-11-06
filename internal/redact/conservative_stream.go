package redact

import (
	"bufio"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrStreamAmbiguous  = errors.New("stream contains context-dependent or encoded content")
	ErrStreamLimit      = errors.New("stream projection budget exceeded")
	streamKeyNormalizer = strings.NewReplacer("_", "", "-", "", ".", "")
	streamAccessLine    = regexp.MustCompile(`^([0-9A-Fa-f:.]+ - [A-Za-z0-9_.-]+ )\[([0-9]{2}/[A-Za-z]{3}/[0-9]{4}:[0-9]{2}:[0-9]{2}:[0-9]{2} [+-][0-9]{4})\]( "[^"\r\n]*" [1-5][0-9]{2} [0-9]+ "[^"\r\n]*" "[^"\r\n]*"(?: "[^"\r\n]*")?)$`)
	streamUnsafeWords   = []string{"password", "passwd", "pwd", "token", "secret", "apikey", "accesskey", "authorization", "cookie", "credential", "privatekey", "密码", "口令", "令牌", "密钥", "私钥", "auth", "key", "kind", "name", "value", "data", "signature", "bearer", "basic", "mysql", "mariadb", "curl"}
)

// ConservativeLogTail is an experimental fail-closed projection for large
// plain text streams. The caller MUST provide an immutable snapshot; reading a
// live file whose contents can change can combine unrelated security contexts.
// A successful result means every complete record was scanned before release.
// Context cancellation is cooperative and cannot interrupt a blocked Read.
func ConservativeLogTail(ctx context.Context, source io.Reader, maxInput, maxOutput int) (string, bool, error) {
	if maxInput < 1 || maxInput >= int(^uint(0)>>1) || maxOutput < 1 || maxOutput > 64<<10 {
		return "", false, ErrStreamLimit
	}
	const maxLine = 64 << 10
	reader := bufio.NewReaderSize(io.LimitReader(source, int64(maxInput)+1), maxLine+1)
	tail := make([]byte, 0, maxOutput+2*maxLine+1)
	batch := make([]byte, 0, maxLine)
	flush := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		original := string(batch)
		safe := !streamMayContainStructure(original) || Text(original) == original
		batch = batch[:0]
		if err := ctx.Err(); err != nil {
			return err
		}
		if !safe {
			return ErrStreamAmbiguous
		}
		return nil
	}
	total := 0
	truncated := false
	tailIncomplete := false
	privateActive := false
	privateLabel := ""
	for {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) || len(line) > maxLine {
			return "", false, ErrStreamLimit
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", false, err
		}
		if len(line) > maxInput-total {
			return "", false, ErrStreamLimit
		}
		total += len(line)
		if len(line) > 0 {
			if !utf8.Valid(line) || strings.IndexByte(string(line), 0) >= 0 {
				return "", false, ErrStreamAmbiguous
			}
			var view, display []byte
			trimmed := strings.TrimSpace(string(line))
			begin := privateKeyBegin.FindStringIndex(trimmed)
			end := privateKeyEnd.FindStringIndex(trimmed)
			standaloneBegin := begin != nil && begin[0] == 0 && begin[1] == len(trimmed)
			standaloneEnd := end != nil && end[0] == 0 && end[1] == len(trimmed)
			switch {
			case privateActive:
				// A marker nested in another representation may belong to a
				// different projection channel; it cannot close this flat state.
				// Encoded/control forms can conceal another channel's marker.
				if (begin != nil || end != nil) && !standaloneEnd || strings.Contains(string(line), `\`) {
					return "", false, ErrStreamAmbiguous
				}
				for _, r := range string(line) {
					if unicode.IsControl(r) && r != '\n' && r != '\t' {
						return "", false, ErrStreamAmbiguous
					}
				}
				if !standaloneEnd && !streamPEMBody(trimmed) {
					return "", false, ErrStreamAmbiguous
				}
				if standaloneEnd && streamPEMLabel(trimmed, "END") != privateLabel {
					return "", false, ErrStreamAmbiguous
				}
				view = []byte("[REDACTED PRIVATE KEY]\n")
				display = view
				if standaloneEnd {
					privateActive = false
				}
			case standaloneBegin:
				privateActive = true
				privateLabel = streamPEMLabel(trimmed, "BEGIN")
				view = []byte("[REDACTED PRIVATE KEY]\n")
				display = view
			case standaloneEnd:
				// An orphan END means prior records may be a key whose BEGIN was
				// outside the file. Discard the uncertain retained prefix.
				if err := flush(); err != nil {
					return "", false, err
				}
				tail = tail[:0]
				truncated = true
				tailIncomplete = false
				view = []byte("[REDACTED PRIVATE KEY]\n")
				display = view
			default:
				var safe bool
				view, display, safe = independentPlainRecord(line)
				if !safe {
					return "", false, ErrStreamAmbiguous
				}
			}
			if len(batch)+len(view) > maxLine {
				if err := flush(); err != nil {
					return "", false, err
				}
			}
			batch = append(batch, view...)
			tail = append(tail, display...)
			if len(tail) > maxOutput+maxLine {
				cut := len(tail) - maxOutput
				tailIncomplete = tail[cut-1] != '\n'
				tail = append(tail[:0], tail[cut:]...)
				truncated = true
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if err := flush(); err != nil {
		return "", false, err
	}
	if len(tail) > maxOutput {
		cut := len(tail) - maxOutput
		tailIncomplete = tail[cut-1] != '\n'
		tail = tail[cut:]
		truncated = true
	}
	if tailIncomplete {
		if newline := strings.IndexByte(string(tail), '\n'); newline >= 0 {
			tail = tail[newline+1:]
		}
		for len(tail) > 0 && !utf8.Valid(tail) {
			tail = tail[1:]
		}
	}
	return string(tail), truncated, nil
}

func streamPEMBody(value string) bool {
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=') {
			return false
		}
	}
	return true
}

func streamPEMLabel(value, action string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.ToUpper(value), "-----"+action+" "), "-----")
}

// Complete, strictly parsed access records with sensitive indicators are
// masked wholesale. Other ambiguous records fail the entire stream closed.
func independentPlainRecord(line []byte) ([]byte, []byte, bool) {
	if !utf8.Valid(line) {
		return nil, nil, false
	}
	text := string(line)
	body := strings.TrimSuffix(text, "\n")
	accessMatch := streamAccessLine.FindStringSubmatchIndex(body)
	view := line
	if accessMatch != nil {
		normalized := body[:accessMatch[4]-1] + "(" + body[accessMatch[4]:accessMatch[5]] + ")" + body[accessMatch[5]+1:]
		if strings.HasSuffix(text, "\n") {
			normalized += "\n"
		}
		view = []byte(normalized)
	}
	if strings.ContainsRune(text, '?') && kubernetesYAMLSecretKind.MatchString(text) {
		return nil, nil, false
	}
	unsafe := strings.ContainsAny(text, "%@")
	ascii := true
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return nil, nil, false
		}
		if r >= utf8.RuneSelf {
			ascii = false
		}
	}
	if strings.Contains(text, `\`) || strings.Contains(text, "-----") {
		return nil, nil, false
	}
	lower := strings.ToLower(text)
	if !ascii {
		lower = streamSimpleFold(text)
	}
	if strings.Contains(lower, "sk-") || strings.Contains(lower, "sig=") || strings.Contains(lower, "docker") && strings.Contains(lower, "login") {
		unsafe = true
	}
	if strings.ContainsAny(text, "?&") {
		for _, match := range urlQueryParameter.FindAllStringSubmatch(text, -1) {
			if isSensitiveURLQueryName(match[2]) {
				unsafe = true
			}
		}
	}
	normalized := streamKeyNormalizer.Replace(lower)
	for _, word := range streamUnsafeWords {
		if strings.Contains(normalized, word) {
			unsafe = true
			break
		}
	}
	if unsafe {
		if accessMatch == nil || streamMayContainStructure(string(view)) {
			return nil, nil, false
		}
		var lexical shellLexicalState
		lexical.observe(text)
		if lexical.active() || lexical.compound {
			return nil, nil, false
		}
		masked := []byte("[REDACTED LOG RECORD]\n")
		return masked, masked, true
	}
	// A standard access-log timestamp resembles a malformed JSON array to the
	// generic text projector. Normalize only that strictly validated span in the
	// validation view; the original bytes remain the eventual output.
	return view, line, true
}

// streamSimpleFold matches the Unicode simple-fold behavior used by the
// case-insensitive regular expressions in the ordinary projector.
func streamSimpleFold(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		if r < utf8.RuneSelf {
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			out.WriteRune(r)
			continue
		}
		folded := unicode.ToLower(r)
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next >= 'A' && next <= 'Z' {
				folded = next + 'a' - 'A'
				break
			}
			if next >= 'a' && next <= 'z' {
				folded = next
				break
			}
		}
		out.WriteRune(folded)
	}
	return out.String()
}

// The ordinary projector only descends into structures at these same lexical
// boundaries. Encoded escapes are rejected before this check.
func streamMayContainStructure(batch string) bool {
	for _, line := range strings.Split(batch, "\n") {
		if looksLikeJSON(line) || containsStructuredFragment(line) {
			return true
		}
		for pos := 0; pos < len(line); pos++ {
			if line[pos] != '"' {
				continue
			}
			end := strings.IndexByte(line[pos+1:], '"')
			if end < 0 {
				if pos+1 < len(line) && (line[pos+1] == '{' || line[pos+1] == '[') {
					return true
				}
				continue
			}
			end += pos + 1
			quoted := line[pos+1 : end]
			if looksLikeJSON(quoted) || containsStructuredFragment(quoted) {
				return true
			}
			pos = end
		}
	}
	return false
}
