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

// appLayerProtocolMatcher evaluates app-layer-protocol, which Suricata applies
// to the protocol negotiated by the application parser. HTTP is the only
// negotiated protocol exposed by this matcher today.
func appLayerProtocolMatcher(c *matchContext) error {
	config := c.Rule.ContentRuleConfig
	if config == nil || config.AppLayerProtocol == "" {
		return nil
	}
	if config.AppLayerProtocol != "http" {
		c.Reject()
		return nil
	}
	detected := looksLikeHTTP(c.GetBuffer(modifier.Default))
	if config.AppLayerProtocolNegated {
		detected = !detected
	}
	c.Must(detected)
	return nil
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
