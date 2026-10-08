package pcaputil

import (
	"errors"
	"fmt"
	"sort"
)

// h3StreamState tracks RFC 9114 framing and QPACK field sections on a QUIC stream.
// Native admission requires authenticated ALPN; port 443 is never consulted.
type h3StreamState struct {
	blocked      [2]bool
	blockedPDU   [2]uint64
	seenSettings bool
	headers      [2]int

	kind                  string
	seenType              bool
	buf                   [2][]byte
	parsed                [2]int
	reqHeaders, respHeads int
	dataBytes             uint64
	fin                   [2]bool
}

func (q *binQUIC) feedHTTP3(dir int, sid, off uint64, data []byte, fin bool, max int, fr, info map[string]any) error {
	st := q.stream(sid, max)
	if st == nil {
		return nil
	}
	if st.h3 == nil {
		st.h3 = &h3StreamState{}
	}
	h3 := st.h3
	if dir != 0 && dir != 1 {
		dir = 0
	}
	if q.native && fin {
		h3.fin[dir] = true
	}
	if off != uint64(len(h3.buf[dir])) {
		return nil
	}
	if len(h3.buf[dir])+len(data) > 1<<20 {
		return protocolError(ErrResourceExceeded, "HTTP/3 stream exceeds 1 MiB")
	}
	h3.buf[dir] = append(h3.buf[dir], data...)
	uni := sid&2 != 0
	buf := h3.buf[dir][h3.parsed[dir]:]
	if uni && !h3.seenType {
		if len(buf) == 0 {
			return nil
		}
		typ, n, err := quicVarint(buf)
		if quicNeedMore(err) {
			return nil
		}
		if err != nil {
			st.h3 = nil
			return nil
		}
		h3.seenType = true
		h3.parsed[dir] += n
		switch typ {
		case 0:
			h3.kind = "control"
			if q.native {
				if q.controlSeen[dir] {
					return protocolError(ErrMalformedMessage, "HTTP3 duplicate control stream")
				}
				q.controlSeen[dir] = true
			}
		case 1:
			h3.kind = "push"
		case 2:
			h3.kind = "encoder"
			if q.native {
				if q.qpackSeen[dir][0] {
					return protocolError(ErrMalformedMessage, "HTTP3 duplicate QPACK encoder stream")
				}
				q.qpackSeen[dir][0] = true
			}
		case 3:
			h3.kind = "decoder"
			if q.native {
				if q.qpackSeen[dir][1] {
					return protocolError(ErrMalformedMessage, "HTTP3 duplicate QPACK decoder stream")
				}
				q.qpackSeen[dir][1] = true
			}
		default:
			h3.kind = "unknown"
		}
		buf = h3.buf[dir][h3.parsed[dir]:]
	}
	if q.native && fin && (h3.kind == "control" || h3.kind == "encoder" || h3.kind == "decoder") {
		return protocolError(ErrMalformedMessage, "HTTP3 critical stream closed")
	}
	if h3.kind == "encoder" {
		n, inst, err := q.qpack.applyEncoder(dir, buf, max)
		if err != nil {
			return err
		}
		h3.parsed[dir] += n
		info["HTTP3"] = true
		info["Protocol Transition"] = "quic->http3"
		info["HTTP3 Stream Kind"] = "encoder"
		info["HTTP3 Stream ID"] = sid
		fr["HTTP3"] = true
		fr["HTTP3 Stream Kind"] = "encoder"
		if len(inst) > 0 {
			fr["QPACK Instructions"] = inst
			info["QPACK Instructions"] = inst
			info["QPACK Insert Count"] = q.qpack.table[dir].insertCount
			info["QPACK Table Size"] = q.qpack.table[dir].size
			info["QPACK Capacity"] = q.qpack.table[dir].capacity
		}

		if q.native {
			ids := make([]uint64, 0)
			for id, other := range q.streams {
				if other.h3 != nil && other.h3.blocked[dir] {
					ids = append(ids, id)
				}
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			var resumed []map[string]any
			for _, id := range ids {
				other := q.streams[id].h3
				sf, si := map[string]any{}, map[string]any{}
				err := q.feedHTTP3(dir, id, uint64(len(other.buf[dir])), nil, other.fin[dir], max, sf, si)
				if err != nil {
					return err
				}
				if !other.blocked[dir] {
					si["Blocked PDU ID"] = other.blockedPDU[dir]
					si["Transaction ID"] = q.streams[id].requestPDU
					resumed = append(resumed, si)
				}
			}
			if len(resumed) > 0 {
				info["QPACK Resumed Streams"] = resumed
			}
		}
		return nil
	}
	if h3.kind == "decoder" {
		n, inst, err := q.qpack.applyDecoder(buf, max)
		if err != nil {
			return err
		}
		h3.parsed[dir] += n
		info["HTTP3"] = true
		info["Protocol Transition"] = "quic->http3"
		info["HTTP3 Stream Kind"] = "decoder"
		info["HTTP3 Stream ID"] = sid
		fr["HTTP3"] = true
		fr["HTTP3 Stream Kind"] = "decoder"
		if len(inst) > 0 {
			fr["QPACK Instructions"] = inst
			info["QPACK Instructions"] = inst
		}
		return nil
	}
	if q.native && uni && (h3.kind == "unknown" || h3.kind == "push") {
		info["HTTP3 Stream Kind"] = h3.kind
		return nil
	}
	var frames []map[string]any
	for len(buf) > 0 {
		ft, n1, err := quicVarint(buf)
		if quicNeedMore(err) {
			break
		}
		if err != nil {
			if h3.kind == "request" || h3.kind == "control" {
				return fmt.Errorf("http3: malformed frame type")
			}
			break
		}
		ln, n2, err := quicVarint(buf[n1:])
		if quicNeedMore(err) {
			break
		}
		if err != nil {
			if h3.kind == "request" || h3.kind == "control" {
				return fmt.Errorf("http3: malformed frame length")
			}
			break
		}
		need := n1 + n2 + int(ln)
		if ln > 1<<20 {
			return protocolError(ErrResourceExceeded, "HTTP/3 frame exceeds 1 MiB")
		}
		if len(buf) < need {
			break
		}
		payload := buf[n1+n2 : need]
		if q.native {
			if h3.kind == "control" && !h3.seenSettings && ft != 4 {
				return protocolError(ErrMalformedMessage, "HTTP3 SETTINGS must be first")
			}
			if h3.kind == "control" && h3.seenSettings && ft == 4 {
				return protocolError(ErrMalformedMessage, "HTTP3 duplicate SETTINGS")
			}
			if !uni && ft == 0 && h3.headers[dir] != 1 {
				return protocolError(ErrMalformedMessage, "HTTP3 DATA outside message body")
			}
			if !uni && ft == 4 {
				return protocolError(ErrMalformedMessage, "HTTP3 SETTINGS on request stream")
			}
		}
		hf, herr := q.h3DecodeFrame(dir, ft, payload, h3, uni)
		if herr != nil {
			var pe *ProtocolError
			if q.native && errors.As(herr, &pe) && pe.Kind == ErrContextRequired {
				blocked := 0
				for _, other := range q.streams {
					if other.h3 != nil && other.h3.blocked[dir] {
						blocked++
					}
				}
				if !h3.blocked[dir] && (blocked >= max || uint64(blocked) >= q.qpack.maxBlocked[dir]) {
					return protocolError(ErrResourceExceeded, "QPACK blocked stream limit")
				}
				if !h3.blocked[dir] {
					h3.blockedPDU[dir] = q.eventID
				}
				h3.blocked[dir] = true
				info["QPACK Blocked"] = true
				info["HTTP3 Stream ID"] = sid
				info["HTTP3"] = true
				return nil
			}
			if h3.kind == "request" || h3.kind == "control" {
				return herr
			}
			if h3.kind == "" && (ft == 0x01 || ft == 0x04) {
				return herr
			}
			break
		}
		if h3.kind == "" {
			if uni || ft != 0x01 {
				break
			}
			h3.kind = "request"
		}
		if ft == 4 {
			h3.seenSettings = true
		}
		h3.blocked[dir] = false
		informational := false
		if ft == 1 && dir == 1 {
			if headers, ok := hf["Headers"].([]map[string]any); ok {
				for _, header := range headers {
					status, _ := header["Value"].(string)
					if header["Name"] == ":status" && len(status) == 3 && status[0] == '1' {
						if status == "101" || h3.headers[dir] != 0 {
							return protocolError(ErrMalformedMessage, "HTTP3 invalid informational response")
						}
						informational = true
					}
				}
			}
		}
		if informational {
			hf["Header Kind"] = "informational"
		} else if ft == 0x01 {
			h3.headers[dir]++
			if dir == 0 {
				h3.reqHeaders++
				hf["Header Kind"] = "request"
			} else {
				h3.respHeads++
				hf["Header Kind"] = "response"
				if h3.reqHeaders > 0 {
					hf["Associated Request"] = true
				}
			}
		}
		if ft == 1 && h3.headers[dir] > 1 {
			hf["Header Kind"] = "trailers"
		}
		if ft == 0x00 {
			h3.dataBytes += uint64(len(payload))
			hf["DATA Length"] = uint64(len(payload))
		}
		frames = append(frames, hf)
		h3.parsed[dir] += need
		buf = h3.buf[dir][h3.parsed[dir]:]
		if len(frames) >= max {
			return protocolError(ErrResourceExceeded, "HTTP/3 frame budget exceeded")
		}
	}
	if q.native && fin && !uni && dir == 1 && h3.headers[dir] == 0 && !h3.blocked[dir] {
		return protocolError(ErrMalformedMessage, "HTTP3 FIN before final response headers")
	}
	if q.native && fin && h3.parsed[dir] != len(h3.buf[dir]) {
		return protocolError(ErrNeedMore, "HTTP3 FIN with incomplete frame")
	}
	if h3.kind == "" || h3.kind == "unknown" {
		if len(frames) == 0 {
			return nil
		}
	}
	if len(frames) == 0 && h3.kind != "control" && h3.kind != "encoder" && h3.kind != "decoder" && h3.kind != "request" {
		return nil
	}
	if h3.kind == "control" || h3.kind == "request" || h3.kind == "encoder" || h3.kind == "decoder" || len(frames) > 0 {
		info["HTTP3"] = true
		info["Protocol Transition"] = "quic->http3"
		info["HTTP3 Stream ID"] = sid
		info["HTTP3 Stream Kind"] = h3.kind
		fr["HTTP3"] = true
		fr["HTTP3 Stream Kind"] = h3.kind
		if len(frames) > 0 {
			fr["HTTP3 Frames"] = frames
			info["HTTP3 Frames"] = frames
			if hs, ok := frames[0]["Headers"].([]map[string]any); ok {
				info["Headers"] = hs
			}
			names := make([]string, len(frames))
			for i, hf := range frames {
				n, _ := hf["Frame Type"].(string)
				names[i] = n
			}
			info["HTTP3 Frame Types"] = names
		}
		if fin {
			if dir == 0 || dir == 1 {
				h3.fin[dir] = true
			}
			state := "half-closed"
			if h3.fin[0] && h3.fin[1] {
				state = "closed"
			}
			info["HTTP3 Stream State"] = state
			fr["HTTP3 Stream State"] = state
		}
	}
	return nil
}

func (q *binQUIC) h3DecodeFrame(dir int, ft uint64, payload []byte, h3 *h3StreamState, uni bool) (map[string]any, error) {
	switch ft {
	case 0x00:
		if h3.kind == "control" {
			return nil, protocolError(ErrMalformedMessage, "http3: DATA is not permitted on the control stream")
		}
		if h3.kind == "request" && h3.reqHeaders == 0 && h3.respHeads == 0 {
			return nil, protocolError(ErrMalformedMessage, "http3: DATA before HEADERS")
		}
		return map[string]any{"Frame Type": "DATA", "Payload": append([]byte(nil), payload...)}, nil
	case 0x01:
		if h3.kind == "control" {
			return nil, protocolError(ErrMalformedMessage, "http3: HEADERS is not permitted on the control stream")
		}
		out := map[string]any{
			"Frame Type":  "HEADERS",
			"Length":      uint64(len(payload)),
			"QPACK Block": append([]byte(nil), payload...),
		}
		headers, meta, err := q.qpack.decodeSection(dir, payload)
		if err != nil {
			return nil, err
		}
		out["Headers"] = headers
		for k, v := range meta {
			out[k] = v
		}
		return out, nil
	case 0x04:
		if h3.kind == "request" {
			return nil, protocolError(ErrMalformedMessage, "http3: SETTINGS is not permitted on a request stream")
		}
		settings, err := h3ParseSettings(payload)
		if err != nil {
			return nil, err
		}
		peer := 1 - dir
		if dir != 0 && dir != 1 {
			peer = 1
		}
		for _, s := range settings {
			id, _ := s["ID"].(uint64)
			val, _ := s["Value"].(uint64)
			if id == 7 {
				q.qpack.maxBlocked[peer] = val
			}
			if id == 0x01 {
				q.qpack.maxCap[peer] = val
			}
		}
		return map[string]any{"Frame Type": "SETTINGS", "Settings": settings}, nil
	case 0x07:
		id, n, err := quicVarint(payload)
		if err != nil || n != len(payload) {
			return nil, fmt.Errorf("http3: malformed GOAWAY")
		}
		return map[string]any{"Frame Type": "GOAWAY", "Last Stream ID": id}, nil
	case 0x03:
		return map[string]any{"Frame Type": "CANCEL_PUSH", "Payload": append([]byte(nil), payload...)}, nil
	case 0x05:
		return map[string]any{"Frame Type": "PUSH_PROMISE", "Length": uint64(len(payload))}, nil
	case 0x0d:
		id, n, err := quicVarint(payload)
		if err != nil || n != len(payload) {
			return nil, fmt.Errorf("http3: malformed MAX_PUSH_ID")
		}
		return map[string]any{"Frame Type": "MAX_PUSH_ID", "Push ID": id}, nil
	default:
		_ = uni
		return map[string]any{"Frame Type": "Unknown", "Type ID": ft, "Length": uint64(len(payload))}, nil
	}
}

func h3ParseSettings(payload []byte) ([]map[string]any, error) {
	var out []map[string]any
	seen := map[uint64]bool{}
	off := 0
	for off < len(payload) {
		id, n, err := quicVarint(payload[off:])
		if err != nil {
			return nil, fmt.Errorf("http3: truncated SETTINGS identifier")
		}
		off += n
		val, n, err := quicVarint(payload[off:])
		if err != nil {
			return nil, fmt.Errorf("http3: truncated SETTINGS value")
		}
		off += n
		if seen[id] {
			return nil, fmt.Errorf("http3: duplicate SETTINGS identifier")
		}
		seen[id] = true
		if id >= 2 && id <= 5 {
			return nil, fmt.Errorf("http3: prohibited HTTP2 SETTINGS identifier")
		}
		name := h3SettingName(id)
		out = append(out, map[string]any{"ID": id, "Name": name, "Value": val})
	}
	return out, nil
}

func h3SettingName(id uint64) string {
	switch id {
	case 0x01:
		return "QPACK_MAX_TABLE_CAPACITY"
	case 0x06:
		return "MAX_FIELD_SECTION_SIZE"
	case 0x07:
		return "QPACK_BLOCKED_STREAMS"
	default:
		return "unknown"
	}
}

func h3Frame(typ uint64, payload []byte) []byte {
	b := quicPutVarint(typ)
	b = append(b, quicPutVarint(uint64(len(payload)))...)
	return append(b, payload...)
}

func h3ControlSETTINGS(idsAndVals ...uint64) []byte {
	var p []byte
	for i := 0; i+1 < len(idsAndVals); i += 2 {
		p = append(p, quicPutVarint(idsAndVals[i])...)
		p = append(p, quicPutVarint(idsAndVals[i+1])...)
	}
	return append([]byte{0x00}, h3Frame(0x04, p)...)
}

func h3RequestHeaders() []byte {
	block := []byte{0, 0, 0xd1, 0xd7, 0xc1, 0x50, 11}
	block = append(block, []byte("example.com")...)
	return h3Frame(0x01, block)
}

func h3ResponseHeaders() []byte {
	return h3Frame(0x01, []byte{0, 0, 0xd9})
}

func h3Data(body []byte) []byte {
	return h3Frame(0x00, body)
}
