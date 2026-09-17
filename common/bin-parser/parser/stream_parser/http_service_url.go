package stream_parser

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// decodeHTTPServiceURL validates the bounded HTTP(S) endpoint profile used by
// discovery rules. It does not resolve names or contact the endpoint. Other URI
// schemes, credentials and fragments are outside this profile, not necessarily
// malformed generic URIs. All returned values are strings; an absent port stays
// empty rather than being replaced with an inferred default.
func decodeHTTPServiceURL(text string) (map[string]any, error) {
	if len(text) == 0 || len(text) > 8192 {
		return nil, fmt.Errorf("http service URL: length outside 1..8192 bytes")
	}
	// net/url also accepts text that it would escape when serialized. Wire
	// discovery fields contain URI bytes, not arbitrary text or an IRI. It also
	// deliberately leaves RawQuery escapes unchecked, so validate them here.
	isHex := func(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~:/?#[]@!$&'()*+,;=", rune(c)) {
			continue
		}
		if c == '%' && i+2 < len(text) && isHex(text[i+1]) && isHex(text[i+2]) {
			i += 2
			continue
		}
		return nil, fmt.Errorf("http service URL: invalid URI octet or percent escape")
	}
	u, err := url.Parse(text)
	if err != nil {
		return nil, fmt.Errorf("http service URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.Host == "" || u.User != nil || strings.Contains(text, "#") {
		return nil, fmt.Errorf("http service URL: expected absolute HTTP(S) endpoint without credentials or fragment")
	}
	authorityStart := strings.Index(text, "://") + 3
	if tail := strings.IndexAny(text[authorityStart:], "/?"); tail >= 0 && strings.ContainsAny(text[authorityStart+tail:], "[]") {
		return nil, fmt.Errorf("http service URL: brackets outside authority must be percent encoded")
	}
	host, port := u.Hostname(), u.Port()
	if host == "" {
		return nil, fmt.Errorf("http service URL: empty hostname")
	}
	if strings.HasPrefix(u.Host, "[") {
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() {
			return nil, fmt.Errorf("http service URL: invalid bracketed IPv6 address")
		}
	} else if strings.ContainsAny(host, ":[]") {
		return nil, fmt.Errorf("http service URL: IPv6 address must be bracketed")
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("http service URL: explicit port is empty")
	}
	if port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return nil, fmt.Errorf("http service URL: port outside 0..65535")
		}
	}
	canonicalIP := ""
	if addr, err := netip.ParseAddr(host); err == nil {
		canonicalIP = addr.String()
	}
	return map[string]any{
		"scheme": u.Scheme, "authority": u.Host, "hostname": host, "port": port,
		"path": u.Path, "escaped_path": u.EscapedPath(), "raw_query": u.RawQuery,
		"canonical_ip": canonicalIP,
	}, nil
}
