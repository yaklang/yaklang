package pcaputil

import (
	"encoding/binary"
	"strings"
)

func (f *binFlow) consumeGRPC(dir int, e *ProtocolEvent) {
	h := f.h2
	if h == nil || e.Session == nil {
		return
	}
	headers, _ := e.Session["Headers"].([]map[string]any)
	for _, hdr := range headers {
		name, _ := hdr["Name"].(string)
		value, _ := hdr["Value"].(string)
		if strings.EqualFold(name, "content-type") && strings.HasPrefix(strings.ToLower(value), "application/grpc") {
			e.Session["GRPC"] = true
			e.Session["GRPC Content Type"] = value
		}
		if strings.EqualFold(name, "grpc-status") {
			e.Session["GRPC"] = true
			e.Session["GRPC Status"] = value
		}
		if strings.EqualFold(name, "grpc-message") {
			e.Session["GRPC Message"] = value
		}
	}
	id, _ := e.Session["Stream ID"].(uint32)
	stream := h.streams[id]
	typ, _ := e.Session["Frame Type"].(byte)
	if typ == 0 && stream != nil {
		payload := grpcDATAPayload(e.Raw)
		if len(stream.grpc)+len(payload) > f.a.config.MaxMessageBytes {
			e.Session["GRPC"] = true
			e.Error = "gRPC buffered DATA exceeds message limit"
			return
		}
		stream.grpc = append(stream.grpc, payload...)
		msgs, rest := splitGRPCMessages(stream.grpc)
		stream.grpc = rest
		if len(msgs) > 0 {
			e.Session["GRPC"] = true
			e.Session["GRPC Messages"] = msgs
		}
	}
	_ = dir
}

func grpcDATAPayload(raw []byte) []byte {
	if len(raw) >= 24 && string(raw[:24]) == binH2Preface {
		raw = raw[24:]
	}
	if len(raw) < 9 || raw[3] != 0 {
		return nil
	}
	payload := raw[9:]
	if raw[4]&0x08 != 0 && len(payload) > 0 {
		n := int(payload[0])
		if n < len(payload) {
			payload = payload[1 : len(payload)-n]
		}
	}
	return append([]byte(nil), payload...)
}

func splitGRPCMessages(w []byte) ([]map[string]any, []byte) {
	var out []map[string]any
	for len(w) >= 5 {
		n := int(binary.BigEndian.Uint32(w[1:5]))
		if n < 0 || 5+n > len(w) {
			return out, w
		}
		out = append(out, map[string]any{
			"Compressed": w[0] != 0,
			"Length":     uint64(n),
			"Payload":    append([]byte(nil), w[5:5+n]...),
		})
		w = w[5+n:]
	}
	return out, w
}
