package match

import (
	"bytes"
	"strings"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

func attachAppLayerMatchers(c *matchContext) {
	if c.Rule == nil || c.Rule.ContentRuleConfig == nil {
		return
	}
	if c.Rule.ContentRuleConfig.AppLayerProtocol != "" {
		c.Attach(appLayerProtocolMatcher)
	}
	if len(c.Rule.ContentRuleConfig.AppLayerEvents) > 0 {
		c.Attach(appLayerEventMatcher)
	}
}

// appLayerProtocolMatcher evaluates app-layer-protocol against lightweight
// application-protocol detectors (http/http1, tls, ssh, dns, ftp, smtp,
// telnet, rdp), supporting both positive and negated ("!tls", "!http") checks.
func appLayerProtocolMatcher(c *matchContext) error {
	config := c.Rule.ContentRuleConfig
	if config == nil || config.AppLayerProtocol == "" {
		return nil
	}
	payload := c.GetBuffer(modifier.Default)
	var detected bool
	switch config.AppLayerProtocol {
	case "http", "http1":
		detected = looksLikeHTTP(payload)
	case "http2":
		detected = LooksLikeHTTP2(payload)
	case "tls":
		detected = LooksLikeTLS(payload)
	case "ssh":
		detected = LooksLikeSSH(payload)
	case "dns":
		_, _, _, detected = ExtractDNSQueryName(c.PK)
	case "telnet":
		detected = looksLikeTelnet(payload)
	case "rdp":
		detected = looksLikeRDP(payload)
	case "smtp":
		detected = looksLikeSMTP(payload)
	case "ftp":
		detected = looksLikeFTP(payload)
	case "snmp":
		_, detected = ParseSNMPVersion(payload)
	case "dcerpc":
		detected = LooksLikeDCERPC(payload)
	default:
		c.Reject()
		return nil
	}
	if config.AppLayerProtocolNegated {
		detected = !detected
	}
	c.Must(detected)
	return nil
}

func looksLikeTelnet(payload []byte) bool {
	// Telnet IAC (0xFF) followed by a valid command byte (WILL/WONT/DO/DONT/SB/etc. 240..255).
	for i := 0; i+1 < len(payload) && i < 64; i++ {
		if payload[i] == 0xff && payload[i+1] >= 0xf0 {
			return true
		}
	}
	return false
}

func looksLikeRDP(payload []byte) bool {
	// TPKT header (version 3, reserved 0) + X.224 COTP length/PDU type.
	return len(payload) >= 7 && payload[0] == 0x03 && payload[1] == 0x00 && (payload[5] == 0xe0 || payload[5] == 0xd0 || payload[5] == 0xf0)
}

func looksLikeSMTP(payload []byte) bool {
	upper := payload
	if len(upper) > 16 {
		upper = upper[:16]
	}
	for _, prefix := range [][]byte{
		[]byte("EHLO "), []byte("HELO "), []byte("MAIL FROM:"), []byte("RCPT TO:"), []byte("220 "),
	} {
		if len(upper) >= len(prefix) && bytes.EqualFold(upper[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func looksLikeFTP(payload []byte) bool {
	upper := payload
	if len(upper) > 16 {
		upper = upper[:16]
	}
	for _, prefix := range [][]byte{
		[]byte("USER "), []byte("PASS "), []byte("PORT "), []byte("PASV"), []byte("RETR "), []byte("STOR "), []byte("220 "),
	} {
		if len(upper) >= len(prefix) && bytes.EqualFold(upper[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func looksLikeHTTP(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	line := payload
	if index := bytes.Index(line, []byte("\r\n")); index >= 0 {
		line = line[:index]
	} else if index := bytes.IndexByte(line, '\n'); index >= 0 {
		line = line[:index]
	}
	fields := bytes.Fields(line)
	if len(fields) == 0 {
		return false
	}
	if bytes.HasPrefix(fields[0], []byte("HTTP/")) {
		return true
	}
	return len(fields) >= 3 && bytes.HasPrefix(fields[2], []byte("HTTP/"))
}

func appLayerEventMatcher(c *matchContext) error {
	config := c.Rule.ContentRuleConfig
	if config == nil || len(config.AppLayerEvents) == 0 {
		return nil
	}
	raw := c.GetBuffer(modifier.Default)
	for _, event := range config.AppLayerEvents {
		var present bool
		switch event.Name {
		case "http.invalid_transfer_encoding_value_in_request":
			present = httpInvalidTransferEncodingInRequest(raw)
		case "http.request_auth_unrecognized":
			present = httpRequestAuthUnrecognized(raw)
		}
		if event.Negated {
			present = !present
		}
		if !c.Must(present) {
			return nil
		}
	}
	return nil
}

// httpInvalidTransferEncodingInRequest mirrors libhtp's request check: a
// Transfer-Encoding value must be exactly "chunked" and HTTP/1.0 and older
// requests cannot use chunked framing.
func httpInvalidTransferEncodingInRequest(raw []byte) bool {
	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		headerEnd = bytes.Index(raw, []byte("\n\n"))
	}
	if headerEnd < 0 {
		return false
	}
	lines := bytes.Split(raw[:headerEnd], []byte("\n"))
	if len(lines) == 0 {
		return false
	}
	requestLine := strings.Fields(strings.TrimSuffix(string(lines[0]), "\r"))
	if len(requestLine) < 3 || strings.HasPrefix(requestLine[0], "HTTP/") {
		return false
	}
	http11OrNewer := requestLine[len(requestLine)-1] != "HTTP/1.0" && requestLine[len(requestLine)-1] != "HTTP/0.9"
	chunked := false
	for _, line := range lines[1:] {
		key, value, ok := bytes.Cut(line, []byte(":"))
		if !ok || !bytes.EqualFold(bytes.TrimSpace(key), []byte("Transfer-Encoding")) {
			continue
		}
		value = bytes.TrimSpace(value)
		if !bytes.EqualFold(value, []byte("chunked")) {
			return true
		}
		chunked = true
	}
	return chunked && !http11OrNewer
}

// httpRequestAuthUnrecognized mirrors libhtp's Authorization scheme check:
// fires when an HTTP request carries an Authorization header whose scheme is
// empty or not a recognized standard HTTP authentication scheme (e.g., bare
// tokens or proprietary phishing kit headers).
func httpRequestAuthUnrecognized(raw []byte) bool {
	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		headerEnd = bytes.Index(raw, []byte("\n\n"))
	}
	if headerEnd < 0 {
		return false
	}
	lines := bytes.Split(raw[:headerEnd], []byte("\n"))
	if len(lines) == 0 {
		return false
	}
	requestLine := strings.Fields(strings.TrimSuffix(string(lines[0]), "\r"))
	if len(requestLine) < 3 || strings.HasPrefix(requestLine[0], "HTTP/") {
		return false
	}
	for _, line := range lines[1:] {
		key, value, ok := bytes.Cut(line, []byte(":"))
		if !ok || !bytes.EqualFold(bytes.TrimSpace(key), []byte("Authorization")) {
			continue
		}
		trimmed := strings.TrimSpace(string(value))
		if trimmed == "" {
			return true
		}
		scheme, _, hasRest := strings.Cut(trimmed, " ")
		if !hasRest {
			// Bare token without scheme (e.g. "Authorization: eyJhbG...") is
			// always unrecognized by libhtp.
			return true
		}
		switch strings.ToLower(strings.TrimSpace(scheme)) {
		case "basic", "digest", "bearer", "negotiate", "ntlm", "oauth",
			"hoba", "mutual", "scram-sha-1", "scram-sha-256", "vapid",
			"aws4-hmac-sha256", "dpop":
			continue
		default:
			return true
		}
	}
	return false
}
