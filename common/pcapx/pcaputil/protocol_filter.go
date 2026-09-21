package pcaputil

import (
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"unicode"
)

// DisplayFields returns stable, typed aliases. Semantic/derived fields carry
// their event's ByteSource, never offsets into compressed or encrypted packets.
func (e *ProtocolEvent) DisplayFields() map[string][]any {
	m := map[string][]any{"protocol": {e.Protocol}, "status": {e.Status}, "pdu.id": {e.ID}, "pdu.length": {e.Length}, "source": {e.Source}, "destination": {e.Destination}}
	for _, endpoint := range []string{e.Source, e.Destination} {
		host, _, err := net.SplitHostPort(endpoint)
		if err != nil {
			host = endpoint
		}
		if net.ParseIP(host) != nil {
			m["ip.addr"] = append(m["ip.addr"], host)
		}
	}
	if d, ok := e.Session["DNS"].(map[string]any); ok {
		m["dns.id"] = []any{d["ID"]}
		if qs, ok := d["Questions"].([]map[string]any); ok {
			for _, q := range qs {
				m["dns.qry.name"] = append(m["dns.qry.name"], q["Name"])
				m["dns.qry.type"] = append(m["dns.qry.type"], q["Type"])
			}
		}
	}
	if v, ok := e.Session["SNI"].(string); ok && v != "" {
		m["tls.handshake.extensions_server_name"] = []any{v}
	}
	if v, ok := e.Session["Negotiated Version"]; ok {
		m["tls.version"] = []any{v}
	}
	if e.Protocol == "http" || e.Protocol == "doh" || e.Protocol == "ipp" {
		if i := strings.IndexByte(string(e.Raw), ' '); i > 0 && i < 16 {
			first := string(e.Raw[:i])
			if strings.HasPrefix(first, "HTTP/") {
				parts := strings.SplitN(string(e.Raw), " ", 3)
				if n, err := strconv.Atoi(parts[1]); err == nil {
					m["http.response.code"] = []any{n}
				}
			} else {
				m["http.request.method"] = []any{first}
			}
		}
	}
	for alias, key := range map[string]string{"http2.streamid": "Stream ID", "grpc.status": "GRPC Status", "tls.record.authenticated": "Authentication Verified"} {
		if v, ok := e.Session[key]; ok {
			if alias == "grpc.status" {
				if n, err := strconv.Atoi(fmt.Sprint(v)); err == nil {
					v = n
				}
			}
			m[alias] = []any{v}
		}
	}
	return m
}

var displayAliases = map[string]bool{"protocol": true, "status": true, "pdu.id": true, "pdu.length": true, "source": true, "destination": true, "ip.addr": true, "dns.id": true, "dns.qry.name": true, "dns.qry.type": true, "tls.handshake.extensions_server_name": true, "tls.version": true, "tls.record.authenticated": true, "http.request.method": true, "http.response.code": true, "http2.streamid": true, "grpc.status": true}

type DisplayFilter struct{ match func(map[string][]any) bool }

func (f *DisplayFilter) Match(e *ProtocolEvent) bool { return f == nil || f.match(e.DisplayFields()) }

type filterToken struct {
	s      string
	quoted bool
}

func CompileDisplayFilter(expr string) (*DisplayFilter, error) {
	if len(expr) > 4096 {
		return nil, fmt.Errorf("display filter exceeds 4096 bytes")
	}
	var tokens []filterToken
	for i := 0; i < len(expr); {
		if unicode.IsSpace(rune(expr[i])) {
			i++
			continue
		}
		if len(tokens) >= 128 {
			return nil, fmt.Errorf("display filter token budget")
		}
		if expr[i] == '"' {
			start := i
			i++
			for i < len(expr) && expr[i] != '"' {
				if expr[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(expr) {
				return nil, fmt.Errorf("unterminated filter string")
			}
			i++
			s, err := strconv.Unquote(expr[start:i])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, filterToken{s, true})
			continue
		}
		if strings.ContainsRune("()=!<>", rune(expr[i])) {
			n := 1
			if i+1 < len(expr) && expr[i+1] == '=' && expr[i] != '(' && expr[i] != ')' {
				n = 2
			}
			tokens = append(tokens, filterToken{s: expr[i : i+n]})
			i += n
			continue
		}
		start := i
		for i < len(expr) && !unicode.IsSpace(rune(expr[i])) && !strings.ContainsRune("()=!<>\"", rune(expr[i])) {
			i++
		}
		tokens = append(tokens, filterToken{s: expr[start:i]})
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	p := filterParser{tokens: tokens}
	fn, err := p.parse(0, 0)
	if err != nil {
		return nil, err
	}
	if p.pos != len(tokens) {
		return nil, fmt.Errorf("unexpected filter token")
	}
	return &DisplayFilter{fn}, nil
}

type filterParser struct {
	tokens []filterToken
	pos    int
}
type fieldPredicate func(map[string][]any) bool

func (p *filterParser) take(s string) bool {
	if p.pos < len(p.tokens) && !p.tokens[p.pos].quoted && p.tokens[p.pos].s == s {
		p.pos++
		return true
	}
	return false
}
func (p *filterParser) parse(level, depth int) (fieldPredicate, error) {
	if depth > 32 {
		return nil, fmt.Errorf("display filter depth")
	}
	if level < 2 {
		left, err := p.parse(level+1, depth+1)
		if err != nil {
			return nil, err
		}
		op := "or"
		if level == 1 {
			op = "and"
		}
		for p.take(op) {
			right, err := p.parse(level+1, depth+1)
			if err != nil {
				return nil, err
			}
			a, b := left, right
			if op == "or" {
				left = func(m map[string][]any) bool { return a(m) || b(m) }
			} else {
				left = func(m map[string][]any) bool { return a(m) && b(m) }
			}
		}
		return left, nil
	}
	if p.take("not") {
		f, err := p.parse(2, depth+1)
		return func(m map[string][]any) bool { return !f(m) }, err
	}
	if p.take("(") {
		f, err := p.parse(0, depth+1)
		if err != nil {
			return nil, err
		}
		if !p.take(")") {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return f, nil
	}
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("missing field")
	}
	name := p.tokens[p.pos].s
	p.pos++
	if !displayAliases[name] {
		return nil, fmt.Errorf("unknown display field %q", name)
	}
	op := ""
	for _, candidate := range []string{"==", "!=", ">=", "<=", ">", "<", "contains", "in"} {
		if p.take(candidate) {
			op = candidate
			break
		}
	}
	if op == "" {
		return func(m map[string][]any) bool { return len(m[name]) > 0 }, nil
	}
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("missing comparison value")
	}
	value := p.tokens[p.pos]
	p.pos++
	var network *net.IPNet
	if op == "in" {
		_, n, err := net.ParseCIDR(value.s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR")
		}
		network = n
	}
	return func(m map[string][]any) bool {
		vals := m[name]
		if len(vals) == 0 {
			return false
		}
		equal := false
		for _, v := range vals {
			str, ok := v.(string)
			if op == "in" {
				if network.Contains(net.ParseIP(fmt.Sprint(v))) {
					return true
				}
				continue
			}
			if op == "contains" {
				if ok && strings.Contains(str, value.s) {
					return true
				}
				continue
			}
			cmp := 2
			if ok {
				cmp = strings.Compare(str, value.s)
			} else if b, ok := v.(bool); ok {
				if value.s == strconv.FormatBool(b) {
					cmp = 0
				}
			} else {
				a, oka := new(big.Rat).SetString(fmt.Sprint(v))
				b, okb := new(big.Rat).SetString(value.s)
				if oka && okb {
					cmp = a.Cmp(b)
				}
			}
			if cmp == 0 {
				equal = true
			}
			switch op {
			case "==":
				if cmp == 0 {
					return true
				}
			case ">":
				if cmp == 1 {
					return true
				}
			case "<":
				if cmp == -1 {
					return true
				}
			case ">=":
				if cmp == 0 || cmp == 1 {
					return true
				}
			case "<=":
				if cmp == 0 || cmp == -1 {
					return true
				}
			}
		}
		return op == "!=" && !equal
	}, nil
}
