package match

import (
	"bytes"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// smtpParser exposes the SMTP DATA body as file data. It intentionally does
// not decode MIME transfer encodings; it is a compatibility buffer for rules
// that scan the raw DATA payload.
func smtpParser(c *matchContext) error {
	tcp, ok := c.PK.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !ok {
		c.Reject()
		return nil
	}
	raw := tcp.Payload
	if len(c.overridePayload) > 0 {
		raw = c.overridePayload
	}
	data := extractSMTPData(raw)
	c.SetBufferProvider(func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.Default:
			return raw
		case modifier.FileData:
			return data
		case modifier.TCPHDR:
			return tcp.Contents
		}
		return nil
	})
	return nil
}

func extractSMTPData(raw []byte) []byte {
	offset := 0
	for offset < len(raw) {
		lineEnd := bytes.IndexByte(raw[offset:], '\n')
		if lineEnd < 0 {
			return nil
		}
		line := bytes.TrimSuffix(raw[offset:offset+lineEnd], []byte("\r"))
		if bytes.EqualFold(bytes.TrimSpace(line), []byte("DATA")) {
			body := raw[offset+lineEnd+1:]
			if index := bytes.Index(body, []byte("\r\n.\r\n")); index >= 0 {
				return body[:index]
			}
			if index := bytes.Index(body, []byte("\n.\n")); index >= 0 {
				return body[:index]
			}
			return nil
		}
		offset += lineEnd + 1
	}
	return nil
}
