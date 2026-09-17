package pcaputil

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

type binWebSocket struct {
	client int
	phase  string
	buf    [2][]byte
	opcode [2]byte
	closed [2]bool
}

func probeWebSocket(w []byte, limit int) ProbeResult {
	if len(w) < 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	op := w[0] & 15
	if w[0]&0x70 != 0 || op == 0 || op > 10 || op >= 3 && op <= 7 || op >= 8 && w[0]&128 == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := websocketFrameLength(w)
	if n < 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		need := 4
		if w[1]&127 == 127 {
			need = 10
		}
		return probeNeed("websocket", "13", len(w), min(limit, need))
	}
	return probeAccept("websocket", "13", 55)
}

func wsPhaseFromProbe(w []byte) string { return "frame" }

func websocketFrameLength(w []byte) int {
	if len(w) < 2 {
		return 0
	}
	n, extra := int(w[1]&127), 2
	if w[1]&128 != 0 {
		extra += 4
	}
	switch n {
	case 126:
		if len(w) < 4 {
			return 0
		}
		n = int(binary.BigEndian.Uint16(w[2:4]))
		extra += 2
		if n < 126 {
			return -1
		}
	case 127:
		if len(w) < 10 {
			return 0
		}
		v := binary.BigEndian.Uint64(w[2:10])
		if v < 65536 || v > 1<<20 {
			return -1
		}
		n = int(v)
		extra += 8
	}
	return extra + n
}

func (f *binFlow) frameWebSocket(dir int, w []byte) (int, *binSpec, error) {
	ws := f.ws
	if ws == nil {
		return 0, nil, sessionContext("WebSocket upgrade was not observed")
	}
	n := websocketFrameLength(w)
	if n < 0 {
		return 0, nil, fmt.Errorf("websocket: invalid or excessive payload length")
	}
	if n > f.a.budget.MaxFrameBytes {
		return 0, nil, protocolError(ErrResourceExceeded, "WebSocket frame exceeds limit")
	}
	if n == 0 || n > len(w) {
		return n, nil, nil
	}
	op := w[0] & 15
	if w[0]&0x70 != 0 {
		return 0, nil, protocolError(ErrUnsupportedFeature, "WebSocket RSV/extension is unsupported")
	}
	if op > 10 || op >= 3 && op <= 7 {
		return 0, nil, fmt.Errorf("websocket: reserved opcode")
	}
	if op >= 8 && (w[0]&128 == 0 || w[1]&127 > 125) {
		return 0, nil, fmt.Errorf("websocket: invalid control frame")
	}
	if (w[1]&128 != 0) != (dir == ws.client) {
		return 0, nil, fmt.Errorf("websocket: mask does not match observed sender role")
	}
	if ws.closed[dir] {
		return 0, nil, fmt.Errorf("websocket: frame after Close")
	}
	if op == 0 && ws.opcode[dir] == 0 {
		return 0, nil, sessionContext("WebSocket continuation has no initial fragment")
	}
	if (op == 1 || op == 2) && ws.opcode[dir] != 0 {
		return 0, nil, fmt.Errorf("websocket: new data frame interrupts fragmented message")
	}
	target := int64(256 + cap(ws.buf[0]) + cap(ws.buf[1]))
	if op <= 2 && (op == 0 || w[0]&128 == 0) {
		header := 2
		if w[1]&127 == 126 {
			header += 2
		}
		if w[1]&127 == 127 {
			header += 8
		}
		if w[1]&128 != 0 {
			header += 4
		}
		size := len(ws.buf[dir]) + n - header
		if size > f.a.config.MaxMessageBytes {
			return 0, nil, protocolError(ErrResourceExceeded, "WebSocket fragmented message exceeds limit")
		}
		target += int64(max(0, size-cap(ws.buf[dir])))
	}
	if err := f.reserveSession(target); err != nil {
		return 0, nil, err
	}
	return n, f.spec("websocket", "WebSocket"), nil
}

func (ws *binWebSocket) consume(dir int, raw []byte) (map[string]any, error) {
	op, fin := raw[0]&15, raw[0]&128 != 0
	info := map[string]any{"Opcode": uint64(op), "Opcode Name": websocketOpcodeName(op), "FIN": fin, "Masked": raw[1]&128 != 0, "Client": dir == ws.client}
	payload := websocketPayload(raw)
	if op == 0 || !fin && op <= 2 {
		if op != 0 {
			ws.opcode[dir] = op
		}
		size := len(ws.buf[dir]) + len(payload)
		if cap(ws.buf[dir]) < size {
			buf := make([]byte, len(ws.buf[dir]), size)
			copy(buf, ws.buf[dir])
			ws.buf[dir] = buf
		}
		ws.buf[dir] = append(ws.buf[dir], payload...)
		info["Fragment"] = true
		if fin {
			opcode := ws.opcode[dir]
			if opcode == 1 && !utf8.Valid(ws.buf[dir]) {
				return info, fmt.Errorf("websocket: text is not UTF-8")
			}
			info["Opcode"], info["Opcode Name"] = uint64(opcode), websocketOpcodeName(opcode)
			info["Application"] = ws.buf[dir]
			ws.buf[dir], ws.opcode[dir] = nil, 0
		}
		return info, nil
	}
	if op == 1 && !utf8.Valid(payload) {
		return info, fmt.Errorf("websocket: text is not UTF-8")
	}
	info["Application"] = payload
	if op == 8 {
		ws.closed[dir] = true
		if len(payload) >= 2 {
			info["Close Code"] = uint64(binary.BigEndian.Uint16(payload[:2]))
			info["Reason"] = string(payload[2:])
		}
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
	n, at := int(raw[1]&127), 2
	if n == 126 {
		n = int(binary.BigEndian.Uint16(raw[2:4]))
		at = 4
	} else if n == 127 {
		n = int(binary.BigEndian.Uint64(raw[2:10]))
		at = 10
	}
	var key []byte
	if raw[1]&128 != 0 {
		key = raw[at : at+4]
		at += 4
	}
	out := append([]byte(nil), raw[at:at+n]...)
	if key != nil {
		for i := range out {
			out[i] ^= key[i%4]
		}
	}
	return out
}
