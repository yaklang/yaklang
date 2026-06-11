package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeHTTPFuzzRequest,
		func() AttachedResourceData { return &AttachedHTTPFuzzRequestData{} },
		"httpfuzzrequest",
		"http_packet",
		"httppacket",
	)
}

type AttachedHTTPFuzzRequestData struct {
	Raw     string
	Packet  string
	IsHTTPS bool
}

func (d *AttachedHTTPFuzzRequestData) Type() string {
	return AttachedResourceTypeHTTPFuzzRequest
}

func (d *AttachedHTTPFuzzRequestData) Unmarshal(raw string) error {
	raw = strings.TrimSpace(raw)
	d.Raw = raw
	if raw == "" {
		return utils.Error("http fuzz request packet is empty")
	}
	if !strings.HasPrefix(raw, "{") {
		d.Packet = raw
		return nil
	}

	var payload struct {
		HTTPFuzzRequest string `json:"http_fuzz_request"`
		HTTPPacket      string `json:"http_packet"`
		HTTPPacketAlt   string `json:"httppacket"`
		HTTPRequest     string `json:"http_request"`
		Packet          string `json:"packet"`
		Content         string `json:"content"`
		Raw             string `json:"raw"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		d.Packet = raw
		return nil
	}
	for _, candidate := range []string{
		payload.HTTPFuzzRequest,
		payload.HTTPPacket,
		payload.HTTPPacketAlt,
		payload.HTTPRequest,
		payload.Packet,
		payload.Content,
		payload.Raw,
	} {
		if packet := strings.TrimSpace(candidate); packet != "" {
			d.Packet = packet
			break
		}
	}
	if strings.TrimSpace(d.Packet) == "" {
		d.Packet = raw
	}
	d.IsHTTPS = parseAttachedHTTPSFlag(raw)
	return nil
}

func (d *AttachedHTTPFuzzRequestData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedHTTPFuzzRequestData) ToAttachData(reactloop ReActLoopIF) string {
	var emitter *Emitter
	if reactloop != nil {
		emitter = reactloop.GetEmitter()
	}
	return FormatAttachedHTTPFuzzRequest(d.Packet, d.IsHTTPS, emitter)
}

func parseAttachedBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "https", "tls":
		return true
	default:
		return false
	}
}

func parseAttachedHTTPSFlag(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	for _, key := range []string{"is_https", "https", "tls"} {
		switch v := payload[key].(type) {
		case bool:
			if v {
				return true
			}
		case string:
			if parseAttachedBool(v) {
				return true
			}
		case float64:
			if v != 0 {
				return true
			}
		}
	}
	return false
}

func FormatAttachedHTTPFuzzRequest(packet string, isHTTPS bool, emitter *Emitter) string {
	packet = strings.TrimSpace(packet)
	var b strings.Builder
	b.WriteString("## Attached HTTP Fuzz Request\n\n")
	b.WriteString("- Resource Type: http_fuzz_request\n")
	b.WriteString(fmt.Sprintf("- IsHTTPS: %t\n", isHTTPS))
	b.WriteString("- Usage: Use this raw HTTP packet as the current target request for HTTP fuzz testing.\n\n")
	if packet == "" {
		b.WriteString("(empty HTTP packet)")
		return strings.TrimSpace(b.String())
	}

	inline, spillNote := inlineOrSpillAttachedText("http_fuzz_request", packet, AttachedHTTPPacketInlineLimit, emitter)
	b.WriteString("### HTTP Packet\n")
	if spillNote != "" {
		b.WriteString(spillNote)
		b.WriteString("\n\nInline preview:\n```\n")
		b.WriteString(inline)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("```\n")
		b.WriteString(inline)
		b.WriteString("\n```\n")
	}
	return strings.TrimSpace(b.String())
}
