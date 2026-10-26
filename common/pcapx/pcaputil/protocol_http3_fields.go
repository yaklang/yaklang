package pcaputil

import (
	"net"
	"strconv"
	"strings"
)

const h3FieldConformanceScope = "field syntax and base pseudo-header/message shape; extension negotiation, authority normalization and Content-Length/body agreement are not certified"

func h3Token(s string, lower bool) bool {
	if s == "" {
		return false
	}
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' || !lower && c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			continue
		}
		return false
	}
	return true
}

// QPACK decompression verifies compression state, not HTTP field semantics.
// Validate only the stated base profile and preserve unselected extensions as
// unsupported, rather than misreporting them as a confirmed malformed message.
func h3ValidateFieldSection(headers []map[string]any, dir, prior int) (string, error) {
	bad := func(reason string) (string, error) {
		return "malformed", protocolError(ErrMalformedMessage, "HTTP3 "+reason)
	}
	if prior >= 2 {
		return bad("multiple trailer sections")
	}
	pseudo := map[string]string{}
	regular, host := false, false
	for _, h := range headers {
		name, nameOK := h["Name"].(string)
		value, valueOK := h["Value"].(string)
		if !nameOK || !valueOK || name == "" {
			return bad("invalid field representation")
		}
		plain := name
		if strings.HasPrefix(name, ":") {
			plain = name[1:]
		}
		if !h3Token(plain, true) {
			return bad("invalid or uppercase field name")
		}
		if len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[len(value)-1] == ' ' || value[len(value)-1] == '\t') {
			return bad("field value has edge whitespace")
		}
		for i := range value {
			if value[i] < 0x20 && value[i] != '\t' || value[i] == 0x7f {
				return bad("invalid field value octet")
			}
		}
		if name[0] == ':' {
			if regular || prior != 0 {
				return bad("misplaced pseudo-header")
			}
			if _, exists := pseudo[name]; exists {
				return bad("duplicate pseudo-header")
			}
			pseudo[name] = value
		} else {
			regular = true
			switch name {
			case "connection", "proxy-connection", "keep-alive", "transfer-encoding", "upgrade":
				return bad("forbidden connection-specific field")
			case "te":
				if dir != 0 || prior != 0 || !strings.EqualFold(value, "trailers") {
					return bad("invalid TE field")
				}
			case "host":
				if value == "" {
					return bad("empty Host field")
				}
				host = true
			}
		}
	}
	if prior != 0 {
		return "trailers", nil
	}
	for k := range pseudo {
		if k != ":method" && k != ":scheme" && k != ":path" && k != ":authority" && k != ":status" && k != ":protocol" {
			return "unsupported", protocolError(ErrUnsupportedFeature, "HTTP3 unselected pseudo-header extension")
		}
	}
	if dir == 1 {
		status := pseudo[":status"]
		if len(pseudo) != 1 || len(status) != 3 || status[0] < '1' || status[0] > '5' || status[1] < '0' || status[1] > '9' || status[2] < '0' || status[2] > '9' || status == "101" {
			return bad("invalid response pseudo-headers/status")
		}
		if status[0] == '1' {
			return "informational", nil
		}
		return "response", nil
	}
	if _, exists := pseudo[":status"]; exists || !h3Token(pseudo[":method"], false) {
		return bad("invalid request method/pseudo-headers")
	}
	_, scheme := pseudo[":scheme"]
	_, path := pseudo[":path"]
	protocol, extended := pseudo[":protocol"]
	if extended && (pseudo[":method"] != "CONNECT" || !h3Token(protocol, false)) {
		return bad("invalid extended CONNECT pseudo-header")
	}
	if pseudo[":method"] == "CONNECT" && !extended {
		if scheme || path {
			return bad("CONNECT includes scheme/path")
		}
		h, port, err := net.SplitHostPort(pseudo[":authority"])
		if err != nil || h == "" || strings.ContainsAny(h, "@ \t") || port == "" {
			return bad("CONNECT requires host and port")
		}
		for i := range port {
			if port[i] < '0' || port[i] > '9' {
				return bad("invalid CONNECT port")
			}
		}
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return bad("invalid CONNECT port")
		}
		return "request", nil
	}
	if !scheme || !path || pseudo[":scheme"] == "" {
		return bad("missing request pseudo-headers")
	}
	s := pseudo[":scheme"]
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			continue
		}
		return bad("invalid request scheme")
	}
	if strings.EqualFold(s, "http") || strings.EqualFold(s, "https") || extended {
		if p := pseudo[":path"]; p == "" || p != "*" && p[0] != '/' || p == "*" && pseudo[":method"] != "OPTIONS" || strings.Contains(p, "#") {
			return bad("invalid request path")
		}
		authority, present := pseudo[":authority"]
		if present && (authority == "" || strings.Contains(authority, "@")) || !present && !host || extended && !present {
			return bad("missing or invalid request authority")
		}
	}
	return "request", nil
}
