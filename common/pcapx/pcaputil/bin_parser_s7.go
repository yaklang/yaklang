package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type s7Key struct {
	dir int
	ref uint16
}
type s7Request struct {
	function byte
	count    int
}
type binS7 struct {
	pending   map[s7Key]s7Request
	fragments [2][]byte
	pdu       uint16
}

func probeS7(w []byte, _ int) ProbeResult {
	if len(w) == 0 || w[0] != 3 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 7 {
		return probeNeed("s7comm", "s7", len(w), 7)
	}
	n := int(binary.BigEndian.Uint16(w[2:]))
	if w[1] != 0 || n < 7 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[5] == 0xf0 && w[4] == 2 {
		if len(w) < 8 {
			return probeNeed("s7comm", "s7", len(w), 8)
		}
		if w[7] == 0x32 {
			return probeAccept("s7comm", "s7", 98)
		}
	}
	if w[5]&0xf0 == 0xe0 || w[5]&0xf0 == 0xd0 {
		if n > 64 || n < 13 {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) < n {
			return probeNeed("s7comm", "s7", len(w), n)
		}
		if int(w[4])+5 != n {
			return ProbeResult{Verdict: ProbeReject}
		}
		p := w[11:n]
		src, dst := false, false
		for len(p) >= 2 {
			l := int(p[1])
			if l+2 > len(p) {
				return ProbeResult{Verdict: ProbeReject}
			}
			if l == 2 && p[2] >= 1 && p[2] <= 3 {
				if p[0] == 0xc1 {
					src = true
				}
				if p[0] == 0xc2 {
					dst = true
				}
			}
			p = p[2+l:]
		}
		if len(p) == 0 && src && dst {
			return probeAccept("s7comm", "s7", 98)
		}
	}
	return ProbeResult{Verdict: ProbeReject}
}
func (f *binFlow) frameS7(w []byte) (int, *binSpec, error) {
	s := f.s7
	reserve := 512 + int64(len(s.pending)+1)*64 + int64(len(s.fragments[0])+len(s.fragments[1])+len(w))
	if err := f.reserveSession(reserve); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	n := int(binary.BigEndian.Uint16(w[2:]))
	if w[0] != 3 || w[1] != 0 || n < 7 {
		return 0, nil, fmt.Errorf("s7comm: TPKT header")
	}
	return n, f.a.specs["session_envelopes/TPKT"], nil
}
func s7Items(p []byte, count, max int) ([]map[string]any, error) {
	if count > max {
		return nil, protocolError(ErrResourceExceeded, "S7 items")
	}
	items := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		if len(p) < 12 || p[0] != 0x12 || p[1] != 10 || p[2] != 0x10 {
			return nil, protocolError(ErrUnsupportedFeature, "S7 non-S7ANY variable specification")
		}
		address := diameter24(p[9:])
		items = append(items, map[string]any{"Transport Size": p[3], "Element Count": binary.BigEndian.Uint16(p[4:]), "DB": binary.BigEndian.Uint16(p[6:]), "Area": p[8], "Bit Address": address, "Byte Address": address / 8, "Bit": address % 8})
		p = p[12:]
	}
	if len(p) != 0 {
		return nil, fmt.Errorf("s7comm: parameter trailing bytes")
	}
	return items, nil
}
func s7Data(p []byte, count, max int, writeResponse bool) ([]map[string]any, error) {
	if count > max {
		return nil, protocolError(ErrResourceExceeded, "S7 data count")
	}
	out := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		if writeResponse {
			if len(p) < 1 {
				return nil, fmt.Errorf("s7comm: write result")
			}
			out = append(out, map[string]any{"Return Code": p[0]})
			p = p[1:]
			continue
		}
		if len(p) < 4 {
			return nil, fmt.Errorf("s7comm: data header")
		}
		n := int(binary.BigEndian.Uint16(p[2:]))
		size := n
		switch p[1] {
		case 3, 4, 5:
			size = (n + 7) / 8
		case 0, 6, 7, 9:
		default:
			return nil, protocolError(ErrUnsupportedFeature, "S7 data transport size")
		}
		// Error items retain the four-byte header but carry no value.
		if p[0] != 0 && p[0] != 0xff && p[1] == 0 {
			size = 0
		}
		if size > len(p)-4 {
			return nil, fmt.Errorf("s7comm: data length")
		}
		out = append(out, map[string]any{"Return Code": p[0], "Transport Size": p[1], "Length": n, "Value": append([]byte(nil), p[4:4+size]...)})
		p = p[4+size:]
		if i+1 < count && size%2 == 1 {
			if len(p) < 1 {
				return nil, fmt.Errorf("s7comm: item padding")
			}
			p = p[1:]
		}
	}
	if len(p) != 0 {
		return nil, fmt.Errorf("s7comm: data trailing bytes")
	}
	return out, nil
}
func (s *binS7) consume(dir int, w []byte, max, bytesMax int) (map[string]any, error) {
	if len(w) < 7 || int(binary.BigEndian.Uint16(w[2:])) != len(w) {
		return nil, fmt.Errorf("s7comm: TPKT")
	}
	li := int(w[4])
	if li < 2 || li+5 > len(w) {
		return nil, fmt.Errorf("s7comm: COTP length")
	}
	code := w[5] & 0xf0
	out := map[string]any{"COTP Type": code}
	if code == 0xe0 || code == 0xd0 {
		if li < 6 || li+5 != len(w) {
			return nil, fmt.Errorf("s7comm: COTP connection header")
		}
		p := w[11:]
		params := []map[string]any{}
		for len(p) > 0 {
			if len(params) >= max {
				return nil, protocolError(ErrResourceExceeded, "COTP parameters")
			}
			if len(p) < 2 || int(p[1])+2 > len(p) {
				return nil, fmt.Errorf("s7comm: COTP parameter")
			}
			n := int(p[1])
			params = append(params, map[string]any{"Code": p[0], "Value": append([]byte(nil), p[2:2+n]...)})
			p = p[2+n:]
		}
		out["Packet Name"] = map[byte]string{0xe0: "Connection Request", 0xd0: "Connection Confirm"}[code]
		out["Parameters"] = params
		return out, nil
	}
	if code != 0xf0 || li != 2 {
		return nil, protocolError(ErrUnsupportedFeature, "S7 COTP TPDU")
	}
	body := w[7:]
	if len(s.fragments[dir])+len(body) > bytesMax {
		return nil, protocolError(ErrResourceExceeded, "S7 reassembled PDU")
	}
	if w[6]&128 == 0 {
		s.fragments[dir] = append(s.fragments[dir], body...)
		out["Packet Name"] = "COTP Fragment"
		out["Buffered Bytes"] = len(s.fragments[dir])
		return out, nil
	}
	if len(s.fragments[dir]) > 0 {
		body = append(s.fragments[dir], body...)
		s.fragments[dir] = nil
		out["Reassembled"] = true
	}
	if len(body) < 10 || body[0] != 0x32 {
		return nil, fmt.Errorf("s7comm: S7 header")
	}
	kind := body[1]
	h := 10
	if kind == 2 || kind == 3 {
		h = 12
	}
	if len(body) < h {
		return nil, fmt.Errorf("s7comm: ack header")
	}
	pl, dl := int(binary.BigEndian.Uint16(body[6:])), int(binary.BigEndian.Uint16(body[8:]))
	if h+pl+dl != len(body) {
		return nil, fmt.Errorf("s7comm: parameter/data boundary")
	}
	if s.pdu != 0 && len(body) > int(s.pdu) {
		return nil, protocolError(ErrResourceExceeded, "S7 negotiated PDU")
	}
	ref := binary.BigEndian.Uint16(body[4:])
	out["ROSCTR"] = kind
	out["PDU Reference"] = ref
	if h == 12 {
		out["Error Class"] = body[10]
		out["Error Code"] = body[11]
	}
	p, data := body[h:h+pl], body[h+pl:]
	out["Packet Name"] = "S7 PDU"
	if kind == 7 {
		out["Semantic Status"] = "unsupported-userdata-service"
		out["Parameter"] = append([]byte(nil), p...)
		out["Data"] = append([]byte(nil), data...)
		return out, nil
	}
	if kind != 1 && kind != 2 && kind != 3 {
		return nil, protocolError(ErrUnsupportedFeature, "S7 ROSCTR")
	}
	var function byte
	count := 0
	if len(p) > 0 {
		function = p[0]
	}
	out["Function"] = function
	if function == 0xf0 {
		if len(p) != 8 || len(data) != 0 {
			return nil, fmt.Errorf("s7comm: setup communication")
		}
		out["Max AMQ Calling"] = binary.BigEndian.Uint16(p[2:])
		out["Max AMQ Called"] = binary.BigEndian.Uint16(p[4:])
		out["PDU Length"] = binary.BigEndian.Uint16(p[6:])
		if kind == 3 {
			if binary.BigEndian.Uint16(p[6:]) < 10 {
				return nil, fmt.Errorf("s7comm: negotiated PDU size")
			}
			s.pdu = binary.BigEndian.Uint16(p[6:])
		}
	}
	if function == 4 || function == 5 {
		if len(p) < 2 {
			return nil, fmt.Errorf("s7comm: item count")
		}
		count = int(p[1])
		if kind == 1 {
			items, err := s7Items(p[2:], count, max)
			if err != nil {
				return nil, err
			}
			out["Items"] = items
			if function == 4 && len(data) != 0 {
				return nil, fmt.Errorf("s7comm: read request data")
			}
		} else if len(p) != 2 {
			return nil, fmt.Errorf("s7comm: response parameter")
		}
		if kind != 1 || function == 5 {
			items, err := s7Data(data, count, max, kind != 1 && function == 5)
			if err != nil {
				return nil, err
			}
			out["Data Items"] = items
		}
	}
	if function != 0 && function != 4 && function != 5 && function != 0xf0 {
		out["Semantic Status"] = "unsupported-function"
		out["Parameter"] = bytes.Clone(p)
		out["Data"] = bytes.Clone(data)
	}
	if s.pending == nil {
		s.pending = map[s7Key]s7Request{}
	}
	key := s7Key{dir, ref}
	if kind == 1 {
		if _, ok := s.pending[key]; ok {
			return nil, protocolError(ErrDesynchronized, "S7 PDU reference reused")
		}
		if len(s.pending) >= max {
			return nil, protocolError(ErrResourceExceeded, "S7 pending")
		}
		s.pending[key] = s7Request{function, count}
	} else {
		key.dir = 1 - dir
		req, ok := s.pending[key]
		out["Matched"] = ok
		if ok {
			if function != 0 && (function != req.function || count != req.count) {
				return nil, protocolError(ErrDesynchronized, "S7 response function/count")
			}
			delete(s.pending, key)
		}
	}
	out["Pending"] = len(s.pending)
	return out, nil
}
