package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const (
	textInternetLineLimit  = 2048
	textInternetBlockLimit = 32 << 10
	textInternetListLimit  = 64
	textInternetValueLimit = 4096
)

type binTextInternet struct {
	protocol  string
	clientDir int

	query             string
	user              string
	fingerPlanPending bool
	whoisReferral     string
	whoisID           string

	gopherMode     string
	gopherSelector string

	dictClient     string
	dictDatabase   string
	dictWord       string
	dictDefinition string
}

var binTextInternetFrameSpec = &binSpec{entry: "TextInternetLine"}

func isTextInternetProtocol(protocol string) bool {
	switch protocol {
	case "finger", "whois", "gopher", "dict":
		return true
	default:
		return false
	}
}

func (f *binFlow) probeTextInternet(dir int, wire []byte) *binTextInternet {
	if len(f.ports) < 2 {
		return nil
	}
	serverPort := f.ports[1]
	switch {
	case serverPort == 79 && dir == 0:
		user, ok := fingerQuery(wire)
		if ok {
			return &binTextInternet{protocol: "finger", clientDir: dir, query: string(bytes.TrimSpace(wire)), user: user}
		}
	case serverPort == 43 && dir == 0:
		query, ok := whoisQuery(wire)
		if ok {
			return &binTextInternet{protocol: "whois", clientDir: dir, query: query}
		}
	case serverPort == 70 && dir == 1:
		if _, ok := gopherMenuItem(wire); ok {
			return &binTextInternet{protocol: "gopher", clientDir: 0, gopherMode: "menu"}
		}
		if gopherTextBlock(wire) {
			return &binTextInternet{protocol: "gopher", clientDir: 0, gopherMode: "text"}
		}
	case serverPort == 2628:
		if dir == 1 && dictGreeting(wire) {
			return &binTextInternet{protocol: "dict", clientDir: 0}
		}
		if dir == 0 {
			if client, ok := dictClient(wire); ok {
				return &binTextInternet{protocol: "dict", clientDir: dir, dictClient: client}
			}
		}
	}
	return nil
}

func fingerQuery(wire []byte) (string, bool) {
	line, ok := firstTextLine(wire, 512)
	if !ok {
		return "", false
	}
	query := strings.TrimSpace(line)
	if strings.HasPrefix(query, "/W ") {
		query = strings.TrimSpace(strings.TrimPrefix(query, "/W "))
	}
	if !validTextToken(query, 64) || strings.ContainsAny(query, "@/ ") {
		return "", false
	}
	return query, true
}

func whoisQuery(wire []byte) (string, bool) {
	line, ok := firstTextLine(wire, 253)
	if !ok {
		return "", false
	}
	line = strings.TrimSpace(line)
	if len(line) < 3 || len(line) > 253 || strings.Count(line, ".") == 0 {
		return "", false
	}
	for _, label := range strings.Split(line, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return "", false
			}
		}
	}
	return line, true
}

func gopherMenuItem(wire []byte) (map[string]any, bool) {
	line, ok := firstTextLine(wire, textInternetLineLimit)
	if !ok || len(line) < 5 || !gopherItemType(line[0]) {
		return nil, false
	}
	parts := strings.Split(line[1:], "\t")
	if len(parts) != 4 || parts[0] == "" || parts[2] == "" {
		return nil, false
	}
	port, err := strconv.Atoi(parts[3])
	if err != nil || port < 1 || port > 65535 || !validTextToken(parts[2], 255) {
		return nil, false
	}
	if len(parts[1]) > 1024 || strings.ContainsAny(parts[1], "\r\n\x00") || strings.ContainsAny(parts[0], "\r\n\x00") {
		return nil, false
	}
	return map[string]any{"Type": string(line[0]), "Display": parts[0], "Selector": parts[1], "Host": parts[2], "Port": port}, true
}

func gopherItemType(c byte) bool {
	return c >= '0' && c <= '9' || bytes.IndexByte([]byte("+TgIh"), c) >= 0
}

func gopherTextBlock(wire []byte) bool {
	lines := textBlockLines(wire)
	if len(wire) > textInternetBlockLimit || len(lines) == 0 || len(lines) > textInternetListLimit {
		return false
	}
	first := []byte(lines[0])
	// A tab-delimited row beginning with a Gopher item type is menu syntax.
	// Do not downgrade a malformed menu row to an otherwise plausible text item.
	if len(first) > 0 && gopherItemType(first[0]) && bytes.Contains(first, []byte{'\t'}) {
		return false
	}
	return true
}

func gopherDotEnd(wire []byte) int {
	for at := 0; at < len(wire); {
		end := bytes.Index(wire[at:], []byte("\r\n"))
		if end < 0 {
			return 0
		}
		end += at
		if end-at == 1 && wire[at] == '.' {
			return end + 2
		}
		at = end + 2
	}
	return 0
}

func dictGreeting(wire []byte) bool {
	line, ok := firstTextLine(wire, 512)
	if !ok || !strings.HasPrefix(line, "220 ") {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "220" {
		return false
	}
	version := fields[len(fields)-1]
	if len(version) < 5 || version[0] != '<' || version[len(version)-1] != '>' {
		return false
	}
	parts := strings.Split(version[1:len(version)-1], ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for i := 0; i < len(part); i++ {
			if part[i] < '0' || part[i] > '9' {
				return false
			}
		}
	}
	return true
}

func dictClient(wire []byte) (string, bool) {
	line, ok := firstTextLine(wire, 512)
	if !ok {
		return "", false
	}
	fields := strings.Fields(line)
	if len(fields) != 2 || fields[0] != "CLIENT" || !validDICTClientInfo(fields[1], 128) {
		return "", false
	}
	return fields[1], true
}

func validDICTClientInfo(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || bytes.IndexByte([]byte("._-+/"), c) >= 0) {
			return false
		}
	}
	return true
}

func exactTextLine(wire []byte, limit int) (string, bool) {
	if len(wire) < 2 || len(wire) > limit+2 || !bytes.HasSuffix(wire, []byte("\r\n")) {
		return "", false
	}
	line := wire[:len(wire)-2]
	if bytes.Contains(line, []byte("\r")) || bytes.Contains(line, []byte("\n")) || !printableInternetText(line) {
		return "", false
	}
	return string(line), true
}

func firstTextLine(wire []byte, limit int) (string, bool) {
	end := bytes.Index(wire, []byte("\r\n"))
	if end < 0 || end > limit || !printableInternetText(wire[:end]) {
		return "", false
	}
	return string(wire[:end]), true
}

func printableInternetText(wire []byte) bool {
	for _, c := range wire {
		if c != '\t' && (c < 0x20 || c > 0x7e) {
			return false
		}
	}
	return true
}

func validTextToken(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || bytes.IndexByte([]byte("._-+"), c) >= 0) {
			return false
		}
	}
	return true
}

func (f *binFlow) frameTextInternet(dir int, wire []byte) (int, *binSpec, error) {
	if len(wire) > f.a.config.MaxMessageBytes {
		return 0, nil, fmt.Errorf("%s message exceeds limit", f.protocol)
	}
	if f.textInternet == nil {
		return 0, nil, fmt.Errorf("%s state is absent", f.protocol)
	}
	client := dir == f.textInternet.clientDir
	switch f.protocol {
	case "gopher":
		if !client {
			if end := gopherDotEnd(wire); end > 0 {
				return end, binTextInternetFrameSpec, nil
			}
			return 0, nil, nil
		}
	case "dict":
		if !client {
			line, ok := firstTextLine(wire, textInternetLineLimit)
			if !ok {
				return 0, nil, nil
			}
			if strings.HasPrefix(line, "110 ") || strings.HasPrefix(line, "150 ") {
				if end := gopherDotEnd(wire); end > 0 {
					return end, binTextInternetFrameSpec, nil
				}
				return 0, nil, nil
			}
		}
	}
	end := bytes.Index(wire, []byte("\r\n"))
	if end < 0 {
		if len(wire) > textInternetLineLimit {
			return 0, nil, fmt.Errorf("%s line exceeds limit", f.protocol)
		}
		return 0, nil, nil
	}
	if end > textInternetLineLimit || !printableInternetText(wire[:end]) {
		return 0, nil, fmt.Errorf("%s line is invalid or exceeds limit", f.protocol)
	}
	return end + 2, binTextInternetFrameSpec, nil
}

func (s *binTextInternet) consume(dir int, wire []byte) (map[string]any, error) {
	if len(wire) == 0 || len(wire) > textInternetBlockLimit || !bytes.HasSuffix(wire, []byte("\r\n")) {
		return nil, fmt.Errorf("%s message boundary is incomplete", s.protocol)
	}
	out := map[string]any{"Protocol": s.protocol}
	client := dir == s.clientDir
	switch s.protocol {
	case "finger":
		if client {
			user, ok := fingerQuery(wire)
			if !ok {
				return nil, fmt.Errorf("finger query is invalid")
			}
			s.user, s.query = user, string(bytes.TrimSpace(wire))
			out["Packet Name"], out["Query"], out["User"] = "Query", s.query, user
		} else {
			line := strings.TrimSuffix(string(wire), "\r\n")
			switch {
			case strings.HasPrefix(line, "Login: "):
				nameAt := strings.Index(line, " Name: ")
				if nameAt < 7 || nameAt+7 >= len(line) {
					return nil, fmt.Errorf("finger login line is malformed")
				}
				login := strings.TrimSpace(line[7:nameAt])
				if !validTextToken(login, 64) {
					return nil, fmt.Errorf("finger login is malformed")
				}
				out["Packet Name"], out["Login"], out["Name"] = "Login", login, line[nameAt+7:]
			case line == "Plan:":
				s.fingerPlanPending = true
				out["Packet Name"], out["Section"] = "Plan Header", "Plan"
			case s.fingerPlanPending:
				s.fingerPlanPending = false
				out["Packet Name"], out["Plan"] = "Plan Text", trimTextValue(line, textInternetValueLimit)
			default:
				out["Packet Name"] = "Response Line"
			}
		}
		out["Query User"] = s.user
	case "whois":
		if client {
			query, ok := whoisQuery(wire)
			if !ok {
				return nil, fmt.Errorf("whois query is invalid")
			}
			s.query = query
			out["Packet Name"], out["Query"] = "Query", query
		} else {
			line := strings.TrimSuffix(string(wire), "\r\n")
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				out["Packet Name"] = "Response Line"
			} else {
				key, value = strings.TrimSpace(key), trimTextValue(strings.TrimSpace(value), 1024)
				out["Packet Name"] = "Response Field"
				switch key {
				case "Domain Name":
					out["Domain Name"] = value
				case "ReferralServer":
					s.whoisReferral = value
					out["Referral Server"] = value
				case "Registry Domain ID":
					s.whoisID = value
					out["Registry Domain ID"] = value
				}
			}
		}
		out["Query Name"], out["Referral Server Value"], out["Registry Domain ID Value"] = s.query, s.whoisReferral, s.whoisID
	case "gopher":
		if client {
			selector := strings.TrimSuffix(string(wire), "\r\n")
			if len(selector) > 1024 || strings.ContainsAny(selector, "\r\n\x00") || !printableInternetText([]byte(selector)) {
				return nil, fmt.Errorf("gopher selector is invalid")
			}
			s.gopherSelector = selector
			out["Packet Name"], out["Selector"] = "Selector", selector
		} else {
			lines := textBlockLines(wire)
			if len(lines) == 0 || len(lines) > textInternetListLimit {
				return nil, fmt.Errorf("gopher response block is invalid")
			}
			out["Selector"] = s.gopherSelector
			if s.gopherMode == "menu" {
				items := make([]map[string]any, 0, len(lines))
				for _, line := range lines {
					item, ok := gopherMenuItem(append([]byte(line), '\r', '\n'))
					if !ok {
						return nil, fmt.Errorf("gopher menu item is malformed")
					}
					items = append(items, item)
				}
				out["Packet Name"], out["Menu Items"] = "Menu", items
				if len(items) > 0 {
					out["Menu Item"], out["Menu Selector"] = items[0]["Display"], items[0]["Selector"]
				}
			} else {
				body := strings.Join(lines, "\r\n")
				out["Packet Name"], out["Body"] = "Text Item", trimTextValue(body, textInternetValueLimit)
			}
		}
	case "dict":
		line, ok := exactTextLine(wire, textInternetLineLimit)
		if !client && (bytes.Contains(wire, []byte("\r\n.\r\n")) || bytes.HasPrefix(wire, []byte("."))) {
			return s.consumeDICTBlock(out, wire)
		}
		if !ok {
			return nil, fmt.Errorf("dict line is malformed")
		}
		if client {
			fields := strings.Fields(line)
			switch {
			case len(fields) == 2 && fields[0] == "CLIENT" && validDICTClientInfo(fields[1], 128):
				s.dictClient = fields[1]
				out["Packet Name"] = "CLIENT"
			case line == "SHOW DB":
				out["Packet Name"] = "SHOW DB"
			case len(fields) == 3 && fields[0] == "DEFINE" && validTextToken(fields[1], 128) && validTextToken(fields[2], 128):
				s.dictDatabase, s.dictWord = fields[1], fields[2]
				out["Packet Name"] = "DEFINE"
			case line == "QUIT":
				out["Packet Name"] = "QUIT"
			default:
				return nil, protocolError(ErrUnsupportedFeature, "DICT command is outside the captured profile")
			}
		} else {
			if len(line) < 4 || line[0] < '0' || line[0] > '9' || line[1] < '0' || line[1] > '9' || line[2] < '0' || line[2] > '9' || line[3] != ' ' {
				return nil, fmt.Errorf("dict status line is malformed")
			}
			status, _ := strconv.Atoi(line[:3])
			out["Status Code"], out["Status"] = status, strings.TrimSpace(line[4:])
			switch status {
			case 220:
				out["Packet Name"] = "Greeting"
			case 250:
				out["Packet Name"] = "Success"
			case 221:
				out["Packet Name"] = "Quit"
			default:
				return nil, protocolError(ErrUnsupportedFeature, "DICT status is outside the captured profile")
			}
		}
		out["Client Name"], out["Database"], out["Word"], out["Definition"] = s.dictClient, s.dictDatabase, s.dictWord, s.dictDefinition
	}
	for key, value := range out {
		if text, ok := value.(string); ok && len(text) > textInternetValueLimit {
			out[key] = text[:textInternetValueLimit]
		}
	}
	return out, nil
}

func (s *binTextInternet) consumeDICTBlock(out map[string]any, wire []byte) (map[string]any, error) {
	lines := textBlockLines(wire)
	if len(lines) < 2 || len(lines) > textInternetListLimit {
		return nil, fmt.Errorf("dict multiline response is invalid")
	}
	first := lines[0]
	if len(first) < 4 {
		return nil, fmt.Errorf("dict multiline status is invalid")
	}
	status, err := strconv.Atoi(first[:3])
	if err != nil || first[3] != ' ' {
		return nil, fmt.Errorf("dict multiline status is invalid")
	}
	out["Status Code"], out["Status"] = status, strings.TrimSpace(first[4:])
	switch status {
	case 110:
		out["Packet Name"] = "Database List"
		name, description, ok := dictDatabaseLine(lines[1])
		if !ok {
			return nil, fmt.Errorf("dict database row is malformed")
		}
		s.dictDatabase = name
		out["Database"], out["Database Description"] = name, description
	case 150:
		if len(lines) < 3 {
			return nil, fmt.Errorf("dict definition header is missing")
		}
		word, database, description, ok := dictDefinitionHeader(lines[1])
		if !ok {
			return nil, fmt.Errorf("dict definition header is malformed")
		}
		s.dictWord, s.dictDatabase = word, database
		body := strings.Join(lines[2:], "\r\n")
		s.dictDefinition = trimTextValue(body, textInternetValueLimit)
		out["Packet Name"], out["Word"], out["Database"], out["Definition Description"] = "Definition", word, database, description
		out["Definition"] = s.dictDefinition
	default:
		return nil, protocolError(ErrUnsupportedFeature, "DICT multiline response is outside the captured profile")
	}
	out["Client Name"], out["Definition"] = s.dictClient, s.dictDefinition
	return out, nil
}

func dictDatabaseLine(line string) (string, string, bool) {
	name, remainder, ok := strings.Cut(line, " ")
	if !ok || !validTextToken(name, 128) {
		return "", "", false
	}
	description, rest, ok := parseDICTQuoted(remainder)
	return name, description, ok && rest == ""
}

func dictDefinitionHeader(line string) (string, string, string, bool) {
	word, rest, ok := parseDICTQuoted(strings.TrimPrefix(line, "151 "))
	if !ok || len(line) < 4 || !strings.HasPrefix(line, "151 ") {
		return "", "", "", false
	}
	database, rest, ok := strings.Cut(rest, " ")
	if !ok || !validTextToken(database, 128) {
		return "", "", "", false
	}
	description, rest, ok := parseDICTQuoted(rest)
	return word, database, description, ok && rest == ""
}

func parseDICTQuoted(value string) (string, string, bool) {
	value = strings.TrimLeft(value, " ")
	if len(value) < 2 || value[0] != '"' {
		return "", "", false
	}
	end := strings.IndexByte(value[1:], '"')
	if end < 0 {
		return "", "", false
	}
	end++
	text := value[1:end]
	return trimTextValue(text, 1024), strings.TrimSpace(value[end+1:]), true
}

func textBlockLines(wire []byte) []string {
	if len(wire) == 0 || !bytes.HasSuffix(wire, []byte("\r\n.\r\n")) && !bytes.Equal(wire, []byte(".\r\n")) {
		return nil
	}
	body := wire
	if bytes.Equal(body, []byte(".\r\n")) {
		return nil
	}
	body = body[:len(body)-5]
	lines := strings.Split(strings.TrimSuffix(string(body), "\r\n"), "\r\n")
	for _, line := range lines {
		if !printableInternetText([]byte(line)) {
			return nil
		}
	}
	return lines
}

func trimTextValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}
