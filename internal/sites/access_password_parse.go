package sites

import (
	"errors"
	"strings"
	"unicode"
)

// Access rules can contain regular expressions whose backslashes are
// significant. Keep those escapes instead of using the display-only tokenizer.
func accessPasswordTokens(input string) ([]nginxToken, error) {
	var result []nginxToken
	var word strings.Builder
	var quote rune
	started, escaped, comment, variable := false, false, false, false
	flush := func() {
		if started {
			result = append(result, nginxToken{value: word.String()})
			word.Reset()
			started = false
		}
	}
	for _, c := range input {
		if comment {
			if c == '\n' {
				comment = false
			}
			continue
		}
		if escaped {
			if !strings.ContainsRune("\\\"' \t\r\n;{}#", c) {
				word.WriteRune('\\')
			}
			word.WriteRune(c)
			started, escaped = true, false
			continue
		}
		if c == '\\' {
			escaped, started = true, true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			continue
		}
		if variable {
			word.WriteRune(c)
			if c == '}' {
				variable = false
			}
			continue
		}
		switch {
		case c == '{' && strings.HasSuffix(word.String(), "$"):
			variable = true
			word.WriteRune(c)
		case c == '\'' || c == '"':
			quote, started = c, true
		case c == '#':
			flush()
			comment = true
		case c == ';' || c == '{' || c == '}':
			flush()
			result = append(result, nginxToken{delimiter: c})
		case unicode.IsSpace(c):
			flush()
		default:
			started = true
			word.WriteRune(c)
		}
	}
	if quote != 0 || escaped || variable {
		return nil, errors.New("incomplete Nginx token")
	}
	flush()
	return result, nil
}

// accessPasswordDirective describes an explicit pair in one Nginx context.
// It deliberately records the actual directives, not a separate panel state.
type accessPasswordDirective struct {
	ServerIndex int
	Scope       string
	Context     string
	Modifier    string
	Realm       string
	UserFile    string
}

// parseAccessPasswordDirectives finds explicit Basic Auth pairs in server and
// location blocks, including configurations edited outside of the panel.
// Conflicting or incomplete directives are not presented as complete rules.
func parseAccessPasswordDirectives(input string) ([]accessPasswordDirective, error) {
	type context struct {
		name, path, modifier, realm, userFile string
		serverIndex                           int
		realms, files                         int
		parent                                *context
	}
	var stack []*context
	var contexts []*context
	var statement []string
	var result []accessPasswordDirective
	serverCount := 0
	tokens, err := accessPasswordTokens(input)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		switch token.delimiter {
		case '{':
			c := &context{}
			if len(stack) > 0 {
				c.parent = stack[len(stack)-1]
			}
			if len(statement) > 0 {
				c.name = statement[0]
			}
			c.serverIndex = -1
			if c.name == "server" && len(statement) == 1 && (len(stack) == 0 || len(stack) == 1 && stack[0].name == "http") {
				c.path = "/"
				c.serverIndex = serverCount
				serverCount++
			} else if c.name == "location" && len(stack) > 0 && stack[len(stack)-1].serverIndex >= 0 {
				c.serverIndex = stack[len(stack)-1].serverIndex
				if len(statement) == 2 && strings.HasPrefix(statement[1], "/") {
					c.path = statement[1]
				} else if len(statement) == 3 && ((statement[1] == "=" || statement[1] == "^~") && strings.HasPrefix(statement[2], "/") || statement[1] == "~" || statement[1] == "~*") {
					c.modifier, c.path = statement[1], statement[2]
				}
			}
			stack = append(stack, c)
			statement = nil
		case ';':
			if len(stack) > 0 && len(statement) == 2 {
				c := stack[len(stack)-1]
				switch statement[0] {
				case "auth_basic":
					c.realm, c.realms = statement[1], c.realms+1
				case "auth_basic_user_file":
					c.userFile, c.files = statement[1], c.files+1
				}
			}
			statement = nil
		case '}':
			if len(stack) == 0 {
				return nil, errors.New("unbalanced Nginx configuration")
			}
			contexts = append(contexts, stack[len(stack)-1])
			stack = stack[:len(stack)-1]
			statement = nil
		default:
			statement = append(statement, token.value)
		}
	}
	if len(stack) != 0 || len(statement) != 0 {
		return nil, errors.New("incomplete Nginx configuration")
	}
	for _, c := range contexts {
		realm, userFile := c.realm, c.userFile
		valid := c.realms <= 1 && c.files <= 1 && (c.realms != 0 || c.files != 0)
		for parent := c.parent; valid && parent != nil; parent = parent.parent {
			valid = parent.realms <= 1 && parent.files <= 1
			if realm == "" && parent.realms > 0 {
				valid = valid && parent.realms == 1
				realm = parent.realm
			}
			if userFile == "" && parent.files > 0 {
				valid = valid && parent.files == 1
				userFile = parent.userFile
			}
		}
		if valid && c.serverIndex >= 0 && c.path != "" && realm != "" && realm != "off" && userFile != "" {
			result = append(result, accessPasswordDirective{ServerIndex: c.serverIndex, Scope: c.name, Context: c.path, Modifier: c.modifier, Realm: realm, UserFile: userFile})
		}
	}
	return result, nil
}
