package sites

import (
	"net"
	"strconv"
	"strings"
	"unicode"
)

// Tokenize directives instead of inferring transport from certificate files.
// This also handles compact imported configs and ignores quoted/comment text.
type nginxToken struct {
	value     string
	delimiter rune
}

func nginxTokens(input string) []nginxToken {
	var tokens []nginxToken
	var word strings.Builder
	var quote rune
	escaped, comment, variable := false, false, false
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, nginxToken{value: word.String()})
			word.Reset()
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
			word.WriteRune(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
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
			quote = c
		case c == '#':
			flush()
			comment = true
		case c == ';' || c == '{' || c == '}':
			flush()
			tokens = append(tokens, nginxToken{delimiter: c})
		case unicode.IsSpace(c):
			flush()
		default:
			word.WriteRune(c)
		}
	}
	flush()
	return tokens
}

type nginxServer struct {
	names, listens []string
	legacySSL      bool
}

func nginxServers(input string) []nginxServer {
	var servers []nginxServer
	var statement, blocks []string
	server, serverDepth := -1, 0
	for _, token := range nginxTokens(input) {
		switch token.delimiter {
		case '{':
			name := ""
			if len(statement) > 0 {
				name = statement[0]
			}
			if name == "server" && len(statement) == 1 && (len(blocks) == 0 || (len(blocks) == 1 && blocks[0] == "http")) {
				servers = append(servers, nginxServer{})
				server = len(servers) - 1
				serverDepth = len(blocks) + 1
			}
			blocks = append(blocks, name)
			statement = nil
		case '}':
			if len(blocks) == serverDepth {
				server = -1
			}
			if len(blocks) > 0 {
				blocks = blocks[:len(blocks)-1]
			}
			statement = nil
		case ';':
			if server >= 0 && len(blocks) == serverDepth && len(statement) > 1 {
				switch statement[0] {
				case "server_name":
					servers[server].names = append(servers[server].names, statement[1:]...)
				case "listen":
					servers[server].listens = append(servers[server].listens, strings.Join(statement[1:], " "))
				case "ssl":
					servers[server].legacySSL = statement[1] == "on"
				}
			}
			statement = nil
		default:
			statement = append(statement, token.value)
		}
	}
	return servers
}

func discoverAccessURLs(config, primary string) []string {
	var urls []string
	for _, server := range nginxServers(config) {
		if !containsString(validDomains(server.names), primary) {
			continue
		}
		listens := server.listens
		if len(listens) == 0 {
			listens = []string{"80"}
		}
		for _, listen := range listens {
			parts := strings.Fields(listen)
			if len(parts) == 0 || strings.HasPrefix(parts[0], "unix:") {
				continue
			}
			portText := parts[0]
			if strings.Contains(portText, ":") {
				_, p, err := net.SplitHostPort(portText)
				if err != nil {
					continue
				}
				portText = p
			} else if net.ParseIP(portText) != nil {
				portText = "80"
			}
			port, err := strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				continue
			}
			scheme := "http"
			if server.legacySSL || containsString(parts[1:], "ssl") || containsString(parts[1:], "quic") {
				scheme = "https"
			}
			host := primary
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			// Keep even default ports: an explicit :80 must survive job reconciliation.
			urls = append(urls, scheme+"://"+host+":"+strconv.Itoa(port))
		}
	}
	return uniqueStrings(urls)
}
