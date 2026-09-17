package pcaputil

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

type binWebSocket struct {
	client int
	phase  string // "http" or "frame"
	buf    []byte
	opcode byte
}

func probeWebSocket(w []byte, limit int) ProbeResult {
	if len(w) < 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	op := w[0] & 0x0f
	rsv := (w[0] >> 4) & 0x07
	fin := w[0]&0x80 != 0
	// Continuation cannot open a session. HTTP/2 frames also start with a
	// 24-bit length whose high byte is usually 0, which would look like opcode 0.
	if rsv != 0 || op == 0 || op > 0x0a || op >= 3 && op <= 7 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if op >= 8 && !fin {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := websocketFrameLength(w)
	if n < 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("websocket", "13", len(w), 2)
	}
	_ = limit
	return probeAccept("websocket", "13", 55)
}

func wsPhaseFromProbe(w []byte) string {
	if len(w) >= 3 && (w[0] == 'G' || w[0] == 'H') {
		return "http"
	}
	return "frame"
}

func websocketFrameLength(w []byte) int {
	if len(w) < 2 {
		return 0
	}
	n := int(w[1] & 0x7f)
	extra := 2
	if w[1]&0x80 != 0 {
		extra += 4
	}
	switch n {
	case 126:
		if len(w) < 4 {
			return 0
		}
		n = int(binary.BigEndian.Uint16(w[2:4]))
		extra += 2
	case 127:
		if len(w) < 10 {
			return 0
		}
		v := binary.BigEndian.Uint64(w[2:10])
		if v > 1<<20 {
			return -1
		}
		n = int(v)
		extra += 8
	}
	return extra + n
}

func (f *binFlow) frameWebSocket(w []byte) (int, *binSpec, error) {
	if f.ws == nil {
		return 0, nil, sessionContext("WebSocket upgrade was not observed")
	}
	if err := f.reserveSession(256 + int64(len(f.ws.buf))); err != nil {
		return 0, nil, err
	}
	n := websocketFrameLength(w)
	if n < 0 {
		return 0, nil, fmt.Errorf("websocket: payload too large")
	}
	if n == 0 || n > len(w) {
		return n, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	op := w[0] & 0x0f
	if (w[0]>>4)&0x07 != 0 {
		return 0, nil, fmt.Errorf("websocket: RSV must be 0")
	}
	if op > 0x0a || op >= 3 && op <= 7 {
		return 0, nil, fmt.Errorf("websocket: reserved opcode")
	}
	if op >= 8 {
		if w[0]&0x80 == 0 {
			return 0, nil, fmt.Errorf("websocket: control frames must not be fragmented")
		}
		if int(w[1]&0x7f) > 125 && w[1]&0x7f != 0x7d {
			plen := int(w[1] & 0x7f)
			if plen == 126 || plen == 127 {
				return 0, nil, fmt.Errorf("websocket: control payload too large")
			}
		}
	}
	return n, f.spec("websocket", "WebSocket"), nil
}

func (ws *binWebSocket) consume(dir int, raw []byte) (map[string]any, error) {
	op := raw[0] & 0x0f
	fin := raw[0]&0x80 != 0
	masked := raw[1]&0x80 != 0
	info := map[string]any{
		"Opcode":      uint64(op),
		"Opcode Name": websocketOpcodeName(op),
		"FIN":         fin,
		"Masked":      masked,
		"Client":      dir == ws.client,
	}
	payload := websocketPayload(raw)
	if op == 0 || !fin && op <= 2 {
		if op != 0 {
			ws.opcode, ws.buf = op, append(ws.buf[:0], payload...)
		} else {
			ws.buf = append(ws.buf, payload...)
		}
		info["Fragment"] = true
		if fin {
			info["Opcode"] = uint64(ws.opcode)
			info["Opcode Name"] = websocketOpcodeName(ws.opcode)
			info["Application"] = append([]byte(nil), ws.buf...)
			if ws.opcode == 1 && !utf8.Valid(ws.buf) {
				return info, fmt.Errorf("websocket: text is not UTF-8")
			}
			ws.buf = ws.buf[:0]
		}
		return info, nil
	}
	if op == 1 && !utf8.Valid(payload) {
		return info, fmt.Errorf("websocket: text is not UTF-8")
	}
	info["Application"] = payload
	if op == 8 && len(payload) >= 2 {
		info["Close Code"] = uint64(binary.BigEndian.Uint16(payload[:2]))
		info["Reason"] = string(payload[2:])
	}
	return info, nil
}

func websocketOpcodeName(op byte) string {
	switch op {
	case 0:
		return "continuation"
	case 1:
		return "text"
	case 2:
		return "binary"
	case 8:
		return "close"
	case 9:
		return "ping"
	case 10:
		return "pong"
	}
	return "reserved"
}

func websocketPayload(raw []byte) []byte {
	n := int(raw[1] & 0x7f)
	at := 2
	if n == 126 {
		n = int(binary.BigEndian.Uint16(raw[2:4]))
		at = 4
	} else if n == 127 {
		n = int(binary.BigEndian.Uint64(raw[2:10]))
		at = 10
	}
	var key []byte
	if raw[1]&0x80 != 0 {
		key = raw[at : at+4]
		at += 4
	}
	if at+n > len(raw) {
		n = len(raw) - at
	}
	out := append([]byte(nil), raw[at:at+n]...)
	if key != nil {
		for i := range out {
			out[i] ^= key[i%4]
		}
	}
	return out
}
