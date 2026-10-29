package pcaputil

// Preserve application identity when reassembly delivers no new contiguous
// bytes. Authenticated h3 ALPN also identifies a client bidirectional stream's
// connection before its prefix arrives, but not its HTTP message or extension.
// This projection never creates framing state or replays an application frame.
func (q *binQUIC) observeKnownHTTP3Wire(st *quicStream, dir int, sid, off uint64, length int, fr, info map[string]any) {
	if q.alpn != "h3" || length == 0 {
		return
	}
	h3 := st.h3
	known := h3 != nil && (h3.kind == "request" || h3.kind == "control" || h3.kind == "encoder" || h3.kind == "decoder" || h3.blocked[dir] || h3.framingFailure[dir] || h3.fieldFailure[dir] != "")
	kind := "buffered-range"
	streamKind := "client-bidirectional"
	if !known {
		// RFC 9114 section 6.1 defines the base request-stream role. Missing
		// bytes cannot distinguish its framing from an extension such as
		// WebTransport, nor identify an unidirectional stream's type.
		if !q.native || sid&3 != 0 || len(st.reassembly[dir].data) != 0 {
			return
		}
		kind = "prefix-missing"
	} else if off+uint64(length) <= uint64(len(st.reassembly[dir].data)) {
		kind = "retransmission"
	}
	if known {
		streamKind = h3.kind
	}
	fr["HTTP3"], info["HTTP3"] = true, true
	fr["HTTP3 Wire Observation"] = kind
	if previous, ok := info["HTTP3 Wire Observation"].(string); ok && previous != kind {
		info["HTTP3 Wire Observation"] = "mixed"
	} else {
		info["HTTP3 Wire Observation"] = kind
	}
	fr["HTTP3 Stream Kind"] = streamKind
	// A later duplicate or buffered range must not replace metadata belonging
	// to a newly decoded application frame elsewhere in this packet.
	if _, exists := info["HTTP3 Stream ID"]; !exists {
		info["HTTP3 Stream ID"], info["HTTP3 Stream Kind"] = sid, streamKind
	}
	info["Protocol Transition"] = "quic->http3"
}

func markHTTP3WireOnly(frames []map[string]any, info map[string]any) {
	if info["HTTP3 Wire Observation"] == nil {
		return
	}
	only := true
	for _, fr := range frames {
		if rows, _ := fr["HTTP3 Frames"].([]map[string]any); len(rows) > 0 {
			only = false
		}
	}
	if resumed, _ := info["QPACK Resumed Streams"].([]map[string]any); len(resumed) > 0 {
		for _, stream := range resumed {
			if rows, _ := stream["HTTP3 Frames"].([]map[string]any); len(rows) > 0 {
				only = false
			}
		}
	}
	info["HTTP3 Wire Only"] = only
}
