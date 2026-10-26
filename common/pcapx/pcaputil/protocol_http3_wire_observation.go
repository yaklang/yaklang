package pcaputil

// Preserve a stream's observed application identity when reassembly delivers no
// new contiguous bytes. This is a wire observation, never another decoded frame.
// Missing prefixes on a new stream/connection cannot borrow another's identity.
func (q *binQUIC) observeKnownHTTP3Wire(st *quicStream, dir int, sid, off uint64, length int, fr, info map[string]any) {
	if q.alpn != "h3" || st.h3 == nil || length == 0 {
		return
	}
	h3 := st.h3
	known := h3.kind == "request" || h3.kind == "control" || h3.kind == "encoder" || h3.kind == "decoder" || h3.blocked[dir] || h3.framingFailure[dir] || h3.fieldFailure[dir] != ""
	if !known {
		return
	}
	kind := "buffered-range"
	if off+uint64(length) <= uint64(len(st.reassembly[dir].data)) {
		kind = "retransmission"
	}
	fr["HTTP3"], info["HTTP3"] = true, true
	fr["HTTP3 Wire Observation"] = kind
	if previous, ok := info["HTTP3 Wire Observation"].(string); ok && previous != kind {
		info["HTTP3 Wire Observation"] = "mixed"
	} else {
		info["HTTP3 Wire Observation"] = kind
	}
	fr["HTTP3 Stream Kind"] = h3.kind
	// A later duplicate or buffered range must not replace metadata belonging
	// to a newly decoded application frame elsewhere in this packet.
	if _, exists := info["HTTP3 Stream ID"]; !exists {
		info["HTTP3 Stream ID"], info["HTTP3 Stream Kind"] = sid, h3.kind
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
